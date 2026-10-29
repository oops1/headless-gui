package core

// hosted_owner_test.go — хостовый диалог и плавающая панель дока в своих
// окнах ОС с вторичными движками (ADR-0008, приёмка 4; задача 5.4): дерево
// диалога и оторванной панели принадлежит вторичному движку, их изменения не
// будят движок главного окна; после закрытия диалога и возврата панели в док
// владение возвращается. Окна ОС подменены (newNativeWindow), путь хоста —
// настоящий: bringUp ставит хост диалогов и хост дока, движки — настоящие.

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

// hostFake — окно ОС для хостов: owner-окна и маршалинг на «UI-поток»
// (синхронно), остальное — как initFake.
type hostFake struct {
	initFake
	closed  atomic.Bool
	enabled atomic.Bool
}

func (f *hostFake) SetOwner(native.NativeWindow) {}
func (f *hostFake) SetEnabled(v bool)            { f.enabled.Store(v) }
func (f *hostFake) InvokeOnUIThread(fn func())   { fn() }
func (f *hostFake) Close()                       { f.closed.Store(true) }
func (f *hostFake) GetSize() (int, int)          { return 800, 600 }
func (f *hostFake) GetPosition() (int, int)      { return 0, 0 }

// hostedRig — главное окно с настоящим движком и подменёнными окнами ОС.
type hostedRig struct {
	eng   *engine.Engine
	win   *Window
	root  *widget.Panel
	label *widget.Label

	// fakes — созданные окна ОС: хост дока создаёт их на горутине движка.
	mu    sync.Mutex
	fakes []*hostFake
}

// fake — i-е созданное окно ОС (nil — ещё нет).
func (r *hostedRig) fake(i int) *hostFake {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i < len(r.fakes) {
		return r.fakes[i]
	}
	return nil
}

func newHostedRig(t *testing.T, extra func(root *widget.Panel)) *hostedRig {
	t.Helper()
	r := &hostedRig{}
	old := newNativeWindow
	newNativeWindow = func() native.NativeWindow {
		f := &hostFake{}
		r.mu.Lock()
		r.fakes = append(r.fakes, f)
		r.mu.Unlock()
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
	// Уборка и на пути падения: вторичные движки диалогов и панелей.
	t.Cleanup(r.win.tearDown)
	r.eng.Start()
	return r
}

// quietFrames ждёт затишья движка и отдаёт счётчик кадров. Барьер Flush:
// возвращается, только когда цикл дошёл до поставленной функции, то есть не
// посреди кадра (счётчик растёт в начале кадра, и долгий кадр без барьера
// выглядел бы тишиной); три стабильных замера подряд.
func quietFrames(e *engine.Engine) uint64 {
	prev := ^uint64(0)
	stable := 0
	for i := 0; i < 80 && stable < 3; i++ {
		time.Sleep(25 * time.Millisecond)
		e.Flush()
		e.Flush()
		cur := e.Stats().Frames
		if cur == prev {
			stable++
		} else {
			stable = 0
		}
		prev = cur
	}
	return prev
}

// framesAfter ждёт кадр сверх from (положительная проверка с дедлайном).
func framesAfter(e *engine.Engine, from uint64) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if e.Stats().Frames > from {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// waitOwner ждёт, пока владелец дерева виджета w станет want (nil — снят).
func waitOwner(t *testing.T, w widget.Widget, want widget.Owner, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if widget.OwnerOf(w) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s: владелец %v, ждали %v", what, widget.OwnerOf(w), want)
}

func TestHostedDialog_BelongsToSecondaryEngine(t *testing.T) {
	testreset.Globals(t)
	r := newHostedRig(t, nil)
	mainOwner := widget.OwnerOf(r.label)
	if mainOwner == nil {
		t.Fatal("дерево главного окна без владельца")
	}

	dlg := widget.NewDialog("Диалог", 320, 200)
	inner := widget.NewLabel("в диалоге", color.RGBA{A: 255})
	inner.SetBounds(image.Rect(20, 60, 200, 84))
	dlg.AddChild(inner)
	r.eng.ShowModal(dlg)
	if r.fake(1) == nil {
		t.Fatal("окно ОС для диалога не создано")
	}
	dlgOwner := widget.OwnerOf(inner)
	if dlgOwner == nil || dlgOwner == mainOwner {
		t.Fatalf("диалог: владелец %v, ждали вторичный движок (главный %v)", dlgOwner, mainOwner)
	}

	// Изменение в диалоге не будит движок главного окна.
	before := quietFrames(r.eng)
	for i := 0; i < 5; i++ {
		inner.SetText("правка " + string(rune('0'+i)))
		time.Sleep(20 * time.Millisecond)
	}
	if after := quietFrames(r.eng); after != before {
		t.Errorf("правки в хостовом диалоге будили главный движок: кадров %d → %d", before, after)
	}
	// А изменение в главном окне — будит.
	r.label.SetText("правка в главном")
	if !framesAfter(r.eng, before) {
		t.Error("правка в главном окне не дала кадра")
	}

	// Закрытие: вторичный движок останавливается и отпускает дерево диалога.
	r.eng.CloseModal(dlg)
	waitOwner(t, inner, nil, "после закрытия диалога")
	if !r.fake(1).closed.Load() {
		t.Error("окно ОС диалога не закрыто")
	}
}

func TestHostedDockPane_FloatsAndReturns(t *testing.T) {
	testreset.Globals(t)
	var dm *widget.DockManager
	var pane *widget.DockPane
	var inner *widget.Label
	r := newHostedRig(t, func(root *widget.Panel) {
		dm = widget.NewDockManager()
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
	if got := widget.OwnerOf(inner); got != mainOwner {
		t.Fatalf("панель в доке: владелец %v, ждали главный %v", got, mainOwner)
	}

	// Отрыв в нативное окно: панель — корень вторичного движка. Барьер Flush:
	// маршалинг подделки синхронный, к его возврату окно создано целиком.
	r.eng.Post(pane.Float)
	r.eng.Flush()
	if r.fake(1) == nil {
		t.Fatal("хост дока не поднял окно для оторванной панели")
	}
	floatOwner := widget.OwnerOf(inner)
	if floatOwner == nil || floatOwner == mainOwner {
		t.Fatalf("оторванная панель: владелец %v, ждали вторичный движок", floatOwner)
	}
	before := quietFrames(r.eng)
	for i := 0; i < 5; i++ {
		inner.SetText("правка " + string(rune('0'+i)))
		time.Sleep(20 * time.Millisecond)
	}
	if after := quietFrames(r.eng); after != before {
		t.Errorf("правки в оторванной панели будили главный движок: кадров %d → %d", before, after)
	}

	// Возврат в док: панель снова в дереве главного окна.
	r.eng.Post(func() { pane.Dock(widget.DockLeft) })
	r.eng.Flush()
	waitOwner(t, inner, mainOwner, "после возврата в док")
}
