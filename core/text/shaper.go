// shaper.go — шейпинг сложного текста через go-text/typesetting (чистый Go).
//
// Гибридная схема: латиница/кириллица и прочие «простые» скрипты рисуются
// быстрым per-rune путём (кэш глифов по рунам, см. canvas.go/font.go);
// шейпинг включается только когда строка содержит руны, требующие его —
// RTL (иврит, арабский), индийские скрипты, тайский, комбинируемые знаки
// и управляющие символы bidi. Так существующие UI не платят за шейпинг
// ни микросекунды, а сложные скрипты рендерятся корректно (лигатуры,
// контекстные формы, mark positioning, порядок RTL).
//
// Кэшируется два уровня:
//   - layout строки (строка+шрифт+размер → позиционированные глифы) —
//     повторная отрисовка того же лейбла не шейпит заново;
//   - альфа-маски глифов по (шрифт, GID, размер) — в общем кэше процесса
//     (glyphstore.go): контур растеризуется один раз на процесс, а не на
//     движок.
//
// Цветные эмодзи (COLRv0-слои и растровые CBDT/sbix PNG) рендерятся в цвете —
// см. emoji.go; COLRv1 (граф paint, градиенты) пока не поддержан. Вертикальный
// текст не поддержан.

package text

import (
	"image"
	"math"
	"sync"

	"github.com/go-text/typesetting/di"
	tsfont "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// ─── Детектор необходимости шейпинга ─────────────────────────────────────────

// NeedsShaping сообщает, требует ли строка полного шейпинга.
// Консервативно: false → простой per-rune путь гарантированно корректен
// (с точностью до кернинга, который он и так применяет).
func NeedsShaping(s string) bool {
	for _, r := range s {
		if runeNeedsShaping(r) {
			return true
		}
	}
	return false
}

// runeNeedsShaping — диапазоны Unicode, где нужны лигатуры, контекстные
// формы, mark positioning или двунаправленный порядок.
func runeNeedsShaping(r rune) bool {
	switch {
	case r < 0x0300:
		return false // ASCII, Latin-1, Latin Extended
	case r <= 0x036F:
		return true // комбинируемые диакритики
	case r < 0x0590:
		return false // греческий, кириллица, армянский
	case r <= 0x08FF:
		return true // иврит, арабский, сирийский, тана, нко, самаритянский
	case r < 0x0900:
		return false
	case r <= 0x0DFF:
		return true // индийские: деванагари … сингальский
	case r <= 0x0EFF:
		return true // тайский, лаосский (mark positioning)
	case r <= 0x0FFF:
		return true // тибетский
	case r <= 0x109F:
		return true // мьянманский
	case r >= 0x1780 && r <= 0x17FF:
		return true // кхмерский
	case r >= 0x1AB0 && r <= 0x1AFF:
		return true // комбинируемые (расширение)
	case r >= 0x200C && r <= 0x200F:
		return true // ZWNJ/ZWJ/LRM/RLM
	case r >= 0x202A && r <= 0x202E:
		return true // bidi-управляющие (LRE/RLE/PDF/LRO/RLO)
	case r >= 0x2066 && r <= 0x2069:
		return true // bidi-изоляты
	case r >= 0x20D0 && r <= 0x20FF:
		return true // комбинируемые для символов
	case r >= 0xFB1D && r <= 0xFDFF:
		return true // еврейские/арабские презентационные формы A
	case r >= 0xFE70 && r <= 0xFEFF:
		return true // арабские презентационные формы B
	case r >= 0xFE00 && r <= 0xFE0F:
		return true // селекторы вариаций (VS1–VS16; VS16 — эмодзи-презентация)
	case r >= 0x1F000 && r <= 0x1FAFF:
		return true // эмодзи и пиктограммы (SMP): ZWJ-кластеры, модификаторы тона,
		//              региональные индикаторы (флаги) — собираются шейпером,
		//              цветные глифы (COLR/CBDT) рендерятся в цвете (см. emoji.go)
	default:
		return false
	}
}

// firstStrongRTL — базовое направление абзаца по первой «сильной» руне
// (упрощённое правило P2/P3 из UAX#9).
func firstStrongRTL(runes []rune) bool {
	for _, r := range runes {
		switch {
		case r >= 0x0590 && r <= 0x08FF, r >= 0xFB1D && r <= 0xFDFF, r >= 0xFE70 && r <= 0xFEFF:
			return true
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z',
			r >= 0x00C0 && r < 0x0590, r >= 0x0900 && r < 0x2000:
			return false
		}
	}
	return false
}

// ─── Fontmap: основной шрифт + fallback-цепочка ──────────────────────────────

// chainFontmap реализует shaping.Fontmap поверх шрифтов движка: сперва
// основной face, затем fallback-цепочка (те же шрифты, что в per-rune пути).
//
// Запасные шрифты загружаются лениво (fallbackfont.go): цепочка просматривается
// по порядку и догружает файлы только до первого шрифта с нужной руной.
type chainFontmap struct {
	primary   *tsfont.Face
	fallbacks []*FallbackFont
	ref       *FontCache // шрифт холста: DPI и субпиксели для загружаемых записей
	// owners — общий шрифт каждого встреченного face: раскладка несёт его шард
	// в PlacedGlyph, чтобы маски брались из кэша процесса без поиска.
	owners map[*tsfont.Face]*fontData
	shards map[*tsfont.Face]*shapedShard
}

// shardFor — шард кэша процесса для face размера sizePx (nil — face не из
// цепочки).
func (cf *chainFontmap) shardFor(face *tsfont.Face, sizePx fixed.Int26_6) *shapedShard {
	if s, ok := cf.shards[face]; ok {
		return s
	}
	var s *shapedShard
	if f := cf.owners[face]; f != nil {
		s = f.shapedShard(sizePx)
	}
	if cf.shards == nil {
		cf.shards = make(map[*tsfont.Face]*shapedShard)
	}
	cf.shards[face] = s
	return s
}

// eachFace обходит face'ы запасной цепочки по порядку, пока fn не вернёт true.
// Записи без шрифта или без typesetting-face пропускаются.
func (cf *chainFontmap) eachFace(fn func(*tsfont.Face) bool) {
	for _, fb := range cf.fallbacks {
		fc := fb.Get(cf.ref)
		if fc == nil {
			continue
		}
		if f := fc.shaperFace(); f != nil {
			cf.owners[f] = fc.font
			if fn(f) {
				return
			}
		}
	}
}

func (cf *chainFontmap) ResolveFace(r rune) *tsfont.Face {
	// Эмодзи: предпочитаем цветной шрифт (COLR/CBDT) монохромному покрытию.
	// На Windows Segoe UI Symbol (ч/б варианты многих эмодзи) стоит в цепочке
	// раньше Segoe UI Emoji — без этой ветки эмодзи вышел бы серым. Ветка
	// активна только для собственно эмодзи-рун и только при наличии цветного
	// покрытия, поэтому обычные символы (✓✗⚠) остаются на прежнем шрифте.
	if isEmojiRune(r) {
		if _, ok := cf.primary.NominalGlyph(r); ok && faceHasColor(cf.primary) {
			return cf.primary
		}
		var colored *tsfont.Face
		cf.eachFace(func(f *tsfont.Face) bool {
			if !faceHasColor(f) {
				return false
			}
			if _, ok := f.NominalGlyph(r); ok {
				colored = f
				return true
			}
			return false
		})
		if colored != nil {
			return colored
		}
	}
	if _, ok := cf.primary.NominalGlyph(r); ok {
		return cf.primary
	}
	var found *tsfont.Face
	cf.eachFace(func(f *tsfont.Face) bool {
		if _, ok := f.NominalGlyph(r); ok {
			found = f
			return true
		}
		return false
	})
	if found != nil {
		return found
	}
	return cf.primary // контракт Fontmap: всегда не-nil
}

// isEmojiRune — руна из эмодзи-блоков SMP (Emoticons, Transport, Symbols &
// Pictographs и расширения), для которых предпочтителен цветной шрифт.
// Ограничено ≥0x1F000, чтобы не менять шрифт для BMP-символов (☺✓✈), которые
// исторически рисуются монохромным fallback'ом.
func isEmojiRune(r rune) bool {
	return r >= 0x1F000 && r <= 0x1FAFF
}

// faceHasColor сообщает, есть ли у шрифта цветные глифы (COLR или растровые
// CBDT/sbix). Дешёвая проверка полей — вызывается при шейпинге эмодзи.
func faceHasColor(f *tsfont.Face) bool {
	return f != nil && f.Font != nil && (f.COLR != nil || len(f.BitmapSizes()) > 0)
}

// ─── Кэшированный layout строки ──────────────────────────────────────────────

// PlacedGlyph — один глиф готового layout'а: face+GID и геометрия
// относительно текущей позиции пера (в fixed 26.6).
type PlacedGlyph struct {
	Face       *tsfont.Face
	GID        tsfont.GID
	XOff, YOff fixed.Int26_6 // смещение точки отрисовки от пера
	Adv        fixed.Int26_6 // продвижение пера
	Cluster    int           // индекс первой руны кластера (логический)

	// shard — шард кэша процесса (общий шрифт face, этот размер): маски и
	// цветные глифы берутся из него без поиска; nil у глифа, собранного не
	// раскладкой, — такой растеризуется без кэша.
	shard *shapedShard
}

// ShapedLayout — результат шейпинга строки: глифы в видимом порядке
// (слева направо) и полная ширина.
type ShapedLayout struct {
	Glyphs []PlacedGlyph
	Width  fixed.Int26_6
}

type layoutKey struct {
	fc   *FontCache
	size float64
	text string
}

// GlyphMask — растеризованная маска глифа с оффсетом от точки отрисовки.
type GlyphMask struct {
	Mask       *image.Alpha // nil — пустой глиф (пробел) или контур недоступен
	OffX, OffY int
}

// Пределы кэшей: при переполнении кэш сбрасывается целиком (простая política,
// как у кэша глифов FontCache). Маски и цветные глифы лежат в шардах кэша
// процесса (glyphstore.go), пределы — на шард.
const (
	maxShapedLayouts  = 2048
	maxShapedMasks    = 4096
	maxDynColorGlyphs = 256 // цветозависимые эмодзи: цвет текста меняется редко
)

// Shaper — состояние шейпинга одного Canvas и его клонов (один на линию
// клонов: личные typesetting-face видов читаются только под его mu). Все
// обращения — под mu (Segmenter/HarfbuzzShaper/face не потокобезопасны).
// Маски и цветные глифы — в общем кэше процесса, под своими замками.
type Shaper struct {
	mu      sync.Mutex
	seg     shaping.Segmenter
	hb      shaping.HarfbuzzShaper
	layouts map[layoutKey]*ShapedLayout
}

// CacheSizes — число раскладок шейпера и цветных глифов в общем кэше
// процесса (для тестов и диагностики).
func (ts *Shaper) CacheSizes() (layouts, colorGlyphs int) {
	ts.mu.Lock()
	layouts = len(ts.layouts)
	ts.mu.Unlock()
	return layouts, store.colorGlyphCount()
}

// DropLayouts сбрасывает кэш раскладок: его ключ не учитывает DPI.
func (ts *Shaper) DropLayouts() {
	ts.mu.Lock()
	ts.layouts = nil
	ts.mu.Unlock()
}

// Layout возвращает кэшированный (или строит новый) layout строки.
// Возвращает nil, если шейпинг недоступен (нет typesetting-face).
func (ts *Shaper) Layout(fc *FontCache, fallbacks []*FallbackFont, text string, sizePt float64) *ShapedLayout {
	primary := fc.shaperFace()
	if primary == nil {
		return nil
	}
	key := layoutKey{fc: fc, size: sizePt, text: text}

	ts.mu.Lock()
	defer ts.mu.Unlock()
	if l, ok := ts.layouts[key]; ok {
		return l
	}

	fm := &chainFontmap{primary: primary, fallbacks: fallbacks, ref: fc,
		owners: map[*tsfont.Face]*fontData{primary: fc.font}}

	runes := []rune(text)
	sizePx := fc.PixelSize(sizePt)
	base := di.DirectionLTR
	if firstStrongRTL(runes) {
		base = di.DirectionRTL
	}
	in := shaping.Input{
		Text:      runes,
		RunStart:  0,
		RunEnd:    len(runes),
		Face:      primary,
		Size:      sizePx,
		Direction: base,
	}

	runs := ts.seg.Split(in, fm) // логический порядок, Direction/Script/Face решены
	outs := make([]shaping.Output, len(runs))
	for i, run := range runs {
		outs[i] = ts.hb.Shape(run)
	}
	reorderRunsVisual(outs, base)

	l := &ShapedLayout{}
	for _, out := range outs {
		for _, g := range out.Glyphs {
			l.Glyphs = append(l.Glyphs, PlacedGlyph{
				Face:    out.Face,
				GID:     g.GlyphID,
				XOff:    g.XOffset,
				YOff:    g.YOffset,
				Adv:     g.Advance,
				Cluster: g.ClusterIndex,
				shard:   fm.shardFor(out.Face, sizePx),
			})
		}
		l.Width += out.Advance
	}

	if ts.layouts == nil {
		ts.layouts = make(map[layoutKey]*ShapedLayout)
	} else if len(ts.layouts) >= maxShapedLayouts {
		ts.layouts = make(map[layoutKey]*ShapedLayout, maxShapedLayouts/4)
	}
	ts.layouts[key] = l
	return l
}

// reorderRunsVisual переупорядочивает runs из логического порядка в видимый.
// Двухуровневая аппроксимация UAX#9 L2 (достаточно для UI-строк без
// вложенных embedding-уровней):
//   - база LTR: каждая максимальная группа подряд идущих RTL-runs
//     переворачивается;
//   - база RTL: весь список переворачивается, затем внутри него
//     переворачиваются группы LTR-runs (восстанавливая их порядок).
func reorderRunsVisual(outs []shaping.Output, base di.Direction) {
	if len(outs) < 2 {
		return
	}
	if base == di.DirectionRTL {
		reverseOutputs(outs)
		reverseGroups(outs, di.DirectionLTR)
		return
	}
	reverseGroups(outs, di.DirectionRTL)
}

// reverseGroups переворачивает каждую максимальную группу подряд идущих
// runs с направлением dir.
func reverseGroups(outs []shaping.Output, dir di.Direction) {
	i := 0
	for i < len(outs) {
		if outs[i].Direction != dir {
			i++
			continue
		}
		j := i
		for j < len(outs) && outs[j].Direction == dir {
			j++
		}
		reverseOutputs(outs[i:j])
		i = j
	}
}

func reverseOutputs(outs []shaping.Output) {
	for a, b := 0, len(outs)-1; a < b; a, b = a+1, b-1 {
		outs[a], outs[b] = outs[b], outs[a]
	}
}

// ─── Растеризация глифа по GID ───────────────────────────────────────────────

// GlyphMaskFor возвращает альфа-маску глифа раскладки g размера sizePx
// (кэшируется в общем кэше процесса по шрифту, GID и размеру). Глиф без
// владельца (собранный не раскладкой) растеризуется без кэша и без хинтинга.
func (ts *Shaper) GlyphMaskFor(g PlacedGlyph, sizePx fixed.Int26_6, hint bool) *GlyphMask {
	if g.shard == nil {
		if g.Face == nil {
			return &GlyphMask{}
		}
		return rasterizeGlyph(g.Face, g.GID, sizePx, nil)
	}
	return g.shard.mask(g.GID, hint)
}

// rasterizeGlyph растеризует контур глифа в альфа-маску.
// Координаты контура — в font units (ось Y вверх); масштаб sizePx/upem,
// при отрисовке Y инвертируется (растровая ось — вниз).
//
// vh != nil — вертикальный автохинтинг: Y-координата каждой точки контура
// проходит через отображение зон (texthint.go), X не меняется.
func rasterizeGlyph(face *tsfont.Face, gid tsfont.GID, sizePx fixed.Int26_6, vh *vHint) *GlyphMask {
	outline, ok := face.GlyphData(gid).(tsfont.GlyphOutline)
	if !ok || len(outline.Segments) == 0 {
		return &GlyphMask{} // пробел/битмап-глиф — рисовать нечего
	}
	ext, ok := face.GlyphExtents(gid)
	if !ok || ext.Width == 0 || ext.Height == 0 {
		return &GlyphMask{}
	}

	scale := float32(sizePx) / 64.0 / float32(face.Upem())

	// Экстенты в font units: XBearing/YBearing — левый верхний угол,
	// Width>0, Height<0 (ось вверх). Переводим в растровые пиксели.
	x0 := ext.XBearing * scale
	y0 := -ext.YBearing * scale // верх глифа относительно baseline (ось вниз)
	x1 := (ext.XBearing + ext.Width) * scale
	y1 := -(ext.YBearing + ext.Height) * scale

	// fy — Y точки контура в растровых пикселях (ось вниз). С хинтингом
	// высота (26.6, ось вверх) проходит через отображение зон.
	fy := func(y float32) float32 { return -y * scale }
	if vh != nil {
		// Штрихи этого глифа (перекладины, полки) — отображение на глиф.
		fx := func(v float32) int { return int(math.Floor(float64(v*scale*64) + 0.5)) }
		var sc hintScan
		for _, seg := range outline.Segments {
			switch seg.Op {
			case ot.SegmentOpMoveTo:
				sc.moveTo(fx(seg.Args[0].X), fx(seg.Args[0].Y))
			case ot.SegmentOpLineTo:
				sc.lineTo(fx(seg.Args[0].X), fx(seg.Args[0].Y))
			case ot.SegmentOpQuadTo:
				sc.curveTo(fx(seg.Args[1].X), fx(seg.Args[1].Y))
			case ot.SegmentOpCubeTo:
				sc.curveTo(fx(seg.Args[2].X), fx(seg.Args[2].Y))
			}
		}
		vh = vh.forGlyph(&sc, int(sizePx), fx(ext.YBearing), fx(ext.YBearing+ext.Height))
		fy = func(y float32) float32 {
			return -float32(vh.apply(int(math.Floor(float64(y*scale*64)+0.5)))) / 64
		}
		// Размах по Y после выравнивания — по всем точкам контура.
		y0, y1 = float32(math.MaxFloat32), -float32(math.MaxFloat32)
		note := func(y float32) {
			v := fy(y)
			if v < y0 {
				y0 = v
			}
			if v > y1 {
				y1 = v
			}
		}
		for _, seg := range outline.Segments {
			n := 1
			switch seg.Op {
			case ot.SegmentOpQuadTo:
				n = 2
			case ot.SegmentOpCubeTo:
				n = 3
			}
			for i := 0; i < n; i++ {
				note(seg.Args[i].Y)
			}
		}
	}

	minX, minY := floorF32(x0), floorF32(y0)
	w := ceilF32(x1) - minX + 1
	h := ceilF32(y1) - minY + 1
	if !glyphBudgetOK(w, h) {
		return &GlyphMask{}
	}

	r := vector.NewRasterizer(w, h)
	offX, offY := float32(minX), float32(minY)
	started := false // открыт ли текущий контур (замыкаем перед новым MoveTo)
	for _, seg := range outline.Segments {
		switch seg.Op {
		case ot.SegmentOpMoveTo:
			if started {
				r.ClosePath()
			}
			p := seg.Args[0]
			r.MoveTo(p.X*scale-offX, fy(p.Y)-offY)
			started = true
		case ot.SegmentOpLineTo:
			p := seg.Args[0]
			r.LineTo(p.X*scale-offX, fy(p.Y)-offY)
		case ot.SegmentOpQuadTo:
			c, p := seg.Args[0], seg.Args[1]
			r.QuadTo(c.X*scale-offX, fy(c.Y)-offY, p.X*scale-offX, fy(p.Y)-offY)
		case ot.SegmentOpCubeTo:
			c1, c2, p := seg.Args[0], seg.Args[1], seg.Args[2]
			r.CubeTo(c1.X*scale-offX, fy(c1.Y)-offY,
				c2.X*scale-offX, fy(c2.Y)-offY,
				p.X*scale-offX, fy(p.Y)-offY)
		}
	}
	if started {
		r.ClosePath()
	}

	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	r.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})

	return &GlyphMask{Mask: mask, OffX: minX, OffY: minY}
}

func floorF32(v float32) int {
	i := int(v)
	if v < 0 && float32(i) != v {
		i--
	}
	return i
}

func ceilF32(v float32) int {
	i := int(v)
	if v > 0 && float32(i) != v {
		i++
	}
	return i
}
