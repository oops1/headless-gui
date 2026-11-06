package core

// shadow.go — откуда у окна и его попапов тень (ShadowSource).
//
// Тень и скругление окну может дать ОС (DWM Windows) или нарисовать сам
// движок. По умолчанию (ShadowAuto) — ОС, где она их даёт, и движок там, где
// не даёт, а тень нужна: окно без тени DWM (тени окон выключены в системе
// или в сеансе удалённого рабочего стола), прямоугольный попап (DWM тени ему
// не даёт), попапы Windows 10, X11 и Wayland.

import (
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// ShadowSource — кто даёт тень окну и его попапам.
type ShadowSource int

const (
	// ShadowAuto — ОС (DWM), где она даёт тень; где не даёт — движок.
	ShadowAuto ShadowSource = iota
	// ShadowSystem — только ОС: где она тени не даёт, тени нет. Попапы,
	// которым ОС тени не даёт (прямоугольные, Windows 10, Linux), — без тени.
	ShadowSystem
	// ShadowEngine — всегда движок: тень DWM у окна снимается (углы окна
	// тогда прямые), попапы оттеняет и скругляет движок.
	ShadowEngine
)

// shadowSourceSetter — бэкенд выбирает источник тени своего окна
// (Win32: тень DWM или своё окно-тень). src — значение ShadowSource.
type shadowSourceSetter interface {
	SetShadowSource(src int)
}

// engineShadowSetter — бэкенд рисует тень окна сам и принимает её вид:
// размытие и смещение в физических точках, цвет premultiplied.
type engineShadowSetter interface {
	SetEngineShadow(blur, offsetX, offsetY float64, col color.RGBA)
}

// systemPopupShadow — бэкенд сообщает, даёт ли ОС тень и скругление
// скруглённым окнам-попапам (Win32: Windows 11 с DWM и включёнными тенями).
type systemPopupShadow interface {
	SystemPopupShadow() bool
}

// popupSystemDecor — окно-попап, которому тень и скругление даёт ОС:
// corner — радиус углов в логических точках (ОС масштабирует его сама), 0 —
// окно с альфой точек (тень рисует движок). Зовётся до CreatePopup.
type popupSystemDecor interface {
	SetPopupSystem(corner int)
}

// defaultWindowShadow — тень окна, которую рисует движок, когда тема своей
// не объявила (стиль "window" без токенов тени): мягкая, как у DWM.
var defaultWindowShadow = theme.ShadowSpec{Blur: 16, OffsetY: 6, Color: theme.RGBA(0, 0, 0, 80)}

// SetShadowSource выбирает, кто даёт тень окну и его попапам (меню,
// раскрывающимся спискам), а также диалогам и панелям, которые окно
// открывает в собственных окнах ОС. По умолчанию — ShadowAuto.
//
// Можно звать до Run и после: окно меняет тень сразу, попапы — со
// следующего показа. Своя тень окна движком сейчас есть у Win32; на X11 и
// Wayland тень окну даёт оконный менеджер.
func (win *Window) SetShadowSource(s ShadowSource) {
	win.shadowSrc.Store(int32(s))
	if win.native != nil {
		applyShadowSource(win.native, s, win.scale, false)
	}
	if win.eng != nil && (win.popupHost != nil || win.childPopups != nil) {
		declarePopupShadow(win.eng, s, win.native)
	}
}

// ShadowSource — источник тени, выбранный SetShadowSource.
func (win *Window) ShadowSource() ShadowSource {
	return ShadowSource(win.shadowSrc.Load())
}

// shadowSourceOf — источник тени окна (nil — по умолчанию).
func shadowSourceOf(win *Window) ShadowSource {
	if win == nil {
		return ShadowAuto
	}
	return win.ShadowSource()
}

// applyShadowSource передаёт бэкенду источник и вид тени окна nw.
// dialog — окно диалога (тень по токенам стиля "dialog").
func applyShadowSource(nw native.NativeWindow, s ShadowSource, scale float64, dialog bool) {
	if es, ok := nw.(engineShadowSetter); ok {
		sp, ok := widget.WindowShadow()
		if dialog {
			if d, ok2 := widget.DialogShadow(); ok2 {
				sp, ok = d, true
			}
		}
		if !ok {
			sp = defaultWindowShadow
		}
		if scale <= 0 || math.IsNaN(scale) {
			scale = 1
		}
		es.SetEngineShadow(sp.Blur*scale, sp.OffsetX*scale, sp.OffsetY*scale, sp.Color)
	}
	if ss, ok := nw.(shadowSourceSetter); ok {
		ss.SetShadowSource(int(s))
	}
}

// popupShadowMode — режим оформления попапов движка (engine.PopupShadowMode)
// для источника s и окна-носителя carrier.
func popupShadowMode(s ShadowSource, carrier native.NativeWindow) engine.PopupShadowMode {
	sys := false
	if sp, ok := carrier.(systemPopupShadow); ok {
		sys = sp.SystemPopupShadow()
	}
	switch s {
	case ShadowEngine:
		return engine.PopupShadowEngine
	case ShadowSystem:
		if sys {
			return engine.PopupShadowSystem
		}
		return engine.PopupShadowNone
	default:
		if sys {
			return engine.PopupShadowSystem | engine.PopupShadowEngine
		}
		return engine.PopupShadowEngine
	}
}

// declarePopupShadow сообщает движку, кто оформляет его окна-попапы
// (engine.SetPopupShadowMode). Окна хостов пакета показывают альфу точек
// (Win32 — UpdateLayeredWindow, X11 с композитором и Wayland — ARGB; X11 без
// композитора выкраивает окно по сплошным точкам, чернеть нечему), а
// Windows 11 с DWM ещё и скругляет и оттеняет скруглённые попапы сама.
func declarePopupShadow(eng any, s ShadowSource, carrier native.NativeWindow) {
	if ps, ok := eng.(interface {
		SetPopupShadowMode(engine.PopupShadowMode)
	}); ok {
		ps.SetPopupShadowMode(popupShadowMode(s, carrier))
	}
}
