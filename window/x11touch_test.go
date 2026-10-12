package window

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
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
	if te.ID != 77 || te.X != 101 || te.Y != 40 || te.Phase != widget.TouchBegan || te.Time != 123456*time.Millisecond {
		t.Errorf("событие виджетов: %+v", te)
	}

	// Отрицательные координаты (палец за краем окна).
	b = xiEventBytes(op, xiEvTouchUpdate, 1, 5, -12.5, -0.25, 0, 0)
	e, _ = parseXITouchEvent(b, op)
	if te := e.touch(); te.X != -12 || te.Y != 0 || te.Phase != widget.TouchMoved {
		t.Errorf("отрицательные координаты: %+v", te)
	}

	b = xiEventBytes(op, xiEvTouchEnd, 1, 5, 3, 4, 0, 0)
	e, _ = parseXITouchEvent(b, op)
	if e.touch().Phase != widget.TouchEnded {
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
