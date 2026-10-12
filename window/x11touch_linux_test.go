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
			binary.LittleEndian.PutUint16(reply[8:10], s.verMajor)
			binary.LittleEndian.PutUint16(reply[10:12], s.verMinor)
		case req[0] == s.major && req[1] == xiMinorSelectEvents:
			// Ответа нет; ошибка (если велено) уйдёт перед ответом на следующий
			// запрос — как на настоящем сервере. Писать её сразу нельзя: net.Pipe
			// синхронный, а клиент ещё пишет следующий запрос.
			pendingErr = s.selectErr
			continue
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
	// QueryExtension, XIQueryVersion, XISelectEvents, GetInputFocus — в таком порядке.
	if len(reqs) != 4 {
		t.Fatalf("запросов %d", len(reqs))
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
