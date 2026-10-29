package core

import (
	"os"
	"testing"
)

// Биты mods_locked для Caps/Num/Scroll Lock из keymap композитора (Wayland).

// Настоящий keymap композитора (Weston в WSLg): Caps Lock на Lock, Num Lock на
// Mod2, Scroll Lock ни на чём не висит — как у большинства композиторов.
func TestLockKeysXkb_RealKeymap(t *testing.T) {
	data, err := os.ReadFile("testdata/wslg_keymap.txt")
	if err != nil {
		t.Skipf("нет testdata/wslg_keymap.txt: %v", err)
	}
	m := lkParseKeymapMods(string(data))
	if m.caps != 1<<xkbModIdxLock {
		t.Errorf("caps = %#x, ждал Lock %#x", m.caps, 1<<xkbModIdxLock)
	}
	if m.num != 1<<xkbModIdxMod2 {
		t.Errorf("num = %#x, ждал Mod2 %#x", m.num, 1<<xkbModIdxMod2)
	}
	if m.scroll != 0 {
		t.Errorf("scroll = %#x: Scroll Lock в этом keymap не модификатор", m.scroll)
	}
}

// keymapWith собирает минимальный keymap с заданными клавишами и modifier_map.
func keymapWith(keys, modmaps string) string {
	return `xkb_keymap {
xkb_keycodes "x" {
	<CAPS> = 66;
	<NMLK> = 77;
	<SCLK> = 78;
};
xkb_symbols "x" {
` + keys + `
` + modmaps + `
};
};`
}

func TestLockKeysXkb_ParseMods(t *testing.T) {
	std := `	key <CAPS> {	[       Caps_Lock ] };
	key <NMLK> {	[        Num_Lock ] };
	key <SCLK> {	[     Scroll_Lock ] };
`
	for _, tc := range []struct {
		name string
		km   string
		want lkWlMods
	}{
		{"штатный", keymapWith(std, "modifier_map Lock { <CAPS> };\nmodifier_map Mod2 { <NMLK> };"),
			lkWlMods{caps: 1 << 1, num: 1 << 4}},
		{"Num Lock переставлен на Mod3", keymapWith(std, "modifier_map Lock { <CAPS> };\nmodifier_map Mod3 { <NMLK> };"),
			lkWlMods{caps: 1 << 1, num: 1 << 5}},
		{"Scroll Lock на Mod5", keymapWith(std, "modifier_map Lock { <CAPS> };\nmodifier_map Mod2 { <NMLK> };\nmodifier_map Mod5 { <SCLK> };"),
			lkWlMods{caps: 1 << 1, num: 1 << 4, scroll: 1 << 7}},
		{"в modifier_map сам символ", keymapWith(std, "modifier_map Mod2 { Num_Lock };\nmodifier_map Mod3 { Scroll_Lock };"),
			lkWlMods{caps: 1 << 1, num: 1 << 4, scroll: 1 << 5}},
		{"несколько клавиш в одном модификаторе", keymapWith(std, "modifier_map Mod1 { <LALT>, <RALT> };\nmodifier_map Mod2 { <LVL5>, <NMLK> };"),
			lkWlMods{caps: 1 << 1, num: 1 << 4}},
		{"Caps Lock на Shift (Shift Lock)", keymapWith(std, "modifier_map Shift { <CAPS> };\nmodifier_map Mod2 { <NMLK> };"),
			lkWlMods{caps: 1 << 0, num: 1 << 4}},
		{"символ в другой группе клавиши", keymapWith(
			"	key <NMLK> {	[ NoSymbol ], [ Num_Lock ] };\n", "modifier_map Mod3 { <NMLK> };"),
			lkWlMods{caps: 1 << 1, num: 1 << 5}},
		{"Num_Lock не путается с KP_Num_Lock", keymapWith(
			"	key <NMLK> {	[ KP_Num_Lock ] };\n", "modifier_map Mod3 { <NMLK> };"),
			lkWlMods{caps: 1 << 1, num: 1 << 4}}, // нет такой клавиши — штатный Mod2
		{"нет modifier_map", keymapWith(std, ""), lkWlDefaultMods},
		{"виртуальный модификатор не берётся", keymapWith(std, "modifier_map NumLock { <NMLK> };"), lkWlDefaultMods},
		{"пустой keymap", "", lkWlDefaultMods},
		{"мусор", "xkb_symbols {{{ modifier_map", lkWlDefaultMods},
	} {
		if got := lkParseKeymapMods(tc.km); got != tc.want {
			t.Errorf("%s: %+v, ждал %+v", tc.name, got, tc.want)
		}
	}
}

func TestLockKeysXkb_FromLocked(t *testing.T) {
	std := lkWlDefaultMods
	scroll := lkWlMods{caps: 2, num: 0x10, scroll: 0x80}
	for _, tc := range []struct {
		name   string
		locked uint32
		m      lkWlMods
		want   LockKeys
	}{
		{"ничего", 0, std, 0},
		{"Caps", 2, std, CapsLock},
		{"Num", 0x10, std, NumLock},
		{"Caps и Num", 0x12, std, CapsLock | NumLock},
		{"чужие биты (Shift, Ctrl, Mod4)", 1 | 4 | 0x40, std, 0},
		{"чужие биты рядом с Caps", 2 | 4 | 0x40, std, CapsLock},
		{"Scroll без отображения не ставится", 0x80, std, 0},
		{"Scroll с отображением", 0x80 | 2, scroll, CapsLock | ScrollLock},
		{"нулевая маска не совпадает со всем подряд", 0xFF, lkWlMods{}, 0},
	} {
		if got := lkFromWlLocked(tc.locked, tc.m); got != tc.want {
			t.Errorf("%s: lkFromWlLocked(%#x) = %v, ждал %v", tc.name, tc.locked, got, tc.want)
		}
	}
}
