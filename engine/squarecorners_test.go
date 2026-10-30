package engine

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// Окно ОС с прямыми углами (Windows 10, X11): корпус widget.Window рисуется
// прямоугольным, и в углу кадра — цвет заголовка, а не фон холста за
// скруглением (тёмные уголки, которые на Windows 10 раньше срезал регион).
func TestWindowSquareCorners_NoCanvasInCorner(t *testing.T) {
	testreset.Globals(t)
	corner := func(square bool) color.RGBA {
		e := New(200, 120, 30)
		defer e.Stop()
		e.SetBackground(nil)
		w := widget.NewWindow("Окно", 200, 120)
		w.CornerRadius = 8
		w.SquareCorners = square
		e.SetRoot(w)
		img := e.RenderOnce()
		return img.RGBAAt(0, 0)
	}
	rounded, square := corner(false), corner(true)
	if rounded == square {
		t.Fatalf("угол одинаков (%v) — скругление и прямые углы неотличимы", rounded)
	}
	// В прямом углу — тот же цвет, что чуть правее по заголовку.
	e := New(200, 120, 30)
	defer e.Stop()
	w := widget.NewWindow("Окно", 200, 120)
	w.CornerRadius = 8
	w.SquareCorners = true
	e.SetRoot(w)
	img := e.RenderOnce()
	if img.RGBAAt(1, 1) != img.RGBAAt(12, 1) {
		t.Errorf("прямой угол %v отличается от заголовка %v", img.RGBAAt(1, 1), img.RGBAAt(12, 1))
	}
}

// Развёрнутое окно ни одна ОС не скругляет — корпус тоже прямой.
func TestWindowMaximized_SquareCorners(t *testing.T) {
	testreset.Globals(t)
	e := New(200, 120, 30)
	defer e.Stop()
	w := widget.NewWindow("Окно", 200, 120)
	w.CornerRadius = 8
	w.SetMaximized(true)
	e.SetRoot(w)
	img := e.RenderOnce()
	if img.RGBAAt(1, 1) != img.RGBAAt(12, 1) {
		t.Errorf("угол развёрнутого окна %v отличается от заголовка %v", img.RGBAAt(1, 1), img.RGBAAt(12, 1))
	}
}
