//go:build linux && !android

package window

// x11touch_linux.go — касания окна X11 (XInput 2.2). Протокол — в x11touch.go.
//
// События касаний подписываются при создании окна и только по просьбе окна
// (SetTouchEnabled до Create): согласование XIQueryVersion и подписка — это
// запросы с ответом, а читать ответы можно, пока на соединении нет посторонних
// событий, то есть до показа окна. Окно, не просившее касаний, не обращается к
// XInput вообще: сервер видит такого клиента ровно так, как видел всегда.
//
// Выключение на лету работает (события просто отбрасываются), включение после
// создания окна — нет: подписаться уже нельзя, не рискуя перепутать ответ с
// событием.

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/widget"
)

// x11Touch — состояние касаний окна.
type x11Touch struct {
	want     atomic.Bool // окно просило касания (и не выключило)
	selected atomic.Bool // подписка на события XI2 удалась
	opcode   atomic.Uint32
	cb       atomic.Pointer[func(ev widget.TouchEvent)]
	guard    x11TouchGuard // только цикл событий
}

// SetTouchEnabled включает и выключает касания. Реализует touchSource.
func (w *X11Window) SetTouchEnabled(on bool) {
	w.touch.want.Store(on)
}

// SetOnTouch подписывает приёмник касаний. Реализует touchSource.
func (w *X11Window) SetOnTouch(fn func(ev widget.TouchEvent)) {
	if fn == nil {
		w.touch.cb.Store(nil)
		return
	}
	w.touch.cb.Store(&fn)
}

// x11InitTouch согласует XInput 2.2 и подписывает окно на касания. Вызывается из
// Create, до показа окна: ответы читаются напрямую (см. x11ShmInit). Любая неудача —
// молчаливый отказ: окно остаётся без касаний, как без этого кода.
func (w *X11Window) x11InitTouch() {
	if !w.touch.want.Load() {
		return
	}
	w.x11Send(x11QueryExtensionRequest(xiExtensionName))
	reply := make([]byte, 32)
	if !w.readFull(reply) {
		return
	}
	major, _, present := parseQueryExtensionReply(reply)
	if !present {
		return
	}

	w.x11Send(xiQueryVersionRequest(major, xiTouchMajor, xiTouchMinor))
	if !w.readFull(reply) {
		return
	}
	vmaj, vmin, ok := parseXIQueryVersionReply(reply)
	if !ok || vmaj < xiTouchMajor || (vmaj == xiTouchMajor && vmin < xiTouchMinor) {
		return // сервер не знает XI 2.2: касаний нет
	}

	w.x11Send(xiSelectEventsRequest(major, w.wid, xiAllMasterDevices, xiTouchEventMask()))
	// XISelectEvents ответа не даёт, а отказ приходит ошибкой — проверяем её
	// тем же приёмом, что и ShmAttach.
	if !w.x11SyncCheckError() {
		return
	}
	w.touch.opcode.Store(uint32(major))
	w.touch.selected.Store(true)
}

// x11CompleteEvent дополняет 32-байтовый пакет до целого события. Обычное
// событие X11 занимает ровно 32 байта; GenericEvent (XInput2) несёт хвост, длина
// которого в 4-байтовых словах стоит в заголовке, и не дочитав его, мы приняли бы
// хвост за следующие события.
//
// (nil, true) — событие отброшено как неправдоподобное; (nil, false) — соединение
// оборвано.
func (w *X11Window) x11CompleteEvent(pkt []byte) ([]byte, bool) {
	if pkt[0]&0x7F != x11GenericEvent {
		return pkt, true
	}
	extra := int(binary.LittleEndian.Uint32(pkt[4:8])) * 4
	if extra == 0 {
		return pkt, true
	}
	if extra > maxGenericEventBytes {
		w.x11Discard(extra)
		return nil, true
	}
	full := make([]byte, 32+extra)
	copy(full, pkt)
	if !w.readFull(full[32:]) {
		return nil, false
	}
	return full, true
}

// handleGenericEvent — GenericEvent: касания XI2.
func (w *X11Window) handleGenericEvent(buf []byte) {
	if !w.touch.selected.Load() {
		return
	}
	if !w.touch.want.Load() {
		w.touch.guard.reset()
		return
	}
	xe, ok := parseXITouchEvent(buf, byte(w.touch.opcode.Load()))
	if !ok {
		return
	}
	w.touch.guard.note(xe)
	if fn := w.touch.cb.Load(); fn != nil {
		(*fn)(xe.touch())
	}
}

// touchSwallowsPointer — событие ядра (кнопка, движение) является эмуляцией
// указателя от касания и должно быть отброшено (см. x11TouchGuard).
func (w *X11Window) touchSwallowsPointer(buf []byte) bool {
	if !w.touch.selected.Load() || !w.touch.want.Load() {
		return false
	}
	return w.touch.guard.swallowsPointer(binary.LittleEndian.Uint32(buf[4:8]))
}
