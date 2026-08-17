package widget

import "testing"

func typeInput(ti *TextInput, s string) {
	for _, r := range s {
		ti.OnKeyEvent(KeyEvent{Rune: r, Pressed: true})
	}
}

// Включение режима пароля стирает историю правок.
func TestTextInput_PasswordModeWipesUndo(t *testing.T) {
	ti := NewTextInput("")
	typeInput(ti, "secret")
	if len(ti.undoStack) == 0 {
		t.Fatal("история пуста ещё до включения пароля")
	}

	ti.SetPasswordMode(true)
	if len(ti.undoStack) != 0 || len(ti.redoStack) != 0 {
		t.Fatalf("undo=%d redo=%d, ожидались пустые", len(ti.undoStack), len(ti.redoStack))
	}

	ti.OnKeyEvent(KeyEvent{Code: KeyZ, Mod: ModCtrl, Pressed: true})
	if got := ti.GetText(); got != "secret" {
		t.Fatalf("Ctrl+Z достал прежний текст: %q", got)
	}
}

// В обычном режиме undo продолжает работать.
func TestTextInput_UndoStillWorks(t *testing.T) {
	ti := NewTextInput("")
	typeInput(ti, "abc")
	ti.OnKeyEvent(KeyEvent{Code: KeyZ, Mod: ModCtrl, Pressed: true})
	if got := ti.GetText(); got != "ab" {
		t.Fatalf("после Ctrl+Z: %q, want \"ab\"", got)
	}
}

// В режиме пароля правки в историю не пишутся.
func TestTextInput_NoUndoInPasswordMode(t *testing.T) {
	ti := NewTextInput("")
	typeInput(ti, "old")
	ti.SetPasswordMode(true)
	typeInput(ti, "X")
	ti.OnKeyEvent(KeyEvent{Code: KeyZ, Mod: ModCtrl, Pressed: true})
	if got := ti.GetText(); got != "oldX" {
		t.Fatalf("после Ctrl+Z: %q, want \"oldX\"", got)
	}
	if len(ti.undoStack) != 0 {
		t.Fatalf("история в режиме пароля: %d записей", len(ti.undoStack))
	}
}
