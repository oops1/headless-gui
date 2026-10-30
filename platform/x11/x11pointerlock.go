package x11

// x11pointerlock.go — X11: захват указателя (Window.SetPointerLock), сборка
// запросов и разбор событий XInput2.
//
// Файл БЕЗ платформенного суффикса — по той же причине, что x11touch.go:
// раскладку байтов можно проверять на любой машине. Порядок обмена и состояние —
// в x11pointerlock_linux.go.
//
// Протокол.
//
//   - Сдвиги дают сырые события XInput 2: XI_RawMotion, подписка XISelectEvents
//     на КОРНЕВОЕ окно для всех мастер-устройств. Сырое событие приходит
//     независимо от того, где стоит указатель и упёрся ли он в край окна, —
//     именно то, что нужно. В нём два набора значений осей: преобразованные
//     (с ускорением указателя) и сырые (raw_values, единицы устройства). Берутся
//     сырые — как и RAWINPUT на Win32, и unaccelerated-сдвиг Wayland.
//   - Указатель удерживает XGrabPointer: confine_to окну (не выйдет за край) и
//     невидимый курсор. Захват действует, пока окно его держит, и снимается
//     XUngrabPointer.
//   - Чтобы указатель не стоял у самого края (там в окне без рамки лежат зоны
//     изменения размера), его возвращают в центр XWarpPointer, как только он
//     отошёл от центра далеко.
//
// Если XInput2 нет, остаётся запасной путь: тот же захват с возвратом указателя
// в центр после каждого движения, а сдвиг считается как отличие положения от
// центра (x11WarpDelta). Он грубее — теряет сдвиги, пока возврат не дошёл, и
// не умеет «сырых» единиц, — зато работает на любом X-сервере.

import (
	"encoding/binary"
	"math"
)

// Значения протокола.
const (
	// XI_RawMotion — тип сырого события XInput 2.
	xiEvRawMotion = 17

	// Запросы ядра.
	x11OpGrabPointer   = 26
	x11OpUngrabPointer = 27
	x11OpWarpPointer   = 41
	x11OpFreeCursor    = 95

	// Результат XGrabPointer (поле status ответа).
	x11GrabSuccess = 0

	// Маска событий захвата: нажатия и отпускания кнопок, движение. С
	// owner_events=1 события окон этого клиента идут как обычно.
	x11GrabEventMask = 0x4 | 0x8 | 0x40 // ButtonPress | ButtonRelease | PointerMotion
)

// xiRawMotionMask — маска XI_RawMotion: бит 17 — байт 2, бит 1.
func xiRawMotionMask() [4]byte {
	var m [4]byte
	m[xiEvRawMotion/8] |= 1 << (xiEvRawMotion % 8)
	return m
}

// x11GrabPointerRequest — GrabPointer (opcode 26): owner_events=1, захват
// окном grab с привязкой к confine и курсором cursor (0 — не менять), режимы
// асинхронные, время — CurrentTime.
func x11GrabPointerRequest(grab, confine, cursor uint32) []byte {
	buf := make([]byte, 24)
	buf[0] = x11OpGrabPointer
	buf[1] = 1 // owner-events
	binary.LittleEndian.PutUint16(buf[2:4], 6)
	binary.LittleEndian.PutUint32(buf[4:8], grab)
	binary.LittleEndian.PutUint16(buf[8:10], x11GrabEventMask)
	buf[10] = 1 // pointer-mode: Asynchronous
	buf[11] = 1 // keyboard-mode: Asynchronous
	binary.LittleEndian.PutUint32(buf[12:16], confine)
	binary.LittleEndian.PutUint32(buf[16:20], cursor)
	binary.LittleEndian.PutUint32(buf[20:24], 0) // CurrentTime
	return buf
}

// parseGrabPointerReply возвращает status ответа GrabPointer; ok=false — это
// не ответ.
func parseGrabPointerReply(reply []byte) (status byte, ok bool) {
	if len(reply) < 8 || reply[0] != 1 {
		return 0, false
	}
	return reply[1], true
}

// x11UngrabPointerRequest — UngrabPointer (opcode 27, CurrentTime).
func x11UngrabPointerRequest() []byte {
	buf := make([]byte, 8)
	buf[0] = x11OpUngrabPointer
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	return buf
}

// x11WarpPointerRequest — WarpPointer (opcode 41): указатель переносится в
// (x, y) окна dst; исходного окна нет, условие переноса не задано.
func x11WarpPointerRequest(dst uint32, x, y int) []byte {
	buf := make([]byte, 24)
	buf[0] = x11OpWarpPointer
	binary.LittleEndian.PutUint16(buf[2:4], 6)
	// src-window = 0 (None): перенос безусловный
	binary.LittleEndian.PutUint32(buf[8:12], dst)
	binary.LittleEndian.PutUint16(buf[20:22], uint16(int16(x)))
	binary.LittleEndian.PutUint16(buf[22:24], uint16(int16(y)))
	return buf
}

// x11WakeRequest — SendEvent (opcode 25): окну window посылается безвредное
// ClientMessage с типом XA_STRING (31). Нужен, чтобы разбудить цикл событий,
// спящий в чтении: тот, кто хочет выполнить запрос с ответом, читать сокет сам
// не может.
//
// Почему не GetInputFocus, как у касаний. Ответ GetInputFocus неотличим от
// ответа на запрос, который цикл сделает сразу после пробуждения: лишний ответ,
// пришедший раньше, был бы принят за чужой. ClientMessage — событие, оно ответов
// не путает, а цикл его пропускает (тип сообщения не совпадает ни с одним из
// наших атомов).
func x11WakeRequest(window uint32) []byte {
	buf := make([]byte, 44)
	buf[0] = 25 // SendEvent
	// propagate = 0, event-mask = 0: событие получает создатель окна, то есть мы
	binary.LittleEndian.PutUint16(buf[2:4], 11)
	binary.LittleEndian.PutUint32(buf[4:8], window)
	ev := buf[12:]
	ev[0] = 33 // ClientMessage
	ev[1] = 32 // format
	binary.LittleEndian.PutUint32(ev[4:8], window)
	binary.LittleEndian.PutUint32(ev[8:12], 31) // XA_STRING
	return buf
}

// x11FreeCursorRequest — FreeCursor (opcode 95).
func x11FreeCursorRequest(cursor uint32) []byte {
	return x11FreeReq(x11OpFreeCursor, cursor)
}

// parseXIRawMotion разбирает GenericEvent XI_RawMotion и возвращает сырой
// (без ускорения) сдвиг по осям 0 и 1 — относительным X и Y мыши. ok=false —
// событие чужого расширения, другого типа или обрезанное; ни X, ни Y в нём
// может не быть (колесо) — тогда сдвиг нулевой, но ok=true.
//
// Раскладка xXIRawEvent (32 байта): type, extension, sequence, length, evtype
// @8, deviceid @10, time @12, detail @16, sourceid @20, valuators_len @22
// (слов маски), flags @24. Дальше маска осей (valuators_len слов), затем для
// каждой выставленной в маске оси пара по 8 байт FP3232: сперва все
// преобразованные значения, потом все сырые.
func parseXIRawMotion(buf []byte, xiOpcode byte) (dx, dy float64, ok bool) {
	if len(buf) < 32 || buf[0]&0x7F != x11GenericEvent || buf[1] != xiOpcode {
		return 0, 0, false
	}
	if binary.LittleEndian.Uint16(buf[8:10]) != xiEvRawMotion {
		return 0, 0, false
	}
	maskLen := int(binary.LittleEndian.Uint16(buf[22:24])) * 4
	if len(buf) < 32+maskLen {
		return 0, 0, false
	}
	mask := buf[32 : 32+maskLen]
	set := 0 // сколько осей выставлено
	for _, b := range mask {
		for ; b != 0; b &= b - 1 {
			set++
		}
	}
	valuesAt := 32 + maskLen
	if len(buf) < valuesAt+2*set*8 {
		return 0, 0, false
	}
	rawAt := valuesAt + set*8 // сырые значения идут за преобразованными
	idx := 0
	for axis := 0; axis < 2 && axis < maskLen*8; axis++ {
		if mask[axis/8]&(1<<(axis%8)) == 0 {
			continue
		}
		off := rawAt + idx*8
		v := fp3232(buf[off : off+8])
		if axis == 0 {
			dx = v
		} else {
			dy = v
		}
		idx++
	}
	if math.IsNaN(dx) || math.IsNaN(dy) {
		return 0, 0, false
	}
	return dx, dy, true
}

// x11WarpDelta — запасной путь без XInput2: указатель возвращают в центр после
// каждого движения, поэтому сдвиг — это отличие положения (x, y) от центра
// (cx, cy). Пришедшее из самого возврата событие (положение уже в центре)
// сдвига не несёт.
func x11WarpDelta(x, y, cx, cy int) (dx, dy float64) {
	return float64(x - cx), float64(y - cy)
}

// x11NeedsRecenter — указатель ушёл от центра окна размером w×h дальше, чем на
// четверть размера, и его пора вернуть: иначе он упрётся в край, где лежат зоны
// изменения размера окна без рамки.
func x11NeedsRecenter(x, y, w, h int) bool {
	cx, cy := w/2, h/2
	return x11Abs(x-cx) > w/4 || x11Abs(y-cy) > h/4
}

func x11Abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
