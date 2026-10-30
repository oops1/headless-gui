package native

// hittest.go — зоны окна HitArea для перемещения и кнопок заголовка.

// HitArea — что находится в точке окна.
type HitArea int

const (
	// HitClient — обычное содержимое: событие идёт приложению, как и раньше.
	HitClient HitArea = iota
	// HitCaption — полоса заголовка: система сама тащит окно (с прилипанием
	// к краям и раскладками) и разворачивает его по двойному щелчку.
	HitCaption
	// HitMinButton — кнопка «свернуть».
	HitMinButton
	// HitMaxButton — кнопка «развернуть»: наведение на неё показывает макеты
	// привязки Windows 11.
	HitMaxButton
	// HitCloseButton — кнопка «закрыть».
	HitCloseButton
)
