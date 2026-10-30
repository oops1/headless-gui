//go:build linux && !android

package x11

import (
	"testing"

	"github.com/oops1/headless-gui/v4/platform/pointer"
)

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
	if len(got) != 1 || got[0] != [2]int{pointer.Outside, pointer.Outside} {
		t.Errorf("движения после LeaveNotify: %v", got)
	}
}
