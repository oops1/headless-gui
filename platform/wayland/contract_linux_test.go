//go:build linux && !android

package wayland

import (
	"os"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/platform/native/contracttest"
)

// Контрактные сценарии бэкендов (ADR-0013 §7) на настоящем компоновщике
// (WAYLAND_DISPLAY); без него — пропуск.
func TestWaylandContract(t *testing.T) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("нет WAYLAND_DISPLAY: компоновщика для живых сценариев нет")
	}
	contracttest.Run(t, contracttest.Env{
		New: func(t *testing.T) native.NativeWindow {
			w := NewWindow()
			if w == nil {
				t.Skip("компоновщик недоступен (нет сокета в XDG_RUNTIME_DIR)")
			}
			return w
		},
		Started: func(nw native.NativeWindow) { nw.(*WaylandWindow).StartEventPump() },
		Pump:    contracttest.Poll,
		Destroy: func(nw native.NativeWindow) { nw.Close() },
		Style:   native.Style{Borderless: true},

		Requests:            true,
		LockKeysSetOptional: true,
	})
}
