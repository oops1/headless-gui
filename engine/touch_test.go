package engine

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// touchSpy — виджет-приёмник касаний: пишет всё, что получил, и берёт касание
// по команде claim.
type touchSpy struct {
	widget.Base
	claim bool
	got   []widget.TouchEvent
	dis   int
}

func newTouchSpy(r image.Rectangle, claim bool) *touchSpy {
	s := &touchSpy{claim: claim}
	s.SetBounds(r)
	return s
}

func (s *touchSpy) Draw(widget.DrawContext) {}
func (s *touchSpy) OnTouch(ev widget.TouchEvent) bool {
	s.got = append(s.got, ev)
	return s.claim
}
func (s *touchSpy) Dismiss() { s.dis++ }

func (s *touchSpy) phases() []widget.TouchPhase {
	var out []widget.TouchPhase
	for _, e := range s.got {
		out = append(out, e.Phase)
	}
	return out
}

func touchRoot() *widget.Panel {
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))
	return root
}

func tch(id, x, y int, ph widget.TouchPhase) widget.TouchEvent {
	return widget.TouchEvent{ID: id, X: x, Y: y, Phase: ph}
}

// Захват по ID: владелец получает весь разговор, даже когда палец ушёл с его
// площади на соседа.
func TestSendTouch_CaptureByID(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := touchRoot()
	a := newTouchSpy(image.Rect(0, 0, 200, 300), true)
	b := newTouchSpy(image.Rect(200, 0, 400, 300), true)
	root.AddChild(a)
	root.AddChild(b)
	e.SetRoot(root)

	if !e.SendTouch(tch(1, 50, 50, widget.TouchBegan)) {
		t.Fatal("Began на виджете, берущем касание, вернул false")
	}
	// Палец ушёл на b.
	if !e.SendTouch(tch(1, 300, 60, widget.TouchMoved)) {
		t.Fatal("Moved владельцу вернул false")
	}
	if len(b.got) != 0 {
		t.Errorf("сосед получил чужое касание: %v", b.got)
	}
	if !e.SendTouch(tch(1, 310, 70, widget.TouchEnded)) {
		t.Fatal("Ended владельцу вернул false")
	}
	want := []widget.TouchPhase{widget.TouchBegan, widget.TouchMoved, widget.TouchEnded}
	if got := a.phases(); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("фазы владельца %v, ждал %v", got, want)
	}
	// Координаты Moved — те же, что отправили (вне площади владельца).
	if a.got[1].X != 300 || a.got[1].Y != 60 {
		t.Errorf("координаты Moved %d,%d", a.got[1].X, a.got[1].Y)
	}
	if e.TouchActive() != 0 {
		t.Errorf("после Ended владелец остался: %d", e.TouchActive())
	}
	// Касание закончилось: запоздалый Moved уже никому не принадлежит.
	if e.SendTouch(tch(1, 10, 10, widget.TouchMoved)) {
		t.Error("Moved после Ended принят")
	}
	if len(a.got) != 3 {
		t.Errorf("после Ended владелец получил ещё: %v", a.phases())
	}
}

// Два пальца на разных виджетах и два пальца на одном: у каждого пальца
// свой разговор.
func TestSendTouch_TwoFingers(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := touchRoot()
	a := newTouchSpy(image.Rect(0, 0, 200, 300), true)
	b := newTouchSpy(image.Rect(200, 0, 400, 300), true)
	root.AddChild(a)
	root.AddChild(b)
	e.SetRoot(root)

	e.SendTouch(tch(1, 50, 50, widget.TouchBegan))
	e.SendTouch(tch(2, 250, 50, widget.TouchBegan))
	e.SendTouch(tch(2, 260, 55, widget.TouchMoved))
	e.SendTouch(tch(1, 60, 55, widget.TouchMoved))
	if e.TouchActive() != 2 {
		t.Fatalf("активных касаний %d, ждал 2", e.TouchActive())
	}
	e.SendTouch(tch(1, 60, 55, widget.TouchEnded))
	if e.TouchActive() != 1 {
		t.Fatalf("после отрыва первого активных %d", e.TouchActive())
	}
	e.SendTouch(tch(2, 260, 55, widget.TouchEnded))

	for _, ev := range a.got {
		if ev.ID != 1 {
			t.Errorf("a получил палец %d", ev.ID)
		}
	}
	for _, ev := range b.got {
		if ev.ID != 2 {
			t.Errorf("b получил палец %d", ev.ID)
		}
	}
	if len(a.got) != 3 || len(b.got) != 3 {
		t.Errorf("a: %v, b: %v", a.phases(), b.phases())
	}

	// Оба пальца на одном виджете — щипок.
	e.SendTouch(tch(7, 20, 20, widget.TouchBegan))
	e.SendTouch(tch(8, 120, 20, widget.TouchBegan))
	e.SendTouch(tch(7, 10, 20, widget.TouchMoved))
	e.SendTouch(tch(8, 140, 20, widget.TouchMoved))
	n := len(a.got)
	ids := map[int]int{}
	for _, ev := range a.got[n-4:] {
		ids[ev.ID]++
	}
	if ids[7] != 2 || ids[8] != 2 {
		t.Errorf("щипок на одном виджете: события по пальцам %v", ids)
	}
}

// Всплытие: глубокий виджет, не взявший касание, передаёт его родителю.
func TestSendTouch_BubblesToParent(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := touchRoot()
	parent := newTouchSpy(image.Rect(0, 0, 400, 300), true)
	child := newTouchSpy(image.Rect(10, 10, 100, 100), false)
	parent.AddChild(child)
	root.AddChild(parent)
	e.SetRoot(root)

	if !e.SendTouch(tch(1, 50, 50, widget.TouchBegan)) {
		t.Fatal("касание не принято родителем")
	}
	if len(child.got) != 1 || len(parent.got) != 1 {
		t.Fatalf("ребёнок %v, родитель %v", child.phases(), parent.phases())
	}
	// Дальше ребёнок молчит: касание у родителя.
	e.SendTouch(tch(1, 55, 55, widget.TouchMoved))
	e.SendTouch(tch(1, 55, 55, widget.TouchEnded))
	if len(child.got) != 1 {
		t.Errorf("отказавшийся ребёнок получил продолжение: %v", child.phases())
	}
	if len(parent.got) != 3 {
		t.Errorf("родитель получил %v", parent.phases())
	}
}

// Если взять касание некому, SendTouch возвращает false, ничего не делая с
// мышью: превращение в мышь — не дело движка.
func TestSendTouch_UnclaimedIsNotMouse(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := touchRoot()
	var clicks int
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(0, 0, 200, 100))
	btn.OnClick = func() { clicks++ }
	root.AddChild(btn)
	spy := newTouchSpy(image.Rect(200, 0, 400, 100), false)
	root.AddChild(spy)
	e.SetRoot(root)

	// Кнопка не TouchHandler.
	if e.SendTouch(tch(1, 50, 50, widget.TouchBegan)) {
		t.Error("касание принято виджетом без TouchHandler")
	}
	e.SendTouch(tch(1, 50, 50, widget.TouchEnded))
	if clicks != 0 {
		t.Error("движок превратил касание в нажатие мыши")
	}
	// Виджет, отказавшийся от касания, тоже ничьё.
	if e.SendTouch(tch(2, 250, 50, widget.TouchBegan)) {
		t.Error("касание принято, хотя виджет вернул false")
	}
	if e.TouchActive() != 0 {
		t.Errorf("владельцев %d", e.TouchActive())
	}
	// Point вне дерева.
	if e.SendTouch(tch(3, 900, 900, widget.TouchBegan)) {
		t.Error("касание вне дерева принято")
	}
}

// Корень, берущий касания, получает те, что не взял никто глубже.
func TestSendTouch_RootHandler(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := newRootSpy()
	e.SetRoot(root)
	if !e.SendTouch(tch(1, 10, 10, widget.TouchBegan)) {
		t.Fatal("корень не получил касание")
	}
	if len(root.got) != 1 {
		t.Errorf("корень: %v", root.got)
	}
}

type rootSpy struct {
	widget.Panel
	got []widget.TouchEvent
}

func newRootSpy() *rootSpy {
	r := &rootSpy{}
	r.SetBounds(image.Rect(0, 0, 400, 300))
	return r
}
func (r *rootSpy) OnTouch(ev widget.TouchEvent) bool {
	r.got = append(r.got, ev)
	return true
}

// Физические пиксели -> логические, и кадр прокрутки.
func TestSendTouch_LogicalAndFrame(t *testing.T) {
	testreset.Globals(t)
	e := New(300, 400, 20)
	e.SetScale(2)
	root, sv, _ := frameScene(t, 200)
	spy := newTouchSpy(image.Rect(10, 200, 200, 230), true)
	sv.AddChild(spy)
	e.SetRoot(root)

	// Логическая точка (50, 15) -> физическая (100, 30). В содержимом
	// прокрутки это (50, 215).
	if !e.SendTouch(tch(1, 100, 30, widget.TouchBegan)) {
		t.Fatal("касание не принято")
	}
	if got := spy.got[0]; got.X != 50 || got.Y != 215 {
		t.Errorf("виджет получил %d,%d, ждал 50,215 (логические, в кадре содержимого)", got.X, got.Y)
	}
	// Прокрутка сдвинулась посреди касания: Moved приходит уже в новом кадре.
	sv.SetScrollY(100)
	e.SendTouch(tch(1, 100, 30, widget.TouchMoved))
	if got := spy.got[1]; got.X != 50 || got.Y != 115 {
		t.Errorf("после сдвига прокрутки Moved %d,%d, ждал 50,115", got.X, got.Y)
	}
}

// Касание получает время: событие без штампа движок помечает сам, штамп
// источника не трогает.
func TestSendTouch_Timestamp(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := touchRoot()
	a := newTouchSpy(image.Rect(0, 0, 400, 300), true)
	root.AddChild(a)
	e.SetRoot(root)

	e.SendTouch(tch(1, 5, 5, widget.TouchBegan))
	ev := tch(1, 6, 6, widget.TouchMoved)
	ev.Time = 12345
	e.SendTouch(ev)
	if a.got[0].Time == 0 {
		t.Error("движок не проставил время")
	}
	if a.got[1].Time != 12345 {
		t.Errorf("штамп источника изменён: %v", a.got[1].Time)
	}
}

// Повторный Began с занятым ID сообщает прежнему владельцу об отмене.
func TestSendTouch_DuplicateBeganCancelsOld(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := touchRoot()
	a := newTouchSpy(image.Rect(0, 0, 200, 300), true)
	b := newTouchSpy(image.Rect(200, 0, 400, 300), true)
	root.AddChild(a)
	root.AddChild(b)
	e.SetRoot(root)

	e.SendTouch(tch(1, 50, 50, widget.TouchBegan))
	e.SendTouch(tch(1, 250, 50, widget.TouchBegan))
	if got := a.phases(); len(got) != 2 || got[1] != widget.TouchCancelled {
		t.Errorf("прежний владелец: %v, ждал Began, Cancelled", got)
	}
	if got := b.phases(); len(got) != 1 || got[0] != widget.TouchBegan {
		t.Errorf("новый владелец: %v", got)
	}
}

func TestCancelTouches(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := touchRoot()
	a := newTouchSpy(image.Rect(0, 0, 400, 300), true)
	root.AddChild(a)
	e.SetRoot(root)

	e.SendTouch(tch(1, 40, 50, widget.TouchBegan))
	e.SendTouch(tch(2, 90, 60, widget.TouchBegan))
	e.CancelTouches()
	if e.TouchActive() != 0 {
		t.Fatalf("после отмены активных %d", e.TouchActive())
	}
	var cancelled int
	for _, ev := range a.got {
		if ev.Phase == widget.TouchCancelled {
			cancelled++
			if (ev.ID == 1 && (ev.X != 40 || ev.Y != 50)) || (ev.ID == 2 && (ev.X != 90 || ev.Y != 60)) {
				t.Errorf("отмена пальца %d в точке %d,%d", ev.ID, ev.X, ev.Y)
			}
		}
	}
	if cancelled != 2 {
		t.Errorf("отмен %d, ждал 2", cancelled)
	}
}

// Открытый оверлей старше дерева: касание по нему не достаётся виджету под
// ним — иначе меню, открытое над просмотрщиком, не работало бы пальцем.
func TestSendTouch_OverlayShadowsTree(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := touchRoot()
	under := newTouchSpy(image.Rect(0, 0, 400, 300), true)
	root.AddChild(under)
	// Хозяин оверлея стоит в углу, а сам оверлей накрывает середину окна: точка
	// внутри него по дереву принадлежит только «under».
	ov := &touchOverlay{}
	ov.SetBounds(image.Rect(10, 10, 50, 30))
	ov.rect = image.Rect(0, 100, 200, 200)
	root.AddChild(ov)
	e.SetRoot(root)

	if e.SendTouch(tch(1, 50, 150, widget.TouchBegan)) {
		t.Error("касание по оверлею принято виджетом под ним")
	}
	if len(under.got) != 0 {
		t.Errorf("виджет под оверлеем получил касание: %v", under.phases())
	}
	// Мимо оверлея дерево работает как обычно.
	if !e.SendTouch(tch(2, 300, 250, widget.TouchBegan)) {
		t.Error("касание вне оверлея не дошло до дерева")
	}

	// Оверлей, сам умеющий касания, берёт своё.
	ov.claim = true
	if !e.SendTouch(tch(3, 60, 120, widget.TouchBegan)) || len(ov.got) != 2 || ov.got[1].ID != 3 {
		t.Errorf("оверлей-TouchHandler не получил касание: %v", ov.got)
	}
}

// touchOverlay — виджет с открытым оверлеем вне собственных границ.
type touchOverlay struct {
	widget.Base
	rect  image.Rectangle
	claim bool
	got   []widget.TouchEvent
}

func (o *touchOverlay) Draw(widget.DrawContext)        {}
func (o *touchOverlay) HasOverlay() bool               { return true }
func (o *touchOverlay) DrawOverlay(widget.DrawContext) {}
func (o *touchOverlay) OverlayBounds() image.Rectangle { return o.rect }
func (o *touchOverlay) OnTouch(ev widget.TouchEvent) bool {
	o.got = append(o.got, ev)
	return o.claim
}

// Принятое касание закрывает всплывающие панели вне своего пути, как нажатие.
func TestSendTouch_DismissesOutside(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := touchRoot()
	a := newTouchSpy(image.Rect(0, 0, 200, 300), true)
	other := newTouchSpy(image.Rect(200, 0, 400, 300), true)
	root.AddChild(a)
	root.AddChild(other)
	e.SetRoot(root)

	e.SendTouch(tch(1, 50, 50, widget.TouchBegan))
	if other.dis != 1 {
		t.Errorf("постороннее Dismissable закрыто %d раз", other.dis)
	}
	if a.dis != 0 {
		t.Errorf("владелец касания закрыт сам: %d", a.dis)
	}
}

// Модальный виджет ограничивает доставку: касание вне его не уходит дереву.
func TestSendTouch_ModalScope(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	root := touchRoot()
	under := newTouchSpy(image.Rect(0, 0, 400, 300), true)
	root.AddChild(under)
	e.SetRoot(root)

	modal := &touchModal{}
	modal.SetBounds(image.Rect(100, 100, 200, 200))
	e.ShowModal(modal)

	if e.SendTouch(tch(1, 10, 10, widget.TouchBegan)) {
		t.Error("касание вне модалки ушло в дерево под ней")
	}
	if len(under.got) != 0 {
		t.Errorf("дерево под модалкой получило %v", under.phases())
	}
}

type touchModal struct {
	widget.Panel
}

func (m *touchModal) IsModal() bool        { return true }
func (m *touchModal) DimColor() color.RGBA { return color.RGBA{A: 100} }

// viewer — виджет «просмотрщик»: жесты через готовый widget.TouchGestures.
type viewer struct {
	widget.Base
	widget.TouchGestures
}

func (v *viewer) Draw(widget.DrawContext) {}

// Сквозной путь: два пальца -> движок -> владелец -> распознаватель -> Pinch.
// Пороги распознавателя логические: на HiDPI x2 физическое смещение в 8 пикселей
// — это 4 логических, и щипком оно ещё не считается.
func TestSendTouch_PinchThroughEngineHiDPI(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 20)
	e.SetScale(2)
	root := touchRoot()
	v := &viewer{}
	v.SetBounds(image.Rect(0, 0, 400, 300))
	var pinches []widget.Pinch
	var swipes []widget.Swipe
	v.OnGesture = func(g widget.Gesture) {
		switch g := g.(type) {
		case widget.Pinch:
			pinches = append(pinches, g)
		case widget.Swipe:
			swipes = append(swipes, g)
		}
	}
	root.AddChild(v)
	e.SetRoot(root)

	send := func(id, x, y int, ph widget.TouchPhase, ms int) {
		e.SendTouch(widget.TouchEvent{ID: id, X: x, Y: y, Phase: ph, Time: time.Duration(ms) * time.Millisecond})
	}

	// Физические координаты; логические — вдвое меньше. Расстояние 200 -> 100.
	send(1, 200, 300, widget.TouchBegan, 1)
	send(2, 400, 300, widget.TouchBegan, 1)
	send(2, 408, 300, widget.TouchMoved, 16) // +4 логических: ниже порога щипка
	if len(pinches) != 0 {
		t.Fatalf("порог в логических пикселях нарушен: %+v", pinches)
	}
	send(2, 440, 300, widget.TouchMoved, 32) // расстояние 240 физ. = 120 лог.
	send(2, 440, 300, widget.TouchEnded, 48)
	send(1, 200, 300, widget.TouchEnded, 48)
	if len(pinches) < 2 {
		t.Fatalf("щипок не распознан: %+v", pinches)
	}
	if got := pinches[0].Scale; got < 1.19 || got > 1.21 {
		t.Errorf("масштаб %.3f, ждал 1.2 (120/100)", got)
	}

	// Свайп одним пальцем: 300 физ. (150 лог.) за ~96 мс — быстрее порога.
	send(5, 600, 400, widget.TouchBegan, 100)
	for i := 1; i <= 6; i++ {
		send(5, 600-50*i, 400, widget.TouchMoved, 100+16*i)
	}
	send(5, 300, 400, widget.TouchEnded, 100+16*6+4)
	if len(swipes) != 1 || swipes[0].Dir != widget.SwipeLeft {
		t.Errorf("свайп: %+v", swipes)
	}
}

// Нажим и площадь контакта доходят до виджета: нажим как есть, оси контакта —
// в логических пикселях (как и точка), угол и нажим масштаб не трогает.
func TestSendTouch_PressureAndContact(t *testing.T) {
	testreset.Globals(t)
	for _, scale := range []float64{1, 2} {
		e := New(400, 300, 20)
		e.SetScale(scale)
		root := touchRoot()
		a := newTouchSpy(image.Rect(0, 0, 400, 300), true)
		root.AddChild(a)
		e.SetRoot(root)

		ev := tch(1, 40, 40, widget.TouchBegan)
		ev.Pressure, ev.Major, ev.Minor, ev.Orientation = 0.5, 40, 20, 30
		e.SendTouch(ev)
		got := a.got[0]
		if got.Pressure != 0.5 || got.Orientation != 30 {
			t.Errorf("scale %v: нажим/угол %v/%v, ждал 0.5/30", scale, got.Pressure, got.Orientation)
		}
		if got.Major != 40/scale || got.Minor != 20/scale {
			t.Errorf("scale %v: оси %v x %v, ждал %v x %v", scale, got.Major, got.Minor, 40/scale, 20/scale)
		}
		if !got.HasPressure() || !got.HasContact() {
			t.Errorf("scale %v: HasPressure/HasContact = %v/%v", scale, got.HasPressure(), got.HasContact())
		}
	}
	// Без сведений — нули, а не «лёгкое касание».
	e := New(400, 300, 20)
	root := touchRoot()
	a := newTouchSpy(image.Rect(0, 0, 400, 300), true)
	root.AddChild(a)
	e.SetRoot(root)
	e.SendTouch(tch(1, 5, 5, widget.TouchBegan))
	if g := a.got[0]; g.HasPressure() || g.HasContact() {
		t.Errorf("неизвестные нажим/контакт стали известными: %+v", g)
	}
}
