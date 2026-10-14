//go:build linux && !android

package window

import (
	"encoding/binary"
	"strings"
	"sync"
	"testing"
	"time"
)

// Вывод окна на передний план и свёрнутость X11 на проводе: окно разговаривает
// с поддельным сервером (см. x11_fullscreen_linux_test.go) и проверяются байты
// запросов.

const (
	tAtomActive  = 310 // _NET_ACTIVE_WINDOW
	tAtomHidden  = 311 // _NET_WM_STATE_HIDDEN
	tAtomWMState = 312 // WM_STATE
	tAtomStBegin = 313 // _NET_STARTUP_INFO_BEGIN
	tAtomStInfo  = 314 // _NET_STARTUP_INFO
)

func newActX11(t *testing.T) (*X11Window, *actRecorder) {
	t.Helper()
	w, server := newFsX11(t)
	w.atomNetActiveWindow = tAtomActive
	w.atomWMStateHidden = tAtomHidden
	w.atomWMStateProp = tAtomWMState
	w.atomStartupBegin = tAtomStBegin
	w.atomStartupInfo = tAtomStInfo
	rec := &actRecorder{}
	go serveFakeX(server, rec.handle)
	return w, rec
}

// actRecorder записывает запросы клиента и отвечает на GetProperty заданным
// списком.
type actRecorder struct {
	mu    sync.Mutex
	reqs  []x11Request
	state []uint32 // ответ на GetProperty _NET_WM_STATE
	wm    []uint32 // ответ на GetProperty WM_STATE
}

func (r *actRecorder) handle(q x11Request) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, q)
	if q.major == 20 { // GetProperty: свойство в байтах 8..12
		if binary.LittleEndian.Uint32(q.raw[8:12]) == tAtomWMState {
			return x11PropReply(r.wm...)
		}
		return x11PropReply(r.state...)
	}
	return nil
}

func (r *actRecorder) setState(atoms ...uint32) { r.mu.Lock(); r.state = atoms; r.mu.Unlock() }
func (r *actRecorder) setWM(vals ...uint32)     { r.mu.Lock(); r.wm = vals; r.mu.Unlock() }

// wait ждёт, пока клиент отправит n запросов, и возвращает копию списка.
func (r *actRecorder) wait(t *testing.T, n int) []x11Request {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		if len(r.reqs) >= n {
			out := append([]x11Request(nil), r.reqs...)
			r.mu.Unlock()
			return out
		}
		r.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("клиент отправил %d запросов, ждал %d: %v", len(r.reqs), n, r.reqs)
	return nil
}

// quiet проверяет, что за короткое время клиент не прислал новых запросов.
func (r *actRecorder) quiet(t *testing.T, have int) {
	t.Helper()
	time.Sleep(60 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) != have {
		t.Errorf("лишние запросы: %v", r.reqs[have:])
	}
}

// activeMsg разбирает _NET_ACTIVE_WINDOW из запроса SendEvent.
func activeMsg(t *testing.T, q x11Request) (wid, atom, source, ts, cur uint32) {
	t.Helper()
	if q.major != 25 || len(q.raw) != 44 {
		t.Fatalf("не SendEvent: %v", q)
	}
	le := binary.LittleEndian
	if le.Uint32(q.raw[4:8]) != tRoot {
		t.Errorf("адресат %d, ждал корневое окно", le.Uint32(q.raw[4:8]))
	}
	if le.Uint32(q.raw[8:12]) != 0x00180000 {
		t.Errorf("маска события %#x", le.Uint32(q.raw[8:12]))
	}
	m := q.raw[12:]
	if m[0] != 33 || m[1] != 32 {
		t.Fatalf("не ClientMessage формата 32: %v", m[:2])
	}
	return le.Uint32(m[4:8]), le.Uint32(m[8:12]), le.Uint32(m[12:16]), le.Uint32(m[16:20]), le.Uint32(m[20:24])
}

// Раскладка _NET_ACTIVE_WINDOW: окно, тип, источник «приложение», метка.
func TestX11ActiveWindowMessage_Layout(t *testing.T) {
	req := x11ActiveWindowMessage(1, 77, 310, 1, 123456, 9)
	le := binary.LittleEndian
	if len(req) != 44 || req[0] != 25 || le.Uint16(req[2:4]) != 11 {
		t.Fatalf("заголовок SendEvent: %v", req[:8])
	}
	if le.Uint32(req[4:8]) != 1 || le.Uint32(req[8:12]) != 0x00180000 {
		t.Errorf("адресат/маска: %d/%#x", le.Uint32(req[4:8]), le.Uint32(req[8:12]))
	}
	m := req[12:]
	if m[0] != 33 || m[1] != 32 ||
		le.Uint32(m[4:8]) != 77 || le.Uint32(m[8:12]) != 310 ||
		le.Uint32(m[12:16]) != 1 || le.Uint32(m[16:20]) != 123456 || le.Uint32(m[20:24]) != 9 {
		t.Errorf("ClientMessage: %v", m)
	}
}

// Время последнего ввода запоминается из событий клавиш и кнопок и только из них.
func TestX11_NoteUserTime(t *testing.T) {
	var a x11Activation
	ev := func(typ byte, tm uint32) []byte {
		b := make([]byte, 32)
		b[0] = typ
		binary.LittleEndian.PutUint32(b[4:8], tm)
		return b
	}
	a.noteUserTime(6, ev(6, 500)) // MotionNotify — не ввод для метки
	if a.userTime.Load() != 0 {
		t.Fatal("движение мыши записано как ввод")
	}
	for i, typ := range []byte{x11EvKeyPress, x11EvKeyRelease, x11EvButtonPress, x11EvButtonRelease} {
		a.noteUserTime(typ, ev(typ, uint32(1000+i)))
		if got := a.userTime.Load(); got != uint32(1000+i) {
			t.Errorf("событие %d: время %d, ждал %d", typ, got, 1000+i)
		}
	}
	a.noteUserTime(x11EvKeyPress, ev(x11EvKeyPress, 0)) // 0 — CurrentTime, меткой не служит
	if a.userTime.Load() != 1003 {
		t.Errorf("нулевое время затёрло метку: %d", a.userTime.Load())
	}
}

// Время ввода доходит до цикла событий: KeyPress, пришедший в handleX11Event,
// становится меткой для _NET_ACTIVE_WINDOW у SetForeground (раньше всегда 0).
func TestX11_SetForeground_UsesInputTime(t *testing.T) {
	w, rec := newActX11(t)
	key := make([]byte, 32)
	key[0] = x11EvKeyPress
	key[1] = 38
	binary.LittleEndian.PutUint32(key[4:8], 424242)
	w.handleX11Event(key)

	w.SetForeground()
	reqs := rec.wait(t, 3) // raise, _NET_ACTIVE_WINDOW, SetInputFocus
	var found bool
	for _, q := range reqs {
		if q.major != 25 {
			continue
		}
		wid, atom, src, ts, _ := activeMsg(t, q)
		found = true
		if wid != tWid || atom != tAtomActive || src != 1 {
			t.Errorf("окно/тип/источник: %d/%d/%d", wid, atom, src)
		}
		if ts != 424242 {
			t.Errorf("метка времени %d, ждал 424242 (последний ввод)", ts)
		}
	}
	if !found {
		t.Fatal("_NET_ACTIVE_WINDOW не отправлен")
	}
}

// Activate без ввода: метка 0 (CurrentTime), набор запросов — raise, активация,
// фокус; окно не свёрнуто — MapWindow не нужен.
func TestX11_Activate_Wire(t *testing.T) {
	w, rec := newActX11(t)
	if !w.Activate() {
		t.Fatal("Activate вернул false")
	}
	reqs := rec.wait(t, 3)
	if _, atom, src, ts, _ := activeMsg(t, reqs[0]); atom != tAtomActive || src != 1 || ts != 0 {
		t.Errorf("атом/источник/метка: %d/%d/%d", atom, src, ts)
	}
	if reqs[1].major != 12 { // ConfigureWindow stack-mode=Above
		t.Errorf("вторым ждал ConfigureWindow, пришло %d", reqs[1].major)
	}
	if reqs[2].major != 42 { // SetInputFocus
		t.Errorf("третьим ждал SetInputFocus (42), пришло %d", reqs[2].major)
	}
	rec.quiet(t, 3)
}

func TestX11_Activate_Unavailable(t *testing.T) {
	w, _ := newActX11(t)
	w.atomNetActiveWindow = 0
	if w.Activate() || w.ActivateWithToken("x_TIME5") {
		t.Error("без атома _NET_ACTIVE_WINDOW активация ответила true")
	}
	w, _ = newActX11(t)
	w.wid = 0
	if w.Activate() {
		t.Error("окно не создано, а Activate вернул true")
	}
}

// ActivateWithToken: метка берётся из «_TIME<число>» стартового идентификатора,
// а не из ввода окна; запускающему уходит «remove: ID=...».
func TestX11_ActivateWithToken_TimeAndStartupRemove(t *testing.T) {
	w, rec := newActX11(t)
	w.act.userTime.Store(111)
	id := "pixgo-1234-host_TIME987654"
	if !w.ActivateWithToken(id) {
		t.Fatal("ActivateWithToken вернул false")
	}
	// raise + active + focus + сообщение startup-info в одном куске
	// («remove: ID=...» длиннее 20 байт — их несколько).
	want := len(x11StartupInfoRequests(tRoot, tWid, tAtomStBegin, tAtomStInfo, "remove: ID="+id))
	reqs := rec.wait(t, 3+want)

	var active, info []x11Request
	for _, q := range reqs {
		if q.major != 25 {
			continue
		}
		if binary.LittleEndian.Uint32(q.raw[8:12]) == 0x00180000 {
			active = append(active, q)
		} else {
			info = append(info, q)
		}
	}
	if len(active) != 1 {
		t.Fatalf("_NET_ACTIVE_WINDOW отправлен %d раз", len(active))
	}
	if _, _, src, ts, _ := activeMsg(t, active[0]); src != 1 || ts != 987654 {
		t.Errorf("источник %d, метка %d; ждал 1 и 987654 из токена", src, ts)
	}

	// Куски startup-info собираются обратно в исходный текст.
	if len(info) != want {
		t.Fatalf("кусков startup-info %d, ждал %d", len(info), want)
	}
	var text []byte
	for i, q := range info {
		le := binary.LittleEndian
		if le.Uint32(q.raw[8:12]) != 0x00400000 {
			t.Errorf("кусок %d: маска %#x, ждал PropertyChange", i, le.Uint32(q.raw[8:12]))
		}
		m := q.raw[12:]
		wantType := uint32(tAtomStInfo)
		if i == 0 {
			wantType = tAtomStBegin
		}
		if m[1] != 8 || le.Uint32(m[4:8]) != tWid || le.Uint32(m[8:12]) != wantType {
			t.Errorf("кусок %d: формат %d, окно %d, тип %d", i, m[1], le.Uint32(m[4:8]), le.Uint32(m[8:12]))
		}
		text = append(text, m[12:32]...)
	}
	if got := strings.TrimRight(string(text), "\x00"); got != "remove: ID="+id {
		t.Errorf("текст startup-info %q", got)
	}
}

// Токен без «_TIME» — метка последнего ввода окна; пустой токен — как Activate
// (и startup-info не шлётся).
func TestX11_ActivateWithToken_Fallbacks(t *testing.T) {
	w, rec := newActX11(t)
	w.act.userTime.Store(777)
	w.ActivateWithToken("plain-token")
	reqs := rec.wait(t, 3)
	var ts uint32
	for _, q := range reqs {
		if q.major == 25 && binary.LittleEndian.Uint32(q.raw[8:12]) == 0x00180000 {
			_, _, _, ts, _ = activeMsg(t, q)
		}
	}
	if ts != 777 {
		t.Errorf("метка %d, ждал время последнего ввода 777", ts)
	}

	w, rec = newActX11(t)
	w.ActivateWithToken("")
	rec.wait(t, 3)
	rec.quiet(t, 3) // ни startup-info, ни чего-либо ещё
}

// Свёрнутое окно Activate сначала показывает (MapWindow, ICCCM), фокус не
// ставит — окно ещё не видимо, фокус отдаст менеджер.
func TestX11_Activate_MinimizedMapsWindow(t *testing.T) {
	w, rec := newActX11(t)
	w.act.minimized.Store(true)
	w.Activate()
	reqs := rec.wait(t, 3)
	if reqs[0].major != 8 || binary.LittleEndian.Uint32(reqs[0].raw[4:8]) != tWid {
		t.Errorf("первым ждал MapWindow окна, пришло %v", reqs[0])
	}
	if reqs[1].major != 25 || reqs[2].major != 12 {
		t.Errorf("дальше ждал _NET_ACTIVE_WINDOW и подъём, пришло %d и %d", reqs[1].major, reqs[2].major)
	}
	rec.quiet(t, 3)
}

// Куски startup-info: границы по 20 байт, нулевой терминатор, первый — begin.
func TestX11StartupInfoRequests_Chunks(t *testing.T) {
	// 19 символов + NUL = ровно один кусок; 20 символов + NUL = два.
	one := x11StartupInfoRequests(1, 2, 3, 4, strings.Repeat("a", 19))
	two := x11StartupInfoRequests(1, 2, 3, 4, strings.Repeat("a", 20))
	if len(one) != 1 || len(two) != 2 {
		t.Fatalf("кусков %d и %d, ждал 1 и 2", len(one), len(two))
	}
	if binary.LittleEndian.Uint32(two[0][12+8:12+12]) != 3 || binary.LittleEndian.Uint32(two[1][12+8:12+12]) != 4 {
		t.Error("типы кусков: ждал begin, затем cont")
	}
	if two[1][12+12] != 0 { // хвост добит нулями
		t.Errorf("второй кусок: %v", two[1][12+12:])
	}
}

func TestX11StartupQuote(t *testing.T) {
	for in, want := range map[string]string{
		"simple_TIME5": "simple_TIME5",
		"with space":   `"with space"`,
		`quo"te`:       `"quo\"te"`,
		`back\slash`:   `"back\\slash"`,
		"":             `""`,
	} {
		if got := x11StartupQuote(in); got != want {
			t.Errorf("x11StartupQuote(%q) = %q, ждал %q", in, got, want)
		}
	}
}

// Свёрнутость из _NET_WM_STATE_HIDDEN по PropertyNotify, с уведомлениями
// ровно на смене; полный экран при этом не страдает.
func TestX11_Minimized_FromNetWMState(t *testing.T) {
	w, rec := newActX11(t)
	var got []bool
	w.SetOnMinimizedChanged(func(m bool) { got = append(got, m) })
	if w.IsMinimized() {
		t.Fatal("новое окно свёрнуто")
	}

	// Чужое окно не вызывает перечитывание.
	w.handleX11Event(propNotify(tWid+1, tAtomState))
	w.x11SyncWMState()
	rec.quiet(t, 0)

	rec.setState(tAtomHidden)
	w.handleX11Event(propNotify(tWid, tAtomState))
	w.x11SyncWMState()
	if !w.IsMinimized() || len(got) != 1 || !got[0] {
		t.Fatalf("после HIDDEN: IsMinimized=%v, уведомления %v", w.IsMinimized(), got)
	}

	// Повтор без изменений — без уведомления.
	w.handleX11Event(propNotify(tWid, tAtomState))
	w.x11SyncWMState()
	if len(got) != 1 {
		t.Errorf("повтор дал уведомление: %v", got)
	}

	rec.setState() // менеджер развернул окно из значка
	w.handleX11Event(propNotify(tWid, tAtomState))
	w.x11SyncWMState()
	if w.IsMinimized() || len(got) != 2 || got[1] {
		t.Fatalf("после снятия HIDDEN: IsMinimized=%v, уведомления %v", w.IsMinimized(), got)
	}
}

// Менеджер без EWMH-свойства: свёрнутость из ICCCM WM_STATE = Iconic; пока
// окно «значок» по WM_STATE, пустой _NET_WM_STATE его не разворачивает.
func TestX11_Minimized_FromICCCMState(t *testing.T) {
	w, rec := newActX11(t)
	var got []bool
	w.SetOnMinimizedChanged(func(m bool) { got = append(got, m) })

	rec.setWM(x11WMStateIconic, 0)
	w.handleX11Event(propNotify(tWid, tAtomWMState))
	w.x11SyncWMState()
	if !w.IsMinimized() || len(got) != 1 || !got[0] {
		t.Fatalf("WM_STATE=Iconic: IsMinimized=%v, уведомления %v", w.IsMinimized(), got)
	}

	// Пустой _NET_WM_STATE (полный экран выключили и т.п.) не снимает «значок».
	rec.setState()
	w.handleX11Event(propNotify(tWid, tAtomState))
	w.x11SyncWMState()
	if !w.IsMinimized() {
		t.Error("пустой _NET_WM_STATE снял свёрнутость при WM_STATE=Iconic")
	}

	rec.setWM(1, 0) // NormalState
	w.handleX11Event(propNotify(tWid, tAtomWMState))
	w.x11SyncWMState()
	if w.IsMinimized() || len(got) != 2 || got[1] {
		t.Fatalf("WM_STATE=Normal: IsMinimized=%v, уведомления %v", w.IsMinimized(), got)
	}
}

// Сворачивание просит менеджера через WM_CHANGE_STATE окна клиента (ICCCM
// 4.1.4), а не корневого окна.
func TestX11_Minimize_RequestTargetsClientWindow(t *testing.T) {
	w, rec := newActX11(t)
	w.atomWMChangeState = 320
	w.Minimize()
	q := rec.wait(t, 1)[0]
	m := q.raw[12:]
	le := binary.LittleEndian
	if q.major != 25 || le.Uint32(m[4:8]) != tWid || le.Uint32(m[8:12]) != 320 || le.Uint32(m[12:16]) != x11WMStateIconic {
		t.Errorf("WM_CHANGE_STATE: окно %d, тип %d, состояние %d", le.Uint32(m[4:8]), le.Uint32(m[8:12]), le.Uint32(m[12:16]))
	}
}
