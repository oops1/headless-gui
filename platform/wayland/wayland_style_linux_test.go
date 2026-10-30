//go:build linux && !android

package wayland

import (
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// Wayland умеет только Borderless: ни зон ресайза, ни маски углов; Tool —
// честное «нет».
func TestWayland_Style_Borderless(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.ptrButtonSerial.Store(42)
	c.w.resizable.Store(true)
	c.w.SetCornerRadius(8)
	if !c.w.RoundsCorners() || !c.w.wantARGB() || c.w.maskRadius() == 0 {
		t.Fatal("обычное окно со скруглением не скругляется")
	}

	if got := c.w.ApplyStyle(native.Style{Borderless: true, Tool: true}); got != (native.Style{Borderless: true}) {
		t.Fatalf("ApplyStyle = %+v, ждал только Borderless", got)
	}
	if c.w.BeginResize(8) {
		t.Error("BeginResize у Borderless-окна = true")
	}
	if c.w.RoundsCorners() || c.w.wantARGB() || c.w.maskRadius() != 0 {
		t.Error("Borderless-окно скругляется")
	}

	if got := c.w.ApplyStyle(native.Style{}); got != (native.Style{}) {
		t.Fatalf("ApplyStyle({}) = %+v", got)
	}
	if !c.w.RoundsCorners() || !c.w.wantARGB() || c.w.maskRadius() == 0 {
		t.Error("скругление не вернулось")
	}
	if !c.w.BeginResize(8) {
		t.Error("BeginResize не вернулся")
	}
}

// Владельца, Z-порядка и положения у окна Wayland нет: ядро отвечает false.
func TestWayland_Style_NoOwnerStackPosition(t *testing.T) {
	var nw native.NativeWindow = &WaylandWindow{}
	if _, ok := nw.(native.OwnerSetter); ok {
		t.Error("окно Wayland объявило владельца: set_parent с окном другого соединения — ошибка протокола")
	}
	if _, ok := nw.(native.Stacker); ok {
		t.Error("окно Wayland объявило Z-порядок")
	}
	if _, ok := nw.(native.Positioner); ok {
		t.Error("окно Wayland объявило положение")
	}
}
