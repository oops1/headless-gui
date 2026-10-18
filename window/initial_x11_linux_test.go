//go:build linux && !android

package window

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"
)

// Начальное положение и «развёрнуто» на X11, на проводе (initial_x11_linux.go).
//
// Create настоящий, против поддельного X-сервера: он отвечает на то, на что
// Create ждёт ответ (атомы, раскладка, расширения, свойства), и записывает все
// запросы по порядку. Проверяется главное: положение и свойства окна уходят
// ДО MapWindow — менеджер читает их в момент показа.

const (
	xOpCreateWindow = 1
	xOpMapWindow    = 8
	xOpChangeProp   = 18
	xOpSendEvent    = 25

	xAtomWMNormalHints = 40 // предопределённый атом WM_NORMAL_HINTS
	xAtomWMSizeHints   = 41 // WM_SIZE_HINTS
)

// initX11Server — поддельный сервер для Create.
type initX11Server struct {
	mu    sync.Mutex
	reqs  []x11Request
	atoms map[string]uint32
	names map[uint32]string
}

func newInitX11Server() *initX11Server {
	return &initX11Server{atoms: map[string]uint32{}, names: map[uint32]string{}}
}

func (s *initX11Server) handle(r x11Request) []byte {
	s.mu.Lock()
	s.reqs = append(s.reqs, r)
	s.mu.Unlock()
	switch r.major {
	case 16: // InternAtom: у каждого имени свой номер выше предопределённых
		n := int(binary.LittleEndian.Uint16(r.raw[4:6]))
		name := string(r.raw[8 : 8+n])
		s.mu.Lock()
		id, ok := s.atoms[name]
		if !ok {
			id = uint32(1000 + len(s.atoms))
			s.atoms[name], s.names[id] = id, name
		}
		s.mu.Unlock()
		return x11Reply(0, func(h []byte) { binary.LittleEndian.PutUint32(h[8:12], id) }, nil)
	case 98: // QueryExtension: расширений нет
		return x11Reply(0, nil, nil)
	case 101: // GetKeyboardMapping: пустая таблица
		return x11Reply(0, nil, nil)
	case 20: // GetProperty: свойства нет
		return x11Reply(0, nil, nil)
	}
	return nil
}

func (s *initX11Server) requests() []x11Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]x11Request(nil), s.reqs...)
}

func (s *initX11Server) atom(name string) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.atoms[name]
}

// createInitX11 создаёт окно с заказанным началом и возвращает запросы,
// пришедшие до MapWindow включительно.
func createInitX11(t *testing.T, p initialPlacement, minW, minH, width, height int) (*X11Window, *initX11Server, []x11Request) {
	t.Helper()
	client, server := x11Pair(t)
	srv := newInitX11Server()
	go serveFakeX(server, srv.handle)

	w := &X11Window{
		conn: client, rootWin: tRoot,
		ridBase: 0x200000, ridMask: 0x1fffff,
	}
	w.screen.Root = tRoot
	w.screen.WidthInPixels, w.screen.HeightInPixels = 1920, 1080
	w.setInitialPlacement(p)
	if minW > 0 || minH > 0 {
		w.SetMinSize(minW, minH)
	}
	if err := w.Create("initial-x11", width, height); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Create вернулся — запросы ещё читаются сервером; ждём MapWindow.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, r := range srv.requests() {
			if r.major == xOpMapWindow {
				return w, srv, srv.requests()
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("MapWindow не пришёл")
	return nil, nil, nil
}

func indexOfReq(reqs []x11Request, pred func(r x11Request) bool) int {
	for i, r := range reqs {
		if pred(r) {
			return i
		}
	}
	return -1
}

func isChangeProp(r x11Request, property uint32) bool {
	return r.major == xOpChangeProp && binary.LittleEndian.Uint32(r.raw[8:12]) == property
}

// propWords — тело ChangeProperty (format 32) как слова.
func propWords(r x11Request) []uint32 {
	n := int(binary.LittleEndian.Uint32(r.raw[20:24]))
	out := make([]uint32, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(r.raw[24+i*4:])
	}
	return out
}

// Положение уходит в CreateWindow, а WM_NORMAL_HINTS объявляет его и размер
// менеджеру — до MapWindow.
func TestX11Initial_PositionAndHintsBeforeMap(t *testing.T) {
	_, _, reqs := createInitX11(t,
		initialPlacement{PosSet: true, X: 210, Y: 120, SizeSet: true}, 0, 0, 640, 480)

	ci := indexOfReq(reqs, func(r x11Request) bool { return r.major == xOpCreateWindow })
	mi := indexOfReq(reqs, func(r x11Request) bool { return r.major == xOpMapWindow })
	hi := indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, xAtomWMNormalHints) })
	if ci < 0 || mi < 0 {
		t.Fatalf("нет CreateWindow (%d) или MapWindow (%d)", ci, mi)
	}
	cw := reqs[ci].raw
	if x, y := int16(binary.LittleEndian.Uint16(cw[12:14])), int16(binary.LittleEndian.Uint16(cw[14:16])); x != 210 || y != 120 {
		t.Errorf("CreateWindow в (%d,%d), ждал (210,120), а не центр экрана", x, y)
	}
	if w, h := binary.LittleEndian.Uint16(cw[16:18]), binary.LittleEndian.Uint16(cw[18:20]); w != 640 || h != 480 {
		t.Errorf("CreateWindow %dx%d, ждал 640x480", w, h)
	}
	if hi < 0 {
		t.Fatal("WM_NORMAL_HINTS не выставлен")
	}
	if hi > mi {
		t.Errorf("WM_NORMAL_HINTS (запрос %d) после MapWindow (%d)", hi, mi)
	}
	hints := reqs[hi]
	if typ := binary.LittleEndian.Uint32(hints.raw[12:16]); typ != xAtomWMSizeHints {
		t.Errorf("тип свойства %d, ждал WM_SIZE_HINTS", typ)
	}
	words := propWords(hints)
	if len(words) != 18 {
		t.Fatalf("WM_SIZE_HINTS из %d слов, ждал 18", len(words))
	}
	if want := uint32(x11USPosition | x11PPosition | x11USSize | x11PSize); words[0] != want {
		t.Errorf("флаги %#x, ждал USPosition|PPosition|USSize|PSize %#x", words[0], want)
	}
	if int32(words[1]) != 210 || int32(words[2]) != 120 || words[3] != 640 || words[4] != 480 {
		t.Errorf("x,y,w,h в подсказке: %d %d %d %d", int32(words[1]), int32(words[2]), words[3], words[4])
	}
}

// Отрицательные координаты (монитор левее основного) доходят как есть.
func TestX11Initial_NegativePosition(t *testing.T) {
	w, _, reqs := createInitX11(t, initialPlacement{PosSet: true, X: -1600, Y: -200, SizeSet: true}, 0, 0, 640, 480)
	ci := indexOfReq(reqs, func(r x11Request) bool { return r.major == xOpCreateWindow })
	cw := reqs[ci].raw
	if x, y := int16(binary.LittleEndian.Uint16(cw[12:14])), int16(binary.LittleEndian.Uint16(cw[14:16])); x != -1600 || y != -200 {
		t.Errorf("CreateWindow в (%d,%d), ждал (-1600,-200)", x, y)
	}
	// GetPosition отвечает заказанным значением, пока менеджер не сообщил своё.
	if x, y := w.GetPosition(); x != -1600 || y != -200 {
		t.Errorf("GetPosition %d,%d", x, y)
	}
}

// Развёрнуто: _NET_WM_STATE со значениями MAXIMIZED_VERT/HORZ ставится
// свойством ДО MapWindow, а не ClientMessage'ем после.
func TestX11Initial_MaximizedStatePropertyBeforeMap(t *testing.T) {
	w, srv, reqs := createInitX11(t, initialPlacement{Maximized: true}, 0, 0, 640, 480)

	state, maxH, maxV := srv.atom("_NET_WM_STATE"), srv.atom("_NET_WM_STATE_MAXIMIZED_HORZ"), srv.atom("_NET_WM_STATE_MAXIMIZED_VERT")
	if state == 0 || maxH == 0 || maxV == 0 {
		t.Fatalf("атомы не интернированы: %d %d %d", state, maxH, maxV)
	}
	si := indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, state) })
	mi := indexOfReq(reqs, func(r x11Request) bool { return r.major == xOpMapWindow })
	if si < 0 {
		t.Fatal("_NET_WM_STATE не выставлен")
	}
	if si > mi {
		t.Errorf("_NET_WM_STATE (запрос %d) после MapWindow (%d)", si, mi)
	}
	r := reqs[si]
	if typ := binary.LittleEndian.Uint32(r.raw[12:16]); typ != 4 {
		t.Errorf("тип свойства %d, ждал ATOM (4)", typ)
	}
	if r.raw[16] != 32 {
		t.Errorf("формат %d, ждал 32", r.raw[16])
	}
	got := map[uint32]bool{}
	for _, a := range propWords(r) {
		got[a] = true
	}
	if !got[maxH] || !got[maxV] || len(got) != 2 {
		t.Errorf("значения %v, ждал MAXIMIZED_HORZ и MAXIMIZED_VERT", propWords(r))
	}
	// Ни одного ClientMessage _NET_WM_STATE: для ещё не показанного окна он
	// не годится.
	if i := indexOfReq(reqs, func(r x11Request) bool { return r.major == xOpSendEvent }); i >= 0 {
		t.Errorf("запрос %d: SendEvent при создании окна", i)
	}
	if !w.IsMaximized() {
		t.Error("IsMaximized false сразу после создания развёрнутого окна")
	}
	// Положение не просили — свойства положения нет, окно в центре.
	if indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, xAtomWMNormalHints) }) >= 0 {
		t.Error("WM_NORMAL_HINTS выставлен без заказа положения и минимума")
	}
}

// Границы вместе с «развёрнуто»: и подсказки положения, и состояние — до показа.
func TestX11Initial_BoundsAndMaximized(t *testing.T) {
	_, srv, reqs := createInitX11(t,
		initialPlacement{PosSet: true, X: 50, Y: 60, SizeSet: true, Maximized: true}, 0, 0, 700, 500)
	mi := indexOfReq(reqs, func(r x11Request) bool { return r.major == xOpMapWindow })
	hi := indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, xAtomWMNormalHints) })
	si := indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, srv.atom("_NET_WM_STATE")) })
	if hi < 0 || si < 0 || hi > mi || si > mi {
		t.Fatalf("подсказки (%d) и состояние (%d) должны предшествовать MapWindow (%d)", hi, si, mi)
	}
}

// Минимальный размер, заданный до Create, не теряется: подсказки несут и
// минимум, и положение одним свойством (оно заменяется целиком).
func TestX11Initial_HintsKeepMinSize(t *testing.T) {
	_, _, reqs := createInitX11(t,
		initialPlacement{PosSet: true, X: 5, Y: 6, SizeSet: true}, 320, 200, 640, 480)

	n := 0
	var words []uint32
	for _, r := range reqs {
		if isChangeProp(r, xAtomWMNormalHints) {
			n++
			words = propWords(r)
		}
	}
	if n != 1 {
		t.Fatalf("WM_NORMAL_HINTS выставлен %d раз, ждал один (второй затёр бы первый)", n)
	}
	if words[0]&x11PMinSize == 0 || words[5] != 320 || words[6] != 200 {
		t.Errorf("минимум потерян: флаги %#x, min %d×%d", words[0], words[5], words[6])
	}
	if words[0]&x11USPosition == 0 {
		t.Errorf("положение потеряно: флаги %#x", words[0])
	}
}

// Ничего не заказано: окно по центру, свойства те же, что раньше (без минимума
// WM_NORMAL_HINTS не пишется вовсе).
func TestX11Initial_NothingRequestedKeepsOldBehaviour(t *testing.T) {
	w, srv, reqs := createInitX11(t, initialPlacement{}, 0, 0, 640, 480)
	ci := indexOfReq(reqs, func(r x11Request) bool { return r.major == xOpCreateWindow })
	cw := reqs[ci].raw
	if x, y := int16(binary.LittleEndian.Uint16(cw[12:14])), int16(binary.LittleEndian.Uint16(cw[14:16])); x != (1920-640)/2 || y != (1080-480)/2 {
		t.Errorf("CreateWindow в (%d,%d), ждал центр", x, y)
	}
	if indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, xAtomWMNormalHints) }) >= 0 {
		t.Error("WM_NORMAL_HINTS выставлен без заказа")
	}
	if indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, srv.atom("_NET_WM_STATE")) }) >= 0 {
		t.Error("_NET_WM_STATE выставлен без заказа")
	}
	if w.IsMaximized() {
		t.Error("обычное окно считается развёрнутым")
	}
}

// Минимум без положения — прежний путь: WM_NORMAL_HINTS только с PMinSize.
func TestX11Initial_MinSizeOnlyUnchanged(t *testing.T) {
	_, _, reqs := createInitX11(t, initialPlacement{}, 320, 200, 640, 480)
	i := indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, xAtomWMNormalHints) })
	if i < 0 {
		t.Fatal("минимум не выставлен")
	}
	words := propWords(reqs[i])
	if words[0] != x11PMinSize {
		t.Errorf("флаги %#x, ждал один PMinSize", words[0])
	}
}

// Только положение (размер у движка): объявляется USPosition|PPosition, но не USSize.
func TestX11Initial_PositionWithoutSize(t *testing.T) {
	_, _, reqs := createInitX11(t, initialPlacement{PosSet: true, X: 11, Y: 22}, 0, 0, 640, 480)
	i := indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, xAtomWMNormalHints) })
	if i < 0 {
		t.Fatal("подсказки не выставлены")
	}
	if got := propWords(reqs[i])[0]; got != x11USPosition|x11PPosition {
		t.Errorf("флаги %#x, ждал USPosition|PPosition", got)
	}
}

func TestX11InitialStateData(t *testing.T) {
	b := x11InitialStateData(7, 9)
	if len(b) != 8 || binary.LittleEndian.Uint32(b[0:4]) != 9 || binary.LittleEndian.Uint32(b[4:8]) != 7 {
		t.Errorf("% x", b)
	}
}

func TestX11ClampInt16(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want int16
	}{{0, 0}, {100, 100}, {-100, -100}, {40000, 32767}, {-40000, -32768}} {
		if got := x11ClampInt16(tc.in); got != tc.want {
			t.Errorf("x11ClampInt16(%d) = %d, ждал %d", tc.in, got, tc.want)
		}
	}
}
