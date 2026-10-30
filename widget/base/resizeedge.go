package base

// resizeedge.go — края изменения размера (окно — widget/chrome, панель дока):
// стороны, ширина полосы захвата и наименьший размер окна.

// ResizeEdge — битовая маска задействованных краёв при ресайзе (8 направлений).
//
// Служебный: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
type ResizeEdge int

// Служебные: для подпакетов widget, в фасад не входят.
//
//facadegen:skip
const (
	EdgeNone ResizeEdge = 0
	EdgeN    ResizeEdge = 1 << iota // верхний край
	EdgeS                           // нижний край
	EdgeW                           // левый край
	EdgeE                           // правый край
)

// Служебные: для подпакетов widget, в фасад не входят.
//
//facadegen:skip
const (
	ResizeBorder = 6   // ширина полосы-захвата вдоль каждого края, px
	WindowMinW   = 120 // минимальная ширина окна при ресайзе, px
	WindowMinH   = 80  // минимальная высота окна при ресайзе, px
)
