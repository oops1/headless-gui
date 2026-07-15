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

// v1Layer — один растеризованный слой COLRv1: маска покрытия + цвет
// (premultiplied). Собирается при обходе графа, композитится позже.
type v1Layer struct {
	m   *glyphMask
	col color.RGBA
}

const (
	maxColrDepth  = 64  // защита от циклов/глубокой вложенности
	maxColrLayers = 512 // защита от патологических шрифтов
)

// renderCOLRv1 обходит граф paint базового глифа и композитит слои в цветную
// RGBA-маску. Второе значение — «динамический» (использует foreground-цвет
// текста, палитра 0xFFFF): такие глифы не кэшируются.
func renderCOLRv1(face *tsfont.Face, colr *tables.COLR1, paint tables.PaintTable, sizePx fixed.Int26_6, textCol color.RGBA) (*colorGlyph, bool) {
	if colr == nil {
		return nil, false
	}
	pal := colrPalette(face)
	var layers []v1Layer
	dynamic := false
	collectColrV1(face, colr, paint, identityAffine, pal, textCol, sizePx, &layers, &dynamic, 0)
	if len(layers) == 0 {
		return nil, dynamic
	}

	const big = 1 << 30
	minX, minY, maxX, maxY := big, big, -big, -big
	for _, l := range layers {
		w, h := l.m.mask.Rect.Dx(), l.m.mask.Rect.Dy()
		if l.m.offX < minX {
			minX = l.m.offX
		}
		if l.m.offY < minY {
			minY = l.m.offY
		}
		if l.m.offX+w > maxX {
			maxX = l.m.offX + w
		}
		if l.m.offY+h > maxY {
			maxY = l.m.offY + h
		}
	}
	if maxX <= minX || maxY <= minY {
		return nil, dynamic
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxX-minX, maxY-minY))
	for _, l := range layers { // порядок сбора = снизу вверх
		compositeMaskColor(dst, l.m.mask, l.m.offX-minX, l.m.offY-minY, l.col)
	}
	return &colorGlyph{img: dst, offX: minX, offY: minY}, dynamic
}

// colrPalette возвращает палитру по умолчанию (палитра 0) или nil.
func colrPalette(face *tsfont.Face) []tables.ColorRecord {
	if cp := face.CPAL; len(cp) > 0 {
		return cp[0]
	}
	return nil
}

// collectColrV1 рекурсивно обходит граф paint, добавляя растеризованные слои
// в out. ctm — накопленное преобразование в font units.
func collectColrV1(face *tsfont.Face, colr *tables.COLR1, paint tables.PaintTable, ctm affine,
	pal []tables.ColorRecord, textCol color.RGBA, sizePx fixed.Int26_6,
	out *[]v1Layer, dynamic *bool, depth int) {

	if depth > maxColrDepth || len(*out) >= maxColrLayers {
		return
	}
	switch v := paint.(type) {
	case tables.PaintColrLayers:
		sub, err := colr.LayerList.Resolve(v)
		if err != nil {
			return
		}
		for _, p := range sub {
			collectColrV1(face, colr, p, ctm, pal, textCol, sizePx, out, dynamic, depth+1)
		}
	case tables.PaintColrGlyph:
		if p, ok := colr.Search(tables.GlyphID(v.GlyphID)); ok {
			collectColrV1(face, colr, p, ctm, pal, textCol, sizePx, out, dynamic, depth+1)
		}
	case tables.PaintGlyph:
		m := rasterizeGlyphAffine(face, tsfont.GID(v.GlyphID), sizePx, ctm)
		if m.mask == nil {
			return
		}
		col, ok := representativeColor(v.Paint, pal, textCol, dynamic, 0)
		if !ok || col.A == 0 {
			return
		}
		*out = append(*out, v1Layer{m: m, col: col})
	case tables.PaintTransform:
		collectColrV1(face, colr, v.Paint, compose(ctm, affFrom2x3(v.Transform)), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintVarTransform:
		collectColrV1(face, colr, v.Paint, compose(ctm, affFromVar2x3(v.Transform)), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintTranslate:
		collectColrV1(face, colr, v.Paint, compose(ctm, affTranslate(float32(v.Dx), float32(v.Dy))), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintVarTranslate:
		collectColrV1(face, colr, v.Paint, compose(ctm, affTranslate(float32(v.Dx), float32(v.Dy))), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintScale:
		collectColrV1(face, colr, v.Paint, compose(ctm, affScale(f2dot14(v.ScaleX), f2dot14(v.ScaleY))), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintScaleAroundCenter:
		collectColrV1(face, colr, v.Paint, compose(ctm, aroundCenter(affScale(f2dot14(v.ScaleX), f2dot14(v.ScaleY)), float32(v.CenterX), float32(v.CenterY))), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintScaleUniform:
		s := f2dot14(v.Scale)
		collectColrV1(face, colr, v.Paint, compose(ctm, affScale(s, s)), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintScaleUniformAroundCenter:
		s := f2dot14(v.Scale)
		collectColrV1(face, colr, v.Paint, compose(ctm, aroundCenter(affScale(s, s), float32(v.CenterX), float32(v.CenterY))), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintRotate:
		collectColrV1(face, colr, v.Paint, compose(ctm, affRotate(colrDeg(v.Angle))), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintRotateAroundCenter:
		collectColrV1(face, colr, v.Paint, compose(ctm, aroundCenter(affRotate(colrDeg(v.Angle)), float32(v.CenterX), float32(v.CenterY))), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintSkew:
		collectColrV1(face, colr, v.Paint, compose(ctm, affSkew(colrDeg(v.XSkewAngle), colrDeg(v.YSkewAngle))), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintSkewAroundCenter:
		collectColrV1(face, colr, v.Paint, compose(ctm, aroundCenter(affSkew(colrDeg(v.XSkewAngle), colrDeg(v.YSkewAngle)), float32(v.CenterX), float32(v.CenterY))), pal, textCol, sizePx, out, dynamic, depth+1)
	case tables.PaintComposite:
		// Приблизительно: сначала фон, затем источник (режим смешивания игнорируем).
		collectColrV1(face, colr, v.BackdropPaint, ctm, pal, textCol, sizePx, out, dynamic, depth+1)
		collectColrV1(face, colr, v.SourcePaint, ctm, pal, textCol, sizePx, out, dynamic, depth+1)
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

// rasterizeGlyphAffine растеризует контур глифа gid, применяя аффинное
// преобразование m (в font units), в альфа-маску покрытия. Возвращает пустую
// маску, если у глифа нет контура (растровый/цветной слой — не сюда).
func rasterizeGlyphAffine(face *tsfont.Face, gid tsfont.GID, sizePx fixed.Int26_6, m affine) *glyphMask {
	outline, ok := face.GlyphData(gid).(tsfont.GlyphOutline)
	if !ok || len(outline.Segments) == 0 {
		return &glyphMask{}
	}
	scale := float32(sizePx) / 64.0 / float32(face.Upem())

	// font unit (x, y) → пиксель (ось Y вниз) через m и масштаб кегля.
	mapPt := func(x, y float32) (float32, float32) {
		fx := m.xx*x + m.xy*y + m.dx
		fy := m.yx*x + m.yy*y + m.dy
		return fx * scale, -fy * scale
	}

	// Проход 1: bbox в пикселях.
	first := true
	var minXf, minYf, maxXf, maxYf float32
	upd := func(px, py float32) {
		if first {
			minXf, maxXf, minYf, maxYf, first = px, px, py, py, false
			return
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
	for _, seg := range outline.Segments {
		for i := 0; i < segArgs(seg.Op); i++ {
			px, py := mapPt(seg.Args[i].X, seg.Args[i].Y)
			upd(px, py)
		}
	}
	if first {
		return &glyphMask{}
	}
	minX, minY := floorF32(minXf), floorF32(minYf)
	w := ceilF32(maxXf) - minX + 1
	h := ceilF32(maxYf) - minY + 1
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return &glyphMask{}
	}

	r := vector.NewRasterizer(w, h)
	ox, oy := float32(minX), float32(minY)
	started := false
	pt := func(i int, seg ot.Segment) (float32, float32) {
		px, py := mapPt(seg.Args[i].X, seg.Args[i].Y)
		return px - ox, py - oy
	}
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
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	r.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	return &glyphMask{mask: mask, offX: minX, offY: minY}
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
