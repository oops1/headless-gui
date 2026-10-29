package core

import (
	"testing"
)

// Минимум, заданный ДО создания окна, не теряется.
//
// WM читает WM_NORMAL_HINTS при показе окна, поэтому свойство обязано попасть
// в него до MapWindow. Раньше SetMinSize с нулевым wid просто выходил, а
// Window звал его уже после Create — то есть после показа.
func TestX11SetMinSize_BeforeCreateIsRemembered(t *testing.T) {
	w := &X11Window{}
	w.SetMinSize(800, 600)

	if !w.minWant || w.minW != 800 || w.minH != 600 {
		t.Fatalf("минимум не отложен: want=%v %dx%d", w.minWant, w.minW, w.minH)
	}

	// applyPendingMinSize при нулевом wid снова отложит его же — проверяем,
	// что признак снимается и повторного применения не будет.
	w.applyPendingMinSize()
	if w.minW != 800 || w.minH != 600 {
		t.Errorf("после применения запомнено %dx%d", w.minW, w.minH)
	}
}

// Ничего не задано — применять нечего.
func TestX11SetMinSize_NothingPending(t *testing.T) {
	w := &X11Window{}
	w.applyPendingMinSize()
	if w.minWant {
		t.Error("признак отложенного минимума взялся из ниоткуда")
	}
}

// То же для Wayland: Create фиксирует размер, и отложенный минимум обязан
// применяться ПОСЛЕ фиксации, иначе она его затрёт.
func TestWaylandSetMinSize_BeforeCreateIsRemembered(t *testing.T) {
	w := &WaylandWindow{}
	w.SetMinSize(640, 480)

	if !w.minWant || w.minW != 640 || w.minH != 480 {
		t.Errorf("минимум не отложен: want=%v %dx%d", w.minWant, w.minW, w.minH)
	}
}
