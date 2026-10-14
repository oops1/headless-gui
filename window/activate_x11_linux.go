//go:build linux && !android

package window

// activate_x11_linux.go — передний план и свёрнутость окна X11 (задание PixGo
// E14).
//
// Просьба к оконному менеджеру — ClientMessage _NET_ACTIVE_WINDOW корневому
// окну (EWMH): источник «приложение» (1) и метка времени пользовательского
// действия, из-за которого просят активацию. Менеджеры сравнивают метку со
// временем последнего ввода и вправе отказать (только подсветить окно), если
// она старее — метка 0 (CurrentTime) для них «ничего не доказано». Откуда
// берётся метка:
//
//   - ActivateWithToken: из стартового идентификатора запустившего процесса
//     (DESKTOP_STARTUP_ID вида "<имя>_TIME<мс>", startup-notification);
//   - иначе — время последнего ввода (клавиша, кнопка) в самом окне: события
//     X11 несут время сервера, мы запоминаем его;
//   - ничего нет — 0 (CurrentTime).
//
// Заодно запускающему сообщается о конце запуска: _NET_STARTUP_INFO «remove:
// ID=...», иначе курсор «занят» у GNOME/KDE висит до таймаута, потому что окно
// второго процесса так и не появилось.
//
// Свёрнутость: _NET_WM_STATE_HIDDEN из _NET_WM_STATE (EWMH) либо WM_STATE =
// Iconic (ICCCM, для менеджеров без EWMH-свойства). Оба перечитываются по
// PropertyNotify на верхнем уровне цикла событий (x11SyncWMState).

import (
	"encoding/binary"
	"strings"
	"sync/atomic"
)

// x11Activation — состояние активации и свёрнутости окна.
type x11Activation struct {
	// userTime — время сервера X11 последнего ввода в окне (мс), 0 — не было.
	userTime atomic.Uint32

	// hidden — _NET_WM_STATE содержит _NET_WM_STATE_HIDDEN; iconic —
	// WM_STATE = Iconic. Свёрнуто, если верно любое.
	hidden, iconic atomic.Bool
	minimized      atomic.Bool
	// staleICCCM — менялось свойство WM_STATE: его надо перечитать на верхнем
	// уровне цикла (как _NET_WM_STATE у x11Fullscreen.stale).
	staleICCCM atomic.Bool

	onChange atomic.Pointer[func(bool)]
}

const (
	// ICCCM WM_STATE: значение state.
	x11WMStateIconic = 3

	// x11EvKeyPress … x11EvButtonRelease — события ввода, у которых по
	// смещению 4 лежит время сервера.
	x11EvKeyPress      = 2
	x11EvKeyRelease    = 3
	x11EvButtonPress   = 4
	x11EvButtonRelease = 5

	// PropertyChangeMask — маска события для _NET_STARTUP_INFO.
	x11PropertyChangeMask = 0x00400000
)

// noteUserTime запоминает время ввода из события buf (KeyPress/Release,
// ButtonPress/Release).
func (a *x11Activation) noteUserTime(evType byte, buf []byte) {
	switch evType {
	case x11EvKeyPress, x11EvKeyRelease, x11EvButtonPress, x11EvButtonRelease:
		if len(buf) >= 8 {
			if t := binary.LittleEndian.Uint32(buf[4:8]); t != 0 {
				a.userTime.Store(t)
			}
		}
	}
}

// IsMinimized — окно свёрнуто, как его видит менеджер. Реализует
// minimizedBackend.
func (w *X11Window) IsMinimized() bool { return w.act.minimized.Load() }

// SetOnMinimizedChanged регистрирует уведомление о смене свёрнутости.
// Реализует minimizedBackend. Вызывается из потока событий окна.
func (w *X11Window) SetOnMinimizedChanged(fn func(bool)) {
	if fn == nil {
		w.act.onChange.Store(nil)
		return
	}
	w.act.onChange.Store(&fn)
}

// setMinimizedFlags обновляет одну из составляющих свёрнутости и сообщает о
// смене итога.
func (w *X11Window) setMinimizedFlags(hidden, iconic *bool) {
	if hidden != nil {
		w.act.hidden.Store(*hidden)
	}
	if iconic != nil {
		w.act.iconic.Store(*iconic)
	}
	now := w.act.hidden.Load() || w.act.iconic.Load()
	if w.act.minimized.Swap(now) != now {
		if fn := w.act.onChange.Load(); fn != nil {
			(*fn)(now)
		}
	}
}

// x11ActiveWindowMessage собирает запрос SendEvent с ClientMessage
// _NET_ACTIVE_WINDOW для окна wid, адресованный корневому окну root: source —
// 1 «приложение», ts — метка времени пользовательского действия (0 —
// CurrentTime), current — текущее активное окно приложения (0 — нет). Чистая
// функция: раскладка проверяется тестом.
func x11ActiveWindowMessage(root, wid, atom, source, ts, current uint32) []byte {
	msg := make([]byte, 32)
	msg[0] = 33 // ClientMessage
	msg[1] = 32 // формат данных: 32 бита
	binary.LittleEndian.PutUint32(msg[4:8], wid)
	binary.LittleEndian.PutUint32(msg[8:12], atom)
	binary.LittleEndian.PutUint32(msg[12:16], source)
	binary.LittleEndian.PutUint32(msg[16:20], ts)
	binary.LittleEndian.PutUint32(msg[20:24], current)

	req := make([]byte, 44)
	req[0] = 25                                 // SendEvent
	binary.LittleEndian.PutUint16(req[2:4], 11) // длина запроса в словах
	binary.LittleEndian.PutUint32(req[4:8], root)
	binary.LittleEndian.PutUint32(req[8:12], 0x00180000) // SubstructureRedirect|SubstructureNotify
	copy(req[12:], msg)
	return req
}

// x11NetActiveWindowAt просит менеджера активировать окно с меткой времени ts.
func (w *X11Window) x11NetActiveWindowAt(ts uint32) {
	if w.atomNetActiveWindow == 0 {
		return
	}
	w.x11Send(x11ActiveWindowMessage(w.rootWin, w.wid, w.atomNetActiveWindow, 1, ts, 0))
}

// Activate выводит окно на передний план: если оно свёрнуто — показывает
// (MapWindow, ICCCM 4.1.4: возврат из Iconic в Normal делает сам клиент), затем
// просит менеджера активировать (_NET_ACTIVE_WINDOW, метка — время последнего
// ввода в окне), поднимает окно и переносит фокус. true — запрос отправлен;
// решение за менеджером. Реализует activateBackend.
func (w *X11Window) Activate() bool { return w.activateAt(w.act.userTime.Load()) }

// ActivateWithToken то же с токеном — стартовым идентификатором запуска
// (DESKTOP_STARTUP_ID): метка времени берётся из его части «_TIME<мс>»
// (если нет — как у Activate), а запускающему сообщается о конце запуска.
// Реализует activateBackend.
func (w *X11Window) ActivateWithToken(token string) bool {
	if token == "" {
		return w.Activate()
	}
	ts, ok := startupIDTime(token)
	if !ok {
		ts = w.act.userTime.Load()
	}
	if !w.activateAt(ts) {
		return false
	}
	w.x11StartupRemove(token)
	return true
}

func (w *X11Window) activateAt(ts uint32) bool {
	if w.wid == 0 || w.atomNetActiveWindow == 0 {
		return false
	}
	if w.act.minimized.Load() {
		w.x11MapWindow(w.wid)
	}
	w.x11NetActiveWindowAt(ts)
	w.x11RaiseWindow()
	if !w.act.minimized.Load() {
		// Окно в пути из значка ещё не видимо — SetInputFocus дал бы BadMatch;
		// фокус ему отдаст сам менеджер по _NET_ACTIVE_WINDOW.
		w.x11SetInputFocus()
	}
	return true
}

// x11StartupQuote оформляет значение для сообщения startup-notification:
// без пробелов, кавычек и обратной косой — как есть, иначе в двойных кавычках
// с экранированием.
func x11StartupQuote(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\"\\") {
		return v
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(v); i++ {
		if v[i] == '"' || v[i] == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(v[i])
	}
	b.WriteByte('"')
	return b.String()
}

// x11StartupInfoRequests режет сообщение startup-notification на запросы
// SendEvent: ClientMessage формата 8, по 20 байт; первый кусок имеет тип begin,
// остальные — cont; текст кончается нулевым байтом, последний кусок добит
// нулями. Окно-отправитель — wid, адресат — корневое окно root, маска события
// PropertyChange (так требует спецификация). Чистая функция.
func x11StartupInfoRequests(root, wid, begin, cont uint32, text string) [][]byte {
	data := append([]byte(text), 0)
	var out [][]byte
	for off, first := 0, true; off < len(data); off, first = off+20, false {
		msg := make([]byte, 32)
		msg[0] = 33 // ClientMessage
		msg[1] = 8  // формат данных: 8 бит
		binary.LittleEndian.PutUint32(msg[4:8], wid)
		typ := cont
		if first {
			typ = begin
		}
		binary.LittleEndian.PutUint32(msg[8:12], typ)
		copy(msg[12:32], data[off:min(off+20, len(data))])

		req := make([]byte, 44)
		req[0] = 25                                 // SendEvent
		binary.LittleEndian.PutUint16(req[2:4], 11) // длина запроса в словах
		binary.LittleEndian.PutUint32(req[4:8], root)
		binary.LittleEndian.PutUint32(req[8:12], x11PropertyChangeMask)
		copy(req[12:], msg)
		out = append(out, req)
	}
	return out
}

// x11StartupRemove сообщает запускающему, что запуск id закончен («remove:
// ID=...» по _NET_STARTUP_INFO, startup-notification 1.0). Атомы интернированы
// при Create; нет их — тихо ничего не делаем.
func (w *X11Window) x11StartupRemove(id string) {
	if w.atomStartupBegin == 0 || w.atomStartupInfo == 0 {
		return
	}
	for _, req := range x11StartupInfoRequests(w.rootWin, w.wid, w.atomStartupBegin, w.atomStartupInfo,
		"remove: ID="+x11StartupQuote(id)) {
		w.x11Send(req)
	}
}

// x11SyncICCCM перечитывает WM_STATE, если оно менялось, и обновляет признак
// «значок». Зовётся из x11SyncWMState на верхнем уровне цикла событий.
func (w *X11Window) x11SyncICCCM() {
	if !w.act.staleICCCM.Swap(false) || w.wid == 0 || w.atomWMStateProp == 0 {
		return
	}
	_, format, data := w.x11GetProperty(w.wid, w.atomWMStateProp, false)
	iconic := format == 32 && len(data) >= 4 &&
		binary.LittleEndian.Uint32(data[0:4]) == x11WMStateIconic
	w.setMinimizedFlags(nil, &iconic)
}
