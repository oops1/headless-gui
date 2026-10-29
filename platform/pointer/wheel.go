package pointer

// wheel.go — колесо в единицах оборудования (120 на щелчок) по ОС.

// Win32WheelRaw переводит дельту WM_MOUSEWHEEL / WM_MOUSEHWHEEL (знаковое
// слово старших 16 бит wParam) в единицы сырого колеса движка.
//
// Windows считает вертикаль «вперёд — положительно», движок — «вниз —
// положительно», поэтому вертикальная дельта меняет знак; горизонтальная
// («вправо — положительно») совпадает. Дельта не кратная WHEEL_DELTA — тачпад или
// колесо со свободным вращением.
func Win32WheelRaw(delta int16, horizontal bool) (dx, dy float64, precise bool) {
	precise = delta%120 != 0
	if horizontal {
		return float64(delta), 0, precise
	}
	return 0, -float64(delta), precise
}

// X11WheelRaw переводит кнопку X11 в сырую прокрутку: 4 — вверх, 5 — вниз,
// 6 — влево, 7 — вправо; один щелчок — ровно 120 единиц, Precise = false.
// ok=false — не кнопка колеса.
func X11WheelRaw(button byte) (dx, dy float64, ok bool) {
	switch button {
	case 4:
		return 0, -120, true
	case 5:
		return 0, 120, true
	case 6:
		return -120, 0, true
	case 7:
		return 120, 0, true
	}
	return 0, 0, false
}

// WaylandAxisRaw переводит событие wl_pointer.axis в сырую прокрутку.
//
// value — wl_fixed (24.8), как есть. Композиторы шлют ≈10 единиц на щелчок
// колеса (wlAxisUnitsPerNotch), поэтому прокрутка в единицах 120 — это value / 10
// * 120. discrete — число целых щелчков по этой оси, если композитор его сообщил
// (wl_pointer.axis_discrete, версия 5); value120 — то же в единицах 120
// (wl_pointer.axis_value120, версия 8), 0 — не сообщал. Когда есть любое из
// двух, событие — настоящие щелчки колеса (precise = false) и дельта берётся
// от них, а не от пикселей; иначе отличить мышь от тачпада нечем, и событие
// считается высокоточным.
func WaylandAxisRaw(value int32, discrete, value120 int32) (raw float64, precise bool) {
	switch {
	case value120 != 0:
		return float64(value120), false
	case discrete != 0:
		return float64(discrete) * 120, false
	}
	return float64(value) / 256.0 / wlAxisUnitsPerNotch * 120, true
}

// wlAxisUnitsPerNotch — сколько единиц wl_pointer.axis (после ÷256) приходится
// на один щелчок колеса: так шлют weston, mutter и kwin.
const wlAxisUnitsPerNotch = 10.0
