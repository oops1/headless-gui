package pointer

import (
	"testing"
)

func TestWin32WheelRaw(t *testing.T) {
	cases := []struct {
		delta   int16
		horiz   bool
		dx, dy  float64
		precise bool
	}{
		{120, false, 0, -120, false}, // щелчок вверх: вверх — отрицательно в движке
		{-120, false, 0, 120, false}, // вниз
		{-240, false, 0, 240, false}, // два щелчка разом
		{30, false, 0, -30, true},    // тачпад: не кратно 120
		{-7, false, 0, 7, true},      //
		{120, true, 120, 0, false},   // горизонталь: вправо — положительно
		{-120, true, -120, 0, false}, // влево
		{-45, true, -45, 0, true},    // тачпад по горизонтали
	}
	for _, c := range cases {
		dx, dy, p := Win32WheelRaw(c.delta, c.horiz)
		if dx != c.dx || dy != c.dy || p != c.precise {
			t.Errorf("delta=%d horiz=%v: (%v,%v,%v), ждал (%v,%v,%v)", c.delta, c.horiz, dx, dy, p, c.dx, c.dy, c.precise)
		}
	}
}

func TestX11WheelRaw(t *testing.T) {
	cases := map[byte][2]float64{4: {0, -120}, 5: {0, 120}, 6: {-120, 0}, 7: {120, 0}}
	for b, want := range cases {
		dx, dy, ok := X11WheelRaw(b)
		if !ok || dx != want[0] || dy != want[1] {
			t.Errorf("кнопка %d: (%v,%v,%v), ждал %v", b, dx, dy, ok, want)
		}
	}
	for _, b := range []byte{1, 2, 3, 8, 9} {
		if _, _, ok := X11WheelRaw(b); ok {
			t.Errorf("кнопка %d принята за колесо", b)
		}
	}
}

func TestWaylandAxisRaw(t *testing.T) {
	fixed := func(px float64) int32 { return int32(px * 256) }
	cases := []struct {
		name        string
		value, d, v int32
		raw         float64
		precise     bool
	}{
		// Без discrete (версия 4): ≈10 единиц на щелчок → 120; Precise.
		{"щелчок без discrete", fixed(10), 0, 0, 120, true},
		{"вверх", fixed(-10), 0, 0, -120, true},
		{"тачпад", fixed(2.5), 0, 0, 30, true},
		// С discrete (версия 5): число щелчков решает, дельта — кратная 120.
		{"discrete", fixed(10), 1, 0, 120, false},
		{"два discrete", fixed(20), -2, 0, -240, false},
		// С value120 (версия 8): значение берётся как есть, оно точнее.
		{"value120", fixed(10), 0, 60, 60, false},
		{"value120 приоритетнее discrete", fixed(10), 1, -120, -120, false},
	}
	for _, c := range cases {
		raw, p := WaylandAxisRaw(c.value, c.d, c.v)
		if raw != c.raw || p != c.precise {
			t.Errorf("%s: (%v,%v), ждал (%v,%v)", c.name, raw, p, c.raw, c.precise)
		}
	}
}
