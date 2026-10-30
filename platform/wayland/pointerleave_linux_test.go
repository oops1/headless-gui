//go:build linux && !android

package wayland

import (
	"encoding/binary"
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/pointer"
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
	if len(got) != 1 || got[0] != [2]int{pointer.Outside, pointer.Outside} {
		t.Errorf("движения после leave: %v", got)
	}
}

// С попапа — движение за попап уходит его обработчику, окну — ничего.
func TestWayland_PointerLeavePopup(t *testing.T) {
	c := newWlTestWindow(t)
	var main, popup int
	c.w.SetOnMouseMove(func(x, y int) { main++ })
	c.w.SetChildPopupHandlers(native.ChildPopupHandlers{Move: func(id uintptr, x, y int) {
		if id == 7 && x == pointer.Outside && y == pointer.Outside {
			popup++
		}
	}})
	c.w.ptrOnPopup, c.w.ptrPopupID = true, 7
	c.w.handleEvent(c.w.pointerID, wlPointerEvLeave, wlLeaveBody(99))
	if popup != 1 || main != 0 {
		t.Errorf("попап получил %d, окно %d", popup, main)
	}
}
