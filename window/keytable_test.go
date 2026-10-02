package window

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Движок передавал приложению коды только навигационных клавиш, A/C/V/X/Y/Z и
// F1–F12 — всё остальное превращалось в KeyUnknown и отбрасывалось. Поэтому
// недостижимы были Ctrl+S, Ctrl+O, Ctrl+N и прочее, на чём стоит любое
// приложение с меню. Тесты держат полную таблицу и, главное, её привязку к
// ФИЗИЧЕСКОЙ клавише: Ctrl+S обязан работать и при русской раскладке.

func TestVKToKeyCode_Letters(t *testing.T) {
	// Все буквы, а не только те шесть, что знал редактор.
	for vk := VK_A; vk <= VK_Z; vk++ {
		got := vkToKeyCode(vk)
		if got == widget.KeyUnknown {
			t.Errorf("буква с кодом 0x%02X отброшена", vk)
			continue
		}
		if int(got) != vk {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал %d", vk, got, vk)
		}
	}
	// Поимённо — то, ради чего всё затевалось.
	cases := map[int]widget.KeyCode{
		VK_S: widget.KeyS,
		VK_O: widget.KeyO,
		VK_N: widget.KeyN,
		VK_F: widget.KeyF,
		VK_H: widget.KeyH,
		VK_G: widget.KeyG,
		VK_W: widget.KeyW,
		VK_B: widget.KeyB,
		VK_I: widget.KeyI,
		VK_K: widget.KeyK,
	}
	for vk, want := range cases {
		if got := vkToKeyCode(vk); got != want {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал %d", vk, got, want)
		}
	}
}

func TestVKToKeyCode_DigitsAndNumpad(t *testing.T) {
	for vk := VK_0; vk <= VK_9; vk++ {
		if got := vkToKeyCode(vk); int(got) != vk {
			t.Errorf("цифра 0x%02X → %d", vk, got)
		}
	}
	cases := map[int]widget.KeyCode{
		VK_NUMPAD0:  widget.KeyNumpad0,
		VK_NUMPAD9:  widget.KeyNumpad9,
		VK_ADD:      widget.KeyAdd,
		VK_SUBTRACT: widget.KeySubtract,
		VK_MULTIPLY: widget.KeyMultiply,
		VK_DIVIDE:   widget.KeyDivide,
		VK_DECIMAL:  widget.KeyDecimal,
	}
	for vk, want := range cases {
		if got := vkToKeyCode(vk); got != want {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал %d", vk, got, want)
		}
	}
}

func TestVKToKeyCode_OemAndSystem(t *testing.T) {
	cases := map[int]widget.KeyCode{
		VK_OEM_PLUS:   widget.KeyOemPlus,
		VK_OEM_MINUS:  widget.KeyOemMinus,
		VK_OEM_COMMA:  widget.KeyOemComma,
		VK_OEM_PERIOD: widget.KeyOemPeriod,
		VK_OEM_1:      widget.KeyOemSemicolon,
		VK_OEM_2:      widget.KeyOemSlash,
		VK_OEM_3:      widget.KeyOemTilde,
		VK_OEM_4:      widget.KeyOemOpenBrace,
		VK_OEM_5:      widget.KeyOemBackslash,
		VK_OEM_6:      widget.KeyOemCloseBrace,
		VK_OEM_7:      widget.KeyOemQuote,
		VK_CAPITAL:    widget.KeyCapsLock,
		VK_SNAPSHOT:   widget.KeyPrintScreen,
		VK_PAUSE:      widget.KeyPause,
		VK_APPS:       widget.KeyMenu,
		VK_NUMLOCK:    widget.KeyNumLock,
		VK_SCROLL:     widget.KeyScrollLock,
		VK_F13:        widget.KeyF13,
		VK_F24:        widget.KeyF24,
	}
	for vk, want := range cases {
		if got := vkToKeyCode(vk); got != want {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал %d", vk, got, want)
		}
	}
}

// Код, для которого константы нет, по-прежнему отбрасывается: безымянное
// число приложению бесполезно, а завтрашняя константа молча поменяла бы
// смысл события.
func TestVKToKeyCode_UnknownStaysUnknown(t *testing.T) {
	for _, vk := range []int{0x00, 0x07, 0x0A, 0xFF, 0x5F, 0xE0} {
		if got := vkToKeyCode(vk); got != widget.KeyUnknown {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал KeyUnknown", vk, got)
		}
	}
	// Модификаторы отдельным событием не ходят: их состояние движок
	// получает через SetModifiers.
	for _, vk := range []int{VK_SHIFT, VK_CONTROL, VK_ALT} {
		if got := vkToKeyCode(vk); got != widget.KeyUnknown {
			t.Errorf("модификатор 0x%02X пришёл как клавиша %d", vk, got)
		}
	}
}

// ─── X11 / Wayland: отображение по месту клавиши ────────────────────────────

func TestX11KeycodeToVK_PhysicalLayout(t *testing.T) {
	// Ключ — X11 keycode (evdev + 8). Проверяем те места, что нужны меню и
	// редактору: буквы в трёх рядах, цифры, OEM, numpad, навигация.
	cases := map[int]int{
		39:  VK_S, // evdev 31 — «S» в QWERTY, «ы» в ЙЦУКЕН
		32:  VK_O,
		57:  VK_N,
		41:  VK_F,
		43:  VK_H,
		42:  VK_G,
		25:  VK_W,
		56:  VK_B,
		31:  VK_I,
		45:  VK_K,
		19:  VK_0,
		10:  VK_0 + 1,
		21:  VK_OEM_PLUS,
		20:  VK_OEM_MINUS,
		86:  VK_ADD,      // numpad +
		82:  VK_SUBTRACT, // numpad −
		63:  VK_MULTIPLY,
		106: VK_DIVIDE,
		90:  VK_NUMPAD0,
		79:  VK_NUMPAD7,
		91:  VK_DECIMAL,
		104: VK_ENTER, // Enter цифровой клавиатуры — тот же код
		66:  VK_CAPITAL,
		107: VK_SNAPSHOT,
		127: VK_PAUSE,
		135: VK_APPS,
		110: VK_HOME,
		119: VK_DELETE,
		95:  VK_F11,
		96:  VK_F12,
	}
	for keycode, want := range cases {
		if got := x11KeycodeToVK(keycode); got != want {
			t.Errorf("x11KeycodeToVK(%d) = 0x%02X, ждал 0x%02X", keycode, got, want)
		}
	}
}

// Прежние значения не поехали: Explorer и калькулятор собраны на старых
// версиях и должны вести себя как раньше.
func TestX11KeycodeToVK_KeepsOldMapping(t *testing.T) {
	cases := map[int]int{
		22: VK_BACKSPACE, 23: VK_TAB, 36: VK_ENTER, 9: VK_ESCAPE, 65: VK_SPACE,
		113: VK_LEFT, 111: VK_UP, 114: VK_RIGHT, 116: VK_DOWN,
		119: VK_DELETE, 118: VK_INSERT, 110: VK_HOME, 115: VK_END,
		112: VK_PRIOR, 117: VK_NEXT,
		67: VK_F1, 76: VK_F10, 95: VK_F11, 96: VK_F12,
		38: VK_A, 54: VK_C, 55: VK_V, 53: VK_X, 29: VK_Y, 52: VK_Z,
		50: VK_SHIFT, 62: VK_SHIFT, 37: VK_CONTROL, 105: VK_CONTROL,
		64: VK_ALT, 108: VK_ALT,
	}
	for keycode, want := range cases {
		if got := x11KeycodeToVK(keycode); got != want {
			t.Errorf("x11KeycodeToVK(%d) = 0x%02X, ждал прежние 0x%02X", keycode, got, want)
		}
	}
	// Незнакомый код по-прежнему ноль.
	if got := x11KeycodeToVK(250); got != 0 {
		t.Errorf("незнакомый keycode дал 0x%02X", got)
	}
}

// Главное требование приложения: сочетание узнаётся по месту клавиши, а не по
// символу на ней. Проверяем сквозной путь evdev → VK → KeyCode.
func TestPhysicalKeyReachesShortcut(t *testing.T) {
	// evdev-коды клавиш, на которых в латинской раскладке написаны S, O, N.
	const (
		evdevS = 31
		evdevO = 24
		evdevN = 49
	)
	want := map[int]widget.KeyCode{
		evdevS: widget.KeyS,
		evdevO: widget.KeyO,
		evdevN: widget.KeyN,
	}
	for evdev, code := range want {
		got := vkToKeyCode(x11KeycodeToVK(evdev + 8))
		if got != code {
			t.Errorf("evdev %d дошёл как %d, ждал %d", evdev, got, code)
		}
	}
}
