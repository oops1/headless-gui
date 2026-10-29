//go:build linux && !android

package core

// x11_fullscreen_linux.go — полный экран на X11 (EWMH _NET_WM_STATE_FULLSCREEN).
//
// Просьба к оконному менеджеру — ClientMessage _NET_WM_STATE корневому окну с
// действием ADD/REMOVE, как уже сделано для разворачивания. Что менеджер
// решил, видно по свойству _NET_WM_STATE окна: он меняет его сам, и мы
// узнаём это из PropertyNotify (маска PropertyChange у окна выставлена в
// Create). Состояние берётся из свойства, а не из нашего запроса: менеджер
// вправе отказать, а пользователь вправе выйти из полного экрана его же
// средствами.

import (
	"encoding/binary"
	"sync/atomic"
)

const (
	// _NET_WM_STATE: действие ClientMessage (EWMH).
	netWMStateRemove = 0
	netWMStateAdd    = 1

	// x11EvPropertyNotify — код события PropertyNotify.
	x11EvPropertyNotify = 28
)

// x11Fullscreen — состояние полного экрана окна.
type x11Fullscreen struct {
	on atomic.Bool
	// unsupported — менеджер объявил список поддерживаемых состояний
	// (_NET_SUPPORTED), и полного экрана в нём нет. Пишет Create до показа
	// окна.
	unsupported bool
	// monitorsSupported — менеджер объявил _NET_WM_FULLSCREEN_MONITORS: полный
	// экран на выбранном мониторе задаётся им, а окно с места не двигается.
	monitorsSupported bool
	onChange          atomic.Pointer[func(bool)]
	// stale — менялось свойство _NET_WM_STATE: его надо перечитать. Читать на
	// месте нельзя: событие может прийти посреди чужого запроса-ответа, и
	// вложенный запрос перепутал бы ответы. Перечитывает цикл событий на
	// верхнем уровне (x11SyncWMState).
	stale atomic.Bool
}

// SetFullscreen просит оконный менеджер перевести окно в полный экран (on) или
// вернуть. Реализует fullscreenBackend. Ответ менеджера приходит позже, через
// PropertyNotify.
func (w *X11Window) SetFullscreen(on bool) bool {
	if w.wid == 0 || w.atomNetWMState == 0 || w.atomWMStateFullscreen == 0 || w.fs.unsupported {
		return false
	}
	action := uint32(netWMStateRemove)
	if on {
		action = netWMStateAdd
	}
	w.x11Send(x11NetWMStateMessage(w.rootWin, w.wid, w.atomNetWMState, action, w.atomWMStateFullscreen))
	return true
}

// IsFullscreen — менеджер держит окно в полном экране. Реализует fullscreenBackend.
func (w *X11Window) IsFullscreen() bool { return w.fs.on.Load() }

// SetOnFullscreenChanged регистрирует уведомление о смене состояния. Реализует
// fullscreenBackend. Вызывается из потока событий окна.
func (w *X11Window) SetOnFullscreenChanged(fn func(on bool)) {
	if fn == nil {
		w.fs.onChange.Store(nil)
		return
	}
	w.fs.onChange.Store(&fn)
}

// x11NetWMStateMessage собирает запрос SendEvent с ClientMessage _NET_WM_STATE
// для окна wid, адресованный корневому окну root: action — ADD/REMOVE/TOGGLE,
// prop — меняемое состояние. Чистая функция: раскладка проверяется тестом.
func x11NetWMStateMessage(root, wid, netWMState, action, prop uint32) []byte {
	msg := make([]byte, 32)
	msg[0] = 33 // ClientMessage
	msg[1] = 32 // формат данных: 32 бита
	binary.LittleEndian.PutUint32(msg[4:8], wid)
	binary.LittleEndian.PutUint32(msg[8:12], netWMState)
	binary.LittleEndian.PutUint32(msg[12:16], action)
	binary.LittleEndian.PutUint32(msg[16:20], prop)
	binary.LittleEndian.PutUint32(msg[20:24], 0)
	binary.LittleEndian.PutUint32(msg[24:28], 1) // источник — приложение

	req := make([]byte, 44)
	req[0] = 25                                 // SendEvent
	binary.LittleEndian.PutUint16(req[2:4], 11) // длина запроса в словах
	binary.LittleEndian.PutUint32(req[4:8], root)
	binary.LittleEndian.PutUint32(req[8:12], 0x00180000) // SubstructureRedirect|SubstructureNotify
	copy(req[12:], msg)
	return req
}

// x11StateHas — среди атомов значения свойства (format 32) есть atom.
func x11StateHas(data []byte, atom uint32) bool {
	if atom == 0 {
		return false
	}
	for i := 0; i+4 <= len(data); i += 4 {
		if binary.LittleEndian.Uint32(data[i:i+4]) == atom {
			return true
		}
	}
	return false
}

// x11DetectFullscreenSupport узнаёт у менеджера, умеет ли он полный экран
// (_NET_SUPPORTED корневого окна). Зовётся из Create до показа окна, пока на
// соединении нет событий — ответ читается напрямую. Свойство отсутствует
// (менеджера ещё нет или он не EWMH) — не считаем, что не умеет: запрос просто
// ничего не изменит, а IsFullscreen останется false.
func (w *X11Window) x11DetectFullscreenSupport() {
	if w.atomNetSupported == 0 || w.atomWMStateFullscreen == 0 {
		return
	}
	_, format, data := w.x11GetProperty(w.rootWin, w.atomNetSupported, false)
	if format == 32 && len(data) > 0 && !x11StateHas(data, w.atomWMStateFullscreen) {
		w.fs.unsupported = true
	}
	if format == 32 {
		w.fs.monitorsSupported = x11StateHas(data, w.atomFsMonitors)
	}
}

// x11MonitorList возвращает мониторы X11 для SetFullscreenOn. Переменная —
// чтобы тест не ходил на настоящий сервер (по умолчанию короткое отдельное
// соединение, как у Monitors).
var x11MonitorList = x11PlatformMonitors

// SetFullscreenOn просит полный экран на мониторе m (fullscreenOnBackend).
//
// Монитор сверяется с текущим списком RandR (ключ — номер в RRGetMonitors, он
// же индекс для EWMH, затем имя, затем границы). Дальше два пути.
//
//   - Менеджер объявил _NET_WM_FULLSCREEN_MONITORS: шлём ему индекс монитора
//     (четыре края — один и тот же), затем _NET_WM_STATE_FULLSCREEN. Окно с
//     места не двигается: по выходе из полного экрана оно вернётся туда, где
//     было. Окно, уже стоящее в полном экране, просто переезжает.
//   - Не объявил: окно переносится на левый верхний угол монитора, затем
//     ADD _NET_WM_STATE_FULLSCREEN — менеджеры ставят полноэкранное окно на
//     монитор, где оно лежит. Окно, уже стоящее в полном экране, сначала из
//     него выходит (REMOVE), переносится и входит снова: приложение увидит
//     false и сразу true. После выхода окно останется на новом мониторе.
func (w *X11Window) SetFullscreenOn(m Monitor) bool {
	if w.wid == 0 || w.atomNetWMState == 0 || w.atomWMStateFullscreen == 0 || w.fs.unsupported {
		return false
	}
	cur, ok := matchMonitor(x11MonitorList(), m)
	if !ok || cur.Bounds.Empty() || cur.id == 0 {
		return false
	}
	on := w.fs.on.Load()
	switch {
	case w.fs.monitorsSupported && w.atomFsMonitors != 0:
		w.x11Send(x11FullscreenMonitorsMessage(w.rootWin, w.wid, w.atomFsMonitors, uint32(cur.id-1)))
	case on:
		w.x11Send(x11NetWMStateMessage(w.rootWin, w.wid, w.atomNetWMState, netWMStateRemove, w.atomWMStateFullscreen))
		w.SetPosition(cur.Bounds.Min.X, cur.Bounds.Min.Y)
	default:
		w.SetPosition(cur.Bounds.Min.X, cur.Bounds.Min.Y)
	}
	if !on || !w.fs.monitorsSupported || w.atomFsMonitors == 0 {
		w.x11Send(x11NetWMStateMessage(w.rootWin, w.wid, w.atomNetWMState, netWMStateAdd, w.atomWMStateFullscreen))
	}
	return true
}

// x11FullscreenMonitorsMessage собирает ClientMessage _NET_WM_FULLSCREEN_MONITORS
// корневому окну: окно wid должно занять монитор с индексом idx (верхний,
// нижний, левый и правый край — один монитор). Чистая функция: раскладка
// проверяется тестом.
func x11FullscreenMonitorsMessage(root, wid, atom, idx uint32) []byte {
	msg := make([]byte, 32)
	msg[0] = 33 // ClientMessage
	msg[1] = 32 // формат данных: 32 бита
	binary.LittleEndian.PutUint32(msg[4:8], wid)
	binary.LittleEndian.PutUint32(msg[8:12], atom)
	for i := 0; i < 4; i++ { // top, bottom, left, right
		binary.LittleEndian.PutUint32(msg[12+i*4:16+i*4], idx)
	}
	binary.LittleEndian.PutUint32(msg[28:32], 1) // источник — приложение

	req := make([]byte, 44)
	req[0] = 25                                 // SendEvent
	binary.LittleEndian.PutUint16(req[2:4], 11) // длина запроса в словах
	binary.LittleEndian.PutUint32(req[4:8], root)
	binary.LittleEndian.PutUint32(req[8:12], 0x00180000) // SubstructureRedirect|SubstructureNotify
	copy(req[12:], msg)
	return req
}

// x11NoteWMStateChanged запоминает, что менеджер поменял _NET_WM_STATE окна.
// Зовётся из handleX11Event по PropertyNotify.
func (w *X11Window) x11NoteWMStateChanged(buf []byte) {
	if len(buf) < 12 {
		return
	}
	win := binary.LittleEndian.Uint32(buf[4:8])
	atom := binary.LittleEndian.Uint32(buf[8:12])
	if win != w.wid {
		return
	}
	if w.atomNetWMState != 0 && atom == w.atomNetWMState {
		w.fs.stale.Store(true)
	}
	// WM_STATE (ICCCM) — свёрнутость у менеджеров без EWMH-свойства.
	if w.atomWMStateProp != 0 && atom == w.atomWMStateProp {
		w.act.staleICCCM.Store(true)
	}
}

// x11SyncWMState перечитывает _NET_WM_STATE, если оно менялось, и сообщает о
// смене полного экрана. Зовётся циклом событий между событиями — на верхнем
// уровне, где запрос-ответ безопасен.
//
// Разворачивание обновляется заодно: менеджер мог развернуть окно сам
// (двойной щелчок по заголовку, клавиатура), а IsMaximized знал только свои
// просьбы. На время полного экрана «развёрнуто» не трогаем: оно хранит прежнее
// значение (так же, как на Win32).
func (w *X11Window) x11SyncWMState() {
	w.x11SyncICCCM()
	if !w.fs.stale.Swap(false) || w.wid == 0 {
		return
	}
	_, format, data := w.x11GetProperty(w.wid, w.atomNetWMState, false)
	if format != 32 {
		data = nil
	}
	hidden := x11StateHas(data, w.atomWMStateHidden)
	w.setMinimizedFlags(&hidden, nil)
	full := x11StateHas(data, w.atomWMStateFullscreen)
	if w.atomWMStateAbove != 0 {
		w.above.Store(x11StateHas(data, w.atomWMStateAbove))
	}
	if !full {
		w.maximized = x11StateHas(data, w.atomWMStateMaxH) && x11StateHas(data, w.atomWMStateMaxV)
	}
	changed := w.fs.on.Swap(full) != full
	// Менеджер вернул окно в обычное состояние сам (двойной щелчок по
	// заголовку, Esc): размер, заказанный SetSize за это время, применяется
	// теперь. Состояние уже записано — SetSize больше не копит заказ.
	w.x11ApplyRestoreSize()
	if changed {
		if fn := w.fs.onChange.Load(); fn != nil {
			(*fn)(full)
		}
	}
}
