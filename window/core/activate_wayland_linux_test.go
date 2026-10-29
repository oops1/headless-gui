//go:build linux && !android

package core

import (
	"encoding/binary"
	"testing"
	"time"
)

// Вывод окна на передний план на Wayland (xdg-activation-v1), на проводе:
// компоновщик подменён парой сокетов, события токена подаются прямо в
// handleEvent.

const (
	testActMgrName = 61 // имя глобала xdg_activation_v1 в реестре
	testActRegID   = 2

	// Опкоды из xdg-activation-v1.xml — буквально, независимо от констант в
	// коде: опечатка в них иначе прошла бы тест.
	tActGetToken   = 1 // xdg_activation_v1.get_activation_token
	tActActivate   = 2 // xdg_activation_v1.activate
	tTokSetSerial  = 0 // xdg_activation_token_v1.set_serial
	tTokSetAppID   = 1 // .set_app_id
	tTokSetSurface = 2 // .set_surface
	tTokCommit     = 3 // .commit
	tTokDestroy    = 4 // .destroy
	tTokEvDone     = 0 // .done
)

// wlRawMsg — сообщение из провода с телом в байтах (строки разбираются wlString).
type wlRawMsg struct {
	obj    uint32
	opcode uint16
	body   []byte
}

func rawWlMsgs(t *testing.T, b []byte) []wlRawMsg {
	t.Helper()
	var out []wlRawMsg
	for len(b) >= 8 {
		size := int(binary.LittleEndian.Uint16(b[6:8]))
		if size < 8 || size > len(b) {
			t.Fatalf("битая длина сообщения: %d при %d байтах", size, len(b))
		}
		out = append(out, wlRawMsg{
			obj:    binary.LittleEndian.Uint32(b[0:4]),
			opcode: binary.LittleEndian.Uint16(b[4:6]),
			body:   append([]byte(nil), b[8:size]...),
		})
		b = b[size:]
	}
	return out
}

func findRaw(msgs []wlRawMsg, obj uint32, opcode uint16) (wlRawMsg, bool) {
	for _, m := range msgs {
		if m.obj == obj && m.opcode == opcode {
			return m, true
		}
	}
	return wlRawMsg{}, false
}

func newWlActWindow(t *testing.T) *wlTestConn {
	c := newWlTestWindow(t)
	c.w.registryID = testActRegID
	c.w.gActivation = testActMgrName
	return c
}

// readAllRaw читает всё, что окно отправило, пока на сокете не наступит тишина.
func readAllRaw(t *testing.T, c *wlTestConn) []byte {
	t.Helper()
	var raw []byte
	wait := time.Second
	for {
		c.peer.SetReadDeadline(time.Now().Add(wait))
		buf := make([]byte, 4096)
		n, err := c.peer.Read(buf)
		if err != nil {
			if len(raw) == 0 {
				t.Fatalf("композитор ничего не получил: %v", err)
			}
			return raw
		}
		raw = append(raw, buf[:n]...)
		wait = 60 * time.Millisecond
	}
}

// doneEvent — тело xdg_activation_token_v1.done(token).
func doneEvent(token string) []byte {
	return newWlMsg(0, 0).putString(token).buf[8:]
}

// Без глобала поднять окно нечем: false и ни байта в сокет.
func TestWayland_Activate_NoGlobal(t *testing.T) {
	c := newWlTestWindow(t)
	if c.w.Activate() || c.w.ActivateWithToken("tok") {
		t.Fatal("без xdg_activation_v1 активация ответила true")
	}
	c.peer.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
	if n, _ := c.peer.Read(make([]byte, 64)); n > 0 {
		t.Errorf("без глобала отправлено %d байт", n)
	}
}

// Токен от запустившего: bind менеджера и activate(token, surface).
func TestWayland_ActivateWithToken_Wire(t *testing.T) {
	c := newWlActWindow(t)
	if !c.w.ActivateWithToken("launcher-token-42") {
		t.Fatal("ActivateWithToken вернул false")
	}
	raw := readAllRaw(t, c)
	msgs := rawWlMsgs(t, raw)

	bind, ok := findRaw(msgs, testActRegID, wlRegistryBind)
	if !ok {
		t.Fatalf("bind не отправлен: %+v", msgs)
	}
	if name := binary.LittleEndian.Uint32(bind.body[0:4]); name != testActMgrName {
		t.Errorf("bind глобала %d, ждал %d", name, testActMgrName)
	}
	iface, off := wlString(bind.body, 4)
	if iface != "xdg_activation_v1" {
		t.Errorf("интерфейс в bind: %q", iface)
	}
	if ver := binary.LittleEndian.Uint32(bind.body[off : off+4]); ver != 1 {
		t.Errorf("версия привязки %d, ждал 1", ver)
	}
	mgr := binary.LittleEndian.Uint32(bind.body[off+4 : off+8])

	act, ok := findRaw(msgs, mgr, tActActivate)
	if !ok {
		t.Fatalf("activate не отправлен: %+v", msgs)
	}
	token, off := wlString(act.body, 0)
	if token != "launcher-token-42" {
		t.Errorf("токен %q", token)
	}
	if surf := binary.LittleEndian.Uint32(act.body[off : off+4]); surf != c.w.surfaceID {
		t.Errorf("поверхность %d, ждал %d (wl_surface окна)", surf, c.w.surfaceID)
	}
	if xdgActivationActivate != tActActivate || xdgActivationGetToken != tActGetToken {
		t.Errorf("опкоды менеджера %d/%d, по протоколу %d/%d",
			xdgActivationGetToken, xdgActivationActivate, tActGetToken, tActActivate)
	}

	// Второй вызов менеджер не привязывает заново.
	c.w.ActivateWithToken("again")
	msgs = rawWlMsgs(t, readAllRaw(t, c))
	if _, again := findRaw(msgs, testActRegID, wlRegistryBind); again {
		t.Error("менеджер привязан повторно")
	}
	if _, ok := findRaw(msgs, mgr, tActActivate); !ok {
		t.Error("второй activate не отправлен")
	}
}

// Самоактивация: get_activation_token, serial последнего ввода и seat,
// поверхность, app_id, commit; по done(token) — activate и destroy токена.
func TestWayland_Activate_SelfToken(t *testing.T) {
	c := newWlActWindow(t)
	c.w.inputSerial.Store(4711)
	c.w.appID = "pixgo"
	if !c.w.Activate() {
		t.Fatal("Activate вернул false")
	}
	msgs := rawWlMsgs(t, readAllRaw(t, c))

	bind, ok := findRaw(msgs, testActRegID, wlRegistryBind)
	if !ok {
		t.Fatalf("bind не отправлен: %+v", msgs)
	}
	_, off := wlString(bind.body, 4)
	mgr := binary.LittleEndian.Uint32(bind.body[off+4 : off+8])

	get, ok := findRaw(msgs, mgr, tActGetToken)
	if !ok {
		t.Fatalf("get_activation_token не отправлен: %+v", msgs)
	}
	tok := binary.LittleEndian.Uint32(get.body[0:4])

	ser, ok := findRaw(msgs, tok, tTokSetSerial)
	if !ok || binary.LittleEndian.Uint32(ser.body[0:4]) != 4711 ||
		binary.LittleEndian.Uint32(ser.body[4:8]) != c.w.seatID {
		t.Errorf("set_serial: %+v (ждал serial 4711 и seat %d)", ser, c.w.seatID)
	}
	if app, ok := findRaw(msgs, tok, tTokSetAppID); !ok {
		t.Error("set_app_id не отправлен")
	} else if s, _ := wlString(app.body, 0); s != "pixgo" {
		t.Errorf("app_id %q", s)
	}
	if sf, ok := findRaw(msgs, tok, tTokSetSurface); !ok || binary.LittleEndian.Uint32(sf.body[0:4]) != c.w.surfaceID {
		t.Errorf("set_surface: %+v", sf)
	}
	// commit — последним из запросов токена.
	var last wlRawMsg
	for _, m := range msgs {
		if m.obj == tok {
			last = m
		}
	}
	if last.opcode != tTokCommit {
		t.Errorf("последний запрос токена — опкод %d, ждал commit (3)", last.opcode)
	}
	if _, destroyed := findRaw(msgs, tok, tTokDestroy); destroyed {
		t.Error("токен уничтожен до ответа компоновщика")
	}
	if _, early := findRaw(msgs, mgr, tActActivate); early {
		t.Error("activate отправлен до получения токена")
	}

	// Компоновщик выдал токен.
	c.w.handleEvent(tok, tTokEvDone, doneEvent("compositor-token"))
	msgs = rawWlMsgs(t, readAllRaw(t, c))
	act, ok := findRaw(msgs, mgr, tActActivate)
	if !ok {
		t.Fatalf("после done activate не отправлен: %+v", msgs)
	}
	if s, off := wlString(act.body, 0); s != "compositor-token" ||
		binary.LittleEndian.Uint32(act.body[off:off+4]) != c.w.surfaceID {
		t.Errorf("activate: токен %q", s)
	}
	if _, ok := findRaw(msgs, tok, tTokDestroy); !ok {
		t.Error("токен не уничтожен после done")
	}

	// Повторное done на уничтоженный токен ничего не делает.
	if c.w.activationEvent(tok, tTokEvDone, doneEvent("late")) {
		t.Error("событие чужого/закрытого токена принято")
	}
}

// Без ввода у окна токен всё равно просится, но без set_serial: решать
// компоновщику.
func TestWayland_Activate_NoSerial(t *testing.T) {
	c := newWlActWindow(t)
	if !c.w.Activate() {
		t.Fatal("Activate вернул false")
	}
	msgs := rawWlMsgs(t, readAllRaw(t, c))
	var tok uint32
	for _, m := range msgs {
		if m.opcode == tTokCommit && m.obj != 0 {
			tok = m.obj
		}
	}
	if tok == 0 {
		t.Fatalf("commit не отправлен: %+v", msgs)
	}
	if _, has := findRaw(msgs, tok, tTokSetSerial); has {
		t.Error("set_serial отправлен без ввода")
	}
	if _, has := findRaw(msgs, tok, tTokSetAppID); has {
		t.Error("set_app_id отправлен при пустом app_id")
	}
}

// Пустой токен — самоактивация; свёрнутость на Wayland неизвестна.
func TestWayland_ActivateWithToken_EmptyIsSelf(t *testing.T) {
	c := newWlActWindow(t)
	c.w.ActivateWithToken("")
	msgs := rawWlMsgs(t, readAllRaw(t, c))
	found := false
	for _, m := range msgs {
		if m.opcode == tActGetToken {
			found = true
		}
	}
	if !found {
		t.Errorf("пустой токен не запросил свой: %+v", msgs)
	}
	if c.w.IsMinimized() {
		t.Error("IsMinimized на Wayland должен быть false")
	}
	c.w.SetOnMinimizedChanged(func(bool) { t.Error("Wayland не шлёт событий свёрнутости") })
}

// Глобал объявляется реестром: handleEvent запоминает его имя.
func TestWayland_Registry_AdvertisesActivation(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.registryID = testActRegID
	body := binary.LittleEndian.AppendUint32(nil, testActMgrName)
	body = append(body, newWlMsg(0, 0).putString("xdg_activation_v1").buf[8:]...)
	body = binary.LittleEndian.AppendUint32(body, 1)
	c.w.handleEvent(testActRegID, wlRegistryEvGlobal, body)
	if c.w.gActivation != testActMgrName {
		t.Errorf("глобал не запомнен: %d", c.w.gActivation)
	}
}
