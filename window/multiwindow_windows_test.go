//go:build windows

package window

import (
	"runtime"
	"testing"
	"unsafe"
)

const wmQuitMsg = 0x0012 // WM_QUIT

// mwQuitPosted уничтожает окно и сообщает, оставил ли WM_DESTROY в очереди
// потока WM_QUIT (тот самый, что завершает Run главного окна). Окно создаётся
// в этом же потоке, поэтому DestroyWindow исполняет wndProc синхронно.
func mwQuitPosted(t *testing.T, secondary bool) bool {
	t.Helper()
	w := &Win32Window{}
	if secondary {
		w.markSecondary()
	}
	if err := w.Create("multiwindow-test", 200, 150); err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	procDestroyWindow.Call(uintptr(w.hwnd))
	quit := false
	var m msg
	for {
		if r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1); r == 0 {
			return quit
		}
		if m.Message == wmQuitMsg {
			quit = true
			continue
		}
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// Окно из OpenWindow (markSecondary) при уничтожении не завершает цикл
// сообщений; обычное главное окно — завершает, как и раньше.
func TestWin32Window_SecondaryDestroyKeepsLoop(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if mwQuitPosted(t, true) {
		t.Error("уничтожение вторичного окна поставило WM_QUIT — Run главного окна вернулся бы")
	}
	if !mwQuitPosted(t, false) {
		t.Error("уничтожение главного окна не поставило WM_QUIT — приложение не завершилось бы")
	}
}
