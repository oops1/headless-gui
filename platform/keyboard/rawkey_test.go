package keyboard

import (
	"testing"
)

func TestWin32KeyLParam(t *testing.T) {
	cases := []struct {
		name     string
		lparam   uintptr
		scan     uint16
		ext      bool
		prevDown bool
	}{
		// «A»: скан-код 0x1E в битах 16–23, повтор 1 (бит 0).
		{"A", 0x001E0001, 0x1E, false, false},
		// Стрелка вправо: расширенная (бит 24), скан 0x4D.
		{"Right", 0x014D0001, 0x4D, true, false},
		// Автоповтор: бит 30.
		{"A repeat", 0x401E0001, 0x1E, false, true},
		// Отпускание: бит 30 и бит 31 (transition), скан-код не искажается.
		{"A up", 0xC01E0001, 0x1E, false, true},
		// Правый Ctrl: 0x1D расширенный; левый — тот же скан без E0.
		{"RCtrl", 0x011D0001, 0x1D, true, false},
		{"LCtrl", 0x001D0001, 0x1D, false, false},
		// Верхние биты слова (контекст Alt, бит 29) не попадают в скан-код.
		{"Alt context", 0x20380001, 0x38, false, false},
	}
	for _, c := range cases {
		scan, ext, prev := Win32KeyLParam(c.lparam)
		if scan != c.scan || ext != c.ext || prev != c.prevDown {
			t.Errorf("%s: (%#x, %v, %v), ждал (%#x, %v, %v)", c.name, scan, ext, prev, c.scan, c.ext, c.prevDown)
		}
	}
}

func TestX11ScanCode(t *testing.T) {
	cases := map[byte]uint16{
		8:   0,   // меньше невозможно: 8 — evdev 0
		9:   1,   // Esc
		38:  30,  // A (evdev KEY_A = 30)
		133: 125, // Super_L
		255: 247,
		0:   0, // протокол такого не присылает
		7:   0,
	}
	for kc, want := range cases {
		if got := X11ScanCode(kc); got != want {
			t.Errorf("X11ScanCode(%d) = %d, ждал %d", kc, got, want)
		}
	}
}
