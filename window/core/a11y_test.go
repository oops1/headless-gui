package core

import (
	"errors"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

// Движок отдаёт всё, что адаптер у него спрашивает: расхождение сигнатур —
// ошибка сборки, а не тихий отказ утверждения типа.
var (
	_ accessibilityTreeProvider = (*engine.Engine)(nil)
	_ accessibilityActor        = (*engine.Engine)(nil)
	_ accessibilityNodeActor    = (*engine.Engine)(nil)
)

// fakeBridge — мост доступности для тестов ядра.
type fakeBridge struct {
	startErr error
	started  int
	stopped  int
	host     native.AccessHost
	// createdAtStart — окно ОС было создано, когда мост поднимали.
	createdAtStart bool
}

func (b *fakeBridge) Start() error {
	b.started++
	if f, ok := b.host.Native().(*initFake); ok {
		for _, e := range f.log {
			if e == "create" {
				b.createdAtStart = true
			}
		}
	}
	return b.startErr
}
func (b *fakeBridge) Stop() { b.stopped++ }

// registerFakeAccess подключает фабрику мостов на время теста.
func registerFakeAccess(t *testing.T, b *fakeBridge) {
	t.Helper()
	t.Cleanup(native.RegisterAccessibility(func(h native.AccessHost) native.AccessBridge {
		b.host = h
		return b
	}))
}

// Без возможности доступности (window/minimal, тестовый бинарь ядра) мост не
// поднимается и окно работает как прежде.
func TestAccessibility_NoCapabilityNoop(t *testing.T) {
	if f, ok := native.Accessibility(); ok && f != nil {
		t.Skip("доступность подключена в этом бинаре")
	}
	win := newTestWindow(widget.NewWindow("x", 100, 100))
	win.startAccessibility()
	if win.a11y != nil {
		t.Fatal("мост поднят без возможности доступности")
	}
	win.stopAccessibility()
}

// С возможностью мост поднимается и снимается; мост, который не поднялся, окну
// не достаётся.
func TestAccessibility_FactoryStartStop(t *testing.T) {
	b := &fakeBridge{}
	registerFakeAccess(t, b)
	win := newTestWindow(widget.NewWindow("x", 100, 100))
	win.startAccessibility()
	if b.started != 1 || win.a11y != b {
		t.Fatalf("Start звали %d раз, мост окна %v", b.started, win.a11y)
	}
	win.startAccessibility() // второй раз — не заводит второй мост
	if b.started != 1 {
		t.Errorf("повторный startAccessibility поднял мост ещё раз")
	}
	win.stopAccessibility()
	if b.stopped != 1 || win.a11y != nil {
		t.Errorf("Stop звали %d раз, мост окна %v", b.stopped, win.a11y)
	}

	failing := &fakeBridge{startErr: errors.New("нет службы")}
	registerFakeAccess(t, failing)
	win2 := newTestWindow(widget.NewWindow("x", 100, 100))
	win2.startAccessibility()
	if win2.a11y != nil {
		t.Error("мост, который не поднялся, достался окну")
	}
}

// bringUp поднимает мост после создания окна ОС, tearDown снимает.
func TestAccessibility_BringUpAndTearDown(t *testing.T) {
	b := &fakeBridge{}
	registerFakeAccess(t, b)
	win, f := initBringUp(t, "1", nil)
	if b.started != 1 {
		t.Fatalf("bringUp не поднял мост (Start %d раз)", b.started)
	}
	if b.host.Native() != native.NativeWindow(f) {
		t.Error("мост получил не то окно ОС")
	}
	if !b.createdAtStart {
		t.Errorf("мост поднят до создания окна ОС (журнал бэкенда %v)", f.log)
	}
	win.tearDown()
	if b.stopped != 1 {
		t.Errorf("tearDown не снял мост (Stop %d раз)", b.stopped)
	}
}

// a11yHost — окно глазами моста: дерево и действия движка, заголовок, масштаб,
// решение SetAccessibilityEnabled.
func TestA11yHost_Adapter(t *testing.T) {
	eng := engine.New(200, 100, 20)
	root := widget.NewWindow("Хост", 200, 100)
	btn := widget.NewButton("Кнопка")
	btn.SetBounds(image.Rect(20, 50, 120, 80))
	clicks := 0
	btn.OnClick = func() { clicks++ }
	root.AddChild(btn)
	eng.SetRoot(root)
	win := New(eng, "Хост")
	win.scale = 1.5
	h := a11yHost{win}

	if h.AccessibilityTree() == nil {
		t.Fatal("дерево семантики не отдано")
	}
	if h.Title() != "Хост" || h.Scale() != 1.5 || h.Native() != nil {
		t.Errorf("заголовок %q, масштаб %v, окно ОС %v", h.Title(), h.Scale(), h.Native())
	}
	if !h.FocusAccessible(btn) || !btn.IsFocused() {
		t.Error("FocusAccessible не сфокусировал кнопку")
	}
	if !h.ActivateAccessible(btn) || clicks != 1 {
		t.Errorf("ActivateAccessible: нажатий %d", clicks)
	}
	if h.FocusAccessible(nil) || h.ActivateAccessible(nil) {
		t.Error("действие над nil = true")
	}
	done := make(chan struct{})
	h.PostToEngine(func() { close(done) })
	eng.Flush()
	select {
	case <-done:
	default:
		t.Error("PostToEngine не выполнил функцию на движке")
	}
	if _, set := h.AccessibilityForced(); set {
		t.Error("решение доступности до SetAccessibilityEnabled")
	}
	win.SetAccessibilityEnabled(false)
	if on, set := h.AccessibilityForced(); !set || on {
		t.Errorf("после SetAccessibilityEnabled(false): on=%v set=%v", on, set)
	}
	win.SetAccessibilityEnabled(true)
	if on, set := h.AccessibilityForced(); !set || !on {
		t.Errorf("после SetAccessibilityEnabled(true): on=%v set=%v", on, set)
	}
}

// actOwner — владелец узлов доступности с обработчиком действий.
type actOwner struct {
	widget.Base
	reply bool
	calls []string
}

func (o *actOwner) Draw(widget.DrawContext) {}

func (o *actOwner) AccessChildren() []widget.AccessInfo {
	return []widget.AccessInfo{{ID: "R", Role: widget.RoleButton, Name: "Узел", Bounds: o.Bounds()}}
}

func (o *actOwner) AccessAction(id string, k widget.AccessActionKind) bool {
	o.calls = append(o.calls, id+":"+k.String())
	return o.reply
}

// Действие узлу владельца доходит до его обработчика через адаптер и движок;
// ответ обработчика — ответ адаптера.
func TestA11yHost_AccessAction(t *testing.T) {
	eng := engine.New(200, 100, 20)
	root := widget.NewWindow("Хост", 200, 100)
	o := &actOwner{reply: true}
	o.SetBounds(image.Rect(20, 50, 120, 80))
	root.AddChild(o)
	eng.SetRoot(root)
	h := a11yHost{New(eng, "")}
	if !h.AccessAction(o, "R", widget.AccessExpand) {
		t.Fatal("AccessAction отказал, хотя обработчик принял действие")
	}
	if len(o.calls) != 1 || o.calls[0] != "R:"+widget.AccessExpand.String() {
		t.Fatalf("обработчик получил %v", o.calls)
	}
	o.reply = false
	if h.AccessAction(o, "R", widget.AccessInvoke) {
		t.Error("AccessAction принял действие, от которого обработчик отказался")
	}
}

// Движок без дерева и действий доступности: хост отвечает «нет».
func TestA11yHost_PartialEngine(t *testing.T) {
	win := New(&postEngine{root: widget.NewWindow("x", 100, 100)}, "")
	h := a11yHost{win}
	if h.AccessibilityTree() != nil {
		t.Error("дерево у движка без AccessibilityTree")
	}
	btn := widget.NewButton("x")
	if h.FocusAccessible(btn) || h.ActivateAccessible(btn) || h.AccessAction(btn, "id", widget.AccessInvoke) {
		t.Error("действие у движка без действий доступности = true")
	}
}
