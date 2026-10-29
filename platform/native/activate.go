package native

// activate.go — активация окна: контракт бэкенда и время из DESKTOP_STARTUP_ID.

import (
	"strings"
)

// ActivateBackend — бэкенд умеет выводить окно на передний план.
//
// Отдельный интерфейс (а не метод NativeWindow): сигнатуру NativeWindow менять
// нельзя, а бэкенд без способности просто не отвечает на Activate.
//
//facadegen:skip
type ActivateBackend interface {
	// Activate выводит окно на передний план (разворачивая свёрнутое).
	// Смысл true зависит от платформы и описан у Window.Activate.
	Activate() bool

	// ActivateWithToken то же, но с токеном активации от запустившего
	// процесса (Wayland: xdg_activation_v1; X11: стартовый идентификатор;
	// Win32: токен не нужен, равно Activate).
	ActivateWithToken(token string) bool
}

// StartupIDTime разбирает из идентификатора запуска (DESKTOP_STARTUP_ID,
// startup-notification) метку времени пользовательского действия: идентификатор
// имеет вид "<имя>_TIME<число>", число — время сервера X11 в миллисекундах
// (CurrentTime, 0, не годится: менеджеру нужно настоящее). ok=false — части
// «_TIME» нет, число не разбирается или равно нулю.
//
//facadegen:skip
func StartupIDTime(id string) (t uint32, ok bool) {
	i := strings.LastIndex(id, "_TIME")
	if i < 0 {
		return 0, false
	}
	digits := id[i+len("_TIME"):]
	end := 0
	for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
		end++
	}
	if end == 0 || end > 10 {
		return 0, false
	}
	var v uint64
	for _, c := range digits[:end] {
		v = v*10 + uint64(c-'0')
	}
	if v == 0 || v > 0xFFFFFFFF {
		return 0, false
	}
	return uint32(v), true
}
