package window

// x11touch.go — касания X11 через XInput 2.2: сборка запросов и разбор событий.
//
// Файл БЕЗ платформенного суффикса — по той же причине, что waylandwire.go и
// waylandtouch.go: проверять раскладку байтов можно на любой машине. Сокет,
// согласование версии и подписка — в x11touch_linux.go.
//
// Протокол. Касания в X11 существуют только в расширении XInput 2.2 и выше:
//
//   - QueryExtension("XInputExtension") даёт major opcode расширения;
//   - XIQueryVersion(2, 2) обязателен до любых запросов XI2 и сообщает версию,
//     которую поддерживает сервер (меньше 2.2 — касаний нет);
//   - XISelectEvents(window, XIAllMasterDevices, XI_TouchBegin|Update|End)
//     подписывает окно; события приходят как GenericEvent (тип 35) с хвостом,
//     длина которого в 4-байтовых словах — в заголовке.
//
// Окно, выбравшее события касаний, получает их вместо эмулируемого указателя
// (сервер эмулирует указатель только для клиентов, не выбравших касания); на
// случай расхождения сервера со спецификацией есть x11TouchGuard.

import (
	"encoding/binary"
	"math"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// Значения протокола.
const (
	x11GenericEvent = 35 // тип события GenericEvent

	xiExtensionName     = "XInputExtension"
	xiMinorSelectEvents = 46
	xiMinorQueryVersion = 47

	xiAllMasterDevices = 1

	// Типы событий XI2 (evtype).
	xiEvTouchBegin  = 18
	xiEvTouchUpdate = 19
	xiEvTouchEnd    = 20

	// Флаги touch-событий (поле flags).
	xiTouchEmulatingPointer = 1 << 17

	// Версия XI, с которой есть касания.
	xiTouchMajor = 2
	xiTouchMinor = 2

	// maxGenericEventBytes — предел хвоста GenericEvent, который мы готовы
	// прочитать: разумное событие XI2 — сотни байт, а заявленные мегабайты —
	// признак порчи потока.
	maxGenericEventBytes = 1 << 20
)

// x11QueryExtensionRequest — core-запрос QueryExtension(name) (opcode 98).
func x11QueryExtensionRequest(name string) []byte {
	nameLen := len(name)
	pad := (4 - nameLen%4) % 4
	reqLen := 2 + (nameLen+pad)/4
	buf := make([]byte, reqLen*4)
	buf[0] = 98
	binary.LittleEndian.PutUint16(buf[2:4], uint16(reqLen))
	binary.LittleEndian.PutUint16(buf[4:6], uint16(nameLen))
	copy(buf[8:], name)
	return buf
}

// xiQueryVersionRequest — XIQueryVersion(major, minor).
func xiQueryVersionRequest(xiOpcode byte, major, minor uint16) []byte {
	buf := make([]byte, 8)
	buf[0] = xiOpcode
	buf[1] = xiMinorQueryVersion
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	binary.LittleEndian.PutUint16(buf[4:6], major)
	binary.LittleEndian.PutUint16(buf[6:8], minor)
	return buf
}

// parseXIQueryVersionReply разбирает reply XIQueryVersion: major@8, minor@10.
// ok=false — это не reply (например, ошибка: сервер не знает такой версии).
func parseXIQueryVersionReply(reply []byte) (major, minor int, ok bool) {
	if len(reply) < 12 || reply[0] != 1 {
		return 0, 0, false
	}
	return int(binary.LittleEndian.Uint16(reply[8:10])), int(binary.LittleEndian.Uint16(reply[10:12])), true
}

// xiTouchEventMask — маска событий XI_TouchBegin, XI_TouchUpdate, XI_TouchEnd:
// бит n лежит в байте n/8 на месте n%8, поэтому 18, 19, 20 — байт 2, биты 2, 3, 4.
func xiTouchEventMask() [4]byte {
	var m [4]byte
	for _, ev := range [...]uint{xiEvTouchBegin, xiEvTouchUpdate, xiEvTouchEnd} {
		m[ev/8] |= 1 << (ev % 8)
	}
	return m
}

// xiSelectEventsRequest — XISelectEvents для одного устройства и маски в одно
// слово: window, num_masks=1, затем (deviceid, mask_len=1, маска).
func xiSelectEventsRequest(xiOpcode byte, window uint32, deviceID uint16, mask [4]byte) []byte {
	buf := make([]byte, 20)
	buf[0] = xiOpcode
	buf[1] = xiMinorSelectEvents
	binary.LittleEndian.PutUint16(buf[2:4], 5) // 20 байт = 5 слов
	binary.LittleEndian.PutUint32(buf[4:8], window)
	binary.LittleEndian.PutUint16(buf[8:10], 1) // num_masks
	// buf[10:12] — pad
	binary.LittleEndian.PutUint16(buf[12:14], deviceID)
	binary.LittleEndian.PutUint16(buf[14:16], 1) // mask_len (в словах)
	copy(buf[16:20], mask[:])
	return buf
}

// xiTouchEvent — разобранное touch-событие XI2 (xXIDeviceEvent).
type xiTouchEvent struct {
	Type     uint16 // xiEvTouchBegin / Update / End
	DeviceID uint16
	SourceID uint16
	Time     uint32  // метка времени сервера, мс
	Detail   uint32  // идентификатор касания
	EventX   float64 // относительно окна, получившего событие
	EventY   float64
	Flags    uint32
}

// fp1616 переводит число с фиксированной точкой 16.16 в float64.
func fp1616(v uint32) float64 { return float64(int32(v)) / 65536 }

// parseXITouchEvent разбирает GenericEvent XI2 касания. ok=false — событие
// чужого расширения, не касание или обрезанное.
func parseXITouchEvent(buf []byte, xiOpcode byte) (xiTouchEvent, bool) {
	// Фиксированная часть xXIDeviceEvent — 80 байт; нам нужно до flags (60).
	if len(buf) < 60 || buf[0]&0x7F != x11GenericEvent || buf[1] != xiOpcode {
		return xiTouchEvent{}, false
	}
	typ := binary.LittleEndian.Uint16(buf[8:10])
	if typ != xiEvTouchBegin && typ != xiEvTouchUpdate && typ != xiEvTouchEnd {
		return xiTouchEvent{}, false
	}
	return xiTouchEvent{
		Type:     typ,
		DeviceID: binary.LittleEndian.Uint16(buf[10:12]),
		SourceID: binary.LittleEndian.Uint16(buf[52:54]),
		Time:     binary.LittleEndian.Uint32(buf[12:16]),
		Detail:   binary.LittleEndian.Uint32(buf[16:20]),
		EventX:   fp1616(binary.LittleEndian.Uint32(buf[40:44])),
		EventY:   fp1616(binary.LittleEndian.Uint32(buf[44:48])),
		Flags:    binary.LittleEndian.Uint32(buf[56:60]),
	}, true
}

// touch переводит событие XI2 в событие виджетов. Координаты — физические
// пиксели окна (X11 не знает логических).
func (e xiTouchEvent) touch() widget.TouchEvent {
	ph := widget.TouchMoved
	switch e.Type {
	case xiEvTouchBegin:
		ph = widget.TouchBegan
	case xiEvTouchEnd:
		ph = widget.TouchEnded
	}
	return widget.TouchEvent{
		ID:    int(e.Detail),
		X:     int(math.Floor(e.EventX + 0.5)),
		Y:     int(math.Floor(e.EventY + 0.5)),
		Phase: ph,
		Time:  time.Duration(e.Time) * time.Millisecond,
	}
}

// x11TouchGuard — защита от двойной доставки одного касания: касанием и
// эмулированным указателем.
//
// По спецификации сервер не эмулирует указатель для окна, чей клиент выбрал
// события касаний. Но спецификацию соблюдают не все серверы и прослойки (XWayland,
// VNC-серверы), а лишняя левая кнопка мыши на каждое касание — это двойной щелчок
// на каждый тап. Пока идёт касание с флагом XITouchEmulatingPointer — и в тот же
// момент времени, что и его отрыв, — события ядра ButtonPress, ButtonRelease и
// MotionNotify окно отбрасывает. Цена — мышь, нажатая во время касания, пропадает;
// это редкий случай и менее неприятный, чем двойные клики.
type x11TouchGuard struct {
	emulating map[uint32]struct{}
	lastEnd   uint32
	hasEnd    bool
}

// note учитывает очередное touch-событие.
func (g *x11TouchGuard) note(e xiTouchEvent) {
	if e.Flags&xiTouchEmulatingPointer == 0 {
		return
	}
	switch e.Type {
	case xiEvTouchBegin:
		if g.emulating == nil {
			g.emulating = make(map[uint32]struct{})
		}
		g.emulating[e.Detail] = struct{}{}
	case xiEvTouchEnd:
		delete(g.emulating, e.Detail)
		g.lastEnd, g.hasEnd = e.Time, true
	}
}

// swallowsPointer — надо ли отбросить событие ядра с меткой времени serverTime.
func (g *x11TouchGuard) swallowsPointer(serverTime uint32) bool {
	if len(g.emulating) > 0 {
		return true
	}
	return g.hasEnd && serverTime == g.lastEnd
}

// reset забывает состояние (касания выключили или окно потеряло ввод).
func (g *x11TouchGuard) reset() {
	g.emulating = nil
	g.hasEnd = false
}
