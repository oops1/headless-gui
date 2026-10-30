//go:build windows

package win32

import (
	"image/color"
	"testing"
)

// Картинки крупных размеров строятся: 384×384 — предел RDP, 512 — наш предел.
func TestWin32CursorImage_LargeSizes(t *testing.T) {
	w := newFsWindow(t)
	for _, side := range []int{32, 128, 384, 512} {
		ci := ciwImage(side, side, color.NRGBA{R: 10, G: 20, B: 30, A: 200})
		w.SetCursorImage(ci)
		if w.cursorHandle == 0 {
			t.Errorf("%d×%d: курсор не построен", side, side)
		}
	}
}
