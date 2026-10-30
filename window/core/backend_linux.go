//go:build linux && !android

package core

// backend_linux.go — выбор бэкенда Linux: Wayland или X11.

import (
	"os"

	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/platform/x11"
)

// X11Window — реализация NativeWindow через X11 протокол.
// Чистый Go без CGO и внешних зависимостей.
// Общается с X-сервером напрямую через Unix socket.
type X11Window = x11.X11Window

// NewNativeWindow выбирает бэкенд: Wayland при доступном композиторе
// (WAYLAND_DISPLAY + живой сокет), иначе X11 (в т.ч. XWayland).
// HEADLESS_GUI_X11=1 — принудительный X11.
func NewNativeWindow() native.NativeWindow {
	if os.Getenv("HEADLESS_GUI_X11") != "1" {
		if ww := newWaylandWindow(); ww != nil {
			return ww
		}
	}
	return &X11Window{}
}
