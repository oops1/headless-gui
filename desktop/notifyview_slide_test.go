package desktop

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Плавное раскрытие карточек, групп и быстрых действий; перетаскивание
// бегунка полосы прокрутки; полоса важности.

// slideFixture — открытый центр Windows 10: в группе «a» (она новее и выше) три
// карточки с длинным текстом (раскрываются), в группе «b» две; шесть плиток быстрых действий.
func slideFixture(t *testing.T) (*NotificationCenter, *FakeNotifications, []NotificationID) {
	t.Helper()
	t.Cleanup(widget.StopAllAnimations)
	long := strings.Repeat("длинное слово ", 30)
	ns := NewFakeNotifications()
	var ids []NotificationID
	for i := 0; i < 3; i++ {
		ids = append(ids, ns.Add(Notification{AppID: "a", AppName: "Приложение a",
			Title: "Длинное " + string(rune('A'+i)), Body: long,
			Time: sampleTime().Add(time.Hour + time.Duration(i)*time.Minute)}))
	}
	for i := 0; i < 2; i++ {
		ids = append(ids, ns.Add(Notification{AppID: "b", AppName: "Приложение b",
			Title: "Другое " + string(rune('A'+i)), Body: long,
			Time: sampleTime().Add(time.Duration(i) * time.Minute)}))
	}
	q := NewQuickActionList(
		QuickAction{ID: "wifi", Title: "Wi-Fi", On: true},
		QuickAction{ID: "bt", Title: "Bluetooth"},
		QuickAction{ID: "night", Title: "Ночной свет"},
		QuickAction{ID: "plane", Title: "В самолёте"},
		QuickAction{ID: "vpn", Title: "VPN"},
		QuickAction{ID: "loc", Title: "Расположение"},
	)
	nc := NewNotificationCenter(richNotifTheme(t), ns)
	nc.Clock = NewFakeClock(sampleTime().Add(2 * time.Hour))
	nc.SetQuickActions(q)
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	nc.Settle()
	finishAnimations()
	t.Cleanup(nc.Close)
	return nc, ns, ids
}

// cardOf находит карточку в раскладке.
func (l *richLayout) cardOf(id NotificationID) (richCard, bool) {
	for _, g := range l.groups {
		for _, c := range g.cards {
			if c.n.ID == id {
				return c, true
			}
		}
	}
	return richCard{}, false
}

func (l *richLayout) groupOf(app AppID) (richGroup, bool) {
	for _, g := range l.groups {
		if g.app == app {
			return g, true
		}
	}
	return richGroup{}, false
}

// stepAnim шагает часы анимаций: старт на t0, затем на d вперёд.
func stepAnim(d time.Duration) {
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(d))
}

// Раскрытие текста карточки идёт плавно: посреди движения высота между
// свёрнутой и раскрытой, карточки ниже съезжают вместе, а к концу всё ровно
// как у статичной раскладки.
func TestSlide_CardExpandsGradually(t *testing.T) {
	nc, _, ids := slideFixture(t)
	id, below := ids[2], ids[1] // новые сверху: ids[2] — самая верхняя карточка группы
	l0 := nc.testLayout()
	c0, _ := l0.cardOf(id)
	b0, _ := l0.cardOf(below)
	closedH := c0.rect.Dy()

	inv := watchInvalidations(t)
	nc.clickZone(t, zoneKey{kind: zoneCardToggle, note: id})
	inv.reset()
	// Сразу после нажатия движение только началось: высота прежняя.
	if c, _ := nc.testLayout().cardOf(id); c.rect.Dy() != closedH {
		t.Fatalf("высота скакнула в момент нажатия: %d, было %d", c.rect.Dy(), closedH)
	}
	stepAnim(60 * time.Millisecond)

	l1 := nc.testLayout()
	c1, _ := l1.cardOf(id)
	b1, _ := l1.cardOf(below)
	if !c1.anim {
		t.Fatal("карточка не помечена как движущаяся")
	}
	finishAnimations()
	l2 := nc.testLayout()
	c2, _ := l2.cardOf(id)
	b2, _ := l2.cardOf(below)
	openH := c2.rect.Dy()
	if openH <= closedH {
		t.Fatalf("раскрытая карточка не выше: %d и %d", openH, closedH)
	}
	if h := c1.rect.Dy(); h <= closedH || h >= openH {
		t.Errorf("посреди движения высота %d вне (%d, %d)", h, closedH, openH)
	}
	if !(b1.rect.Min.Y > b0.rect.Min.Y && b1.rect.Min.Y < b2.rect.Min.Y) {
		t.Errorf("нижняя карточка не едет: %d -> %d -> %d", b0.rect.Min.Y, b1.rect.Min.Y, b2.rect.Min.Y)
	}
	if c2.anim {
		t.Error("после конца движения карточка всё ещё помечена движущейся")
	}
	if len(c2.bodyLines) <= 2 {
		t.Errorf("раскрытый текст в %d строках", len(c2.bodyLines))
	}

	// Перерисовывалось только затронутое: от верха карточки до низа списка. Ни
	// заголовок, ни подвал с плитками не тронуты.
	for _, r := range inv.get() {
		if r.Min.Y < c0.rect.Min.Y || r.Max.Y > l0.viewport.Max.Y {
			t.Errorf("шаг движения заявил %v вне [верх карточки %d, низ списка %d]", r, c0.rect.Min.Y, l0.viewport.Max.Y)
		}
	}
	if len(inv.get()) == 0 {
		t.Error("шаги движения не перерисовывали ничего")
	}
	if !inv.covers(image.Pt(b1.rect.Min.X+10, b1.rect.Min.Y+5)) {
		t.Error("карточка ниже, которая съехала, не перерисована")
	}
	if widget.StepAnimations(time.Now().Add(2 * time.Hour)) {
		t.Error("после конца движения остались активные анимации: простой не должен будить кадры")
	}
}

// Сворачивание — обратное движение; оборванное раскрытие не прыгает.
func TestSlide_CardCollapsesAndReverses(t *testing.T) {
	nc, _, ids := slideFixture(t)
	id := ids[2]
	closedH := func() int { c, _ := nc.testLayout().cardOf(id); return c.rect.Dy() }
	h0 := closedH()
	nc.clickZone(t, zoneKey{kind: zoneCardToggle, note: id})
	finishAnimations()
	openH := closedH()

	nc.clickZone(t, zoneKey{kind: zoneCardToggle, note: id})
	stepAnim(50 * time.Millisecond)
	mid := closedH()
	if mid >= openH || mid <= h0 {
		t.Fatalf("посреди сворачивания высота %d вне (%d, %d)", mid, h0, openH)
	}
	// Передумал на середине: движение идёт назад от того, что на экране.
	nc.clickZone(t, zoneKey{kind: zoneCardToggle, note: id})
	if got := closedH(); got != mid {
		t.Errorf("разворот прыгнул: было %d, стало %d", mid, got)
	}
	finishAnimations()
	if got := closedH(); got != openH {
		t.Errorf("после разворота высота %d, ждали раскрытую %d", got, openH)
	}
}

// Группа сворачивается плавно: нижняя группа едет вверх, нажимается только
// видимая часть карточек.
func TestSlide_GroupCollapsesGradually(t *testing.T) {
	nc, _, ids := slideFixture(t)
	l0 := nc.testLayout()
	a0, _ := l0.groupOf("a")
	b0, _ := l0.groupOf("b")
	if b0.rect.Min.Y < a0.rect.Min.Y {
		a0, b0 = b0, a0
	}
	top, bot := a0.app, b0.app
	// Сворачиваем верхнюю группу; по ней же смотрим, как едет нижняя.
	cards := len(a0.cards)
	if cards < 2 {
		t.Fatalf("в группе %q карточек %d", top, cards)
	}
	last := a0.cards[cards-1].n.ID

	inv := watchInvalidations(t)
	nc.clickZone(t, zoneKey{kind: zoneGroup, app: top})
	inv.reset()
	stepAnim(100 * time.Millisecond) // почти свёрнута: низ карточек уже под заголовком
	l1 := nc.testLayout()
	g1, _ := l1.groupOf(top)
	b1, _ := l1.groupOf(bot)
	if !g1.anim || len(g1.cards) != cards {
		t.Fatalf("группа посреди сворачивания: anim=%v, карточек %d из %d", g1.anim, len(g1.cards), cards)
	}
	if _, ok := l1.find(zoneKey{kind: zoneCard, note: last}); ok {
		t.Error("нижняя карточка свёртывающейся группы ещё нажимается, хотя заехала под заголовок")
	}
	finishAnimations()
	l2 := nc.testLayout()
	b2, _ := l2.groupOf(bot)
	g2, _ := l2.groupOf(top)
	if !(b1.rect.Min.Y < b0.rect.Min.Y && b1.rect.Min.Y > b2.rect.Min.Y) {
		t.Errorf("нижняя группа едет не монотонно: %d -> %d -> %d", b0.rect.Min.Y, b1.rect.Min.Y, b2.rect.Min.Y)
	}
	if !g2.collapsed || len(g2.cards) != 0 || g2.anim {
		t.Errorf("после движения: collapsed=%v, карточек %d, anim=%v", g2.collapsed, len(g2.cards), g2.anim)
	}
	for _, r := range inv.get() {
		if r.Min.Y < a0.rect.Min.Y || r.Max.Y > l0.viewport.Max.Y {
			t.Errorf("шаг заявил %v: вне от верха группы до низа списка", r)
		}
	}
	_ = ids

	// Обратно: группа раскрывается плавно.
	nc.SetGroupCollapsed(top, false)
	stepAnim(10 * time.Millisecond)
	if g, _ := nc.testLayout().groupOf(top); !g.anim || len(g.cards) != cards {
		t.Errorf("раскрытие через SetGroupCollapsed не плавное: anim=%v, карточек %d", g.anim, len(g.cards))
	}
	finishAnimations()
	if g, _ := nc.testLayout().groupOf(top); g.anim || len(g.cards) != cards {
		t.Errorf("раскрытая группа: anim=%v, карточек %d", g.anim, len(g.cards))
	}
}

// Сетка быстрых действий растёт и сжимается плавно; перерисовывается список и
// подвал, шапка не трогается.
func TestSlide_QuickGridExpandsGradually(t *testing.T) {
	nc, _, _ := slideFixture(t)
	l0 := nc.testLayout()
	oneRow := l0.quick.Dy()
	inv := watchInvalidations(t)

	nc.clickZone(t, zoneKey{kind: zoneExpand})
	inv.reset()
	stepAnim(60 * time.Millisecond)
	l1 := nc.testLayout()
	if len(l1.tiles) != 6 {
		t.Errorf("посреди раскрытия плиток %d, ждали все 6 (обрезаются рамкой)", len(l1.tiles))
	}
	finishAnimations()
	l2 := nc.testLayout()
	fullH := l2.quick.Dy()
	if fullH <= oneRow {
		t.Fatalf("раскрытая сетка не выше: %d и %d", fullH, oneRow)
	}
	if h := l1.quick.Dy(); h <= oneRow || h >= fullH {
		t.Errorf("посреди раскрытия высота сетки %d вне (%d, %d)", h, oneRow, fullH)
	}
	if !(l1.viewport.Max.Y < l0.viewport.Max.Y && l1.viewport.Max.Y > l2.viewport.Max.Y) {
		t.Errorf("список не сжимается плавно: низ %d -> %d -> %d", l0.viewport.Max.Y, l1.viewport.Max.Y, l2.viewport.Max.Y)
	}
	for _, r := range inv.get() {
		if r.Min.Y < l0.viewport.Min.Y {
			t.Errorf("шаг заявил %v выше списка: шапка не должна перерисовываться", r)
		}
	}
	if len(inv.get()) == 0 {
		t.Error("шаги движения ничего не перерисовывали")
	}

	nc.clickZone(t, zoneKey{kind: zoneExpand})
	stepAnim(60 * time.Millisecond)
	if h := nc.testLayout().quick.Dy(); h >= fullH || h <= oneRow {
		t.Errorf("посреди сворачивания высота сетки %d вне (%d, %d)", h, oneRow, fullH)
	}
	finishAnimations()
	if h := nc.testLayout().quick.Dy(); h != oneRow {
		t.Errorf("свёрнутая сетка %d, ждали %d", h, oneRow)
	}
}

// Нулевая длительность в теме — мгновенно, без единого кадра анимации.
func TestSlide_ZeroDurationIsInstant(t *testing.T) {
	defer widget.StopAllAnimations()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	still := theme.NewProfile("win10-still")
	still.Parent = theme.ProfileWindows10
	still.Anims[AnimNotificationExpand] = theme.AnimSpec{}
	if err := m.RegisterTheme(still); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("win10-still"); err != nil {
		t.Fatal(err)
	}
	if d, _ := expandAnimation(m); d != 0 {
		t.Fatalf("длительность %v, ждали 0", d)
	}

	_, ns, ids := slideFixture(t)
	nc := NewNotificationCenter(m, ns)
	nc.Clock = NewFakeClock(sampleTime().Add(2 * time.Hour))
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	nc.Settle()
	defer nc.Close()

	c0, _ := nc.testLayout().cardOf(ids[2])
	nc.clickZone(t, zoneKey{kind: zoneCardToggle, note: ids[2]})
	c1, _ := nc.testLayout().cardOf(ids[2])
	if c1.rect.Dy() <= c0.rect.Dy() || c1.anim {
		t.Errorf("при нулевой длительности карточка не раскрылась сразу: %d -> %d, anim=%v", c0.rect.Dy(), c1.rect.Dy(), c1.anim)
	}
	if widget.StepAnimations(time.Now()) {
		t.Error("при нулевой длительности запущена анимация")
	}
}

// Токен не объявлен профилем — берётся menu.open; у Windows 10 объявлен свой.
func TestSlide_AnimationToken(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows10, theme.ProfileWindows10Dark} {
		d, _ := expandAnimation(managerFor(t, name))
		if d <= 0 {
			t.Errorf("%s: нет анимации раскрытия", name)
		}
	}
	m := managerFor(t, theme.ProfileWindows11)
	want, _ := animation(m, AnimMenuOpen)
	if got, _ := expandAnimation(m); got != want {
		t.Errorf("без токена длительность %v, ждали menu.open = %v", got, want)
	}
	if d, _ := expandAnimation(managerFor(t, theme.ProfileWindows2000)); d != 0 {
		t.Errorf("Windows 2000 анимирует раскрытие: %v", d)
	}
}

// Закрытие панели прерывает движения: после повторного открытия всё статично.
func TestSlide_ReopenDoesNotReplay(t *testing.T) {
	nc, _, ids := slideFixture(t)
	nc.clickZone(t, zoneKey{kind: zoneCardToggle, note: ids[2]})
	stepAnim(30 * time.Millisecond)
	nc.Close()
	nc.Settle()
	finishAnimations()
	nc.Open(panelAnchor())
	nc.Settle()
	for _, g := range nc.testLayout().groups {
		for _, c := range g.cards {
			if c.anim {
				t.Errorf("карточка %v движется сразу после открытия", c.n.ID)
			}
		}
	}
	if widget.StepAnimations(time.Now()) {
		t.Error("после закрытия осталась активная анимация")
	}
}

// ─── Перетаскивание бегунка ──────────────────────────────────────────────────

type fakeCapture struct {
	set      widget.Widget
	released int
}

func (f *fakeCapture) SetCapture(w widget.Widget) { f.set = w }
func (f *fakeCapture) ReleaseCapture()            { f.set = nil; f.released++ }

func (nc *NotificationCenter) press(pt image.Point) {
	nc.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
}

func (nc *NotificationCenter) release(pt image.Point) {
	nc.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft})
}

// scrollFixture — центр с длинным списком (прокрутка есть).
func scrollFixture(t *testing.T) (*NotificationCenter, *FakeNotifications, *fakeCapture) {
	t.Helper()
	nc, ns, _ := richFixture(t, 14)
	cm := &fakeCapture{}
	nc.SetCaptureManager(cm)
	if nc.testLayout().maxScrl <= 0 {
		t.Fatal("список не прокручивается")
	}
	return nc, ns, cm
}

func thumbPoint(l *richLayout) image.Point {
	th := l.thumb()
	return image.Pt(th.Min.X+th.Dx()/2, th.Min.Y+th.Dy()/2)
}

// Бегунок хватается мышью, тянется пропорционально, не гаснет, пока его
// держат, и отпускается там, где бы ни оказался курсор.
func TestScrollbar_DragsThumb(t *testing.T) {
	nc, ns, cm := scrollFixture(t)
	inv := watchInvalidations(t)
	before := len(ns.List())
	l := nc.testLayout()
	th := l.thumb()
	if th.Empty() || th.Min.Y != l.viewport.Min.Y {
		t.Fatalf("бегунок %v у верха списка %v", th, l.viewport)
	}
	start := thumbPoint(l)
	nc.OnMouseMove(start.X, start.Y) // курсор над бегунком
	nc.press(start)
	if !nc.view.dragging() {
		t.Fatal("нажатие на бегунок его не схватило")
	}
	if cm.set != widget.Widget(nc) {
		t.Error("мышь не захвачена центром: перетаскивание оборвётся на краю панели")
	}
	if nc.view.scroll != 0 {
		t.Errorf("захват сдвинул список: %d", nc.view.scroll)
	}

	// Пропорция: на dy точек по дорожке список едет на dy*maxScrl/free.
	free := l.viewport.Dy() - th.Dy()
	dy := free / 2
	inv.reset()
	nc.OnMouseMove(start.X, start.Y+dy)
	want := dy * l.maxScrl / free
	if got := nc.view.scroll; got < want-1 || got > want+1 {
		t.Errorf("после смещения на %d список на %d, ждали %d (maxScrl %d, free %d)", dy, got, want, l.maxScrl, free)
	}
	for _, r := range inv.get() {
		if !r.In(l.viewport) {
			t.Errorf("перетаскивание перерисовало %v вне списка %v", r, l.viewport)
		}
	}
	if len(inv.get()) == 0 {
		t.Error("перетаскивание ничего не перерисовало")
	}
	// Бегунок едет за курсором и не прыгает под него.
	if got := nc.testLayout().thumb(); got.Min.Y < th.Min.Y+dy-2 || got.Min.Y > th.Min.Y+dy+2 {
		t.Errorf("бегунок на %d, ждали около %d", got.Min.Y, th.Min.Y+dy)
	}

	// Курсор ушёл далеко за панель: список упирается в край и не вылетает.
	nc.OnMouseMove(start.X-2000, l.viewport.Max.Y+500)
	if nc.view.scroll != l.maxScrl {
		t.Errorf("курсор ниже списка: прокрутка %d, ждали край %d", nc.view.scroll, l.maxScrl)
	}
	nc.OnMouseMove(start.X, -300)
	if nc.view.scroll != 0 {
		t.Errorf("курсор выше списка: прокрутка %d", nc.view.scroll)
	}

	// Пока тянут, бегунок не гаснет, даже когда время показа вышло.
	nc.view.mu.Lock()
	nc.view.thumbUntil = time.Time{}
	nc.view.mu.Unlock()
	if !nc.view.uiSnapshot(false).thumb {
		t.Error("бегунок погас посреди перетаскивания")
	}
	nc.view.thumbExpired()
	if !nc.view.uiSnapshot(false).thumb {
		t.Error("таймер погасил бегунок посреди перетаскивания")
	}
	if !nc.view.uiSnapshot(false).drag {
		t.Error("рисование не знает, что бегунок нажат")
	}

	// Отпускание за пределами панели отпускает бегунок.
	nc.release(image.Pt(5, 5))
	if nc.view.dragging() {
		t.Error("бегунок остался схваченным после отпускания мыши")
	}
	if len(ns.List()) != before {
		t.Error("перетаскивание бегунка нажало карточку")
	}
	nc.OnMouseMove(start.X, 200)
	if nc.view.scroll != 0 {
		t.Errorf("после отпускания курсор всё ещё ведёт список: %d", nc.view.scroll)
	}
}

// Щелчок по дорожке выше или ниже бегунка сдвигает список на страницу.
func TestScrollbar_TrackClickPages(t *testing.T) {
	nc, _, _ := scrollFixture(t)
	l := nc.testLayout()
	page := l.viewport.Dy() * 9 / 10
	x := l.thumb().Min.X

	below := image.Pt(x, l.viewport.Max.Y-3)
	nc.press(below)
	nc.release(below)
	if nc.view.dragging() {
		t.Error("щелчок по дорожке схватил бегунок")
	}
	want := page
	if want > l.maxScrl {
		want = l.maxScrl
	}
	if got := nc.view.scroll; got != want {
		t.Fatalf("щелчок ниже бегунка: прокрутка %d, ждали страницу %d", got, want)
	}
	l = nc.testLayout()
	above := image.Pt(x, l.viewport.Min.Y+1)
	if l.thumb().Min.Y <= above.Y {
		t.Fatal("бегунок у самого верха: щелчок выше него невозможен")
	}
	nc.press(above)
	nc.release(above)
	if got := nc.view.scroll; got != 0 && got != want-page {
		t.Errorf("щелчок выше бегунка: прокрутка %d, ждали %d", got, want-page)
	}
}

// Под курсором бегунок ярче, нажатый — как нажатая кнопка.
func TestScrollbar_ThumbStates(t *testing.T) {
	nc, _, _ := scrollFixture(t)
	l := nc.testLayout()
	p := thumbPoint(l)
	nc.OnMouseMove(l.viewport.Min.X+20, l.viewport.Min.Y+40)
	if nc.view.uiSnapshot(false).thumbHot {
		t.Error("бегунок подсвечен, когда курсор далеко от него")
	}
	nc.OnMouseMove(p.X, p.Y)
	if !nc.view.uiSnapshot(false).thumbHot {
		t.Error("бегунок не подсвечен под курсором")
	}
	nc.press(p)
	if u := nc.view.uiSnapshot(false); !u.drag {
		t.Error("нажатый бегунок не помечен")
	}
	nc.release(p)
	if nc.view.uiSnapshot(false).drag {
		t.Error("бегунок остался нажатым")
	}
}

// ─── Важность ────────────────────────────────────────────────────────────────

// Предупреждение и ошибка метятся полосой цвета у края карточки, обычное
// уведомление — нет. Цвет берётся из темы.
func TestSeverity_StripeOnWindows10(t *testing.T) {
	f := newRenderFixture(t, 1)
	f.ns.Add(Notification{AppID: "s", AppName: "Система", Title: "Инфо", Body: "x", Time: sampleTime().Add(3 * time.Hour)})
	warn := f.ns.Add(Notification{AppID: "s", AppName: "Система", Title: "Внимание", Body: "x", Time: sampleTime().Add(4 * time.Hour), Severity: SeverityWarning})
	fail := f.ns.Add(Notification{AppID: "s", AppName: "Система", Title: "Сбой", Body: "x", Time: sampleTime().Add(5 * time.Hour), Severity: SeverityError})
	l := f.nc.testLayout()
	img := f.frame()
	edge := func(id NotificationID) color.RGBA {
		c, ok := l.cardOf(id)
		if !ok {
			t.Fatalf("карточки %v нет в раскладке", id)
		}
		return px(img, image.Pt(c.rect.Min.X+1, c.rect.Min.Y+c.rect.Dy()/2))
	}
	wantW := f.tm.GetStyle(ComponentNotificationCenter, ncPartSevWarn, theme.StateNormal).Fill
	wantE := f.tm.GetStyle(ComponentNotificationCenter, ncPartSevError, theme.StateNormal).Fill
	if wantW.A == 0 || wantE.A == 0 || wantW == wantE {
		t.Fatalf("тема не задала цвета важности: %v и %v", wantW, wantE)
	}
	if got := edge(warn); !nearColor(got, wantW, 4) {
		t.Errorf("полоса предупреждения %v, ждали %v", got, wantW)
	}
	if got := edge(fail); !nearColor(got, wantE, 4) {
		t.Errorf("полоса ошибки %v, ждали %v", got, wantE)
	}
	var info NotificationID
	for _, n := range f.ns.List() {
		if n.Title == "Инфо" {
			info = n.ID
		}
	}
	if got := edge(info); nearColor(got, wantW, 30) || nearColor(got, wantE, 30) {
		t.Errorf("у обычного уведомления полоса важности: %v", got)
	}
}
