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
	"sort"
)

// Monitor — один монитор рабочего стола.
type Monitor struct {
	// Name — имя монитора, если система его сообщает: Win32 — имя устройства
	// (`\\.\DISPLAY1`), X11 — имя вывода RandR (`HDMI-1`), Wayland — имя
	// wl_output/xdg_output (`screen-0`, `eDP-1`) или «изготовитель модель».
	// Может быть пустым.
	Name string
	// Bounds — монитор целиком, Work — без панелей (таскбар и т. п.). Оба — в
	// координатах рабочего стола (см. описание пакета выше о единицах).
	//
	// На Wayland Work ВСЕГДА равна Bounds: рабочей области протокол не
	// сообщает (панели занимают экран через layer-shell, о них клиент не
	// знает). На X11 Work — приближение по _NET_WORKAREA (см. выше).
	Bounds, Work image.Rectangle
	// Scale — масштаб интерфейса монитора (1.0 = 96 dpi).
	Scale float64
	// Primary — основной монитор. На Wayland такого понятия нет вовсе:
	// главным считается первый вывод в порядке объявления композитором
	// (устойчиво между вызовами, но не обязательно тот, что в панели
	// настроек).
	Primary bool

	// id — внутренний ключ монитора в его бэкенде: Win32 — HMONITOR, X11 —
	// номер в ответе RRGetMonitors (с единицы: он же индекс для
	// _NET_WM_FULLSCREEN_MONITORS), Wayland — имя глобала wl_output в
	// registry (одинаково у любого подключения к композитору). 0 — ключа нет
	// (Monitor собран руками). По нему SetFullscreenOn находит «тот самый»
	// монитор, даже если список переупорядочен, а Name пуст; запасные пути —
	// Name и Bounds (matchMonitor).
	id uint64
}

// monitorSource — бэкенд знает, на каком мониторе его окно. Необязательная
// способность: без неё MonitorOf ищет монитор по прямоугольнику окна.
type monitorSource interface {
	monitorOfWindow() (Monitor, bool)
}

// Monitors возвращает мониторы рабочего стола: основной первым, остальные в
// порядке слева направо, сверху вниз. Пусто, если узнать нельзя (нет сессии
// рабочего стола, macOS).
//
// Можно звать из любой горутины и без окна. На X11 и Wayland это короткое
// отдельное подключение к серверу: вызывать на каждый кадр не стоит.
func Monitors() []Monitor {
	return sortMonitors(platformMonitors())
}

// MonitorOf возвращает монитор, на котором находится окно (с наибольшей долей
// площади). ok=false — окна нет, оно ещё не создано или монитор не определить.
func MonitorOf(win *Window) (Monitor, bool) {
	if win == nil || win.native == nil {
		return Monitor{}, false
	}
	if ms, ok := win.native.(monitorSource); ok {
		if m, ok := ms.monitorOfWindow(); ok {
			return m, true
		}
	}
	x, y := win.native.GetPosition()
	w, h := win.native.GetSize()
	return MonitorForRect(Monitors(), image.Rect(x, y, x+w, y+h))
}

// MonitorAt возвращает монитор, содержащий точку (x, y) рабочего стола.
func MonitorAt(x, y int) (Monitor, bool) {
	return monitorAt(Monitors(), x, y)
}

// MonitorForRect возвращает из списка монитор, на который попадает наибольшая
// часть прямоугольника r. ok=false — r не пересекает ни один монитор: сохранённое
// положение окна «ушло» за отключённый экран и его нужно вернуть на видимое место.
func MonitorForRect(list []Monitor, r image.Rectangle) (Monitor, bool) {
	var best Monitor
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
func monitorAt(list []Monitor, x, y int) (Monitor, bool) {
	pt := image.Pt(x, y)
	for _, m := range list {
		if pt.In(m.Bounds) {
			return m, true
		}
	}
	return Monitor{}, false
}

// matchMonitor находит в списке монитор, который описывает m: сперва по
// внутреннему ключу (он однозначен), затем по имени, затем по границам.
// ok=false — такого монитора больше нет (его отключили, сменилась раскладка).
// Возвращается запись СПИСКА, а не m: у неё свежие границы и ключ.
func matchMonitor(list []Monitor, m Monitor) (Monitor, bool) {
	if m.id != 0 {
		for _, c := range list {
			if c.id == m.id {
				return c, true
			}
		}
	}
	if m.Name != "" {
		for _, c := range list {
			if c.Name == m.Name {
				return c, true
			}
		}
	}
	if !m.Bounds.Empty() {
		for _, c := range list {
			if c.Bounds == m.Bounds {
				return c, true
			}
		}
	}
	return Monitor{}, false
}

// sortMonitors упорядочивает мониторы: основной первым, остальные слева
// направо, затем сверху вниз. Порядок, в котором их перечисляет система, ничем
// не гарантирован, а приложению нужен предсказуемый («второй монитор»).
func sortMonitors(list []Monitor) []Monitor {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Primary != b.Primary {
			return a.Primary
		}
		if a.Bounds.Min.X != b.Bounds.Min.X {
			return a.Bounds.Min.X < b.Bounds.Min.X
		}
		return a.Bounds.Min.Y < b.Bounds.Min.Y
	})
	return list
}
