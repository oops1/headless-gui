package desktop_test

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/widget"
)

// Плитка, заехавшая под край окна сетки, не рисуется за его пределами: ни над
// панелью, ни на ползунках (скруглённый слой не должен сбрасывать отсечение).
func TestQuickPanel_ScrolledTilesStayInGrid(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	q := s.q
	rest := s.render()
	grid := desktop.QSZoneRect(q, "grid")
	q.OnMouseWheelPixels(grid.Min.X+5, grid.Min.Y+5, 0, 60)
	scrolled := s.render()
	panel := desktop.QSPanelRect(q)
	bands := []image.Rectangle{
		image.Rect(grid.Min.X, panel.Min.Y-30, grid.Max.X, panel.Min.Y-2), // над панелью
		image.Rect(grid.Min.X, panel.Min.Y+2, grid.Max.X, grid.Min.Y-1),   // над окном сетки
		image.Rect(grid.Min.X, grid.Max.Y+1, grid.Max.X, grid.Max.Y+10),   // под окном сетки
	}
	for _, b := range bands {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if rest.RGBAAt(x, y) != scrolled.RGBAAt(x, y) {
					t.Fatalf("пиксель (%d,%d) вне окна сетки изменился при прокрутке", x, y)
				}
			}
		}
	}
}

// Пока страницы едут вбок, ничего не рисуется за краем панели: ни плитки, ни
// бегунки ползунков, ни нижняя полоса.
func TestQuickPanel_TransitionStaysInsidePanel(t *testing.T) {
	for _, dark := range []bool{false, true} {
		s := qsBuild(t, qsOpts{dark: dark})
		q := s.q
		rest := s.render()
		panel := desktop.QSPanelRect(q)
		qsClick(q, centreOf(desktop.QSChevronRect(q, "bt")))
		t0 := time.Now()
		widget.StepAnimations(t0)
		widget.StepAnimations(t0.Add(90 * time.Millisecond))
		if p := desktop.QSPage(q); p <= 0.05 || p >= 0.95 {
			t.Fatalf("переход не на середине: %v", p)
		}
		mid := s.render()
		// Слева и справа от панели, с запасом на тень.
		for _, b := range []image.Rectangle{
			image.Rect(panel.Min.X-320, panel.Min.Y, panel.Min.X-50, panel.Max.Y),
			image.Rect(panel.Max.X+50, panel.Min.Y, panel.Max.X+60, panel.Max.Y),
		} {
			b = b.Intersect(rest.Bounds())
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					if rest.RGBAAt(x, y) != mid.RGBAAt(x, y) {
						t.Fatalf("тёмная=%v: пиксель (%d,%d) вне панели изменился в переходе", dark, x, y)
					}
				}
			}
		}
	}
}
