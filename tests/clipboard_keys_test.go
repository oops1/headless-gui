package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// clipboard_keys_test.go — «старошкольные» клавиши буфера обмена и удаление
// слова для TextInput и TextBox:
//   - Ctrl+Insert   = копировать (как Ctrl+C)
//   - Shift+Insert  = вставить   (как Ctrl+V)
//   - Shift+Delete  = вырезать   (как Ctrl+X), приоритетнее обычного Delete
//   - Ctrl+Delete   = удалить слово ВПЕРЁД
//   - Ctrl+Backspace= удалить слово НАЗАД

// tiKey отправляет клавишу с модификатором в TextInput.
func tiKey(ti *widget.TextInput, code widget.KeyCode, mod widget.KeyMod) {
	ti.OnKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
}

// tiSelect выделяет первые n символов от начала (Home + n·Shift+Right).
func tiSelect(ti *widget.TextInput, n int) {
	tiKey(ti, widget.KeyHome, 0)
	for i := 0; i < n; i++ {
		tiKey(ti, widget.KeyRight, widget.ModShift)
	}
}

// ─── TextInput ───────────────────────────────────────────────────────────────

func TestTextInput_CtrlInsertCopies(t *testing.T) {
	widget.UseMemoryClipboard()
	ti := widget.NewTextInput("")
	ti.SetText("hello world")
	tiSelect(ti, 5) // "hello"
	tiKey(ti, widget.KeyInsert, widget.ModCtrl)
	if c := widget.ClipboardGetText(); c != "hello" {
		t.Fatalf("Ctrl+Insert: clipboard=%q, want %q", c, "hello")
	}
	if got := ti.GetText(); got != "hello world" {
		t.Fatalf("Ctrl+Insert не должен менять текст: %q", got)
	}
}

func TestTextInput_ShiftInsertPastes(t *testing.T) {
	widget.UseMemoryClipboard()
	widget.ClipboardSetText("XY")
	ti := widget.NewTextInput("")
	ti.SetText("ab") // каретка в конце
	tiKey(ti, widget.KeyInsert, widget.ModShift)
	if got := ti.GetText(); got != "abXY" {
		t.Fatalf("Shift+Insert: %q, want %q", got, "abXY")
	}
}

func TestTextInput_ShiftDeleteCuts(t *testing.T) {
	widget.UseMemoryClipboard()
	ti := widget.NewTextInput("")
	ti.SetText("hello world")
	tiSelect(ti, 5) // "hello"
	tiKey(ti, widget.KeyDelete, widget.ModShift)
	if got := ti.GetText(); got != " world" {
		t.Fatalf("Shift+Delete текст: %q, want %q", got, " world")
	}
	if c := widget.ClipboardGetText(); c != "hello" {
		t.Fatalf("Shift+Delete clipboard=%q, want %q", c, "hello")
	}
}

func TestTextInput_CtrlDeleteWordForward(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetText("foo bar")
	tiKey(ti, widget.KeyHome, 0)
	tiKey(ti, widget.KeyDelete, widget.ModCtrl)
	if got := ti.GetText(); got != "bar" {
		t.Fatalf("Ctrl+Delete: %q, want %q", got, "bar")
	}
}

func TestTextInput_CtrlBackspaceWordBack(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetText("foo bar") // каретка в конце
	tiKey(ti, widget.KeyBackspace, widget.ModCtrl)
	if got := ti.GetText(); got != "foo " {
		t.Fatalf("Ctrl+Backspace: %q, want %q", got, "foo ")
	}
}

func TestTextInput_PasswordCtrlInsertBlocked(t *testing.T) {
	widget.UseMemoryClipboard()
	widget.ClipboardSetText("prev")
	ti := widget.NewPasswordInput("")
	ti.SetText("secret")
	tiSelect(ti, 3)
	tiKey(ti, widget.KeyInsert, widget.ModCtrl)
	if c := widget.ClipboardGetText(); c != "prev" {
		t.Fatalf("password Ctrl+Insert не должен копировать: clipboard=%q", c)
	}
}

// ─── TextBox ─────────────────────────────────────────────────────────────────

// tbSelect выделяет первые n символов от начала документа.
func tbSelect(tb *widget.TextBox, n int) {
	tbKey(tb, widget.KeyHome, widget.ModCtrl)
	for i := 0; i < n; i++ {
		tbKey(tb, widget.KeyRight, widget.ModShift)
	}
}

func TestTextBox_CtrlInsertCopies(t *testing.T) {
	widget.UseMemoryClipboard()
	tb := newTB(t, 300, 100)
	tb.SetText("hello world")
	tbSelect(tb, 5)
	tbKey(tb, widget.KeyInsert, widget.ModCtrl)
	if c := widget.ClipboardGetText(); c != "hello" {
		t.Fatalf("Ctrl+Insert: clipboard=%q, want %q", c, "hello")
	}
	if got := tb.GetText(); got != "hello world" {
		t.Fatalf("Ctrl+Insert не должен менять текст: %q", got)
	}
}

func TestTextBox_ShiftInsertPastes(t *testing.T) {
	widget.UseMemoryClipboard()
	widget.ClipboardSetText("XY")
	tb := newTB(t, 300, 100)
	tb.SetText("ab") // каретка в конце
	tbKey(tb, widget.KeyInsert, widget.ModShift)
	if got := tb.GetText(); got != "abXY" {
		t.Fatalf("Shift+Insert: %q, want %q", got, "abXY")
	}
}

func TestTextBox_ShiftDeleteCuts(t *testing.T) {
	widget.UseMemoryClipboard()
	tb := newTB(t, 300, 100)
	tb.SetText("hello world")
	tbSelect(tb, 5)
	tbKey(tb, widget.KeyDelete, widget.ModShift)
	if got := tb.GetText(); got != " world" {
		t.Fatalf("Shift+Delete текст: %q, want %q", got, " world")
	}
	if c := widget.ClipboardGetText(); c != "hello" {
		t.Fatalf("Shift+Delete clipboard=%q, want %q", c, "hello")
	}
}

func TestTextBox_CtrlDeleteWordForward(t *testing.T) {
	tb := newTB(t, 300, 100)
	tb.SetText("foo bar")
	tbKey(tb, widget.KeyHome, widget.ModCtrl)
	tbKey(tb, widget.KeyDelete, widget.ModCtrl)
	if got := tb.GetText(); got != "bar" {
		t.Fatalf("Ctrl+Delete: %q, want %q", got, "bar")
	}
}

func TestTextBox_CtrlBackspaceWordBack(t *testing.T) {
	tb := newTB(t, 300, 100)
	tb.SetText("foo bar") // каретка в конце
	tbKey(tb, widget.KeyBackspace, widget.ModCtrl)
	if got := tb.GetText(); got != "foo " {
		t.Fatalf("Ctrl+Backspace: %q, want %q", got, "foo ")
	}
}

func TestTextBox_ReadOnlyBlocksPasteAndCut(t *testing.T) {
	widget.UseMemoryClipboard()
	widget.ClipboardSetText("X")
	tb := newTB(t, 300, 100)
	tb.SetText("data")
	tb.ReadOnly = true

	// Shift+Insert (вставка) заблокирована.
	tbKey(tb, widget.KeyInsert, widget.ModShift)
	if got := tb.GetText(); got != "data" {
		t.Fatalf("ReadOnly Shift+Insert не должен вставлять: %q", got)
	}
	// Shift+Delete (вырезание) заблокировано, текст цел.
	tbSelect(tb, 4)
	tbKey(tb, widget.KeyDelete, widget.ModShift)
	if got := tb.GetText(); got != "data" {
		t.Fatalf("ReadOnly Shift+Delete не должен вырезать: %q", got)
	}
	// Копирование (Ctrl+Insert) при ReadOnly разрешено.
	tbSelect(tb, 4)
	tbKey(tb, widget.KeyInsert, widget.ModCtrl)
	if c := widget.ClipboardGetText(); c != "data" {
		t.Fatalf("ReadOnly Ctrl+Insert должен копировать: clipboard=%q", c)
	}
}
