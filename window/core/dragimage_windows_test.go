//go:build windows

package core

import "testing"

// Окно на бэкенде Win32 без OLE значка перетаскивания не обещает: ядро
// спрашивает бэкенд (DragImageSupported), а не только наличие
// BeginDragFilesImage.
func TestDragImageSupported_Win32WithoutOLE(t *testing.T) {
	win := &Window{}
	win.native = &Win32Window{}
	if win.DragImageSupported() {
		t.Error("Win32 без OLE: DragImageSupported = true")
	}
}
