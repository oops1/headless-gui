package widget

import (
	"image"
	"testing"
)

// TestOverlayBounds_AllWidgets проверяет контракт OverlayBoundsProvider:
// пустой Rect при закрытом оверлее и непустой при открытом — для всех
// popup-подобных виджетов.
func TestOverlayBounds_AllWidgets(t *testing.T) {
	// ── Dropdown ─────────────────────────────────────────────────────────
	dd := NewDropdown("Alpha", "Beta", "Gamma")
	dd.SetBounds(image.Rect(10, 10, 120, 34))
	if r := dd.OverlayBounds(); !r.Empty() {
		t.Fatalf("Dropdown закрыт: OverlayBounds должен быть пустым, got %v", r)
	}
	dd.SetOpen(true)
	if r := dd.OverlayBounds(); r.Empty() {
		t.Fatal("Dropdown открыт: OverlayBounds не должен быть пустым")
	} else if r.Min.Y != 34 || r.Min.X != 10 || r.Max.X != 120 {
		t.Fatalf("Dropdown OverlayBounds неверный: %v", r)
	}
	dd.SetOpen(false)
	if r := dd.OverlayBounds(); !r.Empty() {
		t.Fatalf("Dropdown снова закрыт: OverlayBounds должен быть пустым, got %v", r)
	}

	// ── PopupMenu ────────────────────────────────────────────────────────
	pm := NewPopupMenu()
	pm.AddItem("One", nil)
	pm.AddItem("Two", nil)
	if r := pm.OverlayBounds(); !r.Empty() {
		t.Fatalf("PopupMenu закрыт: OverlayBounds должен быть пустым, got %v", r)
	}
	pm.Show(40, 40)
	if r := pm.OverlayBounds(); r.Empty() {
		t.Fatal("PopupMenu открыт: OverlayBounds не должен быть пустым")
	}
	pm.Close()
	if r := pm.OverlayBounds(); !r.Empty() {
		t.Fatalf("PopupMenu закрыт: OverlayBounds должен быть пустым, got %v", r)
	}

	// ── MenuBar ──────────────────────────────────────────────────────────
	mb := NewMenuBar()
	mb.SetBounds(image.Rect(0, 0, 300, 28))
	mb.AddMenu("File", MenuItem{Text: "New"}, MenuItem{Text: "Open"})
	if r := mb.OverlayBounds(); !r.Empty() {
		t.Fatalf("MenuBar закрыт: OverlayBounds должен быть пустым, got %v", r)
	}
	mb.openSubmenu(0)
	if r := mb.OverlayBounds(); r.Empty() {
		t.Fatal("MenuBar открыт: OverlayBounds не должен быть пустым")
	}
	mb.closeSubmenu()
	if r := mb.OverlayBounds(); !r.Empty() {
		t.Fatalf("MenuBar закрыт: OverlayBounds должен быть пустым, got %v", r)
	}

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
