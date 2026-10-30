package core

import (
	"os"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v4/platform/keyboard"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget"
)

// Клавиша Windows раньше отбрасывалась на всех трёх бэкендах: VK_LWIN и
// VK_RWIN не входили в таблицу vkToKeyCode, а физические коды 133/134 — в
// таблицу X11/Wayland, и оболочка не могла открыть «Пуск» по Win. Тесты держат
// все три пути до widget.KeyWin.

func TestVKToKeyCode_WinKeys(t *testing.T) {
	for _, vk := range []int{native.VK_LWIN, native.VK_RWIN} {
		if got := vkToKeyCode(vk); got != widget.KeyWin {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал KeyWin (%d)", vk, got, widget.KeyWin)
		}
	}
	// Значение совпадает с VK_LWIN: код, как у остальных клавиш, виртуальный.
	if int(widget.KeyWin) != native.VK_LWIN {
		t.Errorf("KeyWin = %d, ждал VK_LWIN = %d", widget.KeyWin, native.VK_LWIN)
	}
}

func TestX11KeycodeToVK_WinKeys(t *testing.T) {
	// X11 keycode = evdev + 8: KEY_LEFTMETA 125 → 133, KEY_RIGHTMETA 126 → 134.
	if got := keyboard.X11KeycodeToVK(125 + 8); got != native.VK_LWIN {
		t.Errorf("левая Win: %#x, ждал VK_LWIN", got)
	}
	if got := keyboard.X11KeycodeToVK(126 + 8); got != native.VK_RWIN {
		t.Errorf("правая Win: %#x, ждал VK_RWIN", got)
	}
	// Сквозной путь evdev → VK → KeyCode: обе приходят как KeyWin.
	for _, evdev := range []int{125, 126} {
		if got := vkToKeyCode(keyboard.X11KeycodeToVK(evdev + 8)); got != widget.KeyWin {
			t.Errorf("evdev %d дошёл как %d, ждал KeyWin", evdev, got)
		}
	}
}

func TestXkbKeymap_RealWSLgSuper(t *testing.T) {
	data, err := os.ReadFile("testdata/wslg_keymap.txt")
	if err != nil {
		t.Fatalf("нет testdata/wslg_keymap.txt (данные лежат в репозитории рядом с тестом): %v", err)
	}
	km := keyboard.ParseXkbKeymap(strings.TrimRight(string(data), "\x00"))
	if km == nil {
		t.Fatal("keymap не разобран")
	}
	if got := km.VKFor(133); got != native.VK_LWIN {
		t.Errorf("LWIN: %#x, ждал VK_LWIN", got)
	}
	if got := km.VKFor(134); got != native.VK_RWIN {
		t.Errorf("RWIN: %#x, ждал VK_RWIN", got)
	}
}

// Нажатие Win доходит до движка как KeyWin с ModMeta, а отпускание — без неё;
// другие клавиши при зажатой Win несут ModMeta.
func TestWinKey_EventAndModifier(t *testing.T) {
	s, rec := newAltSurface()

	s.keyEvent(native.VK_LWIN, true)
	s.keyEvent(native.VK_E, true)
	s.keyEvent(native.VK_E, false)
	s.keyEvent(native.VK_LWIN, false)

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
	s.keyEvent(native.VK_RWIN, true)
	s.keyEvent(native.VK_RWIN, false)
	if len(rec.keys) != 2 || rec.keys[0].Code != widget.KeyWin {
		t.Errorf("правая Win: %+v", rec.keys)
	}
}

// Win посреди жеста «Alt нажали и отпустили» делает его сочетанием: строка
// меню по такому Alt не открывается.
func TestWinKey_CancelsAltTap(t *testing.T) {
	s, rec := newAltSurface()
	s.keyEvent(native.VK_ALT, true)
	s.keyEvent(native.VK_LWIN, true)
	s.keyEvent(native.VK_LWIN, false)
	s.keyEvent(native.VK_ALT, false)
	for _, e := range rec.keys {
		if e.Code == widget.KeyAlt {
			t.Fatalf("после Win пришёл KeyAlt: %+v", rec.keys)
		}
	}
}
