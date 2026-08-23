// Тест DockPanel (ENGINE_ISSUES tts-studio #7):
// SetBounds до AddChild не раздувает первого ребёнка.
package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestDockPanel_SetBoundsBeforeAddChild — репро из ENGINE_ISSUES #7:
// dp.SetBounds → AddChild(top26) → AddChild(fill); top должен резервировать
// свои 26px, а не всю панель.
func TestDockPanel_SetBoundsBeforeAddChild(t *testing.T) {
	dp := widget.NewDockPanel()
	dp.SetBounds(image.Rect(0, 0, 290, 100))

	top := widget.NewWin10Label("шапка")
	top.SetBounds(image.Rect(0, 0, 290, 26))
	top.SetDock(widget.DockTop)
	dp.AddChild(top)

	fill := widget.NewPanel(color.RGBA{R: 64, G: 64, B: 64, A: 255})
	fill.ShowHeader = false
	dp.AddChild(fill)

	if got := top.Bounds().Dy(); got != 26 {
		t.Errorf("высота top = %d, ждали 26 (бывший Fill не восстановил размер)", got)
	}
	if got := fill.Bounds().Min.Y; got != 26 {
		t.Errorf("fill.Min.Y = %d, ждали 26 (зазор сверху)", got)
	}
}
