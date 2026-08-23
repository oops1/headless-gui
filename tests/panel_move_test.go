// Тест Panel.SetBounds (ENGINE_ISSUES avanpost-pam, issue B):
// сдвиг панели сдвигает детей.
package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestPanel_SetBounds_MovesChildren — сдвиг Panel сдвигает детей (issue B).
func TestPanel_SetBounds_MovesChildren(t *testing.T) {
	p := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
	p.ShowHeader = false
	p.SetBounds(image.Rect(0, 0, 200, 100))

	b := widget.NewButton("ok")
	b.SetBounds(image.Rect(10, 20, 90, 50))
	p.AddChild(b)

	p.SetBounds(image.Rect(50, 40, 250, 140))

	want := image.Rect(60, 60, 140, 90)
	if b.Bounds() != want {
		t.Errorf("b.Bounds() = %v, ждали %v (дети не сдвинулись)", b.Bounds(), want)
	}

	// Ресайз без перемещения — дети на месте.
	p.SetBounds(image.Rect(50, 40, 300, 200))
	if b.Bounds() != want {
		t.Errorf("после ресайза: %v, ждали %v", b.Bounds(), want)
	}
}
