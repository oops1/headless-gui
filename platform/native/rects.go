package native

// rects.go — сравнение списков прямоугольников (регионы окна).

import (
	"image"
)

// SameRects — одинаковые наборы прямоугольников в одном порядке (общая:
// ею пользуются и форма окна-попапа Win32, и X11).
//
//facadegen:skip
func SameRects(a, b []image.Rectangle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
