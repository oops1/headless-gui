//go:build windows

package core

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Курсор-картинка (Engine.SetCursorImage) над настоящими окнами-попапами
// Win32: и когда задан после открытия попапа, и когда до.

func TestWin32PopupHost_CursorImage(t *testing.T) {
	carrier := newFsWindow(t)
	eng := &ciPopupEngine{}
	h := newPopupHost(carrier, carrier, eng, 1, nil)
	t.Cleanup(func() { h.closeAll(); fsPumpMessages() })

	pop := pcWin32Popup(t, h, 1)
	ci := ciImage(color.NRGBA{R: 255, A: 255})
	eng.img, eng.pcEngine.set, eng.pcEngine.cur = ci, true, widget.CursorArrow
	h.syncCursorOverride()
	fsPumpMessages()
	if pop.cursorHandle == 0 || pop.imgCreated != 1 {
		t.Fatalf("попап не получил курсор-картинку: handle=%#x создано=%d", pop.cursorHandle, pop.imgCreated)
	}
	img := pop.cursorHandle

	// Снятие возвращает системную стрелку, картинка остаётся в кэше окна.
	eng.img, eng.pcEngine.set = nil, false
	h.syncCursorOverride()
	fsPumpMessages()
	arrow, _ := pop.cursorFor(int(widget.CursorArrow))
	if pop.cursorHandle != arrow {
		t.Errorf("после снятия handle=%#x, ждал стрелку %#x", pop.cursorHandle, arrow)
	}
	eng.img, eng.pcEngine.set = ci, true
	h.syncCursorOverride()
	fsPumpMessages()
	if pop.cursorHandle != img || pop.imgCreated != 1 {
		t.Errorf("повторная картинка не из кэша: handle=%#x создано=%d", pop.cursorHandle, pop.imgCreated)
	}
}

// Картинка уже действует, когда открывается попап: окно получает её при
// создании, а не после следующей смены.
func TestWin32PopupHost_CursorImageBeforeOpen(t *testing.T) {
	carrier := newFsWindow(t)
	eng := &ciPopupEngine{}
	eng.img, eng.pcEngine.set = ciImage(color.NRGBA{G: 255, A: 255}), true
	h := newPopupHost(carrier, carrier, eng, 1, nil)
	t.Cleanup(func() { h.closeAll(); fsPumpMessages() })

	h.syncCursorOverride() // попапов ещё нет — хост лишь запоминает
	pop := pcWin32Popup(t, h, 1)
	if pop.cursorHandle == 0 || pop.imgCreated != 1 {
		t.Errorf("попап, открытый под картинкой, её не показывает: handle=%#x создано=%d", pop.cursorHandle, pop.imgCreated)
	}
}

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
