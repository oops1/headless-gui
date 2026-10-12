//go:build linux && !android

package window

// wayland_touch_linux.go — привязка wl_touch к окну Wayland.
//
// wl_touch берётся у wl_seat только по просьбе окна (SetTouchEnabled) и только
// если компоновщик объявил у сиденья возможность touch: клиент, не привязавший
// wl_touch, остаётся на прежнем содержании ввода — указатель, а касания для него
// компоновщик (если он это умеет) превращает в указатель сам. Разбор событий —
// в waylandtouch.go.
//
// Форма контакта (shape, orientation) есть только у wl_touch версии 6, а версию
// wl_touch задаёт сиденье, из которого он создан. Основное сиденье окна связано
// на версии 4 (wlSeatVersion) намеренно: выше меняются события указателя. Поэтому
// для касаний, если компоновщик объявил wl_seat не ниже 6, привязывается ВТОРОЙ
// объект wl_seat той же глобали на версии 6 — только ради get_touch; указатель и
// клавиатура остаются на старом. События этого второго объекта (capabilities,
// name) окну не нужны и отбрасываются.

import (
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/widget"
)

// wlTouch — состояние касаний окна Wayland.
type wlTouch struct {
	want atomic.Bool   // окно просило касания
	id   atomic.Uint32 // объект wl_touch; 0 — не привязан
	caps atomic.Uint32 // последние возможности wl_seat
	// seat6 — второй wl_seat на версии 6, из которого взят wl_touch (0 — касания
	// идут от основного сиденья без формы контакта). shaped — wl_touch версии 6.
	seat6  atomic.Uint32
	shaped atomic.Bool
	cb     atomic.Pointer[func(ev widget.TouchEvent)]
	sink   wlTouchSink // только цикл событий
	inited bool        // sink связан с окном
}

// SetTouchEnabled включает и выключает приём касаний. Реализует touchSource.
// Включение до Create лишь запоминается: wl_touch привязывается, когда
// компоновщик сообщит возможности сиденья.
func (w *WaylandWindow) SetTouchEnabled(on bool) {
	w.touch.want.Store(on)
	if on && w.conn != nil {
		w.ensureTouch(w.touch.caps.Load())
	}
}

// SetOnTouch подписывает приёмник касаний. Реализует touchSource.
func (w *WaylandWindow) SetOnTouch(fn func(ev widget.TouchEvent)) {
	if fn == nil {
		w.touch.cb.Store(nil)
		return
	}
	w.touch.cb.Store(&fn)
}

// ensureTouch привязывает wl_touch, если окно просило касаний и сиденье их умеет.
func (w *WaylandWindow) ensureTouch(caps uint32) {
	if caps&seatCapTouch == 0 || !w.touch.want.Load() || w.seatID == 0 {
		return
	}
	id := w.newID()
	if !w.touch.id.CompareAndSwap(0, id) {
		return // уже привязан (id остался неиспользованным — id не переиспользуем)
	}
	seat := w.seatID
	if w.gSeat != 0 && w.gSeatVer >= wlSeatTouchShapeVersion {
		seat = w.bind(w.gSeat, "wl_seat", wlSeatTouchShapeVersion)
		w.touch.seat6.Store(seat)
		w.touch.shaped.Store(true)
	}
	w.send(newWlMsg(seat, wlSeatGetTouch).putUint(id), -1)
}

// noteSeatCaps разбирает возможности сиденья: берёт wl_touch, когда он нужен, и
// отпускает, когда сенсорное устройство пропало.
func (w *WaylandWindow) noteSeatCaps(caps uint32) {
	w.touch.caps.Store(caps)
	w.ensureTouch(caps)
	if caps&seatCapTouch == 0 {
		if id := w.touch.id.Swap(0); id != 0 {
			w.touchSink().cancelAll()
			w.send(newWlMsg(id, wlTouchRelease), -1)
			if s6 := w.touch.seat6.Swap(0); s6 != 0 {
				w.send(newWlMsg(s6, wlSeatRelease), -1)
			}
			w.touch.shaped.Store(false)
		}
	}
}

// touchSink возвращает приёмник событий, связав его с окном при первом обращении.
func (w *WaylandWindow) touchSink() *wlTouchSink {
	t := &w.touch.sink
	if !w.touch.inited {
		w.touch.inited = true
		t.scale = w.scaleFactor
		t.shapes = w.touch.shaped.Load
		t.popupFor = func(surf uint32) (uintptr, bool) {
			if pop := w.popupForSurface(surf); pop != nil {
				return pop.id, true
			}
			return 0, false
		}
		t.popup = w.popupHandlers
		t.serial = w.inputSerial.Store
		t.onTouch = func(ev widget.TouchEvent) {
			if fn := w.touch.cb.Load(); fn != nil {
				(*fn)(ev)
			}
		}
	}
	return t
}

// handleTouch — событие объекта wl_touch (цикл событий).
func (w *WaylandWindow) handleTouch(opcode uint16, b []byte) {
	t := w.touchSink()
	if !w.touch.want.Load() {
		t.drop() // касания выключили на лету: отменять уже нечему
		return
	}
	t.handle(opcode, b)
}
