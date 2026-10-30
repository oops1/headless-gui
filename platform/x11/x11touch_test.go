package x11

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// xiEventBytes собирает GenericEvent XI2 касания так, как его шлёт сервер:
// 80 байт фиксированной части xXIDeviceEvent (32 заголовка + 48 хвоста) и
// маски кнопок/значений. Раскладку полей см. в x11touch.go.
func xiEventBytes(opcode byte, evtype uint16, detail uint32, time uint32, x, y float64, flags uint32, extra int) []byte {
	total := 80 + extra
	b := make([]byte, total)
	b[0] = x11GenericEvent
	b[1] = opcode
	binary.LittleEndian.PutUint32(b[4:8], uint32((total-32)/4))
	binary.LittleEndian.PutUint16(b[8:10], evtype)
	binary.LittleEndian.PutUint16(b[10:12], 2) // deviceid: мастер-указатель
	binary.LittleEndian.PutUint32(b[12:16], time)
	binary.LittleEndian.PutUint32(b[16:20], detail)
	binary.LittleEndian.PutUint32(b[40:44], uint32(int32(x*65536)))
	binary.LittleEndian.PutUint32(b[44:48], uint32(int32(y*65536)))
	binary.LittleEndian.PutUint16(b[52:54], 9) // sourceid: сенсорный экран
	binary.LittleEndian.PutUint32(b[56:60], flags)
	return b
}

func TestXI_QueryExtensionRequest(t *testing.T) {
	r := x11QueryExtensionRequest("XInputExtension")
	// 15 букв + 1 байт добивки = 16 байт имени, плюс 8 заголовка.
	if len(r) != 24 {
		t.Fatalf("длина %d, ждал 24", len(r))
	}
	if r[0] != 98 || binary.LittleEndian.Uint16(r[2:4]) != 6 || binary.LittleEndian.Uint16(r[4:6]) != 15 {
		t.Errorf("заголовок: %v", r[:8])
	}
	if string(r[8:23]) != "XInputExtension" {
		t.Errorf("имя %q", r[8:23])
	}
}

func TestXI_QueryVersionRequestAndReply(t *testing.T) {
	r := xiQueryVersionRequest(131, 2, 2)
	want := []byte{131, 47, 2, 0, 2, 0, 2, 0}
	if !bytes.Equal(r, want) {
		t.Errorf("XIQueryVersion: %v, ждал %v", r, want)
	}

	reply := make([]byte, 32)
	reply[0] = 1
	binary.LittleEndian.PutUint16(reply[8:10], 2)
	binary.LittleEndian.PutUint16(reply[10:12], 3)
	if maj, min, ok := parseXIQueryVersionReply(reply); !ok || maj != 2 || min != 3 {
		t.Errorf("ответ: %d.%d %v", maj, min, ok)
	}
	// Ошибка вместо ответа.
	reply[0] = 0
	if _, _, ok := parseXIQueryVersionReply(reply); ok {
		t.Error("ошибка принята за ответ")
	}
}

func TestXI_SelectEventsRequest(t *testing.T) {
	m := xiTouchEventMask()
	// Биты 18, 19, 20 — байт 2, биты 2, 3, 4.
	if m != [4]byte{0, 0, 0x1C, 0} {
		t.Fatalf("маска %v, ждал [0 0 28 0]", m)
	}
	r := xiSelectEventsRequest(131, 0x400001, xiAllMasterDevices, m)
	want := []byte{
		131, 46, 5, 0, // opcode, XISelectEvents, длина 5 слов
		0x01, 0x00, 0x40, 0x00, // window
		1, 0, 0, 0, // num_masks, pad
		1, 0, 1, 0, // deviceid=XIAllMasterDevices, mask_len=1
		0, 0, 0x1C, 0, // маска
	}
	if !bytes.Equal(r, want) {
		t.Errorf("XISelectEvents:\n got  %v\n want %v", r, want)
	}
}

func TestXI_ParseTouchEvent(t *testing.T) {
	const op = 131
	b := xiEventBytes(op, xiEvTouchBegin, 77, 123456, 100.5, 40.25, xiTouchEmulatingPointer, 8)
	e, ok := parseXITouchEvent(b, op)
	if !ok {
		t.Fatal("событие не разобрано")
	}
	if e.Type != xiEvTouchBegin || e.Detail != 77 || e.Time != 123456 || e.EventX != 100.5 || e.EventY != 40.25 ||
		e.Flags != xiTouchEmulatingPointer || e.DeviceID != 2 || e.SourceID != 9 {
		t.Errorf("разбор: %+v", e)
	}
	te := e.touch()
	if te.ID != 77 || te.X != 101 || te.Y != 40 || te.Phase != base.TouchBegan || te.Time != 123456*time.Millisecond {
		t.Errorf("событие виджетов: %+v", te)
	}

	// Отрицательные координаты (палец за краем окна).
	b = xiEventBytes(op, xiEvTouchUpdate, 1, 5, -12.5, -0.25, 0, 0)
	e, _ = parseXITouchEvent(b, op)
	if te := e.touch(); te.X != -12 || te.Y != 0 || te.Phase != base.TouchMoved {
		t.Errorf("отрицательные координаты: %+v", te)
	}

	b = xiEventBytes(op, xiEvTouchEnd, 1, 5, 3, 4, 0, 0)
	e, _ = parseXITouchEvent(b, op)
	if e.touch().Phase != base.TouchEnded {
		t.Errorf("TouchEnd -> %v", e.touch().Phase)
	}
}

func TestXI_ParseRejects(t *testing.T) {
	const op = 131
	good := xiEventBytes(op, xiEvTouchBegin, 1, 1, 1, 1, 0, 0)
	if _, ok := parseXITouchEvent(good, op); !ok {
		t.Fatal("эталон не разобран")
	}
	// Чужое расширение.
	if _, ok := parseXITouchEvent(good, op+1); ok {
		t.Error("событие чужого расширения принято")
	}
	// Не GenericEvent.
	bad := append([]byte(nil), good...)
	bad[0] = 4
	if _, ok := parseXITouchEvent(bad, op); ok {
		t.Error("ButtonPress принят за касание")
	}
	// Другие события XI2 (например, XI_Motion = 6).
	if _, ok := parseXITouchEvent(xiEventBytes(op, 6, 1, 1, 1, 1, 0, 0), op); ok {
		t.Error("XI_Motion принят за касание")
	}
	// Обрезанное.
	if _, ok := parseXITouchEvent(good[:50], op); ok {
		t.Error("обрезанное событие принято")
	}
	// Бит send_event в типе не мешает.
	flagged := append([]byte(nil), good...)
	flagged[0] |= 0x80
	if _, ok := parseXITouchEvent(flagged, op); !ok {
		t.Error("событие с send_event отброшено")
	}
}

func TestXI_TouchGuard(t *testing.T) {
	var g x11TouchGuard
	ev := func(typ uint16, id, tm, flags uint32) xiTouchEvent {
		return xiTouchEvent{Type: typ, Detail: id, Time: tm, Flags: flags}
	}
	if g.swallowsPointer(10) {
		t.Fatal("защита сработала без касаний")
	}
	// Касание без эмуляции указателя защиту не включает.
	g.note(ev(xiEvTouchBegin, 1, 100, 0))
	if g.swallowsPointer(100) {
		t.Error("касание без эмуляции указателя гасит мышь")
	}
	// Эмулирующее касание: пока оно идёт, мышь ядра гасится.
	g.note(ev(xiEvTouchBegin, 2, 110, xiTouchEmulatingPointer))
	if !g.swallowsPointer(111) {
		t.Error("эмулируемый указатель не погашен во время касания")
	}
	// Отрыв: отпускание эмулируемой кнопки приходит с той же меткой времени.
	g.note(ev(xiEvTouchEnd, 2, 150, xiTouchEmulatingPointer))
	if !g.swallowsPointer(150) {
		t.Error("отпускание эмулируемой кнопки не погашено")
	}
	if g.swallowsPointer(151) {
		t.Error("настоящая мышь после касания гасится")
	}
	g.note(ev(xiEvTouchBegin, 3, 200, xiTouchEmulatingPointer))
	g.reset()
	if g.swallowsPointer(200) {
		t.Error("после reset защита осталась")
	}
}

// ─── Нажим и контакт: ответ XIQueryDevice, валюаторы события ────────────────

// putFP3232 записывает число как FP3232 (int32 целая, uint32 дробная часть).
func putFP3232(b []byte, v float64) {
	i := math.Floor(v)
	binary.LittleEndian.PutUint32(b[0:4], uint32(int32(i)))
	binary.LittleEndian.PutUint32(b[4:8], uint32((v-i)*4294967296))
}

// xiQueryDeviceReplyBytes собирает ответ XIQueryDevice, как его шлёт сервер: у
// каждого устройства имя, кнопочный класс (его разбор должен пропустить) и по
// классу на валюатор.
func xiQueryDeviceReplyBytes(devs []xiDevice) []byte {
	var body []byte
	for _, d := range devs {
		name := "dev"
		classes := 1 + len(d.Valuators) // кнопочный + валюаторы
		hdr := make([]byte, 12)
		binary.LittleEndian.PutUint16(hdr[0:2], d.ID)
		binary.LittleEndian.PutUint16(hdr[2:4], 3) // XISlavePointer
		binary.LittleEndian.PutUint16(hdr[6:8], uint16(classes))
		binary.LittleEndian.PutUint16(hdr[8:10], uint16(len(name)))
		hdr[10] = 1
		body = append(body, hdr...)
		body = append(body, name...)
		body = append(body, 0) // добивка имени до 4 байт
		// XIButtonClass на 3 кнопки: заголовок 8, маска 4, три метки по 4 = 24 байта.
		btn := make([]byte, 24)
		binary.LittleEndian.PutUint16(btn[0:2], 1)
		binary.LittleEndian.PutUint16(btn[2:4], 6)
		binary.LittleEndian.PutUint16(btn[4:6], d.ID)
		binary.LittleEndian.PutUint16(btn[6:8], 3)
		body = append(body, btn...)
		for _, v := range d.Valuators {
			c := make([]byte, 44)
			binary.LittleEndian.PutUint16(c[0:2], xiValuatorClass)
			binary.LittleEndian.PutUint16(c[2:4], 11)
			binary.LittleEndian.PutUint16(c[4:6], d.ID)
			binary.LittleEndian.PutUint16(c[6:8], v.Number)
			binary.LittleEndian.PutUint32(c[8:12], v.Label)
			putFP3232(c[12:20], v.Min)
			putFP3232(c[20:28], v.Max)
			body = append(body, c...)
		}
	}
	reply := make([]byte, 32, 32+len(body))
	reply[0] = 1
	binary.LittleEndian.PutUint32(reply[4:8], uint32(len(body)/4))
	binary.LittleEndian.PutUint16(reply[8:10], uint16(len(devs)))
	return append(reply, body...)
}

// xiEventWithAxes — touch-событие с ненулевыми валюаторами axes (номер -> значение)
// и маской кнопок ненулевой длины, чтобы разбор не принимал смещение за константу.
func xiEventWithAxes(opcode byte, evtype uint16, detail uint32, source uint16, axes map[int]float64) []byte {
	maxAxis := -1
	for n := range axes {
		if n > maxAxis {
			maxAxis = n
		}
	}
	maskWords := 0
	if maxAxis >= 0 {
		maskWords = maxAxis/32 + 1
	}
	const buttonWords = 1
	total := 80 + buttonWords*4 + maskWords*4 + len(axes)*8
	b := xiEventBytes(opcode, evtype, detail, 1000, 10, 20, 0, total-80)
	binary.LittleEndian.PutUint16(b[48:50], buttonWords)
	binary.LittleEndian.PutUint16(b[50:52], uint16(maskWords))
	binary.LittleEndian.PutUint16(b[52:54], source)
	maskOff := 80 + buttonWords*4
	for n := range axes {
		b[maskOff+n/8] |= 1 << (n % 8)
	}
	at := maskOff + maskWords*4
	for n := 0; n <= maxAxis; n++ {
		if v, ok := axes[n]; ok {
			putFP3232(b[at:at+8], v)
			at += 8
		}
	}
	return b
}

var testTouchAtoms = x11TouchAtoms{pressure: 501, major: 502, minor: 503, orient: 504, posX: 505}

// touchscreenDevice — сенсорный экран: X (0), Y (1), нажим (2), оси пятна (3, 4),
// угол (5, предел ±180 — угол считается долей от него). Положение 0..4000 на экране шириной 2000 px: пиксель на две единицы.
func touchscreenDevice(id uint16) xiDevice {
	return xiDevice{ID: id, Valuators: []xiValuator{
		{Number: 0, Label: 505, Min: 0, Max: 4000},
		{Number: 1, Label: 506, Min: 0, Max: 2000},
		{Number: 2, Label: 501, Min: 0, Max: 255},
		{Number: 3, Label: 502, Min: 0, Max: 100},
		{Number: 4, Label: 503, Min: 0, Max: 100},
		{Number: 5, Label: 504, Min: -180, Max: 180},
	}}
}

func TestXI_QueryDeviceRequest(t *testing.T) {
	want := []byte{131, 48, 2, 0, 0, 0, 0, 0}
	if got := xiQueryDeviceRequest(131, xiAllDevices); !bytes.Equal(got, want) {
		t.Errorf("XIQueryDevice: %v, ждал %v", got, want)
	}
	r := x11InternAtomRequest("Abs MT Pressure", true)
	// 15 букв + 1 добивка = 16, плюс 8.
	if len(r) != 24 || r[0] != 16 || r[1] != 1 || binary.LittleEndian.Uint16(r[2:4]) != 6 ||
		binary.LittleEndian.Uint16(r[4:6]) != 15 || string(r[8:23]) != "Abs MT Pressure" {
		t.Errorf("InternAtom: %v", r)
	}
	if r := x11InternAtomRequest("ab", false); r[1] != 0 || len(r) != 12 {
		t.Errorf("InternAtom без only_if_exists: %v", r)
	}
}

func TestXI_ParseQueryDeviceReply(t *testing.T) {
	devs := []xiDevice{
		{ID: 2}, // устройство без осей — тоже в списке
		touchscreenDevice(9),
	}
	got := parseXIQueryDeviceReply(xiQueryDeviceReplyBytes(devs))
	if len(got) != 2 || got[0].ID != 2 || len(got[0].Valuators) != 0 || got[1].ID != 9 || len(got[1].Valuators) != 6 {
		t.Fatalf("устройства: %+v", got)
	}
	v := got[1].Valuators[2]
	if v.Number != 2 || v.Label != 501 || v.Min != 0 || v.Max != 255 {
		t.Errorf("ось нажима: %+v", v)
	}
	if o := got[1].Valuators[5]; o.Min != -180 || o.Max != 180 {
		t.Errorf("отрицательный предел: %+v", o)
	}
	// Дробные пределы.
	frac := parseXIQueryDeviceReply(xiQueryDeviceReplyBytes([]xiDevice{{ID: 1, Valuators: []xiValuator{{Number: 0, Label: 1, Min: -1.5, Max: 2.25}}}}))
	if v := frac[0].Valuators[0]; v.Min != -1.5 || v.Max != 2.25 {
		t.Errorf("дробные пределы: %+v", v)
	}
}

// Обрезанный или битый ответ не роняет разбор: любой префикс даёт то, что успело
// уложиться целиком, и ничего больше.
func TestXI_ParseQueryDeviceReplyRobust(t *testing.T) {
	full := xiQueryDeviceReplyBytes([]xiDevice{touchscreenDevice(9), touchscreenDevice(10)})
	for n := 0; n <= len(full); n++ {
		_ = parseXIQueryDeviceReply(full[:n]) // не должно паниковать
	}
	if got := parseXIQueryDeviceReply(full[:len(full)-1]); len(got) != 2 || len(got[1].Valuators) != 5 {
		t.Errorf("обрезанный на байт ответ: %d устройств, осей во втором %d", len(got), len(got[1].Valuators))
	}
	// Ошибка вместо ответа.
	bad := append([]byte(nil), full...)
	bad[0] = 0
	if got := parseXIQueryDeviceReply(bad); got != nil {
		t.Errorf("ошибка принята за ответ: %+v", got)
	}
	// Класс нулевой длины не зацикливает разбор.
	loop := xiQueryDeviceReplyBytes([]xiDevice{touchscreenDevice(9)})
	binary.LittleEndian.PutUint16(loop[32+12+4+2:], 0) // длина кнопочного класса = 0
	_ = parseXIQueryDeviceReply(loop)
}

func TestXI_ResolveTouchAxes(t *testing.T) {
	ax := resolveTouchAxes(touchscreenDevice(9), testTouchAtoms, 2000)
	if !ax.pressure.ok || ax.pressure.num != 2 || ax.pressure.max != 255 {
		t.Errorf("нажим: %+v", ax.pressure)
	}
	if !ax.major.ok || ax.major.num != 3 || !ax.minor.ok || ax.minor.num != 4 || !ax.orient.ok || ax.orient.num != 5 {
		t.Errorf("пятно: %+v %+v %+v", ax.major, ax.minor, ax.orient)
	}
	if ax.pxPerUnit != 0.5 {
		t.Errorf("пикселей на единицу %v, ждал 0.5", ax.pxPerUnit)
	}

	// Атомов на сервере нет — осей нет, остальное не страдает.
	none := resolveTouchAxes(touchscreenDevice(9), x11TouchAtoms{}, 2000)
	if none.pressure.ok || none.major.ok || none.orient.ok {
		t.Errorf("оси найдены без атомов: %+v", none)
	}
	// Метки позиции нет: берётся нулевая ось.
	noPos := resolveTouchAxes(touchscreenDevice(9), x11TouchAtoms{pressure: 501}, 2000)
	if noPos.pxPerUnit != 0.5 {
		t.Errorf("запасная ось положения: %v", noPos.pxPerUnit)
	}
	// Устройство без осей.
	if ax := resolveTouchAxes(xiDevice{ID: 2}, testTouchAtoms, 2000); ax.pressure.ok || ax.pxPerUnit != 0 {
		t.Errorf("устройство без осей: %+v", ax)
	}
	// Ширина экрана неизвестна: размеров пятна нет.
	if ax := resolveTouchAxes(touchscreenDevice(9), testTouchAtoms, 0); ax.pxPerUnit != 0 {
		t.Errorf("пиксели без ширины экрана: %v", ax.pxPerUnit)
	}
}

func TestXI_EventAxisValue(t *testing.T) {
	const op = 131
	b := xiEventWithAxes(op, xiEvTouchUpdate, 3, 9, map[int]float64{0: 1000.5, 2: 128, 5: -45, 35: 7})
	e, ok := parseXITouchEvent(b, op)
	if !ok {
		t.Fatal("событие не разобрано")
	}
	for n, want := range map[uint16]float64{0: 1000.5, 2: 128, 5: -45, 35: 7} {
		if v, ok := e.axisValue(n); !ok || v != want {
			t.Errorf("ось %d: %v, %v, ждал %v", n, v, ok, want)
		}
	}
	// Оси, которой в маске нет, или за пределами маски, у события нет.
	for _, n := range []uint16{1, 3, 4, 34, 36, 200} {
		if _, ok := e.axisValue(n); ok {
			t.Errorf("ось %d, которой в событии нет, нашлась", n)
		}
	}
	// Обрезанные значения: маска есть, а чисел нет.
	cut, _ := parseXITouchEvent(b[:len(b)-12], op)
	if _, ok := cut.axisValue(35); ok {
		t.Error("значение за концом события прочитано")
	}
	// Событие без хвоста.
	plain, _ := parseXITouchEvent(xiEventBytes(op, xiEvTouchBegin, 1, 1, 1, 1, 0, 0), op)
	if _, ok := plain.axisValue(0); ok {
		t.Error("у события без валюаторов нашлась ось")
	}
}

func trackerFor(t *testing.T) *x11TouchAxisTracker {
	t.Helper()
	tr := &x11TouchAxisTracker{}
	tr.setDevices([]xiDevice{touchscreenDevice(9), {ID: 11}}, testTouchAtoms, 2000)
	return tr
}

func applyAxes(tr *x11TouchAxisTracker, evtype uint16, detail uint32, source uint16, axes map[int]float64) (base.TouchEvent, bool) {
	e, _ := parseXITouchEvent(xiEventWithAxes(131, evtype, detail, source, axes), 131)
	ev := e.touch()
	refresh := tr.apply(&ev, e)
	return ev, refresh
}

// Нажим нормируется по пределам оси, пятно переводится в пиксели, угол — в градусы;
// значения, не пришедшие в событии, берутся из предыдущих событий того же касания.
func TestXI_TrackerPressureAndContact(t *testing.T) {
	tr := trackerFor(t)
	ev, refresh := applyAxes(tr, xiEvTouchBegin, 1, 9, map[int]float64{0: 100, 1: 100, 2: 51, 3: 40, 4: 20, 5: 90})
	if refresh {
		t.Fatal("известный источник просит обновления")
	}
	if ev.Pressure != 0.2 {
		t.Errorf("нажим %v, ждал 0.2 (51 из 255)", ev.Pressure)
	}
	if ev.Major != 20 || ev.Minor != 10 || ev.Orientation != 45 {
		t.Errorf("пятно %v x %v под %v, ждал 20 x 10 под 45 (0.5 px/ед, 90 из 180 четверти оборота = 45°)", ev.Major, ev.Minor, ev.Orientation)
	}

	// Событие без осей нажима и пятна: наследует прежние значения.
	ev, _ = applyAxes(tr, xiEvTouchUpdate, 1, 9, map[int]float64{0: 120})
	if ev.Pressure != 0.2 || ev.Major != 20 || ev.Minor != 10 {
		t.Errorf("Update без осей: %+v", ev)
	}
	// Изменившееся значение подхватывается.
	ev, _ = applyAxes(tr, xiEvTouchUpdate, 1, 9, map[int]float64{2: 255})
	if ev.Pressure != 1 {
		t.Errorf("нажим на пределе: %v", ev.Pressure)
	}
	// Другое касание на том же устройстве не наследует чужое.
	ev, _ = applyAxes(tr, xiEvTouchBegin, 2, 9, map[int]float64{0: 5})
	if ev.Pressure != 0 || ev.Major != 0 {
		t.Errorf("новое касание унаследовало чужое: %+v", ev)
	}
	// Отрыв стирает: следующее касание с тем же номером начинает с нуля.
	applyAxes(tr, xiEvTouchEnd, 1, 9, map[int]float64{0: 120})
	ev, _ = applyAxes(tr, xiEvTouchBegin, 1, 9, map[int]float64{0: 5})
	if ev.Pressure != 0 {
		t.Errorf("значения прежнего касания протекли: %+v", ev)
	}
}

// Значение за пределами оси не выходит за 0..1; не отдаётся пятно без большой оси;
// малая не больше большой.
func TestXI_TrackerEdges(t *testing.T) {
	tr := trackerFor(t)
	if ev, _ := applyAxes(tr, xiEvTouchBegin, 1, 9, map[int]float64{2: 9999}); ev.Pressure != 1 {
		t.Errorf("нажим выше предела: %v", ev.Pressure)
	}
	if ev, _ := applyAxes(tr, xiEvTouchBegin, 2, 9, map[int]float64{2: -50}); ev.Pressure != 0 {
		t.Errorf("нажим ниже предела: %v", ev.Pressure)
	}
	if ev, _ := applyAxes(tr, xiEvTouchBegin, 3, 9, map[int]float64{4: 30}); ev.Major != 0 || ev.Minor != 0 {
		t.Errorf("пятно без большой оси: %+v", ev)
	}
	if ev, _ := applyAxes(tr, xiEvTouchBegin, 4, 9, map[int]float64{3: 10, 4: 30}); ev.Major != 15 || ev.Minor != 5 {
		t.Errorf("перепутанные оси: %v x %v, ждал 15 x 5", ev.Major, ev.Minor)
	}
	// Устройство без осей: нули.
	if ev, _ := applyAxes(tr, xiEvTouchBegin, 5, 11, map[int]float64{0: 1}); ev.Pressure != 0 || ev.Major != 0 {
		t.Errorf("устройство без осей: %+v", ev)
	}
}

// Неизвестный источник просит обновить список устройств — один раз на источник,
// и только если список вообще загружался.
func TestXI_TrackerUnknownSourceRefreshesOnce(t *testing.T) {
	tr := trackerFor(t)
	if _, refresh := applyAxes(tr, xiEvTouchBegin, 1, 77, nil); !refresh {
		t.Error("новое устройство не вызвало обновление")
	}
	if _, refresh := applyAxes(tr, xiEvTouchUpdate, 1, 77, nil); refresh {
		t.Error("обновление просится повторно на каждое событие")
	}
	if _, refresh := applyAxes(tr, xiEvTouchBegin, 2, 78, nil); !refresh {
		t.Error("второй новый источник не вызвал обновление")
	}
	// Список не загружался (запрос не удался): тишина, а не поток запросов.
	var empty x11TouchAxisTracker
	if _, refresh := applyAxes(&empty, xiEvTouchBegin, 1, 9, nil); refresh {
		t.Error("пустой трекер просит обновления")
	}
}
