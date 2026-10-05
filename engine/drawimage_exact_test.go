package engine

import (
	"image"
	"image/color"
	"testing"
)

// Картинка, размер которой равен физическому размеру приёмника, ложится на
// холст пиксель в пиксель — без передискретизации (так рисуются векторные
// значки, растеризованные под масштаб: widget.DrawSVG).
func TestDrawImageScaled_ExactPhysicalSizeIsBlitted(t *testing.T) {
	eng := New(40, 40, 20)
	eng.SetScale(1.5)
	c := eng.canvas
	c.blitBackground()

	// Логический приёмник 10×10 в (4, 4) — физически (6, 6)…(21, 21), 15×15.
	src := image.NewRGBA(image.Rect(0, 0, 15, 15))
	for y := 0; y < 15; y++ {
		for x := 0; x < 15; x++ {
			// Шахматка единичных пикселей: любое растяжение её смажет.
			v := uint8(0)
			if (x+y)%2 == 0 {
				v = 255
			}
			src.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	c.DrawImageScaled(src, 4, 4, 10, 10)

	for y := 0; y < 15; y++ {
		for x := 0; x < 15; x++ {
			off := c.back.PixOffset(6+x, 6+y)
			got := c.back.Pix[off]
			want := src.RGBAAt(x, y).R
			if got != want {
				t.Fatalf("(%d,%d): %d, ожидалось %d — картинка передискретизирована", x, y, got, want)
			}
		}
	}
}
