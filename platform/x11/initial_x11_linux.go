//go:build linux && !android

package x11

// initial_x11_linux.go — начальное положение и «развёрнуто» на X11 (window/core/initial.go).
//
// Всё, что касается показа окна, обязано попасть на сервер ДО MapWindow:
// менеджер читает WM_NORMAL_HINTS и _NET_WM_STATE в момент показа и решает по
// ним, где поставить окно и в каком состоянии. После показа то же самое было
// бы переносом и разворачиванием на глазах.

import (
	"encoding/binary"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// SetInitialPlacement запоминает положение и «развёрнуто» до Create.
// Реализует initialPlaceBackend.
func (w *X11Window) SetInitialPlacement(p native.InitialPlacement) { w.initial = p }

// x11ClampInt16 вписывает координату в INT16 запроса CreateWindow.
func x11ClampInt16(v int) int16 {
	switch {
	case v > 32767:
		return 32767
	case v < -32768:
		return -32768
	}
	return int16(v)
}

// x11AtomList — значение свойства-списка атомов (format 32), например
// начального _NET_WM_STATE. Чистая функция: раскладка проверяется тестом.
func x11AtomList(atoms ...uint32) []byte {
	b := make([]byte, 0, 4*len(atoms))
	for _, a := range atoms {
		b = binary.LittleEndian.AppendUint32(b, a)
	}
	return b
}

// applyInitialHints ставит WM_NORMAL_HINTS окна: минимальный размер (если
// задан) и объявленное положение/размер (если приложение их задало). Заменяет
// applyPendingMinSize на Create: свойство одно, и записать его надо целиком.
func (w *X11Window) applyInitialHints(x, y, width, height int) {
	p := w.initial
	if !p.PosSet && !p.SizeSet {
		w.applyPendingMinSize()
		return
	}
	minW, minH := 0, 0
	if w.minWant {
		minW, minH = w.minW, w.minH
		w.minWant = false
	}
	w.x11ChangeProperty(w.wid, 40 /*WM_NORMAL_HINTS*/, 41 /*WM_SIZE_HINTS*/, 32,
		native.X11SizeHints(minW, minH, p, x, y, width, height))
}

// applyInitialState ставит окну начальное состояние: «развёрнуто»
// (MAXIMIZED_VERT/HORZ) и состояния стиля (SKIP_TASKBAR, SKIP_PAGER и
// SKIP_SWITCHER у Style.Tool, x11_style_linux.go) — свойством _NET_WM_STATE до MapWindow, так
// EWMH задаёт начальное состояние (ClientMessage годится только уже
// показанному окну). IsMaximized отвечает true сразу; менеджер, не
// поддерживающий состояние, перепишет свойство при показе, и PropertyNotify
// вернёт false.
func (w *X11Window) applyInitialState() {
	if w.atomNetWMState == 0 {
		return
	}
	maximize := w.initial.Maximized && w.atomWMStateMaxH != 0 && w.atomWMStateMaxV != 0
	var atoms []uint32
	if maximize {
		atoms = append(atoms, w.atomWMStateMaxV, w.atomWMStateMaxH)
	}
	atoms = append(atoms, w.styleStateAtoms()...)
	if len(atoms) == 0 {
		return
	}
	w.x11ChangeProperty(w.wid, w.atomNetWMState, 4 /*ATOM*/, 32, x11AtomList(atoms...))
	if maximize {
		w.maximized = true
	}
}
