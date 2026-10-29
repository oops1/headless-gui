package core

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
	xkbModIdxShift   = 0
	xkbModIdxLock    = 1
	xkbModIdxControl = 2
	xkbModIdxMod1    = 3
	xkbModIdxMod2    = 4
	xkbModIdxMod3    = 5
	xkbModIdxMod4    = 6
	xkbModIdxMod5    = 7
)

// lkWlMods — маски битов mods_locked для переключателей. Ноль — клавиша не
// отображена на модификатор, её состояния в mods_locked не видно.
type lkWlMods struct {
	caps, num, scroll uint32
}

// lkWlDefaultMods — штатное отображение: Caps Lock на Lock, Num Lock на Mod2,
// Scroll Lock ни на чём. Запасное значение, пока keymap не пришёл или не
// разобрался.
var lkWlDefaultMods = lkWlMods{caps: 1 << xkbModIdxLock, num: 1 << xkbModIdxMod2}

// lkFromWlLocked переводит mods_locked в набор переключателей.
func lkFromWlLocked(locked uint32, m lkWlMods) LockKeys {
	var k LockKeys
	if m.caps != 0 && locked&m.caps != 0 {
		k |= CapsLock
	}
	if m.num != 0 && locked&m.num != 0 {
		k |= NumLock
	}
	if m.scroll != 0 && locked&m.scroll != 0 {
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
		return xkbModIdxLock, true
	case "control", "ctrl":
		return xkbModIdxControl, true
	case "mod1":
		return xkbModIdxMod1, true
	case "mod2":
		return xkbModIdxMod2, true
	case "mod3":
		return xkbModIdxMod3, true
	case "mod4":
		return xkbModIdxMod4, true
	case "mod5":
		return xkbModIdxMod5, true
	}
	return 0, false
}

// lkParseKeymapMods разбирает текстовый keymap (формат xkb_v1) и находит
// модификаторы, на которых висят клавиши Caps_Lock, Num_Lock и Scroll_Lock.
// Не нашлось — Caps и Num остаются на штатных Lock и Mod2, Scroll — на нуле
// (у него штатного модификатора нет).
func lkParseKeymapMods(keymap string) lkWlMods {
	m := lkWlDefaultMods
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
		m.caps = mask
	}
	if mask, ok := find("Num_Lock"); ok {
		m.num = mask
	}
	m.scroll = 0
	if mask, ok := find("Scroll_Lock"); ok {
		m.scroll = mask
	}
	return m
}
