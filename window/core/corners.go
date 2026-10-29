package core

import "github.com/oops1/headless-gui/v3/widget"

// cornerRounder — бэкенд, который может сказать, скругляет ли ОС углы окна.
// Бэкенд без интерфейса считается скругляющим (прежнее поведение).
type cornerRounder interface {
	RoundsCorners() bool
}

// syncSquareCorners сообщает корневому widget.Window, что ОС показывает окно с
// прямыми углами (Windows 10, X11): корпус тогда рисуется прямоугольным, иначе
// за скруглением темы (у Windows 11 — 8 px) был бы виден фон холста — тёмные
// уголки, которые раньше на Windows 10 срезал регион окна.
func (win *Window) syncSquareCorners() {
	cr, ok := win.native.(cornerRounder)
	if !ok || win.eng == nil {
		return
	}
	ww, ok := win.eng.Root().(*widget.Window)
	if !ok {
		return
	}
	square := !cr.RoundsCorners()
	if ww.SquareCorners != square {
		ww.SquareCorners = square
		ww.Invalidate()
	}
}
