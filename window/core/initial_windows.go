package core

// initial_windows.go — начальное положение и «развёрнуто» на Win32 (initial.go).
//
// Само решение (стиль, команда показа, место) принимает чистая функция
// native.Win32InitialCreate, а Create и createInternal ей пользуются.

import (
	"unsafe"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// setInitialPlacement запоминает положение и «развёрнуто» до Create.
// Реализует initialPlaceBackend.
func (w *Win32Window) setInitialPlacement(p native.InitialPlacement) { w.initial = p }

// clientSize — размер клиентской области, каким его назначила система.
// Реализует clientSizer: окно, показанное развёрнутым, получает свой размер
// уже внутри Create (WM_SIZE приходит раньше, чем подключён обработчик
// ресайза), и Window сверяет его с холстом сразу после Create. Свёрнутое окно
// размера не сообщает.
func (w *Win32Window) clientSize() (int, int) {
	if w.hwnd == 0 {
		return 0, 0
	}
	if iconic, _, _ := procIsIconic.Call(uintptr(w.hwnd)); iconic != 0 {
		return 0, 0
	}
	var r rect
	procGetClientRect.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&r)))
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}
