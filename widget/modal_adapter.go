// Package widget — ModalAdapter: обёртка Widget → ModalWidget.
//
// Позволяет любой Widget показывать как модальное окно через Engine.ShowModal.
// Ранее этот тип дублировался в cmd/guiview как xamlModal.
package widget

import "image/color"

// ModalAdapter оборачивает произвольный Widget и реализует ModalWidget,
// делегируя ввод (мышь, клавиатура) внутреннему виджету.
type ModalAdapter struct {
	Widget                     // встроенный виджет (рисование, дети, bounds)
	dimColor color.RGBA        // цвет затемнения фона (по умолчанию {A: 140})
	OnClose  func()            // вызывается при необходимости закрыть модальное окно
}

// NewModalAdapter создаёт ModalAdapter с цветом затемнения по умолчанию.
func NewModalAdapter(w Widget) *ModalAdapter {
	return &ModalAdapter{
		Widget:   w,
		dimColor: color.RGBA{A: 140},
	}
}

// NewModalAdapterWithDim создаёт ModalAdapter с заданным цветом затемнения.
func NewModalAdapterWithDim(w Widget, dim color.RGBA) *ModalAdapter {
	return &ModalAdapter{
		Widget:   w,
		dimColor: dim,
	}
}

// ─── ModalWidget interface ──────────────────────────────────────────────────

// IsModal реализует ModalWidget.
func (m *ModalAdapter) IsModal() bool { return true }

// DimColor реализует ModalWidget.
func (m *ModalAdapter) DimColor() color.RGBA { return m.dimColor }

// ─── CaptureRequester ───────────────────────────────────────────────────────

// WantsCapture делегирует CaptureRequester внутреннего виджета (Window, Panel).
func (m *ModalAdapter) WantsCapture(e MouseEvent) bool {
	if cr, ok := m.Widget.(CaptureRequester); ok {
		return cr.WantsCapture(e)
	}
	return false
}

// ─── CaptureAware ───────────────────────────────────────────────────────────

// SetCaptureManager делегирует CaptureAware внутреннего виджета.
func (m *ModalAdapter) SetCaptureManager(cm CaptureManager) {
	if ca, ok := m.Widget.(CaptureAware); ok {
		ca.SetCaptureManager(cm)
	}
}

// ─── MouseClickHandler ─────────────────────────────────────────────────────

// OnMouseButton делегирует MouseClickHandler внутреннего виджета.
func (m *ModalAdapter) OnMouseButton(e MouseEvent) bool {
	if mc, ok := m.Widget.(MouseClickHandler); ok {
		return mc.OnMouseButton(e)
	}
	return false
}

// ─── MouseMoveHandler ───────────────────────────────────────────────────────

// OnMouseMove делегирует MouseMoveHandler внутреннего виджета.
func (m *ModalAdapter) OnMouseMove(x, y int) {
	if mm, ok := m.Widget.(MouseMoveHandler); ok {
		mm.OnMouseMove(x, y)
	}
}
