package engine

import (
	"image"
	"image/color"
	"testing"
)

// GG-73: пустая область отсечения снимала отсечение вовсе. В полном кадре
// SetClip с пустым пересечением выключал клип, и всё дальнейшее рисование шло
// по всему кадру; в частичном — правильно ничего не рисовал.

func clipCanvas(t *testing.T) *Canvas {
	t.Helper()
	e := New(120, 80, 20)
	c := e.canvas
	c.FillRect(0, 0, 120, 80, color.RGBA{A: 255})
	return c
}

func countColor(img *image.RGBA, col color.RGBA) int {
	n := 0
	for i := 0; i+3 < len(img.Pix); i += 4 {
		if img.Pix[i] == col.R && img.Pix[i+1] == col.G && img.Pix[i+2] == col.B {
			n++
		}
	}
	return n
}

// Клип за краем холста в полном кадре — ничего не рисуется.
func TestCanvas_EmptyClipDrawsNothing(t *testing.T) {
	c := clipCanvas(t)
	red := color.RGBA{R: 255, A: 255}

	c.SetClip(image.Rect(-60, -60, -10, -10)) // пересечение с холстом пустое
	if !c.Clip().Empty() {
		t.Fatalf("Clip после пустого SetClip = %v, ждал пустой", c.Clip())
	}
	c.FillRect(0, 0, 120, 80, red)
	c.DrawText("клип", 5, 5, red)
	c.SetPixel(10, 10, red)
	if n := countColor(c.back, red); n != 0 {
		t.Fatalf("при пустом клипе нарисовано %d точек — отсечение снялось", n)
	}

	// Сужение пустого клипа тоже пустое — не «весь кадр».
	c.SetClip(c.Clip().Intersect(image.Rect(0, 0, 50, 50)))
	c.FillRect(0, 0, 120, 80, red)
	if n := countColor(c.back, red); n != 0 {
		t.Fatalf("сужение пустого клипа нарисовало %d точек", n)
	}

	// ClearClip возвращает рисование по всему холсту.
	c.ClearClip()
	c.FillRect(0, 0, 120, 80, red)
	if n := countColor(c.back, red); n == 0 {
		t.Fatal("после ClearClip рисование не вернулось")
	}
}

// Полный и частичный кадр ведут себя одинаково: пустой клип в обоих случаях —
// «ничего».
func TestCanvas_EmptyClipSameInPartialFrame(t *testing.T) {
	c := clipCanvas(t)
	red := color.RGBA{R: 255, A: 255}

	c.setBaseClip(image.Rect(0, 0, 120, 80))
	c.SetClip(image.Rect(200, 200, 260, 260))
	c.FillRect(0, 0, 120, 80, red)
	c.clearBaseClip()
	if n := countColor(c.back, red); n != 0 {
		t.Fatalf("частичный кадр: при пустом клипе нарисовано %d точек", n)
	}
}
