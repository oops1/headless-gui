//go:build windows

package core

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

var procTestSendMessageW = testUser32.NewProc("SendMessageW")

// Путь клиента доступности целиком, на живом окне Win32: WM_GETOBJECT с
// UiaRootObjectId доходит до моста через обработчик бэкенда
// (win32.AddMessageHook) и возвращает корневой провайдер; без моста окна
// отвечает DefWindowProc (0). Перенос моста в другой пакет не должен потерять
// регистрацию обработчика — этот тест её и сторожит.
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
	win.startAccessibility()
	if win.a11y == nil {
		t.Skip("UI Automation в системе недоступна")
	}
	defer win.stopAccessibility()
	if r := send(); r == 0 {
		t.Fatal("мост не ответил на WM_GETOBJECT корневым провайдером")
	}
}
