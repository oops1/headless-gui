//go:build windows

package a11y

import (
	"image"
	"runtime"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/internal/testhook"
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/platform/win32"
	"github.com/oops1/headless-gui/v3/widget"
	"golang.org/x/sys/windows"
)

var (
	testUser32            = windows.NewLazySystemDLL("user32.dll")
	procTestSendMessageW  = testUser32.NewProc("SendMessageW")
	procTestDestroyWindow = testUser32.NewProc("DestroyWindow")
)

// newWin32TestWindow — окно бэкенда на потоке теста; по окончании теста
// уничтожается.
func newWin32TestWindow(t *testing.T) *win32.Win32Window {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	nw, err := testhook.Win32NewWindow("a11y-test", 400, 300)
	if err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	w := nw.(*win32.Win32Window)
	t.Cleanup(func() { procTestDestroyWindow.Call(w.HWND()) })
	return w
}

// Путь клиента доступности целиком, на живом окне Win32: WM_GETOBJECT с
// UiaRootObjectId доходит до моста через обработчик бэкенда
// (win32.AddMessageHook, его ставит init этого пакета) и возвращает корневой
// провайдер; без моста окна отвечает DefWindowProc (0).
func TestUIA_LiveWMGetObject(t *testing.T) {
	t.Setenv("HEADLESS_GUI_A11Y", "1")
	nw := newWin32TestWindow(t)
	eng := engine.New(200, 100, 20)
	root := widget.NewWindow("UIA вживую", 200, 100)
	btn := widget.NewButton("Кнопка")
	btn.SetBounds(image.Rect(10, 10, 100, 40))
	root.AddChild(btn)
	eng.SetRoot(root)
	win := New(eng, "UIA вживую")
	win.native = nw
	win.scale = 1

	const rootObjectID = ^uintptr(24) // UiaRootObjectId (-25) как LPARAM
	send := func() uintptr {
		r, _, _ := procTestSendMessageW.Call(nw.HWND(), wmGetObject, 0, rootObjectID)
		return r
	}
	if r := send(); r != 0 {
		t.Fatalf("без моста WM_GETOBJECT ответил %#x, ждал DefWindowProc (0)", r)
	}
	f, ok := native.Accessibility()
	if !ok || f == nil {
		t.Fatal("init пакета не зарегистрировал доступность")
	}
	b := f(win.host())
	if err := b.Start(); err != nil {
		t.Skipf("UI Automation в системе недоступна: %v", err)
	}
	defer b.Stop()
	if r := send(); r == 0 {
		t.Fatal("мост не ответил на WM_GETOBJECT корневым провайдером")
	}
}
