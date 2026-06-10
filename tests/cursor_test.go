package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

func TestCursor_XAMLAttribute(t *testing.T) {
	const xaml = `<Canvas xmlns="clr">
		<Button Name="b" Cursor="Hand"/>
	</Canvas>`

	root, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	btn := reg["b"].(*widget.Button)

	// Проверяем CursorOverride на уровне виджета.
	c, has := btn.CursorOverride()
	if !has {
		t.Fatal("CursorOverride: has=false, ожидалось true")
	}
	if c != widget.CursorHand {
		t.Fatalf("CursorOverride = %v, want CursorHand", c)
	}

	// Проверяем через движок: CursorAt над кнопкой возвращает Hand.
	eng := engine.New(800, 600, 1)
	root.SetBounds(image.Rect(0, 0, 800, 600))
	eng.SetRoot(root)
	btn.SetBounds(image.Rect(10, 10, 110, 40))

	got := eng.CursorAt(50, 25)
	if got != widget.CursorHand {
		t.Fatalf("CursorAt(50,25) = %v, want CursorHand", got)
	}
}

func TestCursor_TextInputNoOverride(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(0, 0, 200, 30))

	eng := engine.New(200, 100, 1)
	ti.SetBounds(image.Rect(0, 0, 200, 100))
	eng.SetRoot(ti)

	got := eng.CursorAt(100, 50)
	if got != widget.CursorIBeam {
		t.Fatalf("TextInput без Cursor: CursorAt = %v, want CursorIBeam", got)
	}
}
