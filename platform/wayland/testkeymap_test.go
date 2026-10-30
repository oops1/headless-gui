package wayland

// Копия помощника из platform/keyboard (lockkeys_xkb_test.go): тестовые
// помощники между пакетами не делятся.

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
