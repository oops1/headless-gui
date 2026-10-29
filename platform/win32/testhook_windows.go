//go:build windows

package win32

// testhook_windows.go — внутренности окна для интеграционных тестов
// window/core (internal/testhook): ядро и живое окно Win32 проверяются
// вместе, а открытого API ради этого бэкенд не заводит.

import (
	"image"
	"runtime"
	"unsafe"

	"github.com/oops1/headless-gui/v3/internal/testhook"
	"golang.org/x/sys/windows"
)

func init() {
	testhook.Win32NewWindow = func(title string, width, height int) (any, error) {
		w := &Win32Window{noQuit: true}
		if err := w.Create(title, width, height); err != nil {
			return nil, err
		}
		return w, nil
	}
	testhook.Win32SetNoQuit = func(nw any) { nw.(*Win32Window).noQuit = true }
	testhook.Win32WndProc = wndProc
	testhook.Win32WindowRect = func(nw any) image.Rectangle { return nw.(*Win32Window).windowRect() }
	testhook.Win32Cursor = func(nw any) (uintptr, bool, int) {
		w := nw.(*Win32Window)
		return w.cursorHandle, w.cursorHidden, w.imgCreated
	}
	testhook.Win32CursorFor = func(nw any, c int) uintptr {
		h, _ := nw.(*Win32Window).cursorFor(c)
		return h
	}
	testhook.Win32SetCursorMessage = func(nw any, hit uintptr) (uintptr, bool) {
		return nw.(*Win32Window).setCursorMessage(hit)
	}
	testhook.Win32AfterResize = func(nw any, after func(w, h int)) {
		w := nw.(*Win32Window)
		inner := w.onResize
		w.onResize = func(width, height int) {
			after(width, height)
			if inner != nil {
				inner(width, height)
			}
		}
	}
	testhook.Win32SettingWatched = func(nw any) (bool, bool) {
		w := nw.(*Win32Window)
		return w.onSettingChange.Load() != nil, w.onAccentChange.Load() != nil
	}
	testhook.Win32SettingChange = func(nw any, area string) {
		s, err := windows.UTF16PtrFromString(area)
		if err != nil {
			return
		}
		nw.(*Win32Window).handleSettingChange(uintptr(unsafe.Pointer(s)))
		runtime.KeepAlive(s)
	}
}
