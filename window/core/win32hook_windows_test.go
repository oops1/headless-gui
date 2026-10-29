//go:build windows

package core

// win32hook_windows_test.go — помощники интеграционных тестов ядра с живым
// окном Win32. Бэкенд — другой пакет (platform/win32): его внутренности тесты
// берут через internal/testhook, а Win32 API зовут сами.

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/oops1/headless-gui/v3/internal/testhook"
	"golang.org/x/sys/windows"
)

var (
	testUser32                     = windows.NewLazySystemDLL("user32.dll")
	testGdi32                      = windows.NewLazySystemDLL("gdi32.dll")
	procTestDestroyWindow          = testUser32.NewProc("DestroyWindow")
	procTestPeekMessageW           = testUser32.NewProc("PeekMessageW")
	procTestDispatchMessageW       = testUser32.NewProc("DispatchMessageW")
	procTestGetWindowLongPtrW      = testUser32.NewProc("GetWindowLongPtrW")
	procGetLayeredWindowAttributes = testUser32.NewProc("GetLayeredWindowAttributes")
	procScreenGetDC                = testUser32.NewProc("GetDC")
	procScreenReleaseDC            = testUser32.NewProc("ReleaseDC")
	procScreenGetPixel             = testGdi32.NewProc("GetPixel")
)

// Коды Win32, нужные тестам ядра.
const (
	testWMDisplayChange = 0x007E       // WM_DISPLAYCHANGE
	testHTClient        = 1            // HTCLIENT
	testGWLExStyle      = ^uintptr(19) // GWL_EXSTYLE (-20)
	testWSExLayered     = 0x00080000   // WS_EX_LAYERED
)

// newWin32TestWindow — окно бэкенда на потоке теста; закрытие окна не
// завершает цикл сообщений, по окончании теста окно уничтожается.
func newWin32TestWindow(t *testing.T) *Win32Window {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	nw, err := testhook.Win32NewWindow("fs-test", 400, 300)
	if err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	w := nw.(*Win32Window)
	t.Cleanup(func() { procTestDestroyWindow.Call(w.HWND()) })
	return w
}

// testMsg — MSG Win32.
type testMsg struct {
	hwnd    uintptr
	message uint32
	wparam  uintptr
	lparam  uintptr
	time    uint32
	x, y    int32
}

// pumpWin32 разбирает очередь сообщений потока окна (тест — поток окна).
func pumpWin32() {
	var m testMsg
	for {
		if r, _, _ := procTestPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1); r == 0 {
			return
		}
		procTestDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// Курсор окна w: показанный дескриптор, спрятан ли, сколько создано из картинок.
func w32CursorHandle(w *Win32Window) uintptr { h, _, _ := testhook.Win32Cursor(w); return h }

func w32CursorHidden(w *Win32Window) bool { _, hidden, _ := testhook.Win32Cursor(w); return hidden }

func w32CursorsCreated(w *Win32Window) int { _, _, n := testhook.Win32Cursor(w); return n }
