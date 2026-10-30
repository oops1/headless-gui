//go:build linux && !android

package core

import (
	"os"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/wayland"
	"github.com/oops1/headless-gui/v4/platform/x11"
)

// platformMonitors — мониторы без окна: Wayland, если он доступен (так же
// выбирает бэкенд NewNativeWindow), иначе X11 (в том числе XWayland).
func platformMonitors() []native.Monitor {
	if os.Getenv("HEADLESS_GUI_X11") != "1" {
		if list := wayland.EnumMonitors(); len(list) > 0 {
			return list
		}
	}
	return x11.EnumMonitors()
}
