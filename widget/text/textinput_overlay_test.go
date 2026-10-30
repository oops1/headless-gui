package text

import (
	"image"
	"testing"
)

// Поля ввода: контекстное меню даёт OverlayBounds, без меню — пусто
// (часть TestOverlayBounds_AllWidgets, переехавшая с полями в widget/text).
func TestOverlayBounds_TextFields(t *testing.T) {
	// ── TextInput контекстное меню ───────────────────────────────────────
	ti := NewTextInput("")
	ti.SetBounds(image.Rect(0, 0, 120, 26))
	ti.SetText("hello")
	if r := ti.OverlayBounds(); !r.Empty() {
		t.Fatalf("TextInput без меню: OverlayBounds должен быть пустым, got %v", r)
	}
	ti.showContextMenu(10, 26)
	if r := ti.OverlayBounds(); r.Empty() {
		t.Fatal("TextInput с меню: OverlayBounds не должен быть пустым")
	}

	// ── TextBox контекстное меню ─────────────────────────────────────────
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 200, 120))
	tb.SetText("hello world")
	if r := tb.OverlayBounds(); !r.Empty() {
		t.Fatalf("TextBox без меню: OverlayBounds должен быть пустым, got %v", r)
	}
	tb.showContextMenu(10, 30)
	if r := tb.OverlayBounds(); r.Empty() {
		t.Fatal("TextBox с меню: OverlayBounds не должен быть пустым")
	}
}
