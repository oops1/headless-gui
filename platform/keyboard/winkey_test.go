package keyboard

import (
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
)

func TestKeysymToVK_Super(t *testing.T) {
	cases := map[uint32]int{
		0xFFEB: native.VK_LWIN, // Super_L
		0xFFEC: native.VK_RWIN, // Super_R
		0x61:   0,              // 'a'
		0xFFE1: 0,              // Shift_L
		0:      0,
	}
	for sym, want := range cases {
		if got := KeysymToVK(sym); got != want {
			t.Errorf("KeysymToVK(%#x) = %#x, ждал %#x", sym, got, want)
		}
	}
	names := map[string]int{
		"Super_L": native.VK_LWIN, "Super_R": native.VK_RWIN,
		"Hyper_L": 0, "a": 0, "": 0,
	}
	for name, want := range names {
		if got := KeysymNameToVK(name); got != want {
			t.Errorf("KeysymNameToVK(%q) = %#x, ждал %#x", name, got, want)
		}
	}
}

// Win, переставленная на нестандартную клавишу, находится по keymap
// композитора (путь Wayland).
func TestXkbKeymap_FindsSuperKeys(t *testing.T) {
	src := `xkb_keymap {
xkb_keycodes "x" { <AE01> = 10; <LWIN> = 133; <RWIN> = 134; <I254> = 254; };
xkb_symbols "y" {
	key <AE01> { [ 1, exclam ] };
	key <LWIN> { [ Super_L ] };
	key <RWIN> { type = "ONE_LEVEL", symbols[Group1] = [ Super_R ] };
	key <I254> { [ Super_L ] };
};
};`
	km := ParseXkbKeymap(src)
	if km == nil {
		t.Fatal("keymap не разобран")
	}
	for code, want := range map[uint32]int{133: native.VK_LWIN, 134: native.VK_RWIN, 254: native.VK_LWIN, 10: 0, 99: 0} {
		if got := km.VKFor(code); got != want {
			t.Errorf("vkFor(%d) = %#x, ждал %#x", code, got, want)
		}
	}
	var nilMap *XkbKeymap
	if nilMap.VKFor(133) != 0 {
		t.Error("nil keymap должен давать 0")
	}
}

func TestIsModifierVK(t *testing.T) {
	for _, vk := range []int{native.VK_SHIFT, native.VK_CONTROL, native.VK_ALT, native.VK_LWIN, native.VK_RWIN} {
		if !IsModifierVK(vk) {
			t.Errorf("0x%02X должна считаться модификатором", vk)
		}
	}
	for _, vk := range []int{0, native.VK_A, native.VK_SPACE, native.VK_APPS} {
		if IsModifierVK(vk) {
			t.Errorf("0x%02X не модификатор", vk)
		}
	}
}
