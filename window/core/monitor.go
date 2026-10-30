package core

// monitor.go — геометрия мониторов (задание PixGo E11 = E1 п. 5).
//
// До этого окно знало о мониторе только рабочую область под точкой (WorkAreaAt
// у Win32-бэкенда, нужна окнам-попапам). Приложению же нужно знать, где
// находятся мониторы: проверить, что сохранённое положение окна не ушло за
// отключённый экран, открыть слайд-шоу на втором мониторе, выбрать масштаб.
//
// API согласован с WorkAreaAt: прямоугольники — в тех же координатах, что и
// NativeWindow.GetPosition (экранные пиксели рабочего стола).
//
//	Win32   EnumDisplayMonitors + GetMonitorInfoW (+ GetDpiForMonitor):
//	        Bounds/Work — физические пиксели виртуального рабочего стола,
//	        Scale — DPI монитора / 96.
//	X11     RandR 1.5 (RRGetMonitors); без RandR или старше 1.5 — корневое
//	        окно как один монитор. Work — _NET_WORKAREA, обрезанная по
//	        монитору; Scale — Xft.dpi, одинаковый для всех (X11 масштаба
//	        мониторов не знает).
//	Wayland wl_output (+ xdg_output, если композитор его объявил): Bounds —
//	        логические координаты композитора (поверхностные единицы), Scale —
//	        целый масштаб wl_output, то есть физических пикселей буфера на
//	        единицу Bounds. Work равна Bounds: рабочей области протокол не
//	        сообщает. Primary у wl_output нет — главным считается первый.
//	Cocoa   пока не реализовано: Monitors() пуст.

import (
	"image"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// monitorSource — бэкенд знает, на каком мониторе его окно. Необязательная
// способность: без неё MonitorOf ищет монитор по прямоугольнику окна.
type monitorSource interface {
	MonitorOfWindow() (native.Monitor, bool)
}

// Monitors возвращает мониторы рабочего стола: основной первым, остальные в
// порядке слева направо, сверху вниз. Пусто, если узнать нельзя (нет сессии
// рабочего стола, macOS).
//
// Можно звать из любой горутины и без окна. На X11 и Wayland это короткое
// отдельное подключение к серверу: вызывать на каждый кадр не стоит.
func Monitors() []native.Monitor {
	return native.SortMonitors(platformMonitors())
}

// MonitorOf возвращает монитор, на котором находится окно (с наибольшей долей
// площади). ok=false — окна нет, оно ещё не создано или монитор не определить.
func MonitorOf(win *Window) (native.Monitor, bool) {
	if win == nil || win.native == nil {
		return native.Monitor{}, false
	}
	if ms, ok := win.native.(monitorSource); ok {
		if m, ok := ms.MonitorOfWindow(); ok {
			return m, true
		}
	}
	x, y := win.native.GetPosition()
	w, h := win.native.GetSize()
	return MonitorForRect(Monitors(), image.Rect(x, y, x+w, y+h))
}

// MonitorAt возвращает монитор, содержащий точку (x, y) рабочего стола.
func MonitorAt(x, y int) (native.Monitor, bool) {
	return monitorAt(Monitors(), x, y)
}

// MonitorForRect возвращает из списка монитор, на который попадает наибольшая
// часть прямоугольника r. ok=false — r не пересекает ни один монитор: сохранённое
// положение окна «ушло» за отключённый экран и его нужно вернуть на видимое место.
func MonitorForRect(list []native.Monitor, r image.Rectangle) (native.Monitor, bool) {
	var best native.Monitor
	bestArea, found := 0, false
	for _, m := range list {
		in := r.Intersect(m.Bounds)
		if in.Empty() {
			continue
		}
		if a := in.Dx() * in.Dy(); a > bestArea {
			best, bestArea, found = m, a, true
		}
	}
	return best, found
}

// monitorAt — монитор списка, содержащий точку.
func monitorAt(list []native.Monitor, x, y int) (native.Monitor, bool) {
	pt := image.Pt(x, y)
	for _, m := range list {
		if pt.In(m.Bounds) {
			return m, true
		}
	}
	return native.Monitor{}, false
}
