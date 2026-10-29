//go:build linux && !android

package core

import (
	"encoding/binary"
	"image"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Мониторы на Wayland: события wl_output / zxdg_output_v1 и полный опрос по
// отдельному подключению против поддельного композитора на настоящем сокете.

func wlGeometry(x, y int32, mk, model string, transform int32) []byte {
	b := wlU32(uint32(x), uint32(y), 300, 200, 0)
	b = append(b, wlStr(mk)...)
	b = append(b, wlStr(model)...)
	return append(b, wlU32(uint32(transform))...)
}

// Без xdg_output размер берётся из режима, делённого на масштаб; положение —
// из geometry.
func TestWaylandMonitors_FromWlOutput(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.outs.add(1, 30)
	c.w.handleEvent(30, wlOutputEvGeometry, wlGeometry(1920, 0, "ACME", "Panel", 0))
	c.w.handleEvent(30, wlOutputEvMode, wlU32(wlOutputModeCurrent, 3840, 2160, 60000))
	c.w.handleEvent(30, wlOutputEvScale, wlU32(2))
	c.w.handleEvent(30, wlOutputEvName, wlStr("DP-1"))

	list := c.w.monitors()
	if len(list) != 1 {
		t.Fatalf("мониторов %d, ждал 1", len(list))
	}
	m := list[0]
	if m.Bounds != image.Rect(1920, 0, 1920+1920, 1080) {
		t.Errorf("границы %v, ждал логические 1920×1080 в (1920,0)", m.Bounds)
	}
	if m.Scale != 2 || m.Name != "DP-1" || !m.Primary || m.Work != m.Bounds {
		t.Errorf("монитор %+v", m)
	}
}

// Режим без флага current — не текущий, его не берём; повёрнутый вывод
// меняет ширину с высотой.
func TestWaylandMonitors_ModeFlagAndRotation(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.outs.add(1, 30)
	c.w.handleEvent(30, wlOutputEvGeometry, wlGeometry(0, 0, "A", "B", 1)) // поворот на 90°
	c.w.handleEvent(30, wlOutputEvMode, wlU32(0, 800, 600, 60000))         // не текущий
	c.w.handleEvent(30, wlOutputEvMode, wlU32(wlOutputModeCurrent, 1920, 1080, 60000))

	m := c.w.monitors()[0]
	if m.Bounds != image.Rect(0, 0, 1080, 1920) {
		t.Errorf("границы %v, ждал повёрнутые 1080×1920", m.Bounds)
	}
	if m.Name != "A B" {
		t.Errorf("имя %q, ждал «изготовитель модель»", m.Name)
	}
}

// xdg_output точнее: логическая позиция и размер главнее режима.
func TestWaylandMonitors_XdgOutputWins(t *testing.T) {
	c := newWlTestWindow(t)
	info := c.w.outs.add(1, 30)
	info.xdgID = 31
	c.w.handleEvent(30, wlOutputEvGeometry, wlGeometry(0, 0, "A", "B", 0))
	c.w.handleEvent(30, wlOutputEvMode, wlU32(wlOutputModeCurrent, 3000, 2000, 60000))
	c.w.handleEvent(30, wlOutputEvScale, wlU32(2))
	c.w.handleEvent(31, xdgOutEvLogicalPosition, wlU32(10, 20))
	c.w.handleEvent(31, xdgOutEvLogicalSize, wlU32(1500, 1000))
	c.w.handleEvent(31, xdgOutEvName, wlStr("eDP-1"))

	m := c.w.monitors()[0]
	if m.Bounds != image.Rect(10, 20, 1510, 1020) {
		t.Errorf("границы %v, ждал логические (10,20)-(1510,1020)", m.Bounds)
	}
	if m.Name != "eDP-1" || m.Scale != 2 {
		t.Errorf("монитор %+v", m)
	}
}

// Битое событие (строка длиннее тела) не роняет цикл и не портит данные.
func TestWaylandMonitors_MalformedEvents(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.outs.add(1, 30)
	c.w.handleEvent(30, wlOutputEvName, []byte{0xFF, 0xFF, 0, 0, 'a'})
	c.w.handleEvent(30, wlOutputEvGeometry, []byte{1, 2, 3})
	c.w.handleEvent(30, wlOutputEvMode, []byte{1})
	if got := c.w.monitors(); len(got) != 0 {
		t.Errorf("монитор без размера в списке: %+v", got)
	}
}

// Монитор окна — тот, на который компоновщик вывел поверхность.
func TestWaylandMonitors_MonitorOfWindow(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.outs.add(1, 30)
	c.w.outs.add(2, 40)
	for _, id := range []uint32{30, 40} {
		c.w.handleEvent(id, wlOutputEvGeometry, wlGeometry(int32(id)*100, 0, "", "", 0))
		c.w.handleEvent(id, wlOutputEvMode, wlU32(wlOutputModeCurrent, 1000, 500, 0))
	}
	if _, ok := c.w.monitorOfWindow(); ok {
		t.Error("окно нигде не показано, а мониторов два — монитор не определим")
	}
	c.w.handleEvent(c.w.surfaceID, wlSurfaceEvEnter, wlU32(40))
	m, ok := c.w.monitorOfWindow()
	if !ok || m.Bounds.Min.X != 4000 || m.Primary {
		t.Errorf("монитор окна %+v %v, ждал второй (x=4000), не основной", m, ok)
	}
}

// ─── Поддельный композитор ───────────────────────────────────────────────────

// wlFakeOutput — вывод поддельного композитора.
type wlFakeOutput struct {
	x, y, w, h, scale int32
	name              string
	logW, logH        int32
}

// serveFakeCompositor отвечает на запросы клиента: get_registry → глобалы
// (xdg_output при withXdg), bind wl_output → события вывода, get_xdg_output →
// логическое описание, sync → done. Читает, пока сокет не закроют.
func serveFakeCompositor(peer *net.UnixConn, outs []wlFakeOutput, withXdg bool) {
	const (
		regID      = 2
		xdgMgrName = 100
	)
	send := func(obj uint32, opcode uint16, body []byte) {
		h := make([]byte, 8)
		binary.LittleEndian.PutUint32(h[0:4], obj)
		binary.LittleEndian.PutUint32(h[4:8], uint32(8+len(body))<<16|uint32(opcode))
		peer.Write(append(h, body...))
	}
	outByID := map[uint32]int{} // объект wl_output → индекс
	var xdgMgrID uint32         // объект zxdg_output_manager_v1, привязанный клиентом

	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := peer.Read(tmp)
		if err != nil {
			return
		}
		buf = append(buf, tmp[:n]...)
		for len(buf) >= 8 {
			size := int(binary.LittleEndian.Uint16(buf[6:8]))
			if size < 8 || size > len(buf) {
				break
			}
			obj := binary.LittleEndian.Uint32(buf[0:4])
			op := binary.LittleEndian.Uint16(buf[4:6])
			body := buf[8:size]
			buf = buf[size:]

			switch {
			case obj == wlDisplayID && op == wlDisplaySync:
				cb := binary.LittleEndian.Uint32(body[0:4])
				send(cb, 0, wlU32(0)) // wl_callback.done
			case obj == wlDisplayID && op == wlDisplayGetRegistry:
				for i := range outs {
					g := append(wlU32(uint32(i+1)), wlStr("wl_output")...)
					send(regID, wlRegistryEvGlobal, append(g, wlU32(4)...))
				}
				if withXdg {
					g := append(wlU32(xdgMgrName), wlStr("zxdg_output_manager_v1")...)
					send(regID, wlRegistryEvGlobal, append(g, wlU32(3)...))
				}
			case obj == regID && op == wlRegistryBind:
				name := binary.LittleEndian.Uint32(body[0:4])
				iface, off := wlString(body, 4)
				id := binary.LittleEndian.Uint32(body[off+4 : off+8])
				if iface == "zxdg_output_manager_v1" {
					xdgMgrID = id
				}
				if iface != "wl_output" {
					continue
				}
				o := outs[name-1]
				outByID[id] = int(name - 1)
				send(id, wlOutputEvGeometry, wlGeometry(o.x, o.y, "FAKE", "Mon", 0))
				send(id, wlOutputEvMode, wlU32(wlOutputModeCurrent, uint32(o.w), uint32(o.h), 60000))
				send(id, wlOutputEvScale, wlU32(uint32(o.scale)))
				send(id, wlOutputEvName, wlStr(o.name))
				send(id, 2, nil) // done
			case obj == xdgMgrID && xdgMgrID != 0 && op == xdgOutMgrGetXdgOutput:
				xid := binary.LittleEndian.Uint32(body[0:4])
				out := binary.LittleEndian.Uint32(body[4:8])
				i, ok := outByID[out]
				if !ok {
					continue // вывод ещё не привязан; клиент шлёт после bind
				}
				o := outs[i]
				send(xid, xdgOutEvLogicalPosition, wlU32(uint32(o.x), uint32(o.y)))
				send(xid, xdgOutEvLogicalSize, wlU32(uint32(o.logW), uint32(o.logH)))
				send(xid, xdgOutEvName, wlStr("xdg-"+o.name))
			}
		}
	}
}

func fakeWlWindow(t *testing.T) (*WaylandWindow, *net.UnixConn) {
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
	client, peer := mk(fds[0], "c"), mk(fds[1], "s")
	t.Cleanup(func() { peer.Close() })
	w := &WaylandWindow{
		conn:       client,
		nextID:     2,
		bufRelease: make(chan struct{}, 1),
		repeat:     newWlRepeater(),
	}
	w.clip = newWlClipboard(w)
	return w, peer
}

var fakeOutputs = []wlFakeOutput{
	{x: 0, y: 0, w: 1920, h: 1080, scale: 1, name: "HDMI-A-1", logW: 1920, logH: 1080},
	{x: 1920, y: 0, w: 3840, h: 2160, scale: 2, name: "DP-2", logW: 1920, logH: 1080},
}

// Опрос без окна: подключение, registry, bind, события, закрытие.
func TestWaylandMonitors_QueryOverWire(t *testing.T) {
	w, peer := fakeWlWindow(t)
	go serveFakeCompositor(peer, fakeOutputs, false)

	list := w.queryMonitors()
	if len(list) != 2 {
		t.Fatalf("мониторов %d, ждал 2: %+v", len(list), list)
	}
	if !list[0].Primary || list[1].Primary {
		t.Errorf("основным должен быть первый: %+v", list)
	}
	if list[0].Bounds != image.Rect(0, 0, 1920, 1080) || list[0].Name != "HDMI-A-1" || list[0].Scale != 1 {
		t.Errorf("первый монитор %+v", list[0])
	}
	// Без xdg_output размер — режим / масштаб.
	if list[1].Bounds != image.Rect(1920, 0, 3840, 1080) || list[1].Scale != 2 {
		t.Errorf("второй монитор %+v", list[1])
	}
}

// С xdg_output: сообщён логический размер и имя xdg_output.
func TestWaylandMonitors_QueryWithXdgOutput(t *testing.T) {
	w, peer := fakeWlWindow(t)
	go serveFakeCompositor(peer, fakeOutputs, true)

	list := w.queryMonitors()
	if len(list) != 2 {
		t.Fatalf("мониторов %d, ждал 2: %+v", len(list), list)
	}
	if list[1].Name != "xdg-DP-2" || list[1].Bounds != image.Rect(1920, 0, 3840, 1080) {
		t.Errorf("второй монитор через xdg_output: %+v", list[1])
	}
	if w.xdgOutMgrID == 0 {
		t.Error("менеджер xdg_output не привязан, хотя объявлен")
	}
}

// Молчащий композитор не вешает опрос навсегда.
func TestWaylandMonitors_QueryTimeout(t *testing.T) {
	w, peer := fakeWlWindow(t)
	_ = peer // ничего не отвечает
	old := wlQueryTimeout
	wlQueryTimeout = 200 * time.Millisecond
	defer func() { wlQueryTimeout = old }()
	done := make(chan []Monitor, 1)
	go func() { done <- w.queryMonitors() }()
	select {
	case list := <-done:
		if len(list) != 0 {
			t.Errorf("получены мониторы от молчащего композитора: %v", list)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("опрос завис")
	}
}
