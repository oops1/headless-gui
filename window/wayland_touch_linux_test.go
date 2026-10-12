//go:build linux && !android

package window

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// drainAny читает всё, что окно отправило; тишина — законный исход.
func (c *wlTestConn) drainAny(t *testing.T) []byte {
	t.Helper()
	var out []byte
	buf := make([]byte, 8192)
	for {
		c.peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := c.peer.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			return out
		}
	}
}

// wl_touch берётся у сиденья только по просьбе окна: приложение, не просившее
// касаний, остаётся на указателе, как и раньше.

func TestWaylandTouch_NotBoundUnlessRequested(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.handleEvent(c.w.seatID, wlSeatEvCapabilities, wlU32(seatCapPointer|seatCapKeyboard|seatCapTouch))
	msgs := splitWlMsgs(t, c.drainAny(t))
	if _, ok := findWlMsg(msgs, c.w.seatID, wlSeatGetTouch); ok {
		t.Error("wl_touch привязан без просьбы окна")
	}
	if c.w.touch.id.Load() != 0 {
		t.Error("объект wl_touch заведён без просьбы")
	}
}

func TestWaylandTouch_BoundWhenRequestedAndAdvertised(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.SetTouchEnabled(true) // до создания окна: только запоминается
	c.w.handleEvent(c.w.seatID, wlSeatEvCapabilities, wlU32(seatCapPointer|seatCapKeyboard|seatCapTouch))
	msgs := splitWlMsgs(t, c.drainAny(t))
	m, ok := findWlMsg(msgs, c.w.seatID, wlSeatGetTouch)
	if !ok || len(m.args) != 1 || m.args[0] != c.w.touch.id.Load() || m.args[0] == 0 {
		t.Fatalf("get_touch: %+v ok=%v, id=%d", m, ok, c.w.touch.id.Load())
	}

	// Повторное объявление возможностей второй wl_touch не заводит.
	c.w.handleEvent(c.w.seatID, wlSeatEvCapabilities, wlU32(seatCapPointer|seatCapKeyboard|seatCapTouch))
	msgs = splitWlMsgs(t, c.drainAny(t))
	if _, ok := findWlMsg(msgs, c.w.seatID, wlSeatGetTouch); ok {
		t.Error("wl_touch привязан дважды")
	}
}

func TestWaylandTouch_NotBoundWithoutCapability(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.SetTouchEnabled(true)
	c.w.handleEvent(c.w.seatID, wlSeatEvCapabilities, wlU32(seatCapPointer|seatCapKeyboard))
	msgs := splitWlMsgs(t, c.drainAny(t))
	if _, ok := findWlMsg(msgs, c.w.seatID, wlSeatGetTouch); ok {
		t.Error("wl_touch привязан, хотя сенсорного устройства нет")
	}
	// Сенсорный экран появился позже: берём его тогда.
	c.w.handleEvent(c.w.seatID, wlSeatEvCapabilities, wlU32(seatCapPointer|seatCapKeyboard|seatCapTouch))
	msgs = splitWlMsgs(t, c.drainAny(t))
	if _, ok := findWlMsg(msgs, c.w.seatID, wlSeatGetTouch); !ok {
		t.Error("сенсорное устройство появилось, а wl_touch не взят")
	}
}

// Просьба пришла позже, чем возможности сиденья: берём сразу.
func TestWaylandTouch_EnabledAfterCapabilities(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.handleEvent(c.w.seatID, wlSeatEvCapabilities, wlU32(seatCapPointer|seatCapKeyboard|seatCapTouch))
	c.drainAny(t)
	c.w.SetTouchEnabled(true)
	msgs := splitWlMsgs(t, c.drainAny(t))
	if _, ok := findWlMsg(msgs, c.w.seatID, wlSeatGetTouch); !ok {
		t.Error("включение на лету не привязало wl_touch")
	}
}

// События объекта wl_touch идут через handleEvent в приёмник окна.
func TestWaylandTouch_EventsReachWindow(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.SetTouchEnabled(true)
	c.w.handleEvent(c.w.seatID, wlSeatEvCapabilities, wlU32(seatCapPointer|seatCapKeyboard|seatCapTouch))
	c.drainAny(t)
	tid := c.w.touch.id.Load()

	var got []widget.TouchEvent
	c.w.SetOnTouch(func(ev widget.TouchEvent) { got = append(got, ev) })

	c.w.handleEvent(tid, wlTouchEvDown, touchDownBody(5, 100, c.w.surfaceID, 4, 30, 40))
	c.w.handleEvent(tid, wlTouchEvMotion, touchMotionBody(116, 4, 33, 41))
	c.w.handleEvent(tid, wlTouchEvFrame, nil)
	c.w.handleEvent(tid, wlTouchEvUp, touchUpBody(6, 130, 4))
	if len(got) != 3 || got[0].Phase != widget.TouchBegan || got[2].Phase != widget.TouchEnded || got[2].X != 33 {
		t.Fatalf("события окна: %+v", got)
	}
	if c.w.inputSerial.Load() != 6 {
		t.Errorf("serial ввода %d, ждал 6", c.w.inputSerial.Load())
	}

	// Окно выключило касания: события молча отбрасываются.
	c.w.SetTouchEnabled(false)
	got = nil
	c.w.handleEvent(tid, wlTouchEvDown, touchDownBody(7, 200, c.w.surfaceID, 5, 1, 1))
	if len(got) != 0 {
		t.Errorf("выключенные касания доставлены: %+v", got)
	}
}

// Сенсорное устройство пропало: активные касания отменяются, wl_touch отпущен.
func TestWaylandTouch_CapabilityLost(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.SetTouchEnabled(true)
	c.w.handleEvent(c.w.seatID, wlSeatEvCapabilities, wlU32(seatCapPointer|seatCapKeyboard|seatCapTouch))
	c.drainAny(t)
	tid := c.w.touch.id.Load()
	var got []widget.TouchEvent
	c.w.SetOnTouch(func(ev widget.TouchEvent) { got = append(got, ev) })
	c.w.handleEvent(tid, wlTouchEvDown, touchDownBody(5, 100, c.w.surfaceID, 4, 30, 40))

	c.w.handleEvent(c.w.seatID, wlSeatEvCapabilities, wlU32(seatCapPointer|seatCapKeyboard))
	if n := len(got); n != 2 || got[1].Phase != widget.TouchCancelled {
		t.Errorf("события: %+v", got)
	}
	msgs := splitWlMsgs(t, c.drainAny(t))
	if _, ok := findWlMsg(msgs, tid, wlTouchRelease); !ok {
		t.Error("wl_touch не отпущен")
	}
	if c.w.touch.id.Load() != 0 {
		t.Error("объект wl_touch остался за окном")
	}
}

// Палец на попапе: через настоящий реестр попапов окна.
func TestWaylandTouch_PopupSurface(t *testing.T) {
	c := newWlTestWindow(t)
	if err := c.w.OpenChildPopup(1, 10, 10, 80, 60); err != nil {
		t.Fatalf("OpenChildPopup: %v", err)
	}
	p := c.w.popups.byID[1]
	c.drainAny(t)
	c.w.SetTouchEnabled(true)
	c.w.handleEvent(c.w.seatID, wlSeatEvCapabilities, wlU32(seatCapPointer|seatCapKeyboard|seatCapTouch))
	c.drainAny(t)
	tid := c.w.touch.id.Load()

	var moves, clicks int
	c.w.SetChildPopupHandlers(childPopupHandlers{
		Move:   func(id uintptr, x, y int) { moves++ },
		Button: func(id uintptr, x, y, b int, pressed bool) { clicks++ },
	})
	var toWindow int
	c.w.SetOnTouch(func(widget.TouchEvent) { toWindow++ })

	c.w.handleEvent(tid, wlTouchEvDown, touchDownBody(5, 0, p.surfaceID, 0, 20, 20))
	c.w.handleEvent(tid, wlTouchEvUp, touchUpBody(6, 10, 0))
	if clicks != 2 || moves == 0 {
		t.Errorf("мышь попапа: движений %d, кнопок %d", moves, clicks)
	}
	if toWindow != 0 {
		t.Errorf("касание попапа ушло окну: %d", toWindow)
	}
}
