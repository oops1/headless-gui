package wayland

import (
	"testing"
)

// Wayland: Create фиксирует размер, и отложенный минимум обязан
// применяться ПОСЛЕ фиксации, иначе она его затрёт.
func TestWaylandSetMinSize_BeforeCreateIsRemembered(t *testing.T) {
	w := &WaylandWindow{}
	w.SetMinSize(640, 480)

	if !w.minWant || w.minW != 640 || w.minH != 480 {
		t.Errorf("минимум не отложен: want=%v %dx%d", w.minWant, w.minW, w.minH)
	}
}
