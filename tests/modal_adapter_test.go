// Package tests — тесты ModalAdapter.
package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func TestModalAdapter_IsModal(t *testing.T) {
	p := widget.NewPanel(color.RGBA{A: 255})
	m := widget.NewModalAdapter(p)
	if !m.IsModal() {
		t.Fatal("ModalAdapter.IsModal() should return true")
	}
}

func TestModalAdapter_DimColor_Default(t *testing.T) {
	p := widget.NewPanel(color.RGBA{A: 255})
	m := widget.NewModalAdapter(p)
	dim := m.DimColor()
	if dim.A != 140 {
		t.Fatalf("default DimColor alpha = %d, want 140", dim.A)
	}
}

func TestModalAdapter_DimColor_Custom(t *testing.T) {
	p := widget.NewPanel(color.RGBA{A: 255})
	custom := color.RGBA{R: 0, G: 0, B: 0, A: 200}
	m := widget.NewModalAdapterWithDim(p, custom)
	if m.DimColor() != custom {
		t.Fatalf("DimColor = %v, want %v", m.DimColor(), custom)
	}
}

func TestModalAdapter_DelegatesMouseClick(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(0, 0, 100, 40))
	clicked := false
	btn.OnClick = func() { clicked = true }

	m := widget.NewModalAdapter(btn)
	m.SetBounds(image.Rect(0, 0, 100, 40))

	// Нажатие + отпускание через ModalAdapter
	m.OnMouseButton(widget.MouseEvent{X: 50, Y: 20, Button: widget.MouseLeft, Pressed: true})
	m.OnMouseButton(widget.MouseEvent{X: 50, Y: 20, Button: widget.MouseLeft, Pressed: false})
	if !clicked {
		t.Fatal("ModalAdapter should delegate OnMouseButton to inner widget")
	}
}

func TestModalAdapter_DelegatesMouseMove(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(0, 0, 100, 40))

	m := widget.NewModalAdapter(btn)
	m.SetBounds(image.Rect(0, 0, 100, 40))

	m.OnMouseMove(50, 20)
	if !btn.IsHovered() {
		t.Fatal("ModalAdapter should delegate OnMouseMove to inner widget")
	}
}

func TestModalAdapter_ImplementsModalWidget(t *testing.T) {
	p := widget.NewPanel(color.RGBA{A: 255})
	m := widget.NewModalAdapter(p)
	// Проверяем, что ModalAdapter реализует ModalWidget
	var _ widget.ModalWidget = m
}
