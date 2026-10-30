package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// v3.39: сырое колесо (widget.WheelEvent) и уход указателя с окна.

// rawWheelSpy — виджет, принимающий сырое колесо; take — поглощать ли его.
type rawWheelSpy struct {
	widget.Base
	take   bool
	events []widget.WheelEvent
}

func (s *rawWheelSpy) Draw(ctx widget.DrawContext) {}
func (s *rawWheelSpy) OnWheelEvent(e widget.WheelEvent) bool {
	s.events = append(s.events, e)
	return s.take
}

// leaveSpy — виджет, которому нужен уход указателя, и движение мыши.
type leaveSpy struct {
	widget.Base
	leaves int
	moves  [][2]int
}

func (s *leaveSpy) Draw(ctx widget.DrawContext) {}
func (s *leaveSpy) OnPointerLeave()             { s.leaves++ }
func (s *leaveSpy) OnMouseMove(x, y int)        { s.moves = append(s.moves, [2]int{x, y}) }

func rawRoot(t *testing.T) (*Engine, *widget.Panel) {
	t.Helper()
	e := New(300, 300, 20)
	e.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 10, G: 10, B: 10, A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 300))
	e.SetRoot(root)
	return e, root
}

// Сырое колесо доходит до виджета под курсором с дельтой как есть.
func TestWheelRaw_ReachesWidgetUnderCursor(t *testing.T) {
	testreset.Globals(t)
	e, root := rawRoot(t)
	defer e.Stop()

	spy := &rawWheelSpy{take: true}
	spy.SetBounds(image.Rect(20, 20, 220, 120))
	root.AddChild(spy)

	e.SetModifiers(widget.ModCtrl)
	if !e.SendMouseWheelRaw(100, 60, 0, -120, false) {
		t.Fatal("принятое событие не вернуло true")
	}
	if len(spy.events) != 1 {
		t.Fatalf("событий %d, ждал 1", len(spy.events))
	}
	got := spy.events[0]
	want := widget.WheelEvent{X: 100, Y: 60, DeltaY: -120, Mod: widget.ModCtrl}
	if got != want {
		t.Errorf("получено %+v, ждал %+v", got, want)
	}

	// Мимо виджета — ему ничего не приходит, результат false.
	spy.events = nil
	if e.SendMouseWheelRaw(250, 250, 0, 120, false) {
		t.Error("колесо мимо виджета сочтено принятым")
	}
	if len(spy.events) != 0 {
		t.Errorf("виджет получил событие мимо себя: %+v", spy.events)
	}

	// Нулевая дельта — не событие.
	if e.SendMouseWheelRaw(100, 60, 0, 0, true) || len(spy.events) != 0 {
		t.Error("нулевая дельта доставлена")
	}
}

// Горизонталь и признак точности доходят до виджета как есть.
func TestWheelRaw_HorizontalAndPrecise(t *testing.T) {
	testreset.Globals(t)
	e, root := rawRoot(t)
	defer e.Stop()

	spy := &rawWheelSpy{take: false}
	spy.SetBounds(image.Rect(10, 10, 100, 100))
	root.AddChild(spy)

	if e.SendMouseWheelRaw(50, 50, 30, 0, true) {
		t.Error("никто не принимал, а результат true")
	}
	if len(spy.events) != 1 {
		t.Fatalf("получено %d событий", len(spy.events))
	}
	if ev := spy.events[0]; ev.DeltaX != 30 || ev.DeltaY != 0 || !ev.Precise {
		t.Errorf("горизонталь/точность потеряны: %+v", ev)
	}
}

// Всплытие: внутренний не принял — событие идёт внешнему.
func TestWheelRaw_ParentGetsWhatChildDeclines(t *testing.T) {
	testreset.Globals(t)
	e, _ := rawRoot(t)
	defer e.Stop()

	parent := &rawWheelContainer{}
	parent.take = true
	parent.SetBounds(image.Rect(0, 0, 300, 300))
	child := &rawWheelSpy{take: false}
	child.SetBounds(image.Rect(10, 10, 100, 100))
	parent.kids = []widget.Widget{child}
	e.SetRoot(parent)

	if !e.SendMouseWheelRaw(50, 50, 0, 120, false) {
		t.Fatal("родитель принял, а результат false")
	}
	if len(child.events) != 1 || len(parent.events) != 1 {
		t.Errorf("ребёнок получил %d, родитель %d; ждал по одному", len(child.events), len(parent.events))
	}
}

// rawWheelContainer — контейнер, принимающий сырое колесо.
type rawWheelContainer struct {
	rawWheelSpy
	kids []widget.Widget
}

func (c *rawWheelContainer) Children() []widget.Widget { return c.kids }

// Захватившему мышь событие идёт, где бы ни был курсор.
func TestWheelRaw_CapturedWidgetGetsIt(t *testing.T) {
	testreset.Globals(t)
	e, root := rawRoot(t)
	defer e.Stop()

	spy := &rawWheelSpy{take: true}
	spy.SetBounds(image.Rect(20, 20, 60, 60))
	root.AddChild(spy)
	e.SetCapture(spy)

	if !e.SendMouseWheelRaw(250, 250, 0, 120, false) || len(spy.events) != 1 {
		t.Errorf("захватчик не получил колесо: %+v", spy.events)
	}
}

// HiDPI: точка переводится в логическую, дельта — нет (это не пиксели).
func TestWheelRaw_ScaleAffectsPointNotDelta(t *testing.T) {
	testreset.Globals(t)
	e, root := rawRoot(t)
	defer e.Stop()
	e.SetScale(2)

	spy := &rawWheelSpy{take: true}
	spy.SetBounds(image.Rect(20, 20, 220, 120))
	root.AddChild(spy)

	e.SendMouseWheelRaw(100, 60, 0, 120, false)
	if len(spy.events) != 1 {
		t.Fatalf("событий %d", len(spy.events))
	}
	if ev := spy.events[0]; ev.X != 50 || ev.Y != 30 || ev.DeltaY != 120 {
		t.Errorf("при масштабе 2: %+v, ждал X=50 Y=30 DeltaY=120", ev)
	}
}

// Прежний пиксельный путь работает рядом: виджет без WheelEventHandler
// продолжает получать пиксели, а сырой вызов его не задевает.
func TestWheelRaw_PixelPathStillWorks(t *testing.T) {
	testreset.Globals(t)
	e, root := rawRoot(t)
	defer e.Stop()

	items := make([]string, 40)
	for i := range items {
		items[i] = "item"
	}
	lv := widget.NewListView(items...)
	lv.SetBounds(image.Rect(20, 20, 220, 120))
	root.AddChild(lv)

	if e.SendMouseWheelRaw(100, 60, 0, 120, false) {
		t.Error("список без WheelEventHandler принял сырое колесо")
	}
	if lv.ScrollY() != 0 {
		t.Error("сырой вызов прокрутил виджет, не знающий WheelEvent")
	}
	e.SendMouseWheelPixels(100, 60, 0, 120)
	if lv.ScrollY() <= 0 {
		t.Error("пиксельный путь перестал работать")
	}
}

// ─── Уход указателя ─────────────────────────────────────────────────────────

func TestSendPointerLeave_NotifiesVisibleHandlers(t *testing.T) {
	testreset.Globals(t)
	e, root := rawRoot(t)
	defer e.Stop()

	a := &leaveSpy{}
	a.SetBounds(image.Rect(0, 0, 100, 100))
	b := &leaveSpy{}
	b.SetBounds(image.Rect(100, 0, 200, 100))
	hidden := &leaveSpy{}
	hidden.SetBounds(image.Rect(200, 0, 300, 100))
	hidden.SetVisible(false)
	root.AddChild(a)
	root.AddChild(b)
	root.AddChild(hidden)

	e.SendPointerLeave()
	if a.leaves != 1 || b.leaves != 1 {
		t.Errorf("уходов: a=%d b=%d, ждал по одному", a.leaves, b.leaves)
	}
	if hidden.leaves != 0 {
		t.Error("невидимому виджету сообщили об уходе")
	}
}

// Движение в точку «-1» не превращается в 0 на HiDPI: усечение к нулю делало
// из точки вне окна первую точку окна, и наведение не снималось.
func TestMouseMove_OutsidePointStaysOutsideOnHiDPI(t *testing.T) {
	testreset.Globals(t)
	e, root := rawRoot(t)
	defer e.Stop()
	e.SetScale(2)

	spy := &leaveSpy{}
	spy.SetBounds(image.Rect(0, 0, 100, 100))
	root.AddChild(spy)

	e.SendMouseMove(20, 20)
	e.SendMouseMove(-1, -1)
	if n := len(spy.moves); n == 0 || spy.moves[n-1] != [2]int{-1, -1} {
		t.Errorf("движения %v, последнее должно быть (-1,-1)", spy.moves)
	}

	// Положительные координаты округляются как раньше.
	spy.moves = nil
	e.SendMouseMove(41, 21)
	if n := len(spy.moves); n == 0 || spy.moves[n-1] != [2]int{20, 10} {
		t.Errorf("движения %v, последнее должно быть (20,10)", spy.moves)
	}
}

// ─── Клавиши: скан-код через движок ─────────────────────────────────────────

// keySpy — виджет с фокусом, копящий клавиатурные события.
type keySpy struct {
	widget.Base
	focused bool
	keys    []widget.KeyEvent
}

func (s *keySpy) Draw(ctx widget.DrawContext)  {}
func (s *keySpy) SetFocused(f bool)            { s.focused = f }
func (s *keySpy) IsFocused() bool              { return s.focused }
func (s *keySpy) OnKeyEvent(e widget.KeyEvent) { s.keys = append(s.keys, e) }

// Скан-код, признак расширенной клавиши и автоповтор доходят до сфокусированного
// виджета без изменений; клавиша без имени приходит как KeyUnknown.
func TestSendKeyEvent_ScanCodeReachesFocusedWidget(t *testing.T) {
	testreset.Globals(t)
	e, root := rawRoot(t)
	defer e.Stop()

	spy := &keySpy{}
	spy.SetBounds(image.Rect(0, 0, 100, 100))
	root.AddChild(spy)
	e.SetFocus(spy)

	evs := []widget.KeyEvent{
		{Code: widget.KeyRight, Pressed: true, Repeat: true, ScanCode: 0x4D, Extended: true},
		{Code: widget.KeyUnknown, Pressed: true, ScanCode: 0x7E},
		{Code: widget.KeyVolumeUp, Pressed: true, ScanCode: 115},
		{Code: widget.KeyShift, Mod: widget.ModShift, Pressed: true, ScanCode: 0x2A},
	}
	for _, ev := range evs {
		e.SendKeyEvent(ev)
	}
	if len(spy.keys) != len(evs) {
		t.Fatalf("дошло %d событий из %d", len(spy.keys), len(evs))
	}
	for i, ev := range evs {
		if spy.keys[i] != ev {
			t.Errorf("событие %d: %+v, ждал %+v", i, spy.keys[i], ev)
		}
	}
}
