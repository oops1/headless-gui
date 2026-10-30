//go:build linux && !android

package wayland

import (
	"testing"

	"github.com/oops1/headless-gui/v4/platform/pointer"
)

// Wayland: скрытие курсора, новые формы и боковые кнопки — на проводе.

func wlCursorTestWindow(t *testing.T) *wlTestConn {
	t.Helper()
	c := newWlTestWindow(t)
	c.w.ptrEnterSerial.Store(77)
	return c
}

// CursorNone: wl_pointer.set_cursor(serial, NULL, 0, 0).
func TestWayland_CursorNoneHides(t *testing.T) {
	c := wlCursorTestWindow(t)
	c.w.SetCursor(pointer.CursorNone)
	msgs := splitWlMsgs(t, c.read(t))
	m, ok := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if !ok {
		t.Fatalf("set_cursor не отправлен: %v", msgs)
	}
	if len(m.args) != 4 || m.args[0] != 77 || m.args[1] != 0 || m.args[2] != 0 || m.args[3] != 0 {
		t.Fatalf("set_cursor %v, ждал [serial=77 surface=NULL 0 0]", m.args)
	}
}

// Курсор спрятан и при cursor-shape-v1: у него формы «нет», прятать надо
// пустой поверхностью, а не set_shape.
func TestWayland_CursorNoneHidesEvenWithCursorShape(t *testing.T) {
	c := wlCursorTestWindow(t)
	c.w.gCursorShapeMgr = 30
	c.w.SetCursor(pointer.CursorNone)
	msgs := splitWlMsgs(t, c.read(t))
	m, ok := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if !ok || m.args[1] != 0 {
		t.Fatalf("курсор не спрятан set_cursor(NULL): %v", msgs)
	}
	if c.w.cursorShapeDevID != 0 {
		t.Error("для скрытия не нужно связывать cursor-shape")
	}
}

// Курсор спрятан, указатель в окне ещё не было: форма запоминается и
// применится на enter.
func TestWayland_CursorNoneAppliedOnEnter(t *testing.T) {
	c := newWlTestWindow(t) // enter-serial ещё нет
	c.w.SetCursor(pointer.CursorNone)
	c.w.ptrEnterSerial.Store(5)
	c.w.applyCursor()
	msgs := splitWlMsgs(t, c.read(t))
	m, ok := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if !ok || m.args[0] != 5 || m.args[1] != 0 {
		t.Fatalf("после enter курсор не спрятан: %v", msgs)
	}
}

func TestWlHideCursorMsg(t *testing.T) {
	b := wlHideCursorMsg(14, 9).bytes()
	msgs := splitWlMsgs(t, b)
	if len(msgs) != 1 || msgs[0].obj != 14 || msgs[0].opcode != wlPointerSetCursor {
		t.Fatalf("сообщение %v", msgs)
	}
	if got := msgs[0].args; len(got) != 4 || got[0] != 9 || got[1] != 0 {
		t.Fatalf("аргументы %v", got)
	}
}

// С cursor-shape-v1 новые формы просятся по имени.
func TestWayland_NewShapesViaCursorShape(t *testing.T) {
	cases := []struct {
		c     int
		shape uint32
	}{
		{pointer.CursorGrab, 16},
		{pointer.CursorGrabbing, 17},
		{pointer.CursorSizeAll, 33}, // all-scroll
		{pointer.CursorCrosshair, 8},
		{pointer.CursorWait, 6},
	}
	for _, tc := range cases {
		c := wlCursorTestWindow(t)
		c.w.gCursorShapeMgr = 30
		c.w.SetCursor(tc.c)
		msgs := splitWlMsgs(t, c.read(t))
		m, ok := findWlMsg(msgs, c.w.cursorShapeDevID, wpCursorShapeDevSetShape)
		if !ok {
			t.Fatalf("форма %d: set_shape не отправлен: %v", tc.c, msgs)
		}
		if len(m.args) != 2 || m.args[0] != 77 || m.args[1] != tc.shape {
			t.Errorf("форма %d: set_shape %v, ждал [77 %d]", tc.c, m.args, tc.shape)
		}
	}
}

// Без cursor-shape новые формы показываются своими картинками (не больше
// 32×32), скрытие — тем же set_cursor.
func TestWayland_NewShapesOwnSurface(t *testing.T) {
	for _, shape := range []int{pointer.CursorGrab, pointer.CursorGrabbing, pointer.CursorSizeAll, pointer.CursorCrosshair, pointer.CursorWait} {
		pix, w, h, hx, hy := pointer.CursorPixels(shape)
		if w > 32 || h > 32 || len(pix) != w*h*4 {
			t.Errorf("форма %d: картинка %dx%d, %d байт", shape, w, h, len(pix))
		}
		if hx < 0 || hx >= w || hy < 0 || hy >= h {
			t.Errorf("форма %d: горячая точка (%d,%d) вне картинки", shape, hx, hy)
		}
		opaque := false
		for i := 3; i < len(pix); i += 4 {
			if pix[i] != 0 {
				opaque = true
				break
			}
		}
		if !opaque {
			t.Errorf("форма %d: пустая картинка", shape)
		}
	}
	// Пул рассчитан на все формы: слот последней формы укладывается в память.
	if pointer.CursorCount != 13 {
		t.Errorf("pointer.CursorCount = %d, ждал 13", pointer.CursorCount)
	}
}

// Боковые кнопки Wayland: BTN_SIDE/BTN_BACK → X1, BTN_EXTRA/BTN_FORWARD → X2.
func TestWayland_SideButtons(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.ptrX, c.w.ptrY = 12, 34
	var got []mouseCallLinux
	c.w.onMouseButton = func(x, y, b int, p bool) { got = append(got, mouseCallLinux{x, y, b, p}) }
	mk := func(button, state uint32) []byte {
		return wlU32(1, 0, button, state)
	}
	c.w.handlePointer(wlPointerEvButton, mk(pointer.WlBtnSide, 1))
	c.w.handlePointer(wlPointerEvButton, mk(pointer.WlBtnSide, 0))
	c.w.handlePointer(wlPointerEvButton, mk(pointer.WlBtnExtra, 1))
	c.w.handlePointer(wlPointerEvButton, mk(pointer.WlBtnExtra, 0))
	c.w.handlePointer(wlPointerEvButton, mk(pointer.WlBtnBack, 1))
	c.w.handlePointer(wlPointerEvButton, mk(pointer.WlBtnBack, 0))
	c.w.handlePointer(wlPointerEvButton, mk(pointer.WlBtnForward, 1))
	c.w.handlePointer(wlPointerEvButton, mk(pointer.WlBtnForward, 0))
	c.w.handlePointer(wlPointerEvButton, mk(0x117, 1)) // BTN_TASK — не знаем
	want := []mouseCallLinux{
		{12, 34, 5, true}, {12, 34, 5, false},
		{12, 34, 6, true}, {12, 34, 6, false},
		{12, 34, 5, true}, {12, 34, 5, false},
		{12, 34, 6, true}, {12, 34, 6, false},
	}
	if len(got) != len(want) {
		t.Fatalf("доставлено %v, ждал %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("событие %d: %v, ждал %v", i, got[i], want[i])
		}
	}
	if m := c.w.ptrButtons.Load(); m != 0 {
		t.Errorf("после отпускания в маске зажатых осталось 0x%x", m)
	}
}

// Указатель ушёл при зажатой боковой кнопке — отпускание сочиняет окно.
func TestWayland_LeaveReleasesSideButton(t *testing.T) {
	c := newWlTestWindow(t)
	var got []mouseCallLinux
	c.w.onMouseButton = func(x, y, b int, p bool) { got = append(got, mouseCallLinux{x, y, b, p}) }
	c.w.handlePointer(wlPointerEvButton, wlU32(1, 0, pointer.WlBtnExtra, 1))
	c.w.handlePointer(wlPointerEvLeave, make([]byte, 8))
	if len(got) != 2 || got[1].b != 6 || got[1].p {
		t.Fatalf("события %v: ждал нажатие и отпускание X2 при уходе", got)
	}
}
