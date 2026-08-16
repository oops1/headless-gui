// colrv1.go — рендеринг цветных глифов COLR версии 1.
//
// COLRv1 описывает глиф как ГРАФ paint-таблиц: слои (PaintColrLayers),
// раскраска контуров (PaintGlyph), сплошные цвета и градиенты (PaintSolid /
// Paint*Gradient), аффинные преобразования (PaintTransform/Scale/Rotate/…) и
// ссылки на другие базовые глифы (PaintColrGlyph). Так устроен современный
// Segoe UI Emoji (Windows 11).
//
// Реализация обходит граф, накапливая аффинную матрицу (в font units), и для
// каждого PaintGlyph растеризует его контур с текущим преобразованием, заливая
// РЕПРЕЗЕНТАТИВНЫМ цветом дочернего paint'а: сплошной цвет — точно; градиент —
// усреднённым цветом остановок (плавность градиента аппроксимируется плоской
// заливкой). Формы и цвета эмодзи получаются корректными; тонкие переливы
// градиентов — упрощены. Порядок остановок, режимы Extend и попиксельные
// градиенты не реализованы (ограничение).
package engine

import (
	"image"
	"image/color"
	"math"

	tsfont "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// affine — аффинное преобразование в координатах font units (ось Y вверх):
//
//	x' = xx·x + xy·y + dx
//	y' = yx·x + yy·y + dy
type affine struct{ xx, yx, xy, yy, dx, dy float32 }

var identityAffine = affine{xx: 1, yy: 1}

// compose возвращает матрицу «сначала b, затем a» (a∘b).
func compose(a, b affine) affine {
	return affine{
		xx: a.xx*b.xx + a.xy*b.yx,
		yx: a.yx*b.xx + a.yy*b.yx,
		xy: a.xx*b.xy + a.xy*b.yy,
		yy: a.yx*b.xy + a.yy*b.yy,
		dx: a.xx*b.dx + a.xy*b.dy + a.dx,
		dy: a.yx*b.dx + a.yy*b.dy + a.dy,
	}
}

func affTranslate(dx, dy float32) affine { return affine{xx: 1, yy: 1, dx: dx, dy: dy} }
func affScale(sx, sy float32) affine     { return affine{xx: sx, yy: sy} }

func affRotate(deg float32) affine {
	r := float64(deg) * math.Pi / 180
	c, s := float32(math.Cos(r)), float32(math.Sin(r))
	return affine{xx: c, yx: s, xy: -s, yy: c}
}

func affSkew(xDeg, yDeg float32) affine {
	tx := float32(math.Tan(float64(xDeg) * math.Pi / 180))
	ty := float32(math.Tan(float64(yDeg) * math.Pi / 180))
	return affine{xx: 1, yx: ty, xy: tx, yy: 1}
}

// aroundCenter применяет m относительно центра (cx, cy).
func aroundCenter(m affine, cx, cy float32) affine {
	return compose(affTranslate(cx, cy), compose(m, affTranslate(-cx, -cy)))
}

// f2dot14 переводит F2Dot14 (tables.Fixed214) в float32.
func f2dot14(v tables.Fixed214) float32 { return float32(v) / 16384.0 }

// colrDeg переводит угол COLR (1.0 == 180° против часовой) в градусы.
func colrDeg(v tables.Fixed214) float32 { return f2dot14(v) * 180.0 }

// glyphBox — границы слоя в пикселях относительно точки отрисовки.
type glyphBox struct{ offX, offY, w, h int }

// v1Layer — слой COLRv1 до растеризации: глиф, матрица, цвет, границы.
type v1Layer struct {
	gid tsfont.GID
	ctm affine
	col color.RGBA // premultiplied
	box glyphBox
}

const (
	maxColrDepth  = 64  // защита от циклов/глубокой вложенности
	maxColrLayers = 512 // защита от патологических шрифтов
)

// renderCOLRv1 обходит граф paint базового глифа и композитит слои в цветную
// RGBA-маску. Второе значение — «динамический» (использует foreground-цвет
// текста, палитра 0xFFFF).
func renderCOLRv1(face *tsfont.Face, colr *tables.COLR1, paint tables.PaintTable, sizePx fixed.Int26_6, textCol color.RGBA) (*colorGlyph, bool) {
	if colr == nil {
		return nil, false
	}
	w := &colrWalk{face: face, colr: colr, pal: colrPalette(face), textCol: textCol, sizePx: sizePx}
	w.collect(paint, identityAffine, 0)
	if w.over {
		warnGlyphBudget(face)
		return nil, w.dynamic
	}
	if len(w.layers) == 0 {
		return nil, w.dynamic
	}

	const big = 1 << 30
	minX, minY, maxX, maxY := big, big, -big, -big
	maxPix := 0
	for _, l := range w.layers {
		if l.box.offX < minX {
			minX = l.box.offX
		}
		if l.box.offY < minY {
			minY = l.box.offY
		}
		if l.box.offX+l.box.w > maxX {
			maxX = l.box.offX + l.box.w
		}
		if l.box.offY+l.box.h > maxY {
			maxY = l.box.offY + l.box.h
		}
		if n := l.box.w * l.box.h; n > maxPix {
			maxPix = n
		}
	}
	if maxX <= minX || maxY <= minY {
		return nil, w.dynamic
	}
	if !glyphBudgetOK(maxX-minX, maxY-minY) {
		warnGlyphBudget(face)
		return nil, w.dynamic
	}

	dst := image.NewRGBA(image.Rect(0, 0, maxX-minX, maxY-minY))
	// Один буфер маски и один растеризатор на все слои глифа.
	buf := make([]uint8, maxPix)
	var rast vector.Rasterizer
	for _, l := range w.layers { // порядок сбора = снизу вверх
		mask := &image.Alpha{
			Pix:    buf[:l.box.w*l.box.h],
			Stride: l.box.w,
			Rect:   image.Rect(0, 0, l.box.w, l.box.h),
		}
		clear(mask.Pix)
		if !drawGlyphAffine(&rast, mask, face, l.gid, sizePx, l.ctm, l.box) {
			continue
		}
		compositeMaskColor(dst, mask, l.box.offX-minX, l.box.offY-minY, l.col)
	}
	return &colorGlyph{img: dst, offX: minX, offY: minY}, w.dynamic
}

// colrPalette возвращает палитру по умолчанию (палитра 0) или nil.
func colrPalette(face *tsfont.Face) []tables.ColorRecord {
	if cp := face.CPAL; len(cp) > 0 {
		return cp[0]
	}
	return nil
}

// colrWalk — обход графа COLRv1 одного глифа с бюджетом пикселей.
type colrWalk struct {
	face    *tsfont.Face
	colr    *tables.COLR1
	pal     []tables.ColorRecord
	textCol color.RGBA
	sizePx  fixed.Int26_6
	layers  []v1Layer
	pixels  int
	dynamic bool
	over    bool // бюджет исчерпан — глиф пропускаем целиком
}

// addLayer учитывает слой в бюджете пикселей; false — бюджет исчерпан.
func (w *colrWalk) addLayer(l v1Layer) bool {
	w.pixels += l.box.w * l.box.h
	if w.pixels > maxGlyphLayerPixels {
		w.over = true
		return false
	}
	w.layers = append(w.layers, l)
	return true
}

// collect рекурсивно обходит граф paint, накапливая слои. ctm — накопленное
// преобразование в font units.
func (w *colrWalk) collect(paint tables.PaintTable, ctm affine, depth int) {
	if w.over || depth > maxColrDepth || len(w.layers) >= maxColrLayers {
		return
	}
	switch v := paint.(type) {
	case tables.PaintColrLayers:
		sub, err := w.colr.LayerList.Resolve(v)
		if err != nil {
			return
		}
		for _, p := range sub {
			w.collect(p, ctm, depth+1)
		}
	case tables.PaintColrGlyph:
		if p, ok := w.colr.Search(tables.GlyphID(v.GlyphID)); ok {
			w.collect(p, ctm, depth+1)
		}
	case tables.PaintGlyph:
		gid := tsfont.GID(v.GlyphID)
		box, ok := affineGlyphBox(w.face, gid, w.sizePx, ctm)
		if !ok {
			return
		}
		col, ok := representativeColor(v.Paint, w.pal, w.textCol, &w.dynamic, 0)
		if !ok || col.A == 0 {
			return
		}
		w.addLayer(v1Layer{gid: gid, ctm: ctm, col: col, box: box})
	case tables.PaintTransform:
		w.collect(v.Paint, compose(ctm, affFrom2x3(v.Transform)), depth+1)
	case tables.PaintVarTransform:
		w.collect(v.Paint, compose(ctm, affFromVar2x3(v.Transform)), depth+1)
	case tables.PaintTranslate:
		w.collect(v.Paint, compose(ctm, affTranslate(float32(v.Dx), float32(v.Dy))), depth+1)
	case tables.PaintVarTranslate:
		w.collect(v.Paint, compose(ctm, affTranslate(float32(v.Dx), float32(v.Dy))), depth+1)
	case tables.PaintScale:
		w.collect(v.Paint, compose(ctm, affScale(f2dot14(v.ScaleX), f2dot14(v.ScaleY))), depth+1)
	case tables.PaintScaleAroundCenter:
		w.collect(v.Paint, compose(ctm, aroundCenter(affScale(f2dot14(v.ScaleX), f2dot14(v.ScaleY)), float32(v.CenterX), float32(v.CenterY))), depth+1)
	case tables.PaintScaleUniform:
		s := f2dot14(v.Scale)
		w.collect(v.Paint, compose(ctm, affScale(s, s)), depth+1)
	case tables.PaintScaleUniformAroundCenter:
		s := f2dot14(v.Scale)
		w.collect(v.Paint, compose(ctm, aroundCenter(affScale(s, s), float32(v.CenterX), float32(v.CenterY))), depth+1)
	case tables.PaintRotate:
		w.collect(v.Paint, compose(ctm, affRotate(colrDeg(v.Angle))), depth+1)
	case tables.PaintRotateAroundCenter:
		w.collect(v.Paint, compose(ctm, aroundCenter(affRotate(colrDeg(v.Angle)), float32(v.CenterX), float32(v.CenterY))), depth+1)
	case tables.PaintSkew:
		w.collect(v.Paint, compose(ctm, affSkew(colrDeg(v.XSkewAngle), colrDeg(v.YSkewAngle))), depth+1)
	case tables.PaintSkewAroundCenter:
		w.collect(v.Paint, compose(ctm, aroundCenter(affSkew(colrDeg(v.XSkewAngle), colrDeg(v.YSkewAngle)), float32(v.CenterX), float32(v.CenterY))), depth+1)
	case tables.PaintComposite:
		// Приблизительно: сначала фон, затем источник (режим смешивания игнорируем).
		w.collect(v.BackdropPaint, ctm, depth+1)
		w.collect(v.SourcePaint, ctm, depth+1)
	default:
		// PaintSolid/градиент без PaintGlyph-родителя (нет формы) или
		// непокрытый тип — пропускаем.
	}
}

func affFrom2x3(t tables.Affine2x3) affine {
	return affine{xx: t.Xx, yx: t.Yx, xy: t.Xy, yy: t.Yy, dx: t.Dx, dy: t.Dy}
}

func affFromVar2x3(t tables.VarAffine2x3) affine {
	return affine{xx: t.Xx, yx: t.Yx, xy: t.Xy, yy: t.Yy, dx: t.Dx, dy: t.Dy}
}

// representativeColor возвращает премультиплицированный цвет для заливки
// PaintGlyph: сплошной — точно, градиент — усреднённый по остановкам. Обходит
// вложенные преобразования (они не влияют на плоский цвет).
func representativeColor(paint tables.PaintTable, pal []tables.ColorRecord, textCol color.RGBA, dynamic *bool, depth int) (color.RGBA, bool) {
	if depth > maxColrDepth {
		return color.RGBA{}, false
	}
	switch v := paint.(type) {
	case tables.PaintSolid:
		return cpalPremul(pal, v.PaletteIndex, f2dot14(v.Alpha), textCol, dynamic)
	case tables.PaintVarSolid:
		return cpalPremul(pal, v.PaletteIndex, f2dot14(v.Alpha), textCol, dynamic)
	case tables.PaintLinearGradient:
		return gradientColor(v.ColorLine.ColorStops, pal, textCol, dynamic)
	case tables.PaintRadialGradient:
		return gradientColor(v.ColorLine.ColorStops, pal, textCol, dynamic)
	case tables.PaintSweepGradient:
		return gradientColor(v.ColorLine.ColorStops, pal, textCol, dynamic)
	case tables.PaintVarLinearGradient:
		return gradientColorVar(v.ColorLine.ColorStops, pal, textCol, dynamic)
	case tables.PaintVarRadialGradient:
		return gradientColorVar(v.ColorLine.ColorStops, pal, textCol, dynamic)
	case tables.PaintVarSweepGradient:
		return gradientColorVar(v.ColorLine.ColorStops, pal, textCol, dynamic)
	case tables.PaintTransform:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintVarTransform:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintTranslate:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintVarTranslate:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintScale:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintScaleAroundCenter:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintScaleUniform:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintScaleUniformAroundCenter:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintRotate:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintRotateAroundCenter:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintSkew:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	case tables.PaintSkewAroundCenter:
		return representativeColor(v.Paint, pal, textCol, dynamic, depth+1)
	default:
		return color.RGBA{}, false
	}
}

// cpalPremul выбирает цвет палитры idx, домножает на alpha и премультиплицирует.
// idx == 0xFFFF — цвет текста (foreground): помечает результат динамическим.
func cpalPremul(pal []tables.ColorRecord, idx uint16, alpha float32, textCol color.RGBA, dynamic *bool) (color.RGBA, bool) {
	var base color.RGBA
	switch {
	case idx == 0xFFFF:
		base = textCol
		*dynamic = true
	case int(idx) < len(pal):
		cr := pal[idx]
		base = color.RGBA{R: cr.Red, G: cr.Green, B: cr.Blue, A: cr.Alpha}
	default:
		return color.RGBA{}, false
	}
	a := float32(base.A) * alpha
	if a < 0 {
		a = 0
	} else if a > 255 {
		a = 255
	}
	A := uint32(a + 0.5)
	return color.RGBA{
		R: uint8(uint32(base.R) * A / 255),
		G: uint8(uint32(base.G) * A / 255),
		B: uint8(uint32(base.B) * A / 255),
		A: uint8(A),
	}, true
}

// gradientColor усредняет остановки цветовой линии (premultiplied).
func gradientColor(stops []tables.ColorStop, pal []tables.ColorRecord, textCol color.RGBA, dynamic *bool) (color.RGBA, bool) {
	var r, g, b, a, n uint32
	for _, s := range stops {
		c, ok := cpalPremul(pal, s.PaletteIndex, f2dot14(s.Alpha), textCol, dynamic)
		if !ok {
			continue
		}
		r += uint32(c.R)
		g += uint32(c.G)
		b += uint32(c.B)
		a += uint32(c.A)
		n++
	}
	if n == 0 {
		return color.RGBA{}, false
	}
	return color.RGBA{R: uint8(r / n), G: uint8(g / n), B: uint8(b / n), A: uint8(a / n)}, true
}

func gradientColorVar(stops []tables.VarColorStop, pal []tables.ColorRecord, textCol color.RGBA, dynamic *bool) (color.RGBA, bool) {
	var r, g, b, a, n uint32
	for _, s := range stops {
		c, ok := cpalPremul(pal, s.PaletteIndex, f2dot14(s.Alpha), textCol, dynamic)
		if !ok {
			continue
		}
		r += uint32(c.R)
		g += uint32(c.G)
		b += uint32(c.B)
		a += uint32(c.A)
		n++
	}
	if n == 0 {
		return color.RGBA{}, false
	}
	return color.RGBA{R: uint8(r / n), G: uint8(g / n), B: uint8(b / n), A: uint8(a / n)}, true
}

// affMapper возвращает перевод font unit → пиксель (ось Y вниз) через m.
func affMapper(face *tsfont.Face, sizePx fixed.Int26_6, m affine) func(x, y float32) (float32, float32) {
	scale := float32(sizePx) / 64.0 / float32(face.Upem())
	return func(x, y float32) (float32, float32) {
		fx := m.xx*x + m.xy*y + m.dx
		fy := m.yx*x + m.yy*y + m.dy
		return fx * scale, -fy * scale
	}
}

// affineGlyphBox считает границы глифа под преобразованием m без растеризации.
func affineGlyphBox(face *tsfont.Face, gid tsfont.GID, sizePx fixed.Int26_6, m affine) (glyphBox, bool) {
	outline, ok := face.GlyphData(gid).(tsfont.GlyphOutline)
	if !ok || len(outline.Segments) == 0 {
		return glyphBox{}, false
	}
	mapPt := affMapper(face, sizePx, m)
	first := true
	var minXf, minYf, maxXf, maxYf float32
	for _, seg := range outline.Segments {
		for i := 0; i < segArgs(seg.Op); i++ {
			px, py := mapPt(seg.Args[i].X, seg.Args[i].Y)
			if first {
				minXf, maxXf, minYf, maxYf, first = px, px, py, py, false
				continue
			}
			if px < minXf {
				minXf = px
			}
			if px > maxXf {
				maxXf = px
			}
			if py < minYf {
				minYf = py
			}
			if py > maxYf {
				maxYf = py
			}
		}
	}
	if first {
		return glyphBox{}, false
	}
	minX, minY := floorF32(minXf), floorF32(minYf)
	w := ceilF32(maxXf) - minX + 1
	h := ceilF32(maxYf) - minY + 1
	if !glyphBudgetOK(w, h) {
		return glyphBox{}, false
	}
	return glyphBox{offX: minX, offY: minY, w: w, h: h}, true
}

// drawGlyphAffine растеризует контур глифа в готовую маску по границам b.
// Маска и растеризатор переиспользуются между слоями одного глифа.
func drawGlyphAffine(r *vector.Rasterizer, mask *image.Alpha, face *tsfont.Face,
	gid tsfont.GID, sizePx fixed.Int26_6, m affine, b glyphBox) bool {

	outline, ok := face.GlyphData(gid).(tsfont.GlyphOutline)
	if !ok || len(outline.Segments) == 0 {
		return false
	}
	mapPt := affMapper(face, sizePx, m)
	r.Reset(b.w, b.h)
	ox, oy := float32(b.offX), float32(b.offY)
	pt := func(i int, seg ot.Segment) (float32, float32) {
		px, py := mapPt(seg.Args[i].X, seg.Args[i].Y)
		return px - ox, py - oy
	}
	started := false
	for _, seg := range outline.Segments {
		switch seg.Op {
		case ot.SegmentOpMoveTo:
			if started {
				r.ClosePath()
			}
			x, y := pt(0, seg)
			r.MoveTo(x, y)
			started = true
		case ot.SegmentOpLineTo:
			x, y := pt(0, seg)
			r.LineTo(x, y)
		case ot.SegmentOpQuadTo:
			cx, cy := pt(0, seg)
			ex, ey := pt(1, seg)
			r.QuadTo(cx, cy, ex, ey)
		case ot.SegmentOpCubeTo:
			c1x, c1y := pt(0, seg)
			c2x, c2y := pt(1, seg)
			ex, ey := pt(2, seg)
			r.CubeTo(c1x, c1y, c2x, c2y, ex, ey)
		}
	}
	if started {
		r.ClosePath()
	}
	r.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	return true
}

// segArgs — число значащих точек в сегменте контура.
func segArgs(op ot.SegmentOp) int {
	switch op {
	case ot.SegmentOpQuadTo:
		return 2
	case ot.SegmentOpCubeTo:
		return 3
	default: // MoveTo, LineTo
		return 1
	}
}
