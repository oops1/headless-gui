//go:build darwin

package cocoa

import (
	"os"
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/native/contracttest"
)

// Контрактные сценарии бэкендов (ADR-0013 §7) для Cocoa. Живого macOS у
// проекта нет: набор собирается (кросс-сборка и vet под darwin), а гоняется
// только по просьбе на Mac — HEADLESS_GUI_COCOA_LIVE=1. Окну Cocoa нужен
// главный поток и цикл NSApplication, поэтому Pump лишь ждёт.
func TestCocoaContract(t *testing.T) {
	if os.Getenv("HEADLESS_GUI_COCOA_LIVE") != "1" {
		t.Skip("живого macOS нет: набор только собирается (HEADLESS_GUI_COCOA_LIVE=1 — прогон на Mac)")
	}
	contracttest.Run(t, contracttest.Env{
		New:      func(*testing.T) native.NativeWindow { return &CocoaWindow{} },
		Pump:     contracttest.Poll,
		Destroy:  func(nw native.NativeWindow) { nw.Close() },
		Requests: true,
	})
}
