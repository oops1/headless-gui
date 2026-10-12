//go:build linux && !android

package window

// x11touch_linux.go — касания окна X11 (XInput 2.2). Протокол — в x11touch.go.
//
// События касаний подписываются только по просьбе окна. Согласование
// XIQueryVersion и подписка — запросы с ответом, и читать ответ посреди потока
// событий, не отделяя его от событий, нельзя. Поэтому:
//
//   - просьба до Create: подписка при создании окна, до показа, пока посторонних
//     событий на соединении нет;
//   - просьба позже (Window.SetTouchEnabled(true) после создания): запоминается
//     (pending), цикл событий будится безвредным запросом GetInputFocus и на
//     верхнем уровне, между событиями, делает то же самое согласование, но уже
//     читая ответ как x11GetProperty: события, пришедшие раньше ответа,
//     обрабатываются на месте (x11SyncTouch — по образцу x11SyncWMState).
//
// Окно, не просившее касаний, не обращается к XInput вообще: сервер видит такого
// клиента ровно так, как видел всегда. Выключение на лету работает (события
// просто отбрасываются), повторное включение подписку не повторяет.

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

	// pending — подписку надо сделать из цикла событий (просьба пришла после
	// создания окна); looping — цикл событий запущен и будится запросом.
	pending atomic.Bool
	looping atomic.Bool
	// axesStale — появилось устройство, которого нет в списке осей: список надо
	// запросить заново.
	axesStale atomic.Bool
	axesGen   atomic.Uint32 // сколько раз список осей загружен (для тестов)
	atoms     x11TouchAtoms
	axes      x11TouchAxisTracker // только цикл событий (и Create до его запуска)
}

// SetTouchEnabled включает и выключает касания. Реализует touchSource.
//
// Включение до Create подписывается при создании окна. Включение позже
// откладывается до безопасной точки цикла событий (x11SyncTouch).
func (w *X11Window) SetTouchEnabled(on bool) {
	w.touch.want.Store(on)
	if !on || w.touch.selected.Load() {
		return
	}
	w.touch.pending.Store(true)
	if w.touch.looping.Load() {
		// Цикл спит в чтении: будим его запросом, на который придёт ответ. Ответ
		// цикл примет за обычный пакет и пропустит (ответа GetInputFocus никто
		// не ждёт), а сразу после него сделает подписку.
		w.x11Send(x11GetInputFocusRequest())
	}
}

// x11GetInputFocusRequest — GetInputFocus (opcode 43): запрос с гарантированным
// ответом.
func x11GetInputFocusRequest() []byte {
	req := make([]byte, 4)
	req[0] = 43
	binary.LittleEndian.PutUint16(req[2:4], 1)
	return req
}

// x11SyncTouch выполняет отложенную подписку на касания и обновление списка осей.
// Зовётся циклом событий между событиями — на верхнем уровне, где запрос-ответ
// безопасен (как x11SyncWMState).
func (w *X11Window) x11SyncTouch() {
	if w.touch.pending.Load() {
		w.x11InitTouch()
	}
	if w.touch.axesStale.Swap(false) && w.touch.selected.Load() {
		w.x11LoadTouchAxes(byte(w.touch.opcode.Load()), false)
	}
}

// SetOnTouch подписывает приёмник касаний. Реализует touchSource.
func (w *X11Window) SetOnTouch(fn func(ev widget.TouchEvent)) {
	if fn == nil {
		w.touch.cb.Store(nil)
		return
	}
	w.touch.cb.Store(&fn)
}

// x11Reply шлёт запрос и читает пакеты до его ответа. События, пришедшие раньше
// ответа, обрабатываются на месте (как в x11GetProperty): иначе ответ нельзя
// отличить от них, а выбросить их — потерять ввод. Ответ возвращается целиком, с
// хвостом. ok=false — запрос отвергнут ошибкой или соединение оборвано.
func (w *X11Window) x11Reply(req []byte) (reply []byte, ok bool) {
	w.x11Send(req)
	hdr := make([]byte, 32)
	for i := 0; i < 4096; i++ { // предохранитель от бесконечного цикла
		if !w.readFull(hdr) {
			return nil, false
		}
		switch hdr[0] {
		case 1:
			extra := int(binary.LittleEndian.Uint32(hdr[4:8])) * 4
			if extra == 0 {
				return hdr, true
			}
			if extra > maxGenericEventBytes {
				w.x11Discard(extra)
				return nil, false
			}
			full := make([]byte, 32+extra)
			copy(full, hdr)
			if !w.readFull(full[32:]) {
				return nil, false
			}
			return full, true
		case 0:
			return nil, false // запрос отвергнут
		default:
			if full, ok := w.x11CompleteEvent(hdr); ok && full != nil {
				w.handleX11Event(full)
			}
		}
	}
	return nil, false
}

// x11SyncCheck проверяет, что только что отправленный запрос без ответа
// (XISelectEvents) не отвергнут: шлёт GetInputFocus и читает до его ответа, а
// ошибка, пришедшая раньше, относится к предшествующему запросу. Событий не
// теряет (см. x11Reply).
func (w *X11Window) x11SyncCheck() bool {
	w.x11Send(x11GetInputFocusRequest())
	hdr := make([]byte, 32)
	failed := false
	for i := 0; i < 4096; i++ {
		if !w.readFull(hdr) {
			return false
		}
		switch hdr[0] {
		case 1:
			return !failed
		case 0:
			failed = true
		default:
			if full, ok := w.x11CompleteEvent(hdr); ok && full != nil {
				w.handleX11Event(full)
			}
		}
	}
	return false
}

// x11InitTouch согласует XInput 2.2 и подписывает окно на касания. Вызывается из
// Create (до показа окна) и из цикла событий (x11SyncTouch) — читает ответы
// терпимо к событиям между ними. Любая неудача — молчаливый отказ: окно остаётся
// без касаний, как без этого кода.
func (w *X11Window) x11InitTouch() {
	w.touch.pending.Store(false)
	if !w.touch.want.Load() || w.touch.selected.Load() {
		return
	}
	reply, ok := w.x11Reply(x11QueryExtensionRequest(xiExtensionName))
	if !ok {
		return
	}
	major, _, present := parseQueryExtensionReply(reply)
	if !present {
		return
	}

	reply, ok = w.x11Reply(xiQueryVersionRequest(major, xiTouchMajor, xiTouchMinor))
	if !ok {
		return
	}
	vmaj, vmin, ok := parseXIQueryVersionReply(reply)
	if !ok || vmaj < xiTouchMajor || (vmaj == xiTouchMajor && vmin < xiTouchMinor) {
		return // сервер не знает XI 2.2: касаний нет
	}

	w.x11Send(xiSelectEventsRequest(major, w.wid, xiAllMasterDevices, xiTouchEventMask()))
	// XISelectEvents ответа не даёт, а отказ приходит ошибкой — проверяем её
	// тем же приёмом, что и ShmAttach.
	if !w.x11SyncCheck() {
		return
	}
	w.touch.opcode.Store(uint32(major))
	w.touch.selected.Store(true)

	// Нажим и пятно контакта — по желанию: без них касания работают так же.
	w.x11LoadTouchAxes(major, true)
}

// x11LoadTouchAxes узнаёт у сервера оси сенсорных устройств: атомы меток (если
// withAtoms) и XIQueryDevice. Любая неудача оставляет события без нажима и пятна.
func (w *X11Window) x11LoadTouchAxes(xiOpcode byte, withAtoms bool) {
	if withAtoms {
		var at x11TouchAtoms
		names := [...]struct {
			name string
			dst  *uint32
		}{
			{xiAxisPressure, &at.pressure}, {xiAxisMajor, &at.major}, {xiAxisMinor, &at.minor},
			{xiAxisOrient, &at.orient}, {xiAxisPosX, &at.posX},
		}
		for _, n := range names {
			reply, ok := w.x11Reply(x11InternAtomRequest(n.name, true))
			if !ok || len(reply) < 12 {
				return
			}
			*n.dst = binary.LittleEndian.Uint32(reply[8:12])
		}
		w.touch.atoms = at
	}
	reply, ok := w.x11Reply(xiQueryDeviceRequest(xiOpcode, xiAllDevices))
	if !ok {
		return
	}
	w.touch.axes.setDevices(parseXIQueryDeviceReply(reply), w.touch.atoms, int(w.screen.WidthInPixels))
	w.touch.axesGen.Add(1)
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
		ev := xe.touch()
		if w.touch.axes.apply(&ev, xe) {
			w.touch.axesStale.Store(true) // новое устройство: оси обновим между событиями
		}
		(*fn)(ev)
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
