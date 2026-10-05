package svg

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

// rasterKey — ключ кэша растеризаций (иконка неявно — сам Document).
type rasterKey struct {
	w, h    int
	r, g, b uint8
	a       uint8
	tint    bool
	opt     uint8 // Options.bits(): режим входит в ключ
}

type rasterEntry struct {
	img *image.RGBA
}

// maxCacheEntries ограничивает рост кэша (иконка × размеры × цвета).
const maxCacheEntries = 48

// RasterizeCached возвращает растеризацию иконки размера w×h с подстановкой
// currentColor=current. Результат кэшируется по (размер, цвет, tint).
// tint=true перекрашивает ВЕСЬ контент в current (монохромная перекраска).
//
// Возвращаемый *image.RGBA принадлежит кэшу — вызывающий не должен его менять.
func (d *Document) RasterizeCached(w, h int, current color.RGBA, tint bool) *image.RGBA {
	return d.rasterizeCached(w, h, current, tint, d.Options())
}

func (d *Document) rasterizeCached(w, h int, current color.RGBA, tint bool, o Options) *image.RGBA {
	if w <= 0 || h <= 0 {
		return nil
	}
	key := rasterKey{w: w, h: h, r: current.R, g: current.G, b: current.B, a: current.A, tint: tint, opt: o.bits()}
	d.mu.Lock()
	if d.cache != nil {
		if e, ok := d.cache[key]; ok {
			d.mu.Unlock()
			return e.img
		}
	}
	d.mu.Unlock()

	img := d.rasterize(w, h, current, tint, o)

	d.mu.Lock()
	if d.cache == nil {
		d.cache = make(map[rasterKey]*rasterEntry)
	}
	if len(d.cache) >= maxCacheEntries {
		// Простейшая политика: очистить при переполнении.
		d.cache = make(map[rasterKey]*rasterEntry)
	}
	d.cache[key] = &rasterEntry{img: img}
	d.mu.Unlock()
	return img
}

// InvalidateCache сбрасывает кэш растеризаций (например, после мутации иконки).
func (d *Document) InvalidateCache() {
	d.mu.Lock()
	d.cache = nil
	d.mu.Unlock()
}

// Rasterize растеризует иконку в новый *image.RGBA размера w×h (прозрачный
// фон), сохраняя пропорции viewBox и центрируя содержимое. current
// подставляется вместо currentColor; tint=true перекрашивает всё в current.
func (d *Document) Rasterize(w, h int, current color.RGBA, tint bool) *image.RGBA {
	return d.rasterize(w, h, current, tint, d.Options())
}

func (d *Document) rasterize(w, h int, current color.RGBA, tint bool, o Options) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return dst
	}
	vbW := d.ViewBox[2]
	vbH := d.ViewBox[3]
	if vbW <= 0 || vbH <= 0 {
		return dst
	}
	// Масштаб «uniform» (вписать с сохранением пропорций) + центрирование.
	sx := float64(w) / vbW
	sy := float64(h) / vbH
	s := sx
	if sy < s {
		s = sy
	}
	tx := (float64(w)-vbW*s)/2 - d.ViewBox[0]*s
	ty := (float64(h)-vbH*s)/2 - d.ViewBox[1]*s

	c := &rctx{
		w: w, h: h, s: s, tx: tx, ty: ty,
		current: current, tint: tint, opts: o, div: 1,
		mapPt: func(p Point) (float32, float32) {
			return float32(p.X*s + tx), float32(p.Y*s + ty)
		},
		// устройство → viewBox (для градиентов и картинок)
		dev2vb: Matrix{A: 1 / s, D: 1 / s, E: -tx / s, F: -ty / s},
	}
	c.drawShapes(dst, d.Shapes)
	return dst
}

// rctx — состояние одной растеризации.
type rctx struct {
	w, h      int
	s, tx, ty float64 // viewBox → пиксели: x*s+tx, y*s+ty
	current   color.RGBA
	tint      bool
	opts      Options
	mapPt     func(Point) (float32, float32)
	dev2vb    Matrix

	// div — произведение непрозрачностей открытых слоёв групп, уже лежащее в
	// прозрачности текущей фигуры (см. syncLayers); 1 — слоёв нет.
	div   float64
	spare []*image.RGBA // закрытые слои для повторного использования
	tiles map[*Pattern]*patTile

	// Буферы even-odd создаются лениво: обычным иконкам они не нужны.
	eo  *evenOddBuf
	eo2 *evenOddBuf // для clipPath: основной занят маской самой фигуры

	// Покрытия clipPath и яркости mask: каждый объект считается один раз за
	// растеризацию, даже если на него ссылаются десятки фигур.
	clips map[*ClipPath]*image.Alpha
	masks map[*Mask]*image.Alpha
}

// drawShapes рисует фигуры по порядку поверх dst. Фигуры групп с эффектами
// (см. Group) идут в отдельные слои.
func (c *rctx) drawShapes(dst *image.RGBA, shapes []Shape) {
	var ls layerState
	target := dst
	for i := range shapes {
		sh := &shapes[i]
		if len(sh.Groups) > 0 || len(ls.stack) > 0 {
			target = c.syncLayers(&ls, dst, sh)
			if c.div <= 0 {
				continue
			}
		}
		c.drawShape(target, sh)
	}
	for len(ls.stack) > 0 {
		c.closeLayer(&ls, dst)
	}
	c.div = 1
}

// drawShape рисует одну фигуру: заливку, затем обводку.
func (c *rctx) drawShape(dst *image.RGBA, sh *Shape) {
	w, h := c.w, c.h
	if sh.Image != nil {
		c.drawImage(dst, sh)
		return
	}
	fx := sh.hasEffects()

	if sh.HasFill {
		col := sh.Fill
		if sh.FillCurrent || c.tint {
			col = c.current
		}
		if sh.ColorMatrix != nil {
			col = applyColorMatrix(col, sh.ColorMatrix)
		}
		fo := sh.FillOpacity / c.div
		col = applyOpacity(col, fo)
		var gp *gradPainter
		if sh.FillGradient != nil {
			gp = newGradPainter(sh.FillGradient, c.dev2vb, c.current, c.tint, sh.ColorMatrix)
		}
		var pp *patPainter
		if sh.FillPattern != nil {
			pp = c.newPatPainter(sh.FillPattern)
		}
		if (sh.FillGradient == nil && sh.FillPattern == nil && col.A > 0) || gp != nil || pp != nil {
			var mask *image.Alpha
			if sh.EvenOdd {
				if c.eo == nil {
					c.eo = newEvenOddBuf(w, h)
				}
				mask = c.eo.raster(sh.Paths, c.mapPt)
			} else {
				mask = rasterNonzero(w, h, sh.Paths, c.mapPt)
			}
			if fx {
				mask = c.applyEffects(mask, sh)
			}
			switch {
			case gp != nil:
				blitPaint(dst, mask, fo, gp)
			case pp != nil:
				blitPaint(dst, mask, fo, pp)
			default:
				blit(dst, mask, col)
			}
		}
	}

	if sh.HasStroke {
		col := sh.Stroke
		if sh.StrokeCurrent || c.tint {
			col = c.current
		}
		if sh.ColorMatrix != nil {
			col = applyColorMatrix(col, sh.ColorMatrix)
		}
		so := sh.StrokeOpacity / c.div
		col = applyOpacity(col, so)
		var gp *gradPainter
		if sh.StrokeGradient != nil {
			gp = newGradPainter(sh.StrokeGradient, c.dev2vb, c.current, c.tint, sh.ColorMatrix)
		}
		var pp *patPainter
		if sh.StrokePattern != nil {
			pp = c.newPatPainter(sh.StrokePattern)
		}
		if (sh.StrokeGradient == nil && sh.StrokePattern == nil && col.A > 0) || gp != nil || pp != nil {
			sw := sh.StrokeWidth * c.s
			var mask *image.Alpha
			if c.opts.StrokeJoins {
				mask = rasterStrokeJoins(w, h, sh.Paths, c.strokeParams(sh, sw), c.mapPt)
			} else {
				if sw < 0.75 {
					sw = 0.75 // минимальная видимая толщина
				}
				mask = rasterStroke(w, h, sh.Paths, sw, c.mapPt)
			}
			if fx {
				mask = c.applyEffects(mask, sh)
			}
			switch {
			case gp != nil:
				blitPaint(dst, mask, so, gp)
			case pp != nil:
				blitPaint(dst, mask, so, pp)
			default:
				blit(dst, mask, col)
			}
		}
	}
}

// strokeParams переводит параметры обводки фигуры в пиксели устройства.
func (c *rctx) strokeParams(sh *Shape, sw float64) strokeParams {
	p := strokeParams{half: sw / 2, join: sh.StrokeJoin, cap: sh.StrokeCap, miter: sh.MiterLimit}
	if len(sh.Dash) > 0 {
		p.dash = make([]float64, len(sh.Dash))
		for i, d := range sh.Dash {
			p.dash[i] = d * c.s
		}
		p.dashAt = sh.DashOffset * c.s
	}
	return p
}

// hasEffects сообщает, нужны ли фигуре размытие, clip или mask.
func (sh *Shape) hasEffects() bool {
	return len(sh.Clips) > 0 || len(sh.Masks) > 0 || sh.BlurX > 0 || sh.BlurY > 0
}

// drawImage рисует фигуру-картинку (<image>).
func (c *rctx) drawImage(dst *image.RGBA, sh *Shape) {
	p := newImgPainter(sh.Image, c.dev2vb, c.current, c.tint, sh.ColorMatrix)
	if p == nil {
		return
	}
	mask := rasterNonzero(c.w, c.h, sh.Paths, c.mapPt)
	if sh.hasEffects() {
		mask = c.applyEffects(mask, sh)
	}
	blitPaint(dst, mask, sh.FillOpacity/c.div, p)
}

// applyEffects применяет к покрытию фигуры размытие, затем clip-path и mask.
// Маска mask принадлежит вызывающему (свежая или буфер даже-нечётного
// растеризатора) и изменяется на месте, кроме случая размытия (новая).
func (c *rctx) applyEffects(mask *image.Alpha, sh *Shape) *image.Alpha {
	if sh.BlurX > 0 || sh.BlurY > 0 {
		mask = blurAlpha(mask, sh.BlurX*c.s, sh.BlurY*c.s)
	}
	for _, cp := range sh.Clips {
		mulAlpha(mask, c.clipAlpha(cp))
	}
	for _, m := range sh.Masks {
		mulAlpha(mask, c.maskAlpha(m))
	}
	return mask
}

// clipAlpha — покрытие clipPath: объединение его фигур, пересечённое с Chain.
func (c *rctx) clipAlpha(cp *ClipPath) *image.Alpha {
	if a, ok := c.clips[cp]; ok {
		return a
	}
	if c.clips == nil {
		c.clips = map[*ClipPath]*image.Alpha{}
	}
	acc := image.NewAlpha(image.Rect(0, 0, c.w, c.h))
	for i := range cp.Parts {
		part := &cp.Parts[i]
		// Вложенные clip-path фигуры считаем до растеризации самой фигуры:
		// они могут занять тот же even-odd буфер.
		nested := make([]*image.Alpha, len(part.Clips))
		for j, sub := range part.Clips {
			nested[j] = c.clipAlpha(sub)
		}
		var pm *image.Alpha
		if part.EvenOdd {
			if c.eo2 == nil {
				c.eo2 = newEvenOddBuf(c.w, c.h)
			}
			pm = c.eo2.raster(part.Paths, c.mapPt)
		} else {
			pm = rasterNonzero(c.w, c.h, part.Paths, c.mapPt)
		}
		for _, n := range nested {
			mulAlpha(pm, n)
		}
		unionAlpha(acc, pm)
	}
	for _, ch := range cp.Chain {
		mulAlpha(acc, c.clipAlpha(ch))
	}
	c.remember(cp, nil, acc)
	return acc
}

// maxMemoAlphas — сколько покрытий clip/mask держим на время растеризации.
// Каждое весит w×h байт, а документ может ссылаться на тысячи разных; сверх
// предела покрытие просто считается заново.
const maxMemoAlphas = 64

func (c *rctx) remember(cp *ClipPath, m *Mask, a *image.Alpha) {
	if len(c.clips)+len(c.masks) >= maxMemoAlphas {
		return
	}
	if cp != nil {
		c.clips[cp] = a
	} else {
		c.masks[m] = a
	}
}

// maskAlpha — покрытие mask: яркость×альфа растеризованного содержимого
// (или только альфа при mask-type:alpha), ограниченная областью маски.
func (c *rctx) maskAlpha(m *Mask) *image.Alpha {
	if a, ok := c.masks[m]; ok {
		return a
	}
	if c.masks == nil {
		c.masks = map[*Mask]*image.Alpha{}
	}
	tmp := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
	sub := &rctx{
		w: c.w, h: c.h, s: c.s, tx: c.tx, ty: c.ty,
		current: color.RGBA{A: 255}, // currentColor внутри маски — чёрный
		opts:    c.opts, div: 1,
		mapPt: c.mapPt, dev2vb: c.dev2vb,
		clips: c.clips, masks: c.masks,
	}
	sub.drawShapes(tmp, m.Shapes)
	c.clips, c.masks = sub.clips, sub.masks

	out := image.NewAlpha(image.Rect(0, 0, c.w, c.h))
	for i := range out.Pix {
		p := tmp.Pix[i*4 : i*4+4 : i*4+4]
		if p[3] == 0 {
			continue
		}
		if m.Alpha {
			out.Pix[i] = p[3]
			continue
		}
		// Цвет в tmp уже умножен на альфу, так что это яркость × альфа.
		lum := (2125*uint32(p[0]) + 7154*uint32(p[1]) + 721*uint32(p[2]) + 5000) / 10000
		if lum > 255 {
			lum = 255
		}
		out.Pix[i] = uint8(lum)
	}
	if m.Region != nil {
		mulAlpha(out, c.clipAlpha(m.Region))
	}
	c.remember(nil, m, out)
	return out
}

// rasterNonzero растеризует контуры правилом ненулевого числа оборотов.
func rasterNonzero(w, h int, contours []Contour, mapPt func(Point) (float32, float32)) *image.Alpha {
	z := vector.NewRasterizer(w, h)
	for _, c := range contours {
		if len(c.Points) < 2 {
			continue
		}
		x, y := mapPt(c.Points[0])
		z.MoveTo(x, y)
		for _, p := range c.Points[1:] {
			px, py := mapPt(p)
			z.LineTo(px, py)
		}
		z.ClosePath()
	}
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// evenOddBuf — переиспользуемые буферы растеризации по правилу чётности.
type evenOddBuf struct {
	acc []float64
	tmp *image.Alpha // покрытие одного контура
	out *image.Alpha
	z   vector.Rasterizer
}

func newEvenOddBuf(w, h int) *evenOddBuf {
	return &evenOddBuf{
		acc: make([]float64, w*h),
		tmp: image.NewAlpha(image.Rect(0, 0, w, h)),
		out: image.NewAlpha(image.Rect(0, 0, w, h)),
	}
}

// raster растеризует контуры правилом чётности: покрытия складываются
// XOR-формулой acc + a - 2*acc*a.
// Результат живёт до следующего вызова.
func (b *evenOddBuf) raster(contours []Contour, mapPt func(Point) (float32, float32)) *image.Alpha {
	clear(b.acc)
	size := b.tmp.Bounds().Size()
	for _, c := range contours {
		if len(c.Points) < 2 {
			continue
		}
		clear(b.tmp.Pix)
		b.z.Reset(size.X, size.Y)
		x, y := mapPt(c.Points[0])
		b.z.MoveTo(x, y)
		for _, p := range c.Points[1:] {
			px, py := mapPt(p)
			b.z.LineTo(px, py)
		}
		b.z.ClosePath()
		b.z.Draw(b.tmp, b.tmp.Bounds(), image.Opaque, image.Point{})
		for idx, mv := range b.tmp.Pix {
			if mv == 0 {
				continue
			}
			a := float64(mv) / 255
			b.acc[idx] = b.acc[idx] + a - 2*b.acc[idx]*a
		}
	}
	clear(b.out.Pix)
	for i, v := range b.acc {
		if v <= 0 {
			continue
		}
		if v >= 1 {
			b.out.Pix[i] = 255
			continue
		}
		b.out.Pix[i] = uint8(v*255 + 0.5)
	}
	return b.out
}

// rasterStroke растеризует обводку контуров толщиной sw (пиксели), рисуя
// каждый сегмент прямоугольником-квадом (без сложных стыков/капов).
func rasterStroke(w, h int, contours []Contour, sw float64, mapPt func(Point) (float32, float32)) *image.Alpha {
	z := vector.NewRasterizer(w, h)
	half := float32(sw / 2)
	for _, c := range contours {
		n := len(c.Points)
		if n < 2 {
			continue
		}
		last := n - 1
		if c.Closed {
			last = n
		}
		for i := 0; i < last; i++ {
			p1 := c.Points[i]
			p2 := c.Points[(i+1)%n]
			ax, ay := mapPt(p1)
			bx, by := mapPt(p2)
			dx, dy := bx-ax, by-ay
			l := float32(math.Hypot(float64(dx), float64(dy)))
			if l == 0 {
				continue
			}
			// Перпендикуляр половинной толщины.
			px, py := -dy/l*half, dx/l*half
			z.MoveTo(ax+px, ay+py)
			z.LineTo(bx+px, by+py)
			z.LineTo(bx-px, by-py)
			z.LineTo(ax-px, ay-py)
			z.ClosePath()
		}
	}
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// applyOpacity умножает альфу цвета на opacity (0..1).
func applyOpacity(c color.RGBA, opacity float64) color.RGBA {
	if opacity >= 1 {
		return c
	}
	if opacity <= 0 {
		return color.RGBA{}
	}
	c.A = uint8(float64(c.A)*opacity + 0.5)
	return c
}

// pixelPainter даёт цвет (не премультиплицированный) пикселя устройства —
// для градиентов и картинок, где цвет меняется от точки к точке.
type pixelPainter interface {
	at(x, y int) color.RGBA
}

// blitPaint накладывает маску mask краской p (с общей непрозрачностью
// opacity) на dst (premultiplied RGBA) операцией Over.
func blitPaint(dst *image.RGBA, mask *image.Alpha, opacity float64, p pixelPainter) {
	if mask == nil || opacity <= 0 {
		return
	}
	b := dst.Bounds()
	w, hgt := b.Dx(), b.Dy()
	op := uint32(opacity*255 + 0.5)
	if op > 255 {
		op = 255
	}
	for y := 0; y < hgt; y++ {
		mRow := y * mask.Stride
		dOff := dst.PixOffset(0, y)
		for x := 0; x < w; x++ {
			ma := mask.Pix[mRow+x]
			if ma == 0 {
				dOff += 4
				continue
			}
			col := p.at(x, y)
			a := uint32(col.A) * op / 255 * uint32(ma) / 255
			if a == 0 {
				dOff += 4
				continue
			}
			sr := uint32(col.R) * a / 255
			sg := uint32(col.G) * a / 255
			sb := uint32(col.B) * a / 255
			inv := 255 - a
			q := dst.Pix[dOff : dOff+4 : dOff+4]
			q[0] = uint8(sr + uint32(q[0])*inv/255)
			q[1] = uint8(sg + uint32(q[1])*inv/255)
			q[2] = uint8(sb + uint32(q[2])*inv/255)
			q[3] = uint8(a + uint32(q[3])*inv/255)
			dOff += 4
		}
	}
}

// blit накладывает маску alpha цветом col (straight RGBA) на dst (premultiplied
// RGBA) операцией Over.
func blit(dst *image.RGBA, mask *image.Alpha, col color.RGBA) {
	if mask == nil {
		return
	}
	b := dst.Bounds()
	w, hgt := b.Dx(), b.Dy()
	for y := 0; y < hgt; y++ {
		mRow := y * mask.Stride
		dOff := dst.PixOffset(0, y)
		for x := 0; x < w; x++ {
			ma := mask.Pix[mRow+x]
			if ma == 0 {
				dOff += 4
				continue
			}
			// эффективная (straight) альфа источника
			a := uint32(col.A) * uint32(ma) / 255
			if a == 0 {
				dOff += 4
				continue
			}
			sr := uint32(col.R) * a / 255
			sg := uint32(col.G) * a / 255
			sb := uint32(col.B) * a / 255
			inv := 255 - a
			p := dst.Pix[dOff : dOff+4 : dOff+4]
			p[0] = uint8(sr + uint32(p[0])*inv/255)
			p[1] = uint8(sg + uint32(p[1])*inv/255)
			p[2] = uint8(sb + uint32(p[2])*inv/255)
			p[3] = uint8(a + uint32(p[3])*inv/255)
			dOff += 4
		}
	}
}
