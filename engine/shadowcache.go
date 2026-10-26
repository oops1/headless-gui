package engine

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

// SetShadowCache включает или выключает сборку теней из шаблона. Выключенный
// кэш возвращает прямое размытие на каждой отрисовке — для сравнения в
// тестах и на случай расхождения. По умолчанию включён.
func (e *Engine) SetShadowCache(on bool) {
	e.frameMu.Lock()
	e.mu.RLock()
	c := e.canvas
	e.mu.RUnlock()
	if c != nil {
		c.shadowCacheOff = !on
		c.shadowCache = nil
	}
	e.frameMu.Unlock()
	e.Invalidate()
}

// ShadowCache сообщает, включена ли сборка теней из шаблона.
func (e *Engine) ShadowCache() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.canvas == nil || !e.canvas.shadowCacheOff
}

// drawShadowFromTemplate рисует тень силуэта pw×ph с углом pCorner, размытого
// радиусом pRadius в два прохода, в области area (часть full — полной тени с
// полями margin). false — силуэт меньше шаблона, нужен прямой путь.
func (c *Canvas) drawShadowFromTemplate(full, area image.Rectangle, pw, ph, pCorner, pRadius, margin int, col color.RGBA) bool {
	if c.shadowCacheOff {
		return false
	}
	mid := pCorner + 2*pRadius // первый столбец силуэта, не задетый углами
	side := 2*mid + 1          // сторона силуэта шаблона: углы, середина, углы
	if pw < side || ph < side || margin != 2*pRadius {
		return false
	}
	t := c.shadowTemplateFor(shadowKey{corner: pCorner, radius: pRadius, col: col}, side, margin)
	tw := t.Rect.Dx()
	split := margin + mid  // индекс повторяемого столбца (и строки) шаблона
	tail := tw - split - 1 // ширина правой (нижней) части шаблона
	fw, fh := full.Dx(), full.Dy()

	w, h := area.Dx(), area.Dy()
	tmp := c.shadowScratchImage(w, h)
	ax0 := area.Min.X - full.Min.X
	ax1 := ax0 + w
	for y := 0; y < h; y++ {
		ly := area.Min.Y - full.Min.Y + y
		ty := sliceIndex(ly, split, tail, fh, tw)
		src := t.Pix[ty*t.Stride : ty*t.Stride+tw*4]
		dst := tmp.Pix[y*tmp.Stride : y*tmp.Stride+w*4]
		sliceRow(dst, src, ax0, ax1, split, tail, fw, tw)
	}
	c.drawColorGlyph(tmp, area.Min.X, area.Min.Y)
	return true
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

// sliceRow заполняет dst пикселями полной тени [ax0, ax1) строки шаблона src.
func sliceRow(dst, src []byte, ax0, ax1, split, tail, f, t int) {
	x, d := ax0, 0
	if x < split && x < ax1 {
		e := min(split, ax1)
		n := (e - x) * 4
		copy(dst[d:d+n], src[x*4:x*4+n])
		d += n
		x = e
	}
	if mEnd := f - tail; x < mEnd && x < ax1 {
		e := min(mEnd, ax1)
		n := (e - x) * 4
		fillPixel(dst[d:d+n], src[split*4:split*4+4])
		d += n
		x = e
	}
	if x < ax1 {
		sx := x - (f - t)
		n := (ax1 - x) * 4
		copy(dst[d:d+n], src[sx*4:sx*4+n])
	}
}

// fillPixel заполняет dst повторением пикселя px (удвоением копий).
func fillPixel(dst, px []byte) {
	if len(dst) == 0 {
		return
	}
	copy(dst, px)
	for n := 4; n < len(dst); n *= 2 {
		copy(dst[n:], dst[:n])
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

// shadowScratchImage — рабочий буфер w×h для собранной тени; переиспользуется
// между отрисовками (холст рисует одна горутина).
func (c *Canvas) shadowScratchImage(w, h int) *image.RGBA {
	n := w * h * 4
	if cap(c.shadowBuf) < n {
		c.shadowBuf = make([]byte, n)
	}
	return &image.RGBA{Pix: c.shadowBuf[:n], Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
}
