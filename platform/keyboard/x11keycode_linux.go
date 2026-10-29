//go:build linux && !android

package keyboard

// x11keycode_linux.go — символ по keycode X11 без XKB (запасной путь).

//facadegen:skip
func X11KeycodeToRune(keycode int, shift bool) rune {
	// Упрощённый маппинг для ASCII (полная реализация через XKB/xkbcommon)
	if keycode >= 10 && keycode <= 19 {
		// Number row: keycode 10='1', 19='0'
		if keycode == 19 {
			return '0'
		}
		return rune('1' + keycode - 10)
	}
	// Letters a-z: keycodes 38-58 (evdev)
	letters := "asdfghjkl;'qwertyuiop[]\\zxcvbnm,./"
	idx := keycode - 38
	if idx >= 0 && idx < len(letters) {
		r := rune(letters[idx])
		if shift && r >= 'a' && r <= 'z' {
			r -= 32
		}
		return r
	}
	if keycode == 65 {
		return ' '
	}
	return 0
}
