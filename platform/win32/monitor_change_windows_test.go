//go:build windows

package win32

import "testing"

func TestWin32MonitorsChanged_Messages(t *testing.T) {
	w := newFsWindow(t)
	n := 0
	stop := w.WatchMonitors(func() { n++ })
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
	if w.WatchMonitors(nil) != nil {
		t.Error("WatchMonitors(nil) должен вернуть nil")
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
