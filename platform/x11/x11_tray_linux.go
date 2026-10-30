//go:build linux && !android

package x11

// x11_tray_linux.go — окно X11 прячется в трей и возвращается (интерфейс
// ядра trayWindow). Иконка в трее и уведомления — возможность platform/tray.

// HideToTray прячет окно X11: UnmapWindow убирает его и с экрана, и из панели
// задач EWMH-WM (полный аналог SW_HIDE на Windows).
func (w *X11Window) HideToTray() {
	if w.wid != 0 {
		w.x11UnmapWindow(w.wid)
	}
}

// RestoreFromTray показывает окно обратно и просит WM его активировать.
// Спрятанное окно менеджер выводит из обращения и снимает его _NET_WM_STATE
// (EWMH), поэтому окно-инструмент (Style.Tool) повторяет свои SKIP-состояния.
func (w *X11Window) RestoreFromTray() {
	if w.wid == 0 {
		return
	}
	w.x11MapWindow(w.wid)
	if w.style.tool.Load() {
		w.sendSkipStates(netWMStateAdd)
	}
	w.x11RaiseWindow()
	w.x11NetActiveWindow()
}
