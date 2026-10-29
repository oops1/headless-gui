//go:build windows

package core

import "testing"

// Сообщения окна, которые будят подписчика на смену мониторов. wndProc зовётся
// напрямую с теми же аргументами, что и система: окно создано в этом же потоке.

func TestWin32MonitorsChanged_Messages(t *testing.T) {
	w := newFsWindow(t)
	n := 0
	stop := w.watchMonitors(func() { n++ })
	if stop == nil {
		t.Fatal("подписка на Win32Window не поднялась")
	}
	hwnd := uintptr(w.hwnd)

	wndProc(hwnd, wmDisplaychange, 32, 0)
	if n != 1 {
		t.Errorf("WM_DISPLAYCHANGE дал %d вызовов, ждал 1", n)
	}
	wndProc(hwnd, wmDpichanged, 120, 0)
	if n != 2 {
		t.Errorf("WM_DPICHANGED дал %d вызовов, ждал 2", n)
	}
	wndProc(hwnd, wmSettingchange, spiSetWorkArea, 0)
	if n != 3 {
		t.Errorf("WM_SETTINGCHANGE(SPI_SETWORKAREA) дал %d вызовов, ждал 3", n)
	}

	// Прочие параметры системы мониторов не меняют.
	wndProc(hwnd, wmSettingchange, 0x0001 /* SPI_SETBEEP */, 0)
	wndProc(hwnd, wmSettingchange, 0, 0)
	if n != 3 {
		t.Errorf("посторонний WM_SETTINGCHANGE разбудил подписчика: %d", n)
	}

	stop()
	wndProc(hwnd, wmDisplaychange, 32, 0)
	if n != 3 {
		t.Errorf("подписка снята, но вызов был: %d", n)
	}
	if w.watchMonitors(nil) != nil {
		t.Error("watchMonitors(nil) должен вернуть nil")
	}
}

func TestMonitorsChangedBySetting(t *testing.T) {
	if !monitorsChangedBySetting(spiSetWorkArea) || spiSetWorkArea != 0x2F {
		t.Error("SPI_SETWORKAREA (0x2F) не опознан")
	}
	for _, v := range []uintptr{0, 1, 0x2E, 0x30, 0x1F} {
		if monitorsChangedBySetting(v) {
			t.Errorf("wparam %#x принят за смену рабочей области", v)
		}
	}
}

// Сквозная проверка с общим слоем: Window получает событие от настоящего окна,
// и колбэк срабатывает только при перемене списка.
func TestWin32MonitorsChanged_ThroughWindow(t *testing.T) {
	w := newFsWindow(t)
	l := monTestUseList(t, monTestA)
	win, _ := monTestWindow(t)
	win.native = w
	fn, ch := monTestFn()
	win.SetOnMonitorsChanged(fn)

	wndProc(uintptr(w.hwnd), wmDisplaychange, 32, 0) // список прежний
	monTestExpectNone(t, ch)
	l.set(monTestA, monTestB)
	wndProc(uintptr(w.hwnd), wmDisplaychange, 32, 0)
	monTestExpect(t, ch, 2)
	win.stopMonitorWatch()
}
