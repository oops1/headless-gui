package engine

import (
	"image"
	"sync"
)

// Рисование с прозрачностью слоя — появление и исчезновение всплывающих
// панелей.
//
// Панель — это десятки примитивов: подложка со стеклом и тенью, строки,
// значки, текст. Множить альфу каждого по отдельности нельзя: полупрозрачные
// скруглённые заливки пишутся без смешивания, размытая подложка берёт то, что
// под ней, а перекрывающиеся слои просвечивали бы друг через друга. Нужна
// прозрачность СЛОЯ — как у группы в SVG: сначала слой рисуется целиком и
// непрозрачно, потом смешивается с тем, что лежало под ним.
//
// Здесь это сделано без отдельного буфера: перед рисованием область
// снимается из кадра, слой рисуется на месте — со всеми его размытиями и
// тенями, которые видят настоящую подложку, — а затем каждый пиксель области
// заменяется смесью «было» и «стало». Пиксель, которого слой не коснулся,
// смешивается сам с собой и не меняется.

// opacityScratch — буфер снимка области: кадр на каждый тик анимации не должен
// плодить мусор размером с панель.
var opacityScratch = sync.Pool{New: func() any { return new([]byte) }}

// DrawWithOpacity выполняет draw, а затем смешивает результат в области r с тем,
// что было в кадре до него: alpha=1 — слой как нарисован, alpha=0 — его нет.
//
// r — логические координаты. Область обрезается отсечением канваса, как и
// любая другая запись. alpha вне [0,1] зажимается; при alpha ≤ 0 draw не
// вызывается вовсе — смотреть не на что.
//
// Реализует widget.OpacityDrawer.
func (c *Canvas) DrawWithOpacity(r image.Rectangle, alpha float64, draw func()) {
	if draw == nil {
		return
	}
	if alpha >= 1 {
		draw()
		return
	}
	if alpha <= 0 {
		return
	}
	area := c.clampRect(c.sRect(r)).Intersect(c.back.Bounds())
	if area.Empty() {
		draw()
		return
	}

	rowBytes := area.Dx() * 4
	bufp := opacityScratch.Get().(*[]byte)
	defer opacityScratch.Put(bufp)
	if need := rowBytes * area.Dy(); cap(*bufp) < need {
		*bufp = make([]byte, need)
	} else {
		*bufp = (*bufp)[:need]
	}
	under := *bufp
	for y := area.Min.Y; y < area.Max.Y; y++ {
		off := c.back.PixOffset(area.Min.X, y)
		copy(under[(y-area.Min.Y)*rowBytes:], c.back.Pix[off:off+rowBytes])
	}

	draw()

	// Смесь в 8-битной дроби: a/256 вместо плавающей арифметики на пиксель.
	a := int(alpha*256 + 0.5)
	if a > 256 {
		a = 256
	}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		off := c.back.PixOffset(area.Min.X, y)
		now := c.back.Pix[off : off+rowBytes]
		was := under[(y-area.Min.Y)*rowBytes : (y-area.Min.Y+1)*rowBytes]
		for i, n := range now {
			w := was[i]
			if n != w {
				now[i] = uint8((int(w)*(256-a) + int(n)*a + 128) >> 8)
			}
		}
	}
	// Для потребителя кадра область — картинка: смесь не сплошной цвет.
	c.markImage(area)
}
