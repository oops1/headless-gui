//go:build linux && !android

package core

import (
	"encoding/binary"
	"image"
	"net"
	"os"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// Полный экран и мониторы X11 на проводе: окно разговаривает с настоящим
// unix-сокетом, на другом конце которого сидит поддельный X-сервер — читает
// запросы по длине из заголовка и отвечает ровно теми байтами, какими ответил
// бы сервер.

// x11Pair — пара связанных сокетов: клиент (соединение окна) и «сервер».
func x11Pair(t *testing.T) (client, server net.Conn) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Skipf("socketpair недоступен: %v", err)
	}
	mk := func(fd int, name string) net.Conn {
		f := os.NewFile(uintptr(fd), name)
		c, err := net.FileConn(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	client, server = mk(fds[0], "client"), mk(fds[1], "server")
	t.Cleanup(func() { client.Close(); server.Close() })
	return client, server
}

// x11Request — один запрос клиента: опкод, второй байт и весь пакет.
type x11Request struct {
	major, data byte
	raw         []byte
}

// serveFakeX читает запросы и отдаёт каждый в handler; непустой результат
// записывается клиенту как ответ. Читает, пока сокет не закроют.
func serveFakeX(server net.Conn, handler func(r x11Request) []byte) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := server.Read(tmp)
		if err != nil {
			return
		}
		buf = append(buf, tmp[:n]...)
		for len(buf) >= 4 {
			size := int(binary.LittleEndian.Uint16(buf[2:4])) * 4
			if size < 4 || size > len(buf) {
				break
			}
			req := x11Request{major: buf[0], data: buf[1], raw: append([]byte(nil), buf[:size]...)}
			buf = buf[size:]
			if reply := handler(req); len(reply) > 0 {
				server.Write(reply)
			}
		}
	}
}

// x11Reply собирает ответ: 32 байта заголовка (byte1 — «data» ответа) и тело.
func x11Reply(byte1 byte, set func(h []byte), body []byte) []byte {
	for len(body)%4 != 0 {
		body = append(body, 0)
	}
	h := make([]byte, 32)
	h[0], h[1] = 1, byte1
	binary.LittleEndian.PutUint32(h[4:8], uint32(len(body)/4))
	if set != nil {
		set(h)
	}
	return append(h, body...)
}

// x11PropReply — ответ GetProperty со списком атомов (format 32, тип ATOM).
func x11PropReply(atoms ...uint32) []byte {
	var body []byte
	for _, a := range atoms {
		body = binary.LittleEndian.AppendUint32(body, a)
	}
	return x11Reply(32, func(h []byte) {
		if len(atoms) > 0 {
			binary.LittleEndian.PutUint32(h[8:12], 4) // тип ATOM
		}
		binary.LittleEndian.PutUint32(h[16:20], uint32(len(atoms)))
	}, body)
}

const (
	tAtomState = 300 // _NET_WM_STATE
	tAtomFull  = 301 // _NET_WM_STATE_FULLSCREEN
	tAtomMaxH  = 302
	tAtomMaxV  = 303
	tAtomSupp  = 304 // _NET_SUPPORTED
	tRoot      = 1
	tWid       = 77
)

func newFsX11(t *testing.T) (*X11Window, net.Conn) {
	t.Helper()
	client, server := x11Pair(t)
	w := &X11Window{
		conn: client, rootWin: tRoot, wid: tWid,
		atomNetWMState: tAtomState, atomWMStateFullscreen: tAtomFull,
		atomWMStateMaxH: tAtomMaxH, atomWMStateMaxV: tAtomMaxV, atomNetSupported: tAtomSupp,
	}
	return w, server
}

func readN(t *testing.T, c net.Conn, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	for got := 0; got < n; {
		k, err := c.Read(b[got:])
		if err != nil {
			t.Fatalf("сервер получил %d байт из %d: %v", got, n, err)
		}
		got += k
	}
	return b
}

// Запрос уходит ClientMessage'ем _NET_WM_STATE к корневому окну: ADD/REMOVE и
// атом _NET_WM_STATE_FULLSCREEN.
func TestX11_SetFullscreen_ClientMessage(t *testing.T) {
	for _, tc := range []struct {
		on     bool
		action uint32
	}{{true, 1}, {false, 0}} {
		w, server := newFsX11(t)
		if !w.SetFullscreen(tc.on) {
			t.Fatalf("SetFullscreen(%v) вернул false", tc.on)
		}
		b := readN(t, server, 44)
		if b[0] != 25 { // SendEvent
			t.Fatalf("опкод %d, ждал SendEvent (25)", b[0])
		}
		if dst := binary.LittleEndian.Uint32(b[4:8]); dst != tRoot {
			t.Errorf("адресат %d, ждал корневое окно", dst)
		}
		if mask := binary.LittleEndian.Uint32(b[8:12]); mask != 0x00180000 {
			t.Errorf("маска события %#x, ждал SubstructureRedirect|Notify", mask)
		}
		msg := b[12:]
		if msg[0] != 33 || msg[1] != 32 {
			t.Errorf("не ClientMessage формата 32: %v", msg[:2])
		}
		le := binary.LittleEndian
		if le.Uint32(msg[4:8]) != tWid || le.Uint32(msg[8:12]) != tAtomState {
			t.Errorf("окно/тип сообщения: %d/%d", le.Uint32(msg[4:8]), le.Uint32(msg[8:12]))
		}
		if le.Uint32(msg[12:16]) != tc.action || le.Uint32(msg[16:20]) != tAtomFull {
			t.Errorf("on=%v: действие %d, свойство %d; ждал %d и %d",
				tc.on, le.Uint32(msg[12:16]), le.Uint32(msg[16:20]), tc.action, tAtomFull)
		}
		if le.Uint32(msg[24:28]) != 1 {
			t.Errorf("источник %d, ждал 1 (приложение)", le.Uint32(msg[24:28]))
		}
	}
}

func TestX11_SetFullscreen_Unsupported(t *testing.T) {
	w, _ := newFsX11(t)
	w.fs.unsupported = true
	if w.SetFullscreen(true) {
		t.Error("менеджер без полного экрана, а SetFullscreen вернул true")
	}
	w, _ = newFsX11(t)
	w.wid = 0
	if w.SetFullscreen(true) {
		t.Error("окно не создано, а SetFullscreen вернул true")
	}
	w, _ = newFsX11(t)
	w.atomWMStateFullscreen = 0
	if w.SetFullscreen(true) {
		t.Error("атом не интернирован, а SetFullscreen вернул true")
	}
}

// propNotify — событие PropertyNotify: окно @4, атом @8.
func propNotify(win, atom uint32) []byte {
	b := make([]byte, 32)
	b[0] = x11EvPropertyNotify
	binary.LittleEndian.PutUint32(b[4:8], win)
	binary.LittleEndian.PutUint32(b[8:12], atom)
	return b
}

// Состояние берётся из свойства _NET_WM_STATE по PropertyNotify: менеджер мог
// отказать или вернуть окно сам.
func TestX11_Fullscreen_StateFromProperty(t *testing.T) {
	w, server := newFsX11(t)
	var mu sync.Mutex
	var state []uint32
	go serveFakeX(server, func(r x11Request) []byte {
		if r.major != 20 { // GetProperty
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		return x11PropReply(state...)
	})
	set := func(atoms ...uint32) { mu.Lock(); state = atoms; mu.Unlock() }
	var got []bool
	w.SetOnFullscreenChanged(func(on bool) { got = append(got, on) })

	// Чужое окно и чужое свойство не вызывают перечитывание.
	w.handleX11Event(propNotify(tWid+1, tAtomState))
	w.handleX11Event(propNotify(tWid, 999))
	w.x11SyncWMState()
	if len(got) != 0 || w.IsFullscreen() {
		t.Fatal("посторонний PropertyNotify изменил состояние")
	}

	// Менеджер перевёл окно в полный экран.
	set(tAtomFull)
	w.handleX11Event(propNotify(tWid, tAtomState))
	w.x11SyncWMState()
	if !w.IsFullscreen() || len(got) != 1 || !got[0] {
		t.Fatalf("полный экран: IsFullscreen=%v, уведомления %v", w.IsFullscreen(), got)
	}

	// То же состояние ещё раз — без уведомления.
	w.handleX11Event(propNotify(tWid, tAtomState))
	w.x11SyncWMState()
	if len(got) != 1 {
		t.Errorf("повторное состояние дало уведомление: %v", got)
	}

	// Менеджер вернул окно, развернув его.
	set(tAtomMaxH, tAtomMaxV)
	w.handleX11Event(propNotify(tWid, tAtomState))
	w.x11SyncWMState()
	if w.IsFullscreen() || len(got) != 2 || got[1] {
		t.Fatalf("выход: IsFullscreen=%v, уведомления %v", w.IsFullscreen(), got)
	}
	if !w.IsMaximized() {
		t.Error("менеджер развернул окно, а IsMaximized false")
	}

	// В полном экране «развёрнуто» хранит прежнее значение, даже если менеджер
	// на время убрал maximized из свойства.
	set(tAtomFull)
	w.handleX11Event(propNotify(tWid, tAtomState))
	w.x11SyncWMState()
	if !w.IsMaximized() {
		t.Error("IsMaximized потерял прежнее значение в полном экране")
	}
	set(tAtomMaxH, tAtomMaxV)
	w.handleX11Event(propNotify(tWid, tAtomState))
	w.x11SyncWMState()
	if !w.IsMaximized() || w.IsFullscreen() {
		t.Errorf("после выхода: развёрнуто=%v, полный экран=%v", w.IsMaximized(), w.IsFullscreen())
	}
}

// Менеджер, объявивший список поддерживаемого без полного экрана, получает
// отказ сразу; без списка (нет EWMH-менеджера) запрос всё же отправляется.
func TestX11_DetectFullscreenSupport(t *testing.T) {
	for _, tc := range []struct {
		name        string
		supported   []uint32
		unsupported bool
	}{
		{"поддерживается", []uint32{tAtomState, tAtomFull}, false},
		{"в списке нет", []uint32{tAtomState}, true},
		{"списка нет", nil, false},
	} {
		w, server := newFsX11(t)
		go serveFakeX(server, func(r x11Request) []byte {
			if r.major == 20 {
				return x11PropReply(tc.supported...)
			}
			return nil
		})
		w.x11DetectFullscreenSupport()
		if w.fs.unsupported != tc.unsupported {
			t.Errorf("%s: unsupported=%v, ждал %v", tc.name, w.fs.unsupported, tc.unsupported)
		}
	}
}

// ─── Мониторы ────────────────────────────────────────────────────────────────

// rrMonitorRecord — один MONITORINFO ответа RRGetMonitors (24 байта).
func rrMonitorRecord(name uint32, primary bool, x, y, w, h int16, nOutputs int) []byte {
	b := make([]byte, 24, 24+4*nOutputs)
	binary.LittleEndian.PutUint32(b[0:4], name)
	if primary {
		b[4] = 1
	}
	b[5] = 1 // automatic
	binary.LittleEndian.PutUint16(b[6:8], uint16(nOutputs))
	binary.LittleEndian.PutUint16(b[8:10], uint16(x))
	binary.LittleEndian.PutUint16(b[10:12], uint16(y))
	binary.LittleEndian.PutUint16(b[12:14], uint16(w))
	binary.LittleEndian.PutUint16(b[14:16], uint16(h))
	for i := 0; i < nOutputs; i++ {
		b = binary.LittleEndian.AppendUint32(b, uint32(900+i))
	}
	return b
}

const (
	tRandrMajor = 140
	tAtomWork   = 400 // _NET_WORKAREA
)

// fakeRandr — поддельный сервер RandR: версия, мониторы, имена атомов,
// _NET_WORKAREA. present=false — расширения нет вовсе.
func fakeRandr(present bool, major, minor uint32, monitors ...[]byte) func(x11Request) []byte {
	names := map[uint32]string{500: "HDMI-1", 501: "DP-1"}
	return func(r x11Request) []byte {
		switch r.major {
		case 98: // QueryExtension
			return x11Reply(0, func(h []byte) {
				if present {
					h[8], h[9] = 1, tRandrMajor
				}
			}, nil)
		case 16: // InternAtom
			return x11Reply(0, func(h []byte) { binary.LittleEndian.PutUint32(h[8:12], tAtomWork) }, nil)
		case 17: // GetAtomName
			name := names[binary.LittleEndian.Uint32(r.raw[4:8])]
			return x11Reply(0, func(h []byte) { binary.LittleEndian.PutUint16(h[8:10], uint16(len(name))) }, []byte(name))
		case 20: // GetProperty
			if binary.LittleEndian.Uint32(r.raw[8:12]) == tAtomWork {
				var d []byte
				for _, v := range []uint32{0, 0, 3200, 1040} { // рабочая область без панели 40 px внизу
					d = binary.LittleEndian.AppendUint32(d, v)
				}
				return x11Reply(32, func(h []byte) {
					binary.LittleEndian.PutUint32(h[8:12], 6) // CARDINAL
					binary.LittleEndian.PutUint32(h[16:20], 4)
				}, d)
			}
			return x11Reply(0, nil, nil) // RESOURCE_MANAGER пуст → масштаб 1
		case tRandrMajor:
			switch r.data {
			case randrQueryVersion:
				return x11Reply(0, func(h []byte) {
					binary.LittleEndian.PutUint32(h[8:12], major)
					binary.LittleEndian.PutUint32(h[12:16], minor)
				}, nil)
			case randrGetMonitors:
				var body []byte
				for _, m := range monitors {
					body = append(body, m...)
				}
				return x11Reply(0, func(h []byte) {
					binary.LittleEndian.PutUint32(h[12:16], uint32(len(monitors)))
				}, body)
			}
		}
		return nil
	}
}

func fsX11Monitors(t *testing.T, handler func(x11Request) []byte) []Monitor {
	t.Helper()
	t.Setenv("GDK_SCALE", "") // масштаб из окружения разработчика тесту не нужен
	client, server := x11Pair(t)
	w := &X11Window{conn: client, rootWin: tRoot}
	w.screen.WidthInPixels, w.screen.HeightInPixels = 3200, 1080
	go serveFakeX(server, handler)
	return w.x11Monitors()
}

// RandR 1.5: границы, основной, имена; рабочая область — по _NET_WORKAREA.
func TestX11Monitors_RandR15(t *testing.T) {
	got := fsX11Monitors(t, fakeRandr(true, 1, 5,
		rrMonitorRecord(500, true, 0, 0, 1920, 1080, 0),
		rrMonitorRecord(501, false, 1920, 0, 1280, 1024, 2), // с двумя выводами: запись длиннее
	))
	if len(got) != 2 {
		t.Fatalf("мониторов %d, ждал 2: %+v", len(got), got)
	}
	a, b := got[0], got[1]
	if a.Name != "HDMI-1" || !a.Primary || a.Bounds != image.Rect(0, 0, 1920, 1080) || a.Scale != 1 {
		t.Errorf("первый монитор: %+v", a)
	}
	if b.Name != "DP-1" || b.Primary || b.Bounds != image.Rect(1920, 0, 3200, 1024) {
		t.Errorf("второй монитор (после записи с несколькими выводами): %+v", b)
	}
	if a.Work != image.Rect(0, 0, 1920, 1040) {
		t.Errorf("рабочая область %v, ждал без нижней панели", a.Work)
	}
}

// RandR старше 1.5 или без расширения: честно один монитор — весь экран.
func TestX11Monitors_Fallback(t *testing.T) {
	for _, tc := range []struct {
		name string
		h    func(x11Request) []byte
	}{
		{"RandR 1.3", fakeRandr(true, 1, 3)},
		{"нет RandR", fakeRandr(false, 0, 0)},
	} {
		got := fsX11Monitors(t, tc.h)
		if len(got) != 1 || got[0].Bounds != image.Rect(0, 0, 3200, 1080) || !got[0].Primary {
			t.Errorf("%s: %+v, ждал один основной монитор на весь экран", tc.name, got)
		}
	}
}

// Без основного в ответе (бывает) первый назначается основным.
func TestX11Monitors_NoPrimaryFlagged(t *testing.T) {
	got := fsX11Monitors(t, fakeRandr(true, 1, 5, rrMonitorRecord(500, false, 0, 0, 800, 600, 0)))
	if len(got) != 1 || !got[0].Primary {
		t.Errorf("%+v", got)
	}
}

// Обрезанный ответ читается до конца, а не паникует.
func TestParseRRMonitors_Truncated(t *testing.T) {
	reply := x11Reply(0, func(h []byte) { binary.LittleEndian.PutUint32(h[12:16], 3) },
		rrMonitorRecord(500, true, 0, 0, 100, 100, 0))
	if got := parseRRMonitors(reply); len(got) != 1 {
		t.Errorf("разобрано %d мониторов из 1 целого + обрыва", len(got))
	}
	if got := parseRRMonitors([]byte{1, 2, 3}); got != nil {
		t.Errorf("мусор разобран как %v", got)
	}
}
