//go:build windows

package win32

import (
	"runtime"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/native/contracttest"
)

// Контрактные сценарии бэкендов (ADR-0013 §7) на живых окнах Win32: окно
// создаётся и прокачивается на закреплённом потоке сценария.
func TestWin32Contract(t *testing.T) {
	contracttest.Run(t, contracttest.Env{
		New: func(t *testing.T) native.NativeWindow {
			runtime.LockOSThread()
			t.Cleanup(runtime.UnlockOSThread)
			return &Win32Window{noQuit: true}
		},
		Pump: func(cond func() bool, timeout time.Duration) bool {
			deadline := time.Now().Add(timeout)
			for {
				fsPumpMessages()
				if cond() {
					return true
				}
				if time.Now().After(deadline) {
					return cond()
				}
				time.Sleep(5 * time.Millisecond)
			}
		},
		Destroy: func(nw native.NativeWindow) {
			if w := nw.(*Win32Window); w.hwnd != 0 {
				procDestroyWindow.Call(uintptr(w.hwnd))
				fsPumpMessages()
			}
		},
		MinSizeEnforced: true,
		ToggleLockKeys:  true,
		Style:           native.Style{Borderless: true, Tool: true},
	})
}
