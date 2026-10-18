//go:build linux && !android

package window

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Начальное «развёрнуто» и размер на Wayland, на проводе (initial_wayland_linux.go).
//
// Create настоящий, против поддельного компоновщика: он выдаёт глобалы,
// отвечает на sync и на первый commit присылает configure с заказанным
// размером и состояниями. Тест записывает запросы по порядку и проверяет, что
// xdg_toplevel.set_maximized ушёл ДО первого commit, а размер не зафиксирован.

// initWlMsg — запрос окна: интерфейс объекта, опкод и тело (аргументы).
type initWlMsg struct {
	iface  string
	obj    uint32
	opcode uint16
	args   []byte
}

// initWlServer — поддельный компоновщик.
type initWlServer struct {
	conn *net.UnixConn

	// Что присылать в первом configure: размер и состояния.
	cfgW, cfgH int32
	cfgStates  []uint32

	mu      sync.Mutex
	msgs    []initWlMsg
	ids     map[uint32]string
	commits int
	top     uint32
	xdgSurf uint32
}

func newInitWlServer(conn *net.UnixConn, w, h int32, states ...uint32) *initWlServer {
	return &initWlServer{conn: conn, cfgW: w, cfgH: h, cfgStates: states,
		ids: map[uint32]string{1: "wl_display"}}
}

func wlEvent(obj uint32, opcode uint16, body []byte) []byte {
	b := make([]byte, 8+len(body))
	binary.LittleEndian.PutUint32(b[0:4], obj)
	binary.LittleEndian.PutUint32(b[4:8], uint32(8+len(body))<<16|uint32(opcode))
	copy(b[8:], body)
	return b
}

func (s *initWlServer) send(b []byte) { s.conn.Write(b) }

func (s *initWlServer) serve() {
	for {
		hdr := make([]byte, 8)
		oob := make([]byte, 64)
		n, _, _, _, err := s.conn.ReadMsgUnix(hdr, oob)
		if err != nil {
			return
		}
		if n < 8 {
			if _, err := io.ReadFull(s.conn, hdr[n:]); err != nil {
				return
			}
		}
		obj := binary.LittleEndian.Uint32(hdr[0:4])
		word := binary.LittleEndian.Uint32(hdr[4:8])
		opcode, size := uint16(word&0xFFFF), int(word>>16)
		body := make([]byte, size-8)
		if _, err := io.ReadFull(s.conn, body); err != nil {
			return
		}
		s.handle(obj, opcode, body)
	}
}

func (s *initWlServer) handle(obj uint32, opcode uint16, body []byte) {
	s.mu.Lock()
	iface := s.ids[obj]
	s.msgs = append(s.msgs, initWlMsg{iface: iface, obj: obj, opcode: opcode, args: body})
	s.mu.Unlock()

	u32 := func(off int) uint32 { return binary.LittleEndian.Uint32(body[off : off+4]) }
	setID := func(id uint32, name string) {
		s.mu.Lock()
		s.ids[id] = name
		s.mu.Unlock()
	}
	switch {
	case iface == "wl_display" && opcode == wlDisplayGetRegistry:
		reg := u32(0)
		setID(reg, "wl_registry")
		for i, g := range []struct {
			name string
			ver  uint32
		}{{"wl_compositor", 4}, {"wl_shm", 1}, {"xdg_wm_base", 1}} {
			b := binary.LittleEndian.AppendUint32(nil, uint32(i+1))
			b = append(b, wlStr(g.name)...)
			b = binary.LittleEndian.AppendUint32(b, g.ver)
			s.send(wlEvent(reg, wlRegistryEvGlobal, b))
		}
	case iface == "wl_display" && opcode == wlDisplaySync:
		cb := u32(0)
		setID(cb, "wl_callback")
		s.send(wlEvent(cb, 0, make([]byte, 4)))
	case iface == "wl_registry" && opcode == wlRegistryBind:
		// name, interface (строка), version, new_id
		sl := int(u32(4))
		off := 8 + (sl+3)/4*4
		name := string(body[8 : 8+sl-1])
		setID(u32(off+4), name)
	case iface == "wl_compositor" && opcode == wlCompositorCreateSurface:
		setID(u32(0), "wl_surface")
	case iface == "xdg_wm_base" && opcode == xdgWmBaseGetXdgSurface:
		setID(u32(0), "xdg_surface")
		s.mu.Lock()
		s.xdgSurf = u32(0)
		s.mu.Unlock()
	case iface == "xdg_surface" && opcode == xdgSurfaceGetToplevel:
		setID(u32(0), "xdg_toplevel")
		s.mu.Lock()
		s.top = u32(0)
		s.mu.Unlock()
	case iface == "wl_shm" && opcode == 0: // create_pool
		setID(u32(0), "wl_shm_pool")
	case iface == "wl_surface" && opcode == wlSurfaceCommit:
		s.mu.Lock()
		s.commits++
		first := s.commits == 1
		top, xs := s.top, s.xdgSurf
		s.mu.Unlock()
		if first {
			st := binary.LittleEndian.AppendUint32(nil, uint32(4*len(s.cfgStates)))
			for _, v := range s.cfgStates {
				st = binary.LittleEndian.AppendUint32(st, v)
			}
			b := binary.LittleEndian.AppendUint32(nil, uint32(s.cfgW))
			b = binary.LittleEndian.AppendUint32(b, uint32(s.cfgH))
			b = append(b, st...)
			s.send(wlEvent(top, xdgToplevelEvConfigure, b))
			s.send(wlEvent(xs, xdgSurfaceEvConfigure, binary.LittleEndian.AppendUint32(nil, 1)))
		}
	}
}

func (s *initWlServer) snapshot() []initWlMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]initWlMsg(nil), s.msgs...)
}

// createInitWayland создаёт окно против поддельного компоновщика, который на
// первый commit отвечает configure(cfgW×cfgH, states).
func createInitWayland(t *testing.T, p initialPlacement, minW, minH, width, height int, cfgW, cfgH int32, states ...uint32) (*WaylandWindow, []initWlMsg) {
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
			t.Fatal(err)
		}
		return c.(*net.UnixConn)
	}
	client, peer := mk(fds[0], "client"), mk(fds[1], "compositor")
	t.Cleanup(func() { client.Close(); peer.Close() })

	srv := newInitWlServer(peer, cfgW, cfgH, states...)
	go srv.serve()

	w := &WaylandWindow{conn: client, nextID: 2, bufRelease: make(chan struct{}, 1), repeat: newWlRepeater()}
	w.clip = newWlClipboard(w)
	w.setInitialPlacement(p)
	if minW > 0 || minH > 0 {
		w.SetMinSize(minW, minH)
	}
	done := make(chan error, 1)
	go func() { done <- w.Create("initial-wl", width, height) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Create завис")
	}
	// Хвост запросов (буфер первого кадра) ещё читается сервером.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.Lock()
		n := srv.commits
		srv.mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	return w, srv.snapshot()
}

func wlIndex(msgs []initWlMsg, iface string, opcode uint16) int {
	for i, m := range msgs {
		if m.iface == iface && m.opcode == opcode {
			return i
		}
	}
	return -1
}

const wlStateMaximized, wlStateActivated = xdgStateMaximized, xdgStateActivated

// Развёрнуто: set_maximized уходит после get_toplevel и ДО первого commit.
func TestWaylandInitial_SetMaximizedBeforeFirstCommit(t *testing.T) {
	w, msgs := createInitWayland(t, initialPlacement{Maximized: true}, 0, 0, 800, 600,
		1920, 1080, wlStateMaximized, wlStateActivated)

	top := wlIndex(msgs, "xdg_surface", xdgSurfaceGetToplevel)
	mx := wlIndex(msgs, "xdg_toplevel", xdgToplevelSetMaximized)
	commit := wlIndex(msgs, "wl_surface", wlSurfaceCommit)
	if mx < 0 {
		t.Fatal("xdg_toplevel.set_maximized не отправлен")
	}
	if !(top < mx && mx < commit) {
		t.Errorf("порядок: get_toplevel=%d set_maximized=%d первый commit=%d", top, mx, commit)
	}
	// Размер не зафиксирован: ни одного set_max_size с ненулевым размером.
	for _, m := range msgs {
		if m.iface == "xdg_toplevel" && m.opcode == xdgToplevelSetMaxSize {
			if binary.LittleEndian.Uint32(m.args[0:4]) != 0 || binary.LittleEndian.Uint32(m.args[4:8]) != 0 {
				t.Errorf("развёрнутое окно зафиксировано: set_max_size %v", m.args)
			}
		}
	}
	if i := wlIndex(msgs, "xdg_toplevel", xdgToplevelSetMinSize); i >= 0 {
		if a := msgs[i].args; binary.LittleEndian.Uint32(a[0:4]) != 0 && binary.LittleEndian.Uint32(a[4:8]) != 0 {
			t.Errorf("развёрнутое окно зафиксировано: set_min_size %v", a)
		}
	}
	// Итог: окно развёрнуто, размер — компоновщика.
	if !w.IsMaximized() {
		t.Error("IsMaximized false после первого configure с состоянием maximized")
	}
	if cw, ch := w.clientSize(); cw != 1920 || ch != 1080 {
		t.Errorf("clientSize %dx%d, ждал 1920x1080 от компоновщика", cw, ch)
	}
}

// Окно, начавшее жизнь развёрнутым, при «восстановить» возвращается к размеру
// создания, даже если компоновщик присылает 0×0.
func TestWaylandInitial_RestoreReturnsToInitialSize(t *testing.T) {
	w, _ := createInitWayland(t, initialPlacement{Maximized: true, SizeSet: true}, 0, 0, 900, 650,
		1920, 1080, wlStateMaximized)

	w.handleEvent(w.toplevelID, xdgToplevelEvConfigure, toplevelConfigure(0, 0))
	if w.IsMaximized() {
		t.Fatal("после configure без maximized окно считается развёрнутым")
	}
	if cw, ch := w.clientSize(); cw != 900 || ch != 650 {
		t.Errorf("после восстановления %dx%d, ждал размер создания 900x650", cw, ch)
	}
	// Запомненный размер израсходован: следующий 0×0 размера не трогает.
	w.handleEvent(w.toplevelID, xdgToplevelEvConfigure, toplevelConfigure(1000, 700))
	w.handleEvent(w.toplevelID, xdgToplevelEvConfigure, toplevelConfigure(0, 0))
	if cw, ch := w.clientSize(); cw != 1000 || ch != 700 {
		t.Errorf("повторное 0×0 изменило размер: %dx%d", cw, ch)
	}
}

// Компоновщик отказал в разворачивании: окно обычное и честно об этом сообщает.
func TestWaylandInitial_RefusedMaximize(t *testing.T) {
	w, _ := createInitWayland(t, initialPlacement{Maximized: true}, 0, 0, 800, 600, 800, 600, wlStateActivated)
	if w.IsMaximized() {
		t.Error("IsMaximized true, хотя компоновщик не дал состояния maximized")
	}
}

// Положение игнорируется (его выбирает компоновщик), а размер — начальный:
// окно фиксируется на нём, как и раньше.
func TestWaylandInitial_PositionIgnoredSizeUsed(t *testing.T) {
	w, msgs := createInitWayland(t, initialPlacement{PosSet: true, X: 300, Y: 200, SizeSet: true}, 0, 0, 640, 480, 0, 0)

	if i := wlIndex(msgs, "xdg_toplevel", xdgToplevelSetMaximized); i >= 0 {
		t.Error("set_maximized отправлен без заказа")
	}
	i := wlIndex(msgs, "xdg_toplevel", xdgToplevelSetMaxSize)
	if i < 0 {
		t.Fatal("обычное окно не зафиксировано set_max_size (прежнее поведение потеряно)")
	}
	if a := msgs[i].args; binary.LittleEndian.Uint32(a[0:4]) != 640 || binary.LittleEndian.Uint32(a[4:8]) != 480 {
		t.Errorf("set_max_size %v, ждал 640x480", a)
	}
	if x, y := w.GetPosition(); x != 0 || y != 0 {
		t.Errorf("GetPosition %d,%d: положение окна принадлежит компоновщику", x, y)
	}
	if w.IsMaximized() {
		t.Error("обычное окно считается развёрнутым")
	}
}

// Без заказа Wayland ведёт себя как раньше: set_maximized не уходит.
func TestWaylandInitial_NothingRequested(t *testing.T) {
	_, msgs := createInitWayland(t, initialPlacement{}, 0, 0, 800, 600, 0, 0)
	if wlIndex(msgs, "xdg_toplevel", xdgToplevelSetMaximized) >= 0 {
		t.Error("set_maximized отправлен без заказа")
	}
	if wlIndex(msgs, "xdg_toplevel", xdgToplevelSetMaxSize) < 0 {
		t.Error("обычное окно не зафиксировано")
	}
}

// Минимум, заданный приложением, развёрнутое окно не теряет.
func TestWaylandInitial_MaximizedKeepsMinSize(t *testing.T) {
	_, msgs := createInitWayland(t, initialPlacement{Maximized: true}, 400, 300, 800, 600,
		1920, 1080, wlStateMaximized)
	last := -1
	for i, m := range msgs {
		if m.iface == "xdg_toplevel" && m.opcode == xdgToplevelSetMinSize {
			last = i
		}
	}
	if last < 0 {
		t.Fatal("минимум не отправлен")
	}
	a := msgs[last].args
	if binary.LittleEndian.Uint32(a[0:4]) != 400 || binary.LittleEndian.Uint32(a[4:8]) != 300 {
		t.Errorf("последний set_min_size %v, ждал 400x300", a)
	}
}
