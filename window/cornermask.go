package window

// cornermask.go — скруглённые углы окна в буфере с альфой.
//
// Окно со скруглением рисует свои углы само, но буфер верхнего окна под
// Wayland был XRGB: пиксели за дугой оставались непрозрачными и чёрными, и
// поверх рабочего стола (особенно в RemoteApp, где окно показывается само по
// себе) в четырёх углах были видны чёрные уголки (задание WinLine, R3).
//
// Теперь у такого окна буфер ARGB с домноженной альфой, и после обычного
// преобразования кадра на углы накладывается маска: за дугой пиксель
// полностью прозрачный (0,0,0,0), на самой дуге — частичная альфа по
// покрытию пикселя, внутри — без изменений.

import (
	"image"
	"math"
)

// applyCornerMask домножает пиксели BGRA в четырёх углах буфера w×h на
// покрытие скруглённого прямоугольника радиуса r. Трогает только пиксели
// внутри area. Буфер — непрозрачный (как его оставляет convRectBGRX), так
// что домножение на покрытие и есть домноженная альфа.
func applyCornerMask(dst []byte, stride, w, h, r int, area image.Rectangle) {
	if r <= 0 || w <= 0 || h <= 0 {
		return
	}
	r = min(r, w/2, h/2)
	area = area.Intersect(image.Rect(0, 0, w, h))
	if area.Empty() || r <= 0 {
		return
	}
	corners := [4]struct {
		sq     image.Rectangle
		cx, cy float64 // центр дуги
	}{
		{image.Rect(0, 0, r, r), float64(r), float64(r)},
		{image.Rect(w-r, 0, w, r), float64(w - r), float64(r)},
		{image.Rect(0, h-r, r, h), float64(r), float64(h - r)},
		{image.Rect(w-r, h-r, w, h), float64(w - r), float64(h - r)},
	}
	rf := float64(r)
	for _, c := range corners {
		sq := c.sq.Intersect(area)
		for y := sq.Min.Y; y < sq.Max.Y; y++ {
			row := y * stride
			for x := sq.Min.X; x < sq.Max.X; x++ {
				// Покрытие по расстоянию центра пикселя до дуги: сглаживание
				// в одну точку шириной.
				dx := float64(x) + 0.5 - c.cx
				dy := float64(y) + 0.5 - c.cy
				cov := rf + 0.5 - math.Sqrt(dx*dx+dy*dy)
				if cov >= 1 {
					continue
				}
				off := row + x*4
				if cov <= 0 {
					dst[off], dst[off+1], dst[off+2], dst[off+3] = 0, 0, 0, 0
					continue
				}
				k := uint32(cov*255 + 0.5)
				for i := 0; i < 4; i++ {
					dst[off+i] = byte((uint32(dst[off+i])*k + 127) / 255)
				}
			}
		}
	}
}
