//go:build linux && !android

package window

import (
	"encoding/binary"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Тесты разговаривают с окном через НАСТОЯЩИЙ unix-сокет: на другом его конце
// вместо композитора сидит тест. Так проверяется весь путь — от вызова метода
// до байтов в проводе, — а не только сборка сообщения.

// wlTestConn — окно и сокет «композитора».
type wlTestConn struct {
	w    *WaylandWindow
	peer *net.UnixConn
}

// newWlTestWindow поднимает пару связанных сокетов и отдаёт окно с уже
// заполненными id объектов (как после Create).
func newWlTestWindow(t *testing.T) *wlTestConn {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Skipf("socketpair недоступен: %v", err)
	}
	mk := func(fd int, name string) *net.UnixConn {
		f := os.NewFile(uintptr(fd), name)
		c, err := net.FileConn(f)
		f.Close()
		if err != nil {
			t.Fatalf("FileConn: %v", err)
		}
		uc, ok := c.(*net.UnixConn)
		if !ok {
			t.Fatalf("не unix-сокет: %T", c)
		}
		return uc
	}
	client := mk(fds[0], "client")
	peer := mk(fds[1], "compositor")

	w := &WaylandWindow{
		conn:       client,
		nextID:     20,
		bufRelease: make(chan struct{}, 1),
		repeat:     newWlRepeater(),
	}
	w.clip = newWlClipboard(w)
	w.toplevelID = 12
	w.seatID = 7
	w.pointerID = 14
	w.surfaceID = 11
	// Глобалы, без которых не создать попап (см. wayland_popup_linux_test.go).
	w.compositorID = 3
	w.shmID = 4
	w.wmBaseID = 5
	w.xdgSurfaceID = 10
	w.width, w.height = 800, 600
	t.Cleanup(func() {
		client.Close()
		peer.Close()
	})
	return &wlTestConn{w: w, peer: peer}
}

// read читает всё, что окно успело отправить.
func (c *wlTestConn) read(t *testing.T) []byte {
	t.Helper()
	c.peer.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 4096)
	n, err := c.peer.Read(buf)
	if err != nil {
		t.Fatalf("композитор ничего не получил: %v", err)
	}
	return buf[:n]
}

// wlWireMsg — разобранное сообщение из провода.
type wlWireMsg struct {
	obj    uint32
	opcode uint16
	args   []uint32
}

// splitWlMsgs режет поток на сообщения.
func splitWlMsgs(t *testing.T, b []byte) []wlWireMsg {
	t.Helper()
	var out []wlWireMsg
	for len(b) >= 8 {
		size := int(binary.LittleEndian.Uint16(b[6:8]))
		if size < 8 || size > len(b) {
			t.Fatalf("битая длина сообщения: %d при %d байтах", size, len(b))
		}
		m := wlWireMsg{
			obj:    binary.LittleEndian.Uint32(b[0:4]),
			opcode: binary.LittleEndian.Uint16(b[4:6]),
		}
		for i := 8; i+4 <= size; i += 4 {
			m.args = append(m.args, binary.LittleEndian.Uint32(b[i:i+4]))
		}
		out = append(out, m)
		b = b[size:]
	}
	return out
}

// find возвращает первое сообщение с данным opcode объекта obj.
func findWlMsg(msgs []wlWireMsg, obj uint32, opcode uint16) (wlWireMsg, bool) {
	for _, m := range msgs {
		if m.obj == obj && m.opcode == opcode {
			return m, true
		}
	}
	return wlWireMsg{}, false
}

func TestWayland_BeginMove_SendsRequest(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.ptrButtonSerial.Store(31)

	if !c.w.BeginMove() {
		t.Fatal("BeginMove отказался, хотя serial нажатия есть")
	}
	msgs := splitWlMsgs(t, c.read(t))
	m, ok := findWlMsg(msgs, 12, xdgToplevelMove)
	if !ok {
		t.Fatalf("xdg_toplevel.move не отправлен, пришло: %+v", msgs)
	}
	if len(m.args) != 2 || m.args[0] != 7 || m.args[1] != 31 {
		t.Errorf("аргументы move: %v, ждал [7 31] (seat, serial нажатия)", m.args)
	}
}

// Без serial'а нажатия просить компоновщик не о чем: он отклонит запрос, а
// окно должно тихо ответить «не умею» — тогда хост подвинет его по-старому.
func TestWayland_BeginMove_WithoutSerial(t *testing.T) {
	c := newWlTestWindow(t)
	if c.w.BeginMove() {
		t.Error("BeginMove согласился без serial'а нажатия")
	}
}

func TestWayland_BeginResize_Edges(t *testing.T) {
	cases := []struct {
		name  string
		edges int
		want  uint32
	}{
		{"правый край", 8, 8},
		{"нижний правый угол", 2 | 8, 10},
		{"верхний левый угол", 1 | 4, 5},
	}
	for _, tc := range cases {
		c := newWlTestWindow(t)
		c.w.ptrButtonSerial.Store(42)
		c.w.resizable.Store(true)

		if !c.w.BeginResize(tc.edges) {
			t.Errorf("%s: BeginResize отказался", tc.name)
			continue
		}
		msgs := splitWlMsgs(t, c.read(t))
		m, ok := findWlMsg(msgs, 12, xdgToplevelResize)
		if !ok {
			t.Errorf("%s: xdg_toplevel.resize не отправлен", tc.name)
			continue
		}
		if len(m.args) != 3 || m.args[2] != tc.want {
			t.Errorf("%s: аргументы resize %v, ждал край %d", tc.name, m.args, tc.want)
		}
	}
}

// Запрет ресайза — это и запрет тянуть за край: окно зафиксировано по размеру,
// компоновщик всё равно ничего бы не сделал.
func TestWayland_BeginResize_NotResizable(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.ptrButtonSerial.Store(42)
	c.w.resizable.Store(false)

	if c.w.BeginResize(8) {
		t.Error("BeginResize согласился у нерастяжимого окна")
	}
}

// Нажатие, которым началось перетаскивание, забирает компоновщик: отпускания
// клиент не получит, поэтому окно отпускает кнопки само.
func TestWayland_BeginMove_ReleasesHeldButtons(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.ptrButtonSerial.Store(5)
	var released []int
	c.w.onMouseButton = func(x, y, button int, pressed bool) {
		if !pressed {
			released = append(released, button)
		}
	}
	// Левая кнопка зажата — как во время перетаскивания за заголовок.
	c.w.ptrButtons.Store(1)

	c.w.BeginMove()
	if len(released) != 1 || released[0] != 0 {
		t.Errorf("отпущено %v, ждал одну левую кнопку", released)
	}
	if got := c.w.ptrButtons.Load(); got != 0 {
		t.Errorf("маска зажатых кнопок %d, ждал 0", got)
	}
}

func TestWayland_MinimizeMaximizeRestore(t *testing.T) {
	c := newWlTestWindow(t)

	c.w.Minimize()
	if _, ok := findWlMsg(splitWlMsgs(t, c.read(t)), 12, xdgToplevelSetMinimized); !ok {
		t.Error("set_minimized не отправлен")
	}

	c.w.Maximize()
	msgs := splitWlMsgs(t, c.read(t))
	if _, ok := findWlMsg(msgs, 12, xdgToplevelSetMaximized); !ok {
		t.Error("set_maximized не отправлен")
	}
	// Фиксация размера снимается: развернуть окно с min == max нельзя.
	if m, ok := findWlMsg(msgs, 12, xdgToplevelSetMaxSize); !ok {
		t.Error("максимум размера не снят перед разворачиванием")
	} else if len(m.args) != 2 || m.args[0] != 0 || m.args[1] != 0 {
		t.Errorf("set_max_size %v, ждал 0×0 (без ограничения)", m.args)
	}

	c.w.Restore()
	if _, ok := findWlMsg(splitWlMsgs(t, c.read(t)), 12, xdgToplevelUnsetMaximized); !ok {
		t.Error("unset_maximized не отправлен")
	}
}

// Запрет ресайза фиксирует окно на текущем размере — под Wayland это
// единственный способ не дать его растянуть.
func TestWayland_SetResizable_FixesSize(t *testing.T) {
	c := newWlTestWindow(t)

	c.w.SetResizable(false)
	msgs := splitWlMsgs(t, c.read(t))
	minM, okMin := findWlMsg(msgs, 12, xdgToplevelSetMinSize)
	maxM, okMax := findWlMsg(msgs, 12, xdgToplevelSetMaxSize)
	if !okMin || !okMax {
		t.Fatalf("размер не зафиксирован, пришло: %+v", msgs)
	}
	if minM.args[0] != 800 || minM.args[1] != 600 || maxM.args[0] != 800 || maxM.args[1] != 600 {
		t.Errorf("min %v и max %v, ждал по 800×600", minM.args, maxM.args)
	}

	c.w.SetResizable(true)
	msgs = splitWlMsgs(t, c.read(t))
	if maxM, ok := findWlMsg(msgs, 12, xdgToplevelSetMaxSize); !ok {
		t.Error("максимум не снят")
	} else if maxM.args[0] != 0 || maxM.args[1] != 0 {
		t.Errorf("set_max_size %v, ждал 0×0", maxM.args)
	}
}

func TestWayland_SetAppID(t *testing.T) {
	c := newWlTestWindow(t)

	c.w.SetAppID("winline-calc")
	msgs := splitWlMsgs(t, c.read(t))
	m, ok := findWlMsg(msgs, 12, xdgToplevelSetAppID)
	if !ok {
		t.Fatal("set_app_id не отправлен")
	}
	if len(m.args) == 0 || m.args[0] != uint32(len("winline-calc")+1) {
		t.Errorf("длина строки в set_app_id: %v", m.args)
	}
	if got := c.w.effectiveAppID(); got != "winline-calc" {
		t.Errorf("effectiveAppID = %q", got)
	}

	// Не задан — имя исполняемого файла, а не общее «headless-gui».
	c2 := newWlTestWindow(t)
	if got, want := c2.w.effectiveAppID(), defaultAppID(os.Args[0]); got != want {
		t.Errorf("по умолчанию app_id = %q, ждал %q", got, want)
	}
}

// configure с состояниями: окно узнаёт, что развёрнуто, и кнопка заголовка
// переключается в «восстановить».
func TestWayland_ConfigureStates(t *testing.T) {
	c := newWlTestWindow(t)
	var active []bool
	c.w.onActivate = func(a bool) { active = append(active, a) }
	var sizes [][2]int
	c.w.onResize = func(w, h int) { sizes = append(sizes, [2]int{w, h}) }

	// nw, nh, массив состояний (длина в байтах + значения).
	body := make([]byte, 12)
	binary.LittleEndian.PutUint32(body[0:4], 1024)
	binary.LittleEndian.PutUint32(body[4:8], 768)
	binary.LittleEndian.PutUint32(body[8:12], 8) // два состояния
	body = binary.LittleEndian.AppendUint32(body, xdgStateMaximized)
	body = binary.LittleEndian.AppendUint32(body, xdgStateActivated)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, body)

	if !c.w.IsMaximized() {
		t.Error("окно не считает себя развёрнутым")
	}
	if len(active) != 1 || !active[0] {
		t.Errorf("onActivate: %v, ждал один вызов с true", active)
	}
	if len(sizes) != 1 || sizes[0] != [2]int{1024, 768} {
		t.Errorf("onResize: %v, ждал один вызов 1024×768", sizes)
	}

	// Компоновщик вернул окно в обычное состояние и не навязывает размер
	// (0×0 — «решай сам»): прежний размер сохраняется.
	body = make([]byte, 12)
	binary.LittleEndian.PutUint32(body[8:12], 0)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, body)
	if c.w.IsMaximized() {
		t.Error("окно осталось развёрнутым после configure без состояний")
	}
	if len(sizes) != 1 {
		t.Errorf("onResize позвали на 0×0: %v", sizes)
	}
	if c.w.width != 1024 || c.w.height != 768 {
		t.Errorf("размер стал %dx%d, ждал прежние 1024×768", c.w.width, c.w.height)
	}
}

// Полноэкранное окно для кнопки «развернуть» — тоже развёрнутое: нажатие
// должно возвращать окно, а не пытаться развернуть его ещё раз.
func TestWayland_FullscreenCountsAsMaximized(t *testing.T) {
	c := newWlTestWindow(t)
	body := make([]byte, 12)
	binary.LittleEndian.PutUint32(body[8:12], 4)
	body = binary.LittleEndian.AppendUint32(body, xdgStateFullscreen)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, body)

	if !c.w.IsMaximized() {
		t.Error("полноэкранное окно не считается развёрнутым")
	}
}

// Указатель ушёл — в том числе потому, что нажатие забрал компоновщик.
// Отпускания не будет, и кнопки отпускает само окно.
func TestWayland_PointerLeaveReleasesButtons(t *testing.T) {
	c := newWlTestWindow(t)
	var released []int
	c.w.onMouseButton = func(x, y, button int, pressed bool) {
		if !pressed {
			released = append(released, button)
		}
	}
	c.w.ptrButtons.Store(1 << 1) // зажата правая

	c.w.handlePointer(wlPointerEvLeave, make([]byte, 8))
	if len(released) != 1 || released[0] != 1 {
		t.Errorf("отпущено %v, ждал правую кнопку", released)
	}
}

// Serial нажатия запоминается, отпускания — нет: компоновщику для move/resize
// нужен именно serial нажатия.
func TestWayland_ButtonSerialRemembered(t *testing.T) {
	c := newWlTestWindow(t)
	mk := func(serial, button, state uint32) []byte {
		b := make([]byte, 16)
		binary.LittleEndian.PutUint32(b[0:4], serial)
		binary.LittleEndian.PutUint32(b[8:12], button)
		binary.LittleEndian.PutUint32(b[12:16], state)
		return b
	}
	c.w.handlePointer(wlPointerEvButton, mk(100, btnLeft, 1))
	if got := c.w.ptrButtonSerial.Load(); got != 100 {
		t.Errorf("serial нажатия %d, ждал 100", got)
	}
	c.w.handlePointer(wlPointerEvButton, mk(101, btnLeft, 0))
	if got := c.w.ptrButtonSerial.Load(); got != 100 {
		t.Errorf("serial после отпускания %d, ждал прежние 100", got)
	}
}
