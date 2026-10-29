//go:build linux && !android

package core

import "os"

// platformMonitors — мониторы без окна: Wayland, если он доступен (так же
// выбирает бэкенд NewNativeWindow), иначе X11 (в том числе XWayland).
func platformMonitors() []Monitor {
	if os.Getenv("HEADLESS_GUI_X11") != "1" {
		if ww := newWaylandWindow(); ww != nil {
			if list := ww.queryMonitors(); len(list) > 0 {
				return list
			}
		}
	}
	return x11PlatformMonitors()
}
