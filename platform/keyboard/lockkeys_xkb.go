package keyboard

// lockkeys_xkb.go — какие биты wl_keyboard.modifiers.mods_locked означают Caps
// Lock, Num Lock и Scroll Lock. Платформонезависимый разбор строк (как
// xkbmap.go): тестируется на всех ОС, используется Wayland-бэкендом.
//
// mods_locked — маска ИНДЕКСОВ модификаторов keymap'а. Реальные модификаторы
// всегда занимают индексы 0..7 в порядке Shift, Lock, Control, Mod1…Mod5; а
// виртуальные (NumLock, ScrollLock…) xkbcommon при выполнении действий
// сворачивает в реальные, на которые они отображены, так что в маске остаются
// только биты реальных. Какой именно реальный модификатор занимает клавиша,
// говорит секция xkb_symbols:
//
//	key <NMLK> { [ Num_Lock ] };
//	modifier_map Mod2 { <NMLK> };
//
// Обычно Caps Lock — это Lock, а Num Lock — Mod2, но раскладка вправе их
// переставить (и переставляет: setxkbmap -option), поэтому индексы берутся из
// keymap'а композитора, а штатные значения — только запасные.

import (
	"regexp"
	"strings"
)

// Индексы реальных модификаторов xkb (биты mods_locked).
const (
	xkbModIdxShift = 0
	//facadegen:skip
	XkbModIdxLock    = 1
	xkbModIdxControl = 2
	xkbModIdxMod1    = 3
	//facadegen:skip
	XkbModIdxMod2 = 4
	//facadegen:skip
	XkbModIdxMod3 = 5
	//facadegen:skip
	XkbModIdxMod4 = 6
	//facadegen:skip
	XkbModIdxMod5 = 7
)

// WlLockMods — маски битов mods_locked для переключателей. Ноль — клавиша не
// отображена на модификатор, её состояния в mods_locked не видно.
//
//facadegen:skip
type WlLockMods struct {
	Caps, Num, Scroll uint32
}

// WlDefaultLockMods — штатное отображение: Caps Lock на Lock, Num Lock на Mod2,
// Scroll Lock ни на чём. Запасное значение, пока keymap не пришёл или не
// разобрался.
//
//facadegen:skip
var WlDefaultLockMods = WlLockMods{Caps: 1 << XkbModIdxLock, Num: 1 << XkbModIdxMod2}

// LocksFromWlLocked переводит mods_locked в набор переключателей.
//
//facadegen:skip
func LocksFromWlLocked(locked uint32, m WlLockMods) LockKeys {
	var k LockKeys
	if m.Caps != 0 && locked&m.Caps != 0 {
		k |= CapsLock
	}
	if m.Num != 0 && locked&m.Num != 0 {
		k |= NumLock
	}
	if m.Scroll != 0 && locked&m.Scroll != 0 {
		k |= ScrollLock
	}
	return k
}

var (
	lkModifierMapRe = regexp.MustCompile(`modifier_map\s+(\w+)\s*\{([^}]*)\}`)
	lkKeyBodyRe     = regexp.MustCompile(`(?m)\bkey\s+<(\w+)>\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}`)
)

// lkRealModIndex — индекс реального модификатора по имени в modifier_map.
func lkRealModIndex(name string) (int, bool) {
	switch strings.ToLower(name) {
	case "shift":
		return xkbModIdxShift, true
	case "lock":
		return XkbModIdxLock, true
	case "control", "ctrl":
		return xkbModIdxControl, true
	case "mod1":
		return xkbModIdxMod1, true
	case "mod2":
		return XkbModIdxMod2, true
	case "mod3":
		return XkbModIdxMod3, true
	case "mod4":
		return XkbModIdxMod4, true
	case "mod5":
		return XkbModIdxMod5, true
	}
	return 0, false
}

// ParseKeymapLockMods разбирает текстовый keymap (формат xkb_v1) и находит
// модификаторы, на которых висят клавиши Caps_Lock, Num_Lock и Scroll_Lock.
// Не нашлось — Caps и Num остаются на штатных Lock и Mod2, Scroll — на нуле
// (у него штатного модификатора нет).
//
//facadegen:skip
func ParseKeymapLockMods(keymap string) WlLockMods {
	m := WlDefaultLockMods
	symbols := parseXkbSection(keymap, "xkb_symbols")
	if symbols == "" {
		return m
	}
	// Тела клавиш: имя → текст между скобками.
	bodies := map[string]string{}
	for _, kb := range lkKeyBodyRe.FindAllStringSubmatch(symbols, -1) {
		bodies[kb[1]] += " " + kb[2]
	}

	// Модификатор клавиши с данным символом: в modifier_map стоит либо сам
	// символ (modifier_map Mod2 { Num_Lock }), либо клавиша <NAME>, в теле
	// которой он есть.
	find := func(sym string) (uint32, bool) {
		word := regexp.MustCompile(`\b` + regexp.QuoteMeta(sym) + `\b`)
		for _, mm := range lkModifierMapRe.FindAllStringSubmatch(symbols, -1) {
			idx, ok := lkRealModIndex(mm[1])
			if !ok {
				continue
			}
			for _, ent := range strings.Split(mm[2], ",") {
				ent = strings.TrimSpace(ent)
				switch {
				case ent == sym:
					return 1 << idx, true
				case strings.HasPrefix(ent, "<") && strings.HasSuffix(ent, ">"):
					if word.MatchString(bodies[trimAngle(ent)]) {
						return 1 << idx, true
					}
				}
			}
		}
		return 0, false
	}
	if mask, ok := find("Caps_Lock"); ok {
		m.Caps = mask
	}
	if mask, ok := find("Num_Lock"); ok {
		m.Num = mask
	}
	m.Scroll = 0
	if mask, ok := find("Scroll_Lock"); ok {
		m.Scroll = mask
	}
	return m
}
