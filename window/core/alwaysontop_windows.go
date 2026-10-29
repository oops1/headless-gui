package core

// Win32: окно «поверх всех» — WS_EX_TOPMOST, который ставит и снимает
// SetWindowPos с HWND_TOPMOST / HWND_NOTOPMOST (alwaysontop.go).

const (
	hwndTopmost   = ^uintptr(0)              // HWND_TOPMOST   = (HWND)-1
	hwndNotopmost = ^uintptr(1)              // HWND_NOTOPMOST = (HWND)-2
	gwlExstyleAOT = ^uintptr(19)             // GWL_EXSTYLE   = -20
	swpAOTFlags   = 0x0001 | 0x0002 | 0x0010 // SWP_NOSIZE | SWP_NOMOVE | SWP_NOACTIVATE
)

// SetAlwaysOnTop реализует alwaysOnTopBackend.
func (w *Win32Window) SetAlwaysOnTop(on bool) bool {
	if w.hwnd == 0 {
		return false
	}
	after := hwndNotopmost
	if on {
		after = hwndTopmost
	}
	r, _, _ := procSetWindowPos.Call(uintptr(w.hwnd), after, 0, 0, 0, 0, swpAOTFlags)
	return r != 0
}

// IsAlwaysOnTop реализует alwaysOnTopBackend: WS_EX_TOPMOST в стиле окна.
func (w *Win32Window) IsAlwaysOnTop() bool {
	if w.hwnd == 0 {
		return false
	}
	ex, _, _ := procGetWindowLongPtrW.Call(uintptr(w.hwnd), gwlExstyleAOT)
	return ex&wsExTopmost != 0
}
