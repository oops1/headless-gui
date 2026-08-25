// Ресайз за края и перехват перемещения — независимые вещи
// (ENGINE_ISSUES winline: «Setting OnDragMove costs edge resizing»).
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestWindow_OnDragMove_KeepsEdgeResize — приложение забрало себе
// перемещение окна, но ресайз за края продолжает работать.
func TestWindow_OnDragMove_KeepsEdgeResize(t *testing.T) {
	w := widget.NewWindow("win", 400, 300)
	w.SetBounds(image.Rect(100, 100, 500, 400))
	w.OnDragMove = func(dx, dy int) {} // перемещение ведёт приложение

	if c := w.Cursor(499, 250); c != widget.CursorSizeWE {
		t.Errorf("правый край: Cursor = %v, ждали SizeWE", c)
	}

	pressAt(w, 499, 250)
	w.OnMouseMove(560, 250)
	releaseAt(w, 560, 250)

	if got := w.Bounds().Dx(); got != 461 {
		t.Errorf("ширина после ресайза = %d, ждали 461 (окно не изменило размер)", got)
	}
}
