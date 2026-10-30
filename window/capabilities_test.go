package window

import (
	"runtime"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// Фасад window подключает возможности окна (capabilities.go): каждая, что
// умеет текущая ОС, зарегистрирована.
func TestCapabilities_Wired(t *testing.T) {
	want := map[string][]string{
		"windows": {"a11y", "shell", "tray"},
		"linux":   {"a11y", "shell", "tray"},
		"darwin":  {"shell"},
	}[runtime.GOOS]
	got := map[string]bool{}
	for _, c := range native.Capabilities() {
		got[c] = true
	}
	for _, c := range want {
		if !got[c] {
			t.Errorf("возможность %q не подключена: %v", c, native.Capabilities())
		}
	}
}
