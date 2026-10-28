package text

import (
	"image"
	"image/draw"
	"math"
	"sort"

	tsfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Лёгкий автохинтинг по вертикали (TextQuality.Hinting).
//
// x/image не хинтует контуры: HintingFull там лишь округляет шаг глифа. Здесь
// контур глифа правится ДО растеризации: по эталонным буквам находятся
// высотные зоны шрифта (базовая линия, строчные, заглавные, верх выносных
// элементов, низ нижних) в пикселях нужного кегля, затем координата Y каждой
// точки контура переводится кусочно-линейной функцией, у которой эти высоты
// приходятся на целые пиксели. Координата X не трогается — шаги, кернинг и
// ширина строки прежние. Всё считается в целых числах 26.6, поэтому маска
// побайтно одинакова на любой платформе.

// vHint — кусочно-линейное отображение высоты (26.6, ось вверх от базовой
// линии) из исходных размеров глифа в выровненные. Узлы src строго возрастают,
// dst не убывает.
type vHint struct {
	src, dst []int
}

// apply переводит высоту; за крайними узлами — сдвиг без растяжения.
func (h *vHint) apply(up int) int {
	n := len(h.src)
	if up <= h.src[0] {
		return h.dst[0] + (up - h.src[0])
	}
	if up >= h.src[n-1] {
		return h.dst[n-1] + (up - h.src[n-1])
	}
	i := sort.SearchInts(h.src, up+1) - 1 // src[i] <= up < src[i+1]
	s0, s1 := h.src[i], h.src[i+1]
	d0, d1 := h.dst[i], h.dst[i+1]
	return d0 + divRoundInt((up-s0)*(d1-d0), s1-s0)
}

// glyphVExtent сообщает вертикальный размах контура буквы r: верх и низ в
// 26.6 пикселя, ось вверх (верх строчной «x» — положительное число).
type glyphVExtent func(r rune) (top, bottom int, ok bool)

// Эталонные буквы зон: латиница, затем кириллица (берётся первая найденная).
var (
	hintRefX    = []rune{'x', 'х'}      // плоский верх строчных
	hintRefO    = []rune{'o', 'о'}      // круглая строчная: выступ вверх и вниз
	hintRefCap  = []rune{'H', 'Н'}      // плоский верх заглавных
	hintRefCapO = []rune{'O', 'О'}      // круглая заглавная
	hintRefAsc  = []rune{'l', 'd', 'b'} // верх выносных элементов
	hintRefDesc = []rune{'p', 'q', 'р'} // низ нижних элементов
)

func firstExtent(ext glyphVExtent, refs []rune) (top, bottom int, ok bool) {
	for _, r := range refs {
		if top, bottom, ok = ext(r); ok {
			return
		}
	}
	return 0, 0, false
}

// round26 округляет 26.6 до целого пикселя (половина — вверх).
func round26(v int) int { return ((v + 32) >> 6) << 6 }

// hintXHeightBias — доля пикселя (в 1/64), с которой высота строчных округляется
// ВВЕРХ, а не к ближайшему: строчные получают чуть больше чернил (так же
// поступает автохинтер FreeType). 64−bias — порог дробной части.
const hintXHeightBias = 26

// overshoot переводит выступ круглой буквы за плоскую зону (26.6, ≥ 0) в
// выровненный: меньше полупикселя сливается с зоной, больше — округляется.
func overshoot(d int) int {
	if d < 32 {
		return 0
	}
	return round26(d)
}

// buildVHint строит отображение по эталонным буквам; nil — зон не нашлось
// (шрифт без латиницы и кириллицы), глиф тогда рисуется без хинтинга.
func buildVHint(ext glyphVExtent) *vHint {
	xTop, _, okX := firstExtent(ext, hintRefX)
	oTop, oBot, okO := firstExtent(ext, hintRefO)
	if !okX {
		xTop, okX = oTop, okO // нет плоской «x» — верх круглой «o» как высота строчных
		oTop = xTop
	}
	capTop, _, okC := firstExtent(ext, hintRefCap)
	capOTop, _, okCO := firstExtent(ext, hintRefCapO)
	if !okC {
		capTop, okC = capOTop, okCO
		capOTop = capTop
	}
	if !okX && !okC {
		return nil
	}

	type node struct{ src, dst int }
	var ns []node
	add := func(src, dst int) { ns = append(ns, node{src, dst}) }

	// Базовая линия: плоский низ — ноль, круглый («o») прижимается к нему.
	add(0, 0)
	if okO && oBot < 0 {
		add(oBot, -overshoot(-oBot))
	}

	prev := 0 // выровненная высота предыдущей зоны — зоны не должны слипаться
	if okX && xTop > 0 {
		dx := (xTop + 64 - hintXHeightBias) >> 6 << 6
		if dx < prev+64 {
			dx = prev + 64
		}
		add(xTop, dx)
		if okO && oTop > xTop {
			add(oTop, dx+overshoot(oTop-xTop))
		}
		prev = dx
	}
	if okC && capTop > 0 {
		dc := round26(capTop)
		if dc < prev+64 {
			dc = prev + 64
		}
		add(capTop, dc)
		if okCO && capOTop > capTop {
			add(capOTop, dc+overshoot(capOTop-capTop))
		}
		prev = dc
	}
	if aTop, _, ok := firstExtent(ext, hintRefAsc); ok && aTop > 0 {
		da := round26(aTop)
		if da < prev {
			da = prev
		}
		add(aTop, da)
	}
	if _, dBot, ok := firstExtent(ext, hintRefDesc); ok && dBot < 0 {
		dd := round26(dBot)
		if dd > -64 {
			dd = -64
		}
		add(dBot, dd)
	}

	sort.SliceStable(ns, func(i, j int) bool { return ns[i].src < ns[j].src })
	h := &vHint{}
	for _, n := range ns {
		if k := len(h.src); k > 0 {
			if n.src == h.src[k-1] { // совпавшие высоты — берём большую цель
				if n.dst > h.dst[k-1] {
					h.dst[k-1] = n.dst
				}
				continue
			}
			if n.dst < h.dst[k-1] {
				n.dst = h.dst[k-1]
			}
		}
		h.src = append(h.src, n.src)
		h.dst = append(h.dst, n.dst)
	}
	if len(h.src) < 2 {
		return nil
	}
	return h
}

// ppem — размер em в пикселях в 26.6 — ровно так, как его считает
// opentype.NewFace (чтобы контуры и шаги глифов были одного масштаба).
func (fc *FontCache) ppem(sizePt float64) fixed.Int26_6 {
	return fixed.Int26_6(0.5 + (sizePt * fc.dpi * 64 / 72))
}

// vhintLocked возвращает (и кэширует) отображение высот для кегля. Под fc.mu.
func (fc *FontCache) vhintLocked(ppem fixed.Int26_6, buf *sfnt.Buffer) *vHint {
	if h, ok := fc.hints[ppem]; ok {
		return h
	}
	h := buildVHint(func(r rune) (int, int, bool) {
		idx, err := fc.TTF.GlyphIndex(buf, r)
		if err != nil || idx == 0 {
			return 0, 0, false
		}
		segs, err := fc.TTF.LoadGlyph(buf, idx, ppem, nil)
		if err != nil || len(segs) == 0 {
			return 0, 0, false
		}
		b := segs.Bounds()
		return -int(b.Min.Y), -int(b.Max.Y), true
	})
	if fc.hints == nil {
		fc.hints = make(map[fixed.Int26_6]*vHint)
	}
	fc.hints[ppem] = h
	return h
}

// hintedGlyph растеризует глиф r своим растеризатором: с вертикальным
// выравниванием контура (q.Hinting) и/или в тройном разрешении по горизонтали
// для LCD (q.LCD). Шаг и положение маски по горизонтали — как у
// opentype.Face.Glyph (тот же face даёт шаг), поэтому на раскладку режим не
// влияет. ok=false — не вышло (нет зон, глифа нет), вызывающий откатывается на
// обычный путь. Под fc.mu.
func (fc *FontCache) hintedGlyph(face font.Face, sizePt float64, r rune, phase int, q TextQuality) (CachedGlyph, bool) {
	buf := &fc.buf // под fc.mu; сегменты ниже живут до следующего обращения к нему
	ppem := fc.ppem(sizePt)
	var h *vHint
	if q.Hinting {
		h = fc.vhintLocked(ppem, buf)
		if h == nil && !q.LCD {
			return CachedGlyph{}, false
		}
	}
	idx, err := fc.TTF.GlyphIndex(buf, r)
	if err != nil {
		return CachedGlyph{}, false
	}
	adv, ok := face.GlyphAdvance(r)
	if !ok {
		return CachedGlyph{}, false
	}
	segs, err := fc.TTF.LoadGlyph(buf, idx, ppem, nil)
	if err != nil {
		return CachedGlyph{}, false
	}
	dotX := fixed.Int26_6(phase * (64 / SubpixelPhases))
	if h != nil {
		var sc hintScan
		for _, sg := range segs {
			switch sg.Op {
			case sfnt.SegmentOpMoveTo:
				sc.moveTo(int(sg.Args[0].X), -int(sg.Args[0].Y))
			case sfnt.SegmentOpLineTo:
				sc.lineTo(int(sg.Args[0].X), -int(sg.Args[0].Y))
			case sfnt.SegmentOpQuadTo:
				sc.curveTo(int(sg.Args[1].X), -int(sg.Args[1].Y))
			case sfnt.SegmentOpCubeTo:
				sc.curveTo(int(sg.Args[2].X), -int(sg.Args[2].Y))
			}
		}
		gb := segs.Bounds() // sfnt: ось Y вниз
		h = h.forGlyph(&sc, int(ppem), -int(gb.Min.Y), -int(gb.Max.Y))
	}
	xs := fixed.Int26_6(1)
	if q.LCD {
		xs = 3 // три подпикселя на пиксель по горизонтали
	}
	for i := range segs {
		n := 1
		switch segs[i].Op {
		case sfnt.SegmentOpQuadTo:
			n = 2
		case sfnt.SegmentOpCubeTo:
			n = 3
		}
		for j := 0; j < n; j++ {
			p := &segs[i].Args[j]
			p.X = (p.X + dotX) * xs
			if h != nil {
				p.Y = -fixed.Int26_6(h.apply(-int(p.Y))) // sfnt: ось Y вниз
			}
		}
	}

	g := CachedGlyph{Advance: adv, OK: true}
	b := segs.Bounds()
	if len(segs) == 0 {
		return g, true
	}
	if q.LCD {
		fc.rasterLCD(&g, segs, b, q.LCDOrder)
		return g, true
	}
	dr := image.Rect(b.Min.X.Floor(), b.Min.Y.Floor(), b.Max.X.Ceil(), b.Max.Y.Ceil())
	w, hgt := dr.Dx(), dr.Dy()
	if w <= 0 || hgt <= 0 {
		return g, true
	}
	g.OffX, g.OffY = dr.Min.X, dr.Min.Y
	m := image.NewAlpha(image.Rect(0, 0, w, hgt))
	fc.rasterSegs(m, segs, -fixed.Int26_6(dr.Min.X<<6), -fixed.Int26_6(dr.Min.Y<<6))
	g.Mask = m
	return g, true
}

// rasterSegs рисует контур в маску m (покрытие, Src) со сдвигом (biasX, biasY).
func (fc *FontCache) rasterSegs(m *image.Alpha, segs sfnt.Segments, biasX, biasY fixed.Int26_6) {
	if fc.rast == nil {
		fc.rast = vector.NewRasterizer(m.Rect.Dx(), m.Rect.Dy())
	}
	rast := fc.rast
	rast.Reset(m.Rect.Dx(), m.Rect.Dy())
	rast.DrawOp = draw.Src
	for _, seg := range segs {
		switch seg.Op {
		case sfnt.SegmentOpMoveTo:
			rast.MoveTo(float32(seg.Args[0].X+biasX)/64, float32(seg.Args[0].Y+biasY)/64)
		case sfnt.SegmentOpLineTo:
			rast.LineTo(float32(seg.Args[0].X+biasX)/64, float32(seg.Args[0].Y+biasY)/64)
		case sfnt.SegmentOpQuadTo:
			rast.QuadTo(
				float32(seg.Args[0].X+biasX)/64, float32(seg.Args[0].Y+biasY)/64,
				float32(seg.Args[1].X+biasX)/64, float32(seg.Args[1].Y+biasY)/64)
		case sfnt.SegmentOpCubeTo:
			rast.CubeTo(
				float32(seg.Args[0].X+biasX)/64, float32(seg.Args[0].Y+biasY)/64,
				float32(seg.Args[1].X+biasX)/64, float32(seg.Args[1].Y+biasY)/64,
				float32(seg.Args[2].X+biasX)/64, float32(seg.Args[2].Y+biasY)/64)
		}
	}
	rast.Draw(m, m.Bounds(), image.Opaque, image.Point{})
}

// lcdFilter — веса фильтра FreeType FT_LCD_FILTER_DEFAULT (сумма 256).
var lcdFilter = [5]int{0x08, 0x4D, 0x56, 0x4D, 0x08}

// LCDMask — покрытие глифа по цветным каналам: три байта на пиксель в
// ЛОГИЧЕСКОМ порядке R, G, B (какой подпиксель слева — решает LCDOrder при
// построении).
type LCDMask struct {
	W, H int
	Pix  []uint8
}

// rasterLCD растеризует контур segs (X уже утроен) в тройном разрешении по
// горизонтали и сворачивает фильтром FreeType в покрытие по каналам. Маска
// шире глифа на пиксель с каждой стороны: фильтр растекается на два подпикселя.
func (fc *FontCache) rasterLCD(g *CachedGlyph, segs sfnt.Segments, b fixed.Rectangle26_6, order LCDOrder) {
	minX3, maxX3 := b.Min.X.Floor(), b.Max.X.Ceil()
	y0, y1 := b.Min.Y.Floor(), b.Max.Y.Ceil()
	if maxX3 <= minX3 || y1 <= y0 {
		return
	}
	x0 := floorDiv(minX3-2, 3) // пиксель маски, левее которого покрытия нет
	x1 := -floorDiv(-(maxX3 + 2), 3)
	wpx, hgt := x1-x0, y1-y0
	sub := image.NewAlpha(image.Rect(0, 0, wpx*3, hgt))
	fc.rasterSegs(sub, segs, -fixed.Int26_6(x0*3<<6), -fixed.Int26_6(y0<<6))

	lm := &LCDMask{W: wpx, H: hgt, Pix: make([]uint8, wpx*hgt*3)}
	for y := 0; y < hgt; y++ {
		row := sub.Pix[y*sub.Stride : y*sub.Stride+wpx*3]
		for i := 0; i < wpx*3; i++ {
			sum := 128
			for k := -2; k <= 2; k++ {
				if j := i + k; j >= 0 && j < len(row) {
					sum += lcdFilter[k+2] * int(row[j])
				}
			}
			v := uint8(sum >> 8)
			px, ch := i/3, i%3 // ch — подпиксель слева направо
			if order == LCDOrderBGR {
				ch = 2 - ch
			}
			lm.Pix[(y*wpx+px)*3+ch] = v
		}
	}
	g.LCD = lm
	g.OffX, g.OffY = x0, y0
}

// floorDiv — деление с округлением вниз (d > 0).
func floorDiv(n, d int) int {
	q := n / d
	if n%d != 0 && n < 0 {
		q--
	}
	return q
}

// hintKey — ключ отображения высот шейпера: шрифт и размер em в 26.6.
type hintKey struct {
	face   *tsfont.Face
	sizePx fixed.Int26_6
}

// vhintFor возвращает отображение высот для typesetting-face (кэшируется).
// Тот же автохинтер, что и у per-rune пути, но размах эталонных букв берётся
// из экстентов typesetting. Под ts.mu.
func (ts *Shaper) vhintFor(face *tsfont.Face, sizePx fixed.Int26_6) *vHint {
	k := hintKey{face, sizePx}
	if h, ok := ts.vhints[k]; ok {
		return h
	}
	scale := float32(sizePx) / 64.0 / float32(face.Upem())
	toFixed := func(v float32) int { return int(math.Floor(float64(v*scale*64) + 0.5)) }
	h := buildVHint(func(r rune) (int, int, bool) {
		gid, ok := face.NominalGlyph(r)
		if !ok || gid == 0 {
			return 0, 0, false
		}
		ext, ok := face.GlyphExtents(gid)
		if !ok || ext.Width == 0 || ext.Height == 0 {
			return 0, 0, false
		}
		return toFixed(ext.YBearing), toFixed(ext.YBearing + ext.Height), true
	})
	if ts.vhints == nil {
		ts.vhints = make(map[hintKey]*vHint)
	}
	ts.vhints[k] = h
	return h
}

// ── Горизонтальные штрихи ───────────────────────────────────────────────────
//
// Зон по высоте мало: перекладина «e», «H», «A», полки «E», «Z», «5» лежат
// между ними и без выравнивания делятся на две строки пикселей. Для каждого
// глифа находятся плоские горизонтальные отрезки контура, из них собираются
// штрихи (нижняя и верхняя кромки с общим по X участком), и каждому штриху
// добавляются узлы отображения: толщина округляется до целого числа пикселей
// (не меньше одного), а положение — к сетке либо к ближайшей зоне, если штрих
// сидит на ней (верх «T», низ «L»).

// hintScan собирает с контура глифа горизонтальные отрезки и знак площади
// (какая сторона контура — чернила). Координаты — 26.6, ось вверх.
type hintScan struct {
	cur, start [2]int
	have       bool
	area       int // удвоенная площадь по конечным точкам сегментов
	lines      []hLine
	// lo, hi — низ и верх текущего контура по конечным точкам сегментов;
	// edges — эти высоты всех замкнутых контуров (края, которыми знак может
	// стоять на зоне: см. floating).
	lo, hi int
	edges  []int
}

// hLine — отрезок контура на высоте y от x0 к x1 (направление обхода).
type hLine struct{ y, x0, x1 int }

func (s *hintScan) moveTo(x, y int) {
	s.closeContour()
	s.cur, s.start, s.have = [2]int{x, y}, [2]int{x, y}, true
	s.lo, s.hi = y, y
}

// seg учитывает отрезок cur → (x, y); line — прямой ли он (кривые на штрихи
// не претендуют, но площадь считают).
func (s *hintScan) seg(x, y int, line bool) {
	if !s.have {
		return
	}
	s.area += s.cur[0]*y - x*s.cur[1]
	if line && y == s.cur[1] && x != s.cur[0] {
		s.lines = append(s.lines, hLine{y: y, x0: s.cur[0], x1: x})
	}
	s.cur = [2]int{x, y}
	s.lo, s.hi = min(s.lo, y), max(s.hi, y)
}

func (s *hintScan) lineTo(x, y int)  { s.seg(x, y, true) }
func (s *hintScan) curveTo(x, y int) { s.seg(x, y, false) }

// closeContour замыкает контур неявным отрезком к началу.
func (s *hintScan) closeContour() {
	if s.have && s.cur != s.start {
		s.seg(s.start[0], s.start[1], true)
	}
	if s.have {
		s.edges = append(s.edges, s.lo, s.hi)
	}
	s.have = false
}

// stemMinLen — самый короткий отрезок, который считается кромкой штриха
// (26.6): мельче — это скос или засечка, выравнивать нечего.
const stemMinLen = 24

// withStems возвращает отображение для одного глифа: общее h плюс узлы его
// горизонтальных штрихов. emPx — размер em в 26.6 (ограничивает толщину
// штриха). Нет штрихов или они не вписались — возвращается h.
func (h *vHint) withStems(s *hintScan, emPx int) *vHint {
	s.closeContour()
	if len(s.lines) < 2 || s.area == 0 {
		return h
	}
	// Чернила справа от обхода при отрицательной площади (TrueType, ось
	// вверх): отрезок вправо — верхняя кромка, влево — нижняя. Знак площади
	// обратный (PostScript-контуры) меняет роли.
	var tops, bots []hLine
	for _, l := range s.lines {
		ln := l.x1 - l.x0
		if ln < 0 {
			ln = -ln
		}
		if ln < stemMinLen {
			continue
		}
		if (l.x1 > l.x0) == (s.area < 0) {
			tops = append(tops, l)
		} else {
			bots = append(bots, l)
		}
	}
	if len(tops) == 0 || len(bots) == 0 {
		return h
	}
	cur := &vHint{src: append([]int(nil), h.src...), dst: append([]int(nil), h.dst...)}
	maxW := emPx / 5
	const attach = 16 // кромка на расстоянии до 1/4 px от узла зоны — «на зоне»
	for _, t := range tops {
		// Нижняя кромка того же штриха: ближайшая ниже, с общим участком по X.
		best := -1
		for i, b := range bots {
			if b.y >= t.y || t.y-b.y > maxW || overlapLen(t, b) < 16 {
				continue
			}
			if best < 0 || b.y > bots[best].y {
				best = i
			}
		}
		if best < 0 {
			continue
		}
		bot := bots[best]
		onT, onB := cur.nodeNear(t.y, attach), cur.nodeNear(bot.y, attach)
		if onT >= 0 && onB >= 0 {
			continue
		}
		w := round26(cur.apply(t.y) - cur.apply(bot.y))
		if w < 64 {
			w = 64
		}
		var db, dt int
		switch {
		case onT >= 0:
			dt = cur.dst[onT]
			db = dt - w
		case onB >= 0:
			db = cur.dst[onB]
			dt = db + w
		default:
			mid := (cur.apply(t.y) + cur.apply(bot.y)) / 2
			db = round26(mid - w/2)
			dt = db + w
		}
		next := cur.with(bot.y, db).with(t.y, dt)
		if next.valid() {
			cur = next
		}
	}
	return cur
}

// ── Знаки вне зон ───────────────────────────────────────────────────────────
//
// Маркер «•», точка «·», градус «°» не пересекают ни одной зоны: лежат целиком
// между базовой линией и высотой строчных (или между строчными и заглавными),
// разве что касаясь одной из них краем. Растяжение между зонами меняло их высоту на долю пикселя по-разному сверху
// и снизу — круг становился овалом (отчёт RDP-клиента на v3.43.0: «•» в поле
// пароля 4×5 вместо 4×4). Такой знак не растягивается, а сдвигается целиком
// на целое число пикселей — туда, куда растяжение увело бы его середину, —
// и его маска побайтно совпадает с маской без хинтинга, только сдвинута.
// Так поступает и режим light у FreeType со знаками вне зон.

// forGlyph — отображение для одного глифа с размахом [bot, top] (26.6, ось
// вверх): общее h плюс узлы его горизонтальных штрихов (withStems), а знак без
// штрихов, который стоит на зонах меньше чем двумя краями, только сдвигается
// (floating).
func (h *vHint) forGlyph(s *hintScan, emPx, top, bot int) *vHint {
	if hs := h.withStems(s, emPx); hs != h {
		return hs // у знака есть штрихи («-», «=», «+»): их выравнивание и даёт чёткость
	}
	if f := h.floating(s.edges, top, bot); f != nil {
		return f
	}
	return h
}

// floating возвращает чистый сдвиг на целое число пикселей для знака, у
// которого на зонах стоит не больше одного уровня краёв контуров (edges — низ
// и верх каждого контура). Зона, проходящая через середину знака мимо его
// краёв, его не касается: у «°» высота строчных пересекает кольцо, но ни
// верх, ни низ колец на ней не лежат. Знак, стоящий на зонах двумя уровнями
// («x», «o», «l», «i» — базовая линия и высота строчных или выносных), по-прежнему
// растягивается: на этом держится выравнивание букв. nil — растягивать.
//
// Сдвиг: край на зоне ставится к её выровненной высоте (с точностью до
// целого пикселя); край ни на одной зоне не стоит — середина знака идёт туда,
// куда её увело бы растяжение.
func (h *vHint) floating(edges []int, top, bot int) *vHint {
	const attach = 16 // как у штрихов: ближе 1/4 px к узлу — «на зоне»
	if top <= bot || len(edges) == 0 {
		return nil
	}
	anchor, levels := 0, 0
	seen := make([]int, 0, 4)
	for _, e := range edges {
		i := h.nodeNear(e, attach)
		if i < 0 {
			continue
		}
		dup := false
		for _, v := range seen {
			if d := v - e; d >= -2*attach && d <= 2*attach {
				dup = true // тот же уровень (плоская и круглая зона рядом, внутренний контур)
				break
			}
		}
		if dup {
			continue
		}
		seen = append(seen, e)
		levels++
		anchor = e
		if levels > 1 {
			return nil
		}
	}
	if levels == 0 {
		anchor = (top + bot) >> 1
	}
	shift := round26(h.apply(anchor) - anchor)
	return &vHint{src: []int{0}, dst: []int{shift}} // один узел — apply только сдвигает
}

func overlapLen(a, b hLine) int {
	a0, a1 := a.x0, a.x1
	if a0 > a1 {
		a0, a1 = a1, a0
	}
	b0, b1 := b.x0, b.x1
	if b0 > b1 {
		b0, b1 = b1, b0
	}
	lo, hi := a0, a1
	if b0 > lo {
		lo = b0
	}
	if b1 < hi {
		hi = b1
	}
	return hi - lo
}

// nodeNear — индекс узла, чья src отстоит от y не более чем на tol; -1 — нет.
func (h *vHint) nodeNear(y, tol int) int {
	for i, s := range h.src {
		if d := s - y; d >= -tol && d <= tol {
			return i
		}
	}
	return -1
}

// with возвращает копию с узлом src→dst (замена цели, если src уже есть).
func (h *vHint) with(src, dst int) *vHint {
	n := &vHint{}
	placed := false
	for i, s := range h.src {
		if !placed && src <= s {
			placed = true
			n.src, n.dst = append(n.src, src), append(n.dst, dst)
			if src == s {
				continue
			}
		}
		n.src, n.dst = append(n.src, s), append(n.dst, h.dst[i])
	}
	if !placed {
		n.src, n.dst = append(n.src, src), append(n.dst, dst)
	}
	return n
}

// valid — цели не убывают (отображение монотонно).
func (h *vHint) valid() bool {
	for i := 1; i < len(h.dst); i++ {
		if h.dst[i] < h.dst[i-1] {
			return false
		}
	}
	return true
}
