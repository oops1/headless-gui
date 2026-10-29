package native

// pointerlock.go — контракт захвата указателя.

// PointerLocker — нативное окно умеет захват указателя.
//
//facadegen:skip
type PointerLocker interface {
	// SetPointerLock включает и выключает захват. true при включении —
	// запрос принят: захват действует или начнёт действовать, как только окно
	// получит фокус (Wayland — указатель); false — механизма нет или система
	// отказала. Выключение всегда успешно.
	SetPointerLock(on bool) bool
	// SetOnRelativeMotion — приёмник сдвигов (единицы устройства, дробные).
	// Зовётся с потока бэкенда, часто (сотни раз в секунду).
	SetOnRelativeMotion(fn func(dx, dy float64))
	// SetOnPointerLockChanged — захват начал или перестал действовать:
	// окно потеряло фокус (Win32, X11), компоновщик снял блокировку
	// (Wayland); вернулся фокус — захват восстановлен.
	SetOnPointerLockChanged(fn func(active bool))
}
