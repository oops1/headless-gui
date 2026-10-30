//go:build windows

package core

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testhook"
	"github.com/oops1/headless-gui/v4/widget"
)

// Курсор-картинка (Engine.SetCursorImage) над настоящими окнами-попапами
// Win32: и когда задан после открытия попапа, и когда до.

func TestWin32PopupHost_CursorImage(t *testing.T) {
	carrier := newWin32TestWindow(t)
	eng := &ciPopupEngine{}
	h := newPopupHost(carrier, carrier, eng, 1, nil)
	t.Cleanup(func() { h.closeAll(); pumpWin32() })

	pop := pcWin32Popup(t, h, 1)
	ci := ciImage(color.NRGBA{R: 255, A: 255})
	eng.img, eng.pcEngine.set, eng.pcEngine.cur = ci, true, widget.CursorArrow
	h.syncCursorOverride()
	pumpWin32()
	if w32CursorHandle(pop) == 0 || w32CursorsCreated(pop) != 1 {
		t.Fatalf("попап не получил курсор-картинку: handle=%#x создано=%d", w32CursorHandle(pop), w32CursorsCreated(pop))
	}
	img := w32CursorHandle(pop)

	// Снятие возвращает системную стрелку, картинка остаётся в кэше окна.
	eng.img, eng.pcEngine.set = nil, false
	h.syncCursorOverride()
	pumpWin32()
	arrow := testhook.Win32CursorFor(pop, int(widget.CursorArrow))
	if w32CursorHandle(pop) != arrow {
		t.Errorf("после снятия handle=%#x, ждал стрелку %#x", w32CursorHandle(pop), arrow)
	}
	eng.img, eng.pcEngine.set = ci, true
	h.syncCursorOverride()
	pumpWin32()
	if w32CursorHandle(pop) != img || w32CursorsCreated(pop) != 1 {
		t.Errorf("повторная картинка не из кэша: handle=%#x создано=%d", w32CursorHandle(pop), w32CursorsCreated(pop))
	}
}

// Картинка уже действует, когда открывается попап: окно получает её при
// создании, а не после следующей смены.
func TestWin32PopupHost_CursorImageBeforeOpen(t *testing.T) {
	carrier := newWin32TestWindow(t)
	eng := &ciPopupEngine{}
	eng.img, eng.pcEngine.set = ciImage(color.NRGBA{G: 255, A: 255}), true
	h := newPopupHost(carrier, carrier, eng, 1, nil)
	t.Cleanup(func() { h.closeAll(); pumpWin32() })

	h.syncCursorOverride() // попапов ещё нет — хост лишь запоминает
	pop := pcWin32Popup(t, h, 1)
	if w32CursorHandle(pop) == 0 || w32CursorsCreated(pop) != 1 {
		t.Errorf("попап, открытый под картинкой, её не показывает: handle=%#x создано=%d", w32CursorHandle(pop), w32CursorsCreated(pop))
	}
}
