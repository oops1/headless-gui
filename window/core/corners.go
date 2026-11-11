package core

import (
	"image/color"

	"github.com/oops1/headless-gui/v4/widget"
)

// cornerRounder — бэкенд, который может сказать, скругляет ли ОС углы окна.
// Бэкенд без интерфейса считается скругляющим (прежнее поведение).
type cornerRounder interface {
	RoundsCorners() bool
}

// systemFramer — бэкенд, у которого наружную рамку окна рисует ОС (Windows 11:
// DWM срезает углы и кладёт рамку в 1 px). DrawsFrame — рисует ли она её у
// этого окна сейчас; SetFrameColor задаёт её цвет (set=false — системный).
// Бэкенд без интерфейса рамку не рисует: контур остаётся за движком.
type systemFramer interface {
	DrawsFrame() bool
	SetFrameColor(c color.RGBA, set bool)
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
	sys := false
	ww.SystemFrameColor = nil
	if sf, ok := win.native.(systemFramer); ok && sf.DrawsFrame() {
		// Рамку и углы даёт ОС: свой контур окно не рисует, цвет отдаёт ей.
		sys = true
		ww.SystemFrameColor = sf.SetFrameColor
	}
	if ww.SquareCorners != square || ww.SystemFrame != sys {
		ww.SquareCorners, ww.SystemFrame = square, sys
		ww.Invalidate()
	}
}
