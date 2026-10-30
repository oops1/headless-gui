//go:build linux && !android

package x11

import (
	"encoding/binary"
	"sync"
	"testing"
)

// Захват клавиатуры X11 на проводе: окно разговаривает с настоящим unix-сокетом,
// на другом конце которого поддельный X-сервер (x11Pair и serveFakeX из
// x11_fullscreen_linux_test.go) отвечает на GrabKeyboard заданным status'ом.

// grabServer — поддельный сервер: запоминает опкоды запросов и отвечает на
// GrabKeyboard статусом status.
type grabServer struct {
	mu     sync.Mutex
	ops    []byte
	raw    map[byte][]byte
	status byte
}

func newGrabX11(t *testing.T, status byte) (*X11Window, *grabServer) {
	t.Helper()
	client, server := x11Pair(t)
	w := &X11Window{conn: client, rootWin: tRoot, wid: tWid, atomTargets: 555}
	gs := &grabServer{raw: map[byte][]byte{}, status: status}
	go serveFakeX(server, func(r x11Request) []byte {
		gs.mu.Lock()
		defer gs.mu.Unlock()
		gs.ops = append(gs.ops, r.major)
		gs.raw[r.major] = r.raw
		if r.major == x11OpGrabKeyboard {
			return x11Reply(gs.status, nil, nil)
		}
		return nil
	})
	return w, gs
}

func (g *grabServer) opsSeen() []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]byte(nil), g.ops...)
}

func (g *grabServer) lastRaw(op byte) []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.raw[op]
}

// x11FocusEvent — FocusIn (9) / FocusOut (10): detail @1, окно @4, mode @8.
func x11FocusEvent(in bool, mode byte) []byte {
	b := make([]byte, 32)
	b[0] = 10
	if in {
		b[0] = 9
	}
	b[1] = 3 // NotifyNonlinear
	binary.LittleEndian.PutUint32(b[4:8], tWid)
	b[8] = mode
	return b
}

// Раскладка запросов: GrabKeyboard — опкод 31 длиной 4 слова (окно @4, время
// CurrentTime @8, оба режима асинхронные), UngrabKeyboard — 32 длиной 2.
func TestX11KeyGrab_RequestLayout(t *testing.T) {
	g := x11GrabKeyboardRequest(tWid)
	if len(g) != 16 || g[0] != 31 || binary.LittleEndian.Uint16(g[2:4]) != 4 {
		t.Fatalf("GrabKeyboard: %v", g)
	}
	if g[1] != 1 {
		t.Errorf("owner_events %d, ждал 1", g[1])
	}
	if binary.LittleEndian.Uint32(g[4:8]) != tWid || binary.LittleEndian.Uint32(g[8:12]) != 0 {
		t.Errorf("окно/время: %v", g[4:12])
	}
	if g[12] != 1 || g[13] != 1 {
		t.Errorf("режимы %d/%d, ждал Asynchronous (1)", g[12], g[13])
	}
	u := x11UngrabKeyboardRequest()
	if len(u) != 8 || u[0] != 32 || binary.LittleEndian.Uint16(u[2:4]) != 2 {
		t.Errorf("UngrabKeyboard: %v", u)
	}
	// Разбудить цикл можно только сообщением самому себе: SendEvent, маска 0.
	wk := x11SelfWakeRequest(tWid, 555)
	if len(wk) != 44 || wk[0] != 25 || binary.LittleEndian.Uint32(wk[4:8]) != tWid ||
		binary.LittleEndian.Uint32(wk[8:12]) != 0 || wk[12] != 33 {
		t.Errorf("SendEvent себе: %v", wk)
	}
}

// Захват берётся в цикле событий, а не в вызове приложения: SetKeyboardGrab
// лишь помечает состояние.
func TestX11KeyGrab_GrabbedInLoop(t *testing.T) {
	w, srv := newGrabX11(t, x11GrabSuccess)
	if !w.SetKeyboardGrab(true) {
		t.Fatal("SetKeyboardGrab(true) вернул false")
	}
	if got := srv.opsSeen(); len(got) != 0 {
		t.Fatalf("запрос ушёл до цикла событий: %v", got)
	}
	if w.KeyboardGrabActive() {
		t.Error("KeyboardGrabActive true до ответа сервера")
	}
	w.x11SyncKeyGrab()
	if got := srv.opsSeen(); len(got) != 1 || got[0] != 31 {
		t.Fatalf("после цикла запросы %v, ждал [GrabKeyboard]", got)
	}
	if !w.KeyboardGrabActive() {
		t.Error("сервер согласился, а KeyboardGrabActive false")
	}
	// Ничего не менялось — ничего и не шлём.
	w.x11SyncKeyGrab()
	if len(srv.opsSeen()) != 1 {
		t.Errorf("лишние запросы: %v", srv.opsSeen())
	}
}

// Отказ сервера (AlreadyGrabbed): захвата нет, и повтор — со следующим
// событием фокуса, а не на каждом событии.
func TestX11KeyGrab_RefusedRetriesOnFocus(t *testing.T) {
	w, srv := newGrabX11(t, 1) // AlreadyGrabbed
	w.SetKeyboardGrab(true)
	w.x11SyncKeyGrab()
	if w.KeyboardGrabActive() {
		t.Error("сервер отказал, а KeyboardGrabActive true")
	}
	w.x11SyncKeyGrab()
	w.x11SyncKeyGrab()
	if n := len(srv.opsSeen()); n != 1 {
		t.Errorf("после отказа запросов %d, ждал один (без повторов на каждое событие)", n)
	}

	srv.mu.Lock()
	srv.status = x11GrabSuccess
	srv.mu.Unlock()
	w.handleX11Event(x11FocusEvent(true, x11FocusModeNormal)) // фокус получен — пробуем снова
	w.x11SyncKeyGrab()
	if !w.KeyboardGrabActive() {
		t.Error("после получения фокуса захват не взят")
	}
}

// Захват держится, пока у окна фокус: потеряли — отпускаем, вернулся — берём.
func TestX11KeyGrab_FollowsFocus(t *testing.T) {
	w, srv := newGrabX11(t, x11GrabSuccess)
	w.SetKeyboardGrab(true)
	w.handleX11Event(x11FocusEvent(true, x11FocusModeNormal))
	w.x11SyncKeyGrab()
	if !w.KeyboardGrabActive() {
		t.Fatal("с фокусом захват не взят")
	}

	w.handleX11Event(x11FocusEvent(false, x11FocusModeNormal))
	w.x11SyncKeyGrab()
	if w.KeyboardGrabActive() {
		t.Error("фокус потерян, а захват держится")
	}
	x11GrabWaitOps(t, srv, 2)
	if ops := srv.opsSeen(); len(ops) != 2 || ops[1] != 32 {
		t.Fatalf("запросы %v, ждал [Grab Ungrab]", ops)
	}

	w.handleX11Event(x11FocusEvent(true, x11FocusModeNormal))
	w.x11SyncKeyGrab()
	if !w.KeyboardGrabActive() {
		t.Error("фокус вернулся, а захват не взят")
	}
	if ops := srv.opsSeen(); len(ops) != 3 || ops[2] != 31 {
		t.Errorf("запросы %v, ждал [Grab Ungrab Grab]", ops)
	}
}

// Пока окно без фокуса, захват не берётся даже по просьбе.
func TestX11KeyGrab_NoFocusNoGrab(t *testing.T) {
	w, srv := newGrabX11(t, x11GrabSuccess)
	w.handleX11Event(x11FocusEvent(false, x11FocusModeNormal)) // фокус известен: его нет
	w.SetKeyboardGrab(true)
	w.x11SyncKeyGrab()
	if got := srv.opsSeen(); len(got) != 0 {
		t.Errorf("окно без фокуса отправило %v", got)
	}
}

// Выключение отпускает клавиатуру; повторное выключение — ничего не шлёт.
func TestX11KeyGrab_Off(t *testing.T) {
	w, srv := newGrabX11(t, x11GrabSuccess)
	w.SetKeyboardGrab(true)
	w.x11SyncKeyGrab()
	w.SetKeyboardGrab(false)
	w.x11SyncKeyGrab()
	if w.KeyboardGrabActive() {
		t.Error("захват выключен, а KeyboardGrabActive true")
	}
	w.SetKeyboardGrab(false)
	w.x11SyncKeyGrab()
	x11GrabWaitOps(t, srv, 2)
	if ops := srv.opsSeen(); len(ops) != 2 || ops[1] != 32 {
		t.Errorf("запросы %v, ждал [Grab Ungrab]", ops)
	}
}

// Событие фокуса, порождённое нашим же захватом (режим Grab/Ungrab), — не
// потеря активности: приложению о ней не сообщается и сам захват не
// переключается.
func TestX11KeyGrab_OwnGrabFocusEventsIgnored(t *testing.T) {
	w, _ := newGrabX11(t, x11GrabSuccess)
	var acts []bool
	w.onActivate = func(a bool) { acts = append(acts, a) }

	w.handleX11Event(x11FocusEvent(true, x11FocusModeNormal))
	w.SetKeyboardGrab(true)
	w.x11SyncKeyGrab()
	w.handleX11Event(x11FocusEvent(false, x11FocusModeGrab))
	w.handleX11Event(x11FocusEvent(true, x11FocusModeGrab))
	w.handleX11Event(x11FocusEvent(false, x11FocusModeUngrab))
	if len(acts) != 1 || !acts[0] {
		t.Errorf("активность %v, ждал только настоящий [true]", acts)
	}
	if !w.kg.focused.Load() {
		t.Error("событие режима Grab сбросило фокус")
	}

	// Без нашего захвата привычное поведение не меняется: любое FocusOut — потеря.
	w2, _ := newGrabX11(t, x11GrabSuccess)
	var acts2 []bool
	w2.onActivate = func(a bool) { acts2 = append(acts2, a) }
	w2.handleX11Event(x11FocusEvent(false, x11FocusModeGrab))
	if len(acts2) != 1 || acts2[0] {
		t.Errorf("без захвата FocusOut(Grab) → %v, ждал [false] как раньше", acts2)
	}
}

// Пока окно без фокус-событий (нет оконного менеджера), захват берётся сразу.
func TestX11KeyGrab_NoWindowManager(t *testing.T) {
	w, srv := newGrabX11(t, x11GrabSuccess)
	w.SetKeyboardGrab(true)
	w.x11SyncKeyGrab()
	if got := srv.opsSeen(); len(got) != 1 || got[0] != 31 {
		t.Errorf("без WM запросы %v, ждал [GrabKeyboard]", got)
	}
}

// Работающий цикл будится сообщением самому себе, а не запросом с ответом.
func TestX11KeyGrab_WakesLoop(t *testing.T) {
	w, srv := newGrabX11(t, x11GrabSuccess)
	w.kg.looping.Store(true)
	w.SetKeyboardGrab(true)
	x11GrabWaitOps(t, srv, 1)
	if got := srv.opsSeen(); got[0] != 25 {
		t.Errorf("пробуждение %v, ждал SendEvent (25)", got)
	}
}

func x11GrabWaitOps(t *testing.T, srv *grabServer, n int) {
	t.Helper()
	if !fsWaitFor(func() bool { return len(srv.opsSeen()) >= n }) {
		t.Fatalf("сервер получил %v, ждал %d запрос(ов)", srv.opsSeen(), n)
	}
}

// Окно без соединения или закрытое захват не принимает; выключить можно всегда.
func TestX11KeyGrab_NotCreated(t *testing.T) {
	w := &X11Window{}
	if w.SetKeyboardGrab(true) {
		t.Error("окно не создано, а SetKeyboardGrab вернул true")
	}
	if !w.SetKeyboardGrab(false) {
		t.Error("выключение захвата должно быть успешным")
	}
}
