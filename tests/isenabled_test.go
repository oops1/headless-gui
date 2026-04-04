// Package tests — тесты свойства IsEnabled для виджетов.
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// ─── IsEnabled (Base) ───────────────────────────────────────────────────────

func TestBase_IsEnabled_DefaultTrue(t *testing.T) {
	btn := widget.NewButton("B")
	if !btn.IsEnabled() {
		t.Fatal("new widget should be enabled by default (WPF behaviour)")
	}
}

func TestBase_SetEnabled_DisablesWidget(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetEnabled(false)
	if btn.IsEnabled() {
		t.Fatal("SetEnabled(false) should disable widget")
	}
	btn.SetEnabled(true)
	if !btn.IsEnabled() {
		t.Fatal("SetEnabled(true) should enable widget")
	}
}

// ─── Button: disabled blocks input ──────────────────────────────────────────

func TestButton_Disabled_IgnoresClick(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(0, 0, 100, 40))
	clicked := false
	btn.OnClick = func() { clicked = true }
	btn.SetEnabled(false)

	// Нажатие на disabled-кнопку не должно вызвать OnClick
	btn.OnMouseButton(widget.MouseEvent{X: 50, Y: 20, Button: widget.MouseLeft, Pressed: true})
	btn.OnMouseButton(widget.MouseEvent{X: 50, Y: 20, Button: widget.MouseLeft, Pressed: false})
	if clicked {
		t.Fatal("disabled button should not fire OnClick")
	}
}

func TestButton_Disabled_IgnoresHover(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(0, 0, 100, 40))
	btn.SetEnabled(false)

	btn.OnMouseMove(50, 20)
	if btn.IsHovered() {
		t.Fatal("disabled button should not become hovered")
	}
}

func TestButton_Disabled_IgnoresKeyboard(t *testing.T) {
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(0, 0, 100, 40))
	btn.SetFocused(true)
	btn.SetEnabled(false)
	clicked := false
	btn.OnClick = func() { clicked = true }

	btn.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if clicked {
		t.Fatal("disabled button should not fire OnClick on Enter")
	}
}

// ─── CheckBox: disabled blocks toggle ───────────────────────────────────────

func TestCheckBox_Disabled_IgnoresClick(t *testing.T) {
	cb := widget.NewCheckBox("Test")
	cb.SetBounds(image.Rect(0, 0, 200, 30))
	cb.SetEnabled(false)

	cb.OnMouseButton(widget.MouseEvent{X: 10, Y: 15, Button: widget.MouseLeft, Pressed: true})
	cb.OnMouseButton(widget.MouseEvent{X: 10, Y: 15, Button: widget.MouseLeft, Pressed: false})
	if cb.IsChecked() {
		t.Fatal("disabled checkbox should not toggle on click")
	}
}

// ─── Slider: disabled blocks drag ──────────────────────────────────────────

func TestSlider_Disabled_IgnoresMouse(t *testing.T) {
	s := widget.NewSlider()
	s.SetBounds(image.Rect(0, 0, 200, 30))
	s.SetValue(0.5)
	s.SetEnabled(false)

	s.OnMouseButton(widget.MouseEvent{X: 180, Y: 15, Button: widget.MouseLeft, Pressed: true})
	if s.Value() != 0.5 {
		t.Fatalf("disabled slider value changed: got %f, want 0.5", s.Value())
	}
}

// ─── ToggleSwitch: disabled blocks toggle ───────────────────────────────────

func TestToggleSwitch_Disabled_IgnoresClick(t *testing.T) {
	ts := widget.NewToggleSwitch("Test")
	ts.SetBounds(image.Rect(0, 0, 200, 30))
	ts.SetEnabled(false)

	ts.OnMouseButton(widget.MouseEvent{X: 10, Y: 15, Button: widget.MouseLeft, Pressed: true})
	ts.OnMouseButton(widget.MouseEvent{X: 10, Y: 15, Button: widget.MouseLeft, Pressed: false})
	if ts.IsOn() {
		t.Fatal("disabled toggle switch should not toggle")
	}
}

// ─── Dropdown: disabled blocks open ─────────────────────────────────────────

func TestDropdown_Disabled_IgnoresClick(t *testing.T) {
	dd := widget.NewDropdown("A", "B", "C")
	dd.SetBounds(image.Rect(0, 0, 200, 30))
	dd.SetEnabled(false)

	dd.OnMouseButton(widget.MouseEvent{X: 100, Y: 15, Button: widget.MouseLeft, Pressed: true})
	if dd.IsOpen() {
		t.Fatal("disabled dropdown should not open")
	}
}

// ─── TextInput: disabled blocks input ───────────────────────────────────────

func TestTextInput_Disabled_IgnoresKeyboard(t *testing.T) {
	ti := widget.NewTextInput("placeholder")
	ti.SetBounds(image.Rect(0, 0, 200, 30))
	ti.SetFocused(true)
	ti.SetEnabled(false)

	ti.OnKeyEvent(widget.KeyEvent{Code: widget.KeyA, Rune: 'a', Pressed: true})
	if ti.GetText() != "" {
		t.Fatal("disabled text input should not accept keyboard input")
	}
}

// ─── IsEnabled on various widget types ──────────────────────────────────────

// enabledWidget — интерфейс для виджетов с поддержкой IsEnabled.
type enabledWidget interface {
	widget.Widget
	IsEnabled() bool
	SetEnabled(v bool)
}

func TestAllWidgets_IsEnabled_Default(t *testing.T) {
	widgets := []struct {
		name string
		w    enabledWidget
	}{
		{"Panel", widget.NewPanel(widget.DarkTheme().PanelBG)},
		{"Button", widget.NewButton("B")},
		{"CheckBox", widget.NewCheckBox("CB")},
		{"RadioButton", widget.NewRadioButton("RB", "g")},
		{"Slider", widget.NewSlider()},
		{"ToggleSwitch", widget.NewToggleSwitch("TS")},
		{"TextInput", widget.NewTextInput("ph")},
		{"Dropdown", widget.NewDropdown("A")},
		{"ProgressBar", widget.NewProgressBar()},
		{"Label", widget.NewWin10Label("L")},
		{"ListView", widget.NewListView("a", "b")},
		{"ScrollView", widget.NewScrollView()},
		{"TabControl", widget.NewTabControl()},
	}

	for _, tc := range widgets {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.w.IsEnabled() {
				t.Fatalf("%s: IsEnabled should be true by default", tc.name)
			}
			tc.w.SetEnabled(false)
			if tc.w.IsEnabled() {
				t.Fatalf("%s: IsEnabled should be false after SetEnabled(false)", tc.name)
			}
		})
	}
}
