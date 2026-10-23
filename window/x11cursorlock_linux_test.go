//go:build linux && !android

package window

import (
	"encoding/binary"
	"image/color"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/oops1/headless-gui/v3/widget"
)

// X11: курсор-картинка (RENDER) и захват указателя (XInput2 + XGrabPointer) на
// игрушечном X-сервере поверх буферизованной пары сокетов. Цикл событий окна —
// настоящий (RunEventLoop): просьбы исполняются в нём, как в работе.

const (
	xlfRenderMajor = 140
	xlfXIMajor     = 131
	xlfArgbFormat  = 0x77
)

// xlfServer — игрушечный X-сервер: отвечает на то, что нужно этим тестам, и
// запоминает все запросы.
type xlfServer struct {
	conn net.Conn

	wmu sync.Mutex // записи в сокет клиента (ответы и вброшенные события)
	mu  sync.Mutex
	req [][]byte

	render      bool // расширение RENDER есть
	renderMinor uint32
	noArgb      bool // формата ARGB32 в списке нет
	renderErr   bool // CreateCursor RENDER отвергается ошибкой
	xi          bool // XInputExtension есть
	grabStatus  byte // статус ответа GrabPointer

	pendingErr bool
}

func xlfPair(t *testing.T) (client, server net.Conn) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Skipf("socketpair недоступен: %v", err)
	}
	mk := func(fd int) net.Conn {
		f := os.NewFile(uintptr(fd), "x11")
		c, err := net.FileConn(f)
		f.Close()
		if err != nil {
			t.Fatalf("FileConn: %v", err)
		}
		return c
	}
	client, server = mk(fds[0]), mk(fds[1])
	t.Cleanup(func() { client.Close(); server.Close() })
	return client, server
}

func (s *xlfServer) write(b []byte) {
	s.wmu.Lock()
	s.conn.Write(b)
	s.wmu.Unlock()
}

func (s *xlfServer) serve() {
	for {
		hdr := make([]byte, 4)
		if _, err := io.ReadFull(s.conn, hdr); err != nil {
			return
		}
		n := int(binary.LittleEndian.Uint16(hdr[2:4])) * 4
		if n < 4 {
			n = 4
		}
		req := make([]byte, n)
		copy(req, hdr)
		if n > 4 {
			if _, err := io.ReadFull(s.conn, req[4:]); err != nil {
				return
			}
		}
		s.mu.Lock()
		s.req = append(s.req, req)
		s.mu.Unlock()

		reply := make([]byte, 32)
		reply[0] = 1
		switch {
		case req[0] == 98: // QueryExtension
			name := string(req[8 : 8+int(binary.LittleEndian.Uint16(req[4:6]))])
			switch {
			case name == renderExtensionName && s.render:
				reply[8], reply[9] = 1, xlfRenderMajor
			case name == xiExtensionName && s.xi:
				reply[8], reply[9] = 1, xlfXIMajor
			}
		case req[0] == xlfRenderMajor && req[1] == renderMinorQueryVersion:
			binary.LittleEndian.PutUint32(reply[12:], s.renderMinor)
		case req[0] == xlfRenderMajor && req[1] == renderMinorQueryPictFormats:
			formats := [][]byte{xrFormat(0x20, 1, 24, 16, 255, 8, 255, 0, 255, 0, 0)}
			if !s.noArgb {
				formats = append(formats, xrFormat(xlfArgbFormat, 1, 32, 16, 255, 8, 255, 0, 255, 24, 255))
			}
			reply = xrFormatsReply(formats...)
		case req[0] == xlfRenderMajor && req[1] == renderMinorCreateCursor:
			s.pendingErr = s.renderErr
			continue
		case req[0] == xlfXIMajor && req[1] == xiMinorQueryVersion:
			binary.LittleEndian.PutUint16(reply[8:], 2)
			binary.LittleEndian.PutUint16(reply[10:], 2)
		case req[0] == 26: // GrabPointer
			reply[1] = s.grabStatus
		case req[0] == 43: // GetInputFocus
			if s.pendingErr {
				e := make([]byte, 32)
				e[1] = 11 // BadAlloc
				s.write(e)
				s.pendingErr = false
			}
		case req[0] == 25: // SendEvent: сервер доставляет событие получателю
			ev := append([]byte(nil), req[12:44]...)
			ev[0] |= 0x80
			s.write(ev)
			continue
		default:
			continue
		}
		s.write(reply)
	}
}

func (s *xlfServer) saw() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte(nil), s.req...)
}

// count — сколько запросов удовлетворяют pred.
func (s *xlfServer) count(pred func(r []byte) bool) int {
	n := 0
	for _, r := range s.saw() {
		if pred(r) {
			n++
		}
	}
	return n
}

func isOp(op byte) func([]byte) bool { return func(r []byte) bool { return r[0] == op } }

func isRender(minor byte) func([]byte) bool {
	return func(r []byte) bool { return r[0] == xlfRenderMajor && r[1] == minor }
}

// xlfWait ждёт, пока pred выполнится, до 3 с.
func xlfWait(t *testing.T, what string, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("не дождались: %s", what)
}

// xlfWindow поднимает окно с настоящим циклом событий против игрушечного
// сервера.
func xlfWindow(t *testing.T, cfg func(*xlfServer)) (*X11Window, *xlfServer) {
	t.Helper()
	cli, srv := xlfPair(t)
	s := &xlfServer{conn: srv, render: true, renderMinor: 11, xi: true}
	if cfg != nil {
		cfg(s)
	}
	go s.serve()
	w := &X11Window{conn: cli, wid: 0x400001, rootWin: 0x1A5, width: 800, height: 600,
		ridBase: 0x600000, ridMask: 0x1FFFFF}
	done := make(chan struct{})
	go func() { w.RunEventLoop(); close(done) }()
	t.Cleanup(func() {
		w.closed.Store(true)
		cli.Close()
		<-done
	})
	xlfWait(t, "цикл событий", func() bool { return w.curImg.looping.Load() && w.plock.looping.Load() })
	return w, s
}

func colorNRGBA(r uint8) color.NRGBA { return color.NRGBA{R: r, A: 255} }

func xlfImage(r uint8) *widget.CursorImage {
	return x11ciImage(32, 32, colorNRGBA(r), 4, 5)
}

// ─── Курсор-картинка ─────────────────────────────────────────────────────────

func setCursorAttr(r []byte) bool { // ChangeWindowAttributes с курсором
	return r[0] == 2 && len(r) >= 16 && binary.LittleEndian.Uint32(r[8:12]) == 0x4000
}

func TestX11CursorImage_RenderPath(t *testing.T) {
	w, s := xlfWindow(t, nil)
	w.SetCursorImage(xlfImage(200))
	xlfWait(t, "курсор повешен на окно", func() bool { return s.count(setCursorAttr) == 1 })

	reqs := s.saw()
	// Согласование: QueryExtension(RENDER), версия, форматы — по одному разу.
	if s.count(isOp(98)) != 1 || s.count(isRender(renderMinorQueryVersion)) != 1 || s.count(isRender(renderMinorQueryPictFormats)) != 1 {
		t.Errorf("согласование RENDER: QueryExtension %d, версия %d, форматы %d",
			s.count(isOp(98)), s.count(isRender(renderMinorQueryVersion)), s.count(isRender(renderMinorQueryPictFormats)))
	}
	// Построение: пиксмап глубины 32, PutImage, Picture с форматом ARGB32, Cursor.
	var pix, pic, cur []byte
	for _, r := range reqs {
		switch {
		case r[0] == x11OpCreatePixmap && r[1] == 32:
			pix = r
		case isRender(renderMinorCreatePicture)(r):
			pic = r
		case isRender(renderMinorCreateCursor)(r):
			cur = r
		}
	}
	if pix == nil || pic == nil || cur == nil {
		t.Fatalf("построение курсора неполное: pixmap=%v picture=%v cursor=%v", pix != nil, pic != nil, cur != nil)
	}
	if binary.LittleEndian.Uint32(pic[12:16]) != xlfArgbFormat {
		t.Errorf("формат картинки %#x, ждал %#x", binary.LittleEndian.Uint32(pic[12:16]), xlfArgbFormat)
	}
	if binary.LittleEndian.Uint16(cur[12:14]) != 4 || binary.LittleEndian.Uint16(cur[14:16]) != 5 {
		t.Errorf("горячая точка (%d,%d), ждал (4,5)", binary.LittleEndian.Uint16(cur[12:14]), binary.LittleEndian.Uint16(cur[14:16]))
	}
	// На окно повешен именно построенный курсор.
	var attr []byte
	for _, r := range reqs {
		if setCursorAttr(r) {
			attr = r
		}
	}
	if binary.LittleEndian.Uint32(attr[12:16]) != binary.LittleEndian.Uint32(cur[4:8]) {
		t.Error("на окно повешен не тот курсор, что построен")
	}
	// Первый курсор RENDER проверен у сервера (GetInputFocus после построения).
	if s.count(isOp(43)) < 1 {
		t.Error("построенный курсор не проверен у сервера")
	}
}

// Кэш: та же картинка не строится заново, другая строится, возврат к первой — из
// кэша.
func TestX11CursorImage_Cache(t *testing.T) {
	w, s := xlfWindow(t, nil)
	created := func() int { return s.count(isRender(renderMinorCreateCursor)) }
	shown := func() int { return s.count(setCursorAttr) }

	w.SetCursorImage(xlfImage(10))
	xlfWait(t, "первая", func() bool { return shown() == 1 })
	w.SetCursorImage(xlfImage(10)) // равное содержимое — ничего
	w.SetCursorImage(xlfImage(20))
	xlfWait(t, "вторая", func() bool { return shown() == 2 })
	if created() != 2 {
		t.Fatalf("построено %d курсоров, ждал 2", created())
	}
	w.SetCursorImage(xlfImage(10)) // вернулись: из кэша
	xlfWait(t, "возврат к первой", func() bool { return shown() == 3 })
	if created() != 2 {
		t.Errorf("первая картинка построена заново: %d", created())
	}
	// Ресурс курсора на окне у первой и третьей установок один.
	var ids []uint32
	for _, r := range s.saw() {
		if setCursorAttr(r) {
			ids = append(ids, binary.LittleEndian.Uint32(r[12:16]))
		}
	}
	if ids[0] != ids[2] || ids[0] == ids[1] {
		t.Errorf("ресурсы курсора на окне %v: ждал первый == третий != второй", ids)
	}
}

// Кэш ограничен: давний курсор освобождается на сервере.
func TestX11CursorImage_LRUFreesCursors(t *testing.T) {
	w, s := xlfWindow(t, nil)
	for i := 0; i < cursorCacheSize+1; i++ {
		w.SetCursorImage(xlfImage(uint8(i + 1)))
		n := i + 1
		xlfWait(t, "показ", func() bool { return s.count(setCursorAttr) == n })
	}
	xlfWait(t, "FreeCursor", func() bool { return s.count(isOp(x11OpFreeCursor)) == 1 })
	var first uint32
	for _, r := range s.saw() {
		if isRender(renderMinorCreateCursor)(r) {
			first = binary.LittleEndian.Uint32(r[4:8])
			break
		}
	}
	for _, r := range s.saw() {
		if r[0] == x11OpFreeCursor && binary.LittleEndian.Uint32(r[4:8]) != first {
			t.Errorf("освобождён курсор %#x, ждал самый давний %#x", binary.LittleEndian.Uint32(r[4:8]), first)
		}
	}
}

// Нет RENDER — курсор строится 1-битным по порогу альфы.
func TestX11CursorImage_NoRenderFallsBackToMono(t *testing.T) {
	w, s := xlfWindow(t, func(s *xlfServer) { s.render = false })
	w.SetCursorImage(xlfImage(1))
	xlfWait(t, "показ", func() bool { return s.count(setCursorAttr) == 1 })
	if s.count(isOp(x11OpCreateCursor)) != 1 {
		t.Errorf("1-битный CreateCursor: %d, ждал 1", s.count(isOp(x11OpCreateCursor)))
	}
	if s.count(isRender(renderMinorCreateCursor)) != 0 {
		t.Error("RENDER использован без расширения")
	}
	// Решение запомнено: второй курсор не спрашивает расширение снова.
	w.SetCursorImage(xlfImage(2))
	xlfWait(t, "второй", func() bool { return s.count(setCursorAttr) == 2 })
	if s.count(isOp(98)) != 1 {
		t.Errorf("QueryExtension: %d, ждал 1", s.count(isOp(98)))
	}
}

// RENDER есть, но старый (< 0.5) или без формата ARGB32 — тоже 1-битный.
func TestX11CursorImage_OldRenderOrNoFormat(t *testing.T) {
	for name, cfg := range map[string]func(*xlfServer){
		"версия 0.4":  func(s *xlfServer) { s.renderMinor = 4 },
		"нет формата": func(s *xlfServer) { s.noArgb = true },
	} {
		w, s := xlfWindow(t, cfg)
		w.SetCursorImage(xlfImage(3))
		xlfWait(t, name, func() bool { return s.count(setCursorAttr) == 1 })
		if s.count(isRender(renderMinorCreateCursor)) != 0 || s.count(isOp(x11OpCreateCursor)) != 1 {
			t.Errorf("%s: RENDER-курсоров %d, 1-битных %d", name,
				s.count(isRender(renderMinorCreateCursor)), s.count(isOp(x11OpCreateCursor)))
		}
	}
}

// Сервер отверг первый курсор RENDER ошибкой — окно переходит на 1-битные и
// всё равно показывает курсор.
func TestX11CursorImage_RenderErrorFallsBack(t *testing.T) {
	w, s := xlfWindow(t, func(s *xlfServer) { s.renderErr = true })
	w.SetCursorImage(xlfImage(4))
	xlfWait(t, "показ", func() bool { return s.count(setCursorAttr) == 1 })
	if s.count(isOp(x11OpCreateCursor)) != 1 {
		t.Errorf("после ошибки 1-битных курсоров %d, ждал 1", s.count(isOp(x11OpCreateCursor)))
	}
	// Показан именно запасной курсор.
	var mono uint32
	for _, r := range s.saw() {
		if r[0] == x11OpCreateCursor {
			mono = binary.LittleEndian.Uint32(r[4:8])
		}
	}
	var attr uint32
	for _, r := range s.saw() {
		if setCursorAttr(r) {
			attr = binary.LittleEndian.Uint32(r[12:16])
		}
	}
	if attr != mono {
		t.Errorf("на окно повешен %#x, ждал запасной %#x", attr, mono)
	}
}

// Форма, заданная после картинки, отзывает ещё не показанную картинку: вторым
// показывается не она.
func TestX11CursorImage_ShapeCancelsPendingImage(t *testing.T) {
	cli, srv := xlfPair(t)
	s := &xlfServer{conn: srv, render: true, renderMinor: 11, xi: true}
	go s.serve()
	w := &X11Window{conn: cli, wid: 0x400001, ridBase: 0x600000, ridMask: 0x1FFFFF}

	w.SetCursorImage(xlfImage(7)) // цикл не идёт: просьба ждёт
	w.SetCursor(wlCursorIBeam)
	w.x11SyncCursorImage() // цикл «пришёл» позже формы
	if s.count(isRender(renderMinorCreateCursor)) != 0 || s.count(isOp(98)) != 0 {
		t.Error("отозванная картинка всё же построена")
	}
	if w.curImg.pending != nil || w.curImg.shown != 0 {
		t.Errorf("состояние после формы: pending=%v shown=%d", w.curImg.pending != nil, w.curImg.shown)
	}
	// Форма снова может быть показана, даже та же, что до картинки.
	w.SetCursorImage(xlfImage(8))
	w.cursors.mu.Lock()
	known := w.cursors.known
	w.cursors.mu.Unlock()
	if known {
		t.Error("после картинки форма считается показанной: SetCursor с той же формой не дойдёт до сервера")
	}
}

// Просьба будит цикл событием SendEvent себе, а не запросом с ответом.
func TestX11CursorImage_WakesLoopWithEvent(t *testing.T) {
	w, s := xlfWindow(t, nil)
	w.SetCursorImage(xlfImage(9))
	xlfWait(t, "SendEvent", func() bool { return s.count(isOp(25)) >= 1 })
	if s.count(isOp(43)) > 1 { // единственный GetInputFocus — проверка первого курсора RENDER
		t.Errorf("пробуждение запросом GetInputFocus: %d", s.count(isOp(43)))
	}
}

// ─── Захват указателя ────────────────────────────────────────────────────────

func xlfRawEvent(dx, dy float64) []byte {
	return xiRawEventBytes(xlfXIMajor, xiEvRawMotion, 1, []int{0, 1}, []float64{dx * 3, dy * 3}, []float64{dx, dy})
}

func xlfMotionNotify(x, y int) []byte {
	e := make([]byte, 32)
	e[0] = 6
	binary.LittleEndian.PutUint16(e[24:], uint16(int16(x)))
	binary.LittleEndian.PutUint16(e[26:], uint16(int16(y)))
	return e
}

func xlfFocus(in bool) []byte {
	e := make([]byte, 32)
	if in {
		e[0] = 9
	} else {
		e[0] = 10
	}
	return e
}

type xlfLockLog struct {
	mu      sync.Mutex
	motions [][2]float64
	changes []bool
}

func (l *xlfLockLog) m() [][2]float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([][2]float64(nil), l.motions...)
}

func (l *xlfLockLog) c() []bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]bool(nil), l.changes...)
}

func xlfLocked(t *testing.T, cfg func(*xlfServer)) (*X11Window, *xlfServer, *xlfLockLog) {
	t.Helper()
	w, s := xlfWindow(t, cfg)
	log := &xlfLockLog{}
	w.SetOnRelativeMotion(func(dx, dy float64) {
		log.mu.Lock()
		log.motions = append(log.motions, [2]float64{dx, dy})
		log.mu.Unlock()
	})
	w.SetOnPointerLockChanged(func(a bool) {
		log.mu.Lock()
		log.changes = append(log.changes, a)
		log.mu.Unlock()
	})
	return w, s, log
}

func isXI(minor byte) func([]byte) bool {
	return func(r []byte) bool { return r[0] == xlfXIMajor && r[1] == minor }
}

func TestX11PointerLock_Handshake(t *testing.T) {
	w, s, log := xlfLocked(t, nil)
	if !w.SetPointerLock(true) {
		t.Fatal("SetPointerLock(true) отказал")
	}
	// Возврат указателя в центр уходит уже после ответа на GrabPointer.
	xlfWait(t, "WarpPointer", func() bool { return s.count(isOp(41)) >= 1 })
	reqs := s.saw()
	find := func(pred func([]byte) bool) []byte {
		for _, r := range reqs {
			if pred(r) {
				return r
			}
		}
		return nil
	}
	sel := find(isXI(xiMinorSelectEvents))
	if sel == nil {
		t.Fatal("XISelectEvents не отправлен")
	}
	if binary.LittleEndian.Uint32(sel[4:8]) != 0x1A5 {
		t.Errorf("подписка на окно %#x, ждал корневое 0x1A5", binary.LittleEndian.Uint32(sel[4:8]))
	}
	if sel[18] != 2 { // бит XI_RawMotion
		t.Errorf("маска событий %v", sel[16:20])
	}
	grab := find(isOp(26))
	if grab == nil {
		t.Fatal("GrabPointer не отправлен")
	}
	if binary.LittleEndian.Uint32(grab[4:8]) != 0x400001 || binary.LittleEndian.Uint32(grab[12:16]) != 0x400001 {
		t.Errorf("захват окном/привязка: %v", grab[4:16])
	}
	if binary.LittleEndian.Uint32(grab[16:20]) == 0 {
		t.Error("захват без невидимого курсора")
	}
	warp := find(isOp(41))
	if warp == nil || binary.LittleEndian.Uint16(warp[20:22]) != 400 || binary.LittleEndian.Uint16(warp[22:24]) != 300 {
		t.Errorf("указатель не возвращён в центр: %v", warp)
	}
	if c := log.c(); len(c) != 1 || !c[0] {
		t.Errorf("уведомления %v, ждал [true]", c)
	}

	// Сырое движение доходит до приложения; без ускорения.
	s.write(xlfRawEvent(3, -4))
	xlfWait(t, "сдвиг", func() bool { return len(log.m()) == 1 })
	if m := log.m()[0]; m != [2]float64{3, -4} {
		t.Errorf("сдвиг %v, ждал (3,-4)", m)
	}
}

// Указатель, ушедший далеко от центра, возвращается; рядом с центром — нет.
func TestX11PointerLock_RecentersPointer(t *testing.T) {
	w, s, log := xlfLocked(t, nil)
	w.SetPointerLock(true)
	warps := func() int { return s.count(isOp(41)) }
	xlfWait(t, "возврат при захвате", func() bool { return warps() >= 1 })
	base := warps() // первый возврат — при захвате

	s.write(xlfMotionNotify(420, 310)) // рядом с центром
	s.write(xlfMotionNotify(700, 300)) // ушёл за четверть окна
	xlfWait(t, "возврат", func() bool { return warps() == base+1 })
	time.Sleep(20 * time.Millisecond)
	if warps() != base+1 {
		t.Errorf("возвратов %d, ждал %d", warps()-base, 1)
	}
	if len(log.m()) != 0 {
		t.Errorf("абсолютное движение превратилось в сдвиг: %v", log.m())
	}
}

// Потеря фокуса снимает захват с сервера, возврат — ставит снова.
func TestX11PointerLock_FocusLossAndRestore(t *testing.T) {
	w, s, log := xlfLocked(t, nil)
	w.SetPointerLock(true)
	ungrabs := func() int { return s.count(isOp(x11OpUngrabPointer)) }
	grabs := func() int { return s.count(isOp(26)) }

	s.write(xlfFocus(false))
	xlfWait(t, "UngrabPointer", func() bool { return ungrabs() == 1 })
	// Сдвиги без захвата не доставляются.
	s.write(xlfRawEvent(1, 1))
	time.Sleep(20 * time.Millisecond)
	if len(log.m()) != 0 {
		t.Errorf("сдвиг без захвата доставлен: %v", log.m())
	}

	s.write(xlfFocus(true))
	xlfWait(t, "повторный GrabPointer", func() bool { return grabs() == 2 })
	xlfWait(t, "уведомления", func() bool { return len(log.c()) == 3 })
	if c := log.c(); !c[0] || c[1] || !c[2] {
		t.Errorf("уведомления %v, ждал [true false true]", c)
	}
	s.write(xlfRawEvent(2, 2))
	xlfWait(t, "сдвиг после возврата", func() bool { return len(log.m()) == 1 })
}

func TestX11PointerLock_Unlock(t *testing.T) {
	w, s, log := xlfLocked(t, nil)
	w.SetPointerLock(true)
	if !w.SetPointerLock(false) {
		t.Fatal("выключение вернуло false")
	}
	xlfWait(t, "UngrabPointer", func() bool { return s.count(isOp(x11OpUngrabPointer)) == 1 })
	xlfWait(t, "снятие подписки", func() bool { return s.count(isXI(xiMinorSelectEvents)) == 2 })
	// Подписка на сырое движение снята: пустая маска.
	var last []byte
	for _, r := range s.saw() {
		if isXI(xiMinorSelectEvents)(r) {
			last = r
		}
	}
	if last == nil || last[16]|last[17]|last[18]|last[19] != 0 {
		t.Errorf("подписка не снята: %v", last)
	}
	if c := log.c(); len(c) != 2 || c[1] {
		t.Errorf("уведомления %v, ждал [true false]", c)
	}
	// Фокус после выключения ничего не возвращает.
	s.write(xlfFocus(false))
	s.write(xlfFocus(true))
	time.Sleep(30 * time.Millisecond)
	if s.count(isOp(26)) != 1 {
		t.Errorf("GrabPointer после выключения: %d", s.count(isOp(26)))
	}
}

// Сервер не дал захватить указатель (его держит другой клиент): false, подписка
// снимается.
func TestX11PointerLock_GrabRefused(t *testing.T) {
	w, s, log := xlfLocked(t, func(s *xlfServer) { s.grabStatus = 1 }) // AlreadyGrabbed
	if w.SetPointerLock(true) {
		t.Error("захват включён при отказе сервера")
	}
	if len(log.c()) != 0 {
		t.Errorf("уведомления при отказе: %v", log.c())
	}
	xlfWait(t, "подписка и её снятие", func() bool { return s.count(isXI(xiMinorSelectEvents)) == 2 })
}

// Нет XInput2 — запасной путь: сдвиг считается возвратом указателя в центр.
func TestX11PointerLock_WarpFallbackWithoutXInput(t *testing.T) {
	w, s, log := xlfLocked(t, func(s *xlfServer) { s.xi = false })
	if !w.SetPointerLock(true) {
		t.Fatal("запасной путь не включился")
	}
	if s.count(isXI(xiMinorSelectEvents)) != 0 {
		t.Error("XISelectEvents без расширения")
	}
	xlfWait(t, "возврат при захвате", func() bool { return s.count(isOp(41)) >= 1 })
	base := s.count(isOp(41))
	s.write(xlfMotionNotify(410, 295))
	xlfWait(t, "сдвиг", func() bool { return len(log.m()) == 1 })
	if m := log.m()[0]; m != [2]float64{10, -5} {
		t.Errorf("сдвиг %v, ждал (10,-5) — отличие от центра", m)
	}
	xlfWait(t, "возврат", func() bool { return s.count(isOp(41)) == base+1 })
	// Движение, пришедшее от самого возврата (уже в центре), сдвига не несёт.
	s.write(xlfMotionNotify(400, 300))
	time.Sleep(20 * time.Millisecond)
	if len(log.m()) != 1 {
		t.Errorf("возврат в центр дал сдвиг: %v", log.m())
	}
}

// До запуска цикла событий захват не включить.
func TestX11PointerLock_NotBeforeLoop(t *testing.T) {
	w := &X11Window{wid: 1}
	if w.SetPointerLock(true) {
		t.Error("захват включён без цикла событий")
	}
	if !w.SetPointerLock(false) {
		t.Error("выключение без цикла вернуло false")
	}
}
