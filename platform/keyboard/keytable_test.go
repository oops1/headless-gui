package keyboard

import (
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
)

func TestX11KeycodeToVK_PhysicalLayout(t *testing.T) {
	// Ключ — X11 keycode (evdev + 8). Проверяем те места, что нужны меню и
	// редактору: буквы в трёх рядах, цифры, OEM, numpad, навигация.
	cases := map[int]int{
		39:  native.VK_S, // evdev 31 — «S» в QWERTY, «ы» в ЙЦУКЕН
		32:  native.VK_O,
		57:  native.VK_N,
		41:  native.VK_F,
		43:  native.VK_H,
		42:  native.VK_G,
		25:  native.VK_W,
		56:  native.VK_B,
		31:  native.VK_I,
		45:  native.VK_K,
		19:  native.VK_0,
		10:  native.VK_0 + 1,
		21:  native.VK_OEM_PLUS,
		20:  native.VK_OEM_MINUS,
		86:  native.VK_ADD,      // numpad +
		82:  native.VK_SUBTRACT, // numpad −
		63:  native.VK_MULTIPLY,
		106: native.VK_DIVIDE,
		90:  native.VK_NUMPAD0,
		79:  native.VK_NUMPAD7,
		91:  native.VK_DECIMAL,
		104: native.VK_ENTER, // Enter цифровой клавиатуры — тот же код
		66:  native.VK_CAPITAL,
		107: native.VK_SNAPSHOT,
		127: native.VK_PAUSE,
		135: native.VK_APPS,
		110: native.VK_HOME,
		119: native.VK_DELETE,
		95:  native.VK_F11,
		96:  native.VK_F12,
	}
	for keycode, want := range cases {
		if got := X11KeycodeToVK(keycode); got != want {
			t.Errorf("X11KeycodeToVK(%d) = 0x%02X, ждал 0x%02X", keycode, got, want)
		}
	}
}

// Прежние значения не поехали: Explorer и калькулятор собраны на старых
// версиях и должны вести себя как раньше.
func TestX11KeycodeToVK_KeepsOldMapping(t *testing.T) {
	cases := map[int]int{
		22: native.VK_BACKSPACE, 23: native.VK_TAB, 36: native.VK_ENTER, 9: native.VK_ESCAPE, 65: native.VK_SPACE,
		113: native.VK_LEFT, 111: native.VK_UP, 114: native.VK_RIGHT, 116: native.VK_DOWN,
		119: native.VK_DELETE, 118: native.VK_INSERT, 110: native.VK_HOME, 115: native.VK_END,
		112: native.VK_PRIOR, 117: native.VK_NEXT,
		67: native.VK_F1, 76: native.VK_F10, 95: native.VK_F11, 96: native.VK_F12,
		38: native.VK_A, 54: native.VK_C, 55: native.VK_V, 53: native.VK_X, 29: native.VK_Y, 52: native.VK_Z,
		50: native.VK_SHIFT, 62: native.VK_SHIFT, 37: native.VK_CONTROL, 105: native.VK_CONTROL,
		64: native.VK_ALT, 108: native.VK_ALT,
	}
	for keycode, want := range cases {
		if got := X11KeycodeToVK(keycode); got != want {
			t.Errorf("X11KeycodeToVK(%d) = 0x%02X, ждал прежние 0x%02X", keycode, got, want)
		}
	}
	// Незнакомый код по-прежнему ноль.
	if got := X11KeycodeToVK(250); got != 0 {
		t.Errorf("незнакомый keycode дал 0x%02X", got)
	}
}
