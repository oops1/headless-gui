//go:build linux && !android

package tray

import (
	"github.com/oops1/headless-gui/v3/platform/native"
)

// linuxTray — трей окна на Linux: иконка StatusNotifierItem и уведомления
// org.freedesktop.Notifications. От окна бэкенда (X11 или Wayland) не зависит:
// и иконка, и уведомления — разговор с панелью и демоном по D-Bus, поэтому
// трей одинаков для любого окна.
type linuxTray struct {
	SNI
	Notifier
}

var _ native.Tray = (*linuxTray)(nil)

// Close снимает иконку и уведомления окна.
func (t *linuxTray) Close() {
	t.SNI.Close()
	t.Notifier.Close()
}

func init() {
	native.RegisterTray(func(native.NativeWindow) native.Tray { return &linuxTray{} })
}
