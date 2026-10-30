//go:build linux && !android

package wayland

// wayland_tray_linux.go — «спрятать в трей» на Wayland (интерфейс ядра
// trayWindow) и передний план. Иконка в трее и уведомления — возможность
// platform/tray, композитора они не касаются.

// HideToTray на Wayland — no-op: xdg_toplevel скрыть нельзя (нет ни unmap, ни
// «спрятать» в протоколе), а set_minimized лишь просит композитор свернуть окно
// и на многих композиторах не реализован. Иконка в трее при этом работает.
func (w *WaylandWindow) HideToTray() {}

// RestoreFromTray на Wayland — no-op: скрывать было нечего (см. HideToTray).
func (w *WaylandWindow) RestoreFromTray() {}

// SetForeground на Wayland — no-op: активацию окна раздаёт композитор. Вывод
// окна на передний план по запросу приложения — Activate (xdg-activation-v1,
// activate_wayland_linux.go). Метод нужен модальным окнам и доку (интерфейс
// ядра foregrounder).
func (w *WaylandWindow) SetForeground() {}
