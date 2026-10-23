package window

import (
	"encoding/binary"
	"testing"
)

// X11: захват указателя — раскладка запросов и разбор сырого движения XInput2.

// xiRawEventBytes собирает GenericEvent XI_RawMotion: axes — номера осей,
// выставленных в маске; transformed и raw — их значения (целая часть и доля
// 32.32 как float).
func xiRawEventBytes(xiOpcode byte, evtype uint16, maskWords int, axes []int, transformed, raw []float64) []byte {
	n := len(axes)
	buf := make([]byte, 32+maskWords*4+2*n*8)
	buf[0] = x11GenericEvent
	buf[1] = xiOpcode
	binary.LittleEndian.PutUint32(buf[4:8], uint32((len(buf)-32)/4))
	binary.LittleEndian.PutUint16(buf[8:10], evtype)
	binary.LittleEndian.PutUint16(buf[22:24], uint16(maskWords))
	for _, a := range axes {
		buf[32+a/8] |= 1 << (a % 8)
	}
	put := func(off int, v float64) {
		ip := int32(v)
		if float64(ip) > v {
			ip--
		}
		frac := uint32((v - float64(ip)) * 4294967296)
		binary.LittleEndian.PutUint32(buf[off:], uint32(ip))
		binary.LittleEndian.PutUint32(buf[off+4:], frac)
	}
	base := 32 + maskWords*4
	for i := range axes {
		put(base+i*8, transformed[i])
		put(base+n*8+i*8, raw[i])
	}
	return buf
}

func TestParseXIRawMotion(t *testing.T) {
	// Оси 0 и 1: преобразованные (с ускорением) 9, 9; сырые 3 и -2.5.
	ev := xiRawEventBytes(131, xiEvRawMotion, 1, []int{0, 1}, []float64{9, 9}, []float64{3, -2.5})
	dx, dy, ok := parseXIRawMotion(ev, 131)
	if !ok || dx != 3 || dy != -2.5 {
		t.Errorf("(%v,%v,%v), ждал сырые (3,-2.5)", dx, dy, ok)
	}
	// Есть и другие оси (колесо — ось 2): X и Y находятся по номерам, а не по
	// порядку первых значений.
	ev = xiRawEventBytes(131, xiEvRawMotion, 1, []int{0, 1, 2}, []float64{1, 2, 3}, []float64{10, 20, 30})
	if dx, dy, ok = parseXIRawMotion(ev, 131); !ok || dx != 10 || dy != 20 {
		t.Errorf("с осью 2: (%v,%v,%v)", dx, dy, ok)
	}
	// Только вертикальная ось: dx нулевой.
	ev = xiRawEventBytes(131, xiEvRawMotion, 1, []int{1}, []float64{4}, []float64{7})
	if dx, dy, ok = parseXIRawMotion(ev, 131); !ok || dx != 0 || dy != 7 {
		t.Errorf("только ось 1: (%v,%v,%v)", dx, dy, ok)
	}
	// Ось 0 пропущена, но выставлена ось 1 и 3: индекс среди значений считается
	// по числу выставленных бит.
	ev = xiRawEventBytes(131, xiEvRawMotion, 1, []int{1, 3}, []float64{4, 5}, []float64{7, 8})
	if _, dy, _ = parseXIRawMotion(ev, 131); dy != 7 {
		t.Errorf("оси 1 и 3: dy = %v, ждал 7", dy)
	}
	// Маска в два слова.
	ev = xiRawEventBytes(131, xiEvRawMotion, 2, []int{0, 1}, []float64{1, 1}, []float64{-1, 1})
	if dx, dy, ok = parseXIRawMotion(ev, 131); !ok || dx != -1 || dy != 1 {
		t.Errorf("маска в два слова: (%v,%v,%v)", dx, dy, ok)
	}
}

func TestParseXIRawMotion_Rejects(t *testing.T) {
	good := xiRawEventBytes(131, xiEvRawMotion, 1, []int{0, 1}, []float64{1, 1}, []float64{1, 1})
	if _, _, ok := parseXIRawMotion(good, 132); ok {
		t.Error("событие чужого расширения принято")
	}
	other := xiRawEventBytes(131, xiEvTouchUpdate, 1, []int{0, 1}, []float64{1, 1}, []float64{1, 1})
	if _, _, ok := parseXIRawMotion(other, 131); ok {
		t.Error("событие другого типа принято")
	}
	if _, _, ok := parseXIRawMotion(good[:good[0]+20], 131); ok {
		t.Error("обрезанное событие принято")
	}
	if _, _, ok := parseXIRawMotion(good[:40], 131); ok {
		t.Error("событие без значений принято")
	}
	notGeneric := append([]byte(nil), good...)
	notGeneric[0] = 6
	if _, _, ok := parseXIRawMotion(notGeneric, 131); ok {
		t.Error("не GenericEvent принят")
	}
	// Заявленная маска длиннее события.
	short := append([]byte(nil), good...)
	binary.LittleEndian.PutUint16(short[22:24], 200)
	if _, _, ok := parseXIRawMotion(short, 131); ok {
		t.Error("маска за пределами события принята")
	}
}

func TestXIRawMotionMask(t *testing.T) {
	m := xiRawMotionMask()
	if m != [4]byte{0, 0, 2, 0} { // бит 17: байт 2, бит 1
		t.Errorf("маска %v, ждал [0 0 2 0]", m)
	}
	// Подписка на корневое окно для всех мастер-устройств.
	req := xiSelectEventsRequest(131, 0x1A5, xiAllMasterDevices, m)
	if binary.LittleEndian.Uint32(req[4:8]) != 0x1A5 || binary.LittleEndian.Uint16(req[12:14]) != xiAllMasterDevices || req[18] != 2 {
		t.Errorf("XISelectEvents: %v", req)
	}
}

func TestX11GrabRequests(t *testing.T) {
	g := x11GrabPointerRequest(0x400001, 0x400001, 0x600)
	if len(g) != 24 || g[0] != 26 || g[1] != 1 || binary.LittleEndian.Uint16(g[2:4]) != 6 {
		t.Fatalf("GrabPointer: %v", g)
	}
	if binary.LittleEndian.Uint32(g[4:8]) != 0x400001 || binary.LittleEndian.Uint32(g[12:16]) != 0x400001 ||
		binary.LittleEndian.Uint32(g[16:20]) != 0x600 {
		t.Errorf("GrabPointer: окно/confine/курсор %v", g[4:20])
	}
	if binary.LittleEndian.Uint16(g[8:10]) != x11GrabEventMask || g[10] != 1 || g[11] != 1 {
		t.Errorf("GrabPointer: маска событий %#x режимы %d %d", binary.LittleEndian.Uint16(g[8:10]), g[10], g[11])
	}
	reply := make([]byte, 32)
	reply[0], reply[1] = 1, 1
	if st, ok := parseGrabPointerReply(reply); !ok || st != 1 {
		t.Errorf("ответ: %d %v", st, ok)
	}
	if _, ok := parseGrabPointerReply(reply[:4]); ok {
		t.Error("обрезанный ответ принят")
	}
	u := x11UngrabPointerRequest()
	if len(u) != 8 || u[0] != 27 || binary.LittleEndian.Uint16(u[2:4]) != 2 {
		t.Errorf("UngrabPointer: %v", u)
	}
	wp := x11WarpPointerRequest(0x400001, 400, -3)
	if len(wp) != 24 || wp[0] != 41 || binary.LittleEndian.Uint16(wp[2:4]) != 6 ||
		binary.LittleEndian.Uint32(wp[4:8]) != 0 || binary.LittleEndian.Uint32(wp[8:12]) != 0x400001 ||
		binary.LittleEndian.Uint16(wp[20:22]) != 400 || int16(binary.LittleEndian.Uint16(wp[22:24])) != -3 {
		t.Errorf("WarpPointer: %v", wp)
	}
	fc := x11FreeCursorRequest(0x77)
	if len(fc) != 8 || fc[0] != 95 || binary.LittleEndian.Uint32(fc[4:8]) != 0x77 {
		t.Errorf("FreeCursor: %v", fc)
	}
}

// Пробуждение цикла — SendEvent себе: событие ClientMessage, а не запрос с
// ответом.
func TestX11WakeRequest(t *testing.T) {
	r := x11WakeRequest(0x400001)
	if len(r) != 44 || r[0] != 25 || r[1] != 0 || binary.LittleEndian.Uint16(r[2:4]) != 11 {
		t.Fatalf("SendEvent: %v", r)
	}
	if binary.LittleEndian.Uint32(r[4:8]) != 0x400001 || binary.LittleEndian.Uint32(r[8:12]) != 0 {
		t.Errorf("SendEvent: окно/маска событий %v", r[4:12])
	}
	ev := r[12:]
	if ev[0] != 33 || ev[1] != 32 || binary.LittleEndian.Uint32(ev[8:12]) != 31 {
		t.Errorf("событие: тип %d формат %d атом %d", ev[0], ev[1], binary.LittleEndian.Uint32(ev[8:12]))
	}
}

func TestX11WarpDeltaAndRecenter(t *testing.T) {
	if dx, dy := x11WarpDelta(410, 290, 400, 300); dx != 10 || dy != -10 {
		t.Errorf("сдвиг (%v,%v), ждал (10,-10)", dx, dy)
	}
	if dx, dy := x11WarpDelta(400, 300, 400, 300); dx != 0 || dy != 0 {
		t.Errorf("сдвиг из центра (%v,%v), ждал нулевой", dx, dy)
	}
	for _, tc := range []struct {
		x, y int
		want bool
	}{
		{400, 300, false}, {450, 340, false}, {599, 300, false},
		{601, 300, true}, {400, 451, true}, {100, 100, true}, {0, 0, true},
	} {
		if got := x11NeedsRecenter(tc.x, tc.y, 800, 600); got != tc.want {
			t.Errorf("(%d,%d): %v, ждал %v", tc.x, tc.y, got, tc.want)
		}
	}
}
