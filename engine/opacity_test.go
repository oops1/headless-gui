package engine

import (
	"image"
	"image/color"
	"testing"
)

func solidCanvas(w, h int, c color.RGBA) *Canvas {
	cv := newCanvas(w, h, newFontCache("assets"))
	cv.blitBackground()
	cv.FillRect(0, 0, w, h, c)
	return cv
}

func near(a, b uint8) bool {
	d := int(a) - int(b)
	return d >= -1 && d <= 1
}

// TestDrawWithOpacity_MixesLayerWithWhatWasUnderIt — слой с прозрачностью 0,5
// даёт среднее между прежним кадром и нарисованным.
func TestDrawWithOpacity_MixesLayerWithWhatWasUnderIt(t *testing.T) {
	c := solidCanvas(100, 100, color.RGBA{R: 200, A: 255})
	area := image.Rect(20, 20, 80, 80)

	c.DrawWithOpacity(area, 0.5, func() {
		c.FillRect(30, 30, 40, 40, color.RGBA{B: 100, A: 255})
	})

	in := c.back.RGBAAt(50, 50)
	if !near(in.R, 100) || !near(in.B, 50) || in.A != 255 {
		t.Errorf("внутри слоя %v, ждали около {100 0 50 255}", in)
	}
	// Область слоя, которой слой не коснулся, остаётся как была.
	if got := c.back.RGBAAt(25, 25); got != (color.RGBA{R: 200, A: 255}) {
		t.Errorf("нетронутая часть области изменилась: %v", got)
	}
	// И всё за областью.
	if got := c.back.RGBAAt(5, 5); got != (color.RGBA{R: 200, A: 255}) {
		t.Errorf("пиксель вне области изменился: %v", got)
	}
}

func TestDrawWithOpacity_Extremes(t *testing.T) {
	base := color.RGBA{R: 200, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	area := image.Rect(10, 10, 60, 60)

	c := solidCanvas(80, 80, base)
	called := false
	c.DrawWithOpacity(area, 0, func() { called = true; c.FillRect(10, 10, 50, 50, blue) })
	if called || c.back.RGBAAt(30, 30) != base {
		t.Errorf("при прозрачности 0 слой должен пропускаться (вызван=%v, пиксель %v)", called, c.back.RGBAAt(30, 30))
	}

	c = solidCanvas(80, 80, base)
	c.DrawWithOpacity(area, 1, func() { c.FillRect(10, 10, 50, 50, blue) })
	if c.back.RGBAAt(30, 30) != blue {
		t.Errorf("при прозрачности 1 слой должен лечь как есть: %v", c.back.RGBAAt(30, 30))
	}

	// Вне [0,1] зажимается.
	c = solidCanvas(80, 80, base)
	c.DrawWithOpacity(area, 7, func() { c.FillRect(10, 10, 50, 50, blue) })
	if c.back.RGBAAt(30, 30) != blue {
		t.Errorf("прозрачность 7 не зажата до 1: %v", c.back.RGBAAt(30, 30))
	}
}

// TestDrawWithOpacity_RespectsClip — отсечение канваса действует и на смесь.
func TestDrawWithOpacity_RespectsClip(t *testing.T) {
	base := color.RGBA{R: 200, A: 255}
	c := solidCanvas(80, 80, base)
	c.SetClip(image.Rect(0, 0, 40, 80))
	c.DrawWithOpacity(image.Rect(0, 0, 80, 80), 0.5, func() {
		c.FillRect(0, 0, 80, 80, color.RGBA{B: 100, A: 255})
	})
	if got := c.back.RGBAAt(60, 40); got != base {
		t.Errorf("за отсечением пиксель изменился: %v", got)
	}
	if got := c.back.RGBAAt(20, 40); got == base {
		t.Error("внутри отсечения слой не лёг")
	}
}

// TestDrawWithOpacity_BlurSeesTheRealBackdrop — размытие внутри слоя берёт
// настоящую подложку, а не пустоту: стекло проявляется стеклом.
func TestDrawWithOpacity_BlurSeesTheRealBackdrop(t *testing.T) {
	area := image.Rect(20, 20, 100, 100)
	tint := color.RGBA{R: 10, G: 10, B: 10, A: 40}

	ref := checkerCanvas(120, 120, 10)
	ref.BlurBehind(area, 8, tint)

	under := checkerCanvas(120, 120, 10)
	got := checkerCanvas(120, 120, 10)
	got.DrawWithOpacity(area, 0.5, func() { got.BlurBehind(area, 8, tint) })

	for _, p := range []image.Point{{40, 40}, {60, 60}, {75, 55}} {
		u, r, g := under.back.RGBAAt(p.X, p.Y), ref.back.RGBAAt(p.X, p.Y), got.back.RGBAAt(p.X, p.Y)
		want := color.RGBA{
			R: uint8((int(u.R) + int(r.R) + 1) / 2),
			G: uint8((int(u.G) + int(r.G) + 1) / 2),
			B: uint8((int(u.B) + int(r.B) + 1) / 2),
			A: 255,
		}
		if !near(g.R, want.R) || !near(g.G, want.G) || !near(g.B, want.B) {
			t.Errorf("(%d,%d): %v, ждали среднее %v между подложкой %v и размытием %v", p.X, p.Y, g, want, u, r)
		}
	}
}
