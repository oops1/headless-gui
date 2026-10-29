package core

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
// Нажим и размер пятна контакта приходят осями устройства (валюаторами) в хвосте
// события; как их найти и перевести — в блоке «Нажим и контакт» ниже.
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
	// raw — событие целиком: за фиксированной частью идут маски и значения
	// осей (нажим, пятно контакта), см. axisValue.
	raw []byte
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
		raw:      buf,
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

// ─── Нажим и контакт: валюаторы устройства ──────────────────────────────────
//
// Давление и размер пятна контакта в X11 — не поля события, а оси (валюаторы)
// сенсорного устройства: у каждой есть метка-атом ("Abs MT Pressure",
// "Abs MT Touch Major"…) и пределы. Поэтому порядок такой:
//
//   - XIQueryDevice(XIAllDevices) один раз, при подписке, даёт для каждого
//     устройства список его валюаторов: номер, атом-метка, min, max;
//   - InternAtom(only_if_exists) по именам нужных меток даёт сами атомы (0 —
//     такой оси ни у кого нет);
//   - в touch-событии за фиксированной частью идут маска кнопок, маска
//     валюаторов и по одному FP3232 на каждый установленный бит маски.
//
// Событие несёт только те оси, которые менялись, поэтому последнее значение
// каждой оси помним на время касания (x11TouchAxisTracker).
//
// Единицы. Давление нормируется по min..max оси в 0..1. Размеры пятна драйвер
// отдаёт в единицах той же оси, что и положение (для evdev — «поверхностные
// единицы» устройства), поэтому в пиксели их переводит множитель «ширина экрана в
// пикселях / размах оси X». Это верно, если сенсорный экран растянут на весь
// экран X; на нескольких мониторах с экраном на одном из них размеры будут
// завышены. Угол ABS_MT_ORIENTATION — «знаковая четверть оборота»: значение,
// делённое на предел оси, умноженное на 90°.

const (
	xiMinorQueryDevice = 48
	xiAllDevices       = 0

	xiValuatorClass = 2 // XIValuatorClass в XIDeviceInfo
)

// Метки осей касания, по которым ищутся валюаторы.
const (
	xiAxisPressure = "Abs MT Pressure"
	xiAxisMajor    = "Abs MT Touch Major"
	xiAxisMinor    = "Abs MT Touch Minor"
	xiAxisOrient   = "Abs MT Orientation"
	xiAxisPosX     = "Abs MT Position X"
)

// x11InternAtomRequest — InternAtom (opcode 16).
func x11InternAtomRequest(name string, onlyIfExists bool) []byte {
	pad := (4 - len(name)%4) % 4
	reqLen := 2 + (len(name)+pad)/4
	buf := make([]byte, reqLen*4)
	buf[0] = 16
	if onlyIfExists {
		buf[1] = 1
	}
	binary.LittleEndian.PutUint16(buf[2:4], uint16(reqLen))
	binary.LittleEndian.PutUint16(buf[4:6], uint16(len(name)))
	copy(buf[8:], name)
	return buf
}

// xiQueryDeviceRequest — XIQueryDevice(deviceid).
func xiQueryDeviceRequest(xiOpcode byte, deviceID uint16) []byte {
	buf := make([]byte, 8)
	buf[0] = xiOpcode
	buf[1] = xiMinorQueryDevice
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	binary.LittleEndian.PutUint16(buf[4:6], deviceID)
	return buf
}

// fp3232 переводит FP3232 (int32 целая часть, uint32 дробная) в float64.
func fp3232(b []byte) float64 {
	return float64(int32(binary.LittleEndian.Uint32(b[0:4]))) +
		float64(binary.LittleEndian.Uint32(b[4:8]))/4294967296
}

// xiValuator — ось устройства (XIValuatorClass).
type xiValuator struct {
	Number   uint16
	Label    uint32 // атом
	Min, Max float64
}

// xiDevice — устройство из ответа XIQueryDevice и его валюаторы.
type xiDevice struct {
	ID        uint16
	Valuators []xiValuator
}

// parseXIQueryDeviceReply разбирает ответ XIQueryDevice целиком (32 байта
// заголовка и хвост). Обрезанное или неправдоподобное — разбор прекращается на
// последнем целом устройстве, а не паникой.
func parseXIQueryDeviceReply(reply []byte) []xiDevice {
	if len(reply) < 32 || reply[0] != 1 {
		return nil
	}
	n := int(binary.LittleEndian.Uint16(reply[8:10]))
	var out []xiDevice
	off := 32
	for i := 0; i < n; i++ {
		if off+12 > len(reply) {
			break
		}
		dev := xiDevice{ID: binary.LittleEndian.Uint16(reply[off : off+2])}
		numClasses := int(binary.LittleEndian.Uint16(reply[off+6 : off+8]))
		nameLen := int(binary.LittleEndian.Uint16(reply[off+8 : off+10]))
		off += 12 + (nameLen+3)&^3
		for c := 0; c < numClasses; c++ {
			if off+4 > len(reply) {
				break
			}
			typ := binary.LittleEndian.Uint16(reply[off : off+2])
			size := int(binary.LittleEndian.Uint16(reply[off+2:off+4])) * 4
			if size < 8 || off+size > len(reply) {
				off = len(reply)
				break
			}
			if typ == xiValuatorClass && size >= 40 {
				dev.Valuators = append(dev.Valuators, xiValuator{
					Number: binary.LittleEndian.Uint16(reply[off+6 : off+8]),
					Label:  binary.LittleEndian.Uint32(reply[off+8 : off+12]),
					Min:    fp3232(reply[off+12 : off+20]),
					Max:    fp3232(reply[off+20 : off+28]),
				})
			}
			off += size
		}
		out = append(out, dev)
	}
	return out
}

// x11TouchAtoms — атомы меток осей касания (0 — такого атома на сервере нет).
type x11TouchAtoms struct {
	pressure, major, minor, orient, posX uint32
}

// x11AxisSel — выбранная ось: есть ли, её номер и пределы.
type x11AxisSel struct {
	ok       bool
	num      uint16
	min, max float64
}

// x11TouchAxes — нужные оси одного сенсорного устройства.
type x11TouchAxes struct {
	pressure, major, minor, orient x11AxisSel
	// pxPerUnit — пикселей экрана на единицу оси положения (для размеров пятна);
	// 0 — перевести нечем, и размеров пятна нет.
	pxPerUnit float64
}

// resolveTouchAxes выбирает оси касания из валюаторов устройства. screenW — ширина
// экрана в пикселях (для перевода единиц пятна в пиксели).
func resolveTouchAxes(dev xiDevice, at x11TouchAtoms, screenW int) x11TouchAxes {
	var ax x11TouchAxes
	pick := func(atom uint32) x11AxisSel {
		if atom == 0 {
			return x11AxisSel{}
		}
		for _, v := range dev.Valuators {
			if v.Label == atom {
				return x11AxisSel{ok: true, num: v.Number, min: v.Min, max: v.Max}
			}
		}
		return x11AxisSel{}
	}
	ax.pressure = pick(at.pressure)
	ax.major = pick(at.major)
	ax.minor = pick(at.minor)
	ax.orient = pick(at.orient)
	// Ось положения X: по метке, а если метки нет, то нулевая — по соглашению XI
	// абсолютные устройства начинают с X.
	pos := pick(at.posX)
	if !pos.ok {
		for _, v := range dev.Valuators {
			if v.Number == 0 {
				pos = x11AxisSel{ok: true, num: 0, min: v.Min, max: v.Max}
			}
		}
	}
	if pos.ok && pos.max > pos.min && screenW > 0 {
		ax.pxPerUnit = float64(screenW) / (pos.max - pos.min)
	}
	return ax
}

// axisValue достаёт из touch-события значение оси n; ok=false — события эта ось
// не несёт (или событие обрезано).
func (e xiTouchEvent) axisValue(n uint16) (float64, bool) {
	b := e.raw
	if len(b) < 80 {
		return 0, false
	}
	buttonsLen := int(binary.LittleEndian.Uint16(b[48:50])) * 4
	maskLen := int(binary.LittleEndian.Uint16(b[50:52])) * 4
	maskOff := 80 + buttonsLen
	valOff := maskOff + maskLen
	if valOff > len(b) || int(n)/8 >= maskLen || b[maskOff+int(n)/8]&(1<<(n%8)) == 0 {
		return 0, false
	}
	idx := 0 // сколько осей с меньшим номером в событии есть до нашей
	for i := 0; i < int(n); i++ {
		if b[maskOff+i/8]&(1<<(i%8)) != 0 {
			idx++
		}
	}
	at := valOff + idx*8
	if at+8 > len(b) {
		return 0, false
	}
	return fp3232(b[at : at+8]), true
}

// x11Contact — последние известные нажим и пятно пальца в единицах осей.
type x11Contact struct {
	pressure, major, minor, orient float64
	has                            [4]bool
}

// x11TouchAxisTracker переводит значения осей в события виджетов. Только цикл
// событий.
type x11TouchAxisTracker struct {
	axes map[uint16]x11TouchAxes // по SourceID: устройство-источник касания
	last map[uint32]*x11Contact  // по Detail: касание
	// asked — источники, ради которых уже просили обновить список устройств: без
	// этого неизвестный источник гнал бы повторные запросы на каждое событие.
	asked map[uint16]bool
}

// setDevices запоминает оси всех устройств (устройство без осей тоже вносится:
// «известно, что давления нет»).
func (tr *x11TouchAxisTracker) setDevices(devs []xiDevice, at x11TouchAtoms, screenW int) {
	tr.axes = make(map[uint16]x11TouchAxes, len(devs))
	for _, d := range devs {
		tr.axes[d.ID] = resolveTouchAxes(d, at, screenW)
	}
}

// apply дописывает в ev нажим и пятно, а возвращает true, если источник события
// неизвестен и стоит обновить список устройств (не чаще раза на источник).
func (tr *x11TouchAxisTracker) apply(ev *widget.TouchEvent, e xiTouchEvent) (refresh bool) {
	ax, known := tr.axes[e.SourceID]
	if !known {
		if tr.asked == nil {
			tr.asked = make(map[uint16]bool)
		}
		if tr.axes != nil && !tr.asked[e.SourceID] {
			tr.asked[e.SourceID] = true
			refresh = true
		}
		return refresh
	}
	c := tr.last[e.Detail]
	if c == nil {
		c = &x11Contact{}
		if tr.last == nil {
			tr.last = make(map[uint32]*x11Contact)
		}
		tr.last[e.Detail] = c
	}
	read := func(sel x11AxisSel, i int, dst *float64) {
		if !sel.ok {
			return
		}
		if v, ok := e.axisValue(sel.num); ok {
			*dst, c.has[i] = v, true
		}
	}
	read(ax.pressure, 0, &c.pressure)
	read(ax.major, 1, &c.major)
	read(ax.minor, 2, &c.minor)
	read(ax.orient, 3, &c.orient)

	if c.has[0] && ax.pressure.max > ax.pressure.min {
		p := (c.pressure - ax.pressure.min) / (ax.pressure.max - ax.pressure.min)
		ev.Pressure = math.Min(math.Max(p, 0), 1)
	}
	if ax.pxPerUnit > 0 && c.has[1] && c.major > 0 {
		ev.Major = c.major * ax.pxPerUnit
		if c.has[2] {
			ev.Minor = c.minor * ax.pxPerUnit
		}
		if ev.Minor > ev.Major {
			ev.Major, ev.Minor = ev.Minor, ev.Major
		}
		lim := math.Max(math.Abs(ax.orient.min), math.Abs(ax.orient.max))
		if c.has[3] && lim > 0 {
			ev.Orientation = c.orient / lim * 90
		}
	}
	if e.Type == xiEvTouchEnd {
		delete(tr.last, e.Detail)
	}
	return false
}
