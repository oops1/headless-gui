//go:build linux && !android

package window

// x11_keygrab_linux.go — захват клавиатуры на X11 (XGrabKeyboard; описание
// поведения — в keygrab.go).
//
// Активный захват клавиатуры отбирает ввод у всех: и у оконного менеджера с его
// горячими клавишами (XGrabKey на корневом окне), и у других клиентов, пока он
// не отпущен. Поэтому держим его только пока окно в фокусе: потеряли фокус —
// отпускаем, получили — берём снова.
//
// XGrabKeyboard отвечает status'ом (Success, AlreadyGrabbed, NotViewable,
// Frozen, InvalidTime), а читать ответ посреди потока событий нельзя
// (x11touch_linux.go). Поэтому SetKeyboardGrab и смена фокуса лишь помечают
// состояние «изменилось», будят цикл событий безвредным GetInputFocus, а сам
// запрос с чтением ответа делает цикл на верхнем уровне (x11SyncKeyGrab).
//
// Будить цикл нельзя GetInputFocus, как делают касания: его ответ — такой же
// пакет, как и ответ на GrabKeyboard, и цикл, ждущий ответа на захват, принял
// бы чужой за свой. Будим ClientMessage самому себе (SendEvent с пустой
// маской уходит создателю окна, то есть нам), он ответа не даёт.

import (
	"encoding/binary"
	"sync/atomic"
)

const (
	x11OpGrabKeyboard   = 31
	x11OpUngrabKeyboard = 32
	// x11GrabSuccess (статус ответа GrabKeyboard) — общий с захватом
	// указателя, см. x11pointerlock.go.

	// Поле mode событий FocusIn/FocusOut. Захват, который берём мы сами,
	// порождает пару событий с режимом Grab/Ungrab — это не смена фокуса
	// пользователем.
	x11FocusModeNormal       = 0
	x11FocusModeGrab         = 1
	x11FocusModeUngrab       = 2
	x11FocusModeWhileGrabbed = 3
)

// x11KeyGrab — состояние захвата клавиатуры окна.
type x11KeyGrab struct {
	want atomic.Bool // приложение просило захват
	// focused/focusSeen — есть ли у окна фокус. Пока ни одного события фокуса не
	// было (нет оконного менеджера, окно только создано), считаем, что фокус
	// есть: иначе захват без менеджера не включился бы никогда.
	focused   atomic.Bool
	focusSeen atomic.Bool
	held      atomic.Bool // сервер подтвердил захват и он не отпущен
	dirty     atomic.Bool // want/focused менялись: цикл событий должен свериться
	looping   atomic.Bool // цикл событий запущен и будится запросом
	status    atomic.Int32
}

// x11GrabKeyboardRequest — GrabKeyboard (опкод 31): owner_events=true (клавиши
// окнам этого клиента идут обычным путём), время — CurrentTime, оба режима —
// асинхронные.
func x11GrabKeyboardRequest(wid uint32) []byte {
	req := make([]byte, 16)
	req[0] = x11OpGrabKeyboard
	req[1] = 1 // owner_events
	binary.LittleEndian.PutUint16(req[2:4], 4)
	binary.LittleEndian.PutUint32(req[4:8], wid)
	binary.LittleEndian.PutUint32(req[8:12], 0) // CurrentTime
	req[12] = 1                                 // pointer_mode: Asynchronous
	req[13] = 1                                 // keyboard_mode: Asynchronous
	return req
}

// x11SelfWakeRequest — ClientMessage самому окну (SendEvent, маска 0 — событие
// идёт создателю окна без подписки). Ответа не даёт; цикл событий просыпается,
// а тип сообщения никому не известен и обработчик его пропускает.
func x11SelfWakeRequest(wid, msgType uint32) []byte {
	req := make([]byte, 44)
	req[0] = 25 // SendEvent
	binary.LittleEndian.PutUint16(req[2:4], 11)
	binary.LittleEndian.PutUint32(req[4:8], wid)
	// event_mask @8 = 0
	ev := req[12:]
	ev[0] = 33 // ClientMessage
	ev[1] = 32
	binary.LittleEndian.PutUint32(ev[4:8], wid)
	binary.LittleEndian.PutUint32(ev[8:12], msgType)
	return req
}

// x11UngrabKeyboardRequest — UngrabKeyboard (опкод 32), время CurrentTime.
func x11UngrabKeyboardRequest() []byte {
	req := make([]byte, 8)
	req[0] = x11OpUngrabKeyboard
	binary.LittleEndian.PutUint16(req[2:4], 2)
	return req
}

// SetKeyboardGrab включает или выключает захват клавиатуры (keyboardGrabBackend).
// Захват берётся, когда у окна есть фокус; ответ true — запрос принят.
func (w *X11Window) SetKeyboardGrab(on bool) bool {
	if w.wid == 0 || w.conn == nil {
		return !on
	}
	w.kg.want.Store(on)
	w.kg.dirty.Store(true)
	if w.kg.looping.Load() {
		// Цикл спит в чтении: будим сообщением самому себе (см. описание файла).
		w.x11Send(x11SelfWakeRequest(w.wid, w.atomTargets))
	}
	return true
}

// KeyboardGrabActive — сервер держит захват за окном (keyboardGrabQuerier).
func (w *X11Window) KeyboardGrabActive() bool { return w.kg.held.Load() }

// x11NoteFocus учитывает событие фокуса. Возвращает true, если событие
// порождено нашим же захватом и приложению о смене активности сообщать не надо.
func (w *X11Window) x11NoteFocus(in bool, mode byte) (ownGrab bool) {
	switch mode {
	case x11FocusModeGrab, x11FocusModeUngrab:
		// Не наш захват меняет фокус под нами — не повод считать окно
		// неактивным; свой же захват «активность» не меняет вовсе.
		return w.kg.want.Load()
	}
	w.kg.focusSeen.Store(true)
	w.kg.focused.Store(in)
	if w.kg.want.Load() {
		w.kg.dirty.Store(true)
	}
	return false
}

// x11SyncKeyGrab сверяет захват с тем, что нужно: берёт при фокусе, отпускает
// при его потере или выключении. Зовётся циклом событий между событиями — на
// верхнем уровне, где запрос-ответ безопасен.
func (w *X11Window) x11SyncKeyGrab() {
	if !w.kg.dirty.Swap(false) || w.wid == 0 {
		return
	}
	want := w.kg.want.Load() && !w.closed.Load() &&
		(w.kg.focused.Load() || !w.kg.focusSeen.Load())
	held := w.kg.held.Load()
	switch {
	case want && !held:
		reply, ok := w.x11Reply(x11GrabKeyboardRequest(w.wid))
		if !ok || len(reply) < 2 {
			return
		}
		w.kg.status.Store(int32(reply[1]))
		// Отказ (AlreadyGrabbed — клавиатуру держит другой клиент, NotViewable —
		// окно ещё не показано) повторяется со следующим событием фокуса, а не
		// на каждом событии: иначе движение мыши порождало бы круговой обмен.
		w.kg.held.Store(reply[1] == x11GrabSuccess)
	case !want && held:
		w.x11Send(x11UngrabKeyboardRequest())
		w.kg.held.Store(false)
	}
}

// x11FocusByGrab — событие фокуса порождено захватом (XGrabKeyboard или
// XUngrabKeyboard), а не настоящей сменой фокуса окна.
func x11FocusByGrab(mode byte) bool {
	return mode == x11FocusModeGrab || mode == x11FocusModeUngrab
}
