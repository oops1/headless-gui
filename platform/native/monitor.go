package native

// monitor.go — монитор рабочего стола и его поиск в списке.

import (
	"image"
)

// Monitor — один монитор рабочего стола.
type Monitor struct {
	// Name — имя монитора, если система его сообщает: Win32 — имя устройства
	// (`\\.\DISPLAY1`), X11 — имя вывода RandR (`HDMI-1`), Wayland — имя
	// wl_output/xdg_output (`screen-0`, `eDP-1`) или «изготовитель модель».
	// Может быть пустым.
	Name string
	// Bounds — монитор целиком, Work — без панелей (таскбар и т. п.). Оба — в
	// координатах рабочего стола; единицы по бэкендам — в шапке
	// window/core/monitor.go.
	//
	// На Wayland Work ВСЕГДА равна Bounds: рабочей области протокол не
	// сообщает (панели занимают экран через layer-shell, о них клиент не
	// знает). На X11 Work — приближение по _NET_WORKAREA, обрезанной по монитору.
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
	// Name и Bounds (MatchMonitor).
	id uint64
}

// MonitorWithKey — m с внутренним ключом бэкенда (поле id): бэкенд ставит
// его, собирая список мониторов.
//
//facadegen:skip
func MonitorWithKey(m Monitor, key uint64) Monitor {
	m.id = key
	return m
}

// MonitorKey — внутренний ключ монитора в его бэкенде; 0 — ключа нет.
//
//facadegen:skip
func MonitorKey(m Monitor) uint64 { return m.id }

// MatchMonitor находит в списке монитор, который описывает m: сперва по
// внутреннему ключу (он однозначен), затем по имени, затем по границам.
// ok=false — такого монитора больше нет (его отключили, сменилась раскладка).
// Возвращается запись СПИСКА, а не m: у неё свежие границы и ключ.
//
//facadegen:skip
func MatchMonitor(list []Monitor, m Monitor) (Monitor, bool) {
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
