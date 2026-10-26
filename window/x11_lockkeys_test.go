package window

import (
	"encoding/binary"
	"reflect"
	"testing"
)

// Клавиши-переключатели X11: раскладка запросов XKB и ядерного протокола,
// разбор ответов, сопоставление индикаторов и модификаторов. Обмен с сервером
// на проводе — в x11_lockkeys_linux_test.go.

func TestX11LockKeys_RequestLayout(t *testing.T) {
	const major = 140
	le := binary.LittleEndian

	use := x11XkbUseExtensionRequest(major)
	if len(use) != 8 || use[0] != major || use[1] != 0 || le.Uint16(use[2:4]) != 2 ||
		le.Uint16(use[4:6]) != 1 || le.Uint16(use[6:8]) != 0 {
		t.Errorf("UseExtension: %v", use)
	}

	for _, tc := range []struct {
		name  string
		req   []byte
		minor byte
		words int
	}{
		{"GetState", x11XkbGetStateRequest(major), 4, 2},
		{"GetIndicatorState", x11XkbGetIndicatorStateRequest(major), 12, 2},
		{"GetNames", x11XkbGetIndicatorNamesRequest(major), 17, 3},
		{"LatchLockState", x11XkbLatchLockStateRequest(major, 0x12, 0x10), 5, 4},
	} {
		r := tc.req
		if len(r) != tc.words*4 || r[0] != major || r[1] != tc.minor || int(le.Uint16(r[2:4])) != tc.words {
			t.Errorf("%s: %v", tc.name, r)
		}
		// deviceSpec — основная клавиатура (XkbUseCoreKbd = 0x100).
		if le.Uint16(r[4:6]) != 0x100 {
			t.Errorf("%s: deviceSpec %#x", tc.name, le.Uint16(r[4:6]))
		}
	}
	if n := x11XkbGetIndicatorNamesRequest(major); le.Uint32(n[8:12]) != 1<<8 {
		t.Errorf("GetNames.which = %#x, ждал IndicatorNames (1<<8)", le.Uint32(n[8:12]))
	}
	// LatchLockState: affectModLocks @6, modLocks @7; остальное — нули (группу и
	// защёлки запрос не трогает).
	ll := x11XkbLatchLockStateRequest(major, 0x12, 0x10)
	if ll[6] != 0x12 || ll[7] != 0x10 {
		t.Errorf("LatchLockState: affect %#x, locks %#x", ll[6], ll[7])
	}
	for i := 8; i < 16; i++ {
		if ll[i] != 0 {
			t.Errorf("LatchLockState: байт %d = %#x, ждал 0", i, ll[i])
		}
	}

	qp := x11QueryPointerRequest(0xABCD)
	if len(qp) != 8 || qp[0] != 38 || le.Uint16(qp[2:4]) != 2 || le.Uint32(qp[4:8]) != 0xABCD {
		t.Errorf("QueryPointer: %v", qp)
	}
	mm := x11GetModifierMappingRequest()
	if len(mm) != 4 || mm[0] != 119 || le.Uint16(mm[2:4]) != 1 {
		t.Errorf("GetModifierMapping: %v", mm)
	}
}

func TestX11LockKeys_ParseReplies(t *testing.T) {
	le := binary.LittleEndian

	// UseExtension: supported — байт 1.
	ok := make([]byte, 32)
	ok[0], ok[1] = 1, 1
	if !parseXkbUseExtensionReply(ok) {
		t.Error("supported=1 не принят")
	}
	ok[1] = 0
	if parseXkbUseExtensionReply(ok) {
		t.Error("supported=0 принят")
	}
	if parseXkbUseExtensionReply(nil) || parseXkbUseExtensionReply(make([]byte, 32)) {
		t.Error("пустой или не ответ принят")
	}

	// GetState: lockedMods — байт 11 (mods@8, base@9, latched@10, locked@11).
	st := make([]byte, 32)
	st[0], st[8], st[9], st[10], st[11] = 1, 0x01, 0x02, 0x04, 0x12
	if locked, ok := parseXkbGetStateReply(st); !ok || locked != 0x12 {
		t.Errorf("GetState: locked = %#x, %v; ждал 0x12", locked, ok)
	}
	if _, ok := parseXkbGetStateReply(st[:20]); ok {
		t.Error("обрезанный GetState принят")
	}

	// GetIndicatorState: маска — слово @8.
	ind := make([]byte, 32)
	ind[0] = 1
	le.PutUint32(ind[8:12], 0b101)
	if s, ok := parseXkbIndicatorStateReply(ind); !ok || s != 0b101 {
		t.Errorf("GetIndicatorState: %b, %v", s, ok)
	}
	if _, ok := parseXkbIndicatorStateReply(ind[:8]); ok {
		t.Error("обрезанный GetIndicatorState принят")
	}

	// QueryPointer: mask — @24.
	qp := make([]byte, 32)
	qp[0] = 1
	le.PutUint16(qp[24:26], 0x0012)
	if m, ok := parseQueryPointerMask(qp); !ok || m != 0x12 {
		t.Errorf("QueryPointer: %#x, %v", m, ok)
	}
	if _, ok := parseQueryPointerMask(qp[:20]); ok {
		t.Error("обрезанный QueryPointer принят")
	}
}

// xkbNamesReply собирает ответ XkbGetNames(IndicatorNames): present — поле
// indicators, atoms — имена в порядке возрастания битов.
func xkbNamesReply(which, present uint32, atoms ...uint32) []byte {
	b := make([]byte, 32+4*len(atoms))
	b[0] = 1
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(atoms)))
	binary.LittleEndian.PutUint32(b[8:12], which)
	binary.LittleEndian.PutUint32(b[20:24], present)
	for i, a := range atoms {
		binary.LittleEndian.PutUint32(b[32+4*i:], a)
	}
	return b
}

func TestX11LockKeys_ParseIndicatorNames(t *testing.T) {
	// Индикаторы 0, 1 и 4 имеют имена (атомы 100, 101, 104).
	atoms, ok := parseXkbIndicatorNamesReply(xkbNamesReply(1<<8, 0b10011, 100, 101, 104))
	if !ok || atoms[0] != 100 || atoms[1] != 101 || atoms[4] != 104 || atoms[2] != 0 || atoms[3] != 0 {
		t.Errorf("atoms[:6] = %v, %v", atoms[:6], ok)
	}
	// Сервер вернул не только IndicatorNames: список перед ним переменной длины,
	// разобрать его нельзя.
	if _, ok := parseXkbIndicatorNamesReply(xkbNamesReply(1<<8|1<<2, 1, 100)); ok {
		t.Error("ответ с чужими списками принят")
	}
	// Список короче, чем обещает indicators.
	if _, ok := parseXkbIndicatorNamesReply(xkbNamesReply(1<<8, 0b11, 100)); ok {
		t.Error("обрезанный список принят")
	}
	if _, ok := parseXkbIndicatorNamesReply(nil); ok {
		t.Error("пустой ответ принят")
	}
	// Ни одного индикатора — это не ошибка.
	if atoms, ok := parseXkbIndicatorNamesReply(xkbNamesReply(1<<8, 0)); !ok || atoms != [32]uint32{} {
		t.Errorf("пустой набор: %v, %v", atoms[:3], ok)
	}
}

func TestX11LockKeys_IndicatorBitsByName(t *testing.T) {
	atomOf := map[string]uint32{"Caps Lock": 100, "Num Lock": 101, "Scroll Lock": 102, "Kana": 103}
	var atoms [32]uint32
	atoms[0], atoms[1], atoms[2], atoms[5] = 100, 101, 102, 103
	got := x11LockIndicatorBits(atoms, func(n string) uint32 { return atomOf[n] })
	want := [4]uint32{1 << 0, 1 << 1, 1 << 2, 1 << 5}
	if got != want {
		t.Errorf("%v, ждал %v", got, want)
	}

	// Атома имени нет на сервере (никто его не создавал) — индикатора нет.
	delete(atomOf, "Scroll Lock")
	got = x11LockIndicatorBits(atoms, func(n string) uint32 { return atomOf[n] })
	if got[2] != 0 || got[0] == 0 {
		t.Errorf("без атома Scroll Lock: %v", got)
	}

	// Атом есть, но индикатора с таким именем у клавиатуры нет.
	atoms[1] = 0
	got = x11LockIndicatorBits(atoms, func(n string) uint32 { return atomOf[n] })
	if got[1] != 0 {
		t.Errorf("индикатор без имени сопоставлен: %v", got)
	}

	// Kana под другим именем.
	atoms[5] = 0
	atoms[6] = 104
	got = x11LockIndicatorBits(atoms, func(n string) uint32 {
		if n == "Kana Lock" {
			return 104
		}
		return atomOf[n]
	})
	if got[3] != 1<<6 {
		t.Errorf("Kana Lock: %v", got)
	}
}

func TestX11LockKeys_Resolve(t *testing.T) {
	xkbInd := x11LockMap{xkb: true,
		indicator: [4]uint32{1 << 0, 1 << 1, 1 << 2, 0},
		mods:      [4]uint8{x11LockMask, x11Mod2Mask, 0, 0}}
	// Индикаторы читаются по битам, модификаторы не нужны.
	if got := xkbInd.resolve(0b011, 0, 0); got != CapsLock|NumLock {
		t.Errorf("индикаторы: %v", got)
	}
	// Индикатор важнее модификатора: зафиксированные биты игнорируются.
	if got := xkbInd.resolve(0, x11LockMask|x11Mod2Mask, 0xFFFF); got != 0 {
		t.Errorf("индикаторы пусты, а набор %v", got)
	}
	if got := xkbInd.resolve(0b100, 0, 0); got != ScrollLock {
		t.Errorf("Scroll Lock по индикатору: %v", got)
	}

	// Индикаторов с нужными именами нет — зафиксированные модификаторы.
	xkbMods := x11LockMap{xkb: true, mods: [4]uint8{x11LockMask, x11Mod2Mask, 1 << 5, 0}}
	if got := xkbMods.resolve(0xFFFF, 0x02, 0xFFFF); got != CapsLock {
		t.Errorf("locked=Lock: %v", got)
	}
	if got := xkbMods.resolve(0, 0x12|1<<5, 0); got != CapsLock|NumLock|ScrollLock {
		t.Errorf("locked=Lock+Mod2+Mod3: %v", got)
	}

	// Без XKB — маска QueryPointer, зафиксированные биты ядерной маски.
	core := x11LockMap{mods: [4]uint8{x11LockMask, x11Mod2Mask, 0, 0}}
	if got := core.resolve(0xFFFF, 0xFF, 0x0012); got != CapsLock|NumLock {
		t.Errorf("ядерная маска: %v", got)
	}
	// Нажатый Shift (бит 0) и Control (бит 2) переключателями не считаются.
	if got := core.resolve(0, 0, 0x0005); got != 0 {
		t.Errorf("Shift+Ctrl приняты за переключатели: %v", got)
	}
	// Клавиша без модификатора и без индикатора не может быть включена.
	if got := (x11LockMap{xkb: true}).resolve(0xFFFF, 0xFF, 0xFFFF); got != 0 {
		t.Errorf("неизвестные клавиши включены: %v", got)
	}

	if !xkbInd.needsIndicators() || xkbMods.needsIndicators() || core.needsIndicators() {
		t.Error("needsIndicators")
	}
	// Caps читается по индикатору, Num — по модификатору: нужны оба источника.
	mixed := x11LockMap{xkb: true, indicator: [4]uint32{1, 0, 0, 0}, mods: [4]uint8{x11LockMask, x11Mod2Mask, 0, 0}}
	if !mixed.needsIndicators() || !mixed.needsMods() {
		t.Error("смешанная карта: ждал оба источника")
	}
	if xkbInd.needsMods() || !xkbMods.needsMods() || !core.needsMods() {
		t.Error("needsMods")
	}
}

func TestX11LockKeys_ModifierMapping(t *testing.T) {
	// Ответ GetModifierMapping: по 2 кода на модификатор.
	reply := make([]byte, 32+16)
	reply[0], reply[1] = 1, 2
	codes := []byte{
		50, 62, // Shift
		66, 0, // Lock
		37, 105, // Control
		64, 108, // Mod1
		0, 77, // Mod2: Num Lock на втором месте
		78, 0, // Mod3: Scroll Lock
		133, 134, // Mod4
		203, 92, // Mod5
	}
	copy(reply[32:], codes)
	perMod, got, ok := parseModifierMappingReply(reply)
	if !ok || perMod != 2 || !reflect.DeepEqual(got, codes) {
		t.Fatalf("parseModifierMappingReply = %d, %v, %v", perMod, got, ok)
	}
	if _, _, ok := parseModifierMappingReply(reply[:40]); ok {
		t.Error("обрезанная карта принята")
	}
	if _, _, ok := parseModifierMappingReply(nil); ok {
		t.Error("пустой ответ принят")
	}
	bad := append([]byte(nil), reply...)
	bad[1] = 0
	if _, _, ok := parseModifierMappingReply(bad); ok {
		t.Error("карта с нулём кодов на модификатор принята")
	}

	syms := func(code byte) []uint32 {
		switch code {
		case 66:
			return []uint32{0xFFE5} // Caps_Lock
		case 77:
			return []uint32{0xFF7F, 0xFF7F} // Num_Lock
		case 78:
			return []uint32{0xFF14} // Scroll_Lock
		case 108:
			return []uint32{0x1008FF00, 0xFF7E} // Mode_switch
		}
		return nil
	}
	if m := x11ModMaskForKeysym(perMod, got, syms, keysymNumLock); m != 1<<4 {
		t.Errorf("Num_Lock: маска %#x, ждал Mod2 (0x10)", m)
	}
	if m := x11ModMaskForKeysym(perMod, got, syms, keysymScrollLock); m != 1<<5 {
		t.Errorf("Scroll_Lock: маска %#x, ждал Mod3 (0x20)", m)
	}
	if m := x11ModMaskForKeysym(perMod, got, syms, keysymKanaLock); m != 0 {
		t.Errorf("Kana_Lock нет, а маска %#x", m)
	}
	// Нулевой код клавиши — «пусто», а не клавиша с кодом 0.
	zero := func(code byte) []uint32 { return []uint32{keysymNumLock} }
	if m := x11ModMaskForKeysym(2, make([]byte, 16), zero, keysymNumLock); m != 0 {
		t.Errorf("пустые слоты нашли клавишу: %#x", m)
	}
}
