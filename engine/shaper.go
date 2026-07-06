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
//   - альфа-маски глифов по (face, GID, размер) — контур растеризуется
//     через golang.org/x/image/vector один раз.
//
// Ограничения v1: эмодзи-битмапы и цветные глифы не растеризуются
// (пропускаются с продвижением пера); вертикальный текст не поддержан.
package engine

import (
	"image"
	"sync"

	"github.com/go-text/typesetting/di"
	tsfont "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// ─── Детектор необходимости шейпинга ─────────────────────────────────────────

// needsShaping сообщает, требует ли строка полного шейпинга.
// Консервативно: false → простой per-rune путь гарантированно корректен
// (с точностью до кернинга, который он и так применяет).
func needsShaping(s string) bool {
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
type chainFontmap struct {
	primary   *tsfont.Face
	fallbacks []*tsfont.Face
}

func (cf *chainFontmap) ResolveFace(r rune) *tsfont.Face {
	if _, ok := cf.primary.NominalGlyph(r); ok {
		return cf.primary
	}
	for _, f := range cf.fallbacks {
		if f == nil {
			continue
		}
		if _, ok := f.NominalGlyph(r); ok {
			return f
		}
	}
	return cf.primary // контракт Fontmap: всегда не-nil
}

// ─── Кэшированный layout строки ──────────────────────────────────────────────

// placedGlyph — один глиф готового layout'а: face+GID и геометрия
// относительно текущей позиции пера (в fixed 26.6).
type placedGlyph struct {
	face       *tsfont.Face
	gid        tsfont.GID
	xOff, yOff fixed.Int26_6 // смещение точки отрисовки от пера
	adv        fixed.Int26_6 // продвижение пера
	cluster    int           // индекс первой руны кластера (логический)
}

// shapedLayout — результат шейпинга строки: глифы в видимом порядке
// (слева направо) и полная ширина.
type shapedLayout struct {
	glyphs []placedGlyph
	width  fixed.Int26_6
}

type layoutKey struct {
	fc   *FontCache
	size float64
	text string
}

type maskKey struct {
	face   *tsfont.Face
	gid    tsfont.GID
	sizePx fixed.Int26_6
}

// glyphMask — растеризованная маска глифа с оффсетом от точки отрисовки.
type glyphMask struct {
	mask       *image.Alpha // nil — пустой глиф (пробел) или контур недоступен
	offX, offY int
}

// Пределы кэшей: при переполнении кэш сбрасывается целиком (простая política,
// как у кэша глифов FontCache).
const (
	maxShapedLayouts = 2048
	maxShapedMasks   = 4096
)

// textShaper — состояние шейпинга одного Canvas. Все обращения — под mu
// (Segmenter/HarfbuzzShaper/vector.Rasterizer не потокобезопасны).
type textShaper struct {
	mu      sync.Mutex
	seg     shaping.Segmenter
	hb      shaping.HarfbuzzShaper
	layouts map[layoutKey]*shapedLayout
	masks   map[maskKey]*glyphMask
}

// layout возвращает кэшированный (или строит новый) layout строки.
// Возвращает nil, если шейпинг недоступен (нет typesetting-face).
func (ts *textShaper) layout(fc *FontCache, fallbacks []*FontCache, text string, sizePt float64) *shapedLayout {
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

	fm := &chainFontmap{primary: primary}
	for _, fb := range fallbacks {
		if f := fb.shaperFace(); f != nil {
			fm.fallbacks = append(fm.fallbacks, f)
		}
	}

	runes := []rune(text)
	sizePx := fixed.Int26_6(sizePt * fc.dpi / 72.0 * 64.0)
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

	l := &shapedLayout{}
	for _, out := range outs {
		for _, g := range out.Glyphs {
			l.glyphs = append(l.glyphs, placedGlyph{
				face:    out.Face,
				gid:     g.GlyphID,
				xOff:    g.XOffset,
				yOff:    g.YOffset,
				adv:     g.Advance,
				cluster: g.ClusterIndex,
			})
		}
		l.width += out.Advance
	}

	if ts.layouts == nil {
		ts.layouts = make(map[layoutKey]*shapedLayout)
	} else if len(ts.layouts) >= maxShapedLayouts {
		ts.layouts = make(map[layoutKey]*shapedLayout, maxShapedLayouts/4)
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

// glyphMaskFor возвращает альфа-маску глифа (кэшируется по face+GID+размеру).
func (ts *textShaper) glyphMaskFor(face *tsfont.Face, gid tsfont.GID, sizePx fixed.Int26_6) *glyphMask {
	key := maskKey{face: face, gid: gid, sizePx: sizePx}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if m, ok := ts.masks[key]; ok {
		return m
	}

	m := rasterizeGlyph(face, gid, sizePx)

	if ts.masks == nil {
		ts.masks = make(map[maskKey]*glyphMask)
	} else if len(ts.masks) >= maxShapedMasks {
		ts.masks = make(map[maskKey]*glyphMask, maxShapedMasks/4)
	}
	ts.masks[key] = m
	return m
}

// rasterizeGlyph растеризует контур глифа в альфа-маску.
// Координаты контура — в font units (ось Y вверх); масштаб sizePx/upem,
// при отрисовке Y инвертируется (растровая ось — вниз).
func rasterizeGlyph(face *tsfont.Face, gid tsfont.GID, sizePx fixed.Int26_6) *glyphMask {
	outline, ok := face.GlyphData(gid).(tsfont.GlyphOutline)
	if !ok || len(outline.Segments) == 0 {
		return &glyphMask{} // пробел/битмап-глиф — рисовать нечего
	}
	ext, ok := face.GlyphExtents(gid)
	if !ok || ext.Width == 0 || ext.Height == 0 {
		return &glyphMask{}
	}

	scale := float32(sizePx) / 64.0 / float32(face.Upem())

	// Экстенты в font units: XBearing/YBearing — левый верхний угол,
	// Width>0, Height<0 (ось вверх). Переводим в растровые пиксели.
	x0 := ext.XBearing * scale
	y0 := -ext.YBearing * scale // верх глифа относительно baseline (ось вниз)
	x1 := (ext.XBearing + ext.Width) * scale
	y1 := -(ext.YBearing + ext.Height) * scale

	minX, minY := floorF32(x0), floorF32(y0)
	w := ceilF32(x1) - minX + 1
	h := ceilF32(y1) - minY + 1
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return &glyphMask{}
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
			r.MoveTo(p.X*scale-offX, -p.Y*scale-offY)
			started = true
		case ot.SegmentOpLineTo:
			p := seg.Args[0]
			r.LineTo(p.X*scale-offX, -p.Y*scale-offY)
		case ot.SegmentOpQuadTo:
			c, p := seg.Args[0], seg.Args[1]
			r.QuadTo(c.X*scale-offX, -c.Y*scale-offY, p.X*scale-offX, -p.Y*scale-offY)
		case ot.SegmentOpCubeTo:
			c1, c2, p := seg.Args[0], seg.Args[1], seg.Args[2]
			r.CubeTo(c1.X*scale-offX, -c1.Y*scale-offY,
				c2.X*scale-offX, -c2.Y*scale-offY,
				p.X*scale-offX, -p.Y*scale-offY)
		}
	}
	if started {
		r.ClosePath()
	}

	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	r.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})

	return &glyphMask{mask: mask, offX: minX, offY: minY}
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
