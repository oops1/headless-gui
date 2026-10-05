package window

import (
	"os"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Клавиша Windows раньше отбрасывалась на всех трёх бэкендах: VK_LWIN и
// VK_RWIN не входили в таблицу vkToKeyCode, а физические коды 133/134 — в
// таблицу X11/Wayland, и оболочка не могла открыть «Пуск» по Win. Тесты держат
// все три пути до widget.KeyWin.

func TestVKToKeyCode_WinKeys(t *testing.T) {
	for _, vk := range []int{VK_LWIN, VK_RWIN} {
		if got := vkToKeyCode(vk); got != widget.KeyWin {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал KeyWin (%d)", vk, got, widget.KeyWin)
		}
	}
	// Значение совпадает с VK_LWIN: код, как у остальных клавиш, виртуальный.
	if int(widget.KeyWin) != VK_LWIN {
		t.Errorf("KeyWin = %d, ждал VK_LWIN = %d", widget.KeyWin, VK_LWIN)
	}
}

func TestX11KeycodeToVK_WinKeys(t *testing.T) {
	// X11 keycode = evdev + 8: KEY_LEFTMETA 125 → 133, KEY_RIGHTMETA 126 → 134.
	if got := x11KeycodeToVK(125 + 8); got != VK_LWIN {
		t.Errorf("левая Win: %#x, ждал VK_LWIN", got)
	}
	if got := x11KeycodeToVK(126 + 8); got != VK_RWIN {
		t.Errorf("правая Win: %#x, ждал VK_RWIN", got)
	}
	// Сквозной путь evdev → VK → KeyCode: обе приходят как KeyWin.
	for _, evdev := range []int{125, 126} {
		if got := vkToKeyCode(x11KeycodeToVK(evdev + 8)); got != widget.KeyWin {
			t.Errorf("evdev %d дошёл как %d, ждал KeyWin", evdev, got)
		}
	}
}

func TestKeysymToVK_Super(t *testing.T) {
	cases := map[uint32]int{
		0xFFEB: VK_LWIN, // Super_L
		0xFFEC: VK_RWIN, // Super_R
		0x61:   0,       // 'a'
		0xFFE1: 0,       // Shift_L
		0:      0,
	}
	for sym, want := range cases {
		if got := keysymToVK(sym); got != want {
			t.Errorf("keysymToVK(%#x) = %#x, ждал %#x", sym, got, want)
		}
	}
	names := map[string]int{
		"Super_L": VK_LWIN, "Super_R": VK_RWIN,
		"Hyper_L": 0, "a": 0, "": 0,
	}
	for name, want := range names {
		if got := keysymNameToVK(name); got != want {
			t.Errorf("keysymNameToVK(%q) = %#x, ждал %#x", name, got, want)
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
	km := parseXkbKeymap(src)
	if km == nil {
		t.Fatal("keymap не разобран")
	}
	for code, want := range map[uint32]int{133: VK_LWIN, 134: VK_RWIN, 254: VK_LWIN, 10: 0, 99: 0} {
		if got := km.vkFor(code); got != want {
			t.Errorf("vkFor(%d) = %#x, ждал %#x", code, got, want)
		}
	}
	var nilMap *xkbKeymap
	if nilMap.vkFor(133) != 0 {
		t.Error("nil keymap должен давать 0")
	}
}

func TestXkbKeymap_RealWSLgSuper(t *testing.T) {
	data, err := os.ReadFile("testdata/wslg_keymap.txt")
	if err != nil {
		t.Skipf("нет testdata/wslg_keymap.txt: %v", err)
	}
	km := parseXkbKeymap(strings.TrimRight(string(data), "\x00"))
	if km == nil {
		t.Fatal("keymap не разобран")
	}
	if got := km.vkFor(133); got != VK_LWIN {
		t.Errorf("LWIN: %#x, ждал VK_LWIN", got)
	}
	if got := km.vkFor(134); got != VK_RWIN {
		t.Errorf("RWIN: %#x, ждал VK_RWIN", got)
	}
}

// Нажатие Win доходит до движка как KeyWin с ModMeta, а отпускание — без неё;
// другие клавиши при зажатой Win несут ModMeta.
func TestWinKey_EventAndModifier(t *testing.T) {
	s, rec := newAltSurface()

	s.keyEvent(VK_LWIN, true)
	s.keyEvent(VK_E, true)
	s.keyEvent(VK_E, false)
	s.keyEvent(VK_LWIN, false)

	if len(rec.keys) != 4 {
		t.Fatalf("событий %d, ждал 4: %+v", len(rec.keys), rec.keys)
	}
	if e := rec.keys[0]; e.Code != widget.KeyWin || !e.Pressed || e.Mod&widget.ModMeta == 0 {
		t.Errorf("нажатие Win: %+v, ждал KeyWin с ModMeta", e)
	}
	if e := rec.keys[1]; e.Code != widget.KeyE || e.Mod&widget.ModMeta == 0 {
		t.Errorf("Win+E: %+v, ждал ModMeta", e)
	}
	if e := rec.keys[3]; e.Code != widget.KeyWin || e.Pressed || e.Mod&widget.ModMeta != 0 {
		t.Errorf("отпускание Win: %+v, ждал KeyWin без ModMeta", e)
	}

	// Правая Win — тот же код.
	rec.keys = nil
	s.keyEvent(VK_RWIN, true)
	s.keyEvent(VK_RWIN, false)
	if len(rec.keys) != 2 || rec.keys[0].Code != widget.KeyWin {
		t.Errorf("правая Win: %+v", rec.keys)
	}
}

// Win посреди жеста «Alt нажали и отпустили» делает его сочетанием: строка
// меню по такому Alt не открывается.
func TestWinKey_CancelsAltTap(t *testing.T) {
	s, rec := newAltSurface()
	s.keyEvent(VK_ALT, true)
	s.keyEvent(VK_LWIN, true)
	s.keyEvent(VK_LWIN, false)
	s.keyEvent(VK_ALT, false)
	for _, e := range rec.keys {
		if e.Code == widget.KeyAlt {
			t.Fatalf("после Win пришёл KeyAlt: %+v", rec.keys)
		}
	}
}

func TestIsModifierVK(t *testing.T) {
	for _, vk := range []int{VK_SHIFT, VK_CONTROL, VK_ALT, VK_LWIN, VK_RWIN} {
		if !isModifierVK(vk) {
			t.Errorf("0x%02X должна считаться модификатором", vk)
		}
	}
	for _, vk := range []int{0, VK_A, VK_SPACE, VK_APPS} {
		if isModifierVK(vk) {
			t.Errorf("0x%02X не модификатор", vk)
		}
	}
}
