//go:build linux && !android

package core

import "github.com/oops1/headless-gui/v3/platform/tray"

// traySNI и trayNotifier — трей и уведомления, встроенные в WaylandWindow.
// Неэкспортированные псевдонимы: методы продвигаются, а поле окна снаружи
// не видно (в открытом API window.WaylandWindow его нет).
type (
	traySNI      = tray.SNI
	trayNotifier = tray.Notifier
)

// HideToTray на Wayland — no-op: xdg_toplevel скрыть нельзя (нет ни unmap, ни
// «спрятать» в протоколе), а set_minimized лишь просит композитор свернуть окно
// и на многих композиторах не реализован. Иконка в трее при этом работает.
func (w *WaylandWindow) HideToTray() {}

// RestoreFromTray на Wayland — no-op: скрывать было нечего (см. HideToTray).
func (w *WaylandWindow) RestoreFromTray() {}

// SetForeground на Wayland — no-op: активацию окна раздаёт композитор. Вывод
// окна на передний план по запросу приложения — Activate (xdg-activation-v1,
// activate_wayland_linux.go). Метод нужен, чтобы окно удовлетворяло интерфейсу
// trayHost.
func (w *WaylandWindow) SetForeground() {}
