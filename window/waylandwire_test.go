package window

import (
	"encoding/binary"
	"testing"
)

// Сообщения Wayland собираются вручную, байт за байтом, и проверять их можно
// только так же — по сырым байтам. Тесты идут в общем прогоне: сокет
// композитора для этого не нужен.

// wlHeader разбирает заголовок сообщения: id объекта, opcode, длина.
func wlHeader(t *testing.T, buf []byte) (obj uint32, opcode uint16, size uint16) {
	t.Helper()
	if len(buf) < 8 {
		t.Fatalf("сообщение короче заголовка: %d байт", len(buf))
	}
	return binary.LittleEndian.Uint32(buf[0:4]),
		binary.LittleEndian.Uint16(buf[4:6]),
		binary.LittleEndian.Uint16(buf[6:8])
}

func TestWlMsg_HeaderAndArgs(t *testing.T) {
	m := newWlMsg(7, 5).putUint(3).putInt(-2)
	buf := m.bytes()

	obj, op, size := wlHeader(t, buf)
	if obj != 7 || op != 5 {
		t.Errorf("заголовок: объект %d, opcode %d; ждал 7 и 5", obj, op)
	}
	if int(size) != len(buf) || size != 16 {
		t.Errorf("длина в заголовке %d при %d байтах, ждал 16", size, len(buf))
	}
	if got := binary.LittleEndian.Uint32(buf[8:12]); got != 3 {
		t.Errorf("первый аргумент %d, ждал 3", got)
	}
	if got := int32(binary.LittleEndian.Uint32(buf[12:16])); got != -2 {
		t.Errorf("второй аргумент %d, ждал -2", got)
	}
}

func TestWlMsg_StringPadding(t *testing.T) {
	// "winline-calc" — 12 байт + NUL = 13, с выравниванием до 16.
	buf := newWlMsg(3, 3).putString("winline-calc").bytes()
	if _, _, size := wlHeader(t, buf); int(size) != len(buf) {
		t.Fatalf("длина в заголовке %d при %d байтах", size, len(buf))
	}
	if len(buf)%4 != 0 {
		t.Fatalf("сообщение не выровнено на 4: %d байт", len(buf))
	}
	if got := binary.LittleEndian.Uint32(buf[8:12]); got != 13 {
		t.Errorf("длина строки %d, ждал 13 (с NUL)", got)
	}
	if got := string(buf[12:24]); got != "winline-calc" {
		t.Errorf("строка %q", got)
	}
	if buf[24] != 0 {
		t.Error("строка не закрыта NUL")
	}
	if len(buf) != 28 {
		t.Errorf("всего %d байт, ждал 28 (8 заголовок + 4 длина + 16 строка с NUL и выравниванием)", len(buf))
	}
}

// Сообщения, которыми окно просит компоновщика о перемещении и размере.
func TestWlMsg_ToplevelRequests(t *testing.T) {
	const (
		toplevel = 11
		seat     = 4
		serial   = 0x2A
	)
	cases := []struct {
		name   string
		msg    *wlMsg
		opcode uint16
		args   []uint32
	}{
		{"move", newWlMsg(toplevel, xdgToplevelMove).putUint(seat).putUint(serial),
			5, []uint32{seat, serial}},
		{"resize нижний правый", newWlMsg(toplevel, xdgToplevelResize).
			putUint(seat).putUint(serial).putUint(wlResizeEdge(2 | 8)),
			6, []uint32{seat, serial, 10}},
		{"set_maximized", newWlMsg(toplevel, xdgToplevelSetMaximized), 9, nil},
		{"unset_maximized", newWlMsg(toplevel, xdgToplevelUnsetMaximized), 10, nil},
		{"set_minimized", newWlMsg(toplevel, xdgToplevelSetMinimized), 13, nil},
	}
	for _, c := range cases {
		buf := c.msg.bytes()
		obj, op, size := wlHeader(t, buf)
		if obj != toplevel {
			t.Errorf("%s: объект %d, ждал %d", c.name, obj, toplevel)
		}
		if op != c.opcode {
			t.Errorf("%s: opcode %d, ждал %d", c.name, op, c.opcode)
		}
		if want := 8 + 4*len(c.args); int(size) != want || len(buf) != want {
			t.Errorf("%s: длина %d (в заголовке %d), ждал %d", c.name, len(buf), size, want)
		}
		for i, want := range c.args {
			if got := binary.LittleEndian.Uint32(buf[8+4*i : 12+4*i]); got != want {
				t.Errorf("%s: аргумент %d = %d, ждал %d", c.name, i, got, want)
			}
		}
	}
}

func TestWlResizeEdge(t *testing.T) {
	const (
		top    = 1
		bottom = 2
		left   = 4
		right  = 8
	)
	cases := []struct {
		edges int
		want  uint32
	}{
		{top, 1},
		{bottom, 2},
		{left, 4},
		{right, 8},
		{top | left, 5},
		{bottom | left, 6},
		{top | right, 9},
		{bottom | right, 10},
		{0, 0},
		// Противоположные края разом — бессмыслица: компоновщик ответил бы
		// протокольной ошибкой, а это разрыв соединения.
		{top | bottom, 0},
		{left | right, 0},
	}
	for _, c := range cases {
		if got := wlResizeEdge(c.edges); got != c.want {
			t.Errorf("wlResizeEdge(%d) = %d, ждал %d", c.edges, got, c.want)
		}
	}
}

// wlStatesArray собирает массив состояний xdg_toplevel.configure: длина в
// БАЙТАХ, затем значения.
func wlStatesArray(states ...uint32) []byte {
	b := make([]byte, 4+4*len(states))
	binary.LittleEndian.PutUint32(b[0:4], uint32(4*len(states)))
	for i, s := range states {
		binary.LittleEndian.PutUint32(b[4+4*i:8+4*i], s)
	}
	return b
}

func TestWlParseStates(t *testing.T) {
	cases := []struct {
		name                          string
		arr                           []byte
		active, maximized, fullscreen bool
	}{
		{"пусто", wlStatesArray(), false, false, false},
		{"активно", wlStatesArray(xdgStateActivated), true, false, false},
		{"развёрнуто и активно", wlStatesArray(xdgStateMaximized, xdgStateActivated), true, true, false},
		{"полный экран", wlStatesArray(xdgStateFullscreen), false, false, true},
		{"идёт ресайз — не состояние окна", wlStatesArray(xdgStateResizing), false, false, false},
		// Компоновщик вправе прислать состояния из версий протокола, к
		// которым клиент не привязывался: пропускаем, а не падаем.
		{"незнакомое состояние", wlStatesArray(999, xdgStateMaximized), false, true, false},
		{"обрезанный массив", []byte{0x10, 0, 0, 0, 1, 0, 0, 0}, false, true, false},
		{"мусор короче длины", []byte{1, 2}, false, false, false},
	}
	for _, c := range cases {
		active, maximized, fullscreen := wlParseStates(c.arr)
		if active != c.active || maximized != c.maximized || fullscreen != c.fullscreen {
			t.Errorf("%s: active=%v maximized=%v fullscreen=%v; ждал %v/%v/%v",
				c.name, active, maximized, fullscreen, c.active, c.maximized, c.fullscreen)
		}
	}
}

func TestDefaultAppID(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/winline-calc":         "winline-calc",
		"./winline-explorer":            "winline-explorer",
		"winline-calc":                  "winline-calc",
		"C:\\Program Files\\app\\x.exe": "x",
		"":                              "headless-gui",
		"/":                             "headless-gui",
	}
	for argv0, want := range cases {
		if got := defaultAppID(argv0); got != want {
			t.Errorf("defaultAppID(%q) = %q, ждал %q", argv0, got, want)
		}
	}
}

func TestWlCursorPixels(t *testing.T) {
	seen := map[string]int{}
	for c := 0; c < wlCursorCount; c++ {
		pix, w, h, hx, hy := wlCursorPixels(c)
		if w != wlCursorSize || h != wlCursorSize {
			t.Fatalf("курсор %d: размер %dx%d, ждал %dx%d", c, w, h, wlCursorSize, wlCursorSize)
		}
		if len(pix) != w*h*4 {
			t.Fatalf("курсор %d: %d байт при %dx%d", c, len(pix), w, h)
		}
		if hx < 0 || hx >= w || hy < 0 || hy >= h {
			t.Errorf("курсор %d: горячая точка (%d,%d) вне картинки", c, hx, hy)
		}
		opaque := 0
		for i := 3; i < len(pix); i += 4 {
			if pix[i] != 0 {
				opaque++
			}
		}
		if opaque == 0 {
			t.Errorf("курсор %d пустой — под окном остался бы чужой курсор", c)
		}
		seen[string(pix)]++
	}
	// Формы обязаны отличаться: одинаковая картинка у стрелки и текста
	// означала бы, что курсор над полем ввода не меняется.
	for _, n := range seen {
		if n > 1 {
			t.Error("две формы курсора нарисованы одинаково")
		}
	}

	// Неизвестная форма — стрелка, а не пустота.
	unknown, _, _, _, _ := wlCursorPixels(wlCursorCount + 5)
	arrow, _, _, _, _ := wlCursorPixels(wlCursorArrow)
	if string(unknown) != string(arrow) {
		t.Error("неизвестная форма нарисована не стрелкой")
	}
}

func TestWlCursorShape(t *testing.T) {
	// Имена форм wp_cursor_shape_device_v1: движок просит ровно те, что
	// описаны в расширении.
	want := map[int]uint32{
		wlCursorArrow:    1,  // default
		wlCursorIBeam:    9,  // text
		wlCursorHand:     4,  // pointer
		wlCursorSizeWE:   26, // ew-resize
		wlCursorSizeNS:   27, // ns-resize
		wlCursorSizeNWSE: 29, // nwse-resize
		wlCursorSizeNESW: 28, // nesw-resize
	}
	for c, w := range want {
		if got := wlCursorShape(c); got != w {
			t.Errorf("форма %d: shape %d, ждал %d", c, got, w)
		}
	}
	if got := wlCursorShape(wlCursorCount + 1); got != 0 {
		t.Errorf("неизвестная форма: shape %d, ждал 0 (рисовать самим)", got)
	}
}
