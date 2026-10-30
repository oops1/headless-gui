//go:build linux && !android

package x11

// Стиль, владелец, Z-порядок и положение X11 на проводе (x11_style_linux.go):
// до MapWindow — свойства, у показанного окна — ClientMessage _NET_WM_STATE и
// смена свойств.

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget/base"
)

const (
	xOpDeleteProp  = 19
	xOpConfigureWn = 12

	tAtomType     = 310 // _NET_WM_WINDOW_TYPE
	tAtomNormal   = 311 // _NET_WM_WINDOW_TYPE_NORMAL
	tAtomSkipTB   = 313 // _NET_WM_STATE_SKIP_TASKBAR
	tAtomSkipPg   = 314 // _NET_WM_STATE_SKIP_PAGER
	tAtomSkipSw   = 315 // _KDE_NET_WM_STATE_SKIP_SWITCHER
	tAtomFrameExt = 316 // _GTK_FRAME_EXTENTS
	tAtomMoveRes  = 317 // _NET_WM_MOVERESIZE
)

// newStyleX11 — созданное окно на поддельном сервере с атомами стиля.
func newStyleX11(t *testing.T) (*X11Window, net.Conn) {
	t.Helper()
	w, server := newFsX11(t)
	w.style.atomType, w.style.atomTypeNormal = tAtomType, tAtomNormal
	w.style.atomSkipTaskbar, w.style.atomSkipPager, w.style.atomSkipSwitcher = tAtomSkipTB, tAtomSkipPg, tAtomSkipSw
	w.style.atomFrameExtents = tAtomFrameExt
	return w, server
}

// readReq читает с сервера один запрос клиента целиком.
func readReq(t *testing.T, c net.Conn) x11Request {
	t.Helper()
	h := readN(t, c, 4)
	size := int(binary.LittleEndian.Uint16(h[2:4])) * 4
	raw := append(h, readN(t, c, size-4)...)
	return x11Request{major: raw[0], data: raw[1], raw: raw}
}

// noReq — клиент больше ничего не прислал.
func noReq(t *testing.T, c net.Conn) {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	defer c.SetReadDeadline(time.Time{})
	b := make([]byte, 1)
	if n, _ := c.Read(b); n > 0 {
		t.Fatalf("лишний запрос, опкод %d", b[0])
	}
}

// wantStateMsg — запрос: ClientMessage _NET_WM_STATE с action и prop.
func wantStateMsg(t *testing.T, r x11Request, action, prop uint32) {
	t.Helper()
	le := binary.LittleEndian
	if r.major != xOpSendEvent || len(r.raw) != 44 || r.raw[12] != 33 {
		t.Fatalf("не SendEvent ClientMessage: опкод %d, %d байт", r.major, len(r.raw))
	}
	msg := r.raw[12:]
	if le.Uint32(msg[8:12]) != tAtomState || le.Uint32(msg[12:16]) != action || le.Uint32(msg[16:20]) != prop {
		t.Fatalf("тип %d, действие %d, свойство %d; ждал действие %d свойства %d",
			le.Uint32(msg[8:12]), le.Uint32(msg[12:16]), le.Uint32(msg[16:20]), action, prop)
	}
}

// wantProp — запрос: ChangeProperty prop со списком атомов want.
func wantProp(t *testing.T, r x11Request, prop uint32, want ...uint32) {
	t.Helper()
	if !isChangeProp(r, prop) {
		t.Fatalf("не ChangeProperty %d: опкод %d", prop, r.major)
	}
	if got := propWords(r); !equalWords(got, want) {
		t.Fatalf("свойство %d = %v, ждал %v", prop, got, want)
	}
}

func equalWords(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// createStyleX11 создаёт окно на поддельном сервере; before — то, что задаётся
// до Create. Возвращает запросы до MapWindow включительно.
func createStyleX11(t *testing.T, p native.InitialPlacement, before func(w *X11Window)) (*initX11Server, []x11Request) {
	t.Helper()
	client, server := x11Pair(t)
	srv := newInitX11Server()
	go serveFakeX(server, srv.handle)
	w := &X11Window{conn: client, rootWin: tRoot, ridBase: 0x200000, ridMask: 0x1fffff}
	w.screen.Root = tRoot
	w.screen.WidthInPixels, w.screen.HeightInPixels = 1920, 1080
	w.SetInitialPlacement(p)
	if before != nil {
		before(w)
	}
	if err := w.Create("style-x11", 640, 480); err != nil {
		t.Fatalf("Create: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		reqs := srv.requests()
		if i := indexOfReq(reqs, func(r x11Request) bool { return r.major == xOpMapWindow }); i >= 0 {
			return srv, reqs[:i+1]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("MapWindow не пришёл")
	return nil, nil
}

// propBeforeMap — значение свойства, поставленного до MapWindow (ok=false — не
// ставилось).
func propBeforeMap(reqs []x11Request, prop uint32) ([]uint32, bool) {
	i := indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, prop) })
	if i < 0 {
		return nil, false
	}
	return propWords(reqs[i]), true
}

// Обычное окно (и модальный диалог, которому владельца ставит SetOwner после
// показа) не получает ни типа, ни начальных состояний: поведение прежнее.
func TestX11Style_PlainWindowUntouched(t *testing.T) {
	srv, reqs := createStyleX11(t, native.InitialPlacement{}, nil)
	if v, ok := propBeforeMap(reqs, srv.atom("_NET_WM_WINDOW_TYPE")); ok {
		t.Errorf("обычному окну поставлен тип %v", v)
	}
	if v, ok := propBeforeMap(reqs, srv.atom("_NET_WM_STATE")); ok {
		t.Errorf("обычному окну поставлено состояние %v", v)
	}
	if _, ok := propBeforeMap(reqs, xAtomWMTransientFor); ok {
		t.Error("обычному окну поставлен WM_TRANSIENT_FOR")
	}
	if _, ok := propBeforeMap(reqs, srv.atom("_GTK_FRAME_EXTENTS")); ok {
		t.Error("обычному окну поставлен _GTK_FRAME_EXTENTS: композитор снял бы его тень")
	}
}

// Borderless до Create: нулевые _GTK_FRAME_EXTENTS до MapWindow — композитор
// не рисует свою тень.
func TestX11Style_BorderlessBeforeMap(t *testing.T) {
	srv, reqs := createStyleX11(t, native.InitialPlacement{}, func(w *X11Window) {
		w.ApplyStyle(native.Style{Borderless: true})
	})
	v, ok := propBeforeMap(reqs, srv.atom("_GTK_FRAME_EXTENTS"))
	if !ok || !equalWords(v, []uint32{0, 0, 0, 0}) {
		t.Errorf("_GTK_FRAME_EXTENTS %v (поставлено %v), ждал четыре нуля", v, ok)
	}
	i := indexOfReq(reqs, func(r x11Request) bool { return isChangeProp(r, srv.atom("_GTK_FRAME_EXTENTS")) })
	if i >= 0 && binary.LittleEndian.Uint32(reqs[i].raw[12:16]) != xAtomCardinal {
		t.Error("_GTK_FRAME_EXTENTS не типа CARDINAL")
	}
	if _, ok := propBeforeMap(reqs, srv.atom("_NET_WM_WINDOW_TYPE")); ok {
		t.Error("Borderless без владельца получил тип окна")
	}
}

// Tool до Create: SKIP-состояния — свойством до MapWindow; тип окна не
// меняется (UTILITY у mutter лишает окно фокуса, у KWin прячет его).
func TestX11Style_ToolBeforeMap(t *testing.T) {
	srv, reqs := createStyleX11(t, native.InitialPlacement{}, func(w *X11Window) {
		if got := w.ApplyStyle(native.Style{Tool: true}); got != (native.Style{Tool: true}) {
			t.Fatalf("ApplyStyle до Create = %+v", got)
		}
	})
	if typ, ok := propBeforeMap(reqs, srv.atom("_NET_WM_WINDOW_TYPE")); ok {
		t.Errorf("окну-инструменту поставлен тип %v", typ)
	}
	st, ok := propBeforeMap(reqs, srv.atom("_NET_WM_STATE"))
	want := []uint32{srv.atom("_NET_WM_STATE_SKIP_TASKBAR"), srv.atom("_NET_WM_STATE_SKIP_PAGER"),
		srv.atom("_KDE_NET_WM_STATE_SKIP_SWITCHER")}
	if !ok || !equalWords(st, want) {
		t.Errorf("_NET_WM_STATE %v (поставлено %v), ждал %v", st, ok, want)
	}
	for _, r := range reqs {
		if r.major == xOpSendEvent {
			t.Error("до показа ушёл ClientMessage: начальное состояние ставится свойством")
		}
	}
}

// «Развёрнуто» и Tool — одним значением _NET_WM_STATE.
func TestX11Style_ToolAndMaximizedOneProperty(t *testing.T) {
	srv, reqs := createStyleX11(t, native.InitialPlacement{Maximized: true}, func(w *X11Window) {
		w.ApplyStyle(native.Style{Tool: true})
	})
	want := []uint32{
		srv.atom("_NET_WM_STATE_MAXIMIZED_VERT"), srv.atom("_NET_WM_STATE_MAXIMIZED_HORZ"),
		srv.atom("_NET_WM_STATE_SKIP_TASKBAR"), srv.atom("_NET_WM_STATE_SKIP_PAGER"),
		srv.atom("_KDE_NET_WM_STATE_SKIP_SWITCHER"),
	}
	n := 0
	for _, r := range reqs {
		if isChangeProp(r, srv.atom("_NET_WM_STATE")) {
			n++
			if got := propWords(r); !equalWords(got, want) {
				t.Errorf("_NET_WM_STATE = %v, ждал %v", got, want)
			}
		}
	}
	if n != 1 {
		t.Errorf("_NET_WM_STATE ставилось %d раз, ждал одно значение", n)
	}
}

// Владелец до Create: WM_TRANSIENT_FOR и тип NORMAL до MapWindow, без MODAL.
func TestX11Style_OwnerBeforeMap(t *testing.T) {
	owner := &X11Window{wid: 0x55}
	srv, reqs := createStyleX11(t, native.InitialPlacement{}, func(w *X11Window) {
		if !w.SetOwnerWindow(owner) {
			t.Fatal("SetOwnerWindow до Create = false")
		}
	})
	if v, ok := propBeforeMap(reqs, xAtomWMTransientFor); !ok || !equalWords(v, []uint32{0x55}) {
		t.Errorf("WM_TRANSIENT_FOR %v (поставлен %v), ждал 0x55", v, ok)
	}
	if v, ok := propBeforeMap(reqs, srv.atom("_NET_WM_WINDOW_TYPE")); !ok || !equalWords(v, []uint32{srv.atom("_NET_WM_WINDOW_TYPE_NORMAL")}) {
		t.Errorf("тип окна %v (поставлен %v), ждал NORMAL: иначе менеджер сочтёт окно диалогом", v, ok)
	}
	if v, ok := propBeforeMap(reqs, srv.atom("_NET_WM_STATE")); ok {
		t.Errorf("окну с владельцем поставлено состояние %v (MODAL — только у модалки)", v)
	}
}

// Владелец, который ещё не создан или уже закрыт, не принимается; окно не
// принимает владельцем себя.
func TestX11Style_OwnerRefusals(t *testing.T) {
	w := &X11Window{}
	if w.SetOwnerWindow(&X11Window{}) {
		t.Error("принят владелец без окна")
	}
	if w.SetOwnerWindow(x11Foreign{}) {
		t.Error("принят владелец чужого бэкенда")
	}
	closed := &X11Window{wid: 0x44}
	closed.closed.Store(true)
	if w.SetOwnerWindow(closed) {
		t.Error("принят закрытый владелец")
	}
	live, server := newStyleX11(t)
	if live.SetOwnerWindow(live) {
		t.Error("созданное окно приняло себя владельцем")
	}
	noReq(t, server)
}

// x11Foreign — окно не X11 (владельцем быть не может).
type x11Foreign struct{ native.NativeWindow }

// Tool у показанного окна: SKIP-состояния ClientMessage'ами, тип окна не
// трогается; повтор того же стиля ничего не шлёт.
func TestX11Style_ToolLive(t *testing.T) {
	w, server := newStyleX11(t)
	if got := w.ApplyStyle(native.Style{Tool: true}); got != (native.Style{Tool: true}) {
		t.Fatalf("ApplyStyle(Tool) = %+v", got)
	}
	for _, a := range []uint32{tAtomSkipTB, tAtomSkipPg, tAtomSkipSw} {
		wantStateMsg(t, readReq(t, server), netWMStateAdd, a)
	}
	w.ApplyStyle(native.Style{Tool: true})
	noReq(t, server)

	if got := w.ApplyStyle(native.Style{}); got != (native.Style{}) {
		t.Fatalf("ApplyStyle({}) = %+v", got)
	}
	for _, a := range []uint32{tAtomSkipTB, tAtomSkipPg, tAtomSkipSw} {
		wantStateMsg(t, readReq(t, server), netWMStateRemove, a)
	}
	noReq(t, server)
}

// Без атома KWin Tool всё равно выполним: SKIP_SWITCHER — расширение.
func TestX11Style_ToolWithoutSwitcherAtom(t *testing.T) {
	w, server := newStyleX11(t)
	w.style.atomSkipSwitcher = 0
	if got := w.ApplyStyle(native.Style{Tool: true}); !got.Tool {
		t.Fatal("Tool без _KDE_NET_WM_STATE_SKIP_SWITCHER не принят")
	}
	wantStateMsg(t, readReq(t, server), netWMStateAdd, tAtomSkipTB)
	wantStateMsg(t, readReq(t, server), netWMStateAdd, tAtomSkipPg)
	noReq(t, server)
}

// Borderless у показанного окна: нулевые _GTK_FRAME_EXTENTS, снятие —
// DeleteProperty.
func TestX11Style_BorderlessLive(t *testing.T) {
	w, server := newStyleX11(t)
	if got := w.ApplyStyle(native.Style{Borderless: true}); got != (native.Style{Borderless: true}) {
		t.Fatalf("ApplyStyle(Borderless) = %+v", got)
	}
	wantProp(t, readReq(t, server), tAtomFrameExt, 0, 0, 0, 0)
	w.ApplyStyle(native.Style{Borderless: true})
	noReq(t, server)
	w.ApplyStyle(native.Style{})
	r := readReq(t, server)
	if r.major != xOpDeleteProp || binary.LittleEndian.Uint32(r.raw[8:12]) != tAtomFrameExt {
		t.Fatalf("снятие Borderless: опкод %d, % x", r.major, r.raw)
	}
}

// Сервер не дал атомов EWMH: Tool честно не применяется.
func TestX11Style_ToolWithoutAtoms(t *testing.T) {
	w, server := newFsX11(t)
	if got := w.ApplyStyle(native.Style{Tool: true, Borderless: true}); got != (native.Style{Borderless: true}) {
		t.Fatalf("ApplyStyle без атомов = %+v, ждал только Borderless", got)
	}
	noReq(t, server)
}

// Borderless: менеджеру тянуть край окна не предлагается; без Borderless —
// предлагается (_NET_WM_MOVERESIZE).
func TestX11Style_BorderlessNoResize(t *testing.T) {
	w, server := newStyleX11(t)
	w.style.atomFrameExtents = 0 // здесь проверяется только ресайз
	w.atomNetWMMoveResize = tAtomMoveRes
	w.screen.Root = tRoot
	w.SetResizable(true)
	if !w.BeginResize(base.NativeEdgeLeft) {
		t.Fatal("BeginResize обычного окна = false")
	}
	if r := readReq(t, server); r.major != 27 { // UngrabPointer
		t.Fatalf("ждал UngrabPointer, пришёл опкод %d", r.major)
	}
	if r := readReq(t, server); r.major != xOpSendEvent || binary.LittleEndian.Uint32(r.raw[12+8:12+12]) != tAtomMoveRes {
		t.Fatalf("ждал _NET_WM_MOVERESIZE, пришёл опкод %d", r.major)
	}
	w.ApplyStyle(native.Style{Borderless: true})
	if w.BeginResize(base.NativeEdgeLeft) {
		t.Error("BeginResize у Borderless-окна = true")
	}
	noReq(t, server)
}

// Владелец у показанного окна: тип NORMAL, затем WM_TRANSIENT_FOR; снятие —
// DeleteProperty. MODAL не ставится.
func TestX11Style_OwnerLive(t *testing.T) {
	w, server := newStyleX11(t)
	owner := &X11Window{wid: 0x66}
	if !w.SetOwnerWindow(owner) {
		t.Fatal("SetOwnerWindow = false")
	}
	wantProp(t, readReq(t, server), tAtomType, tAtomNormal)
	wantProp(t, readReq(t, server), xAtomWMTransientFor, 0x66)
	noReq(t, server)

	if !w.SetOwnerWindow(nil) {
		t.Fatal("SetOwnerWindow(nil) = false")
	}
	r := readReq(t, server)
	if r.major != xOpDeleteProp || binary.LittleEndian.Uint32(r.raw[4:8]) != tWid ||
		binary.LittleEndian.Uint32(r.raw[8:12]) != xAtomWMTransientFor {
		t.Fatalf("снятие владельца: опкод %d, % x", r.major, r.raw)
	}

	// Закрытый владелец не принимается.
	owner.closed.Store(true)
	if w.SetOwnerWindow(owner) {
		t.Error("принят закрытый владелец")
	}
	noReq(t, server)
}

// Закрытое окно не шлёт ничего и отвечает false (стиль — тот, что был).
func TestX11Style_ClosedWindow(t *testing.T) {
	w, server := newStyleX11(t)
	w.style.borderless.Store(true) // стиль, который был до закрытия
	w.closed.Store(true)
	if w.RaiseWindow() || w.LowerWindow() || w.MoveTo(1, 1) || w.SetOwnerWindow(&X11Window{wid: 0x66}) {
		t.Error("закрытое окно ответило true")
	}
	if _, _, ok := w.ScreenPosition(); ok {
		t.Error("ScreenPosition закрытого окна ok")
	}
	if got := w.ApplyStyle(native.Style{Tool: true}); got != (native.Style{Borderless: true}) {
		t.Errorf("ApplyStyle закрытого окна = %+v, ждал прежний стиль {Borderless}", got)
	}
	noReq(t, server)
}

func TestX11Stack_RaiseLower(t *testing.T) {
	w, server := newStyleX11(t)
	w.atomNetActiveWindow = 318 // иначе _NET_ACTIVE_WINDOW не ушёл бы и при ошибке
	for _, tc := range []struct {
		do   func() bool
		mode uint32
	}{{w.RaiseWindow, x11StackAbove}, {w.LowerWindow, x11StackBelow}} {
		if !tc.do() {
			t.Fatal("Raise или Lower = false")
		}
		r := readReq(t, server)
		le := binary.LittleEndian
		if r.major != xOpConfigureWn || le.Uint32(r.raw[4:8]) != tWid || le.Uint16(r.raw[8:10]) != 0x40 ||
			le.Uint32(r.raw[12:16]) != tc.mode {
			t.Fatalf("ConfigureWindow stack-mode %d: % x", tc.mode, r.raw)
		}
	}
	noReq(t, server) // без _NET_ACTIVE_WINDOW и SetInputFocus: фокус не передаётся
	var empty X11Window
	if empty.RaiseWindow() || empty.LowerWindow() {
		t.Error("Raise или Lower без окна = true")
	}
}

func TestX11Position_MoveTo(t *testing.T) {
	w, server := newStyleX11(t)
	if !w.MoveTo(150, 120) {
		t.Fatal("MoveTo = false")
	}
	r := readReq(t, server)
	le := binary.LittleEndian
	if r.major != xOpConfigureWn || le.Uint16(r.raw[8:10]) != 0x03 ||
		int32(le.Uint32(r.raw[12:16])) != 150 || int32(le.Uint32(r.raw[16:20])) != 120 {
		t.Fatalf("ConfigureWindow x|y: % x", r.raw)
	}
	if x, y, ok := w.ScreenPosition(); !ok || x != 150 || y != 120 {
		t.Fatalf("ScreenPosition = %d, %d, %v", x, y, ok)
	}
	w.maximized = true
	if w.MoveTo(1, 1) {
		t.Error("MoveTo у развёрнутого окна = true")
	}
	w.maximized = false
	w.act.minimized.Store(true)
	if w.MoveTo(1, 1) {
		t.Error("MoveTo у свёрнутого окна = true")
	}
	if _, _, ok := w.ScreenPosition(); ok {
		t.Error("ScreenPosition у свёрнутого окна ok")
	}
	w.act.minimized.Store(false)
	w.fs.on.Store(true)
	if w.MoveTo(1, 1) {
		t.Error("MoveTo у полноэкранного окна = true")
	}
	noReq(t, server)
	var empty X11Window
	if empty.MoveTo(1, 1) {
		t.Error("MoveTo без окна = true")
	}
	if _, _, ok := empty.ScreenPosition(); ok {
		t.Error("ScreenPosition без окна ok")
	}
}

// Из трея окно-инструмент возвращается со своими SKIP-состояниями: менеджер
// снял _NET_WM_STATE, когда окно спрятали.
func TestX11Style_RestoreFromTrayRepeatsSkip(t *testing.T) {
	w, server := newStyleX11(t)
	w.style.tool.Store(true)
	w.RestoreFromTray()
	if r := readReq(t, server); r.major != xOpMapWindow {
		t.Fatalf("первым ждал MapWindow, пришёл опкод %d", r.major)
	}
	for _, a := range []uint32{tAtomSkipTB, tAtomSkipPg, tAtomSkipSw} {
		wantStateMsg(t, readReq(t, server), netWMStateAdd, a)
	}
}

// Обычное окно из трея SKIP-состояний не получает.
func TestX11Style_RestoreFromTrayPlain(t *testing.T) {
	w, server := newStyleX11(t)
	w.RestoreFromTray()
	if r := readReq(t, server); r.major != xOpMapWindow {
		t.Fatalf("первым ждал MapWindow, пришёл опкод %d", r.major)
	}
	if r := readReq(t, server); r.major == xOpSendEvent && binary.LittleEndian.Uint32(r.raw[12+8:12+12]) == tAtomState {
		t.Fatal("обычному окну из трея ушли состояния _NET_WM_STATE")
	}
}
