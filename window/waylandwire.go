package window

import (
	"encoding/binary"
	"strings"
)

// waylandwire.go — чистая часть Wayland-протокола: сборка исходящих сообщений
// и разбор тех событий, где кроме байтов ничего не нужно.
//
// Файл БЕЗ платформенного суффикса и без build-тега намеренно: сокет
// композитора есть только под Linux, а вот проверять раскладку байтов можно
// (и нужно) на любой машине — тесты этих функций идут в общем прогоне, а не
// «только если разработчик сидит в Wayland-сессии». Отправкой занимается
// native_wayland.go.

// Значения протокола, нужные чистой части. Остальные — в native_wayland.go.
const (
	// xdg_toplevel — запросы. Все они есть в версии 1 xdg-shell: версию
	// привязки поднимать не нужно (новая обязывает принимать новые события).
	xdgToplevelSetTitle       = 2
	xdgToplevelSetAppID       = 3
	xdgToplevelMove           = 5
	xdgToplevelResize         = 6
	xdgToplevelSetMaxSize     = 7
	xdgToplevelSetMinSize     = 8
	xdgToplevelSetMaximized   = 9
	xdgToplevelUnsetMaximized = 10
	xdgToplevelSetMinimized   = 13

	// xdg_toplevel.state — всё, что нужно, есть в версии 1 протокола.
	// Неизвестные значения пропускаем: компоновщик вправе прислать состояния
	// из версий, к которым клиент не привязывался.
	xdgStateMaximized  = 1
	xdgStateFullscreen = 2
	xdgStateResizing   = 3
	xdgStateActivated  = 4

	// xdg_toplevel.resize_edge: top 1, bottom 2, left 4, right 8, углы — их
	// комбинации (5, 6, 9, 10).
	xdgResizeEdgeNone = 0
)

// wlMsg — конструктор исходящего сообщения.
//
// Раскладка wire-формата: object id (uint32), затем слово, где младшие
// 16 бит — opcode, старшие — длина всего сообщения в байтах. Длину знать
// заранее нельзя, поэтому её проставляет bytes() перед отправкой.
type wlMsg struct {
	buf []byte
}

func newWlMsg(objectID uint32, opcode uint16) *wlMsg {
	m := &wlMsg{buf: make([]byte, 8, 32)}
	binary.LittleEndian.PutUint32(m.buf[0:4], objectID)
	// размер заполним при отправке; opcode — младшие 16 бит второго слова
	binary.LittleEndian.PutUint16(m.buf[4:6], opcode)
	return m
}

func (m *wlMsg) putUint(v uint32) *wlMsg {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	m.buf = append(m.buf, b[:]...)
	return m
}

func (m *wlMsg) putInt(v int32) *wlMsg { return m.putUint(uint32(v)) }

// putString — строка протокола: len (с NUL) + байты + NUL + padding до 4.
func (m *wlMsg) putString(s string) *wlMsg {
	n := len(s) + 1
	m.putUint(uint32(n))
	m.buf = append(m.buf, s...)
	m.buf = append(m.buf, 0)
	for len(m.buf)%4 != 0 {
		m.buf = append(m.buf, 0)
	}
	return m
}

// bytes проставляет длину в заголовке и возвращает готовое сообщение.
func (m *wlMsg) bytes() []byte {
	binary.LittleEndian.PutUint16(m.buf[6:8], uint16(len(m.buf)))
	return m.buf
}

// ─── Разбор состояний xdg_toplevel.configure ─────────────────────────────────

// wlParseStates читает массив состояний из xdg_toplevel.configure.
//
// arr — хвост тела события начиная с длины массива: uint32 длины в БАЙТАХ,
// затем сами состояния по uint32. Незнакомые значения пропускаются молча:
// компоновщик вправе прислать состояния из версий протокола, к которым
// клиент не привязывался, и падать из-за этого нельзя.
//
// Обрезанный массив (компоновщик прислал меньше, чем обещал) читается до
// фактического конца буфера — лучше неполный ответ, чем паника в цикле
// событий.
func wlParseStates(arr []byte) (active, maximized, fullscreen bool) {
	if len(arr) < 4 {
		return
	}
	n := int(binary.LittleEndian.Uint32(arr[0:4]))
	if n > len(arr)-4 {
		n = len(arr) - 4
	}
	for i := 0; i+4 <= n; i += 4 {
		switch binary.LittleEndian.Uint32(arr[4+i : 8+i]) {
		case xdgStateActivated:
			active = true
		case xdgStateMaximized:
			maximized = true
		case xdgStateFullscreen:
			fullscreen = true
		}
	}
	return
}

// ─── Края для xdg_toplevel.resize ────────────────────────────────────────────

// wlResizeEdge переводит биты краёв движка (widget.NativeEdge*) в
// xdg_toplevel.resize_edge.
//
// Значения совпадают один в один (top 1, bottom 2, left 4, right 8, углы —
// их комбинации), но перевод всё равно явный: это граница двух протоколов, и
// случайное совпадение чисел не повод сращивать их молча. Бессмысленные
// сочетания (сразу верх и низ) отбрасываются — компоновщик на них ответит
// протокольной ошибкой, а это разрыв соединения.
func wlResizeEdge(edges int) uint32 {
	const (
		top    = 1
		bottom = 2
		left   = 4
		right  = 8
	)
	e := uint32(0)
	if edges&top != 0 && edges&bottom == 0 {
		e |= top
	}
	if edges&bottom != 0 && edges&top == 0 {
		e |= bottom
	}
	if edges&left != 0 && edges&right == 0 {
		e |= left
	}
	if edges&right != 0 && edges&left == 0 {
		e |= right
	}
	return e
}

// ─── app_id ──────────────────────────────────────────────────────────────────

// defaultAppID — имя исполняемого файла без пути и расширения.
//
// По app_id среда рабочего стола ищет <app_id>.desktop и значок программы.
// Общее имя движка ставило все программы на нём в одну кучу: панель задач не
// отличала проводник от калькулятора. Так же поступают GTK и Qt.
func defaultAppID(argv0 string) string {
	// Путь режем сами, а не filepath.Base: разделитель тут от ЧУЖОЙ системы
	// (argv0 из теста или из кросс-сборки), а filepath знает только про свою.
	base := strings.ReplaceAll(argv0, "\\", "/")
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".exe")
	if base == "" || base == "." {
		return "headless-gui"
	}
	return base
}
