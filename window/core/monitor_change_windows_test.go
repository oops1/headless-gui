//go:build windows

package core

import (
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testhook"
)

// Сообщения окна, которые будят подписчика на смену мониторов. wndProc зовётся
// напрямую с теми же аргументами, что и система: окно создано в этом же потоке.

// Сквозная проверка с общим слоем: Window получает событие от настоящего окна,
// и колбэк срабатывает только при перемене списка.
func TestWin32MonitorsChanged_ThroughWindow(t *testing.T) {
	w := newWin32TestWindow(t)
	l := monTestUseList(t, monTestA)
	win, _ := monTestWindow(t)
	win.native = w
	fn, ch := monTestFn()
	win.SetOnMonitorsChanged(fn)

	testhook.Win32WndProc(w.HWND(), testWMDisplayChange, 32, 0) // список прежний
	monTestExpectNone(t, ch)
	l.set(monTestA, monTestB)
	testhook.Win32WndProc(w.HWND(), testWMDisplayChange, 32, 0)
	monTestExpect(t, ch, 2)
	win.stopMonitorWatch()
}
