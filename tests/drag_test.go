package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// ─── ShiftWidget ─────────────────────────────────────────────────────────────

func TestShiftWidget_SimpleMove(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(10, 20, 110, 50))

	widget.ShiftWidget(btn, 5, 10)

	b := btn.Bounds()
	if b.Min.X != 15 || b.Min.Y != 30 {
		t.Fatalf("after shift: Min = (%d,%d), want (15,30)", b.Min.X, b.Min.Y)
	}
	if b.Max.X != 115 || b.Max.Y != 60 {
		t.Fatalf("after shift: Max = (%d,%d), want (115,60)", b.Max.X, b.Max.Y)
	}
}

func TestShiftWidget_RecursiveChildren(t *testing.T) {
	panel := widget.NewPanel(widget.DarkTheme().PanelBG)
	panel.SetBounds(image.Rect(0, 0, 300, 200))

	child1 := widget.NewButton("B1")
	child1.SetBounds(image.Rect(10, 10, 110, 40))

	child2 := widget.NewButton("B2")
	child2.SetBounds(image.Rect(10, 50, 110, 80))

	panel.AddChild(child1)
	panel.AddChild(child2)

	widget.ShiftWidget(panel, 20, 30)

	if panel.Bounds().Min.X != 20 || panel.Bounds().Min.Y != 30 {
		t.Fatalf("panel Min = %v, want (20,30)", panel.Bounds().Min)
	}
	if child1.Bounds().Min.X != 30 || child1.Bounds().Min.Y != 40 {
		t.Fatalf("child1 Min = %v, want (30,40)", child1.Bounds().Min)
	}
	if child2.Bounds().Min.X != 30 || child2.Bounds().Min.Y != 80 {
		t.Fatalf("child2 Min = %v, want (30,80)", child2.Bounds().Min)
	}
}

func TestShiftWidget_DeepNesting(t *testing.T) {
	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))

	inner := widget.NewPanel(widget.DarkTheme().PanelBG)
	inner.SetBounds(image.Rect(10, 10, 200, 100))

	deepBtn := widget.NewButton("Deep")
	deepBtn.SetBounds(image.Rect(20, 20, 100, 50))

	inner.AddChild(deepBtn)
	root.AddChild(inner)

	widget.ShiftWidget(root, -5, -5)

	if root.Bounds().Min.X != -5 {
		t.Fatalf("root shifted X = %d, want -5", root.Bounds().Min.X)
	}
	if inner.Bounds().Min.X != 5 {
		t.Fatalf("inner shifted X = %d, want 5", inner.Bounds().Min.X)
	}
	if deepBtn.Bounds().Min.X != 15 {
		t.Fatalf("deepBtn shifted X = %d, want 15", deepBtn.Bounds().Min.X)
	}
}

func TestShiftWidget_ZeroDelta(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(10, 10, 100, 40))
	original := btn.Bounds()

	widget.ShiftWidget(btn, 0, 0)

	if btn.Bounds() != original {
		t.Fatal("zero shift should not change bounds")
	}
}

func TestShiftWidget_NegativeDelta(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(100, 100, 200, 150))

	widget.ShiftWidget(btn, -30, -50)

	b := btn.Bounds()
	if b.Min.X != 70 || b.Min.Y != 50 {
		t.Fatalf("after negative shift: Min = %v, want (70,50)", b.Min)
	}
}

// ─── ShiftWidget с BaseBoundsProvider (Dropdown) ────────────────────────────

func TestShiftWidget_Dropdown_UseBaseBounds(t *testing.T) {
	d := widget.NewDropdown("A", "B", "C")
	d.SetBounds(image.Rect(100, 100, 300, 130))

	// Открываем список — Bounds() расширяются
	d.SetOpen(true)
	openBounds := d.Bounds()
	if openBounds.Max.Y <= 130 {
		t.Fatal("open dropdown should have expanded bounds")
	}

	// ShiftWidget должен использовать BaseBounds(), не расширенные Bounds()
	widget.ShiftWidget(d, 10, 20)

	// После сдвига BaseBounds должны сдвинуться корректно
	newBase := d.BaseBounds()
	if newBase.Min.X != 110 || newBase.Min.Y != 120 {
		t.Fatalf("BaseBounds after shift = %v, want Min(110,120)", newBase.Min)
	}
	if newBase.Max.X != 310 || newBase.Max.Y != 150 {
		t.Fatalf("BaseBounds Max after shift = %v, want Max(310,150)", newBase.Max)
	}
}

func TestShiftWidget_OpenDropdown_SizeNotInflated(t *testing.T) {
	// Проблема которую мы фиксировали: без BaseBoundsProvider каждый ShiftWidget
	// раздувал бы bounds дропдауна из-за открытого списка.
	d := widget.NewDropdown("A", "B", "C")
	d.SetBounds(image.Rect(0, 0, 200, 30))
	d.SetOpen(true)

	// Многократный сдвиг не должен раздувать размер
	for i := 0; i < 5; i++ {
		widget.ShiftWidget(d, 1, 1)
	}

	// BaseBounds должен иметь исходный размер (200×30), только смещённый
	base := d.BaseBounds()
	if base.Dx() != 200 {
		t.Fatalf("after repeated shifts Dx = %d, want 200 (not inflated)", base.Dx())
	}
	if base.Dy() != 30 {
		t.Fatalf("after repeated shifts Dy = %d, want 30 (not inflated)", base.Dy())
	}
}

// ─── DismissAll ──────────────────────────────────────────────────────────────

func TestDismissAll_SingleDropdown(t *testing.T) {
	d := widget.NewDropdown("A", "B")
	d.SetOpen(true)

	widget.DismissAll(d)
	if d.IsOpen() {
		t.Fatal("DismissAll should close the dropdown")
	}
}

func TestDismissAll_ClosedDropdown_NoEffect(t *testing.T) {
	d := widget.NewDropdown("A", "B")
	d.SetOpen(false)

	widget.DismissAll(d)
	if d.IsOpen() {
		t.Fatal("DismissAll should not affect already-closed dropdown")
	}
}

func TestDismissAll_Recursive(t *testing.T) {
	panel := widget.NewPanel(widget.DarkTheme().PanelBG)

	drop1 := widget.NewDropdown("A", "B")
	drop1.SetOpen(true)
	drop2 := widget.NewDropdown("X", "Y")
	drop2.SetOpen(true)

	inner := widget.NewPanel(widget.DarkTheme().PanelBG)
	inner.AddChild(drop2)

	panel.AddChild(drop1)
	panel.AddChild(inner)

	widget.DismissAll(panel)

	if drop1.IsOpen() {
		t.Fatal("drop1 should be dismissed")
	}
	if drop2.IsOpen() {
		t.Fatal("drop2 (nested) should be dismissed")
	}
}

func TestDismissAll_NonDismissable_NoPanic(t *testing.T) {
	// Кнопка не Dismissable — не должна паниковать
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(0, 0, 100, 30))

	inner := widget.NewPanel(widget.DarkTheme().PanelBG)
	inner.AddChild(btn)

	widget.DismissAll(inner) // не должен паниковать
}

// ─── Panel.DismissAll при drag ─────────────────────────────────────────────

func TestPanel_OnMouseButton_DismissesDropdownOnDrag(t *testing.T) {
	panel := widget.NewWin10Panel()
	panel.SetBounds(image.Rect(0, 0, 300, 200))
	panel.Drag.Enabled = true
	panel.Drag.HandleHeight = 30

	drop := widget.NewDropdown("A", "B", "C")
	drop.SetBounds(image.Rect(10, 50, 200, 80))
	drop.SetOpen(true)
	panel.AddChild(drop)

	// Нажатие в drag-zone панели должно закрыть dropdown
	ev := widget.MouseEvent{
		X: 150, Y: 15, // в drag-handle zone
		Button:  widget.MouseLeft,
		Pressed: true,
	}
	panel.OnMouseButton(ev)

	if drop.IsOpen() {
		t.Fatal("DismissAll should close dropdown when panel starts dragging")
	}
}

// ─── Dropdown Dismiss interface ───────────────────────────────────────────

func TestDropdown_Dismiss_ClosesAndResetsHover(t *testing.T) {
	d := widget.NewDropdown("A", "B", "C")
	d.SetBounds(image.Rect(0, 0, 200, 30))
	d.SetOpen(true)

	// Симулируем hover над пунктом списка
	d.OnMouseMove(50, 45) // наводим на область списка

	d.Dismiss()

	if d.IsOpen() {
		t.Fatal("Dismiss should close dropdown")
	}
}

// ─── Dropdown BaseBounds ──────────────────────────────────────────────────

func TestDropdown_BaseBounds_AlwaysReturnsHeaderBounds(t *testing.T) {
	d := widget.NewDropdown("A", "B", "C")
	r := image.Rect(50, 100, 250, 130)
	d.SetBounds(r)

	// Закрытый
	base := d.BaseBounds()
	if base != r {
		t.Fatalf("BaseBounds (closed) = %v, want %v", base, r)
	}

	// Открытый — Bounds() расширяется, BaseBounds() нет
	d.SetOpen(true)
	expanded := d.Bounds()
	base = d.BaseBounds()

	if base != r {
		t.Fatalf("BaseBounds (open) = %v, want %v (not expanded)", base, r)
	}
	if expanded == r {
		t.Fatal("open Bounds() should be larger than base")
	}
	if expanded.Dy() <= r.Dy() {
		t.Fatalf("open Bounds() height = %d, should exceed base height %d", expanded.Dy(), r.Dy())
	}
}

// ─── CollectFocusables с Dropdown ────────────────────────────────────────

func TestCollectFocusables_IncludesDropdown(t *testing.T) {
	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	d := widget.NewDropdown("A", "B")
	d.SetBounds(image.Rect(0, 0, 200, 30))
	root.AddChild(d)

	result := widget.CollectFocusables(root)
	if len(result) != 1 {
		t.Fatalf("expected 1 focusable (dropdown), got %d", len(result))
	}
	if result[0] != widget.Widget(d) {
		t.Fatal("focusable should be the dropdown")
	}
}

func TestCollectFocusables_IncludesAllFocusableTypes(t *testing.T) {
	root := widget.NewPanel(widget.DarkTheme().PanelBG)

	btn := widget.NewButton("B")
	cb := widget.NewCheckBox("C")
	rb := widget.NewRadioButton("R", "grp_cf")
	defer rb.RemoveFromGroup()
	ts := widget.NewToggleSwitch("T")
	drop := widget.NewDropdown("A")
	lv := widget.NewListView("X")
	sl := widget.NewSlider()
	ti := widget.NewTextInput("P")

	root.AddChild(btn)
	root.AddChild(cb)
	root.AddChild(rb)
	root.AddChild(ts)
	root.AddChild(drop)
	root.AddChild(lv)
	root.AddChild(sl)
	root.AddChild(ti)

	result := widget.CollectFocusables(root)
	if len(result) != 8 {
		t.Fatalf("expected 8 focusables, got %d", len(result))
	}
}
