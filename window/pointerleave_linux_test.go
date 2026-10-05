//go:build linux && !android

package window

import (
	"encoding/binary"
	"testing"
)

func wlLeaveBody(surface uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b[0:4], 1)
	binary.LittleEndian.PutUint32(b[4:8], surface)
	return b
}

// wl_pointer.leave снимает наведение: движку уходит движение за окно. Раньше
// бэкенд только отпускал кнопки, и виджет под последним местом указателя
// оставался подсвеченным.
func TestWayland_PointerLeaveMovesOutside(t *testing.T) {
	c := newWlTestWindow(t)
	var got [][2]int
	c.w.SetOnMouseMove(func(x, y int) { got = append(got, [2]int{x, y}) })
	c.w.handleEvent(c.w.pointerID, wlPointerEvLeave, wlLeaveBody(c.w.surfaceID))
	if len(got) != 1 || got[0] != [2]int{pointerOutside, pointerOutside} {
		t.Errorf("движения после leave: %v", got)
	}
}

// С попапа — движение за попап уходит его обработчику, окну — ничего.
func TestWayland_PointerLeavePopup(t *testing.T) {
	c := newWlTestWindow(t)
	var main, popup int
	c.w.SetOnMouseMove(func(x, y int) { main++ })
	c.w.SetChildPopupHandlers(childPopupHandlers{Move: func(id uintptr, x, y int) {
		if id == 7 && x == pointerOutside && y == pointerOutside {
			popup++
		}
	}})
	c.w.ptrOnPopup, c.w.ptrPopupID = true, 7
	c.w.handleEvent(c.w.pointerID, wlPointerEvLeave, wlLeaveBody(99))
	if popup != 1 || main != 0 {
		t.Errorf("попап получил %d, окно %d", popup, main)
	}
}

// X11: LeaveNotify с обычным уходом — движение за окно; в дочернее окно — нет.
func TestX11_LeaveNotifyMovesOutside(t *testing.T) {
	w, _ := newX11TestWindow(t)
	var got [][2]int
	w.onMouseMove = func(x, y int) { got = append(got, [2]int{x, y}) }
	ev := make([]byte, 32)
	ev[0] = 8 // LeaveNotify
	w.handleX11Event(ev)
	ev[1] = 2 // NotifyInferior
	w.handleX11Event(ev)
	if len(got) != 1 || got[0] != [2]int{pointerOutside, pointerOutside} {
		t.Errorf("движения после LeaveNotify: %v", got)
	}
}
