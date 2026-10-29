package core

// x11_lockkeys.go — клавиши-переключатели X11: сборка запросов XKEYBOARD и
// ядерного протокола и разбор ответов. Файл БЕЗ платформенного суффикса — по
// той же причине, что x11touch.go: раскладку байтов можно проверить на любой
// машине. Сокет, цикл событий и обмен — в x11_lockkeys_linux.go.
//
// Откуда берётся состояние (по убыванию предпочтения):
//
//  1. XKB, индикаторы. XkbGetIndicatorState — маска зажжённых индикаторов
//     клавиатуры, а какой бит чей, говорят их имена (XkbGetNames →
//     IndicatorNames): «Caps Lock», «Num Lock», «Scroll Lock», «Kana». Так
//     читают состояние и сами оболочки: индикатор показывает ровно то, что
//     видит пользователь, даже если раскладка нестандартна.
//  2. XKB, зафиксированные модификаторы. Нет индикатора с нужным именем —
//     XkbGetState.lockedMods по маске модификатора, на котором висит клавиша
//     (Lock для Caps Lock; для Num Lock и Scroll Lock маска берётся из
//     GetModifierMapping по символу клавиши).
//  3. Ядерный протокол без XKB: QueryPointer.mask (в ней отражены и
//     зафиксированные модификаторы) по тем же маскам.
//
// Выставление — XkbLatchLockState по маскам модификаторов (пункт 2); без XKB
// состояние извне не поменять.

import (
	"encoding/binary"
	"math/bits"
)

// Значения протокола.
const (
	xkbExtensionName          = "XKEYBOARD"
	xkbMinorUseExtension      = 0
	xkbMinorGetState          = 4
	xkbMinorLatchLockState    = 5
	xkbMinorGetIndicatorState = 12
	xkbMinorGetNames          = 17

	xkbUseCoreKbd = 0x0100 // KB_DEVICESPEC: основная клавиатура

	// xkbNamesIndicatorNames — бит IndicatorNames в маске XkbGetNames.which.
	xkbNamesIndicatorNames = 1 << 8

	x11OpQueryPointer       = 38
	x11OpGetModifierMapping = 119

	// x11LockMask — маска модификатора Lock (Caps Lock) в ядерном протоколе.
	x11LockMask = 1 << 1
	// x11Mod2Mask — штатный модификатор Num Lock.
	x11Mod2Mask = 1 << 4

	// Символы клавиш-переключателей (keysym).
	keysymNumLock    = 0xFF7F
	keysymScrollLock = 0xFF14
	keysymKanaLock   = 0xFF2D
)

// x11LockFlags — порядок флагов в таблицах x11LockMap.
var x11LockFlags = [4]LockKeys{CapsLock, NumLock, ScrollLock, KanaLock}

// x11LockIndicatorNames — как индикатор клавиши называется в XKB. Первое имя в
// списке — основное, остальные — встречающиеся в раскладках варианты.
var x11LockIndicatorNames = [4][]string{
	{"Caps Lock"}, {"Num Lock"}, {"Scroll Lock"}, {"Kana", "Kana Lock"},
}

// x11LockMap — как читать каждый переключатель на этом сервере.
type x11LockMap struct {
	// xkb — расширение XKEYBOARD есть и согласовано.
	xkb bool
	// indicator — маска бита индикатора в XkbGetIndicatorState; 0 — индикатора
	// с таким именем нет.
	indicator [4]uint32
	// mods — маска модификатора клавиши (Lock, Mod1…Mod5); 0 — клавиша не
	// отображена на модификатор.
	mods [4]uint8
}

// needsIndicators — какой-нибудь переключатель читается по индикатору.
func (m x11LockMap) needsIndicators() bool {
	for _, b := range m.indicator {
		if b != 0 {
			return true
		}
	}
	return false
}

// needsMods — какой-нибудь переключатель читается по модификатору (нет
// индикатора, а модификатор известен).
func (m x11LockMap) needsMods() bool {
	for i := range m.mods {
		if m.indicator[i] == 0 && m.mods[i] != 0 {
			return true
		}
	}
	return false
}

// resolve собирает набор переключателей. indicators — XkbGetIndicatorState,
// locked — lockedMods из XkbGetState, pointer — mask из QueryPointer; ненужные
// источники передаются нулями.
func (m x11LockMap) resolve(indicators uint32, locked uint8, pointer uint16) LockKeys {
	var k LockKeys
	for i, f := range x11LockFlags {
		on := false
		switch {
		case m.xkb && m.indicator[i] != 0:
			on = indicators&m.indicator[i] != 0
		case m.xkb && m.mods[i] != 0:
			on = locked&m.mods[i] != 0
		case !m.xkb && m.mods[i] != 0:
			on = pointer&uint16(m.mods[i]) != 0
		}
		if on {
			k |= f
		}
	}
	return k
}

// x11XkbRequest — запрос XKB с четырёхбайтовым телом из deviceSpec: заголовок
// (major, minor, длина в словах) и deviceSpec; остальное — нули.
func x11XkbRequest(major, minor byte, words int) []byte {
	req := make([]byte, words*4)
	req[0], req[1] = major, minor
	binary.LittleEndian.PutUint16(req[2:4], uint16(words))
	binary.LittleEndian.PutUint16(req[4:6], xkbUseCoreKbd)
	return req
}

// x11XkbUseExtensionRequest — XkbUseExtension(1, 0): без него остальные запросы
// XKB недопустимы. Ответ: supported@1, версия сервера @8 и @10.
func x11XkbUseExtensionRequest(major byte) []byte {
	req := make([]byte, 8)
	req[0], req[1] = major, xkbMinorUseExtension
	binary.LittleEndian.PutUint16(req[2:4], 2)
	binary.LittleEndian.PutUint16(req[4:6], 1) // wantedMajor
	binary.LittleEndian.PutUint16(req[6:8], 0) // wantedMinor
	return req
}

// parseXkbUseExtensionReply: сервер поддерживает запрошенную версию.
func parseXkbUseExtensionReply(reply []byte) bool {
	return len(reply) >= 12 && reply[0] == 1 && reply[1] != 0
}

// x11XkbGetStateRequest — XkbGetState(основная клавиатура).
func x11XkbGetStateRequest(major byte) []byte {
	return x11XkbRequest(major, xkbMinorGetState, 2)
}

// parseXkbGetStateReply достаёт зафиксированные модификаторы (lockedMods@11).
func parseXkbGetStateReply(reply []byte) (locked uint8, ok bool) {
	if len(reply) < 24 || reply[0] != 1 {
		return 0, false
	}
	return reply[11], true
}

// x11XkbGetIndicatorStateRequest — XkbGetIndicatorState(основная клавиатура).
func x11XkbGetIndicatorStateRequest(major byte) []byte {
	return x11XkbRequest(major, xkbMinorGetIndicatorState, 2)
}

// parseXkbIndicatorStateReply достаёт маску зажжённых индикаторов (@8).
func parseXkbIndicatorStateReply(reply []byte) (state uint32, ok bool) {
	if len(reply) < 12 || reply[0] != 1 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(reply[8:12]), true
}

// x11XkbGetIndicatorNamesRequest — XkbGetNames(which = IndicatorNames).
func x11XkbGetIndicatorNamesRequest(major byte) []byte {
	req := x11XkbRequest(major, xkbMinorGetNames, 3)
	binary.LittleEndian.PutUint32(req[8:12], xkbNamesIndicatorNames)
	return req
}

// parseXkbIndicatorNamesReply разбирает ответ XkbGetNames, запрошенный только
// ради имён индикаторов: atoms[i] — атом имени индикатора с номером i (бита i
// в XkbGetIndicatorState), 0 — у индикатора имени нет.
//
// Список имён в ответе идёт за заголовком в 32 байта и содержит по атому на
// каждый установленный бит поля indicators (@20) в порядке возрастания. Если
// сервер вернул что-то кроме IndicatorNames, список перед ним имеет
// переменную длину, разобрать его по заголовку нельзя — ответ отбрасывается.
func parseXkbIndicatorNamesReply(reply []byte) (atoms [32]uint32, ok bool) {
	if len(reply) < 32 || reply[0] != 1 {
		return atoms, false
	}
	if binary.LittleEndian.Uint32(reply[8:12]) != xkbNamesIndicatorNames {
		return atoms, false
	}
	present := binary.LittleEndian.Uint32(reply[20:24])
	if need := 32 + 4*bits.OnesCount32(present); len(reply) < need {
		return atoms, false
	}
	off := 32
	for i := 0; i < 32; i++ {
		if present&(1<<i) == 0 {
			continue
		}
		atoms[i] = binary.LittleEndian.Uint32(reply[off : off+4])
		off += 4
	}
	return atoms, true
}

// x11LockIndicatorBits сопоставляет флаги индикаторам по именам. atomOf отдаёт
// атом имени (0 — такого атома на сервере нет, значит, нет и индикатора).
func x11LockIndicatorBits(atoms [32]uint32, atomOf func(name string) uint32) [4]uint32 {
	var out [4]uint32
	for i, names := range x11LockIndicatorNames {
		for _, name := range names {
			a := atomOf(name)
			if a == 0 {
				continue
			}
			for bit, at := range atoms {
				if at == a {
					out[i] = 1 << bit
					break
				}
			}
			if out[i] != 0 {
				break
			}
		}
	}
	return out
}

// x11XkbLatchLockStateRequest — XkbLatchLockState: меняет зафиксированные
// модификаторы. affect — маска затрагиваемых, locks — их новые значения; группу
// и «защёлки» не трогает.
func x11XkbLatchLockStateRequest(major byte, affect, locks uint8) []byte {
	req := make([]byte, 16)
	req[0], req[1] = major, xkbMinorLatchLockState
	binary.LittleEndian.PutUint16(req[2:4], 4)
	binary.LittleEndian.PutUint16(req[4:6], xkbUseCoreKbd)
	req[6] = affect // affectModLocks
	req[7] = locks  // modLocks
	// lockGroup, groupLock, affectModLatches, pad, pad, latchGroup, groupLatch — нули
	return req
}

// x11QueryPointerRequest — QueryPointer(root): в ответе mask с клавишами-
// модификаторами, зафиксированные тоже в ней.
func x11QueryPointerRequest(window uint32) []byte {
	req := make([]byte, 8)
	req[0] = x11OpQueryPointer
	binary.LittleEndian.PutUint16(req[2:4], 2)
	binary.LittleEndian.PutUint32(req[4:8], window)
	return req
}

// parseQueryPointerMask достаёт mask (@24) из ответа QueryPointer.
func parseQueryPointerMask(reply []byte) (mask uint16, ok bool) {
	if len(reply) < 26 || reply[0] != 1 {
		return 0, false
	}
	return binary.LittleEndian.Uint16(reply[24:26]), true
}

// x11GetModifierMappingRequest — GetModifierMapping.
func x11GetModifierMappingRequest() []byte {
	req := make([]byte, 4)
	req[0] = x11OpGetModifierMapping
	binary.LittleEndian.PutUint16(req[2:4], 1)
	return req
}

// parseModifierMappingReply разбирает ответ GetModifierMapping: keycodes —
// восемь групп (Shift, Lock, Control, Mod1…Mod5) по perMod кодов клавиш.
func parseModifierMappingReply(reply []byte) (perMod int, keycodes []byte, ok bool) {
	if len(reply) < 32 || reply[0] != 1 {
		return 0, nil, false
	}
	perMod = int(reply[1])
	if perMod == 0 || len(reply) < 32+8*perMod {
		return 0, nil, false
	}
	return perMod, reply[32 : 32+8*perMod], true
}

// x11ModMaskForKeysym ищет среди модификаторов тот, на котором висит клавиша с
// символом sym: syms отдаёт символы клавиши по её коду. 0 — такого нет.
func x11ModMaskForKeysym(perMod int, keycodes []byte, syms func(code byte) []uint32, sym uint32) uint8 {
	for m := 0; m < 8; m++ {
		for _, code := range keycodes[m*perMod : (m+1)*perMod] {
			if code == 0 {
				continue
			}
			for _, s := range syms(code) {
				if s == sym {
					return 1 << m
				}
			}
		}
	}
	return 0
}
