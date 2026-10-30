//go:build linux && !android

package x11

import (
	"encoding/binary"
	"image"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// Полный экран на выбранном мониторе X11 (SetFullscreenOn), на проводе: что
// уходит серверу при менеджере с _NET_WM_FULLSCREEN_MONITORS и без него.

const tAtomFsMon = 305 // _NET_WM_FULLSCREEN_MONITORS

// fsOnLog — запросы, которые «сервер» получил от окна.
type fsOnLog struct {
	mu   sync.Mutex
	reqs []x11Request
}

func (l *fsOnLog) add(r x11Request) { l.mu.Lock(); l.reqs = append(l.reqs, r); l.mu.Unlock() }

// wait ждёт n запросов (запись идёт в другой горутине).
func (l *fsOnLog) wait(t *testing.T, n int) []x11Request {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		got := len(l.reqs)
		l.mu.Unlock()
		if got >= n {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // лишние запросы успели бы прийти
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]x11Request(nil), l.reqs...)
	if len(out) != n {
		t.Fatalf("сервер получил %d запросов, ждал %d", len(out), n)
	}
	return out
}

// fsOnWindow — окно с поддельным сервером и списком из двух мониторов.
func fsOnWindow(t *testing.T, monitorsSupported bool) (*X11Window, *fsOnLog) {
	t.Helper()
	w, server := newFsX11(t)
	w.atomFsMonitors = tAtomFsMon
	w.fs.monitorsSupported = monitorsSupported
	log := &fsOnLog{}
	go serveFakeX(server, func(r x11Request) []byte { log.add(r); return nil })

	old := x11MonitorList
	x11MonitorList = func() []native.Monitor {
		return []native.Monitor{
			native.MonitorWithKey(native.Monitor{Name: "HDMI-1", Bounds: image.Rect(0, 0, 1920, 1080), Primary: true}, 1),
			native.MonitorWithKey(native.Monitor{Name: "DP-1", Bounds: image.Rect(1920, 0, 3840, 1080)}, 2),
		}
	}
	t.Cleanup(func() { x11MonitorList = old })
	return w, log
}

// stateMsg разбирает SendEvent _NET_WM_STATE: действие и свойство.
func fsOnState(t *testing.T, r x11Request) (action, prop uint32) {
	t.Helper()
	if r.major != 25 || len(r.raw) != 44 {
		t.Fatalf("ждал SendEvent (44 байта), получил опкод %d длиной %d", r.major, len(r.raw))
	}
	msg := r.raw[12:]
	if binary.LittleEndian.Uint32(msg[8:12]) != tAtomState {
		t.Fatalf("не _NET_WM_STATE: тип %d", binary.LittleEndian.Uint32(msg[8:12]))
	}
	return binary.LittleEndian.Uint32(msg[12:16]), binary.LittleEndian.Uint32(msg[16:20])
}

// configurePos разбирает ConfigureWindow x|y.
func fsOnConfigure(t *testing.T, r x11Request) (x, y int) {
	t.Helper()
	if r.major != 12 || binary.LittleEndian.Uint16(r.raw[8:10]) != 0x03 {
		t.Fatalf("ждал ConfigureWindow x|y, получил опкод %d", r.major)
	}
	return int(int32(binary.LittleEndian.Uint32(r.raw[12:16]))), int(int32(binary.LittleEndian.Uint32(r.raw[16:20])))
}

// Менеджер объявил _NET_WM_FULLSCREEN_MONITORS: сначала индекс монитора (все
// четыре края), затем ADD fullscreen; окно с места не двигается.
func TestX11_FullscreenOn_WithMonitorsHint(t *testing.T) {
	w, log := fsOnWindow(t, true)
	if !w.SetFullscreenOn(native.MonitorWithKey(native.Monitor{Name: "DP-1"}, 2)) {
		t.Fatal("SetFullscreenOn вернул false")
	}
	reqs := log.wait(t, 2)

	msg := reqs[0].raw
	if msg[0] != 25 || binary.LittleEndian.Uint32(msg[4:8]) != tRoot {
		t.Fatalf("первый запрос не SendEvent корневому окну: %v", msg[:8])
	}
	ev := msg[12:]
	le := binary.LittleEndian
	if le.Uint32(ev[4:8]) != tWid || le.Uint32(ev[8:12]) != tAtomFsMon {
		t.Errorf("окно/тип %d/%d, ждал %d/%d", le.Uint32(ev[4:8]), le.Uint32(ev[8:12]), tWid, tAtomFsMon)
	}
	for i := 0; i < 4; i++ {
		if got := le.Uint32(ev[12+i*4 : 16+i*4]); got != 1 {
			t.Errorf("край %d: индекс монитора %d, ждал 1 (второй, DP-1)", i, got)
		}
	}
	if le.Uint32(ev[28:32]) != 1 {
		t.Errorf("источник %d, ждал 1 (приложение)", le.Uint32(ev[28:32]))
	}
	if action, prop := fsOnState(t, reqs[1]); action != netWMStateAdd || prop != tAtomFull {
		t.Errorf("второй запрос: действие %d, свойство %d", action, prop)
	}
}

// Менеджер без подсказки: окно переносится на угол монитора, затем ADD.
func TestX11_FullscreenOn_MovesWithoutHint(t *testing.T) {
	w, log := fsOnWindow(t, false)
	if !w.SetFullscreenOn(native.Monitor{Bounds: image.Rect(1920, 0, 3840, 1080)}) { // по границам
		t.Fatal("SetFullscreenOn вернул false")
	}
	reqs := log.wait(t, 2)
	if x, y := fsOnConfigure(t, reqs[0]); x != 1920 || y != 0 {
		t.Errorf("окно перенесено в (%d,%d), ждал угол монитора (1920,0)", x, y)
	}
	if action, prop := fsOnState(t, reqs[1]); action != netWMStateAdd || prop != tAtomFull {
		t.Errorf("второй запрос: действие %d, свойство %d", action, prop)
	}
}

// Уже в полном экране. С подсказкой — только сообщение о мониторе; без неё —
// REMOVE, перенос и новый ADD.
func TestX11_FullscreenOn_WhileFullscreen(t *testing.T) {
	w, log := fsOnWindow(t, true)
	w.fs.on.Store(true)
	if !w.SetFullscreenOn(native.MonitorWithKey(native.Monitor{}, 1)) {
		t.Fatal("SetFullscreenOn вернул false")
	}
	reqs := log.wait(t, 1)
	if got := binary.LittleEndian.Uint32(reqs[0].raw[12+8 : 12+12]); got != tAtomFsMon {
		t.Errorf("единственный запрос — тип %d, ждал _NET_WM_FULLSCREEN_MONITORS", got)
	}

	w, log = fsOnWindow(t, false)
	w.fs.on.Store(true)
	if !w.SetFullscreenOn(native.Monitor{Name: "DP-1"}) {
		t.Fatal("SetFullscreenOn вернул false")
	}
	reqs = log.wait(t, 3)
	if action, _ := fsOnState(t, reqs[0]); action != netWMStateRemove {
		t.Errorf("первым ждал REMOVE, действие %d", action)
	}
	if x, _ := fsOnConfigure(t, reqs[1]); x != 1920 {
		t.Errorf("перенос в x=%d, ждал 1920", x)
	}
	if action, _ := fsOnState(t, reqs[2]); action != netWMStateAdd {
		t.Errorf("последним ждал ADD, действие %d", action)
	}
}

// Отключённый монитор, нулевой Monitor и менеджер без полного экрана: false и
// ни одного байта серверу.
func TestX11_FullscreenOn_Refusals(t *testing.T) {
	w, log := fsOnWindow(t, true)
	if w.SetFullscreenOn(native.MonitorWithKey(native.Monitor{Name: "VGA-1", Bounds: image.Rect(5000, 0, 6000, 800)}, 9)) {
		t.Error("отключённый монитор принят")
	}
	if w.SetFullscreenOn(native.Monitor{}) {
		t.Error("нулевой Monitor принят")
	}
	w.fs.unsupported = true
	if w.SetFullscreenOn(native.MonitorWithKey(native.Monitor{}, 1)) {
		t.Error("менеджер без полного экрана, а SetFullscreenOn вернул true")
	}
	log.wait(t, 0)
}

// Раскладка ClientMessage _NET_WM_FULLSCREEN_MONITORS.
func TestX11FullscreenMonitorsMessage(t *testing.T) {
	b := x11FullscreenMonitorsMessage(tRoot, tWid, tAtomFsMon, 3)
	if len(b) != 44 || b[0] != 25 || binary.LittleEndian.Uint16(b[2:4]) != 11 {
		t.Fatalf("не SendEvent длиной 11 слов: %v", b[:4])
	}
	if mask := binary.LittleEndian.Uint32(b[8:12]); mask != 0x00180000 {
		t.Errorf("маска %#x", mask)
	}
	ev := b[12:]
	if ev[0] != 33 || ev[1] != 32 {
		t.Errorf("не ClientMessage формата 32: %v", ev[:2])
	}
	for i := 0; i < 4; i++ {
		if binary.LittleEndian.Uint32(ev[12+i*4:16+i*4]) != 3 {
			t.Errorf("край %d", i)
		}
	}
}

// Объявленный менеджером _NET_WM_FULLSCREEN_MONITORS попадает в окно при
// проверке _NET_SUPPORTED.
func TestX11_DetectFullscreenMonitorsSupport(t *testing.T) {
	for _, tc := range []struct {
		name      string
		supported []uint32
		want      bool
	}{
		{"объявлен", []uint32{tAtomState, tAtomFull, tAtomFsMon}, true},
		{"не объявлен", []uint32{tAtomState, tAtomFull}, false},
	} {
		w, server := newFsX11(t)
		w.atomFsMonitors = tAtomFsMon
		go serveFakeX(server, func(r x11Request) []byte {
			if r.major == 20 {
				return x11PropReply(tc.supported...)
			}
			return nil
		})
		w.x11DetectFullscreenSupport()
		if w.fs.monitorsSupported != tc.want {
			t.Errorf("%s: monitorsSupported=%v, ждал %v", tc.name, w.fs.monitorsSupported, tc.want)
		}
	}
}

// Ключи мониторов RandR: номер в ответе RRGetMonitors с единицы (он же индекс
// для _NET_WM_FULLSCREEN_MONITORS), а при сортировке списка ключ едет с монитором.
func TestX11Monitors_Keys(t *testing.T) {
	got := fsX11Monitors(t, fakeRandr(true, 1, 5,
		rrMonitorRecord(500, false, 1920, 0, 1280, 1024, 0),
		rrMonitorRecord(501, true, 0, 0, 1920, 1080, 0),
	))
	if len(got) != 2 || native.MonitorKey(got[0]) != 1 || native.MonitorKey(got[1]) != 2 {
		t.Fatalf("ключи %+v, ждал 1 и 2 в порядке ответа сервера", got)
	}
	sorted := native.SortMonitors(got)
	if !sorted[0].Primary || native.MonitorKey(sorted[0]) != 2 {
		t.Errorf("основной монитор после сортировки потерял ключ: %+v", sorted[0])
	}
}
