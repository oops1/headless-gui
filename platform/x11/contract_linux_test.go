//go:build linux && !android

package x11

import (
	"os"
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/native/contracttest"
)

// Контрактные сценарии бэкендов (ADR-0013 §7) на настоящем X-сервере
// (DISPLAY); без него — пропуск.
func TestX11Contract(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("нет DISPLAY: X-сервера для живых сценариев нет")
	}
	contracttest.Run(t, contracttest.Env{
		New:     func(*testing.T) native.NativeWindow { return &X11Window{} },
		Started: func(nw native.NativeWindow) { nw.(*X11Window).StartEventPump() },
		Pump:    contracttest.Poll,
		Destroy: func(nw native.NativeWindow) { nw.Close() },
		Style:   native.Style{Borderless: true, Tool: true},

		Requests: true,
	})
}
