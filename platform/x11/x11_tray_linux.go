//go:build linux && !android

package x11

import "github.com/oops1/headless-gui/v3/platform/tray"

// traySNI и trayNotifier — трей и уведомления, встроенные в X11Window.
// Неэкспортированные псевдонимы: методы продвигаются, а поле окна снаружи
// не видно (в открытом API window.X11Window его нет).
type (
	traySNI      = tray.SNI
	trayNotifier = tray.Notifier
)

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
