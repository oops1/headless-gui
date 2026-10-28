package raster

import (
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"image"
	"image/color"
	"testing"
)

// Кэш ограничен и вытесняет давно не использованные шаблоны.
func TestShadowCache_Bounded(t *testing.T) {
	testreset.Globals(t)
	c := engineCanvas(400, 300)
	for i := 0; i < maxShadowTemplates+10; i++ {
		col := color.RGBA{A: uint8(10 + i)}
		c.DrawShadow(image.Rect(50, 50, 250, 200), 6, 12, 0, 6, col)
	}
	if n := len(c.shadowCache); n > maxShadowTemplates {
		t.Fatalf("шаблонов %d, предел %d", n, maxShadowTemplates)
	}
	// Последний использованный — в кэше.
	last := shadowKey{corner: 6, radius: 12, col: color.RGBA{A: uint8(10 + maxShadowTemplates + 9)}}
	if _, ok := c.shadowCache[last]; !ok {
		t.Fatal("последний шаблон вытеснен")
	}
}
