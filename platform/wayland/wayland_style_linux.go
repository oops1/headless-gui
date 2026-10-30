//go:build linux && !android

package wayland

// wayland_style_linux.go — стиль окна (native.Styler), ADR-0013 §4.
//
// Wayland умеет только Borderless: рамку и тень окна и так рисует
// приложение, остаётся не предлагать компоновщику тянуть край (BeginResize)
// и не скруглять буфер (маска углов, cornermask). Tool нет: в xdg-shell нет
// ни «без кнопки на панели задач», ни «вне переключателя». Владельца, Z-порядка
// и положения окна тоже нет (OwnerSetter, Stacker и Positioner окно не
// реализует): xdg_toplevel.set_parent принимает только окно того же
// соединения, а у каждого окна своё; стек и место окна решает компоновщик.

import "github.com/oops1/headless-gui/v3/platform/native"

var _ native.Styler = (*WaylandWindow)(nil)

// ApplyStyle реализует native.Styler: Borderless — да, Tool — нет.
func (w *WaylandWindow) ApplyStyle(s native.Style) native.Style {
	w.borderless.Store(s.Borderless)
	return native.Style{Borderless: s.Borderless}
}

// RoundsCorners реализует cornerRounder ядра: окно со Style.Borderless не
// скругляется (маски углов нет), корпус рисуется прямоугольным. Без
// Borderless — true, как у бэкенда без этого интерфейса прежде.
func (w *WaylandWindow) RoundsCorners() bool { return !w.borderless.Load() }
