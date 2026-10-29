package keyboard

// rawkey.go — клавиша вместе с сырым скан-кодом (v3.39). Сырое колесо —
// platform/pointer (wheel.go).
//
// Раньше бэкенд сообщал окну только виртуальный код (VK) и только для клавиш из
// таблицы: остальные отбрасывались на месте, а скан-кода не было вовсе. Клиенту
// удалённого рабочего стола (RDP шлёт скан-коды, а не VK) нужны обе вещи: что
// физически нажато и любая клавиша, а не только известные таблице.
//
// Файл без платформенного суффикса намеренно: разбор lParam и типы — чистые
// данные, и проверять их можно на любой машине.

// RawKey — одно нажатие или отпускание клавиши, как его увидел бэкенд.
//
//facadegen:skip
type RawKey struct {
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

// Win32KeyLParam разбирает lParam сообщений WM_KEYDOWN/KEYUP/SYSKEYDOWN/SYSKEYUP:
//
//	биты 16–23 — аппаратный скан-код,
//	бит 24     — «расширенная» клавиша (E0-префикс),
//	бит 30     — клавиша была нажата до этого сообщения (автоповтор).
//
// Для WM_KEYUP бит 30 всегда 1, поэтому повтором его считать нельзя —
// вызывающий смотрит на repeat только для нажатий.
//
//facadegen:skip
func Win32KeyLParam(lparam uintptr) (scan uint16, extended, prevDown bool) {
	scan = uint16((lparam >> 16) & 0xFF)
	extended = lparam&(1<<24) != 0
	prevDown = lparam&(1<<30) != 0
	return
}

// X11ScanCode — сырой скан-код X11: keycode − 8, то есть код evdev.
// Меньше 8 keycode не бывает (протокол X11 резервирует 0–7); такой вернёт 0.
//
//facadegen:skip
func X11ScanCode(keycode byte) uint16 {
	if keycode < 8 {
		return 0
	}
	return uint16(keycode) - 8
}
