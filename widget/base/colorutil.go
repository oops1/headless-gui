package base

// colorutil.go — смешение и осветление цвета, круг: общие помощники рисования.

import "image/color"

// MixRGBA линейно смешивает a→b на долю t (0 = a, 1 = b).
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func MixRGBA(a, b color.RGBA, t float64) color.RGBA {
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: a.A}
}

// FillCircle рисует закрашенный круг (для traffic lights).
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func FillCircle(ctx DrawContext, cx, cy, r int, col color.RGBA) {
	ctx.FillRoundRect(cx-r, cy-r, r*2, r*2, r, col)
}

// Brighten увеличивает яркость каждого канала на delta (без переполнения).
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func Brighten(c color.RGBA, delta uint8) color.RGBA {
	add := func(v, d uint8) uint8 {
		s := uint16(v) + uint16(d)
		if s > 255 {
			return 255
		}
		return uint8(s)
	}
	return color.RGBA{R: add(c.R, delta), G: add(c.G, delta), B: add(c.B, delta), A: c.A}
}
