//go:build windows

package core

import (
	"os"
	"testing"
	"unsafe"
)

// Передний план и свёрнутость Win32 на настоящем окне: окно создаётся в этом
// же потоке, поэтому ShowWindow/SetForegroundWindow выполняют wndProc синхронно
// и цикл сообщений не нужен (см. newFsWindow).

// FLASHWINFO: размер по документации (32 байта на 64-битной Windows, 20 на
// 32-битной), «мигать, пока окно не станет активным».
func TestFlashRequest(t *testing.T) {
	fi := flashRequest(0x1234)
	want := uint32(20)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		want = 32
	}
	if fi.CbSize != want {
		t.Errorf("cbSize %d, ждал %d", fi.CbSize, want)
	}
	if fi.Hwnd != 0x1234 {
		t.Errorf("hwnd %#x", fi.Hwnd)
	}
	// FLASHW_ALL (3) | FLASHW_TIMERNOFG (0xC) по winuser.h.
	if fi.Flags != 0xF {
		t.Errorf("флаги %#x, ждал 0xF", fi.Flags)
	}
}

// AllowSetForegroundWindow: отрицательный pid — ASFW_ANY, ноль — не процесс.
func TestAllowForegroundArg(t *testing.T) {
	for _, tc := range []struct {
		pid int
		arg uintptr
		ok  bool
	}{
		{4242, 4242, true},
		{-1, 0xFFFFFFFF, true},
		{0, 0, false},
	} {
		arg, ok := allowForegroundArg(tc.pid)
		if arg != tc.arg || ok != tc.ok {
			t.Errorf("allowForegroundArg(%d) = %#x, %v; ждал %#x, %v", tc.pid, arg, ok, tc.arg, tc.ok)
		}
	}
	if AllowForeground(0) {
		t.Error("AllowForeground(0) принят")
	}
	// Выдать право может только процесс, которому оно самому принадлежит, —
	// у тестового процесса оно есть не всегда: ответ только в журнал.
	t.Logf("AllowForeground(свой pid) = %v", AllowForeground(os.Getpid()))
}

// Свернуть → Activate → окно не свёрнуто; смена свёрнутости доходит до
// колбэка, IsMinimized совпадает с IsIconic.
func TestWin32Activate_RestoresMinimized(t *testing.T) {
	w := newFsWindow(t)
	var seen []bool
	w.SetOnMinimizedChanged(func(m bool) { seen = append(seen, m) })

	if w.IsMinimized() {
		t.Fatal("новое окно свёрнуто")
	}
	w.Minimize()
	if !w.IsMinimized() {
		t.Skip("окно не сворачивается в этой сессии")
	}
	if len(seen) != 1 || !seen[0] {
		t.Errorf("уведомления %v после сворачивания, ждал [true]", seen)
	}

	ok := w.Activate()
	if w.IsMinimized() {
		t.Fatal("после Activate окно осталось свёрнутым")
	}
	if len(seen) != 2 || seen[1] {
		t.Errorf("уведомления %v после Activate, ждал [true false]", seen)
	}
	// true только если система действительно сделала окно активным.
	fg, _, _ := procGetForegroundWindow.Call()
	if ok && fg != uintptr(w.hwnd) {
		t.Errorf("Activate вернул true, но передний план у %#x, окно %#x", fg, w.hwnd)
	}
	t.Logf("Activate = %v (передний план у окна: %v)", ok, fg == uintptr(w.hwnd))
}

// Скрытое окно (убрано в трей) Activate показывает; без токена тот же путь.
func TestWin32Activate_ShowsHidden(t *testing.T) {
	w := newFsWindow(t)
	procShowWindow.Call(uintptr(w.hwnd), 0) // SW_HIDE
	if v, _, _ := procIsWindowVisible.Call(uintptr(w.hwnd)); v != 0 {
		t.Skip("окно не скрывается")
	}
	w.ActivateWithToken("токен не нужен")
	if v, _, _ := procIsWindowVisible.Call(uintptr(w.hwnd)); v == 0 {
		t.Error("Activate не показал скрытое окно")
	}
}

// Окно без дескриптора (до Create) — false, без падений.
func TestWin32Activate_NoWindow(t *testing.T) {
	w := &Win32Window{}
	if w.Activate() || w.IsMinimized() {
		t.Error("окно без hwnd ответило true")
	}
}

// WM_SIZE с SIZE_MINIMIZED меняет признак и шлёт уведомление ровно на смене.
func TestWin32NoteMinimized(t *testing.T) {
	w := &Win32Window{}
	var seen []bool
	w.SetOnMinimizedChanged(func(m bool) { seen = append(seen, m) })
	w.noteMinimized(false) // и так не свёрнуто
	w.noteMinimized(true)
	w.noteMinimized(true)
	w.noteMinimized(false)
	if len(seen) != 2 || !seen[0] || seen[1] {
		t.Errorf("уведомления %v, ждал [true false]", seen)
	}
}
