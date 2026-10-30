package tray

import (
	"runtime"
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// Трей регистрирует init пакета там, где он есть: Windows и Linux (X11 и
// Wayland одинаково); на прочих ОС и android — нет.
func TestRegister_Tray(t *testing.T) {
	_, ok := native.TrayService()
	want := runtime.GOOS == "windows" || runtime.GOOS == "linux"
	if ok != want {
		t.Errorf("трей зарегистрирован: %v, на %s ждал %v", ok, runtime.GOOS, want)
	}
}
