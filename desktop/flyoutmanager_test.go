package desktop

import (
	"image"
	"image/color"
	"reflect"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Менеджер всплывающих панелей.

// recorder собирает события менеджера строками «Имя:Opened».
type recorder struct{ log []string }

func (r *recorder) handle(e FlyoutEvent) {
	k := "Opened"
	if e.Kind == FlyoutClosed {
		k = "Closed"
	}
	r.log = append(r.log, e.Name+":"+k)
}

func managerFixture(t *testing.T, themeName string, names ...string) (*FlyoutManager, map[string]*Flyout, *recorder) {
	t.Helper()
	tm := managerFor(t, themeName)
	m := NewFlyoutManager()
	rec := &recorder{}
	m.Subscribe(rec.handle)
	panels := map[string]*Flyout{}
	for _, n := range names {
		f := motionFlyout(tm)
		panels[n] = f
		m.Register(n, f)
	}
	return m, panels, rec
}

func TestFlyoutManager_OpeningOneClosesTheOthers(t *testing.T) {
	defer widget.StopAllAnimations()
	m, p, rec := managerFixture(t, theme.ProfileWindows2000, "start", "quick")

	var shell []string
	p["start"].OnOpen = func() { shell = append(shell, "start open") }
	p["start"].OnClose = func() { shell = append(shell, "start close") }
	p["quick"].OnOpen = func() { shell = append(shell, "quick open") }
	p["quick"].OnClose = func() { shell = append(shell, "quick close") }

	if !m.Open("start", motionAnchor) {
		t.Fatal("Open неизвестной панели не должен удаваться, а известной — должен")
	}
	m.Open("quick", motionAnchor)

	if p["start"].IsOpen() {
		t.Error("«Пуск» остался открытым после открытия быстрых настроек")
	}
	if !p["quick"].IsOpen() {
		t.Error("быстрые настройки не открылись")
	}
	want := []string{"start:Opened", "start:Closed", "quick:Opened"}
	if !reflect.DeepEqual(rec.log, want) {
		t.Errorf("события %v, ждали %v", rec.log, want)
	}
	// Оболочка получает свои колбэки: старая панель гаснет раньше, чем
	// зажигается новая.
	wantShell := []string{"start open", "start close", "quick open"}
	if !reflect.DeepEqual(shell, wantShell) {
		t.Errorf("колбэки оболочки %v, ждали %v", shell, wantShell)
	}
	if got := m.Opened(); !reflect.DeepEqual(got, []string{"quick"}) {
		t.Errorf("открыты %v, ждали [quick]", got)
	}
}

func TestFlyoutManager_ClosedByAnythingIsReported(t *testing.T) {
	defer widget.StopAllAnimations()
	m, p, rec := managerFixture(t, theme.ProfileWindows2000, "start")
	m.Open("start", motionAnchor)

	// Клик мимо — со стороны движка, не через менеджер.
	p["start"].DismissAt(790, 10)
	if p["start"].IsOpen() {
		t.Fatal("клик мимо не закрыл панель")
	}
	if want := []string{"start:Opened", "start:Closed"}; !reflect.DeepEqual(rec.log, want) {
		t.Errorf("события %v, ждали %v", rec.log, want)
	}
}

func TestFlyoutManager_GroupOpensTogether(t *testing.T) {
	defer widget.StopAllAnimations()
	m, p, _ := managerFixture(t, theme.ProfileWindows2000, "calendar", "notifications", "start")
	g := m.Group("calendar", "notifications")

	m.Open("start", motionAnchor)
	g.OpenAll(motionAnchor)

	if !p["calendar"].IsOpen() || !p["notifications"].IsOpen() {
		t.Error("панели одной группы закрыли друг друга")
	}
	if p["start"].IsOpen() {
		t.Error("«Пуск» остался открытым рядом с группой")
	}
	// Esc закрывает группу целиком.
	if !widget.DismissOnEscape(m) {
		t.Fatal("Esc ничего не закрыл")
	}
	if p["calendar"].IsOpen() || p["notifications"].IsOpen() {
		t.Error("Esc закрыл не всю группу")
	}
}

// customPanel — панель потребителя: свой тип, встраивающий *Flyout, со своим
// Open.
type customPanel struct {
	*Flyout
	opens int
}

func (c *customPanel) Open(a image.Rectangle) {
	c.opens++
	c.Flyout.Open(a)
}

func TestFlyoutManager_ConsumerCanRegisterOwnPanel(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows2000)
	m := NewFlyoutManager()
	own := &customPanel{Flyout: motionFlyout(tm)}
	other := motionFlyout(tm)
	m.Register("own", own)
	m.Register("other", other)

	m.Open("own", motionAnchor)
	if own.opens != 1 {
		t.Errorf("менеджер открыл панель мимо её Open: %d вызовов", own.opens)
	}
	m.Open("other", motionAnchor)
	if own.IsOpen() {
		t.Error("своя панель не закрылась при открытии другой")
	}
	if m.Panel("own") != FlyoutPanel(own) {
		t.Error("Panel вернул не тот виджет, что регистрировали")
	}
	if got := m.Names(); !reflect.DeepEqual(got, []string{"own", "other"}) {
		t.Errorf("Names = %v", got)
	}
}

func TestFlyoutManager_Unregister(t *testing.T) {
	defer widget.StopAllAnimations()
	m, p, rec := managerFixture(t, theme.ProfileWindows2000, "a", "b")
	unreg := m.Register("c", motionFlyout(managerFor(t, theme.ProfileWindows2000)))
	if len(m.Children()) != 3 {
		t.Fatalf("детей %d, ждали 3", len(m.Children()))
	}
	unreg()
	if len(m.Children()) != 2 || m.Panel("c") != nil {
		t.Error("панель не снялась с учёта")
	}

	// Снятая панель менеджер больше не слышит и не закрывает.
	m.Unregister("a")
	rec.log = nil
	p["a"].Open(motionAnchor)
	if len(rec.log) != 0 {
		t.Errorf("снятая панель шлёт события: %v", rec.log)
	}
	m.Open("b", motionAnchor)
	if !p["a"].IsOpen() {
		t.Error("менеджер закрыл панель, которой не управляет")
	}
}

func TestFlyoutManager_ReplacingNameDropsTheOld(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows2000)
	m := NewFlyoutManager()
	a, b := motionFlyout(tm), motionFlyout(tm)
	m.Register("x", a)
	m.Register("x", b)
	if m.Panel("x").AsFlyout() != b || len(m.Children()) != 1 {
		t.Error("повторная регистрация имени не заменила панель")
	}
}

func TestFlyoutManager_ToggleAndAnchorDismiss(t *testing.T) {
	defer widget.StopAllAnimations()
	m, p, _ := managerFixture(t, theme.ProfileWindows2000, "start")

	if !m.Toggle("start", motionAnchor) {
		t.Fatal("первый Toggle не открыл панель")
	}
	// Нажатие на её же кнопку: движок гасит панель на нажатии…
	p["start"].DismissAt(motionAnchor.Min.X+5, motionAnchor.Min.Y+5)
	if p["start"].IsOpen() {
		t.Fatal("нажатие на якорь не закрыло панель")
	}
	if !p["start"].DismissedByAnchor() {
		t.Error("закрытие нажатием на якорь не запомнилось")
	}
	// …а кнопка срабатывает на отпускании и просит Toggle. Заново панель
	// открывать нельзя — это тот же клик.
	if m.Toggle("start", motionAnchor) {
		t.Error("тот же клик открыл панель заново")
	}
	// Следующий клик — уже настоящий.
	if !m.Toggle("start", motionAnchor) {
		t.Error("следующий клик не открыл панель")
	}
}

func TestFlyoutManager_AnchorDismissIsForgottenByDeadline(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows2000)
	f := motionFlyout(tm)
	f.Open(motionAnchor)
	f.DismissAt(motionAnchor.Min.X+1, motionAnchor.Min.Y+1)
	// Состарим отметку: клик по якорю, не дошедший до Toggle, не должен съесть
	// настоящее открытие позже.
	f.anchorDismissAt.Store(time.Now().Add(-2 * anchorDismissWindow).UnixNano())
	f.Toggle(motionAnchor)
	if !f.IsOpen() {
		t.Error("устаревшая отметка съела настоящее открытие")
	}
}

func TestFlyout_ClickOutsideNotOnAnchorDoesNotMarkIt(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows2000)
	f := motionFlyout(tm)
	f.Open(motionAnchor)
	f.DismissAt(700, 20)
	if f.DismissedByAnchor() {
		t.Error("клик в сторону записан как клик по якорю")
	}
	f.Toggle(motionAnchor)
	if !f.IsOpen() {
		t.Error("Toggle после обычного клика мимо не открыл панель")
	}
}

// ─── В дереве движка ────────────────────────────────────────────────────────

// clickProbe — панель потребителя, считающая нажатия внутри себя.
type clickProbe struct {
	*Flyout
	clicks int
}

func (c *clickProbe) OnMouseButton(e widget.MouseEvent) bool {
	if c.IsOpen() && e.Pressed && image.Pt(e.X, e.Y).In(c.restRect()) {
		c.clicks++
		return true
	}
	return c.Flyout.OnMouseButton(e)
}

func engineFixture(t *testing.T) (*engine.Engine, *FlyoutManager, *clickProbe, *Flyout) {
	t.Helper()
	const w, h = 800, 600
	tm := managerFor(t, theme.ProfileWindows2000)

	root := widget.NewPanel(color.RGBA{R: 20, G: 40, B: 80, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	m := NewFlyoutManager()
	a := &clickProbe{Flyout: motionFlyout(tm)}
	b := motionFlyout(tm)
	m.Register("a", a)
	m.Register("b", b)
	root.AddChild(m)

	eng := engine.New(w, h, 60)
	eng.SetRoot(root)
	eng.RenderOnce()
	return eng, m, a, b
}

func TestFlyoutManager_EscapeClosesWithoutFocus(t *testing.T) {
	defer widget.StopAllAnimations()
	eng, m, _, _ := engineFixture(t)
	m.Open("a", motionAnchor)

	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if m.IsOpen("a") {
		t.Error("Esc не закрыл панель: у неё нет клавиатурного фокуса, но закрываться она обязана")
	}
}

func TestFlyoutManager_ClickOutsideClosesInsideDoesNot(t *testing.T) {
	defer widget.StopAllAnimations()
	eng, m, a, _ := engineFixture(t)
	m.Open("a", motionAnchor)
	eng.RenderOnce()

	rest := a.restRect()
	in := rest.Min.Add(image.Pt(20, 20))
	eng.SendMouseButton(in.X, in.Y, widget.MouseLeft, true)
	eng.SendMouseButton(in.X, in.Y, widget.MouseLeft, false)
	if !m.IsOpen("a") {
		t.Fatal("клик внутри закрыл панель")
	}
	if a.clicks != 1 {
		t.Errorf("панель получила %d нажатий внутри, ждали 1: события не доходят через менеджер", a.clicks)
	}

	eng.SendMouseButton(700, 20, widget.MouseLeft, true)
	eng.SendMouseButton(700, 20, widget.MouseLeft, false)
	if m.IsOpen("a") {
		t.Error("клик мимо не закрыл панель")
	}
}

func TestFlyoutManager_BoundsFollowOpenPanels(t *testing.T) {
	defer widget.StopAllAnimations()
	_, m, a, _ := engineFixture(t)
	if !m.Bounds().Empty() {
		t.Errorf("у менеджера без открытых панелей границы %v", m.Bounds())
	}
	m.Open("a", motionAnchor)
	if m.Bounds() != a.restRect() {
		t.Errorf("границы менеджера %v, ждали область открытой панели %v", m.Bounds(), a.restRect())
	}
}

// Слой над панелью задач: оверлей менеджера рисуется поверх соседа, стоящего
// в дереве раньше него.
func TestFlyoutManager_LayerIsAboveTheTaskbar(t *testing.T) {
	defer widget.StopAllAnimations()
	const w, h = 800, 600
	tm := managerFor(t, theme.ProfileWindows2000)

	root := widget.NewPanel(color.RGBA{R: 20, G: 40, B: 80, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	// «Панель задач» — полоса, заходящая под область панели.
	bar := newWallPanel(image.Rect(0, 400, w, h))
	root.AddChild(bar)

	m := NewFlyoutManager()
	f := motionFlyout(tm)
	f.Margin = -100 // панель заходит на полосу
	m.Register("start", f)
	root.AddChild(m)

	eng := engine.New(w, h, 60)
	eng.SetRoot(root)
	before := eng.RenderOnce()
	m.Open("start", motionAnchor)
	after := eng.RenderOnce()

	r := f.restRect().Intersect(image.Rect(0, 400, w, h))
	if r.Empty() {
		t.Fatalf("панель %v не заходит на полосу — тест не проверяет слой", f.restRect())
	}
	p := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	if before.RGBAAt(p.X, p.Y) == after.RGBAAt(p.X, p.Y) {
		t.Error("панель не перекрыла полосу — слой лежит под панелью задач")
	}
}

// Повторный клик по кнопке, открывшей панель: движок гасит панель на нажатии
// (клик мимо неё), а кнопка срабатывает на отпускании. Панель должна остаться
// закрытой, а не открыться заново.
func TestFlyoutManager_ClickOnAnchorOfOpenPanelClosesIt(t *testing.T) {
	defer widget.StopAllAnimations()
	const w, h = 800, 600
	tm := managerFor(t, theme.ProfileWindows2000)

	root := widget.NewPanel(color.RGBA{R: 20, G: 40, B: 80, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	m := NewFlyoutManager()
	m.Register("start", motionFlyout(tm))
	btn := NewStartButton(tm)
	btn.SetBounds(motionAnchor)
	btn.OnClick = func() { m.Toggle("start", motionAnchor) }
	root.AddChild(btn)
	root.AddChild(m)

	eng := engine.New(w, h, 60)
	eng.SetRoot(root)
	eng.RenderOnce()

	click := func() {
		x, y := motionAnchor.Min.X+10, motionAnchor.Min.Y+10
		eng.SendMouseButton(x, y, widget.MouseLeft, true)
		eng.SendMouseButton(x, y, widget.MouseLeft, false)
	}

	click()
	if !m.IsOpen("start") {
		t.Fatal("первый клик по кнопке не открыл панель")
	}
	click()
	if m.IsOpen("start") {
		t.Fatal("повторный клик по кнопке закрыл панель на нажатии и открыл её заново на отпускании")
	}
	click()
	if !m.IsOpen("start") {
		t.Error("третий клик не открыл панель")
	}
}
