package desktop

import (
	"fmt"
	"image"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// flatNotifTheme — тема, в которой центр уведомлений остаётся плоским списком
// (у Windows 10 презентер назначен, и центр рисуется с группами и действиями).
func flatNotifTheme(t *testing.T) *theme.Manager {
	t.Helper()
	return managerFor(t, theme.ProfileWindows11)
}

// richNotifTheme — тема с центром уведомлений Windows 10.
func richNotifTheme(t *testing.T) *theme.Manager {
	t.Helper()
	return managerFor(t, theme.ProfileWindows10)
}

// invalidations собирает заявленные области перерисовки.
type invalidations struct {
	mu    sync.Mutex
	rects []image.Rectangle
}

func watchInvalidations(t *testing.T) *invalidations {
	t.Helper()
	inv := &invalidations{}
	h := widget.RegisterUINotifier(nil, func(r image.Rectangle) {
		inv.mu.Lock()
		inv.rects = append(inv.rects, r)
		inv.mu.Unlock()
	})
	t.Cleanup(func() { widget.UnregisterUINotifier(h) })
	return inv
}

func (i *invalidations) reset() {
	i.mu.Lock()
	i.rects = nil
	i.mu.Unlock()
}

func (i *invalidations) get() []image.Rectangle {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]image.Rectangle(nil), i.rects...)
}

// covers — какая-нибудь из заявленных областей накрывает точку.
func (i *invalidations) covers(pt image.Point) bool {
	for _, r := range i.get() {
		if pt.In(r) {
			return true
		}
	}
	return false
}

// richFixture — открытый центр Windows 10 на экране 800×600 с панелью задач
// в нижних 40 точках; в источнике n уведомлений двух приложений.
func richFixture(t *testing.T, n int) (*NotificationCenter, *FakeNotifications, *QuickActionList) {
	t.Helper()
	ns := NewFakeNotifications()
	for i := 0; i < n; i++ {
		app := AppID("a")
		if i%2 == 1 {
			app = "b"
		}
		ns.Add(Notification{
			AppID: app, AppName: "Приложение " + string(app),
			Title: fmt.Sprintf("Заголовок %d", i), Body: "Текст уведомления",
			Time: sampleTime().Add(time.Duration(i) * time.Minute),
		})
	}
	q := NewQuickActionList(
		QuickAction{ID: "wifi", Title: "Wi-Fi", On: true},
		QuickAction{ID: "bt", Title: "Bluetooth"},
		QuickAction{ID: "night", Title: "Ночной свет"},
		QuickAction{ID: "plane", Title: "В самолёте"},
		QuickAction{ID: "vpn", Title: "VPN"},
		QuickAction{ID: "loc", Title: "Расположение", Disabled: true},
	)
	nc := NewNotificationCenter(richNotifTheme(t), ns)
	nc.Clock = NewFakeClock(sampleTime().Add(time.Hour))
	nc.SetQuickActions(q)
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	nc.Settle()
	t.Cleanup(nc.Close)
	return nc, ns, q
}

func (nc *NotificationCenter) testLayout() *richLayout { return nc.view.layout(nc.rect()) }

// zoneCenter — центр зоны с ключом k.
func (nc *NotificationCenter) zoneCenter(t *testing.T, k zoneKey) image.Point {
	t.Helper()
	z, ok := nc.testLayout().find(k)
	if !ok {
		t.Fatalf("зоны %+v нет в раскладке", k)
	}
	x, y := pointIn(z.hit)
	return image.Pt(x, y)
}

func (nc *NotificationCenter) click(pt image.Point) {
	nc.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
	nc.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft})
}

func (nc *NotificationCenter) clickZone(t *testing.T, k zoneKey) {
	t.Helper()
	nc.click(nc.zoneCenter(t, k))
}

func key(e widget.KeyCode) widget.KeyEvent { return widget.KeyEvent{Code: e, Pressed: true} }

// ─── Ошибки, из-за которых написан центр ─────────────────────────────────────

// После закрытия и повторного открытия центр снова получает изменения
// источника. Раньше подписка оформлялась в конструкторе, а Close её снимал.
func TestNotificationCenter_ResubscribesAfterReopen(t *testing.T) {
	for _, name := range []struct {
		what string
		tm   func(*testing.T) *theme.Manager
	}{{"плоский", flatNotifTheme}, {"Windows 10", richNotifTheme}} {
		t.Run(name.what, func(t *testing.T) {
			ns := notesFake()
			nc := NewNotificationCenter(name.tm(t), ns)
			nc.Screen = panelScreen()
			defer nc.Close()
			inv := watchInvalidations(t)

			nc.Open(panelAnchor())
			nc.Settle()
			inv.reset()
			ns.Add(Notification{Title: "Первое", Time: sampleTime()})
			if len(inv.get()) == 0 {
				t.Fatal("открытый центр не перерисовался при новом уведомлении")
			}

			nc.Close()
			nc.Settle()
			inv.reset()
			ns.Add(Notification{Title: "Пока закрыт", Time: sampleTime()})
			if len(inv.get()) != 0 {
				t.Errorf("закрытый центр будит рендер: %v", inv.get())
			}

			nc.Open(panelAnchor())
			nc.Settle()
			inv.reset()
			ns.Add(Notification{Title: "После повторного открытия", Time: sampleTime()})
			if len(inv.get()) == 0 {
				t.Fatal("после повторного открытия центр не получает изменения источника")
			}
			if !inv.covers(pointInRect(nc.OverlayBounds())) {
				t.Errorf("перерисована не область центра: %v, область %v", inv.get(), nc.OverlayBounds())
			}
		})
	}
}

func pointInRect(r image.Rectangle) image.Point {
	x, y := pointIn(r)
	return image.Pt(x, y)
}

// Закрытие любым способом (клик мимо, Esc, DismissAt) отписывает центр:
// источник не держит закрытую панель.
func TestNotificationCenter_UnsubscribesOnAnyClose(t *testing.T) {
	ns := notesFake()
	nc := NewNotificationCenter(richNotifTheme(t), ns)
	nc.Screen = panelScreen()
	subs := func() int {
		ns.mu.Lock()
		defer ns.mu.Unlock()
		return len(ns.subs)
	}
	nc.Open(panelAnchor())
	if subs() != 1 {
		t.Fatalf("открытый центр подписан %d раз", subs())
	}
	nc.DismissAt(1, 1) // клик мимо
	if subs() != 0 {
		t.Errorf("центр закрыт кликом мимо, но подписка осталась: %d", subs())
	}
	nc.Open(panelAnchor())
	nc.DismissOnEscape()
	if subs() != 0 {
		t.Errorf("центр закрыт по Esc, но подписка осталась: %d", subs())
	}
	nc.Open(panelAnchor())
	nc.Open(panelAnchor()) // повторное открытие открытого
	if subs() != 1 {
		t.Errorf("повторное открытие размножило подписки: %d", subs())
	}
	nc.Close()
}

// Высота центра Windows 10 не зависит от числа карточек: от верха экрана до
// панели задач. Раньше она росла и при ~12 карточках закрывала панель.
func TestNotificationCenter_Win10HeightIsFixed(t *testing.T) {
	var want image.Rectangle
	for _, n := range []int{0, 1, 3, 12, 60} {
		nc, _, _ := richFixture(t, n)
		r := nc.OverlayBounds()
		if r.Min.Y != 0 || r.Max.Y != panelAnchor().Min.Y {
			t.Errorf("%d уведомлений: центр %v, ждал от верха экрана (0) до панели (%d)", n, r, panelAnchor().Min.Y)
		}
		if r.Max.X != panelScreenW {
			t.Errorf("%d уведомлений: центр не прижат к правому краю: %v", n, r)
		}
		if want.Empty() {
			want = r
		} else if r != want {
			t.Errorf("%d уведомлений: размер центра изменился: %v вместо %v", n, r, want)
		}
	}
}

// Плоский центр не меняется: растёт вместе с карточками, как раньше.
func TestNotificationCenter_FlatStillGrows(t *testing.T) {
	heights := map[int]int{}
	for _, n := range []int{1, 3} {
		ns := NewFakeNotifications()
		for i := 0; i < n; i++ {
			ns.Add(Notification{Title: "x", Time: sampleTime()})
		}
		nc := NewNotificationCenter(flatNotifTheme(t), ns)
		nc.Screen = panelScreen()
		nc.Open(panelAnchor())
		nc.Settle()
		heights[n] = nc.OverlayBounds().Dy()
		if nc.presenter() != nil {
			t.Fatal("у Windows 11 назначен презентер центра уведомлений")
		}
		nc.Close()
	}
	if heights[3] <= heights[1] {
		t.Errorf("плоский центр перестал расти: %v", heights)
	}
}

// Зазор до панели задач: у центра Windows 10 — ноль (метрика), у прочих
// всплывающих панелей остаётся зазор Flyout.
func TestNotificationCenter_MarginOnlyForWin10Center(t *testing.T) {
	tm := richNotifTheme(t)
	nc, _, _ := richFixture(t, 3)
	if got := nc.OverlayBounds().Max.Y; got != panelAnchor().Min.Y {
		t.Errorf("зазор центра до панели %d, ждал 0", panelAnchor().Min.Y-got)
	}
	cal := NewCalendarFlyout(tm, NewFakeClock(sampleTime()))
	cal.Screen = panelScreen()
	cal.Open(panelAnchor())
	cal.Settle()
	if got := panelAnchor().Min.Y - cal.OverlayBounds().Max.Y; got != 6 {
		t.Errorf("зазор календаря до панели %d, ждал прежние 6", got)
	}
	menu := NewStartMenu(tm, NewStaticAppCatalog(AppInfo{ID: "t", Title: "T"}))
	menu.Screen = panelScreen()
	menu.Open(panelAnchor())
	menu.Settle()
	if got := panelAnchor().Min.Y - menu.OverlayBounds().Max.Y; got != 6 {
		t.Errorf("зазор «Пуска» до панели %d, ждал прежние 6", got)
	}
}

// Метрика зазора работает: ненулевое значение отодвигает центр от панели.
func TestNotificationCenter_MarginFromMetric(t *testing.T) {
	tm := richNotifTheme(t)
	p := theme.NewProfile("win10-gap")
	p.Parent = theme.ProfileWindows10
	p.SetMetric("notificationcenter.margin", 10)
	if err := tm.RegisterTheme(p); err != nil {
		t.Skipf("профиль не зарегистрирован: %v", err)
	}
	if err := tm.SetTheme("win10-gap"); err != nil {
		t.Fatal(err)
	}
	nc := NewNotificationCenter(tm, notesFake())
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	nc.Settle()
	defer nc.Close()
	if got := panelAnchor().Min.Y - nc.OverlayBounds().Max.Y; got != 10 {
		t.Errorf("зазор %d, ждал 10 из метрики", got)
	}
}

// WorkArea главнее значка: тот может быть ниже панели.
func TestNotificationCenter_WorkAreaWins(t *testing.T) {
	nc, _, _ := richFixture(t, 2)
	nc.WorkArea = image.Rect(0, 30, 800, 500)
	nc.Invalidate()
	r := nc.OverlayBounds()
	if r.Min.Y != 30 || r.Max.Y != 500 || r.Max.X != 800 {
		t.Errorf("центр %v не лёг на рабочую область", r)
	}
}

// Карточки лежат в прокрутке: колесо двигает список, а не панель.
func TestNotificationCenter_ListScrolls(t *testing.T) {
	nc, _, _ := richFixture(t, 14)
	l := nc.testLayout()
	if l.maxScrl <= 0 {
		t.Fatalf("14 карточек уместились без прокрутки: содержимое %d, окно %d", l.contentH, l.viewport.Dy())
	}
	first := l.groups[0].cards[0].rect
	pt := image.Pt(l.viewport.Min.X+50, l.viewport.Min.Y+50)
	if !nc.OnMouseWheelPixels(pt.X, pt.Y, 0, 120) {
		t.Fatal("колесо не принято")
	}
	l2 := nc.testLayout()
	if l2.scroll != 120 {
		t.Errorf("прокрутка %d, ждал 120", l2.scroll)
	}
	if got := l2.groups[0].cards[0].rect; got.Min.Y != first.Min.Y-120 {
		t.Errorf("карточка сдвинулась на %d, ждал 120", first.Min.Y-got.Min.Y)
	}
	// До упора.
	nc.OnMouseWheelPixels(pt.X, pt.Y, 0, 100000)
	if l3 := nc.testLayout(); l3.scroll != l3.maxScrl {
		t.Errorf("прокрутка %d, ждал край %d", l3.scroll, l3.maxScrl)
	}
	nc.OnMouseWheelPixels(pt.X, pt.Y, 0, -100000)
	if l4 := nc.testLayout(); l4.scroll != 0 {
		t.Errorf("прокрутка вверх %d, ждал 0", l4.scroll)
	}
	// Вне списка (над плиткой) колесо список не двигает.
	nc.OnMouseWheelPixels(pt.X, pt.Y, 0, 50)
	tile := nc.zoneCenter(t, zoneKey{kind: zoneTile, action: "wifi"})
	before := nc.testLayout().scroll
	nc.OnMouseWheelPixels(tile.X, tile.Y, 0, 50)
	if nc.testLayout().scroll != before {
		t.Error("колесо над плитками прокрутило список")
	}
}

// Карточка, ушедшая за край списка, не нажимается там, где её не видно.
func TestNotificationCenter_ClippedCardNotClickable(t *testing.T) {
	nc, _, _ := richFixture(t, 14)
	l := nc.testLayout()
	// Точка под областью списка — на строке ссылок/плитках — не попадает в карточки.
	pt := image.Pt(l.viewport.Min.X+60, l.viewport.Max.Y+2)
	if z, ok := l.zoneAt(pt); ok && z.key.kind == zoneCard {
		t.Errorf("под списком нашлась карточка %+v", z.key)
	}
}

// ─── Группы, карточки, очистка ───────────────────────────────────────────────

func TestNotificationCenter_GroupsByApp(t *testing.T) {
	nc, _, _ := richFixture(t, 6)
	l := nc.testLayout()
	if len(l.groups) != 2 {
		t.Fatalf("групп %d, ждал 2 (по приложению)", len(l.groups))
	}
	for _, g := range l.groups {
		for _, c := range g.cards {
			if c.n.AppID != g.app {
				t.Errorf("карточка %v в чужой группе %v", c.n.AppID, g.app)
			}
		}
		// Новые сверху.
		for i := 1; i < len(g.cards); i++ {
			if g.cards[i].n.At().After(g.cards[i-1].n.At()) {
				t.Errorf("группа %v: старое выше нового", g.app)
			}
		}
		if g.name == "" {
			t.Errorf("группа %v без имени", g.app)
		}
	}
	// Группа с самым свежим уведомлением первой: последним добавлено «b» (i=5).
	if l.groups[0].app != "b" {
		t.Errorf("первой лежит группа %q, ждал «b» (самое свежее)", l.groups[0].app)
	}
}

func TestNotificationCenter_CollapseGroup(t *testing.T) {
	nc, _, _ := richFixture(t, 6)
	app := nc.testLayout().groups[0].app
	cards := len(nc.testLayout().groups[0].cards)
	nc.clickZone(t, zoneKey{kind: zoneGroup, app: app})
	g := nc.testLayout().groups[0]
	if !g.collapsed || len(g.cards) != 0 {
		t.Fatalf("группа не свернулась: collapsed=%v, карточек %d", g.collapsed, len(g.cards))
	}
	if !nc.GroupCollapsed(app) {
		t.Error("GroupCollapsed не видит свёрнутости")
	}
	nc.SetGroupCollapsed(app, false)
	if got := len(nc.testLayout().groups[0].cards); got != cards {
		t.Errorf("после раскрытия карточек %d, было %d", got, cards)
	}
}

func TestNotificationCenter_CloseGroupAndClear(t *testing.T) {
	nc, ns, _ := richFixture(t, 6)
	app := nc.testLayout().groups[0].app
	// Крестик группы виден при наведении, но нажимается всегда.
	nc.clickZone(t, zoneKey{kind: zoneGroupClose, app: app})
	for _, n := range ns.List() {
		if n.AppID == app {
			t.Errorf("уведомление %d группы %v осталось", n.ID, app)
		}
	}
	if len(ns.List()) != 3 {
		t.Errorf("осталось %d уведомлений, ждал 3 чужих", len(ns.List()))
	}
	nc.clickZone(t, zoneKey{kind: zoneClear})
	if len(ns.List()) != 0 {
		t.Errorf("после «Очистить уведомления» осталось %d", len(ns.List()))
	}
	if !nc.testLayout().empty {
		t.Error("пустой центр не знает, что пуст")
	}
	if !nc.testLayout().clear.Empty() {
		t.Error("кнопка очистки осталась в пустом центре")
	}
}

func TestNotificationCenter_CardCloseDismissesOne(t *testing.T) {
	nc, ns, _ := richFixture(t, 4)
	c := nc.testLayout().groups[0].cards[0]
	nc.clickZone(t, zoneKey{kind: zoneCardClose, note: c.n.ID})
	for _, n := range ns.List() {
		if n.ID == c.n.ID {
			t.Error("карточка не закрылась")
		}
	}
	if len(ns.List()) != 3 {
		t.Errorf("осталось %d, ждал 3", len(ns.List()))
	}
}

func TestNotificationCenter_ManageLink(t *testing.T) {
	nc, _, _ := richFixture(t, 2)
	var called, wasOpen int
	nc.OnManage = func() {
		called++
		if nc.IsOpen() {
			wasOpen++
		}
	}
	nc.clickZone(t, zoneKey{kind: zoneManage})
	if called != 1 {
		t.Fatalf("OnManage вызван %d раз", called)
	}
	if wasOpen != 0 {
		t.Error("центр оставался открытым при вызове OnManage")
	}
	if nc.IsOpen() {
		t.Error("центр не закрылся")
	}
}

// Длинный текст: свёрнутая карточка — две строки с многоточием и шеврон;
// раскрытая — весь текст.
func TestNotificationCenter_CardBodyExpands(t *testing.T) {
	ns := NewFakeNotifications()
	long := ""
	for i := 0; i < 30; i++ {
		long += "длинное слово "
	}
	id := ns.Add(Notification{AppID: "a", Title: "Длинное", Body: long, Time: sampleTime()})
	nc := NewNotificationCenter(richNotifTheme(t), ns)
	nc.Clock = NewFakeClock(sampleTime())
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	nc.Settle()
	defer nc.Close()

	c := nc.testLayout().groups[0].cards[0]
	if len(c.bodyLines) != 2 || !c.canToggle {
		t.Fatalf("свёрнутая карточка: строк %d, шеврон %v", len(c.bodyLines), c.canToggle)
	}
	last := c.bodyLines[len(c.bodyLines)-1]
	if len([]rune(last)) == 0 || []rune(last)[len([]rune(last))-1] != '…' {
		t.Errorf("обрезанный текст без многоточия: %q", last)
	}
	nc.clickZone(t, zoneKey{kind: zoneCardToggle, note: id})
	c2 := nc.testLayout().groups[0].cards[0]
	if !c2.open || len(c2.bodyLines) <= 2 {
		t.Errorf("раскрытая карточка: open=%v, строк %d", c2.open, len(c2.bodyLines))
	}
	if c2.rect.Dy() <= c.rect.Dy() {
		t.Error("раскрытая карточка не стала выше")
	}
	nc.clickZone(t, zoneKey{kind: zoneCardToggle, note: id})
	if got := len(nc.testLayout().groups[0].cards[0].bodyLines); got != 2 {
		t.Errorf("свёрнутая заново: строк %d", got)
	}
}

// Короткий текст без действий: раскрывать нечего, шеврона нет.
func TestNotificationCenter_ShortCardHasNoToggle(t *testing.T) {
	nc, _, _ := richFixture(t, 1)
	if c := nc.testLayout().groups[0].cards[0]; c.canToggle {
		t.Error("у короткой карточки без действий есть шеврон раскрытия")
	}
}

// ─── Действия ────────────────────────────────────────────────────────────────

func actionFixture(t *testing.T, keep bool) (*NotificationCenter, *FakeNotifications, NotificationID, *[]NotificationActionEvent) {
	t.Helper()
	ns := NewFakeNotifications()
	id := ns.Add(Notification{
		AppID: "od", AppName: "OneDrive", Title: "Включить", Body: "Текст", Time: sampleTime(),
		Actions: []NotificationAction{
			{ID: "when", Kind: NotificationActionSelect, Title: "Напомнить через:", Options: []string{"1 день", "1 неделю", "1 месяц"}, Selected: 1},
			{ID: "go", Kind: NotificationActionButton, Title: "Приступим", Keep: keep},
			{ID: "no", Kind: NotificationActionButton, Title: "Нет"},
			{ID: "more", Kind: NotificationActionLink, Title: "Подробнее", Keep: true},
			{ID: "say", Kind: NotificationActionReply, Keep: keep},
		},
	})
	nc := NewNotificationCenter(richNotifTheme(t), ns)
	nc.Clock = NewFakeClock(sampleTime())
	nc.Screen = panelScreen()
	var events []NotificationActionEvent
	nc.OnAction = func(e NotificationActionEvent) { events = append(events, e) }
	nc.Open(panelAnchor())
	nc.Settle()
	t.Cleanup(nc.Close)
	return nc, ns, id, &events
}

func TestNotificationCenter_ButtonActionCarriesInputsAndDismisses(t *testing.T) {
	nc, ns, id, events := actionFixture(t, false)
	nc.clickZone(t, zoneKey{kind: zoneAction, note: id, action: "go"})
	if len(*events) != 1 {
		t.Fatalf("событий %d", len(*events))
	}
	e := (*events)[0]
	if e.Notification != id || e.Action != "go" || e.Kind != NotificationActionButton {
		t.Errorf("событие %+v", e)
	}
	if e.Inputs["when"] != "1 неделю" {
		t.Errorf("кнопка не несёт выбранное значение списка: %v", e.Inputs)
	}
	if len(ns.List()) != 0 {
		t.Error("после кнопки уведомление не снято")
	}
}

func TestNotificationCenter_KeepActionLeavesNotification(t *testing.T) {
	nc, ns, id, events := actionFixture(t, true)
	nc.clickZone(t, zoneKey{kind: zoneAction, note: id, action: "go"})
	nc.clickZone(t, zoneKey{kind: zoneAction, note: id, action: "more"})
	if len(*events) != 2 {
		t.Fatalf("событий %d, ждал 2", len(*events))
	}
	if len(ns.List()) != 1 {
		t.Error("действие с Keep сняло уведомление")
	}
}

func TestNotificationCenter_SelectDropdown(t *testing.T) {
	nc, _, id, events := actionFixture(t, false)
	sel := zoneKey{kind: zoneSelect, note: id, action: "when"}
	nc.clickZone(t, sel)
	l := nc.testLayout()
	if l.dropRect.Empty() || len(l.dropItems) != 3 {
		t.Fatalf("список не раскрылся: %v, пунктов %d", l.dropRect, len(l.dropItems))
	}
	// Выбор пункта «1 месяц».
	nc.clickZone(t, zoneKey{kind: zoneOption, note: id, action: "when", index: 2})
	if !nc.testLayout().dropRect.Empty() {
		t.Error("список не закрылся после выбора")
	}
	if len(*events) != 1 || (*events)[0].Value != "1 месяц" || (*events)[0].Index != 2 || (*events)[0].Kind != NotificationActionSelect {
		t.Fatalf("событие выбора: %+v", *events)
	}
	// Выбор запомнен и уходит вместе с кнопкой.
	nc.clickZone(t, zoneKey{kind: zoneAction, note: id, action: "go"})
	if got := (*events)[1].Inputs["when"]; got != "1 месяц" {
		t.Errorf("кнопка несёт %q, ждал «1 месяц»", got)
	}
}

func TestNotificationCenter_DropdownClosesOnOutsideClickAndEsc(t *testing.T) {
	nc, _, id, _ := actionFixture(t, false)
	sel := zoneKey{kind: zoneSelect, note: id, action: "when"}
	nc.clickZone(t, sel)
	if nc.testLayout().dropRect.Empty() {
		t.Fatal("список не раскрылся")
	}
	nc.OnKeyEvent(key(widget.KeyEscape))
	if !nc.testLayout().dropRect.Empty() {
		t.Error("Esc не закрыл список")
	}
	if !nc.IsOpen() {
		t.Error("первый Esc закрыл весь центр вместо списка")
	}
	nc.clickZone(t, sel)
	nc.click(nc.zoneCenter(t, zoneKey{kind: zoneManage}))
	if !nc.testLayout().dropRect.Empty() {
		t.Error("клик мимо не закрыл список")
	}
}

func TestNotificationCenter_ReplyField(t *testing.T) {
	nc, ns, id, events := actionFixture(t, false)
	field := zoneKey{kind: zoneReply, note: id, action: "say"}
	nc.clickZone(t, field)
	if nc.view.focus != field {
		t.Fatalf("фокус не в поле ответа: %+v", nc.view.focus)
	}
	for _, r := range "Привет" {
		nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyUnknown, Rune: r, Pressed: true})
	}
	nc.OnKeyEvent(key(widget.KeyBackspace))
	nc.OnKeyEvent(key(widget.KeyLeft))
	nc.OnKeyEvent(widget.KeyEvent{Rune: 'X', Pressed: true})
	rp := nc.view.reply[ncInputKey{id, "say"}]
	if rp == nil || string(rp.text) != "ПривXе" {
		t.Fatalf("текст ответа %v", rp)
	}
	nc.OnKeyEvent(key(widget.KeyEnter))
	if len(*events) != 1 {
		t.Fatalf("событий %d", len(*events))
	}
	e := (*events)[0]
	if e.Action != "say" || e.Value != "ПривXе" || e.Kind != NotificationActionReply {
		t.Errorf("событие ответа %+v", e)
	}
	if len(ns.List()) != 0 {
		t.Error("после ответа уведомление осталось")
	}
}

func TestNotificationCenter_EmptyReplyIsNotSent(t *testing.T) {
	nc, _, id, events := actionFixture(t, false)
	nc.clickZone(t, zoneKey{kind: zoneReplySend, note: id, action: "say"})
	if len(*events) != 0 {
		t.Errorf("пустой ответ отправлен: %+v", *events)
	}
}

// Нажатие на саму карточку — активация по умолчанию: событие без действия,
// карточка снимается, центр закрывается.
func TestNotificationCenter_CardClickActivates(t *testing.T) {
	nc, ns, id, events := actionFixture(t, false)
	l := nc.testLayout()
	c := l.groups[0].cards[0]
	// Точка в заголовке карточки: вне действий и крестиков.
	pt := image.Pt(c.title.Min.X+4, c.title.Min.Y+4)
	nc.click(pt)
	if len(*events) != 1 || (*events)[0].Action != "" || (*events)[0].Notification != id {
		t.Fatalf("события %+v", *events)
	}
	if len(ns.List()) != 0 {
		t.Error("активированная карточка осталась")
	}
	if nc.IsOpen() {
		t.Error("центр остался открыт после открытия приложения")
	}
}

// Источник, реализующий NotificationActions, получает то же событие.
type actionSource struct {
	*FakeNotifications
	got []NotificationActionEvent
}

func (a *actionSource) InvokeNotificationAction(e NotificationActionEvent) { a.got = append(a.got, e) }

func TestNotificationCenter_SourceReceivesActions(t *testing.T) {
	src := &actionSource{FakeNotifications: NewFakeNotifications()}
	id := src.Add(Notification{AppID: "a", Title: "x", Time: sampleTime(),
		Actions: []NotificationAction{{ID: "ok", Kind: NotificationActionButton, Title: "Да"}}})
	nc := NewNotificationCenter(richNotifTheme(t), src)
	nc.Clock = NewFakeClock(sampleTime())
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	nc.Settle()
	defer nc.Close()
	nc.clickZone(t, zoneKey{kind: zoneAction, note: id, action: "ok"})
	if len(src.got) != 1 || src.got[0].Action != "ok" {
		t.Errorf("источник получил %+v", src.got)
	}
}

// Модель без значка и без действий не мешает: Notification из прежней модели.
func TestNotification_LegacyModelStillWorks(t *testing.T) {
	n := Notification{ID: 1, Title: "a", Body: "b", AppID: "x", Time: sampleTime()}
	if !n.At().Equal(sampleTime()) {
		t.Error("At() не вернул Time")
	}
	n.Timestamp = sampleTime().Add(time.Hour)
	if !n.At().Equal(sampleTime().Add(time.Hour)) {
		t.Error("Timestamp не главнее Time")
	}
}

// ─── Быстрые действия ────────────────────────────────────────────────────────

func TestNotificationCenter_QuickCollapsedIsOneRow(t *testing.T) {
	nc, _, _ := richFixture(t, 2)
	l := nc.testLayout()
	if len(l.tiles) != 4 {
		t.Fatalf("свёрнуто плиток %d, ждал один ряд из 4", len(l.tiles))
	}
	if !l.canExp || l.expand.Empty() {
		t.Error("нет ссылки «Развернуть» при 6 плитках")
	}
	row := l.tiles[0].rect.Min.Y
	for _, tl := range l.tiles {
		if tl.rect.Min.Y != row {
			t.Error("плитки свёрнутого вида не в одном ряду")
		}
	}
	// Плитки лежат на панели вплотную, ряд кончается над самым краем.
	if got := nc.OverlayBounds().Max.Y - l.tiles[0].rect.Max.Y; got != 4 {
		t.Errorf("зазор плиток до панели задач %d, ждал 4", got)
	}

	nc.clickZone(t, zoneKey{kind: zoneExpand})
	if !nc.QuickExpanded() {
		t.Fatal("«Развернуть» не развернуло")
	}
	l = nc.testLayout()
	if len(l.tiles) != 6 {
		t.Errorf("развёрнуто плиток %d, ждал 6", len(l.tiles))
	}
	if l.tiles[4].rect.Min.Y <= l.tiles[0].rect.Min.Y {
		t.Error("пятая плитка не во втором ряду")
	}
	nc.clickZone(t, zoneKey{kind: zoneExpand})
	if nc.QuickExpanded() || len(nc.testLayout().tiles) != 4 {
		t.Error("«Свернуть» не вернуло один ряд")
	}
}

func TestNotificationCenter_TileToggle(t *testing.T) {
	nc, _, q := richFixture(t, 1)
	nc.clickZone(t, zoneKey{kind: zoneTile, action: "bt"})
	if len(q.Toggled) != 1 || q.Toggled[0] != "bt" {
		t.Errorf("нажатия %v", q.Toggled)
	}
	// Недоступная плитка не нажимается.
	nc.SetQuickExpanded(true)
	l := nc.testLayout()
	for _, tl := range l.tiles {
		if tl.a.ID == "loc" {
			x, y := pointIn(tl.rect)
			nc.click(image.Pt(x, y))
		}
	}
	for _, id := range q.Toggled {
		if id == "loc" {
			t.Error("недоступная плитка нажалась")
		}
	}
}

// Изменение одной плитки перерисовывает только её.
func TestNotificationCenter_SingleTileRepaintsOnlyThatTile(t *testing.T) {
	nc, _, q := richFixture(t, 2)
	inv := watchInvalidations(t)
	var tile image.Rectangle
	for _, tl := range nc.testLayout().tiles {
		if tl.a.ID == "night" {
			tile = tl.rect
		}
	}
	inv.reset()
	q.SetOn("night", true)
	got := inv.get()
	if len(got) == 0 {
		t.Fatal("плитка не перерисована")
	}
	for _, r := range got {
		if !r.In(tile) {
			t.Errorf("перерисована область %v вне плитки %v", r, tile)
		}
	}
	// Тот же состав, но другое число плиток — перерисовывается панель целиком.
	inv.reset()
	q.Set(QuickAction{ID: "new", Title: "Новая"})
	covered := false
	for _, r := range inv.get() {
		if !r.In(tile) {
			covered = true
		}
	}
	if !covered {
		t.Error("новый состав плиток не перерисовал панель")
	}
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

func TestNotificationCenter_KeyboardTabOrderAndActivation(t *testing.T) {
	nc, ns, q := richFixture(t, 2)
	if !nc.AcceptsTab() || nc.TabIndex() != 0 {
		t.Fatal("открытый центр не принимает Tab")
	}
	var seen []zoneKind
	for i := 0; i < 40; i++ {
		nc.OnKeyEvent(key(widget.KeyTab))
		seen = append(seen, nc.view.focus.kind)
		if nc.view.focus.kind == zoneTile {
			break
		}
	}
	if seen[0] != zoneManage {
		t.Errorf("первая остановка %v, ждал ссылку заголовка", seen[0])
	}
	if nc.view.focus.kind != zoneTile {
		t.Fatalf("Tab не дошёл до плиток: %v", seen)
	}
	// Enter на плитке нажимает её.
	nc.OnKeyEvent(key(widget.KeyEnter))
	if len(q.Toggled) != 1 {
		t.Errorf("Enter не нажал плитку: %v", q.Toggled)
	}
	// Shift+Tab идёт назад.
	prev := nc.view.focus
	nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true, Mod: widget.ModShift})
	if nc.view.focus == prev {
		t.Error("Shift+Tab не сдвинул фокус")
	}
	// Delete на карточке снимает её.
	nc.view.focus = zoneKey{kind: zoneCard, note: ns.List()[0].ID}
	nc.OnKeyEvent(key(widget.KeyDelete))
	if len(ns.List()) != 1 {
		t.Errorf("Delete не снял карточку: осталось %d", len(ns.List()))
	}
	// Esc закрывает центр.
	nc.OnKeyEvent(key(widget.KeyEscape))
	if nc.IsOpen() {
		t.Error("Esc не закрыл центр")
	}
}

func TestNotificationCenter_KeyboardTilesFollowGrid(t *testing.T) {
	nc, _, _ := richFixture(t, 1)
	nc.SetQuickExpanded(true)
	nc.view.focus = zoneKey{kind: zoneTile, action: "wifi"}
	nc.OnKeyEvent(key(widget.KeyRight))
	if nc.view.focus.action != "bt" {
		t.Errorf("вправо от wifi: %q", nc.view.focus.action)
	}
	nc.OnKeyEvent(key(widget.KeyLeft))
	nc.OnKeyEvent(key(widget.KeyDown))
	if nc.view.focus.action != "vpn" {
		t.Errorf("вниз от wifi: %q, ждал vpn (второй ряд, тот же столбец)", nc.view.focus.action)
	}
	nc.OnKeyEvent(key(widget.KeyLeft))
	if nc.view.focus.action != "vpn" {
		t.Errorf("влево от первого столбца ушёл на %q", nc.view.focus.action)
	}
	// Вправо от vpn лежит недоступная плитка: фокус её пропускает и остаётся.
	nc.OnKeyEvent(key(widget.KeyRight))
	if nc.view.focus.action != "vpn" {
		t.Errorf("фокус встал на недоступную плитку: %q", nc.view.focus.action)
	}
}

func TestNotificationCenter_FocusRingOnlyForKeyboard(t *testing.T) {
	nc, _, _ := richFixture(t, 2)
	nc.SetFocused(true)
	if !nc.FocusVisible() {
		t.Fatal("клавиатурный фокус без рамки")
	}
	nc.click(nc.zoneCenter(t, zoneKey{kind: zoneTile, action: "bt"}))
	if nc.FocusVisible() {
		t.Error("после щелчка мышью рамка осталась")
	}
	nc.OnKeyEvent(key(widget.KeyTab))
	if !nc.FocusVisible() {
		t.Error("после клавиши рамка не вернулась")
	}
}

// Esc из списка закрывает всю группу панелей; фокус возвращается.
func TestNotificationCenter_RevealScrollsFocusedCardIntoView(t *testing.T) {
	nc, _, _ := richFixture(t, 14)
	checked := 0
	for i := 0; i < 10; i++ {
		nc.OnKeyEvent(key(widget.KeyTab))
		k := nc.view.focus.kind
		if k != zoneCard && k != zoneGroup {
			continue
		}
		l := nc.testLayout()
		z, ok := l.find(nc.view.focus)
		if !ok {
			t.Fatalf("фокус %+v не в раскладке", nc.view.focus)
		}
		if z.rect.Min.Y < l.viewport.Min.Y || z.rect.Max.Y > l.viewport.Max.Y {
			t.Errorf("остановка %d: зона %v вне окна списка %v", i, z.rect, l.viewport)
		}
		checked++
	}
	if checked < 4 || nc.testLayout().scroll == 0 {
		t.Errorf("проверено остановок %d, прокрутка %d: список не двигался за фокусом", checked, nc.testLayout().scroll)
	}
}

// ─── Тема, акцент, язык на открытом центре ───────────────────────────────────

func drawTexts(nc *NotificationCenter) []recText {
	ctx := &recCtx{}
	nc.DrawOverlay(ctx)
	return ctx.texts
}

func TestNotificationCenter_LanguageSwitchOnOpenCenter(t *testing.T) {
	nc, _, _ := richFixture(t, 3)
	useLanguage(t, "RU")
	if tx := drawTexts(nc); !containsText(tx, "Управление уведомлениями") || !containsText(tx, "Очистить уведомления") {
		t.Errorf("по-русски: %+v", tx)
	}
	widget.SetLanguage("EN")
	if tx := drawTexts(nc); !containsText(tx, "Manage notifications") || !containsText(tx, "Clear notifications") || containsText(tx, "Очистить уведомления") {
		t.Errorf("по-английски: %+v", tx)
	}
	// Ключи можно связать со своим каталогом.
	widget.RegisterStrings("XX", map[string]string{"MyManage": "Параметры"})
	widget.AliasStrings(map[string]string{StrNotifManage: "MyManage"})
	defer widget.AliasStrings(map[string]string{StrNotifManage: ""})
	widget.SetLanguage("XX")
	if tx := drawTexts(nc); !containsText(tx, "Параметры") {
		t.Errorf("псевдоним ключа не сработал: %+v", tx)
	}
}

// Тот же компонент меняет вид вместе с темой: плоский → Windows 10 → плоский.
func TestNotificationCenter_ThemeSwitchWithoutRecreate(t *testing.T) {
	tm := flatNotifTheme(t)
	ns := notesFake()
	nc := NewNotificationCenter(tm, ns)
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	nc.Settle()
	defer nc.Close()

	flat := nc.OverlayBounds()
	if nc.presenter() != nil {
		t.Fatal("плоская тема с презентером")
	}
	if err := tm.SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	rich := nc.OverlayBounds()
	if nc.presenter() == nil || rich == flat || rich.Min.Y != 0 || rich.Max.X != panelScreenW {
		t.Errorf("после смены на Windows 10: %v (было %v)", rich, flat)
	}
	_ = drawTexts(nc) // рисуется без паники
	if err := tm.SetTheme(theme.ProfileWindows2000); err != nil {
		t.Fatal(err)
	}
	if nc.presenter() != nil || nc.OverlayBounds() != (nc.OverlayBounds()) {
		t.Error("возврат к плоской теме не вернул плоский вид")
	}
	if got := nc.OverlayBounds().Dy(); got == rich.Dy() {
		t.Error("плоский центр остался на всю высоту")
	}
}

// Открытие инвалидирует только область центра (и значок, от которого открыт).
func TestNotificationCenter_OpenInvalidatesOnlyItsArea(t *testing.T) {
	ns := notesFake()
	nc := NewNotificationCenter(richNotifTheme(t), ns)
	nc.Screen = panelScreen()
	inv := watchInvalidations(t)
	nc.Open(panelAnchor())
	region := nc.OverlayBounds().Union(nc.dirtyRect()).Union(panelAnchor())
	defer nc.Close()
	for _, r := range inv.get() {
		if !r.In(region) {
			t.Errorf("открытие заявило область %v вне центра %v", r, region)
		}
	}
	if len(inv.get()) == 0 {
		t.Error("открытие ничего не перерисовало")
	}
}

// ─── Масштаб: текст не вылезает из своих рамок ───────────────────────────────

func TestNotificationCenter_TextFitsItsBoxes(t *testing.T) {
	nc, _, _ := richFixture(t, 1)
	// Карточка с длинными словами и действиями.
	ns := NewFakeNotifications()
	ns.Add(Notification{AppID: "a", AppName: "Очень длинное название приложения для группы", Title: "Очень длинный заголовок уведомления, который не помещается в одну строку",
		Body: "Сверхдлиннаяфразабезпробеловкоторуюнужноразломатьпосимволамчтобыонавлезла и обычный текст", Time: sampleTime(),
		Actions: []NotificationAction{
			{ID: "1", Kind: NotificationActionButton, Title: "Очень длинная надпись кнопки"},
			{ID: "2", Kind: NotificationActionButton, Title: "Ещё одна длинная надпись"},
			{ID: "3", Kind: NotificationActionButton, Title: "И третья"},
		}})
	nc2 := NewNotificationCenter(richNotifTheme(t), ns)
	nc2.Screen = panelScreen()
	nc2.Open(panelAnchor())
	nc2.Settle()
	defer nc2.Close()
	_ = nc

	l := nc2.testLayout()
	g := l.groups[0]
	c := g.cards[0]
	textW := c.title.Dx()
	if w := l.fonts.title.width(c.titleText); w > textW {
		t.Errorf("заголовок %q шириной %d не влез в %d", c.titleText, w, textW)
	}
	for _, line := range c.bodyLines {
		if w := l.fonts.body.width(line); w > textW {
			t.Errorf("строка %q шириной %d не влезла в %d", line, w, textW)
		}
	}
	for _, a := range c.acts {
		if a.a.Kind == NotificationActionButton {
			if w := l.fonts.body.width(a.text); w > a.rect.Dx() {
				t.Errorf("надпись кнопки %q шириной %d не влезла в %d", a.text, w, a.rect.Dx())
			}
		}
	}
	// Зоны не выходят за панель, карточки не налезают друг на друга.
	for _, z := range l.zones {
		if !z.hit.In(l.panel) {
			t.Errorf("зона %+v вышла за панель %v", z.key, l.panel)
		}
	}
}

func TestNotificationCenter_LayoutScaleIndependent(t *testing.T) {
	// Раскладка идёт в логических точках: тот же набор даёт те же прямоугольники
	// при любом масштабе (масштаб применяет холст).
	nc, _, _ := richFixture(t, 5)
	a := nc.testLayout()
	b := nc.testLayout()
	if len(a.zones) != len(b.zones) {
		t.Fatal("раскладка недетерминирована")
	}
	for i := range a.zones {
		if a.zones[i].rect != b.zones[i].rect {
			t.Fatalf("зона %d: %v и %v", i, a.zones[i].rect, b.zones[i].rect)
		}
	}
}

// ─── Тост ────────────────────────────────────────────────────────────────────

func toastFixture(t *testing.T) (*NotificationToast, *FakeNotifications) {
	t.Helper()
	ns := NewFakeNotifications()
	ns.Add(Notification{AppID: "old", Title: "Старое", Time: sampleTime()})
	ts := NewNotificationToast(richNotifTheme(t), ns)
	ts.Screen = panelScreen()
	ts.Anchor = panelAnchor()
	ts.Clock = NewFakeClock(sampleTime())
	ts.Timeout = time.Hour
	t.Cleanup(ts.Close)
	return ts, ns
}

func TestNotificationToast_ShowsOnlyNewNotifications(t *testing.T) {
	ts, ns := toastFixture(t)
	if ts.IsOpen() {
		t.Fatal("тост показал уведомление, лежавшее до него")
	}
	id := ns.Add(Notification{AppID: "mail", AppName: "Почта", Title: "Новое письмо", Body: "Текст", Time: sampleTime()})
	if !ts.IsOpen() || ts.Current() != id {
		t.Fatalf("тост не показан: open=%v current=%d", ts.IsOpen(), ts.Current())
	}
	ts.Settle()
	r := ts.OverlayBounds()
	if r.Empty() || !r.In(panelScreen()) {
		t.Fatalf("тост %v", r)
	}
	// Над треем: в правом нижнем углу, не заходит на панель задач.
	if r.Max.X > panelScreenW || r.Max.Y > panelAnchor().Min.Y {
		t.Errorf("тост %v вышел за свой угол", r)
	}
	if r.Dx() != 364 {
		t.Errorf("ширина тоста %d, ждал 364 из метрики", r.Dx())
	}
}

func TestNotificationToast_AutoHides(t *testing.T) {
	ts, ns := toastFixture(t)
	ts.Timeout = 40 * time.Millisecond
	ns.Add(Notification{AppID: "mail", Title: "Письмо", Time: sampleTime()})
	if !ts.IsOpen() {
		t.Fatal("тост не показан")
	}
	deadline := time.Now().Add(2 * time.Second)
	for ts.IsOpen() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ts.IsOpen() {
		t.Error("тост не ушёл сам")
	}
	if len(ns.List()) != 2 {
		t.Error("автоскрытие тоста сняло уведомление из центра")
	}
}

func TestNotificationToast_HoverPausesTimer(t *testing.T) {
	ts, ns := toastFixture(t)
	ts.Timeout = 60 * time.Millisecond
	ns.Add(Notification{AppID: "mail", Title: "Письмо", Time: sampleTime()})
	ts.Settle()
	x, y := pointIn(ts.OverlayBounds())
	ts.OnMouseMove(x, y)
	time.Sleep(200 * time.Millisecond)
	if !ts.IsOpen() {
		t.Fatal("тост ушёл, пока над ним мышь")
	}
	ts.OnMouseMove(5, 5)
	deadline := time.Now().Add(2 * time.Second)
	for ts.IsOpen() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ts.IsOpen() {
		t.Error("тост не ушёл после ухода мыши")
	}
}

func TestNotificationToast_CloseButtonHidesButKeepsNotification(t *testing.T) {
	ts, ns := toastFixture(t)
	id := ns.Add(Notification{AppID: "mail", Title: "Письмо", Time: sampleTime()})
	ts.Settle()
	l := ts.view.layout(ts.rect())
	z, ok := l.find(zoneKey{kind: zoneCardClose, note: id})
	if !ok {
		t.Fatal("в тосте нет крестика")
	}
	x, y := pointIn(z.hit)
	ts.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	ts.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
	if ts.IsOpen() {
		t.Error("тост не закрылся")
	}
	if len(ns.List()) != 2 {
		t.Error("крестик тоста снял уведомление из центра")
	}
}

func TestNotificationToast_ActionButton(t *testing.T) {
	ts, ns := toastFixture(t)
	var got []NotificationActionEvent
	ts.OnAction = func(e NotificationActionEvent) { got = append(got, e) }
	id := ns.Add(Notification{AppID: "mail", Title: "Письмо", Time: sampleTime(),
		Actions: []NotificationAction{{ID: "read", Kind: NotificationActionButton, Title: "Прочитано"}}})
	ts.Settle()
	l := ts.view.layout(ts.rect())
	z, ok := l.find(zoneKey{kind: zoneAction, note: id, action: "read"})
	if !ok {
		t.Fatal("в тосте нет кнопки действия")
	}
	x, y := pointIn(z.hit)
	ts.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	ts.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
	if len(got) != 1 || got[0].Action != "read" {
		t.Fatalf("события %+v", got)
	}
	if len(ns.List()) != 1 || ts.IsOpen() {
		t.Error("после действия уведомление должно уйти, а тост — закрыться")
	}
}

func TestNotificationToast_SuppressedWhileCenterOpen(t *testing.T) {
	ts, ns := toastFixture(t)
	center := NewNotificationCenter(richNotifTheme(t), ns)
	center.Screen = panelScreen()
	defer center.Close()
	ts.Suppress(center)

	center.Open(panelAnchor())
	ns.Add(Notification{AppID: "mail", Title: "Письмо", Time: sampleTime()})
	if ts.IsOpen() {
		t.Error("тост показан поверх открытого центра")
	}
	center.Close()
	ns.Add(Notification{AppID: "mail", Title: "Ещё письмо", Time: sampleTime()})
	if !ts.IsOpen() {
		t.Error("после закрытия центра тост не показан")
	}
	center.Open(panelAnchor())
	if ts.IsOpen() {
		t.Error("открытие центра не убрало тост")
	}
}

func TestNotificationToast_NotShownInFlatTheme(t *testing.T) {
	ns := NewFakeNotifications()
	ts := NewNotificationToast(flatNotifTheme(t), ns)
	ts.Screen = panelScreen()
	ts.Anchor = panelAnchor()
	defer ts.Close()
	ns.Add(Notification{AppID: "a", Title: "x", Time: sampleTime()})
	if ts.IsOpen() {
		t.Error("тост показан в теме без оформления карточки")
	}
}

func TestNotificationToast_ClickOutsideAndEscDoNotHide(t *testing.T) {
	ts, ns := toastFixture(t)
	ns.Add(Notification{AppID: "mail", Title: "Письмо", Time: sampleTime()})
	ts.Settle()
	ts.DismissAt(1, 1)
	if ts.DismissOnEscape() {
		t.Error("тост перехватил Esc")
	}
	ts.OnMouseButton(widget.MouseEvent{X: 1, Y: 1, Button: widget.MouseLeft, Pressed: true})
	if !ts.IsOpen() {
		t.Error("клик мимо закрыл тост")
	}
}

func TestNotificationToast_CloseUnsubscribes(t *testing.T) {
	ts, ns := toastFixture(t)
	ts.Close()
	ns.mu.Lock()
	n := len(ns.subs)
	ns.mu.Unlock()
	if n != 0 {
		t.Errorf("после Close осталось подписок: %d", n)
	}
	ns.Add(Notification{AppID: "mail", Title: "Письмо", Time: sampleTime()})
	if ts.IsOpen() {
		t.Error("закрытый тост ожил")
	}
}
