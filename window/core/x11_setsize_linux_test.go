//go:build linux && !android

package core

import (
	"encoding/binary"
	"os"
	"sync"
	"testing"
	"time"
)

// SetSize на X11 на проводе (x11_setsize_linux.go): окно разговаривает с
// поддельным сервером, события менеджера окон подаются в handleX11Event так,
// как их прислал бы сервер. Размер окна меняет только ConfigureNotify.

// szX11Server записывает ConfigureWindow (опкод 12) и остальные запросы.
type szX11Server struct {
	mu    sync.Mutex
	reqs  []x11Request
	state []uint32 // ответ на GetProperty _NET_WM_STATE
}

func (s *szX11Server) handle(r x11Request) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, r)
	if r.major == 20 { // GetProperty
		return x11PropReply(s.state...)
	}
	return nil
}

// configures — размеры из запросов ConfigureWindow, по порядку.
func (s *szX11Server) configures(t *testing.T) [][2]int {
	t.Helper()
	// Сервер читает асинхронно: даём ему дочитать.
	time.Sleep(60 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	var out [][2]int
	for _, r := range s.reqs {
		if r.major != 12 || len(r.raw) < 20 || binary.LittleEndian.Uint16(r.raw[8:10]) != 0x0C {
			continue // не «ширина|высота»
		}
		out = append(out, [2]int{
			int(binary.LittleEndian.Uint32(r.raw[12:16])),
			int(binary.LittleEndian.Uint32(r.raw[16:20])),
		})
	}
	return out
}

func (s *szX11Server) count(major byte) int {
	time.Sleep(60 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.reqs {
		if r.major == major {
			n++
		}
	}
	return n
}

// szX11Window — окно 1400×860 с подсчётом onResize.
type szX11Window struct {
	*X11Window
	srv     *szX11Server
	resizes [][2]int
}

func newSzX11(t *testing.T) *szX11Window {
	t.Helper()
	w, server := newFsX11(t)
	srv := &szX11Server{}
	go serveFakeX(server, srv.handle)
	w.width, w.height = 1400, 860
	sw := &szX11Window{X11Window: w, srv: srv}
	w.SetOnResize(func(rw, rh int) { sw.resizes = append(sw.resizes, [2]int{rw, rh}) })
	return sw
}

// configureNotify — ConfigureNotify с размером w×h (ширина @20, высота @22).
func szConfigureNotify(w, h int, synthetic bool) []byte {
	buf := makeConfigureNotify(0, 0, synthetic)
	binary.LittleEndian.PutUint16(buf[20:22], uint16(w))
	binary.LittleEndian.PutUint16(buf[22:24], uint16(h))
	return buf
}

// Сценарий отчёта Tandem: 1400×860 → SetSize(520,370) → onResize ровно один
// раз с 520×370, GetSize отвечает 520×370; затем обратно.
func TestX11SetSize_ResizeThroughConfigureNotify(t *testing.T) {
	w := newSzX11(t)

	w.SetSize(520, 370)
	if got := w.srv.configures(t); len(got) != 1 || got[0] != [2]int{520, 370} {
		t.Fatalf("ConfigureWindow: %v, ждал один запрос 520x370", got)
	}
	// Менеджер ещё не ответил: размер окна прежний, и это правда.
	if gw, gh := w.GetSize(); gw != 1400 || gh != 860 {
		t.Errorf("GetSize до ответа менеджера = %dx%d, ждал фактический 1400x860", gw, gh)
	}
	if len(w.resizes) != 0 {
		t.Fatalf("onResize до ответа менеджера: %v", w.resizes)
	}

	w.handleX11Event(szConfigureNotify(520, 370, false))
	if len(w.resizes) != 1 || w.resizes[0] != [2]int{520, 370} {
		t.Fatalf("onResize: %v, ждал один вызов 520x370", w.resizes)
	}
	if gw, gh := w.GetSize(); gw != 520 || gh != 370 {
		t.Errorf("GetSize = %dx%d, ждал 520x370", gw, gh)
	}

	// Менеджер повторил событие (synthetic после перемещения): второго
	// onResize нет.
	w.handleX11Event(szConfigureNotify(520, 370, true))
	if len(w.resizes) != 1 {
		t.Errorf("повторный ConfigureNotify с тем же размером вызвал onResize: %v", w.resizes)
	}

	// Обратно.
	w.SetSize(1400, 860)
	w.handleX11Event(szConfigureNotify(1400, 860, false))
	if got := w.srv.configures(t); len(got) != 2 || got[1] != [2]int{1400, 860} {
		t.Fatalf("ConfigureWindow: %v, ждал вторым 1400x860", got)
	}
	if len(w.resizes) != 2 || w.resizes[1] != [2]int{1400, 860} {
		t.Errorf("onResize: %v, ждал вторым 1400x860", w.resizes)
	}
	if gw, gh := w.GetSize(); gw != 1400 || gh != 860 {
		t.Errorf("GetSize = %dx%d, ждал 1400x860", gw, gh)
	}
}

// Окно с запретом ресайза (SetResizable(false)) меняется программно так же: на
// X11 запрет — дело менеджера по размерным подсказкам, на ConfigureWindow
// приложения он не влияет.
func TestX11SetSize_NonResizableWindow(t *testing.T) {
	w := newSzX11(t)
	w.SetResizable(false)

	w.SetSize(520, 370)
	w.handleX11Event(szConfigureNotify(520, 370, false))
	if len(w.resizes) != 1 || w.resizes[0] != [2]int{520, 370} {
		t.Fatalf("onResize: %v, ждал один вызов 520x370", w.resizes)
	}
	if gw, gh := w.GetSize(); gw != 520 || gh != 370 {
		t.Errorf("GetSize = %dx%d, ждал 520x370", gw, gh)
	}
}

// Менеджер отказал: прислал синхронный ConfigureNotify с прежним размером.
// Окно осталось прежним, onResize не зовётся, GetSize не врёт.
func TestX11SetSize_RefusedByManager(t *testing.T) {
	w := newSzX11(t)

	w.SetSize(520, 370)
	w.handleX11Event(szConfigureNotify(1400, 860, true))
	if len(w.resizes) != 0 {
		t.Errorf("onResize после отказа менеджера: %v", w.resizes)
	}
	if gw, gh := w.GetSize(); gw != 1400 || gh != 860 {
		t.Errorf("GetSize = %dx%d, ждал фактический 1400x860", gw, gh)
	}
}

// Менеджер промолчал вовсе: по истечении срока GetSize всё равно честный, а
// новый SetSize того же размера уходит заново (запрос «в пути» истёк).
func TestX11SetSize_ManagerSilent(t *testing.T) {
	w := newSzX11(t)

	w.SetSize(520, 370)
	if gw, gh := w.GetSize(); gw != 1400 || gh != 860 {
		t.Fatalf("GetSize = %dx%d, ждал фактический 1400x860", gw, gh)
	}
	w.SetSize(520, 370) // запрос ещё в пути: не дублируется
	if got := w.srv.configures(t); len(got) != 1 {
		t.Fatalf("запрос в пути продублирован: %v", got)
	}
	w.sz.mu.Lock()
	w.sz.at = time.Now().Add(-2 * x11SizeReqTTL) // менеджер так и не ответил
	w.sz.mu.Unlock()
	w.SetSize(520, 370)
	if got := w.srv.configures(t); len(got) != 2 {
		t.Errorf("после истечения срока запрос не повторён: %v", got)
	}
	if len(w.resizes) != 0 {
		t.Errorf("onResize без ответа менеджера: %v", w.resizes)
	}
}

// Менеджер поправил размер (минимум окна): движок получает фактический, один
// раз.
func TestX11SetSize_ManagerAdjustsSize(t *testing.T) {
	w := newSzX11(t)

	w.SetSize(300, 200)
	w.handleX11Event(szConfigureNotify(520, 370, false))
	if len(w.resizes) != 1 || w.resizes[0] != [2]int{520, 370} {
		t.Fatalf("onResize: %v, ждал один вызов с фактическим 520x370", w.resizes)
	}
	if gw, gh := w.GetSize(); gw != 520 || gh != 370 {
		t.Errorf("GetSize = %dx%d, ждал фактический 520x370", gw, gh)
	}
}

// Размер уже такой — запроса нет. Быстрая пара «туда-обратно» до ответа
// менеджера не теряет возврата: второй вызов сравнивается с запрошенным, а не
// с ещё не изменившимся фактическим размером.
func TestX11SetSize_SameSizeAndQuickRoundTrip(t *testing.T) {
	w := newSzX11(t)

	w.SetSize(1400, 860)
	if got := w.srv.configures(t); len(got) != 0 {
		t.Fatalf("SetSize того же размера отправил запрос: %v", got)
	}

	w.SetSize(520, 370)
	w.SetSize(1400, 860) // до ответа менеджера: фактический всё ещё 1400x860
	if got := w.srv.configures(t); len(got) != 2 || got[1] != [2]int{1400, 860} {
		t.Fatalf("ConfigureWindow: %v, ждал 520x370, затем 1400x860", got)
	}
	// Менеджер отвечает по порядку.
	w.handleX11Event(szConfigureNotify(520, 370, false))
	w.handleX11Event(szConfigureNotify(1400, 860, false))
	if gw, gh := w.GetSize(); gw != 1400 || gh != 860 {
		t.Errorf("GetSize = %dx%d, ждал 1400x860", gw, gh)
	}
	if len(w.resizes) != 2 {
		t.Errorf("onResize: %v, ждал два вызова", w.resizes)
	}
}

// Окно, которого ещё нет на сервере (wid == 0), размер только запоминает —
// запроса нет, GetSize отвечает им.
func TestX11SetSize_BeforeWindowExists(t *testing.T) {
	w := newSzX11(t)
	w.wid = 0

	w.SetSize(640, 480)
	if gw, gh := w.GetSize(); gw != 640 || gh != 480 {
		t.Errorf("GetSize = %dx%d, ждал запомненный 640x480", gw, gh)
	}
	if got := w.srv.configures(t); len(got) != 0 {
		t.Errorf("запрос для несуществующего окна: %v", got)
	}
}

// Попап (override-redirect): менеджера нет, StructureNotify не выбран — размер
// применяется сразу.
func TestX11SetSize_PopupAppliesImmediately(t *testing.T) {
	w := newSzX11(t)
	w.popup = true

	w.SetSize(200, 100)
	if gw, gh := w.GetSize(); gw != 200 || gh != 100 {
		t.Errorf("GetSize = %dx%d, ждал 200x100", gw, gh)
	}
	if got := w.srv.configures(t); len(got) != 1 || got[0] != [2]int{200, 100} {
		t.Errorf("ConfigureWindow: %v", got)
	}
}

// Нулевой и отрицательный размер игнорируются.
func TestX11SetSize_NonPositiveIgnored(t *testing.T) {
	w := newSzX11(t)
	w.SetSize(0, 100)
	w.SetSize(100, -5)
	if got := w.srv.configures(t); len(got) != 0 {
		t.Errorf("недопустимый размер ушёл серверу: %v", got)
	}
	if gw, gh := w.GetSize(); gw != 1400 || gh != 860 {
		t.Errorf("GetSize = %dx%d", gw, gh)
	}
}

// Развёрнутому окну размер выбирает менеджер: SetSize запоминает заказ и
// отправляет его, когда окно вернулось в обычное состояние — по Restore.
func TestX11SetSize_WhileMaximizedAppliesAfterRestore(t *testing.T) {
	w := newSzX11(t)
	w.maximized = true

	w.SetSize(520, 370)
	if got := w.srv.configures(t); len(got) != 0 {
		t.Fatalf("развёрнутому окну отправлен ConfigureWindow: %v", got)
	}
	if gw, gh := w.GetSize(); gw != 1400 || gh != 860 {
		t.Errorf("GetSize = %dx%d, ждал размер развёрнутого окна", gw, gh)
	}

	w.Restore()
	got := w.srv.configures(t)
	if len(got) != 1 || got[0] != [2]int{520, 370} {
		t.Fatalf("после Restore ConfigureWindow: %v, ждал 520x370", got)
	}
	w.handleX11Event(szConfigureNotify(520, 370, false))
	if len(w.resizes) != 1 || w.resizes[0] != [2]int{520, 370} {
		t.Errorf("onResize: %v, ждал один вызов 520x370", w.resizes)
	}
}

// Окно после Restore из развёрнутого: SetSize идёт обычным путём.
func TestX11SetSize_AfterRestore(t *testing.T) {
	w := newSzX11(t)
	w.maximized = true
	w.width, w.height = 1920, 1080 // размер развёрнутого окна

	w.Restore()
	w.handleX11Event(szConfigureNotify(1400, 860, false)) // менеджер вернул прежний
	if len(w.resizes) != 1 {
		t.Fatalf("onResize после Restore: %v", w.resizes)
	}

	w.SetSize(520, 370)
	w.handleX11Event(szConfigureNotify(520, 370, false))
	if len(w.resizes) != 2 || w.resizes[1] != [2]int{520, 370} {
		t.Errorf("onResize: %v, ждал вторым 520x370", w.resizes)
	}
	if gw, gh := w.GetSize(); gw != 520 || gh != 370 {
		t.Errorf("GetSize = %dx%d, ждал 520x370", gw, gh)
	}
}

// Менеджер сам вернул окно из полного экрана (PropertyNotify по
// _NET_WM_STATE, окно перечитывает свойство): заказ, сделанный в полном
// экране, уходит серверу.
func TestX11SetSize_WhileFullscreenAppliesOnLeave(t *testing.T) {
	w := newSzX11(t)
	w.srv.state = []uint32{tAtomFull}
	w.fs.stale.Store(true)
	w.x11SyncWMState() // менеджер держит окно в полном экране
	if !w.IsFullscreen() {
		t.Fatal("полный экран не распознан")
	}

	w.SetSize(520, 370)
	if got := w.srv.configures(t); len(got) != 0 {
		t.Fatalf("полноэкранному окну отправлен ConfigureWindow: %v", got)
	}

	w.srv.mu.Lock()
	w.srv.state = nil
	w.srv.mu.Unlock()
	w.fs.stale.Store(true)
	w.x11SyncWMState() // вышел из полного экрана
	if got := w.srv.configures(t); len(got) != 1 || got[0] != [2]int{520, 370} {
		t.Errorf("после выхода из полного экрана ConfigureWindow: %v", got)
	}
	// Заказ применяется один раз.
	w.fs.stale.Store(true)
	w.x11SyncWMState()
	if got := w.srv.configures(t); len(got) != 1 {
		t.Errorf("заказ применён повторно: %v", got)
	}
}

// SHM-сегмент пересоздаётся под новый размер по ConfigureNotify (не при самом
// SetSize): блит не уходит на PutImage-фолбэк, а сервер получает ShmAttach.
func TestX11SetSize_ShmFollowsConfigureNotify(t *testing.T) {
	w := newSzX11(t)
	w.shm = &x11ShmState{major: 130, firstEvent: 90}
	w.shm.width, w.shm.height = 1400, 860

	w.SetSize(520, 370)
	if w.shm.width != 1400 || w.shm.height != 860 {
		t.Fatalf("сегмент пересоздан до ответа менеджера: %dx%d", w.shm.width, w.shm.height)
	}
	w.handleX11Event(szConfigureNotify(520, 370, false))
	w.blitMu.Lock()
	sw, sh, fallback := w.shm.width, w.shm.height, w.shm.fallback
	w.blitMu.Unlock()
	if fallback {
		t.Skip("SysV shm недоступен в этом окружении")
	}
	if sw != 520 || sh != 370 {
		t.Errorf("сегмент %dx%d, ждал пересозданный под 520x370", sw, sh)
	}
	if n := w.srv.count(130); n == 0 {
		t.Error("серверу не ушёл ShmAttach нового сегмента")
	}
}

// Живая проверка против настоящего X-сервера (Xvfb, WSLg/XWayland через
// DISPLAY): SetSize доходит до onResize ровно один раз и GetSize отвечает
// фактическим размером. Без DISPLAY или если сервер/менеджер размера не
// изменил (нечего проверять) — пропуск.
func TestX11SetSize_LiveServer(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}
	t.Setenv("HEADLESS_GUI_X11", "1")
	w := &X11Window{}
	if err := w.Create("x11 setsize e2e", 320, 240); err != nil {
		t.Skipf("не удалось поднять окно: %v", err)
	}
	defer w.Close()

	var mu sync.Mutex
	var got [][2]int
	w.SetOnResize(func(rw, rh int) {
		mu.Lock()
		got = append(got, [2]int{rw, rh})
		mu.Unlock()
	})
	snapshot := func() [][2]int {
		mu.Lock()
		defer mu.Unlock()
		return append([][2]int(nil), got...)
	}
	go w.RunEventLoop()

	// Менеджер мог ещё и не показать окно: ждём, пока размер устоится.
	time.Sleep(300 * time.Millisecond)
	base := len(snapshot())

	w.SetSize(260, 200)
	if !fsWaitFor(func() bool { return len(snapshot()) > base }) {
		t.Skip("сервер или оконный менеджер не изменил размер окна")
	}
	time.Sleep(200 * time.Millisecond) // дождаться возможных повторов
	ev := snapshot()[base:]
	gw, gh := w.GetSize()
	last := ev[len(ev)-1]
	if gw != last[0] || gh != last[1] {
		t.Errorf("GetSize = %dx%d, последний onResize %v: размер разошёлся", gw, gh, last)
	}
	if len(ev) != 1 {
		t.Errorf("onResize на один SetSize вызван %d раз: %v", len(ev), ev)
	}
	// SHM-сегмент (если сервер его даёт) следует за фактическим размером.
	if w.shm != nil {
		w.blitMu.Lock()
		sw, sh, fallback := w.shm.width, w.shm.height, w.shm.fallback
		w.blitMu.Unlock()
		if !fallback && (sw != gw || sh != gh) {
			t.Errorf("SHM-сегмент %dx%d, окно %dx%d", sw, sh, gw, gh)
		}
	}
	t.Logf("запрошено 260x200, фактически %dx%d", gw, gh)
}

// Менеджер сам вернул окно из развёрнутого (двойной щелчок по заголовку):
// заказ, сделанный при развёрнутом окне, уходит серверу.
func TestX11SetSize_WhileMaximizedAppliesWhenManagerRestores(t *testing.T) {
	w := newSzX11(t)
	w.srv.state = []uint32{tAtomMaxH, tAtomMaxV}
	w.fs.stale.Store(true)
	w.x11SyncWMState()
	if !w.IsMaximized() {
		t.Fatal("развёрнутое состояние не распознано")
	}

	w.SetSize(520, 370)
	if got := w.srv.configures(t); len(got) != 0 {
		t.Fatalf("развёрнутому окну отправлен ConfigureWindow: %v", got)
	}

	w.srv.mu.Lock()
	w.srv.state = nil
	w.srv.mu.Unlock()
	w.fs.stale.Store(true)
	w.x11SyncWMState()
	if got := w.srv.configures(t); len(got) != 1 || got[0] != [2]int{520, 370} {
		t.Errorf("после возврата из развёрнутого ConfigureWindow: %v", got)
	}
}
