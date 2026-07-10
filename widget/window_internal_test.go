package widget

import (
	"image/color"
	"testing"
)

// TestXorBorderColor_Inversion — чистая функция: побитовое НЕ каждого канала,
// alpha = 255.
func TestXorBorderColor_Inversion(t *testing.T) {
	cases := []struct {
		bg   color.RGBA
		want color.RGBA
	}{
		{color.RGBA{R: 240, G: 240, B: 240, A: 255}, color.RGBA{R: 15, G: 15, B: 15, A: 255}},
		{color.RGBA{R: 0, G: 0, B: 0, A: 255}, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		{color.RGBA{R: 255, G: 255, B: 255, A: 255}, color.RGBA{R: 0, G: 0, B: 0, A: 255}},
		{color.RGBA{R: 30, G: 30, B: 46, A: 200}, color.RGBA{R: 225, G: 225, B: 209, A: 255}},
	}
	for _, c := range cases {
		got := xorBorderColor(c.bg)
		if got != c.want {
			t.Fatalf("xorBorderColor(%v) = %v, want %v", c.bg, got, c.want)
		}
	}
}

// TestXorBorderBase_Fallback — при полупрозрачном/нулевом фоне (A==0) берётся
// WindowBG темы.
func TestXorBorderBase_Fallback(t *testing.T) {
	w := NewWindow("t", 100, 100)

	w.Background = color.RGBA{R: 12, G: 34, B: 56, A: 255}
	if got := w.xorBorderBase(); got != w.Background {
		t.Fatalf("непрозрачный фон: base = %v, want %v", got, w.Background)
	}

	w.Background = color.RGBA{} // A == 0
	if got := w.xorBorderBase(); got != win10.WindowBG {
		t.Fatalf("нулевой фон: base = %v, want WindowBG %v", got, win10.WindowBG)
	}
}
