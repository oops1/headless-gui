//go:build linux && !android

package core

// initial_x11_linux.go — начальное положение и «развёрнуто» на X11 (initial.go).
//
// Всё, что касается показа окна, обязано попасть на сервер ДО MapWindow:
// менеджер читает WM_NORMAL_HINTS и _NET_WM_STATE в момент показа и решает по
// ним, где поставить окно и в каком состоянии. После показа то же самое было
// бы переносом и разворачиванием на глазах.

import (
	"encoding/binary"

	"github.com/oops1/headless-gui/v3/platform/native"
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

// x11InitialStateData — значение свойства _NET_WM_STATE для окна, которое
// должно появиться развёрнутым: список атомов (format 32) MAXIMIZED_VERT и
// MAXIMIZED_HORZ. Чистая функция: раскладка проверяется тестом.
func x11InitialStateData(maxH, maxV uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b[0:4], maxV)
	binary.LittleEndian.PutUint32(b[4:8], maxH)
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

// applyInitialState ставит окну начальное состояние «развёрнуто»: свойство
// _NET_WM_STATE со значениями MAXIMIZED_VERT/HORZ до MapWindow — так EWMH
// задаёт начальное состояние (ClientMessage годится только уже показанному
// окну). IsMaximized отвечает true сразу; менеджер, не поддерживающий
// состояние, перепишет свойство при показе, и PropertyNotify вернёт false.
func (w *X11Window) applyInitialState() {
	if !w.initial.Maximized || w.atomNetWMState == 0 || w.atomWMStateMaxH == 0 || w.atomWMStateMaxV == 0 {
		return
	}
	w.x11ChangeProperty(w.wid, w.atomNetWMState, 4 /*ATOM*/, 32,
		x11InitialStateData(w.atomWMStateMaxH, w.atomWMStateMaxV))
	w.maximized = true
}
