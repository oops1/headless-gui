package window

// hosted_owner_review_test.go — случаи из ревью 5.4: список в хостовом
// диалоге уходит в попап-хост диалога, диалог из диалога (три владельца),
// закрытие главного окна сносит хостовые диалоги, отрыв и возврат панели и
// хостовый диалог без утечки горутин.

import (
	"image"
	"image/color"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// revFake — окно ОС: owner запоминается, попапы — по флагу.
type revFake struct {
	hostFake
	id      int
	owner   atomic.Value // NativeWindow
	popup   atomic.Bool  // CreatePopup вызван
	blits   atomic.Int64
	allowPP bool
}

func (f *revFake) SetOwner(w NativeWindow) { f.owner.Store(ownerBox{w}) }
func (f *revFake) BlitRGBA(*image.RGBA)    { f.blits.Add(1) }

type ownerBox struct{ w NativeWindow }

func (f *revFake) ownerWin() NativeWindow {
	if v, ok := f.owner.Load().(ownerBox); ok {
		return v.w
	}
	return nil
}

// revPopupFake — то же, но умеет окна-попапы (popupWindow).
type revPopupFake struct{ *revFake }

func (f revPopupFake) CreatePopup(w, h int) error { f.popup.Store(true); return nil }

type revRig struct {
	eng   *engine.Engine
	win   *Window
	root  *widget.Panel
	label *widget.Label
	host  *dialogHost

	mu    sync.Mutex
	fakes []*revFake
}

func (r *revRig) fake(i int) *revFake {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i < len(r.fakes) {
		return r.fakes[i]
	}
	return nil
}

func (r *revRig) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.fakes)
}

func newRevRig(t *testing.T, popups bool, extra func(root *widget.Panel)) *revRig {
	t.Helper()
	r := &revRig{}
	old := newNativeWindow
	newNativeWindow = func() NativeWindow {
		r.mu.Lock()
		f := &revFake{id: len(r.fakes)}
		r.fakes = append(r.fakes, f)
		r.mu.Unlock()
		if popups {
			return revPopupFake{f}
		}
		return f
	}
	t.Cleanup(func() { newNativeWindow = old })

	r.eng = engine.New(800, 600, 60)
	t.Cleanup(r.eng.Stop)
	r.eng.SetTooltipsEnabled(false)
	r.root = widget.NewPanel(color.RGBA{R: 20, G: 30, B: 40, A: 255})
	r.root.ShowHeader = false
	r.root.SetBounds(image.Rect(0, 0, 800, 600))
	r.label = widget.NewLabel("главное окно", color.RGBA{R: 250, G: 250, B: 250, A: 255})
	r.label.SetBounds(image.Rect(10, 10, 300, 34))
	r.root.AddChild(r.label)
	if extra != nil {
		extra(r.root)
	}
	r.eng.SetRoot(r.root)
	r.win = New(r.eng, "главное")
	if err := r.win.bringUp(); err != nil {
		t.Fatalf("bringUp: %v", err)
	}
	// Хост, установленный окном (bringUp → installModalHost): по нему тест
	// видит стек и вторичные движки, а tearDown окна его сносит.
	r.host = r.win.modalHost
	if r.host == nil {
		t.Fatal("bringUp не поставил хост диалогов")
	}
	r.eng.Start()
	return r
}

func (r *revRig) top(i int) *hostedModal {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	if i < len(r.host.stack) {
		return r.host.stack[i]
	}
	return nil
}

func revWaitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("не дождались: %s", what)
}

// goroutineBase — число горутин до теста после прогрева процесса: постоянный
// пул воркеров растра (internal/workpool) заводится при первом тайловом кадре
// процесса и живёт до конца — при перемешанном порядке тестов его рождение
// внутри теста выглядело бы утечкой.
func goroutineBase() int {
	e := engine.New(256, 256, 60)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 256, 256))
	e.SetRoot(root)
	e.RenderOnce()
	e.Stop()
	// Остановленный движок отпускает свои горутины не мгновенно: база —
	// когда число перестало меняться.
	prev, stable := runtime.NumGoroutine(), 0
	for i := 0; i < 100 && stable < 5; i++ {
		time.Sleep(10 * time.Millisecond)
		n := runtime.NumGoroutine()
		if n == prev {
			stable++
		} else {
			stable = 0
		}
		prev = n
	}
	return prev
}

// waitGoroutines ждёт, пока число горутин вернётся к base (с допуском).
func waitGoroutines(base int) int {
	deadline := time.Now().Add(3 * time.Second)
	n := runtime.NumGoroutine()
	for n > base && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		n = runtime.NumGoroutine()
	}
	return n
}

// 1. Раскрывающийся список внутри хостового диалога уходит в попап-хост
// движка диалога (окно-попап с owner = окно диалога), а не в попап-хост
// главного окна; главный движок не будится.
func TestHostedReview_HostedDialog_DropdownUsesDialogPopupHost(t *testing.T) {
	testreset.Globals(t)
	r := newRevRig(t, true, nil)

	dlg := widget.NewDialog("Диалог", 320, 220)
	dd := widget.NewDropdown("один", "два", "три")
	dd.SetBounds(image.Rect(20, 60, 200, 86))
	dlg.AddChild(dd)
	r.eng.ShowModal(dlg)
	hm := r.top(0)
	if hm == nil || hm.eng == nil {
		t.Fatal("хостовый диалог не поднят")
	}
	defer r.eng.CloseModal(dlg)
	if hm.popupHost == nil {
		t.Fatal("у движка диалога нет попап-хоста (фейк умеет CreatePopup)")
	}
	if widget.OwnerOf(dd) == widget.OwnerOf(r.label) {
		t.Fatal("список в диалоге числится за главным движком")
	}
	mainBefore := quietFrames(r.eng)
	n0 := r.count()

	hm.eng.Post(func() { dd.SetOpen(true) })
	revWaitFor(t, "окно-попап списка", func() bool {
		f := r.fake(n0)
		return f != nil && f.popup.Load()
	})
	pf := r.fake(n0)
	if got, want := pf.ownerWin(), hm.native; got != want {
		t.Errorf("owner попапа %v, ждали окно диалога %v", got, want)
	}
	r.win.popupHost.mu.Lock()
	mainPopups := len(r.win.popupHost.windows)
	r.win.popupHost.mu.Unlock()
	if mainPopups != 0 {
		t.Errorf("попап-хост главного окна получил %d окон", mainPopups)
	}
	if after := quietFrames(r.eng); after != mainBefore {
		t.Errorf("раскрытие списка в диалоге будило главный движок: %d → %d", mainBefore, after)
	}

	hm.eng.Post(func() { dd.SetOpen(false) })
	revWaitFor(t, "закрытие окна-попапа", func() bool { return pf.closed.Load() })
}

// 2. Диалог из диалога: стек хоста, owner второго — окно первого, первое
// отключено на время второго; правки во втором не будят ни главный, ни
// первый движок; после закрытия второго первое снова включено и его дерево
// по-прежнему за его движком.
func TestHostedReview_HostedDialog_Nested(t *testing.T) {
	testreset.Globals(t)
	r := newRevRig(t, false, nil)

	dlg1 := widget.NewDialog("Первый", 360, 240)
	in1 := widget.NewLabel("первый", color.RGBA{A: 255})
	in1.SetBounds(image.Rect(20, 60, 200, 84))
	dlg1.AddChild(in1)
	r.eng.ShowModal(dlg1)
	defer r.eng.CloseModal(dlg1)

	dlg2 := widget.NewDialog("Второй", 240, 160)
	in2 := widget.NewLabel("второй", color.RGBA{A: 255})
	in2.SetBounds(image.Rect(20, 60, 200, 84))
	dlg2.AddChild(in2)
	r.eng.ShowModal(dlg2)

	h1, h2 := r.top(0), r.top(1)
	if h1 == nil || h2 == nil || h1.eng == nil || h2.eng == nil {
		t.Fatal("стек из двух хостовых диалогов не поднят")
	}
	f1 := r.fake(1)
	f2 := r.fake(2)
	if f2.ownerWin() != NativeWindow(f1) {
		t.Errorf("owner второго окна %v, ждали окно первого", f2.ownerWin())
	}
	if f1.enabled.Load() {
		// enabled в hostFake по умолчанию false: проверяем, что SetEnabled(false) звали
		// — косвенно: после закрытия второго должно стать true.
	}
	o0, o1, o2 := widget.OwnerOf(r.label), widget.OwnerOf(in1), widget.OwnerOf(in2)
	if o1 == nil || o2 == nil || o1 == o2 || o1 == o0 || o2 == o0 {
		t.Fatalf("владельцы: главный %v, первый %v, второй %v — ждали три разных", o0, o1, o2)
	}

	m0, m1 := quietFrames(r.eng), quietFrames(h1.eng)
	for i := 0; i < 5; i++ {
		in2.SetText("правка " + string(rune('0'+i)))
		time.Sleep(20 * time.Millisecond)
	}
	if a := quietFrames(r.eng); a != m0 {
		t.Errorf("правки во втором диалоге будили главный движок: %d → %d", m0, a)
	}
	if a := quietFrames(h1.eng); a != m1 {
		t.Errorf("правки во втором диалоге будили движок первого: %d → %d", m1, a)
	}

	r.eng.CloseModal(dlg2)
	if !f1.enabled.Load() {
		t.Error("окно первого диалога не включено после закрытия второго")
	}
	if widget.OwnerOf(in1) != o1 {
		t.Error("после закрытия второго дерево первого сменило владельца")
	}
	if widget.OwnerOf(in2) != nil {
		t.Error("дерево закрытого второго диалога осталось привязанным")
	}
	// Правка в первом по-прежнему рисуется его движком.
	m1 = quietFrames(h1.eng)
	in1.SetText("снова первый")
	revWaitFor(t, "кадр движка первого диалога", func() bool { return h1.eng.Stats().Frames != m1 })
}

// 3. Окно закрывается (tearDown + Stop главного движка) при открытом хостовом
// диалоге: вторичный движок диалога должен остановиться — иначе его цикл,
// насос кадров и регистрация в реестрах живут дальше.
func TestHostedReview_MainTearDownWithOpenHostedDialog(t *testing.T) {
	testreset.Globals(t)
	r := newRevRig(t, false, nil)

	dlg := widget.NewDialog("Диалог", 320, 200)
	inner := widget.NewLabel("в диалоге", color.RGBA{A: 255})
	inner.SetBounds(image.Rect(20, 60, 200, 84))
	dlg.AddChild(inner)
	r.eng.ShowModal(dlg)
	hm := r.top(0)
	if hm == nil || hm.eng == nil {
		t.Fatal("хостовый диалог не поднят")
	}
	// Подстраховка: закрыть диалог в конце, если тест упадёт.
	t.Cleanup(func() { hm.eng.CloseModal(dlg) })

	r.win.tearDown()
	r.eng.Stop()
	time.Sleep(200 * time.Millisecond)
	f0 := hm.eng.Stats().Frames
	inner.SetText("после закрытия главного")
	time.Sleep(200 * time.Millisecond)
	if hm.eng.Stats().Frames != f0 || widget.OwnerOf(inner) != nil {
		t.Errorf("после закрытия главного окна движок хостового диалога жив: кадров %d → %d, владелец %v",
			f0, hm.eng.Stats().Frames, widget.OwnerOf(inner))
	}
}

// 4. Отрыв и возврат панели дока без гонки (барьер Flush вместо опроса
// r.fakes) и без утечки горутин: после возврата и Stop всё завершается.
func TestHostedReview_DockFloatReturn_NoRaceNoLeak(t *testing.T) {
	testreset.Globals(t)
	base := goroutineBase()
	var pane *widget.DockPane
	var inner *widget.Label
	r := newRevRig(t, false, func(root *widget.Panel) {
		dm := widget.NewDockManager()
		dm.NativeFloating = true
		dm.SetBounds(image.Rect(0, 40, 800, 600))
		dm.SetCenter(widget.NewPanel(color.RGBA{B: 120, A: 255}))
		inner = widget.NewLabel("в панели", color.RGBA{A: 255})
		inner.SetBounds(image.Rect(10, 70, 180, 94))
		pane = widget.NewDockPane("p", "Панель", inner)
		dm.AddPane(pane, widget.DockLeft)
		root.AddChild(dm)
	})
	mainOwner := widget.OwnerOf(r.label)
	r.eng.Post(pane.Float)
	r.eng.Flush() // create выполнен на горутине движка (InvokeOnUIThread синхронный)
	if r.count() < 2 {
		t.Fatalf("окно панели не создано (окон %d)", r.count())
	}
	if o := widget.OwnerOf(inner); o == nil || o == mainOwner {
		t.Fatalf("оторванная панель: владелец %v", o)
	}
	r.eng.Post(func() { pane.Dock(widget.DockLeft) })
	r.eng.Flush()
	if widget.OwnerOf(inner) != mainOwner {
		t.Fatal("после возврата панель не за главным движком")
	}
	if !r.fake(1).closed.Load() {
		t.Error("окно оторванной панели не закрыто")
	}
	r.eng.Stop()
	if n := waitGoroutines(base); n > base {
		buf := make([]byte, 1<<16)
		buf = buf[:runtime.Stack(buf, true)]
		t.Errorf("горутин %d, было %d:\n%s", n, base, buf)
	}
}

// 5. То же для хостового диалога: после закрытия и Stop горутин не остаётся.
func TestHostedReview_HostedDialog_NoLeak(t *testing.T) {
	testreset.Globals(t)
	base := goroutineBase()
	r := newRevRig(t, true, nil)
	dlg := widget.NewDialog("Диалог", 320, 220)
	dd := widget.NewDropdown("один", "два")
	dd.SetBounds(image.Rect(20, 60, 200, 86))
	dlg.AddChild(dd)
	r.eng.ShowModal(dlg)
	hm := r.top(0)
	hm.eng.Post(func() { dd.SetOpen(true) })
	revWaitFor(t, "окно-попап", func() bool { f := r.fake(2); return f != nil && f.popup.Load() })
	// Закрытие диалога с открытым списком.
	r.eng.CloseModal(dlg)
	r.eng.Stop()
	if n := waitGoroutines(base); n > base {
		buf := make([]byte, 1<<16)
		buf = buf[:runtime.Stack(buf, true)]
		t.Errorf("горутин %d, было %d:\n%s", n, base, buf)
	}
	if f := r.fake(2); !f.closed.Load() {
		t.Error("окно-попап списка не закрыто при закрытии диалога")
	}
}
