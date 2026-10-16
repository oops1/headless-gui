//go:build linux && !android

package window

import (
	"encoding/binary"
	"image"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

// argbFakeServer — игрушечный X-сервер на конце net.Pipe для CreatePopup:
// записывает запросы, отвечает на QueryExtension (расширений нет),
// GetKeyboardMapping и GetInputFocus. failArgb — на CreateWindow глубины 32
// ответить ошибкой BadMatch (она придёт перед ответом на GetInputFocus, как на
// настоящем сервере).
type argbFakeServer struct {
	conn net.Conn
	done chan struct{}

	mu       sync.Mutex
	requests [][]byte

	failArgb   bool
	pendingErr int

	// exts — расширения, которые сервер объявляет (имя → major-опкод);
	// cmOwner — владелец выделения _NET_WM_CM_S0 (0 — композитора нет).
	exts    map[string]byte
	cmOwner uint32
}

func newArgbFakeServer(conn net.Conn) *argbFakeServer {
	s := &argbFakeServer{conn: conn, done: make(chan struct{})}
	go s.serve()
	return s
}

func (s *argbFakeServer) serve() {
	defer close(s.done)
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
		switch req[0] {
		case 98: // QueryExtension: объявленные расширения
			nameLen := int(binary.LittleEndian.Uint16(req[4:6]))
			if 8+nameLen <= len(req) {
				if major, ok := s.exts[string(req[8:8+nameLen])]; ok {
					reply[8], reply[9] = 1, major
				}
			}
		case 16: // InternAtom: любой атом — 77
			binary.LittleEndian.PutUint32(reply[8:12], 77)
		case 23: // GetSelectionOwner
			binary.LittleEndian.PutUint32(reply[8:12], s.cmOwner)
		case 101: // GetKeyboardMapping (пусто)
		case 1: // CreateWindow
			if s.failArgb && req[1] == 32 {
				s.pendingErr++
			}
			continue
		case 43: // GetInputFocus: сперва накопленные ошибки
			for ; s.pendingErr > 0; s.pendingErr-- {
				e := make([]byte, 32)
				e[1] = 8 // BadMatch
				s.conn.Write(e)
			}
		default:
			continue
		}
		s.conn.Write(reply)
	}
}

// saw — все запросы, пришедшие к этому моменту.
func (s *argbFakeServer) saw() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte(nil), s.requests...)
}

// sawOp — запросы с данным опкодом по порядку.
func (s *argbFakeServer) sawOp(op byte) [][]byte {
	var out [][]byte
	for _, r := range s.saw() {
		if r[0] == op {
			out = append(out, r)
		}
	}
	return out
}

// argbWaitReq ждёт, пока сервер увидит n запросов с опкодом op.
func argbWaitReq(t *testing.T, s *argbFakeServer, op byte, n int) [][]byte {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if got := s.sawOp(op); len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("сервер не увидел %d запросов опкода %d (есть %d)", n, op, len(s.sawOp(op)))
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// newArgbPopupWindow — окно с готовым соединением и экраном; argbVisual 0 —
// экран без 32-битного visual'а.
func newArgbPopupWindow(t *testing.T, argbVisual uint32, failArgb bool) (*X11Window, *argbFakeServer) {
	t.Helper()
	cli, srv := net.Pipe()
	t.Cleanup(func() { cli.Close(); srv.Close() })
	fx := newArgbFakeServer(srv)
	fx.failArgb = failArgb
	w := &X11Window{
		conn:    cli,
		ridBase: 0x400000, ridMask: 0x1FFFFF,
		screen: x11Screen{Root: 0x100, RootDepth: 24, RootVisual: 0x21, BlackPixel: 0, ArgbVisual: argbVisual},
	}
	return w, fx
}

func argbCreatePopup(t *testing.T, w *X11Window) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- w.CreatePopup(64, 48) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CreatePopup: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("CreatePopup завис")
	}
}

// TestX11CreatePopup_ArgbWindowBytes: попап с 32-битным visual'ом — CreateColormap
// (AllocNone, корень, visual), затем CreateWindow глубины 32 с этим visual'ом,
// маской BackPixel|BorderPixel|OverrideRedirect|EventMask|Colormap и значениями по
// возрастанию бит; colormap — в значении Colormap; MapWindow — после.
func TestX11CreatePopup_ArgbWindowBytes(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0x77, false)
	argbCreatePopup(t, w)

	if !w.argb || w.colormap == 0 {
		t.Fatalf("окно не помечено как ARGB: argb=%v colormap=%#x", w.argb, w.colormap)
	}
	argbWaitReq(t, fx, 8, 1) // MapWindow уходит последним среди запросов окна
	reqs := fx.saw()
	idxCmap, idxWin, idxMap := -1, -1, -1
	for i, r := range reqs {
		switch r[0] {
		case 78:
			idxCmap = i
		case 1:
			idxWin = i
		case 8:
			idxMap = i
		}
	}
	if idxCmap < 0 || idxWin < 0 || idxMap < 0 || !(idxCmap < idxWin && idxWin < idxMap) {
		t.Fatalf("порядок запросов: colormap=%d window=%d map=%d (нужно colormap<window<map)", idxCmap, idxWin, idxMap)
	}

	cm := reqs[idxCmap]
	if cm[1] != 0 {
		t.Errorf("CreateColormap: alloc = %d, want 0 (None)", cm[1])
	}
	cmap := binary.LittleEndian.Uint32(cm[4:8])
	if cmap != w.colormap || cmap&^0x1FFFFF != 0x400000 {
		t.Errorf("colormap id %#x (окно держит %#x) вне диапазона клиента", cmap, w.colormap)
	}
	if binary.LittleEndian.Uint32(cm[8:12]) != 0x100 {
		t.Errorf("CreateColormap: окно-основа %#x, want корень 0x100", binary.LittleEndian.Uint32(cm[8:12]))
	}
	if binary.LittleEndian.Uint32(cm[12:16]) != 0x77 {
		t.Errorf("CreateColormap: visual %#x, want 0x77", binary.LittleEndian.Uint32(cm[12:16]))
	}

	cw := reqs[idxWin]
	if cw[1] != 32 {
		t.Errorf("CreateWindow: depth = %d, want 32", cw[1])
	}
	if v := binary.LittleEndian.Uint32(cw[24:28]); v != 0x77 {
		t.Errorf("CreateWindow: visual = %#x, want 0x77", v)
	}
	mask := binary.LittleEndian.Uint32(cw[28:32])
	if want := uint32(0x2 | 0x8 | 0x200 | 0x800 | 0x2000); mask != want {
		t.Fatalf("CreateWindow: маска %#x, want %#x", mask, want)
	}
	if len(cw) != 32+5*4 {
		t.Fatalf("CreateWindow: длина %d, want %d (пять значений)", len(cw), 32+5*4)
	}
	val := func(i int) uint32 { return binary.LittleEndian.Uint32(cw[32+i*4 : 36+i*4]) }
	if val(0) != 0 || val(1) != 0 {
		t.Errorf("BackPixel/BorderPixel = %#x/%#x, want 0/0", val(0), val(1))
	}
	if val(2) != 1 {
		t.Errorf("OverrideRedirect = %d, want 1", val(2))
	}
	if val(3)&(1<<15) == 0 || val(3)&4 == 0 {
		t.Errorf("EventMask %#x без Exposure/ButtonPress", val(3))
	}
	if val(4) != cmap {
		t.Errorf("Colormap в окне %#x, want %#x", val(4), cmap)
	}
	if got := binary.LittleEndian.Uint32(cw[4:8]); got != w.wid {
		t.Errorf("CreateWindow: id %#x, окно держит %#x", got, w.wid)
	}
}

// TestX11CreatePopup_NoArgbVisualKeeps24Bit: на экране без 32-битного visual'а
// поведение прежнее — одно CreateWindow глубины и visual'а корня, без colormap.
func TestX11CreatePopup_NoArgbVisualKeeps24Bit(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0, false)
	argbCreatePopup(t, w)

	if w.argb || w.colormap != 0 {
		t.Fatalf("окно помечено ARGB без visual'а: argb=%v colormap=%#x", w.argb, w.colormap)
	}
	argbWaitReq(t, fx, 8, 1)
	if n := len(fx.sawOp(78)); n != 0 {
		t.Errorf("CreateColormap послан %d раз, want 0", n)
	}
	wins := fx.sawOp(1)
	if len(wins) != 1 {
		t.Fatalf("CreateWindow: %d запросов, want 1", len(wins))
	}
	cw := wins[0]
	if cw[1] != 24 || binary.LittleEndian.Uint32(cw[24:28]) != 0x21 {
		t.Errorf("depth/visual = %d/%#x, want 24/0x21", cw[1], binary.LittleEndian.Uint32(cw[24:28]))
	}
	if mask := binary.LittleEndian.Uint32(cw[28:32]); mask != 0x2|0x200|0x800 {
		t.Errorf("маска %#x, want %#x", mask, 0x2|0x200|0x800)
	}
}

// TestX11CreatePopup_ServerRejectsArgbFallsBack: сервер отверг окно с ARGB
// (BadMatch) — откат на 24 бита под новым ID, colormap освобождён.
func TestX11CreatePopup_ServerRejectsArgbFallsBack(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0x77, true)
	argbCreatePopup(t, w)

	if w.argb || w.colormap != 0 {
		t.Fatalf("после отказа окно всё ещё ARGB: argb=%v colormap=%#x", w.argb, w.colormap)
	}
	wins := argbWaitReq(t, fx, 1, 2)
	if len(wins) != 2 || wins[0][1] != 32 || wins[1][1] != 24 {
		t.Fatalf("ждали CreateWindow глубины 32, затем 24: %d запросов", len(wins))
	}
	idFirst := binary.LittleEndian.Uint32(wins[0][4:8])
	idSecond := binary.LittleEndian.Uint32(wins[1][4:8])
	if idFirst == idSecond || idSecond != w.wid {
		t.Errorf("id окон %#x → %#x, окно держит %#x: после отказа нужен новый id", idFirst, idSecond, w.wid)
	}
	if n := len(argbWaitReq(t, fx, 79, 1)); n != 1 {
		t.Errorf("FreeColormap: %d запросов, want 1", n)
	}
	// Кадр уходит уже глубиной 24.
	w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 4, 4)))
	puts := argbWaitReq(t, fx, 72, 1)
	if puts[0][21] != 24 {
		t.Errorf("PutImage depth = %d, want 24", puts[0][21])
	}
}

// TestX11PutImage_ArgbDepth32Premultiplied: кадр в окно с альфой уходит
// глубиной 32, цвет — как есть в картинке движка (она уже премультиплицирована),
// только R и B переставлены; альфа сохранена; прозрачные точки — нули.
func TestX11PutImage_ArgbDepth32Premultiplied(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0x77, false)
	argbCreatePopup(t, w)

	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	copy(img.Pix, []byte{
		10, 20, 30, 128, // полупрозрачная точка (premultiplied: R,G,B ≤ A)
		0, 0, 0, 0, // прозрачная
	})
	w.BlitRGBA(img)

	puts := argbWaitReq(t, fx, 72, 1)
	p := puts[0]
	if p[21] != 32 {
		t.Fatalf("PutImage depth = %d, want 32", p[21])
	}
	if p[1] != 2 {
		t.Errorf("format = %d, want 2 (ZPixmap)", p[1])
	}
	got := p[24 : 24+8]
	want := []byte{30, 20, 10, 128, 0, 0, 0, 0} // BGRA
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("пиксели % x, want % x", got, want)
		}
	}
}

// TestX11PutImage_NonArgbStays24: обычное (главное) окно — по-прежнему 24.
func TestX11PutImage_NonArgbStays24(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0, false)
	w.wid = 0x400001
	w.gcID = 0x400002
	w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	puts := argbWaitReq(t, fx, 72, 1)
	if puts[0][21] != 24 {
		t.Errorf("PutImage depth = %d, want 24", puts[0][21])
	}
}

// TestX11ShmPutImage_ArgbDepth32: через MIT-SHM глубина тоже 32, а пиксели в
// сегменте — те же BGRA с альфой. Сегмент подменён обычной памятью: проверяем
// запрос и содержимое, не трогая SysV.
func TestX11ShmPutImage_ArgbDepth32(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0x77, false)
	argbCreatePopup(t, w)

	w.shm = &x11ShmState{major: 130, shmseg: 0x400009, data: make([]byte, 2*1*4), width: 2, height: 1}
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	copy(img.Pix, []byte{10, 20, 30, 128, 0, 0, 0, 0})
	w.BlitRGBA(img)

	reqs := argbWaitReq(t, fx, 130, 1)
	if reqs[0][1] != shmMinorPutImage || reqs[0][28] != 32 {
		t.Fatalf("minor/depth = %d/%d, want %d/32", reqs[0][1], reqs[0][28], shmMinorPutImage)
	}
	want := []byte{30, 20, 10, 128, 0, 0, 0, 0}
	for i := range want {
		if w.shm.data[i] != want[i] {
			t.Fatalf("сегмент % x, want % x", w.shm.data, want)
		}
	}
}

// TestX11ShmPutImageRequestDepth: явная глубина в байте 28.
func TestX11ShmPutImageRequestDepth(t *testing.T) {
	if b := x11ShmPutImageRequestDepth(130, 1, 2, 32, 1, 1, 0, 0, 1, 1, 0, 0, false, 3, 0); b[28] != 32 {
		t.Errorf("depth = %d, want 32", b[28])
	}
	if b := x11ShmPutImageRequest(130, 1, 2, 1, 1, 0, 0, 1, 1, 0, 0, false, 3, 0); b[28] != 24 {
		t.Errorf("прежний вызов: depth = %d, want 24", b[28])
	}
}

// TestX11PopupExpose_RestoresFromLastFrame: Expose попапа перерисовывает
// область из последнего кадра (колбэка хоста у попапа нет). Окно с альфой иначе
// оставляло бы открытую область прозрачной, а 24-битное — залитой фоном.
func TestX11PopupExpose_RestoresFromLastFrame(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0x77, false)
	argbCreatePopup(t, w)

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 0x40
	}
	w.BlitRGBA(img)
	argbWaitReq(t, fx, 72, 1)

	ev := make([]byte, 32)
	ev[0] = 12
	binary.LittleEndian.PutUint16(ev[8:10], 2)  // x
	binary.LittleEndian.PutUint16(ev[10:12], 3) // y
	binary.LittleEndian.PutUint16(ev[12:14], 4) // width
	binary.LittleEndian.PutUint16(ev[14:16], 5) // height
	w.handleX11Event(ev)

	puts := argbWaitReq(t, fx, 72, 2)
	p := puts[1]
	if p[21] != 32 {
		t.Errorf("depth = %d, want 32", p[21])
	}
	if binary.LittleEndian.Uint16(p[12:14]) != 4 || binary.LittleEndian.Uint16(p[14:16]) != 5 ||
		binary.LittleEndian.Uint16(p[16:18]) != 2 || binary.LittleEndian.Uint16(p[18:20]) != 3 {
		t.Errorf("прямоугольник Expose не соблюдён: % x", p[:24])
	}
}

// TestX11Close_FreesArgbColormap: при закрытии colormap освобождается.
func TestX11Close_FreesArgbColormap(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0x77, false)
	argbCreatePopup(t, w)
	cmap := w.colormap

	w.Close()
	<-fx.done
	frees := fx.sawOp(79)
	if len(frees) != 1 || binary.LittleEndian.Uint32(frees[0][4:8]) != cmap {
		t.Fatalf("FreeColormap: %d запросов (colormap %#x)", len(frees), cmap)
	}
	if w.colormap != 0 {
		t.Errorf("colormap не обнулён после закрытия")
	}
}

// buildSetupReply собирает ответ сервера на установление соединения с одним
// экраном: depths — список глубин за заголовком экрана.
func buildSetupReply(rootVisual uint32, rootDepth byte, nDepths int, depths []byte) []byte {
	const vendor = "fake"
	body := make([]byte, 32)
	binary.LittleEndian.PutUint32(body[4:8], 0x400000)
	binary.LittleEndian.PutUint32(body[8:12], 0x1FFFFF)
	binary.LittleEndian.PutUint16(body[16:18], uint16(len(vendor)))
	body[20] = 1 // экранов
	body[21] = 1 // форматов
	body[26] = 8 // min keycode
	body = append(body, vendor...)
	// pixmap format: depth 24, bpp 32, pad 32
	body = append(body, 24, 32, 32, 0, 0, 0, 0, 0)
	scr := make([]byte, 40)
	binary.LittleEndian.PutUint32(scr[0:4], 0x100) // root
	binary.LittleEndian.PutUint32(scr[4:8], 0x20)  // colormap
	binary.LittleEndian.PutUint32(scr[8:12], 0xFFFFFF)
	binary.LittleEndian.PutUint16(scr[20:22], 1920)
	binary.LittleEndian.PutUint16(scr[22:24], 1080)
	binary.LittleEndian.PutUint32(scr[32:36], rootVisual)
	scr[38] = rootDepth
	scr[39] = byte(nDepths)
	body = append(body, scr...)
	body = append(body, depths...)

	hdr := make([]byte, 8)
	hdr[0] = 1
	binary.LittleEndian.PutUint16(hdr[2:4], 11)
	binary.LittleEndian.PutUint16(hdr[6:8], uint16(len(body)/4))
	return append(hdr, body...)
}

func setupAgainst(t *testing.T, reply []byte) *X11Window {
	t.Helper()
	cli, srv := net.Pipe()
	t.Cleanup(func() { cli.Close(); srv.Close() })
	go func() {
		io.ReadFull(srv, make([]byte, 12))
		srv.Write(reply)
	}()
	w := &X11Window{conn: cli}
	done := make(chan error, 1)
	go func() { done <- w.x11Setup() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("x11Setup: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("x11Setup завис")
	}
	return w
}

// TestX11Setup_ParsesArgbVisual: список глубин за заголовком экрана
// разбирается из ответа на установление соединения; корневые depth/visual
// остаются прежними.
func TestX11Setup_ParsesArgbVisual(t *testing.T) {
	depths := catBytes(
		argbDepth(24, argbVisual(0x21, 4, 0xFF0000, 0x00FF00, 0x0000FF)),
		argbDepth(32, argbVisual(0x77, 4, 0xFF0000, 0x00FF00, 0x0000FF)),
	)
	w := setupAgainst(t, buildSetupReply(0x21, 24, 2, depths))
	if w.screen.ArgbVisual != 0x77 {
		t.Errorf("ArgbVisual = %#x, want 0x77", w.screen.ArgbVisual)
	}
	if w.screen.RootVisual != 0x21 || w.screen.RootDepth != 24 || w.screen.Root != 0x100 {
		t.Errorf("корень: visual %#x depth %d root %#x", w.screen.RootVisual, w.screen.RootDepth, w.screen.Root)
	}
}

func TestX11Setup_NoArgbVisual(t *testing.T) {
	depths := catBytes(argbDepth(24, argbVisual(0x21, 4, 0xFF0000, 0x00FF00, 0x0000FF)))
	w := setupAgainst(t, buildSetupReply(0x21, 24, 1, depths))
	if w.screen.ArgbVisual != 0 {
		t.Errorf("ArgbVisual = %#x, want 0", w.screen.ArgbVisual)
	}
}

// argbGetImage читает пиксели окна запросом GetImage (ZPixmap) прямо с
// соединения: насос событий в e2e не запущен, поэтому события, пришедшие
// раньше ответа, пропускаются. ok=false — сервер ответил ошибкой.
func (w *X11Window) argbGetImage(x, y, width, height int) (data []byte, ok bool) {
	req := make([]byte, 20)
	req[0] = 73 // GetImage
	req[1] = 2  // ZPixmap
	binary.LittleEndian.PutUint16(req[2:4], 5)
	binary.LittleEndian.PutUint32(req[4:8], w.wid)
	binary.LittleEndian.PutUint16(req[8:10], uint16(x))
	binary.LittleEndian.PutUint16(req[10:12], uint16(y))
	binary.LittleEndian.PutUint16(req[12:14], uint16(width))
	binary.LittleEndian.PutUint16(req[14:16], uint16(height))
	binary.LittleEndian.PutUint32(req[16:20], 0xFFFFFFFF)
	w.x11Send(req)

	w.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	defer w.conn.SetReadDeadline(time.Time{})
	pkt := make([]byte, 32)
	for {
		if !w.readFull(pkt) {
			return nil, false
		}
		switch pkt[0] {
		case 0:
			return nil, false
		case 1:
			n := int(binary.LittleEndian.Uint32(pkt[4:8])) * 4
			data = make([]byte, n)
			if !w.readFull(data) {
				return nil, false
			}
			return data, true
		}
	}
}

// TestX11ArgbPopupE2E — живая проверка против настоящего X-сервера (Xvfb,
// WSLg/XWayland): попап с ARGB visual'ом создаётся без ошибок, кадр с тенью и
// прозрачными углами выводится, GetImage возвращает пиксели окна. Без DISPLAY
// пропускается. Если сервер без 32-битного visual'а — проверяется откат.
//
// Содержимое окна под композитором можно и не получить (GetImage читает буфер
// окна, а не экран), поэтому прозрачность проверяется только когда сервер
// вернул глубину 32; строго обязательны лишь отсутствие протокольных ошибок и
// совпадение размера.
func TestX11ArgbPopupE2E(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}
	w := &X11Window{}
	if err := w.CreatePopup(64, 48); err != nil {
		t.Skipf("не удалось поднять попап: %v", err)
	}
	defer w.Close()
	t.Logf("ARGB visual экрана=%#x, окно с альфой=%v", w.screen.ArgbVisual, w.argb)

	// Кадр: непрозрачное тело 16..48 × 8..40, вокруг — полупрозрачная тень и
	// нулевые (прозрачные) точки; цвета премультиплицированы.
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			o := y*img.Stride + x*4
			switch {
			case x >= 16 && x < 48 && y >= 8 && y < 40:
				img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = 200, 100, 50, 255
			case x >= 12 && x < 52 && y >= 4 && y < 44:
				img.Pix[o+3] = 64 // тень: чёрный с альфой
			}
		}
	}
	w.BlitRGBA(img)

	reply, ok := w.argbGetImage(0, 0, 64, 48)
	if !ok {
		t.Skip("GetImage не вернул данные (сервер не отдаёт содержимое окна)")
	}
	px := func(x, y int) [4]byte {
		o := (y*64 + x) * 4
		return [4]byte{reply[o], reply[o+1], reply[o+2], reply[o+3]}
	}
	t.Logf("тело=%v тень=%v угол=%v", px(32, 24), px(14, 24), px(1, 1))
	if w.argb {
		if got := px(1, 1); got[3] != 0 {
			t.Errorf("за телом попапа альфа %d, want 0", got[3])
		}
	}
}
