package core

import "testing"

// Живое окно Win32: WS_EX_TOPMOST ставится и снимается.
func TestWin32AlwaysOnTop_Toggles(t *testing.T) {
	w := newFsWindow(t)
	if w.IsAlwaysOnTop() {
		t.Fatal("новое окно уже поверх всех")
	}
	if !w.SetAlwaysOnTop(true) || !w.IsAlwaysOnTop() {
		t.Fatal("SetAlwaysOnTop(true) не поставил WS_EX_TOPMOST")
	}
	if !w.SetAlwaysOnTop(false) || w.IsAlwaysOnTop() {
		t.Fatal("SetAlwaysOnTop(false) не снял WS_EX_TOPMOST")
	}
}
