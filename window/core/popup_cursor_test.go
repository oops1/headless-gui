package core

import (
	"fmt"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Принудительный курсор (Engine.SetCursorOverride) и вынесенные попапы: попап —
// отдельное окно ОС со своим курсором, форма окна-носителя до него не
// доходила. Хосты попапов (popupHost — Win32/X11, childPopups — Wayland)
// переносят её на свои окна.

// pcPopup — окно ОС попапа: SetCursor записывает формы, вызовы потока окна
// выполняются сразу.
type pcPopup struct {
	popupFakeNative
	shapes []int
}

func (f *pcPopup) SetCursor(c int)            { f.shapes = append(f.shapes, c) }
func (f *pcPopup) InvokeOnUIThread(fn func()) { fn() }

// pcEngine — движок попапов с принудительным курсором.
type pcEngine struct {
	fakePopupEngine
	cur widget.Cursor
	set bool
}

func (e *pcEngine) CursorOverride() (widget.Cursor, bool) { return e.cur, e.set }

func pcHost(t *testing.T, n int) (*popupHost, *pcEngine, []*pcPopup) {
	t.Helper()
	eng := &pcEngine{}
	carrier := &pcPopup{}
	h := newPopupHost(carrier, carrier, eng, 1, nil)
	var pops []*pcPopup
	for i := 0; i < n; i++ {
		p := &pcPopup{}
		h.windows[uintptr(i+1)] = &hostedPopup{native: p, rect: image.Rect(0, 0, 10, 10)}
		pops = append(pops, p)
	}
	return h, eng, pops
}

// Заданная форма уходит всем окнам-попапам, снятая возвращает стрелку, повтор
// того же значения окон не беспокоит.
func TestPopupHost_CursorOverrideReachesPopups(t *testing.T) {
	h, eng, pops := pcHost(t, 2)

	h.syncCursorOverride() // override не задан — окна не трогаем
	for i, p := range pops {
		if len(p.shapes) != 0 {
			t.Fatalf("попап %d получил форму без override: %v", i, p.shapes)
		}
	}

	eng.cur, eng.set = widget.CursorNone, true
	h.syncCursorOverride()
	h.syncCursorOverride() // то же значение
	for i, p := range pops {
		if len(p.shapes) != 1 || p.shapes[0] != int(widget.CursorNone) {
			t.Errorf("попап %d: формы %v, ждал [None]", i, p.shapes)
		}
	}

	eng.cur = widget.CursorGrabbing // сменили форму
	h.syncCursorOverride()
	eng.set = false // сняли
	h.syncCursorOverride()
	for i, p := range pops {
		want := []int{int(widget.CursorNone), int(widget.CursorGrabbing), int(widget.CursorArrow)}
		if len(p.shapes) != 3 || p.shapes[1] != want[1] || p.shapes[2] != want[2] {
			t.Errorf("попап %d: формы %v, ждал %v", i, p.shapes, want)
		}
	}
}

// Окно, которого ещё нет (поднимается), форму не получает, но хост её помнит:
// createPopup ставит запомненное после создания окна.
func TestPopupHost_CursorOverrideRemembered(t *testing.T) {
	h, eng, _ := pcHost(t, 0)
	h.windows[9] = &hostedPopup{} // native == nil: окно ещё поднимается
	eng.cur, eng.set = widget.CursorHand, true
	h.syncCursorOverride()
	h.mu.Lock()
	c, set := h.ovrCursor, h.ovrSet
	h.mu.Unlock()
	if !set || c != widget.CursorHand {
		t.Errorf("хост не запомнил override: %v %v", c, set)
	}
}

// От движка до окна-попапа целиком: смена override → surface.refreshCursor →
// хост попапов.
func TestPopupHost_CursorOverrideFromSurface(t *testing.T) {
	oe := &overrideEngine{queueEngine: &queueEngine{}}
	fn := &inputNative{}
	s := &surface{eng: oe, native: fn, scale: 1}
	s.setupInput()

	peng := &pcEngine{}
	carrier := &pcPopup{}
	h := newPopupHost(carrier, carrier, peng, 1, nil)
	pop := &pcPopup{}
	h.windows[1] = &hostedPopup{native: pop}
	s.setPopupCursor(h.syncCursorOverride)

	peng.cur, peng.set = widget.CursorGrabbing, true
	oe.set(widget.CursorGrabbing)
	pump(oe.queueEngine, fn)
	if len(pop.shapes) != 1 || pop.shapes[0] != int(widget.CursorGrabbing) {
		t.Fatalf("попап получил %v, ждал [Grabbing]", pop.shapes)
	}

	peng.set = false
	oe.clear()
	pump(oe.queueEngine, fn)
	if len(pop.shapes) != 2 || pop.shapes[1] != int(widget.CursorArrow) {
		t.Fatalf("после снятия попап получил %v, ждал возврат стрелки", pop.shapes)
	}
}

// ─── Попапы-потомки (Wayland) ────────────────────────────────────────────────

// pcChildHost — бэкенд попапов-потомков с курсором.
type pcChildHost struct {
	fakeChildHost
	calls []string
}

func (f *pcChildHost) SetPopupCursor(c int, set bool) {
	if set {
		f.calls = append(f.calls, fmt.Sprintf("set %d", c))
	} else {
		f.calls = append(f.calls, "clear")
	}
}

func TestChildPopups_CursorOverride(t *testing.T) {
	be := &pcChildHost{}
	eng := &pcEngine{}
	c := newChildPopups(be, eng, 1, nil)

	c.syncCursorOverride() // ничего не задано — бэкенд не беспокоим
	if len(be.calls) != 0 {
		t.Fatalf("лишний вызов: %v", be.calls)
	}
	eng.cur, eng.set = widget.CursorNone, true
	c.syncCursorOverride()
	c.syncCursorOverride()
	eng.set = false
	c.syncCursorOverride()
	if len(be.calls) != 2 || be.calls[0] != fmt.Sprintf("set %d", widget.CursorNone) || be.calls[1] != "clear" {
		t.Errorf("вызовы %v, ждал [set None, clear]", be.calls)
	}

	// Попап открылся под действующим override: бэкенд получает форму, и
	// повторно — только при следующей смене.
	eng.cur, eng.set = widget.CursorHand, true
	be.calls = nil
	c.apply([]engine.PopupFrame{popupFrame(1, image.Rect(0, 0, 50, 40), 1)})
	if len(be.calls) != 1 || be.calls[0] != fmt.Sprintf("set %d", widget.CursorHand) {
		t.Errorf("после открытия попапа вызовы %v, ждал [set Hand]", be.calls)
	}
	c.apply([]engine.PopupFrame{popupFrame(1, image.Rect(0, 0, 50, 40), 1)})
	if len(be.calls) != 1 {
		t.Errorf("кадр без изменений снова трогает курсор: %v", be.calls)
	}
}

// Бэкенд без поддержки курсора над попапами и движок без override не мешают.
func TestChildPopups_CursorOverrideOptional(t *testing.T) {
	c := newChildPopups(&fakeChildHost{}, &pcEngine{cur: widget.CursorNone, set: true}, 1, nil)
	c.syncCursorOverride() // бэкенд без SetPopupCursor — не паникует
	c = newChildPopups(&pcChildHost{}, &fakePopupEngine{}, 1, nil)
	c.syncCursorOverride() // движок без CursorOverride — не паникует
}
