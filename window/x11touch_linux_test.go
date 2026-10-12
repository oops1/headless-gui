//go:build linux && !android

package window

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

func TestXI_QueryExtensionMatchesShm(t *testing.T) {
	if !bytes.Equal(x11QueryExtensionRequest("MIT-SHM"), x11QueryShmExtensionRequest()) {
		t.Errorf("общий построитель QueryExtension расходится с готовым запросом MIT-SHM")
	}
}

// fakeXServer — игрушечный X-сервер на одном конце net.Pipe: читает запросы,
// отвечает на нужные и запоминает все, чтобы тест видел, что и в каком порядке
// ушло.
type fakeXServer struct {
	conn net.Conn

	mu       sync.Mutex
	requests [][]byte

	present   bool   // XInputExtension есть
	major     byte   // его major opcode
	verMajor  uint16 // версия XI в ответе
	verMinor  uint16
	selectErr bool // ответить на XISelectEvents ошибкой

	atoms   map[string]uint32 // ответы InternAtom; нет в карте — атома нет (0)
	devices []xiDevice        // ответ XIQueryDevice
	// beforeVersion — события, которые сервер присылает ПЕРЕД ответом на
	// XIQueryVersion: ответ обязан отделяться от них, а они — обрабатываться.
	beforeVersion [][]byte
}

func newFakeXServer(conn net.Conn) *fakeXServer {
	return &fakeXServer{conn: conn, present: true, major: 131, verMajor: 2, verMinor: 2}
}

func (s *fakeXServer) serve() {
	pendingErr := false
	for {
		hdr := make([]byte, 4)
		if _, err := io.ReadFull(s.conn, hdr); err != nil {
			return
		}
		n := int(binary.LittleEndian.Uint16(hdr[2:4])) * 4
		req := make([]byte, n)
		copy(req, hdr)
		if n > 4 {
			if _, err := io.ReadFull(s.conn, req[4:]); err != nil {
				return
			}
		}
		s.mu.Lock()
		s.requests = append(s.requests, req)
		s.mu.Unlock()

		reply := make([]byte, 32)
		reply[0] = 1
		switch {
		case req[0] == 98: // QueryExtension
			if s.present {
				reply[8], reply[9], reply[10] = 1, s.major, 100
			}
		case req[0] == s.major && req[1] == xiMinorQueryVersion:
			for _, ev := range s.beforeVersion {
				s.conn.Write(ev)
			}
			binary.LittleEndian.PutUint16(reply[8:10], s.verMajor)
			binary.LittleEndian.PutUint16(reply[10:12], s.verMinor)
		case req[0] == s.major && req[1] == xiMinorSelectEvents:
			// Ответа нет; ошибка (если велено) уйдёт перед ответом на следующий
			// запрос — как на настоящем сервере. Писать её сразу нельзя: net.Pipe
			// синхронный, а клиент ещё пишет следующий запрос.
			pendingErr = s.selectErr
			continue
		case req[0] == 16: // InternAtom
			n := int(binary.LittleEndian.Uint16(req[4:6]))
			binary.LittleEndian.PutUint32(reply[8:12], s.atoms[string(req[8:8+n])])
		case req[0] == s.major && req[1] == xiMinorQueryDevice:
			reply = xiQueryDeviceReplyBytes(s.devices)
		case req[0] == 43: // GetInputFocus
			if pendingErr {
				e := make([]byte, 32)
				e[1] = 3 // BadWindow
				s.conn.Write(e)
				pendingErr = false
			}
		default:
			continue
		}
		s.conn.Write(reply)
	}
}

func (s *fakeXServer) saw() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte(nil), s.requests...)
}

func initTouchAgainst(t *testing.T, cfg func(*fakeXServer)) (*X11Window, *fakeXServer) {
	t.Helper()
	cli, srv := net.Pipe()
	t.Cleanup(func() { cli.Close(); srv.Close() })
	fx := newFakeXServer(srv)
	if cfg != nil {
		cfg(fx)
	}
	go fx.serve()
	w := &X11Window{conn: cli, wid: 0x400001}
	w.SetTouchEnabled(true)
	done := make(chan struct{})
	go func() { w.x11InitTouch(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("x11InitTouch завис")
	}
	return w, fx
}

func TestX11InitTouch_Handshake(t *testing.T) {
	w, fx := initTouchAgainst(t, nil)
	if !w.touch.selected.Load() {
		t.Fatal("подписка на касания не удалась")
	}
	if w.touch.opcode.Load() != 131 {
		t.Errorf("opcode расширения %d", w.touch.opcode.Load())
	}
	reqs := fx.saw()
	// QueryExtension, XIQueryVersion, XISelectEvents, GetInputFocus — в таком порядке,
	// затем пять InternAtom (метки осей) и XIQueryDevice.
	if len(reqs) != 4+5+1 {
		t.Fatalf("запросов %d", len(reqs))
	}
	for i := 4; i < 9; i++ {
		if reqs[i][0] != 16 || reqs[i][1] != 1 {
			t.Errorf("запрос %d: %v, ждал InternAtom(only_if_exists)", i, reqs[i])
		}
	}
	if !bytes.Equal(reqs[9], xiQueryDeviceRequest(131, xiAllDevices)) {
		t.Errorf("XIQueryDevice: %v", reqs[9])
	}
	if reqs[0][0] != 98 || string(reqs[0][8:23]) != "XInputExtension" {
		t.Errorf("первый запрос: %v", reqs[0])
	}
	if !bytes.Equal(reqs[1], xiQueryVersionRequest(131, 2, 2)) {
		t.Errorf("XIQueryVersion: %v", reqs[1])
	}
	if !bytes.Equal(reqs[2], xiSelectEventsRequest(131, 0x400001, xiAllMasterDevices, xiTouchEventMask())) {
		t.Errorf("XISelectEvents: %v", reqs[2])
	}
	if reqs[3][0] != 43 {
		t.Errorf("проверка ошибки: %v", reqs[3])
	}
}

func TestX11InitTouch_NotRequested(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	w := &X11Window{conn: cli, wid: 1}
	// Окно не просило касаний: к серверу ни одного запроса.
	w.x11InitTouch()
	srv.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if n, _ := srv.Read(make([]byte, 64)); n != 0 {
		t.Errorf("окно без касаний обратилось к серверу (%d байт)", n)
	}
	if w.touch.selected.Load() {
		t.Error("подписка без просьбы")
	}
}

func TestX11InitTouch_NoExtension(t *testing.T) {
	w, fx := initTouchAgainst(t, func(s *fakeXServer) { s.present = false })
	if w.touch.selected.Load() {
		t.Error("подписка при отсутствии XInput")
	}
	if n := len(fx.saw()); n != 1 {
		t.Errorf("после отказа запросов %d, ждал 1", n)
	}
}

func TestX11InitTouch_OldVersion(t *testing.T) {
	for _, v := range [][2]uint16{{2, 0}, {2, 1}, {1, 5}} {
		w, fx := initTouchAgainst(t, func(s *fakeXServer) { s.verMajor, s.verMinor = v[0], v[1] })
		if w.touch.selected.Load() {
			t.Errorf("XI %d.%d: подписка есть, а касаний там нет", v[0], v[1])
		}
		for _, r := range fx.saw() {
			if r[0] == 131 && r[1] == xiMinorSelectEvents {
				t.Errorf("XI %d.%d: XISelectEvents отправлен", v[0], v[1])
			}
		}
	}
	// Новее требуемого — годится.
	w, _ := initTouchAgainst(t, func(s *fakeXServer) { s.verMajor, s.verMinor = 2, 4 })
	if !w.touch.selected.Load() {
		t.Error("XI 2.4 отвергнут")
	}
}

func TestX11InitTouch_SelectRejected(t *testing.T) {
	w, _ := initTouchAgainst(t, func(s *fakeXServer) { s.selectErr = true })
	if w.touch.selected.Load() {
		t.Error("подписка отмечена удавшейся при ошибке сервера")
	}
}

// pipeWindow — окно X11 с готовой подпиской, читающее события из net.Pipe.
func pipeWindow(t *testing.T) (*X11Window, net.Conn) {
	t.Helper()
	cli, srv := net.Pipe()
	t.Cleanup(func() { cli.Close(); srv.Close() })
	w := &X11Window{conn: cli}
	w.touch.want.Store(true)
	w.touch.selected.Store(true)
	w.touch.opcode.Store(131)
	return w, srv
}

// Хвост GenericEvent обязан быть прочитан целиком, иначе следующие события
// читаются со смещением — и всё соединение превращается в мусор.
func TestX11EventLoop_GenericEventTail(t *testing.T) {
	w, srv := pipeWindow(t)

	var mu sync.Mutex
	var touches []widget.TouchEvent
	var activations []bool
	w.SetOnTouch(func(ev widget.TouchEvent) { mu.Lock(); touches = append(touches, ev); mu.Unlock() })
	w.SetOnActivate(func(a bool) { mu.Lock(); activations = append(activations, a); mu.Unlock() })

	focusIn := make([]byte, 32)
	focusIn[0] = 9
	focusOut := make([]byte, 32)
	focusOut[0] = 10

	go func() {
		// Хвост 16 байт: маска кнопок и значения валюаторов.
		srv.Write(xiEventBytes(131, xiEvTouchBegin, 5, 1000, 10, 20, 0, 16))
		srv.Write(focusIn)
		srv.Write(xiEventBytes(131, xiEvTouchUpdate, 5, 1016, 12, 24, 0, 0))
		srv.Write(focusOut)
		srv.Write(xiEventBytes(131, xiEvTouchEnd, 5, 1032, 12, 24, 0, 8))
		srv.Close()
	}()
	w.RunEventLoop()

	mu.Lock()
	defer mu.Unlock()
	if len(touches) != 3 {
		t.Fatalf("касаний %d: %+v", len(touches), touches)
	}
	if touches[0].Phase != widget.TouchBegan || touches[0].X != 10 || touches[0].Y != 20 ||
		touches[1].Phase != widget.TouchMoved || touches[2].Phase != widget.TouchEnded {
		t.Errorf("касания: %+v", touches)
	}
	if len(activations) != 2 || !activations[0] || activations[1] {
		t.Errorf("обычные события вокруг GenericEvent: %v, ждал [true false]", activations)
	}
}

// Окно без подписки не получает ничего от XI2 и не падает на чужих событиях.
func TestX11EventLoop_GenericIgnoredWithoutSelection(t *testing.T) {
	w, srv := pipeWindow(t)
	w.touch.selected.Store(false)
	var n int
	w.SetOnTouch(func(widget.TouchEvent) { n++ })
	go func() {
		srv.Write(xiEventBytes(131, xiEvTouchBegin, 1, 1, 1, 1, 0, 0))
		srv.Close()
	}()
	w.RunEventLoop()
	if n != 0 {
		t.Errorf("касание доставлено без подписки: %d", n)
	}
}

// Выключенные на лету касания молча отбрасываются.
func TestX11EventLoop_DisabledDropsTouches(t *testing.T) {
	w, srv := pipeWindow(t)
	w.SetTouchEnabled(false)
	var n int
	w.SetOnTouch(func(widget.TouchEvent) { n++ })
	go func() {
		srv.Write(xiEventBytes(131, xiEvTouchBegin, 1, 1, 1, 1, 0, 0))
		srv.Close()
	}()
	w.RunEventLoop()
	if n != 0 {
		t.Errorf("выключенное касание доставлено: %d", n)
	}
}

// coreButton — событие ядра ButtonPress/ButtonRelease с меткой времени.
func coreButton(typ byte, tm uint32) []byte {
	b := make([]byte, 32)
	b[0] = typ
	b[1] = 1 // левая кнопка
	binary.LittleEndian.PutUint32(b[4:8], tm)
	return b
}

// Эмулированный сервером указатель не должен дублировать касание.
func TestX11EventLoop_EmulatedPointerSwallowed(t *testing.T) {
	w, srv := pipeWindow(t)
	var mu sync.Mutex
	var buttons []bool
	w.SetOnMouseButton(func(x, y, b int, p bool) { mu.Lock(); buttons = append(buttons, p); mu.Unlock() })
	w.SetOnTouch(func(widget.TouchEvent) {})

	go func() {
		srv.Write(xiEventBytes(131, xiEvTouchBegin, 1, 100, 5, 5, xiTouchEmulatingPointer, 0))
		srv.Write(coreButton(4, 100)) // эмулированное нажатие
		srv.Write(xiEventBytes(131, xiEvTouchEnd, 1, 150, 5, 5, xiTouchEmulatingPointer, 0))
		srv.Write(coreButton(5, 150)) // эмулированное отпускание
		srv.Write(coreButton(4, 900)) // настоящая мышь, позже
		srv.Write(coreButton(5, 910))
		srv.Close()
	}()
	w.RunEventLoop()

	mu.Lock()
	defer mu.Unlock()
	if len(buttons) != 2 || !buttons[0] || buttons[1] {
		t.Errorf("кнопки мыши: %v, ждал только настоящие нажатие и отпускание", buttons)
	}
}

// Окно, которое касаний не просило, мышь не подавляет вовсе.
func TestX11EventLoop_NoGuardWithoutTouch(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	w := &X11Window{conn: cli}
	var n int
	w.SetOnMouseButton(func(x, y, b int, p bool) { n++ })
	go func() {
		srv.Write(coreButton(4, 100))
		srv.Write(coreButton(5, 110))
		srv.Close()
	}()
	w.RunEventLoop()
	if n != 2 {
		t.Errorf("мышь окна без касаний: %d событий, ждал 2", n)
	}
}

// Живая проверка против настоящего X-сервера (Xvfb, WSLg/XWayland): окно
// просит касания, и согласование XInput 2.2 проходит, не ломая соединение.
// Касаний на таком сервере нет, поэтому проверяется только подписка и то, что
// после неё цикл событий продолжает жить. Нет DISPLAY — тест пропускается.
func TestX11TouchLive(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера")
	}
	w := &X11Window{}
	w.SetTouchEnabled(true)
	if err := w.Create("touch e2e", 64, 48); err != nil {
		t.Skipf("не удалось поднять окно: %v", err)
	}
	t.Logf("XI2 касания: подписка=%v opcode=%d", w.touch.selected.Load(), w.touch.opcode.Load())

	done := make(chan struct{})
	go func() { w.RunEventLoop(); close(done) }()
	// Соединение живо и после подписки отвечает на запросы.
	w.SetTitle("после подписки")
	time.Sleep(200 * time.Millisecond)
	w.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("цикл событий не завершился")
	}
}

// ─── Оси касания через настоящее согласование ───────────────────────────────

func touchFakeAtoms() map[string]uint32 {
	return map[string]uint32{
		xiAxisPressure: 501, xiAxisMajor: 502, xiAxisMinor: 503, xiAxisOrient: 504, xiAxisPosX: 505,
	}
}

// Подписка на касания узнаёт оси устройств, и дальше нажим с пятном доходят до
// приёмника с событиями, пришедшими по тому же соединению.
func TestX11InitTouch_LoadsAxesAndEventsCarryPressure(t *testing.T) {
	w, _ := initTouchAgainst(t, func(s *fakeXServer) {
		s.atoms = touchFakeAtoms()
		s.devices = []xiDevice{{ID: 3}, touchscreenDevice(9)}
	})
	if !w.touch.selected.Load() {
		t.Fatal("подписка не удалась")
	}
	if w.touch.atoms != testTouchAtoms {
		t.Errorf("атомы: %+v", w.touch.atoms)
	}
	if len(w.touch.axes.axes) != 2 || !w.touch.axes.axes[9].pressure.ok {
		t.Fatalf("оси устройств: %+v", w.touch.axes.axes)
	}
}

// Атомов нет и устройств нет: касания работают, нажима и пятна нет.
func TestX11InitTouch_NoAxesStillWorks(t *testing.T) {
	w, _ := initTouchAgainst(t, nil)
	if !w.touch.selected.Load() {
		t.Fatal("подписка не удалась")
	}
	tr := &w.touch.axes
	e, _ := parseXITouchEvent(xiEventWithAxes(131, xiEvTouchBegin, 1, 9, map[int]float64{2: 100}), 131)
	ev := e.touch()
	tr.apply(&ev, e)
	if ev.Pressure != 0 || ev.Major != 0 {
		t.Errorf("нажим откуда-то взялся: %+v", ev)
	}
}

// Событие с осями проходит цикл событий и доходит до приёмника с нажимом.
func TestX11EventLoop_PressureReachesHandler(t *testing.T) {
	w, srv := pipeWindow(t)
	w.touch.axes.setDevices([]xiDevice{touchscreenDevice(9)}, testTouchAtoms, 2000)
	var got []widget.TouchEvent
	w.SetOnTouch(func(ev widget.TouchEvent) { got = append(got, ev) })
	go func() {
		srv.Write(xiEventWithAxes(131, xiEvTouchBegin, 4, 9, map[int]float64{0: 10, 1: 10, 2: 255, 3: 20, 4: 10}))
		srv.Write(xiEventWithAxes(131, xiEvTouchUpdate, 4, 9, map[int]float64{0: 12}))
		srv.Write(xiEventWithAxes(131, xiEvTouchEnd, 4, 9, nil))
		srv.Close()
	}()
	w.RunEventLoop()
	if len(got) != 3 {
		t.Fatalf("касаний %d", len(got))
	}
	if got[0].Pressure != 1 || got[0].Major != 10 || got[0].Minor != 5 {
		t.Errorf("Began: %+v", got[0])
	}
	if got[1].Pressure != 1 || got[1].Major != 10 {
		t.Errorf("Update унаследовал: %+v", got[1])
	}
	if len(w.touch.axes.last) != 0 {
		t.Errorf("после отрыва остались значения: %v", w.touch.axes.last)
	}
}

// Новое устройство, которого не было в списке, помечает список устаревшим; цикл
// между событиями запрашивает XIQueryDevice заново, и касания этого устройства
// получают нажим.
func TestX11EventLoop_NewDeviceRefreshesAxes(t *testing.T) {
	cli, srv := net.Pipe()
	t.Cleanup(func() { cli.Close(); srv.Close() })
	fx := newFakeXServer(srv)
	fx.devices = []xiDevice{touchscreenDevice(9), touchscreenDevice(12)}
	go fx.serve()

	w := &X11Window{conn: cli}
	w.screen.WidthInPixels = 2000
	w.touch.want.Store(true)
	w.touch.selected.Store(true)
	w.touch.opcode.Store(131)
	w.touch.atoms = testTouchAtoms
	w.touch.axes.setDevices([]xiDevice{touchscreenDevice(9)}, testTouchAtoms, 2000)

	var mu sync.Mutex
	var got []widget.TouchEvent
	w.SetOnTouch(func(ev widget.TouchEvent) { mu.Lock(); got = append(got, ev); mu.Unlock() })
	done := make(chan struct{})
	go func() { w.RunEventLoop(); close(done) }()

	// Палец с устройства 12: на первом событии списка ещё нет.
	srv2 := fx.conn
	srv2.Write(xiEventWithAxes(131, xiEvTouchBegin, 1, 12, map[int]float64{2: 255}))
	waitX11(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == 1 })
	waitX11(t, func() bool { return w.touch.axesGen.Load() >= 1 })
	srv2.Write(xiEventWithAxes(131, xiEvTouchUpdate, 1, 12, map[int]float64{2: 255}))
	waitX11(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == 2 })
	mu.Lock()
	if got[0].Pressure != 0 || got[1].Pressure != 1 {
		t.Errorf("нажим до/после обновления: %v / %v, ждал 0 / 1", got[0].Pressure, got[1].Pressure)
	}
	mu.Unlock()
	queries := 0
	for _, r := range fx.saw() {
		if r[0] == 131 && r[1] == xiMinorQueryDevice {
			queries++
		}
	}
	if queries != 1 {
		t.Errorf("XIQueryDevice отправлен %d раз, ждал 1", queries)
	}
	cli.Close()
	<-done
}

// waitX11 ждёт условия до трёх секунд.
func waitX11(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("условие не наступило")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ─── Включение касаний после создания окна ──────────────────────────────────

// Окно создано без касаний, цикл событий уже крутится; приложение просит касания.
// Подписка делается в безопасной точке цикла: события, пришедшие раньше ответов,
// не теряются, а само включение будит спящий цикл безвредным запросом.
func TestX11SetTouchEnabled_AfterCreate(t *testing.T) {
	cli, srv := net.Pipe()
	t.Cleanup(func() { cli.Close(); srv.Close() })
	fx := newFakeXServer(srv)
	fx.atoms = touchFakeAtoms()
	fx.devices = []xiDevice{touchscreenDevice(9)}
	focusIn := make([]byte, 32)
	focusIn[0] = 9
	fx.beforeVersion = [][]byte{focusIn}
	go fx.serve()

	w := &X11Window{conn: cli, wid: 0x400001}
	w.screen.WidthInPixels = 2000
	var mu sync.Mutex
	var activations int
	var touches []widget.TouchEvent
	w.SetOnActivate(func(bool) { mu.Lock(); activations++; mu.Unlock() })
	w.SetOnTouch(func(ev widget.TouchEvent) { mu.Lock(); touches = append(touches, ev); mu.Unlock() })

	done := make(chan struct{})
	go func() { w.RunEventLoop(); close(done) }()
	waitX11(t, w.touch.looping.Load)

	if w.touch.selected.Load() {
		t.Fatal("подписка без просьбы")
	}
	if n := len(fx.saw()); n != 0 {
		t.Fatalf("окно без касаний обратилось к серверу: %d запросов", n)
	}

	w.SetTouchEnabled(true)
	waitX11(t, w.touch.selected.Load)

	reqs := fx.saw()
	// Первым ушёл будящий GetInputFocus, затем согласование.
	if len(reqs) < 4 || reqs[0][0] != 43 || reqs[1][0] != 98 {
		t.Fatalf("порядок запросов: первый %v, второй %v", reqs[0][:1], reqs[1][:1])
	}
	var sawSelect bool
	for _, r := range reqs {
		if r[0] == 131 && r[1] == xiMinorSelectEvents {
			sawSelect = true
		}
	}
	if !sawSelect {
		t.Error("XISelectEvents не отправлен")
	}
	mu.Lock()
	if activations != 1 {
		t.Errorf("событие, пришедшее раньше ответа, обработано %d раз, ждал 1", activations)
	}
	mu.Unlock()
	if w.touch.pending.Load() {
		t.Error("pending остался взведённым")
	}

	// И подписка действует: касание с нажимом доходит до приёмника.
	waitX11(t, func() bool { return w.touch.axesGen.Load() >= 1 })
	fx.conn.Write(xiEventWithAxes(131, xiEvTouchBegin, 1, 9, map[int]float64{2: 255}))
	waitX11(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(touches) == 1 })
	mu.Lock()
	if touches[0].Pressure != 1 {
		t.Errorf("нажим %v", touches[0].Pressure)
	}
	mu.Unlock()

	// Повторное включение подписку не повторяет.
	n := len(fx.saw())
	w.SetTouchEnabled(true)
	time.Sleep(50 * time.Millisecond)
	if len(fx.saw()) != n {
		t.Error("повторное включение снова ходит к серверу")
	}
	cli.Close()
	<-done
}

// Просьба пришла после Create, но ДО запуска цикла: цикл выполняет её первым делом.
func TestX11SetTouchEnabled_BeforeLoopStarts(t *testing.T) {
	cli, srv := net.Pipe()
	t.Cleanup(func() { cli.Close(); srv.Close() })
	fx := newFakeXServer(srv)
	go fx.serve()
	w := &X11Window{conn: cli, wid: 0x400001}
	w.SetTouchEnabled(true) // цикла ещё нет: будить некого, запрос не уходит
	if n := len(fx.saw()); n != 0 {
		t.Fatalf("до цикла ушло %d запросов", n)
	}
	if !w.touch.pending.Load() {
		t.Fatal("просьба не запомнена")
	}
	done := make(chan struct{})
	go func() { w.RunEventLoop(); close(done) }()
	waitX11(t, w.touch.selected.Load)
	cli.Close()
	<-done
}

// Живая проверка включения касаний ПОСЛЕ создания окна: окно создано без просьбы,
// цикл событий крутится, затем приложение просит касания. На настоящем сервере
// подписка должна состояться из цикла, а соединение — остаться живым. Заодно
// проверяется разбор настоящего ответа XIQueryDevice (устройства сервера, их
// валюаторы). Нет DISPLAY — тест пропускается.
func TestX11TouchLiveDeferred(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера")
	}
	w := &X11Window{}
	if err := w.Create("touch e2e deferred", 64, 48); err != nil {
		t.Skipf("не удалось поднять окно: %v", err)
	}
	if w.touch.selected.Load() {
		t.Fatal("подписка есть, хотя касания не просили")
	}
	done := make(chan struct{})
	go func() { w.RunEventLoop(); close(done) }()
	waitX11(t, w.touch.looping.Load)

	w.SetTouchEnabled(true)
	deadline := time.Now().Add(3 * time.Second)
	for !w.touch.selected.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !w.touch.selected.Load() {
		t.Fatal("отложенная подписка на касания не состоялась")
	}
	// Соединение живо и после подписки отвечает на запросы.
	w.SetTitle("после отложенной подписки")
	time.Sleep(200 * time.Millisecond)
	w.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("цикл событий не завершился")
	}
	t.Logf("XI2: подписка=%v, загрузок списка осей=%d, устройств в списке=%d",
		w.touch.selected.Load(), w.touch.axesGen.Load(), len(w.touch.axes.axes))
	if w.touch.axesGen.Load() == 0 {
		t.Error("список осей устройств не загружен")
	}
	for id, ax := range w.touch.axes.axes {
		t.Logf("  устройство %d: нажим=%v пятно=%v", id, ax.pressure.ok, ax.major.ok)
	}
}
