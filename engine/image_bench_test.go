package engine

import (
	"image"
	"image/color"
	"testing"
)

// benchIcon — исходная иконка 64×64 с неоднородным содержимым.
func benchIcon() *image.RGBA {
	ic := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			ic.SetRGBA(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 4), B: 128, A: 255})
		}
	}
	return ic
}

// BenchmarkDrawImageScaled_Icon64to24: повторная отрисовка одной иконки.
func BenchmarkDrawImageScaled_Icon64to24(b *testing.B) {
	c := newCanvas(200, 200, newFontCache("assets"))
	ic := benchIcon()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.DrawImageScaled(ic, 10, 10, 24, 24)
	}
}

// BenchmarkDrawImageScaled_MixedSizes: несколько иконок разных размеров.
func BenchmarkDrawImageScaled_MixedSizes(b *testing.B) {
	c := newCanvas(200, 200, newFontCache("assets"))
	icons := []*image.RGBA{benchIcon(), benchIcon(), benchIcon(), benchIcon()}
	sizes := []int{16, 24, 32, 48}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j, ic := range icons {
			s := sizes[j]
			c.DrawImageScaled(ic, 10, 10, s, s)
		}
	}
}
