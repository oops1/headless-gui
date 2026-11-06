package raster

// shadowcache.go — тень из готового шаблона вместо размытия на каждом кадре
// (ADR-0002, 2.5).
//
// DrawShadow размывал силуэт заново при каждой отрисовке: буфер размером с
// тень, два прохода box-размытия по строкам и столбцам. Окно, которое тащат по
// рабочему столу, платило это на каждом кадре, а частичный кадр размывал
// только кусок в пределах отсечения — и на краю перерисованной области тень
// выходила со швом: размытие видело обрезанный силуэт.
//
// Размытие здесь — целочисленная скользящая сумма с зажимом к краю, то есть
// точное: значение пикселя зависит только от силуэта в окрестности ±2R (R —
// радиус, два прохода). Вдали от углов строки и столбцы тени одинаковы при
// любой длине стороны. Поэтому тень любого размера собирается из одного
// шаблона — силуэта минимального размера, размытого один раз: углы берутся
// как есть, середины сторон и центр повторяют одну строку и один столбец.
// Результат совпадает с прямым размытием побайтно (тест), и теперь — в том
// числе в частичном кадре.
//
// Шаблон зависит от физического радиуса угла, радиуса размытия и цвета; их
// немного (тени меню, окон, флайаутов одной темы), кэш ограничен и вытесняет
// давно не использованные.

import (
	"image"
	"image/color"
)

// maxShadowTemplates — сколько шаблонов теней держит холст.
const maxShadowTemplates = 64

type shadowKey struct {
	corner, radius int
	col            color.RGBA
}

type shadowTemplate struct {
	img  *image.RGBA
	used uint64
}

// drawShadowFromTemplate рисует тень силуэта pw×ph с углом pCorner, размытого
// радиусом pRadius в два прохода, в области area (часть full — полной тени с
// полями margin). hole — физическая область, которую тень пропускает: слой
// закрасит её сам (DrawShadowAround); пустая — тень целиком. false — силуэт
// меньше шаблона, нужен прямой путь.
func (c *Canvas) drawShadowFromTemplate(full, area, hole image.Rectangle, pw, ph, pCorner, pRadius, margin int, col color.RGBA) bool {
	if c.shadowCacheOff {
		return false
	}
	mid := pCorner + 2*pRadius // первый столбец силуэта, не задетый углами
	side := 2*mid + 1          // сторона силуэта шаблона: углы, середина, углы
	if pw < side || ph < side || margin != 2*pRadius {
		return false
	}
	cut := area.Intersect(hole)
	if cut == area {
		return true // область целиком под слоем (тайл под окном): рисовать нечего
	}
	t := c.shadowTemplateFor(shadowKey{corner: pCorner, radius: pRadius, col: col}, side, margin)
	if cut.Empty() {
		c.blitShadowTemplate(t, full, area, margin+mid)
		return true
	}
	// Кольцо вокруг выреза: полосы сверху и снизу на всю ширину области,
	// слева и справа — в строках выреза.
	for _, part := range [...]image.Rectangle{
		image.Rect(area.Min.X, area.Min.Y, area.Max.X, cut.Min.Y),
		image.Rect(area.Min.X, cut.Max.Y, area.Max.X, area.Max.Y),
		image.Rect(area.Min.X, cut.Min.Y, cut.Min.X, cut.Max.Y),
		image.Rect(cut.Max.X, cut.Min.Y, area.Max.X, cut.Max.Y),
	} {
		if !part.Empty() {
			c.blitShadowTemplate(t, full, part, margin+mid)
		}
	}
	return true
}

// blitShadowTemplate смешивает с холстом часть area полной тени full, беря
// пиксели прямо из шаблона t: без сборки во временный буфер. split — индекс
// повторяемого столбца (и строки) шаблона. Отсечения — те же, что у
// drawColorGlyph, которым тень ложилась раньше: прямоугольное и скруглённое.
func (c *Canvas) blitShadowTemplate(t *image.RGBA, full, area image.Rectangle, split int) {
	c.markKind(area)
	r := c.clampRect(area)
	if r.Empty() {
		return
	}
	tw := t.Rect.Dx()
	tail := tw - split - 1 // ширина правой (нижней) части шаблона
	fw, fh := full.Dx(), full.Dy()
	swap := c.format == FormatBGRX
	for yy := r.Min.Y; yy < r.Max.Y; yy++ {
		lx, rx, ok := c.round.spanX(yy, r.Min.X, r.Max.X)
		if !ok {
			continue
		}
		ty := sliceIndex(yy-full.Min.Y, split, tail, fh, tw)
		src := t.Pix[ty*t.Stride : ty*t.Stride+tw*4]
		off := c.back.PixOffset(lx, yy)
		overSliceRow(c.back.Pix[off:off+(rx-lx)*4], src, lx-full.Min.X, rx-full.Min.X, split, tail, fw, tw, swap)
	}
}

// sliceIndex переводит координату l полной тени длины f в координату шаблона
// длины t: начало как есть, середина — повторяемый индекс split, конец со
// сдвигом.
func sliceIndex(l, split, tail, f, t int) int {
	switch {
	case l < split:
		return l
	case l >= f-tail:
		return l - (f - t)
	default:
		return split
	}
}

// overSliceRow смешивает с dst пиксели [ax0, ax1) строки полной тени длины f,
// собранной из строки шаблона src длины t: начало и конец — как есть, середина
// — один пиксель на всю длину.
func overSliceRow(dst, src []byte, ax0, ax1, split, tail, f, t int, swap bool) {
	x, d := ax0, 0
	if x < split && x < ax1 {
		e := min(split, ax1)
		n := (e - x) * 4
		overRow(dst[d:d+n], src[x*4:x*4+n], swap)
		d += n
		x = e
	}
	if mEnd := f - tail; x < mEnd && x < ax1 {
		e := min(mEnd, ax1)
		n := (e - x) * 4
		overFill(dst[d:d+n], src[split*4:split*4+4], swap)
		d += n
		x = e
	}
	if x < ax1 {
		sx := x - (f - t)
		n := (ax1 - x) * 4
		overRow(dst[d:d+n], src[sx*4:sx*4+n], swap)
	}
}

// shadowTemplateFor возвращает шаблон для ключа, строя его при первом
// запросе: силуэт side×side с полями margin, размытый так же, как прямой путь.
func (c *Canvas) shadowTemplateFor(k shadowKey, side, margin int) *image.RGBA {
	c.shadowClock++
	if t, ok := c.shadowCache[k]; ok {
		t.used = c.shadowClock
		return t.img
	}
	if c.shadowCache == nil {
		c.shadowCache = make(map[shadowKey]*shadowTemplate)
	}
	if len(c.shadowCache) >= maxShadowTemplates {
		var oldK shadowKey
		var oldUsed uint64 = ^uint64(0)
		for key, t := range c.shadowCache {
			if t.used < oldUsed {
				oldK, oldUsed = key, t.used
			}
		}
		delete(c.shadowCache, oldK)
	}
	n := side + 2*margin
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	fillSilhouette(img, margin, margin, side, side, k.corner, k.col)
	BlurRGBA(img, k.radius, 2)
	c.shadowCache[k] = &shadowTemplate{img: img, used: c.shadowClock}
	return img
}
