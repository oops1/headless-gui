package core

import (
	"testing"

	"github.com/oops1/headless-gui/v3/platform/keyboard"
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

// Движок передавал приложению коды только навигационных клавиш, A/C/V/X/Y/Z и
// F1–F12 — всё остальное превращалось в KeyUnknown и отбрасывалось. Поэтому
// недостижимы были Ctrl+S, Ctrl+O, Ctrl+N и прочее, на чём стоит любое
// приложение с меню. Тесты держат полную таблицу и, главное, её привязку к
// ФИЗИЧЕСКОЙ клавише: Ctrl+S обязан работать и при русской раскладке.

func TestVKToKeyCode_Letters(t *testing.T) {
	// Все буквы, а не только те шесть, что знал редактор.
	for vk := native.VK_A; vk <= native.VK_Z; vk++ {
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
		native.VK_S: widget.KeyS,
		native.VK_O: widget.KeyO,
		native.VK_N: widget.KeyN,
		native.VK_F: widget.KeyF,
		native.VK_H: widget.KeyH,
		native.VK_G: widget.KeyG,
		native.VK_W: widget.KeyW,
		native.VK_B: widget.KeyB,
		native.VK_I: widget.KeyI,
		native.VK_K: widget.KeyK,
	}
	for vk, want := range cases {
		if got := vkToKeyCode(vk); got != want {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал %d", vk, got, want)
		}
	}
}

func TestVKToKeyCode_DigitsAndNumpad(t *testing.T) {
	for vk := native.VK_0; vk <= native.VK_9; vk++ {
		if got := vkToKeyCode(vk); int(got) != vk {
			t.Errorf("цифра 0x%02X → %d", vk, got)
		}
	}
	cases := map[int]widget.KeyCode{
		native.VK_NUMPAD0:  widget.KeyNumpad0,
		native.VK_NUMPAD9:  widget.KeyNumpad9,
		native.VK_ADD:      widget.KeyAdd,
		native.VK_SUBTRACT: widget.KeySubtract,
		native.VK_MULTIPLY: widget.KeyMultiply,
		native.VK_DIVIDE:   widget.KeyDivide,
		native.VK_DECIMAL:  widget.KeyDecimal,
	}
	for vk, want := range cases {
		if got := vkToKeyCode(vk); got != want {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал %d", vk, got, want)
		}
	}
}

func TestVKToKeyCode_OemAndSystem(t *testing.T) {
	cases := map[int]widget.KeyCode{
		native.VK_OEM_PLUS:   widget.KeyOemPlus,
		native.VK_OEM_MINUS:  widget.KeyOemMinus,
		native.VK_OEM_COMMA:  widget.KeyOemComma,
		native.VK_OEM_PERIOD: widget.KeyOemPeriod,
		native.VK_OEM_1:      widget.KeyOemSemicolon,
		native.VK_OEM_2:      widget.KeyOemSlash,
		native.VK_OEM_3:      widget.KeyOemTilde,
		native.VK_OEM_4:      widget.KeyOemOpenBrace,
		native.VK_OEM_5:      widget.KeyOemBackslash,
		native.VK_OEM_6:      widget.KeyOemCloseBrace,
		native.VK_OEM_7:      widget.KeyOemQuote,
		native.VK_CAPITAL:    widget.KeyCapsLock,
		native.VK_SNAPSHOT:   widget.KeyPrintScreen,
		native.VK_PAUSE:      widget.KeyPause,
		native.VK_APPS:       widget.KeyMenu,
		native.VK_NUMLOCK:    widget.KeyNumLock,
		native.VK_SCROLL:     widget.KeyScrollLock,
		native.VK_F13:        widget.KeyF13,
		native.VK_F24:        widget.KeyF24,
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
	for _, vk := range []int{0x00, 0x07, 0x0A, 0xFF, 0x5E, 0xE0} {
		if got := vkToKeyCode(vk); got != widget.KeyUnknown {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал KeyUnknown", vk, got)
		}
	}
	// Модификаторы отдельным событием не ходят: их состояние движок
	// получает через SetModifiers.
	for _, vk := range []int{native.VK_SHIFT, native.VK_CONTROL, native.VK_ALT} {
		if got := vkToKeyCode(vk); got != widget.KeyUnknown {
			t.Errorf("модификатор 0x%02X пришёл как клавиша %d", vk, got)
		}
	}
}

// ─── X11 / Wayland: отображение по месту клавиши ────────────────────────────

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
		got := vkToKeyCode(keyboard.X11KeycodeToVK(evdev + 8))
		if got != code {
			t.Errorf("evdev %d дошёл как %d, ждал %d", evdev, got, code)
		}
	}
}
