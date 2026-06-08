package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// type a string char-by-char.
func typeStr(ti *widget.TextInput, s string) {
	for _, r := range s {
		ti.OnKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
	}
}

func TestTextInput_MaxLength(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.MaxLength = 5
	typeStr(ti, "abcdefgh")
	if got := ti.GetText(); got != "abcde" {
		t.Fatalf("MaxLength: got %q, want %q", got, "abcde")
	}
}

func TestTextInput_Undo(t *testing.T) {
	ti := widget.NewTextInput("")
	typeStr(ti, "abc")
	// Ctrl+Z three times → empty (each char is one edit step).
	ctrlZ := widget.KeyEvent{Code: widget.KeyZ, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlZ)
	if got := ti.GetText(); got != "ab" {
		t.Fatalf("after 1 undo: got %q, want %q", got, "ab")
	}
	ti.OnKeyEvent(ctrlZ)
	ti.OnKeyEvent(ctrlZ)
	if got := ti.GetText(); got != "" {
		t.Fatalf("after 3 undo: got %q, want empty", got)
	}
}

func TestTextInput_Redo(t *testing.T) {
	ti := widget.NewTextInput("")
	typeStr(ti, "ab")
	ctrlZ := widget.KeyEvent{Code: widget.KeyZ, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlZ) // -> "a"
	ti.OnKeyEvent(ctrlZ) // -> ""
	// Ctrl+Y redo
	ctrlY := widget.KeyEvent{Code: widget.KeyY, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlY) // -> "a"
	if got := ti.GetText(); got != "a" {
		t.Fatalf("after redo: got %q, want %q", got, "a")
	}
	ti.OnKeyEvent(ctrlY) // -> "ab"
	if got := ti.GetText(); got != "ab" {
		t.Fatalf("after 2 redo: got %q, want %q", got, "ab")
	}
}

func TestTextInput_UndoRedoShiftZ(t *testing.T) {
	ti := widget.NewTextInput("")
	typeStr(ti, "x")
	ctrlZ := widget.KeyEvent{Code: widget.KeyZ, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlZ) // -> ""
	// Ctrl+Shift+Z is redo
	ctrlShiftZ := widget.KeyEvent{Code: widget.KeyZ, Mod: widget.ModCtrl | widget.ModShift, Pressed: true}
	ti.OnKeyEvent(ctrlShiftZ)
	if got := ti.GetText(); got != "x" {
		t.Fatalf("after Ctrl+Shift+Z redo: got %q, want %q", got, "x")
	}
}

// New edits should clear the redo stack (WPF behavior).
func TestTextInput_RedoClearedOnEdit(t *testing.T) {
	ti := widget.NewTextInput("")
	typeStr(ti, "ab")
	ctrlZ := widget.KeyEvent{Code: widget.KeyZ, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlZ) // -> "a", redo has "ab"
	typeStr(ti, "c")     // -> "ac", redo cleared
	ctrlY := widget.KeyEvent{Code: widget.KeyY, Mod: widget.ModCtrl, Pressed: true}
	ti.OnKeyEvent(ctrlY) // nothing to redo
	if got := ti.GetText(); got != "ac" {
		t.Fatalf("redo after new edit: got %q, want %q", got, "ac")
	}
}
