//go:build windows

package tray

import (
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/platform/win32"
)

func init() {
	native.RegisterTray(newTray)
	// События иконки приходят окну сообщением; WM_DESTROY снимает иконку
	// вместе с окном и пропускает сообщение дальше (handled=false).
	win32.AddMessageHook(wmTrayCallback, onTrayCallback)
	win32.AddMessageHook(wmDestroy, onDestroy)
}
