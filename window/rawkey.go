package window

// rawkey.go — клавиша вместе с сырым скан-кодом (v3.39) и сырое колесо.
//
// Раньше бэкенд сообщал окну только виртуальный код (VK) и только для клавиш из
// таблицы: остальные отбрасывались на месте, а скан-кода не было вовсе. Клиенту
// удалённого рабочего стола (RDP шлёт скан-коды, а не VK) нужны обе вещи: что
// физически нажато и любая клавиша, а не только известные таблице.
//
// Файл без платформенного суффикса намеренно: разбор lParam и типы — чистые
// данные, и проверять их можно на любой машине.

// rawKey — одно нажатие или отпускание клавиши, как его увидел бэкенд.
type rawKey struct {
	// VK — виртуальный код Windows (0 — клавиши нет в таблице бэкенда).
	VK int
	// Scan — сырой скан-код платформы (см. widget.KeyEvent.ScanCode); 0 —
	// неизвестен.
	Scan uint16
	// Extended — признак «расширенной» клавиши (E0-префикс; только Windows).
	Extended bool
	// Pressed — нажатие (true) или отпускание (false).
	Pressed bool
	// Repeat — автоповтор удерживаемой клавиши, а не новое нажатие.
	Repeat bool
}

// rawKeySource — бэкенд умеет сообщать клавиши вместе со скан-кодом, в том
// числе те, которых нет в таблице VK.
//
// Отдельный интерфейс по той же причине, что и keyRepeatSource: менять
// сигнатуры SetOnKeyDown/SetOnKeyUp значит ломать сборку каждому, кто
// реализует NativeWindow. Бэкенд, зарегистрировавший приёмник, больше НЕ
// вызывает SetOnKeyDown/SetOnKeyUp/SetOnKeyDownRepeat: всё идёт сюда. Текстовый
// ввод (SetOnChar) от этого не зависит и идёт как прежде.
type rawKeySource interface {
	SetOnRawKey(fn func(k rawKey))
}

// win32KeyLParam разбирает lParam сообщений WM_KEYDOWN/KEYUP/SYSKEYDOWN/SYSKEYUP:
//
//	биты 16–23 — аппаратный скан-код,
//	бит 24     — «расширенная» клавиша (E0-префикс),
//	бит 30     — клавиша была нажата до этого сообщения (автоповтор).
//
// Для WM_KEYUP бит 30 всегда 1, поэтому повтором его считать нельзя —
// вызывающий смотрит на repeat только для нажатий.
func win32KeyLParam(lparam uintptr) (scan uint16, extended, prevDown bool) {
	scan = uint16((lparam >> 16) & 0xFF)
	extended = lparam&(1<<24) != 0
	prevDown = lparam&(1<<30) != 0
	return
}

// x11ScanCode — сырой скан-код X11: keycode − 8, то есть код evdev.
// Меньше 8 keycode не бывает (протокол X11 резервирует 0–7); такой вернёт 0.
func x11ScanCode(keycode byte) uint16 {
	if keycode < 8 {
		return 0
	}
	return uint16(keycode) - 8
}

// ─── Колесо ──────────────────────────────────────────────────────────────────

// wheelRawSource — бэкенд умеет сообщать колесо в единицах оборудования
// (120 на щелчок) до перевода в пиксели.
//
// Приёмник получает событие и функцию legacy: прежний путь доставки этого же
// события (пиксели или тики кнопок). Окно вызывает legacy само, если виджеты
// сырое колесо не приняли, — так одно движение колеса не прокручивает дерево
// дважды. Бэкенд без приёмника зовёт legacy сразу, как раньше.
type wheelRawSource interface {
	SetOnWheelRaw(fn func(x, y int, dx, dy float64, precise bool, legacy func()))
}

// wheelRawEngine — движок умеет принять сырое колесо (реализует *engine.Engine).
// Возвращает true, если какой-то виджет событие принял.
type wheelRawEngine interface {
	SendMouseWheelRaw(x, y int, dx, dy float64, precise bool) bool
}

// win32WheelRaw переводит дельту WM_MOUSEWHEEL / WM_MOUSEHWHEEL (знаковое
// слово старших 16 бит wParam) в единицы сырого колеса движка.
//
// Windows считает вертикаль «вперёд — положительно», движок — «вниз —
// положительно», поэтому вертикальная дельта меняет знак; горизонтальная
// («вправо — положительно») совпадает. Дельта не кратная WHEEL_DELTA — тачпад или
// колесо со свободным вращением.
func win32WheelRaw(delta int16, horizontal bool) (dx, dy float64, precise bool) {
	precise = delta%120 != 0
	if horizontal {
		return float64(delta), 0, precise
	}
	return 0, -float64(delta), precise
}

// x11WheelRaw переводит кнопку X11 в сырую прокрутку: 4 — вверх, 5 — вниз,
// 6 — влево, 7 — вправо; один щелчок — ровно 120 единиц, Precise = false.
// ok=false — не кнопка колеса.
func x11WheelRaw(button byte) (dx, dy float64, ok bool) {
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

// waylandAxisRaw переводит событие wl_pointer.axis в сырую прокрутку.
//
// value — wl_fixed (24.8), как есть. Композиторы шлют ≈10 единиц на щелчок
// колеса (wlAxisUnitsPerNotch), поэтому прокрутка в единицах 120 — это value / 10
// * 120. discrete — число целых щелчков по этой оси, если композитор его сообщил
// (wl_pointer.axis_discrete, версия 5); value120 — то же в единицах 120
// (wl_pointer.axis_value120, версия 8), 0 — не сообщал. Когда есть любое из
// двух, событие — настоящие щелчки колеса (precise = false) и дельта берётся
// от них, а не от пикселей; иначе отличить мышь от тачпада нечем, и событие
// считается высокоточным.
func waylandAxisRaw(value int32, discrete, value120 int32) (raw float64, precise bool) {
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
