package widget

import (
	"image/color"
	"math"
)

// contrast.go — контраст цвета текста и фона по WCAG 2.
//
// luminance (button.go) — быстрая взвешенная сумма каналов для выбора «светлое
// или тёмное»; для порога читаемости её мало: восприятие яркости нелинейно, и
// цвет, который она считает достаточно тёмным, на деле может не дотягивать до
// контраста 4,5:1.

// wcagMinContrast — порог WCAG AA для обычного текста.
const wcagMinContrast = 4.5

// wcagLuminance — относительная яркость цвета по WCAG 2, от 0 до 1.
func wcagLuminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// wcagContrast — контраст двух цветов, от 1 до 21.
func wcagContrast(a, b color.RGBA) float64 {
	la, lb := wcagLuminance(a), wcagLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// readableOn доводит цвет текста до читаемости на фоне: на светлом фоне
// затемняет, на тёмном осветляет, пока контраст не дойдёт до порога WCAG AA.
// Смешивание идёт малыми шагами к чёрному или белому, поэтому оттенок
// сохраняется — зелёный остаётся зелёным. Цвет, который уже читается,
// возвращается как есть.
func readableOn(c, bg color.RGBA) color.RGBA {
	c.A, bg.A = 255, 255
	target := color.RGBA{A: 255}
	if wcagLuminance(bg) < 0.18 {
		target = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	for i := 0; i < 40 && wcagContrast(c, bg) < wcagMinContrast; i++ {
		c = mixRGBA(c, target, 0.06)
	}
	return c
}
