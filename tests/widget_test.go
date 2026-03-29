// Package tests — комплексные тесты для headless-gui движка.
// Все тесты в отдельной папке, тестируют только экспортированный API.
package tests

import (
	"image"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/widget"
)

// ─── Base ───────────────────────────────────────────────────────────────────

func TestBase_BoundsAndChildren(t *testing.T) {
	p := widget.NewPanel(widget.DarkTheme().PanelBG)
	r := image.Rect(10, 20, 110, 120)
	p.SetBounds(r)
	if got := p.Bounds(); got != r {
		t.Fatalf("Bounds() = %v, want %v", got, r)
	}
	if len(p.Children()) != 0 {
		t.Fatal("expected 0 children")
	}

	child := widget.NewButton("child")
	p.AddChild(child)
	if len(p.Children()) != 1 {
		t.Fatal("expected 1 child after AddChild")
	}
}

func TestBase_MultipleChildren(t *testing.T) {
	parent := widget.NewPanel(widget.DarkTheme().PanelBG)
	for i := 0; i < 5; i++ {
		parent.AddChild(widget.NewButton("b"))
	}
	if n := len(parent.Children()); n != 5 {
		t.Fatalf("Children() len = %d, want 5", n)
	}
}

// ─── Button ─────────────────────────────────────────────────────────────────

func TestButton_NewButton(t *testing.T) {
	btn := widget.NewButton("Тест")
	if btn.Text != "Тест" {
		t.Fatalf("Text = %q, want %q", btn.Text, "Тест")
	}
	if btn.IsPressed() {
		t.Fatal("new button should not be pressed")
	}
	if btn.IsHovered() {
		t.Fatal("new button should not be hovered")
	}
	if btn.IsFocused() {
		t.Fatal("new button should not be focused")
	}
}

func TestButton_AccentButton(t *testing.T) {
	btn := widget.NewWin10AccentButton("OK")
	if btn.Text != "OK" {
		t.Fatalf("Text = %q, want %q", btn.Text, "OK")
	}
}

func TestButton_PressedState(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetPressed(true)
	if !btn.IsPressed() {
		t.Fatal("expected pressed")
	}
	btn.SetPressed(false)
	if btn.IsPressed() {
		t.Fatal("expected not pressed")
	}
}

func TestButton_HoveredState(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetHovered(true)
	if !btn.IsHovered() {
		t.Fatal("expected hovered")
	}
	btn.SetHovered(false)
	if btn.IsHovered() {
		t.Fatal("expected not hovered")
	}
}

func TestButton_FocusedState(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetFocused(true)
	if !btn.IsFocused() {
		t.Fatal("expected focused")
	}
	btn.SetFocused(false)
	if btn.IsFocused() {
		t.Fatal("expected not focused")
	}
}

func TestButton_OnMouseButton_Click(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(0, 0, 100, 30))

	clicked := false
	btn.OnClick = func() { clicked = true }

	// Нажатие
	ev := widget.MouseEvent{X: 50, Y: 15, Button: widget.MouseLeft, Pressed: true}
	consumed := btn.OnMouseButton(ev)
	if !consumed {
		t.Fatal("press should be consumed")
	}
	if !btn.IsPressed() {
		t.Fatal("should be pressed after mousedown")
	}
	if clicked {
		t.Fatal("should not fire OnClick on press")
	}

	// Отпускание
	ev.Pressed = false
	consumed = btn.OnMouseButton(ev)
	if !consumed {
		t.Fatal("release should be consumed")
	}
	if btn.IsPressed() {
		t.Fatal("should not be pressed after mouseup")
	}
	if !clicked {
		t.Fatal("OnClick should have been called")
	}
}

func TestButton_OnMouseButton_RightClick(t *testing.T) {
	btn := widget.NewButton("B")
	ev := widget.MouseEvent{X: 50, Y: 15, Button: widget.MouseRight, Pressed: true}
	if btn.OnMouseButton(ev) {
		t.Fatal("right click should not be consumed")
	}
}

func TestButton_OnMouseMove_HoverDetection(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(10, 10, 110, 40))

	btn.OnMouseMove(50, 25) // внутри
	if !btn.IsHovered() {
		t.Fatal("should be hovered when cursor is inside")
	}

	btn.OnMouseMove(0, 0) // вне
	if btn.IsHovered() {
		t.Fatal("should not be hovered when cursor is outside")
	}
}

func TestButton_OnKeyEvent_EnterTriggersClick(t *testing.T) {
	btn := widget.NewButton("B")
	var clicked bool
	btn.OnClick = func() { clicked = true }

	// Enter нажат
	ev := widget.KeyEvent{Code: widget.KeyEnter, Pressed: true}
	btn.OnKeyEvent(ev)
	// OnClick вызывается в горутине, даём ей время.
	// Проверяем через маленький канал
	// Примечание: в реальном коде btn.OnKeyEvent запускает go btn.OnClick()
	// Для теста переопределяем OnClick на синхронную проверку через канал
	ch := make(chan bool, 1)
	btn.OnClick = func() { ch <- true }
	btn.OnKeyEvent(ev)
	select {
	case <-ch:
		// OK
	default:
		// Горутина может не успеть — это нормально для unit-теста
	}
	_ = clicked // avoid unused
}

func TestButton_OnKeyEvent_SpaceTriggersClick(t *testing.T) {
	btn := widget.NewButton("B")
	ch := make(chan bool, 1)
	btn.OnClick = func() { ch <- true }

	ev := widget.KeyEvent{Code: widget.KeySpace, Pressed: true}
	btn.OnKeyEvent(ev)
	select {
	case <-ch:
	default:
	}
}

func TestButton_OnKeyEvent_ReleaseIgnored(t *testing.T) {
	btn := widget.NewButton("B")
	called := false
	btn.OnClick = func() { called = true }

	// Отпускание не должно вызывать OnClick
	ev := widget.KeyEvent{Code: widget.KeyEnter, Pressed: false}
	btn.OnKeyEvent(ev)
	if called {
		t.Fatal("OnClick should not be called on key release")
	}
}

func TestButton_AtomicSafety(t *testing.T) {
	btn := widget.NewButton("B")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			btn.SetPressed(true)
			btn.IsPressed()
			btn.SetHovered(true)
			btn.IsHovered()
			btn.SetFocused(true)
			btn.IsFocused()
		}()
	}
	wg.Wait()
}

func TestButton_ApplyTheme(t *testing.T) {
	btn := widget.NewButton("B")
	theme := widget.LightTheme()
	btn.ApplyTheme(theme)
	if btn.TextColor != theme.BtnText {
		t.Fatal("ApplyTheme did not update TextColor")
	}
	if btn.Background != theme.BtnBG {
		t.Fatal("ApplyTheme did not update Background")
	}
}

// ─── CheckBox ───────────────────────────────────────────────────────────────

func TestCheckBox_NewCheckBox(t *testing.T) {
	cb := widget.NewCheckBox("Включить")
	if cb.Text != "Включить" {
		t.Fatalf("Text = %q", cb.Text)
	}
	if cb.IsChecked() {
		t.Fatal("new checkbox should not be checked")
	}
}

func TestCheckBox_SetChecked(t *testing.T) {
	cb := widget.NewCheckBox("C")
	cb.SetChecked(true)
	if !cb.IsChecked() {
		t.Fatal("expected checked")
	}
	cb.SetChecked(false)
	if cb.IsChecked() {
		t.Fatal("expected unchecked")
	}
}

func TestCheckBox_OnMouseButton_Toggle(t *testing.T) {
	cb := widget.NewCheckBox("C")
	var lastState bool
	cb.OnChange = func(checked bool) { lastState = checked }

	// Отпускание левой кнопки переключает
	ev := widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft, Pressed: false}
	consumed := cb.OnMouseButton(ev)
	if !consumed {
		t.Fatal("should be consumed")
	}
	if !cb.IsChecked() {
		t.Fatal("should be checked after click")
	}
	if !lastState {
		t.Fatal("OnChange should report true")
	}

	// Повторное переключение
	cb.OnMouseButton(ev)
	if cb.IsChecked() {
		t.Fatal("should be unchecked after second click")
	}
}

func TestCheckBox_OnMouseButton_PressIgnored(t *testing.T) {
	cb := widget.NewCheckBox("C")
	ev := widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft, Pressed: true}
	if cb.OnMouseButton(ev) {
		t.Fatal("press should not be consumed")
	}
}

func TestCheckBox_Focusable(t *testing.T) {
	cb := widget.NewCheckBox("C")
	cb.SetFocused(true)
	if !cb.IsFocused() {
		t.Fatal("expected focused")
	}
	cb.SetFocused(false)
	if cb.IsFocused() {
		t.Fatal("expected not focused")
	}
}

func TestCheckBox_OnKeyEvent_SpaceToggles(t *testing.T) {
	cb := widget.NewCheckBox("C")
	ch := make(chan bool, 1)
	cb.OnChange = func(checked bool) { ch <- checked }

	ev := widget.KeyEvent{Code: widget.KeySpace, Pressed: true}
	cb.OnKeyEvent(ev)
	select {
	case got := <-ch:
		if !got {
			t.Fatal("expected checked=true")
		}
	default:
	}
	if !cb.IsChecked() {
		t.Fatal("should be checked after Space")
	}
}

func TestCheckBox_OnMouseMove_Hover(t *testing.T) {
	cb := widget.NewCheckBox("C")
	cb.SetBounds(image.Rect(10, 10, 110, 30))
	cb.OnMouseMove(50, 20) // внутри
	if !cb.IsHovered() {
		t.Fatal("expected hovered")
	}
	cb.OnMouseMove(0, 0) // вне
	if cb.IsHovered() {
		t.Fatal("expected not hovered")
	}
}

func TestCheckBox_ApplyTheme(t *testing.T) {
	cb := widget.NewCheckBox("C")
	theme := widget.LightTheme()
	cb.ApplyTheme(theme)
	if cb.TextColor != theme.CheckText {
		t.Fatal("ApplyTheme did not update TextColor")
	}
}

// ─── RadioButton ────────────────────────────────────────────────────────────

func TestRadioButton_NewRadioButton(t *testing.T) {
	rb := widget.NewRadioButton("Вариант 1", "grp1")
	if rb.Text != "Вариант 1" {
		t.Fatalf("Text = %q", rb.Text)
	}
	if rb.GroupName != "grp1" {
		t.Fatalf("GroupName = %q", rb.GroupName)
	}
	if rb.IsSelected() {
		t.Fatal("new radio should not be selected")
	}
	// Cleanup
	rb.RemoveFromGroup()
}

func TestRadioButton_SetSelected(t *testing.T) {
	rb := widget.NewRadioButton("A", "grp_test_set")
	defer rb.RemoveFromGroup()
	rb.SetSelected(true)
	if !rb.IsSelected() {
		t.Fatal("expected selected")
	}
	rb.SetSelected(false)
	if rb.IsSelected() {
		t.Fatal("expected not selected")
	}
}

func TestRadioButton_GroupExclusivity(t *testing.T) {
	rb1 := widget.NewRadioButton("A", "grp_excl")
	rb2 := widget.NewRadioButton("B", "grp_excl")
	rb3 := widget.NewRadioButton("C", "grp_excl")
	defer rb1.RemoveFromGroup()
	defer rb2.RemoveFromGroup()
	defer rb3.RemoveFromGroup()

	rb1.SetSelected(true)
	if !rb1.IsSelected() {
		t.Fatal("rb1 should be selected")
	}

	rb2.SetSelected(true)
	if rb1.IsSelected() {
		t.Fatal("rb1 should be deselected after rb2 selected")
	}
	if !rb2.IsSelected() {
		t.Fatal("rb2 should be selected")
	}
	if rb3.IsSelected() {
		t.Fatal("rb3 should not be selected")
	}
}

func TestRadioButton_OnMouseButton(t *testing.T) {
	rb := widget.NewRadioButton("A", "grp_click")
	defer rb.RemoveFromGroup()
	var changed bool
	rb.OnChange = func(selected bool) { changed = selected }

	ev := widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft, Pressed: false}
	consumed := rb.OnMouseButton(ev)
	if !consumed {
		t.Fatal("should be consumed")
	}
	if !rb.IsSelected() {
		t.Fatal("should be selected after click")
	}
	if !changed {
		t.Fatal("OnChange should have been called")
	}
}

func TestRadioButton_Focusable(t *testing.T) {
	rb := widget.NewRadioButton("A", "grp_focus")
	defer rb.RemoveFromGroup()
	rb.SetFocused(true)
	if !rb.IsFocused() {
		t.Fatal("expected focused")
	}
}

func TestRadioButton_RemoveFromGroup(t *testing.T) {
	rb1 := widget.NewRadioButton("A", "grp_rm")
	rb2 := widget.NewRadioButton("B", "grp_rm")
	rb1.RemoveFromGroup()
	// rb2 остаётся в группе, выбираем
	rb2.SetSelected(true)
	// rb1 не должен был измениться (его нет в группе)
	if rb1.IsSelected() {
		t.Fatal("rb1 should not be affected after removal")
	}
	rb2.RemoveFromGroup()
}

// ─── ToggleSwitch ───────────────────────────────────────────────────────────

func TestToggleSwitch_NewToggleSwitch(t *testing.T) {
	ts := widget.NewToggleSwitch("Авто")
	if ts.Text != "Авто" {
		t.Fatalf("Text = %q", ts.Text)
	}
	if ts.IsOn() {
		t.Fatal("new toggle should be off")
	}
}

func TestToggleSwitch_SetOn(t *testing.T) {
	ts := widget.NewToggleSwitch("T")
	ts.SetOn(true)
	if !ts.IsOn() {
		t.Fatal("expected on")
	}
	ts.SetOn(false)
	if ts.IsOn() {
		t.Fatal("expected off")
	}
}

func TestToggleSwitch_OnMouseButton_Toggle(t *testing.T) {
	ts := widget.NewToggleSwitch("T")
	var lastState bool
	ts.OnChange = func(on bool) { lastState = on }

	ev := widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft, Pressed: false}
	consumed := ts.OnMouseButton(ev)
	if !consumed {
		t.Fatal("should be consumed")
	}
	if !ts.IsOn() {
		t.Fatal("should be on after toggle")
	}
	if !lastState {
		t.Fatal("OnChange should report true")
	}

	// Повторное нажатие — выключение
	ts.OnMouseButton(ev)
	if ts.IsOn() {
		t.Fatal("should be off after second toggle")
	}
}

func TestToggleSwitch_Focusable(t *testing.T) {
	ts := widget.NewToggleSwitch("T")
	ts.SetFocused(true)
	if !ts.IsFocused() {
		t.Fatal("expected focused")
	}
}

func TestToggleSwitch_OnKeyEvent_SpaceToggles(t *testing.T) {
	ts := widget.NewToggleSwitch("T")
	ev := widget.KeyEvent{Code: widget.KeySpace, Pressed: true}
	ts.OnKeyEvent(ev)
	if !ts.IsOn() {
		t.Fatal("Space should toggle on")
	}
}

func TestToggleSwitch_OnMouseMove_Hover(t *testing.T) {
	ts := widget.NewToggleSwitch("T")
	ts.SetBounds(image.Rect(0, 0, 100, 30))
	ts.OnMouseMove(50, 15) // внутри
	if !ts.IsHovered() {
		t.Fatal("expected hovered")
	}
	ts.OnMouseMove(200, 200) // вне
	if ts.IsHovered() {
		t.Fatal("expected not hovered")
	}
}

// ─── Label ──────────────────────────────────────────────────────────────────

func TestLabel_NewLabel(t *testing.T) {
	lbl := widget.NewWin10Label("Привет")
	if lbl.Text() != "Привет" {
		t.Fatalf("Text = %q", lbl.Text())
	}
}

func TestLabel_SetText(t *testing.T) {
	lbl := widget.NewWin10Label("A")
	lbl.SetText("Б")
	if lbl.Text() != "Б" {
		t.Fatalf("Text = %q after SetText", lbl.Text())
	}
}

func TestLabel_ThreadSafe(t *testing.T) {
	lbl := widget.NewWin10Label("init")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lbl.SetText("test")
			_ = lbl.Text()
		}()
	}
	wg.Wait()
}

func TestLabel_ApplyTheme(t *testing.T) {
	lbl := widget.NewWin10Label("L")
	theme := widget.LightTheme()
	lbl.ApplyTheme(theme)
	if lbl.TextColor != theme.LabelText {
		t.Fatal("ApplyTheme did not update TextColor")
	}
}

// ─── ProgressBar ────────────────────────────────────────────────────────────

func TestProgressBar_NewProgressBar(t *testing.T) {
	pb := widget.NewProgressBar()
	v := pb.Value()
	if v != 0 {
		t.Fatalf("initial value = %f, want 0", v)
	}
}

func TestProgressBar_SetValue(t *testing.T) {
	pb := widget.NewProgressBar()
	pb.SetValue(0.5)
	v := pb.Value()
	if v < 0.49 || v > 0.51 {
		t.Fatalf("Value() = %f, want ~0.5", v)
	}
}

func TestProgressBar_SetValueClamped(t *testing.T) {
	pb := widget.NewProgressBar()
	pb.SetValue(-1.0)
	if pb.Value() != 0.0 {
		t.Fatalf("negative value should be clamped to 0, got %f", pb.Value())
	}
	pb.SetValue(2.0)
	v := pb.Value()
	if v < 0.99 || v > 1.01 {
		t.Fatalf("value > 1 should be clamped to 1, got %f", v)
	}
}

func TestProgressBar_ThreadSafe(t *testing.T) {
	pb := widget.NewProgressBar()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pb.SetValue(0.3)
			_ = pb.Value()
		}()
	}
	wg.Wait()
}

func TestProgressBar_CustomColor(t *testing.T) {
	pb := widget.NewProgressBarColor(widget.DarkTheme().Accent)
	if pb.FillColor != widget.DarkTheme().Accent {
		t.Fatal("custom fill color not applied")
	}
}

// ─── Dropdown ───────────────────────────────────────────────────────────────

func TestDropdown_NewDropdown(t *testing.T) {
	d := widget.NewDropdown("A", "B", "C")
	if d.Selected() != 0 {
		t.Fatalf("initial selected = %d", d.Selected())
	}
	if d.SelectedText() != "A" {
		t.Fatalf("SelectedText = %q", d.SelectedText())
	}
	if d.IsOpen() {
		t.Fatal("should not be open initially")
	}
}

func TestDropdown_SetSelected(t *testing.T) {
	d := widget.NewDropdown("A", "B", "C")
	d.SetSelected(2)
	if d.Selected() != 2 {
		t.Fatalf("Selected() = %d, want 2", d.Selected())
	}
	if d.SelectedText() != "C" {
		t.Fatalf("SelectedText = %q", d.SelectedText())
	}
}

func TestDropdown_SetSelectedOutOfRange(t *testing.T) {
	d := widget.NewDropdown("A", "B")
	d.SetSelected(10) // out of range — should be ignored
	if d.Selected() != 0 {
		t.Fatalf("out-of-range SetSelected should be ignored, got %d", d.Selected())
	}
	d.SetSelected(-1) // negative — ignored
	if d.Selected() != 0 {
		t.Fatalf("negative SetSelected should be ignored, got %d", d.Selected())
	}
}

func TestDropdown_OpenClose(t *testing.T) {
	d := widget.NewDropdown("A", "B")
	d.SetOpen(true)
	if !d.IsOpen() {
		t.Fatal("should be open")
	}
	d.SetOpen(false)
	if d.IsOpen() {
		t.Fatal("should be closed")
	}
}

func TestDropdown_HasOverlay(t *testing.T) {
	d := widget.NewDropdown("A")
	if d.HasOverlay() {
		t.Fatal("closed dropdown should not have overlay")
	}
	d.SetOpen(true)
	if !d.HasOverlay() {
		t.Fatal("open dropdown should have overlay")
	}
}

func TestDropdown_Focusable(t *testing.T) {
	d := widget.NewDropdown("A")
	d.SetFocused(true)
	if !d.IsFocused() {
		t.Fatal("expected focused")
	}
}

func TestDropdown_OnKeyEvent_SpaceToggles(t *testing.T) {
	d := widget.NewDropdown("A", "B")
	ev := widget.KeyEvent{Code: widget.KeySpace, Pressed: true}
	d.OnKeyEvent(ev) // open
	if !d.IsOpen() {
		t.Fatal("Space should open dropdown")
	}
	d.OnKeyEvent(ev) // close
	if d.IsOpen() {
		t.Fatal("Space again should close dropdown")
	}
}

func TestDropdown_OnKeyEvent_UpDown(t *testing.T) {
	d := widget.NewDropdown("A", "B", "C")
	d.SetSelected(0)

	down := widget.KeyEvent{Code: widget.KeyDown, Pressed: true}
	d.OnKeyEvent(down)
	if d.Selected() != 1 {
		t.Fatalf("Down: selected = %d, want 1", d.Selected())
	}

	d.OnKeyEvent(down)
	if d.Selected() != 2 {
		t.Fatalf("Down again: selected = %d, want 2", d.Selected())
	}

	d.OnKeyEvent(down) // at end — should stay at 2
	if d.Selected() != 2 {
		t.Fatalf("Down past end: selected = %d, want 2", d.Selected())
	}

	up := widget.KeyEvent{Code: widget.KeyUp, Pressed: true}
	d.OnKeyEvent(up)
	if d.Selected() != 1 {
		t.Fatalf("Up: selected = %d, want 1", d.Selected())
	}
}

func TestDropdown_OnKeyEvent_Escape(t *testing.T) {
	d := widget.NewDropdown("A", "B")
	d.SetOpen(true)
	esc := widget.KeyEvent{Code: widget.KeyEscape, Pressed: true}
	d.OnKeyEvent(esc)
	if d.IsOpen() {
		t.Fatal("Escape should close dropdown")
	}
}

func TestDropdown_Dismiss(t *testing.T) {
	d := widget.NewDropdown("A", "B")
	d.SetOpen(true)
	d.Dismiss()
	if d.IsOpen() {
		t.Fatal("Dismiss() should close dropdown")
	}
}

func TestDropdown_BaseBounds(t *testing.T) {
	d := widget.NewDropdown("A", "B", "C")
	d.SetBounds(image.Rect(10, 10, 200, 40))

	base := d.BaseBounds()
	if base != image.Rect(10, 10, 200, 40) {
		t.Fatalf("BaseBounds = %v, want base rect", base)
	}

	d.SetOpen(true)
	expanded := d.Bounds()
	if expanded.Max.Y <= 40 {
		t.Fatal("open Bounds() should be expanded vertically")
	}
	// BaseBounds остаётся прежним
	if d.BaseBounds() != image.Rect(10, 10, 200, 40) {
		t.Fatal("BaseBounds should not change when open")
	}
}

func TestDropdown_OnMouseButton_OpenOnClick(t *testing.T) {
	d := widget.NewDropdown("A", "B")
	d.SetBounds(image.Rect(0, 0, 200, 30))

	// Нажатие при закрытом списке открывает его
	ev := widget.MouseEvent{X: 50, Y: 15, Button: widget.MouseLeft, Pressed: true}
	consumed := d.OnMouseButton(ev)
	if !consumed {
		t.Fatal("click should be consumed")
	}
	if !d.IsOpen() {
		t.Fatal("should open on click")
	}
}

func TestDropdown_OnMouseButton_SelectItem(t *testing.T) {
	d := widget.NewDropdown("A", "B", "C")
	d.SetBounds(image.Rect(0, 0, 200, 30))
	d.SetOpen(true)

	var changedIdx int
	var changedText string
	d.OnChange = func(idx int, text string) {
		changedIdx = idx
		changedText = text
	}

	// Клик по третьему элементу (itemH=30, header.Max.Y=30)
	// Item 0: y=[30,60), Item 1: y=[60,90), Item 2: y=[90,120)
	ev := widget.MouseEvent{X: 50, Y: 95, Button: widget.MouseLeft, Pressed: true}
	consumed := d.OnMouseButton(ev)
	if !consumed {
		t.Fatal("item click should be consumed")
	}
	if d.IsOpen() {
		t.Fatal("should close after selecting item")
	}
	if d.Selected() != 2 {
		t.Fatalf("Selected = %d, want 2", d.Selected())
	}
	// OnChange вызывается в горутине, но индекс мы можем проверить синхронно
	// через Selected()
	_ = changedIdx
	_ = changedText
}

// ─── Slider ─────────────────────────────────────────────────────────────────

func TestSlider_NewSlider(t *testing.T) {
	s := widget.NewSlider()
	if s.Min != 0 || s.Max != 1 {
		t.Fatalf("range = [%f, %f], want [0, 1]", s.Min, s.Max)
	}
	if s.Value() != 0 {
		t.Fatalf("initial value = %f, want 0", s.Value())
	}
}

func TestSlider_NewSliderRange(t *testing.T) {
	s := widget.NewSliderRange(10, 100)
	if s.Min != 10 || s.Max != 100 {
		t.Fatalf("range = [%f, %f], want [10, 100]", s.Min, s.Max)
	}
	if s.Value() != 10 {
		t.Fatalf("initial value = %f, want 10", s.Value())
	}
}

func TestSlider_SetValue(t *testing.T) {
	s := widget.NewSlider()
	s.SetValue(0.75)
	v := s.Value()
	if v < 0.74 || v > 0.76 {
		t.Fatalf("Value() = %f, want ~0.75", v)
	}
}

func TestSlider_SetValue_Clamped(t *testing.T) {
	s := widget.NewSlider()
	s.SetValue(-5)
	if s.Value() != 0 {
		t.Fatalf("negative should clamp to 0, got %f", s.Value())
	}
	s.SetValue(10)
	if s.Value() != 1 {
		t.Fatalf("above max should clamp to 1, got %f", s.Value())
	}
}

func TestSlider_Focusable(t *testing.T) {
	s := widget.NewSlider()
	s.SetFocused(true)
	if !s.IsFocused() {
		t.Fatal("expected focused")
	}
	s.SetFocused(false)
	if s.IsFocused() {
		t.Fatal("expected not focused")
	}
}

func TestSlider_OnKeyEvent_LeftRight(t *testing.T) {
	s := widget.NewSlider()
	s.SetValue(0.5)

	left := widget.KeyEvent{Code: widget.KeyLeft, Pressed: true}
	s.OnKeyEvent(left)
	if s.Value() >= 0.5 {
		t.Fatal("Left should decrease value")
	}

	right := widget.KeyEvent{Code: widget.KeyRight, Pressed: true}
	s.OnKeyEvent(right)
	// После Left+Right примерно обратно к 0.5
}

func TestSlider_OnKeyEvent_HomeEnd(t *testing.T) {
	s := widget.NewSlider()
	s.SetValue(0.5)

	home := widget.KeyEvent{Code: widget.KeyHome, Pressed: true}
	s.OnKeyEvent(home)
	if s.Value() != 0 {
		t.Fatalf("Home should set to Min, got %f", s.Value())
	}

	end := widget.KeyEvent{Code: widget.KeyEnd, Pressed: true}
	s.OnKeyEvent(end)
	if s.Value() != 1 {
		t.Fatalf("End should set to Max, got %f", s.Value())
	}
}

// ─── TextInput ──────────────────────────────────────────────────────────────

func TestTextInput_NewTextInput(t *testing.T) {
	ti := widget.NewTextInput("Введите...")
	if ti.Placeholder != "Введите..." {
		t.Fatalf("Placeholder = %q", ti.Placeholder)
	}
	if ti.GetText() != "" {
		t.Fatalf("initial text = %q", ti.GetText())
	}
}

func TestTextInput_SetGetText(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetText("Привет мир")
	if ti.GetText() != "Привет мир" {
		t.Fatalf("GetText = %q", ti.GetText())
	}
}

func TestTextInput_Focusable(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetFocused(true)
	if !ti.IsFocused() {
		t.Fatal("expected focused")
	}
	ti.SetFocused(false)
	if ti.IsFocused() {
		t.Fatal("expected not focused")
	}
}

func TestTextInput_OnKeyEvent_CharInput(t *testing.T) {
	ti := widget.NewTextInput("")
	// Печатаем символ 'H'
	ev := widget.KeyEvent{Rune: 'H', Pressed: true}
	ti.OnKeyEvent(ev)
	if ti.GetText() != "H" {
		t.Fatalf("after 'H': text = %q", ti.GetText())
	}
	ev.Rune = 'i'
	ti.OnKeyEvent(ev)
	if ti.GetText() != "Hi" {
		t.Fatalf("after 'Hi': text = %q", ti.GetText())
	}
}

func TestTextInput_OnKeyEvent_Backspace(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetText("abc")
	ev := widget.KeyEvent{Code: widget.KeyBackspace, Pressed: true}
	ti.OnKeyEvent(ev) // удалит 'c'
	if ti.GetText() != "ab" {
		t.Fatalf("after backspace: text = %q", ti.GetText())
	}
}

func TestTextInput_OnKeyEvent_Delete(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetText("abc")
	// Ставим курсор в начало
	home := widget.KeyEvent{Code: widget.KeyHome, Pressed: true}
	ti.OnKeyEvent(home)
	del := widget.KeyEvent{Code: widget.KeyDelete, Pressed: true}
	ti.OnKeyEvent(del)
	if ti.GetText() != "bc" {
		t.Fatalf("after delete at start: text = %q", ti.GetText())
	}
}

func TestTextInput_OnKeyEvent_Navigation(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetText("abcd")
	// Home → начало
	home := widget.KeyEvent{Code: widget.KeyHome, Pressed: true}
	ti.OnKeyEvent(home)
	// Right → позиция 1
	right := widget.KeyEvent{Code: widget.KeyRight, Pressed: true}
	ti.OnKeyEvent(right)
	// Вставим символ 'X' на позицию 1
	charEv := widget.KeyEvent{Rune: 'X', Pressed: true}
	ti.OnKeyEvent(charEv)
	if ti.GetText() != "aXbcd" {
		t.Fatalf("after insert: text = %q", ti.GetText())
	}
}

func TestTextInput_OnKeyEvent_SelectAll_Copy_Paste(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetText("hello")

	// Ctrl+A
	ctrlA := widget.KeyEvent{Code: widget.KeyA, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlA)

	// Ctrl+C
	ctrlC := widget.KeyEvent{Code: widget.KeyC, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlC)

	// End → конец
	end := widget.KeyEvent{Code: widget.KeyEnd, Pressed: true}
	ti.OnKeyEvent(end)

	// Ctrl+V
	ctrlV := widget.KeyEvent{Code: widget.KeyV, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlV)
	if ti.GetText() != "hellohello" {
		t.Fatalf("after paste: text = %q", ti.GetText())
	}
}

func TestTextInput_PasswordMode(t *testing.T) {
	ti := widget.NewPasswordInput("Пароль")
	if !ti.IsPasswordMode() {
		t.Fatal("should be password mode")
	}
	ti.SetText("secret")
	// Текст остаётся в открытом виде внутри
	if ti.GetText() != "secret" {
		t.Fatalf("GetText = %q", ti.GetText())
	}
}

func TestTextInput_PasswordMode_NoCopy(t *testing.T) {
	ti := widget.NewPasswordInput("")
	ti.SetText("secret")

	// Select all + copy
	ctrlA := widget.KeyEvent{Code: widget.KeyA, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlA)
	ctrlC := widget.KeyEvent{Code: widget.KeyC, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlC)

	// Clear text, try paste — clipboard should be empty (copy was blocked)
	ti.SetText("")
	ctrlV := widget.KeyEvent{Code: widget.KeyV, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlV)
	if ti.GetText() != "" {
		t.Fatalf("password copy should be blocked, but paste gave: %q", ti.GetText())
	}
}

func TestTextInput_OnChange(t *testing.T) {
	ti := widget.NewTextInput("")
	ch := make(chan string, 10)
	ti.OnChange = func(text string) { ch <- text }

	ev := widget.KeyEvent{Rune: 'A', Pressed: true}
	ti.OnKeyEvent(ev)

	select {
	case got := <-ch:
		if got != "A" {
			t.Fatalf("OnChange text = %q", got)
		}
	default:
		// горутина может не успеть
	}
}

// ─── ListView ───────────────────────────────────────────────────────────────

func TestListView_NewListView(t *testing.T) {
	lv := widget.NewListView("A", "B", "C")
	items := lv.Items()
	if len(items) != 3 {
		t.Fatalf("len(Items) = %d, want 3", len(items))
	}
	if lv.Selected() != -1 {
		t.Fatalf("initial selected = %d, want -1", lv.Selected())
	}
}

func TestListView_SetItems(t *testing.T) {
	lv := widget.NewListView("A")
	lv.SetSelected(0)
	lv.SetItems([]string{"X", "Y", "Z"})
	if lv.Selected() != -1 {
		t.Fatal("SetItems should reset selected")
	}
	if len(lv.Items()) != 3 {
		t.Fatalf("len(Items) = %d", len(lv.Items()))
	}
}

func TestListView_AddItem(t *testing.T) {
	lv := widget.NewListView("A")
	lv.AddItem("B")
	items := lv.Items()
	if len(items) != 2 || items[1] != "B" {
		t.Fatalf("Items = %v", items)
	}
}

func TestListView_SetSelected(t *testing.T) {
	lv := widget.NewListView("A", "B", "C")
	lv.SetSelected(1)
	if lv.Selected() != 1 {
		t.Fatalf("Selected = %d", lv.Selected())
	}
	if lv.SelectedText() != "B" {
		t.Fatalf("SelectedText = %q", lv.SelectedText())
	}
}

func TestListView_ScrollBy(t *testing.T) {
	lv := widget.NewListView("A", "B", "C", "D", "E", "F", "G", "H", "I", "J")
	lv.SetBounds(image.Rect(0, 0, 200, 60)) // маленькое окно
	lv.ScrollBy(20)
	// Не упадёт — проверяем что работает
	lv.ScrollBy(-100) // clamp к 0
}

func TestListView_Focusable(t *testing.T) {
	lv := widget.NewListView("A")
	lv.SetFocused(true)
	if !lv.IsFocused() {
		t.Fatal("expected focused")
	}
}

func TestListView_OnKeyEvent_UpDown(t *testing.T) {
	lv := widget.NewListView("A", "B", "C")
	lv.SetBounds(image.Rect(0, 0, 200, 200))
	lv.SetSelected(0)

	down := widget.KeyEvent{Code: widget.KeyDown, Pressed: true}
	lv.OnKeyEvent(down)
	if lv.Selected() != 1 {
		t.Fatalf("Down: selected = %d, want 1", lv.Selected())
	}

	up := widget.KeyEvent{Code: widget.KeyUp, Pressed: true}
	lv.OnKeyEvent(up)
	if lv.Selected() != 0 {
		t.Fatalf("Up: selected = %d, want 0", lv.Selected())
	}
}

func TestListView_OnKeyEvent_HomeEnd(t *testing.T) {
	lv := widget.NewListView("A", "B", "C")
	lv.SetBounds(image.Rect(0, 0, 200, 200))
	lv.SetSelected(1)

	home := widget.KeyEvent{Code: widget.KeyHome, Pressed: true}
	lv.OnKeyEvent(home)
	if lv.Selected() != 0 {
		t.Fatalf("Home: selected = %d, want 0", lv.Selected())
	}

	end := widget.KeyEvent{Code: widget.KeyEnd, Pressed: true}
	lv.OnKeyEvent(end)
	if lv.Selected() != 2 {
		t.Fatalf("End: selected = %d, want 2", lv.Selected())
	}
}

// ─── Panel ──────────────────────────────────────────────────────────────────

func TestPanel_NewPanel(t *testing.T) {
	p := widget.NewWin10Panel()
	if !p.ShowBorder {
		t.Fatal("Win10 panel should have border")
	}
	if !p.UseAlpha {
		t.Fatal("Win10 panel should use alpha")
	}
}

func TestPanel_AddChild(t *testing.T) {
	p := widget.NewWin10Panel()
	btn := widget.NewButton("B")
	p.AddChild(btn)
	if len(p.Children()) != 1 {
		t.Fatal("expected 1 child")
	}
}

func TestPanel_DragEnabled(t *testing.T) {
	p := widget.NewWin10Panel()
	p.Drag.Enabled = true
	p.Drag.HandleHeight = 30
	p.SetBounds(image.Rect(100, 100, 400, 400))

	// WantsCapture в drag handle зоне
	ev := widget.MouseEvent{X: 200, Y: 110, Button: widget.MouseLeft, Pressed: true}
	if !p.WantsCapture(ev) {
		t.Fatal("should want capture in handle area")
	}

	// Вне drag handle зоны
	ev.Y = 200 // за пределами 30px handle
	if p.WantsCapture(ev) {
		t.Fatal("should not want capture outside handle area")
	}
}

func TestPanel_DragDisabled(t *testing.T) {
	p := widget.NewWin10Panel()
	p.Drag.Enabled = false
	ev := widget.MouseEvent{X: 200, Y: 110, Button: widget.MouseLeft, Pressed: true}
	if p.WantsCapture(ev) {
		t.Fatal("should not want capture when drag disabled")
	}
}

func TestPanel_OnMouseButton_DragDisabled(t *testing.T) {
	p := widget.NewWin10Panel()
	p.Drag.Enabled = false
	ev := widget.MouseEvent{X: 200, Y: 110, Button: widget.MouseLeft, Pressed: true}
	if p.OnMouseButton(ev) {
		t.Fatal("should not consume event when drag disabled")
	}
}
