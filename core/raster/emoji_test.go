package raster

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// TestEmoji_RenderColorPixels — рендер «🙂» реальным цветным шрифтом
// (Segoe UI Emoji и т.п.): даёт непрозрачные ЦВЕТНЫЕ (не серые) пиксели.
// Пропускается, если в системе нет шрифта с покрытием эмодзи.
func TestEmoji_RenderColorPixels(t *testing.T) {
	testreset.Globals(t)
	c := engineCanvas(120, 80)
	if _, found := c.fcForRune(c.fontCache, '🙂'); !found {
		t.Skip("нет системного шрифта с цветными эмодзи (напр. seguiemj.ttf) — пропуск")
	}
	c.BlitBackground()
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	c.DrawTextSize("🙂", 8, 8, 32, white)

	colored, opaque := 0, 0
	for i := 0; i < len(c.back.Pix); i += 4 {
		r, g, b, a := c.back.Pix[i], c.back.Pix[i+1], c.back.Pix[i+2], c.back.Pix[i+3]
		if a == 255 && (r != 0 || g != 0 || b != 0) {
			opaque++
		}
		// «Цветной» = каналы различаются (серый/белый текст дал бы R==G==B).
		if maxu8(r, g, b)-minu8(r, g, b) > 24 {
			colored++
		}
	}
	if opaque == 0 {
		t.Fatal("после отрисовки эмодзи нет непрозрачных пикселей — глиф не нарисован")
	}
	if colored == 0 {
		t.Errorf("эмодзи отрисован, но нет цветных пикселей (%d непрозрачных) — цвет не применён", opaque)
	}
}

func maxu8(a, b, c uint8) uint8 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

func minu8(a, b, c uint8) uint8 {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
