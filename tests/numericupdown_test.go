package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func TestNumericUpDown_StepClamp(t *testing.T) {
	n := widget.NewNumericUpDown()
	n.Min, n.Max, n.Step = 0, 5, 2
	n.SetValue(0)

	// Up arrow → +2
	n.OnKeyEvent(widget.KeyEvent{Code: widget.KeyUp, Pressed: true})
	if n.Value() != 2 {
		t.Fatalf("after up: got %v want 2", n.Value())
	}
	n.OnKeyEvent(widget.KeyEvent{Code: widget.KeyUp, Pressed: true}) // 4
	n.OnKeyEvent(widget.KeyEvent{Code: widget.KeyUp, Pressed: true}) // clamps to 5
	if n.Value() != 5 {
		t.Fatalf("clamp max: got %v want 5", n.Value())
	}
	// Down below min clamps to 0
	for i := 0; i < 5; i++ {
		n.OnKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
	}
	if n.Value() != 0 {
		t.Fatalf("clamp min: got %v want 0", n.Value())
	}
}

func TestNumericUpDown_TypeAndCommit(t *testing.T) {
	n := widget.NewNumericUpDown()
	n.Min, n.Max = 0, 1000
	n.SetValue(0)

	// Focus, type "42", commit via Enter
	n.SetFocused(true)
	for _, r := range "42" {
		n.OnKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
	}
	n.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if n.Value() != 42 {
		t.Fatalf("typed value: got %v want 42", n.Value())
	}
}

func TestNumericUpDown_OnChange(t *testing.T) {
	n := widget.NewNumericUpDown()
	n.Min, n.Max, n.Step = 0, 10, 1
	got := -1.0
	n.OnChange = func(v float64) { got = v }
	n.OnKeyEvent(widget.KeyEvent{Code: widget.KeyUp, Pressed: true})
	if got != 1 {
		t.Fatalf("OnChange: got %v want 1", got)
	}
}

func TestNumericUpDown_XAML(t *testing.T) {
	xaml := `<Canvas xmlns="clr">
		<NumericUpDown Minimum="0" Maximum="100" Increment="5" Value="20" Decimals="0"/>
	</Canvas>`
	root, _, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("load xaml: %v", err)
	}
	if root == nil {
		t.Fatal("nil root")
	}
}
