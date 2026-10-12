package window

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Принудительный курсор движка (Engine.SetCursorOverride) должен дойти до окна
// ОС сразу, без движения мыши: иначе курсор не спрятать по таймеру
// бездействия — мышь-то как раз никто не двигает.

// overrideEngine — queueEngine с принудительным курсором и уведомлением о его
// смене, как у *engine.Engine.
type overrideEngine struct {
	*queueEngine
	ovr  widget.Cursor
	has  bool
	hook func()
}

func (e *overrideEngine) CursorAt(x, y int) widget.Cursor {
	if e.has {
		return e.ovr
	}
	return e.queueEngine.CursorAt(x, y)
}

func (e *overrideEngine) SetOnCursorOverrideChanged(fn func()) { e.hook = fn }

// set/clear имитируют Engine.SetCursorOverride / ClearCursorOverride.
func (e *overrideEngine) set(c widget.Cursor) {
	if e.has && e.ovr == c {
		return
	}
	e.ovr, e.has = c, true
	e.hook()
}

func (e *overrideEngine) clear() {
	if !e.has {
		return
	}
	e.has = false
	e.hook()
}

// pump доигрывает очередь движка и вызовы потока окна, пока обе не опустеют.
func pump(eng *queueEngine, fn *inputNative) {
	for {
		n := eng.run()
		inv := fn.invoked
		fn.invoked = nil
		for _, f := range inv {
			f()
		}
		if n == 0 && len(inv) == 0 {
			return
		}
	}
}

func overrideSurface(t *testing.T) (*surface, *inputNative, *overrideEngine) {
	t.Helper()
	eng := &overrideEngine{queueEngine: &queueEngine{}}
	fn := &inputNative{}
	s := &surface{eng: eng, native: fn, scale: 1}
	s.setupInput()
	if eng.hook == nil {
		t.Fatal("окно не подписалось на смену принудительного курсора")
	}
	return s, fn, eng
}

func TestCursorOverride_AppliedWithoutMouseMove(t *testing.T) {
	_, fn, eng := overrideSurface(t)

	fn.onMove(60, 10) // над «рукой» (queueEngine: x>=50 → 2)
	pump(eng.queueEngine, fn)
	if got := fn.cursors; len(got) != 1 || got[0] != 2 {
		t.Fatalf("курсор после движения: %v, ждал [2]", got)
	}

	// Мышь стоит. Прячем курсор — окно должно получить его немедленно.
	eng.set(widget.CursorNone)
	pump(eng.queueEngine, fn)
	if got := fn.cursors; len(got) != 2 || got[1] != int(widget.CursorNone) {
		t.Fatalf("после SetCursorOverride(None) окну отдано %v", got)
	}

	// Снимаем override — возвращается курсор виджета под мышью, опять без
	// движения.
	eng.clear()
	pump(eng.queueEngine, fn)
	if got := fn.cursors; len(got) != 3 || got[2] != 2 {
		t.Fatalf("после ClearCursorOverride окну отдано %v, ждал возврат к 2", got)
	}
}

// Пока override стоит, движение мыши курсор не меняет.
func TestCursorOverride_StrongerThanMove(t *testing.T) {
	_, fn, eng := overrideSurface(t)
	eng.set(widget.CursorGrabbing)
	pump(eng.queueEngine, fn)
	fn.onMove(60, 10)
	pump(eng.queueEngine, fn)
	fn.onMove(5, 5)
	pump(eng.queueEngine, fn)
	want := []int{int(widget.CursorGrabbing)}
	if len(fn.cursors) != 1 || fn.cursors[0] != want[0] {
		t.Fatalf("курсоры %v, ждал только %v", fn.cursors, want)
	}
}

// Мыши в окне ещё не было: override всё равно применяется, а после снятия
// возвращается стрелка.
func TestCursorOverride_NoMouseYet(t *testing.T) {
	_, fn, eng := overrideSurface(t)
	eng.set(widget.CursorNone)
	pump(eng.queueEngine, fn)
	eng.clear()
	pump(eng.queueEngine, fn)
	if len(fn.cursors) != 2 || fn.cursors[0] != int(widget.CursorNone) || fn.cursors[1] != int(widget.CursorArrow) {
		t.Fatalf("курсоры %v, ждал [None Arrow]", fn.cursors)
	}
}

// Настоящий движок: SetCursorOverride из любой горутины доходит до окна через
// очередь движка; повторная та же форма окно не беспокоит.
func TestCursorOverride_RealEngine(t *testing.T) {
	eng := engine.New(200, 100, 30)
	btn := widget.NewButton("ok")
	btn.SetBounds(image.Rect(0, 0, 200, 100))
	btn.SetCursor(widget.CursorHand)
	eng.SetRoot(btn)
	eng.RenderOnce()

	fn := &inputNative{}
	s := &surface{eng: eng, native: fn, scale: 1}
	s.setupInput()

	step := func() {
		eng.RenderOnce() // без цикла кадров разбирает очередь Post
		inv := fn.invoked
		fn.invoked = nil
		for _, f := range inv {
			f()
		}
	}

	fn.onMove(10, 10)
	step()
	if len(fn.cursors) != 1 || fn.cursors[0] != int(widget.CursorHand) {
		t.Fatalf("курсор над кнопкой: %v", fn.cursors)
	}
	eng.SetCursorOverride(widget.CursorNone)
	step()
	eng.SetCursorOverride(widget.CursorNone) // та же форма — уведомления нет
	step()
	if len(fn.cursors) != 2 || fn.cursors[1] != int(widget.CursorNone) {
		t.Fatalf("после override: %v", fn.cursors)
	}
	eng.ClearCursorOverride()
	step()
	if len(fn.cursors) != 3 || fn.cursors[2] != int(widget.CursorHand) {
		t.Fatalf("после снятия override: %v", fn.cursors)
	}
}
