package engine

import (
	"image"
	"image/color"
	"testing"
)

// TestCanvasSnapshot_CopiesFilledRegion: Snapshot возвращает независимую копию
// области back-буфера с известной заливкой (пиксельная проверка).
func TestCanvasSnapshot_CopiesFilledRegion(t *testing.T) {
	e := New(100, 80, 20)
	c := e.canvas
	c.blitBackground() // чёрный фон

	fill := color.RGBA{R: 10, G: 200, B: 60, A: 255}
	c.FillRect(20, 15, 30, 25, fill)

	snap := c.Snapshot(image.Rect(20, 15, 50, 40))
	if snap == nil {
		t.Fatal("Snapshot вернул nil для непустой области")
	}
	if got := snap.Bounds(); got != image.Rect(0, 0, 30, 25) {
		t.Fatalf("размер снимка %v, want 30x25 с началом (0,0)", got)
	}
	for y := 0; y < 25; y++ {
		for x := 0; x < 30; x++ {
			if got := snap.RGBAAt(x, y); got != fill {
				t.Fatalf("пиксель снимка (%d,%d) = %v, want %v", x, y, got, fill)
			}
		}
	}

	// Снимок независим: последующая перерисовка back его не меняет.
	c.FillRect(20, 15, 30, 25, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	if got := snap.RGBAAt(0, 0); got != fill {
		t.Fatalf("снимок изменился после перерисовки back: %v, want %v", got, fill)
	}
}

// TestCanvasSnapshot_ClipAndEmpty: область клипится по границам холста;
// полностью вне холста → nil.
func TestCanvasSnapshot_ClipAndEmpty(t *testing.T) {
	e := New(60, 40, 20)
	c := e.canvas
	c.blitBackground()

	// Частично за холстом → клип до видимой части (20x20).
	snap := c.Snapshot(image.Rect(40, 20, 200, 200))
	if snap == nil {
		t.Fatal("nil для частично видимой области")
	}
	if got := snap.Bounds(); got != image.Rect(0, 0, 20, 20) {
		t.Fatalf("клип %v, want 20x20", got)
	}
	// Полностью вне холста → nil.
	if s := c.Snapshot(image.Rect(100, 100, 120, 120)); s != nil {
		t.Fatalf("ожидался nil вне холста, got %v", s.Bounds())
	}
}

// TestCanvasSnapshot_HiDPIPhysicalPixels: при scale>1 снимок отдаёт ФИЗИЧЕСКИЕ
// пиксели (логический прямоугольник × scale).
func TestCanvasSnapshot_HiDPIPhysicalPixels(t *testing.T) {
	e := New(100, 80, 20)
	e.SetScale(2)
	c := e.canvas
	c.blitBackground()

	fill := color.RGBA{R: 200, G: 30, B: 30, A: 255}
	c.FillRect(10, 10, 20, 20, fill)

	snap := c.Snapshot(image.Rect(10, 10, 30, 30))
	if snap == nil {
		t.Fatal("nil при scale=2")
	}
	// 20 логических × 2 = 40 физических.
	if got := snap.Bounds(); got != image.Rect(0, 0, 40, 40) {
		t.Fatalf("размер снимка %v, want 40x40 (физические пиксели)", got)
	}
	if got := snap.RGBAAt(20, 20); got != fill {
		t.Fatalf("центр снимка %v, want %v", got, fill)
	}
}
