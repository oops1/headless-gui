package keyboard

import "github.com/oops1/headless-gui/v4/platform/native"

// keytable.go — физические клавиши: X11/Wayland keycode → виртуальный код
// Windows.
//
// Файл без платформенного суффикса намеренно: таблица — чистые данные, и
// проверять её можно на любой машине, а не только там, где есть X-сервер или
// композитор. Пользуются ею native_linux.go (X11) и native_wayland.go
// (evdev + 8).

// IsModifierVK — клавиша-модификатор, повторять которую при удержании нечего.
//
//facadegen:skip
func IsModifierVK(vk int) bool {
	switch vk {
	case native.VK_SHIFT, native.VK_CONTROL, native.VK_ALT, native.VK_LWIN, native.VK_RWIN:
		return true
	}
	return false
}

//facadegen:skip
func X11KeycodeToVK(keycode int) int {
	if vk, ok := x11VKTable[keycode]; ok {
		return vk
	}
	return 0
}

// x11VKTable — физическая клавиша → виртуальный код Windows.
//
// Ключ — X11 keycode, то есть evdev-код плюс восемь; Wayland отдаёт сырой
// evdev, и бэкенд прибавляет восьмёрку сам. Отображение идёт по МЕСТУ
// клавиши на клавиатуре, а не по символу, который она сейчас печатает:
// Ctrl+S обязан работать и при русской раскладке, где на той же клавише
// написано «ы». Символ приходит отдельным событием (руна), и его даёт
// раскладка — здесь он не участвует.
//
// Коды, которых нет в таблице, окну не помеха: с v3.39 они доходят до
// приложения событием KeyUnknown со скан-кодом (evdev), а не отбрасываются.
var x11VKTable = map[int]int{
	9: native.VK_ESCAPE,
	// Цифровой ряд: 10..18 — «1».. «9», 19 — «0».
	10: native.VK_0 + 1, 11: native.VK_0 + 2, 12: native.VK_0 + 3, 13: native.VK_0 + 4, 14: native.VK_0 + 5,
	15: native.VK_0 + 6, 16: native.VK_0 + 7, 17: native.VK_0 + 8, 18: native.VK_0 + 9, 19: native.VK_0,
	20: native.VK_OEM_MINUS, 21: native.VK_OEM_PLUS, 22: native.VK_BACKSPACE, 23: native.VK_TAB,
	// Верхний буквенный ряд.
	24: native.VK_Q, 25: native.VK_W, 26: native.VK_E, 27: native.VK_R, 28: native.VK_T,
	29: native.VK_Y, 30: native.VK_U, 31: native.VK_I, 32: native.VK_O, 33: native.VK_P,
	34: native.VK_OEM_4, 35: native.VK_OEM_6, 36: native.VK_ENTER, 37: native.VK_CONTROL,
	// Средний ряд.
	38: native.VK_A, 39: native.VK_S, 40: native.VK_D, 41: native.VK_F, 42: native.VK_G,
	43: native.VK_H, 44: native.VK_J, 45: native.VK_K, 46: native.VK_L,
	47: native.VK_OEM_1, 48: native.VK_OEM_7, 49: native.VK_OEM_3, 50: native.VK_SHIFT, 51: native.VK_OEM_5,
	// Нижний ряд.
	52: native.VK_Z, 53: native.VK_X, 54: native.VK_C, 55: native.VK_V, 56: native.VK_B, 57: native.VK_N, 58: native.VK_M,
	59: native.VK_OEM_COMMA, 60: native.VK_OEM_PERIOD, 61: native.VK_OEM_2, 62: native.VK_SHIFT,
	63: native.VK_MULTIPLY, 64: native.VK_ALT, 65: native.VK_SPACE, 66: native.VK_CAPITAL,
	// F1–F10 подряд, F11 и F12 отдельно (так лежат evdev-коды).
	67: native.VK_F1, 68: native.VK_F2, 69: native.VK_F3, 70: native.VK_F4, 71: native.VK_F5,
	72: native.VK_F6, 73: native.VK_F7, 74: native.VK_F8, 75: native.VK_F9, 76: native.VK_F10,
	77: native.VK_NUMLOCK, 78: native.VK_SCROLL,
	// Цифровая клавиатура.
	79: native.VK_NUMPAD7, 80: native.VK_NUMPAD8, 81: native.VK_NUMPAD9, 82: native.VK_SUBTRACT,
	83: native.VK_NUMPAD4, 84: native.VK_NUMPAD5, 85: native.VK_NUMPAD6, 86: native.VK_ADD,
	87: native.VK_NUMPAD1, 88: native.VK_NUMPAD2, 89: native.VK_NUMPAD3, 90: native.VK_NUMPAD0,
	91: native.VK_DECIMAL,
	95: native.VK_F11, 96: native.VK_F12,
	// Enter цифровой клавиатуры — тот же VK_ENTER, что и основной: своего
	// виртуального кода у него нет и в Windows.
	104: native.VK_ENTER, 105: native.VK_CONTROL, 106: native.VK_DIVIDE, 107: native.VK_SNAPSHOT,
	108: native.VK_ALT,
	// Навигация.
	110: native.VK_HOME, 111: native.VK_UP, 112: native.VK_PRIOR, 113: native.VK_LEFT, 114: native.VK_RIGHT,
	115: native.VK_END, 116: native.VK_DOWN, 117: native.VK_NEXT, 118: native.VK_INSERT, 119: native.VK_DELETE,
	127: native.VK_PAUSE, 135: native.VK_APPS,
	// Клавиши Windows: evdev KEY_LEFTMETA (125) и KEY_RIGHTMETA (126), на
	// стороне X11 и Wayland — Super_L и Super_R. Если раскладка вынесла Super
	// на другую физическую клавишу, её находит KeysymToVK (keysym.go).
	133: native.VK_LWIN, 134: native.VK_RWIN,

	// Редкие клавиши (v3.39): раньше их не было в таблице, и событие терялось
	// молча. Коды evdev — в комментариях, keycode = evdev + 8.
	//
	// F13–F24 (evdev 183–194).
	191: native.VK_F13, 192: native.VK_F13 + 1, 193: native.VK_F13 + 2, 194: native.VK_F13 + 3,
	195: native.VK_F13 + 4, 196: native.VK_F13 + 5, 197: native.VK_F13 + 6, 198: native.VK_F13 + 7,
	199: native.VK_F13 + 8, 200: native.VK_F13 + 9, 201: native.VK_F13 + 10, 202: native.VK_F24,
	// Японская и корейская раскладки: ZENKAKUHANKAKU 85, HENKAN 92,
	// KATAKANAHIRAGANA 93, MUHENKAN 94, HANGEUL 122, HANJA 123.
	93: native.VK_KANJI, 100: native.VK_CONVERT, 101: native.VK_KANA, 102: native.VK_NONCONVERT,
	130: native.VK_KANA, 131: native.VK_KANJI,
	// Мультимедиа: MUTE 113, VOLUMEDOWN 114, VOLUMEUP 115, NEXTSONG 163,
	// PLAYPAUSE 164, PREVIOUSSONG 165, STOPCD 166.
	121: native.VK_VOLUME_MUTE, 122: native.VK_VOLUME_DOWN, 123: native.VK_VOLUME_UP,
	171: native.VK_MEDIA_NEXT_TRACK, 172: native.VK_MEDIA_PLAY_PAUSE,
	173: native.VK_MEDIA_PREV_TRACK, 174: native.VK_MEDIA_STOP,
	// Запуск и браузер: MAIL 155, CALC 140, FILE 144, MEDIA 226, BACK 158,
	// FORWARD 159, STOP 128, REFRESH 173, SEARCH 217, BOOKMARKS 156,
	// HOMEPAGE 172, SLEEP 142.
	163: native.VK_LAUNCH_MAIL, 148: native.VK_LAUNCH_APP2, 152: native.VK_LAUNCH_APP1,
	234: native.VK_LAUNCH_MEDIA,
	166: native.VK_BROWSER_BACK, 167: native.VK_BROWSER_FORWARD, 136: native.VK_BROWSER_STOP,
	181: native.VK_BROWSER_REFRESH, 225: native.VK_BROWSER_SEARCH,
	164: native.VK_BROWSER_FAVORITES, 180: native.VK_BROWSER_HOME,
	150: native.VK_SLEEP,
}
