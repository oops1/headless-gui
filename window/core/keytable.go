package core

// keytable.go — физические клавиши: X11/Wayland keycode → виртуальный код
// Windows.
//
// Файл без платформенного суффикса намеренно: таблица — чистые данные, и
// проверять её можно на любой машине, а не только там, где есть X-сервер или
// композитор. Пользуются ею native_linux.go (X11) и native_wayland.go
// (evdev + 8).

// isModifierVK — клавиша-модификатор, повторять которую при удержании нечего.
func isModifierVK(vk int) bool {
	switch vk {
	case VK_SHIFT, VK_CONTROL, VK_ALT, VK_LWIN, VK_RWIN:
		return true
	}
	return false
}

func x11KeycodeToVK(keycode int) int {
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
	9: VK_ESCAPE,
	// Цифровой ряд: 10..18 — «1».. «9», 19 — «0».
	10: VK_0 + 1, 11: VK_0 + 2, 12: VK_0 + 3, 13: VK_0 + 4, 14: VK_0 + 5,
	15: VK_0 + 6, 16: VK_0 + 7, 17: VK_0 + 8, 18: VK_0 + 9, 19: VK_0,
	20: VK_OEM_MINUS, 21: VK_OEM_PLUS, 22: VK_BACKSPACE, 23: VK_TAB,
	// Верхний буквенный ряд.
	24: VK_Q, 25: VK_W, 26: VK_E, 27: VK_R, 28: VK_T,
	29: VK_Y, 30: VK_U, 31: VK_I, 32: VK_O, 33: VK_P,
	34: VK_OEM_4, 35: VK_OEM_6, 36: VK_ENTER, 37: VK_CONTROL,
	// Средний ряд.
	38: VK_A, 39: VK_S, 40: VK_D, 41: VK_F, 42: VK_G,
	43: VK_H, 44: VK_J, 45: VK_K, 46: VK_L,
	47: VK_OEM_1, 48: VK_OEM_7, 49: VK_OEM_3, 50: VK_SHIFT, 51: VK_OEM_5,
	// Нижний ряд.
	52: VK_Z, 53: VK_X, 54: VK_C, 55: VK_V, 56: VK_B, 57: VK_N, 58: VK_M,
	59: VK_OEM_COMMA, 60: VK_OEM_PERIOD, 61: VK_OEM_2, 62: VK_SHIFT,
	63: VK_MULTIPLY, 64: VK_ALT, 65: VK_SPACE, 66: VK_CAPITAL,
	// F1–F10 подряд, F11 и F12 отдельно (так лежат evdev-коды).
	67: VK_F1, 68: VK_F2, 69: VK_F3, 70: VK_F4, 71: VK_F5,
	72: VK_F6, 73: VK_F7, 74: VK_F8, 75: VK_F9, 76: VK_F10,
	77: VK_NUMLOCK, 78: VK_SCROLL,
	// Цифровая клавиатура.
	79: VK_NUMPAD7, 80: VK_NUMPAD8, 81: VK_NUMPAD9, 82: VK_SUBTRACT,
	83: VK_NUMPAD4, 84: VK_NUMPAD5, 85: VK_NUMPAD6, 86: VK_ADD,
	87: VK_NUMPAD1, 88: VK_NUMPAD2, 89: VK_NUMPAD3, 90: VK_NUMPAD0,
	91: VK_DECIMAL,
	95: VK_F11, 96: VK_F12,
	// Enter цифровой клавиатуры — тот же VK_ENTER, что и основной: своего
	// виртуального кода у него нет и в Windows.
	104: VK_ENTER, 105: VK_CONTROL, 106: VK_DIVIDE, 107: VK_SNAPSHOT,
	108: VK_ALT,
	// Навигация.
	110: VK_HOME, 111: VK_UP, 112: VK_PRIOR, 113: VK_LEFT, 114: VK_RIGHT,
	115: VK_END, 116: VK_DOWN, 117: VK_NEXT, 118: VK_INSERT, 119: VK_DELETE,
	127: VK_PAUSE, 135: VK_APPS,
	// Клавиши Windows: evdev KEY_LEFTMETA (125) и KEY_RIGHTMETA (126), на
	// стороне X11 и Wayland — Super_L и Super_R. Если раскладка вынесла Super
	// на другую физическую клавишу, её находит keysymToVK (keysym.go).
	133: VK_LWIN, 134: VK_RWIN,

	// Редкие клавиши (v3.39): раньше их не было в таблице, и событие терялось
	// молча. Коды evdev — в комментариях, keycode = evdev + 8.
	//
	// F13–F24 (evdev 183–194).
	191: VK_F13, 192: VK_F13 + 1, 193: VK_F13 + 2, 194: VK_F13 + 3,
	195: VK_F13 + 4, 196: VK_F13 + 5, 197: VK_F13 + 6, 198: VK_F13 + 7,
	199: VK_F13 + 8, 200: VK_F13 + 9, 201: VK_F13 + 10, 202: VK_F24,
	// Японская и корейская раскладки: ZENKAKUHANKAKU 85, HENKAN 92,
	// KATAKANAHIRAGANA 93, MUHENKAN 94, HANGEUL 122, HANJA 123.
	93: VK_KANJI, 100: VK_CONVERT, 101: VK_KANA, 102: VK_NONCONVERT,
	130: VK_KANA, 131: VK_KANJI,
	// Мультимедиа: MUTE 113, VOLUMEDOWN 114, VOLUMEUP 115, NEXTSONG 163,
	// PLAYPAUSE 164, PREVIOUSSONG 165, STOPCD 166.
	121: VK_VOLUME_MUTE, 122: VK_VOLUME_DOWN, 123: VK_VOLUME_UP,
	171: VK_MEDIA_NEXT_TRACK, 172: VK_MEDIA_PLAY_PAUSE,
	173: VK_MEDIA_PREV_TRACK, 174: VK_MEDIA_STOP,
	// Запуск и браузер: MAIL 155, CALC 140, FILE 144, MEDIA 226, BACK 158,
	// FORWARD 159, STOP 128, REFRESH 173, SEARCH 217, BOOKMARKS 156,
	// HOMEPAGE 172, SLEEP 142.
	163: VK_LAUNCH_MAIL, 148: VK_LAUNCH_APP2, 152: VK_LAUNCH_APP1,
	234: VK_LAUNCH_MEDIA,
	166: VK_BROWSER_BACK, 167: VK_BROWSER_FORWARD, 136: VK_BROWSER_STOP,
	181: VK_BROWSER_REFRESH, 225: VK_BROWSER_SEARCH,
	164: VK_BROWSER_FAVORITES, 180: VK_BROWSER_HOME,
	150: VK_SLEEP,
}
