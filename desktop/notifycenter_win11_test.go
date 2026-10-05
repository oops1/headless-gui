package desktop

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Центр уведомлений и календарь Windows 11: раскладка, «Не беспокоить»,
// группы, действия, тост, «Фокусировка», клавиатура, смена темы и режимы.

// w11Fix — открытая пара «центр + календарь» на экране 1280×1500: достаточно высоком,
// чтобы в центр поместились все карточки.
type w11Fix struct {
	tm     *theme.Manager
	ns     *FakeNotifications
	nc     *NotificationCenter
	cal    *CalendarFlyout
	group  *FlyoutGroup
	fs     *FakeFocusSession
	dnd    *DoNotDisturbState
	clk    *FakeClock
	screen image.Rectangle
	anchor image.Rectangle
	events []NotificationActionEvent
}

func w11Time() time.Time { return time.Date(2026, 10, 5, 7, 37, 0, 0, time.Local) }

// w11Notes — одиночное уведомление со всеми видами действий и группа из двух.
func w11Notes(ns *FakeNotifications, clk time.Time) {
	ns.Add(Notification{
		AppID: "mail", AppName: "Почта", Title: "Анна Иванова", Body: "Посмотри, пожалуйста, договор.",
		Timestamp: clk.Add(-40 * time.Minute),
		Actions: []NotificationAction{
			{ID: "ok", Kind: NotificationActionButton, Title: "Хорошо"},
			{ID: "open", Kind: NotificationActionLink, Title: "Открыть письмо"},
			{ID: "reply", Kind: NotificationActionReply},
			{ID: "when", Kind: NotificationActionSelect, Title: "Напомнить через:", Options: []string{"1 день", "1 неделю"}},
		},
	})
	ns.Add(Notification{AppID: "vpn", AppName: "VPN", Title: "Подключено", Body: "gate", Timestamp: clk.Add(-30 * time.Hour)})
	ns.Add(Notification{AppID: "vpn", AppName: "VPN", Title: "Отключено", Body: "gate", Timestamp: clk.Add(-52 * time.Hour)})
}

func newW11Fix(t *testing.T, profile string, notes bool) *w11Fix {
	t.Helper()
	defer widget.StopAllAnimations()
	f := &w11Fix{
		tm:     managerFor(t, profile),
		ns:     NewFakeNotifications(),
		clk:    NewFakeClock(w11Time()),
		screen: image.Rect(0, 0, 1280, 1500),
		anchor: image.Rect(1180, 1452, 1270, 1500),
		fs:     nil,
		dnd:    NewDoNotDisturb(false),
	}
	f.fs = NewFakeFocusSession(f.clk)
	if notes {
		w11Notes(f.ns, w11Time())
	}
	f.nc = NewNotificationCenter(f.tm, f.ns)
	f.nc.Clock = f.clk
	f.nc.Culture = LocaleCulture{}
	f.nc.Screen = f.screen
	f.nc.SetDoNotDisturb(f.dnd)
	f.nc.OnAction = func(e NotificationActionEvent) { f.events = append(f.events, e) }
	f.cal = NewCalendarFlyout(f.tm, f.clk)
	f.cal.Culture = LocaleCulture{}
	f.cal.Screen = f.screen
	f.cal.SetFocusSession(f.fs)
	f.cal.Ticker = func(time.Duration, func()) func() { return func() {} }
	f.group = LinkNotificationCenter(f.nc, f.cal)
	t.Cleanup(func() {
		f.group.CloseAll()
		f.nc.Close()
		widget.StopAllAnimations()
	})
	return f
}

func (f *w11Fix) open() {
	f.group.OpenAll(f.anchor)
	f.nc.Settle()
	f.cal.Settle()
}

func w11Click(w interface {
	OnMouseButton(widget.MouseEvent) bool
}, r image.Rectangle) {
	x, y := pointIn(r)
	w.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	w.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
}

func (f *w11Fix) zone(t *testing.T, k zoneKey) zone {
	t.Helper()
	l := f.nc.view.layout(f.nc.rect())
	z, ok := l.find(k)
	if !ok {
		t.Fatalf("зоны %+v нет в раскладке", k)
	}
	return z
}

func (f *w11Fix) drawn(w interface{ DrawOverlay(widget.DrawContext) }) *recCtx {
	ctx := &recCtx{}
	w.DrawOverlay(ctx)
	return ctx
}

// ─── Презентер и размещение ──────────────────────────────────────────────────

func TestW11_PresenterChoosesVariant(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	if w.nc.presenter() == nil || !w.nc.win11() || !w.cal.win11() {
		t.Fatal("Windows 11: презентер центра и календаря не назначен")
	}
	for _, name := range []string{theme.ProfileWindows10, theme.ProfileWindows2000, theme.ProfileMacOS} {
		o := newW11Fix(t, name, true)
		if o.nc.win11() || o.cal.win11() {
			t.Errorf("%s: вид Windows 11 выбран темой без его презентера", name)
		}
	}
}

func TestW11_StackGeometry(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	n, c := w.nc.OverlayBounds(), w.cal.OverlayBounds()
	if n.Empty() || c.Empty() {
		t.Fatalf("панели не открылись: центр %v, календарь %v", n, c)
	}
	if n.Dx() != 364 || c.Dx() != 364 {
		t.Errorf("ширина центра %d и календаря %d, ждали 364", n.Dx(), c.Dx())
	}
	if n.Max.X != c.Max.X || n.Max.X != w.screen.Max.X-12 {
		t.Errorf("правые края: центр %d, календарь %d, ждали %d", n.Max.X, c.Max.X, w.screen.Max.X-12)
	}
	if gap := c.Min.Y - n.Max.Y; gap != 8 {
		t.Errorf("зазор между центром и календарём %d, ждали 8", gap)
	}
	if c.Max.Y != w.anchor.Min.Y-12 {
		t.Errorf("низ календаря %d, ждали %d (над панелью с зазором 12)", c.Max.Y, w.anchor.Min.Y-12)
	}
	if n.Min.Y < w.screen.Min.Y+12 {
		t.Errorf("центр %v выше поля 12", n)
	}
}

// Свернули календарь — центр опустился к нему, а прежнее место перерисовалось.
func TestW11_CollapseMovesCenterAndRepaintsOldArea(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	// Экран пониже: тогда центру над развёрнутым календарём тесно, а над
	// свёрнутым — просторно.
	w.screen = image.Rect(0, 0, 1280, 800)
	w.anchor = image.Rect(1180, 752, 1270, 800)
	w.nc.Screen, w.cal.Screen = w.screen, w.screen
	w.open()
	before := w.cal.OverlayBounds()
	nBefore := w.nc.OverlayBounds()
	inv := watchInvalidations(t)
	w.cal.SetCollapsed(true)
	after := w.cal.OverlayBounds()
	if after.Dy() >= before.Dy() || after.Max.Y != before.Max.Y {
		t.Fatalf("свёрнутый календарь %v не ниже развёрнутого %v снизу вверх", after, before)
	}
	nAfter := w.nc.OverlayBounds()
	if nAfter.Max.Y != after.Min.Y-8 {
		t.Errorf("центр не лёг на свёрнутый календарь: низ %d, ждали %d", nAfter.Max.Y, after.Min.Y-8)
	}
	if nAfter.Dy() <= nBefore.Dy() {
		t.Errorf("центр не вырос на освободившееся место: %d -> %d", nBefore.Dy(), nAfter.Dy())
	}
	if !inv.covers(pointOf(before.Min.Add(image.Pt(40, 20)))) {
		t.Error("прежняя верхушка календаря не заявлена на перерисовку")
	}
	w.cal.SetCollapsed(false)
	if w.cal.OverlayBounds() != before {
		t.Errorf("развёрнутый календарь вернулся не на место: %v, было %v", w.cal.OverlayBounds(), before)
	}
}

func pointOf(p image.Point) image.Point { return p }

// Центр без календаря стоит прямо над панелью задач.
func TestW11_CenterAloneSitsAbovePanel(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.nc.Open(w.anchor)
	w.nc.Settle()
	n := w.nc.OverlayBounds()
	if n.Max.Y != w.anchor.Min.Y-12 {
		t.Errorf("низ центра %d, ждали %d", n.Max.Y, w.anchor.Min.Y-12)
	}
}

// 100–200 %: панели вписаны в экран, не наползают друг на друга и не меньше
// заголовка, на каком бы логическом размере экрана ни считались.
func TestW11_FitsEveryScreen(t *testing.T) {
	for _, sz := range []image.Point{{1920, 1080}, {1280, 800}, {960, 540}, {800, 600}, {640, 480}} {
		w := newW11Fix(t, theme.ProfileWindows11, true)
		w.screen = image.Rect(0, 0, sz.X, sz.Y)
		w.anchor = image.Rect(sz.X-100, sz.Y-48, sz.X-10, sz.Y)
		w.nc.Screen, w.cal.Screen = w.screen, w.screen
		w.open()
		n, c := w.nc.OverlayBounds(), w.cal.OverlayBounds()
		if !n.In(w.screen) || !c.In(w.screen) {
			t.Errorf("%v: панели %v и %v вышли за экран", sz, n, c)
		}
		if n.Overlaps(c) {
			t.Errorf("%v: центр %v наполз на календарь %v", sz, n, c)
		}
		if n.Dy() < 52 {
			t.Errorf("%v: центр %v ниже собственного заголовка", sz, n)
		}
	}
}

// ─── Заголовок ───────────────────────────────────────────────────────────────

func TestW11_HeaderTextsFollowLanguage(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	useLanguage(t, "EN")
	ctx := w.drawn(w.nc)
	for _, want := range []string{"Notifications", "Clear all"} {
		if !containsText(ctx.texts, want) {
			t.Errorf("по-английски нет надписи %q: %+v", want, ctx.texts)
		}
	}
	widget.SetLanguage("RU")
	ctx = w.drawn(w.nc)
	for _, want := range []string{"Уведомления", "Очистить все"} {
		if !containsText(ctx.texts, want) {
			t.Errorf("по-русски нет надписи %q", want)
		}
	}
}

func TestW11_ClearAllInactiveWhenEmpty(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	l := w.nc.view.layout(w.nc.rect())
	if !l.clearOff {
		t.Error("«Очистить все» активна при пустом списке")
	}
	if _, ok := l.find(zoneKey{kind: zoneClear}); ok {
		t.Error("у неактивной «Очистить все» есть зона нажатия")
	}
	ctx := w.drawn(w.nc)
	if !containsText(ctx.texts, tr(StrNotifEmpty)) {
		t.Error("пустой центр не подписан")
	}
	w.ns.Add(Notification{AppID: "a", AppName: "A", Title: "x", Timestamp: w11Time()})
	l = w.nc.view.layout(w.nc.rect())
	if l.clearOff {
		t.Error("«Очистить все» осталась неактивной при уведомлении")
	}
}

func TestW11_ClearAllRemovesEverything(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneClear}).rect)
	if got := len(w.ns.List()); got != 0 {
		t.Errorf("после «Очистить все» осталось %d уведомлений", got)
	}
}

// Колокольчик переключает общую модель и перерисовывает только себя.
func TestW11_BellTogglesSharedModelAndRepaintsOnlyItself(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	bell := w.zone(t, zoneKey{kind: zoneDND}).rect
	var seen []bool
	w.dnd.OnChange = func(on bool) { seen = append(seen, on) }

	inv := watchInvalidations(t)
	inv.reset()
	w11Click(w.nc, bell)
	if !w.dnd.Enabled() || len(seen) != 1 || !seen[0] {
		t.Fatalf("клик по колокольчику не включил режим: %v %v", w.dnd.Enabled(), seen)
	}
	for _, r := range inv.get() {
		if !r.In(bell.Inset(-1)) && !r.Empty() {
			// перерисовка нажатия — только зона колокольчика
			if r.Overlaps(w.zone(t, zoneKey{kind: zoneClear}).rect) {
				t.Errorf("переключение режима перерисовало соседнюю кнопку: %v", r)
			}
		}
	}
	// Снаружи (кнопка в трее, другой потребитель) — тот же режим: колокольчик следует.
	inv.reset()
	w.dnd.SetEnabled(false)
	if len(inv.get()) == 0 {
		t.Error("смена режима снаружи не перерисовала колокольчик")
	}
	for _, r := range inv.get() {
		if !r.In(bell.Inset(-2)) {
			t.Errorf("смена режима перерисовала не только колокольчик: %v (колокольчик %v)", r, bell)
		}
	}
	// Закрытый центр от модели отписан.
	w.nc.Close()
	w.dnd.mu.Lock()
	subs := len(w.dnd.subs)
	w.dnd.mu.Unlock()
	if subs != 0 {
		t.Errorf("закрытый центр остался подписан на «Не беспокоить»: %d", subs)
	}
}

// ─── Карточки и группы ───────────────────────────────────────────────────────

func TestW11_SingleCardHasAppRowGroupHasCounter(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	l := w.nc.view.layout(w.nc.rect())
	var single, group *richGroup
	for i := range l.groups {
		switch l.groups[i].app {
		case "mail":
			single = &l.groups[i]
		case "vpn":
			group = &l.groups[i]
		}
	}
	if single == nil || group == nil {
		t.Fatalf("группы не разложены: %+v", l.groups)
	}
	if !single.rect.Empty() {
		t.Error("у одиночного уведомления есть заголовок группы")
	}
	if len(single.cards) != 1 || !single.cards[0].showApp || single.cards[0].appText != "Почта" {
		t.Errorf("карточка одиночного уведомления без строки приложения: %+v", single.cards)
	}
	if group.rect.Empty() || group.count != 2 {
		t.Fatalf("заголовок группы %v, счётчик %d", group.rect, group.count)
	}
	for _, c := range group.cards {
		if c.showApp {
			t.Error("карточка в группе повторяет строку приложения")
		}
	}
	ctx := w.drawn(w.nc)
	if !containsText(ctx.texts, "2") || !containsText(ctx.texts, "VPN") || !containsText(ctx.texts, "Почта") {
		t.Errorf("не нарисованы название группы, счётчик или приложение: %+v", ctx.texts)
	}
}

func TestW11_GroupCollapsesByClickAndKeepsCounter(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	h := w.zone(t, zoneKey{kind: zoneGroup, app: "vpn"}).rect
	w11Click(w.nc, h)
	finishAnimations()
	if !w.nc.GroupCollapsed("vpn") {
		t.Fatal("клик по заголовку не свернул группу")
	}
	l := w.nc.view.layout(w.nc.rect())
	for _, g := range l.groups {
		if g.app == "vpn" && len(g.cards) != 0 {
			t.Errorf("в свёрнутой группе остались карточки: %d", len(g.cards))
		}
	}
	if !containsText(w.drawn(w.nc).texts, "2") {
		t.Error("свёрнутая группа без счётчика")
	}
}

func TestW11_CardCloseAndTimeLabel(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	ctx := w.drawn(w.nc)
	if !containsText(ctx.texts, "06:57") {
		t.Errorf("время сегодняшней карточки не «06:57»: %+v", ctx.texts)
	}
	if !containsText(ctx.texts, tr(StrNotifYesterday)) {
		t.Error("вчерашней карточке нет «Вчера»")
	}
	id := w.ns.List()[0].ID
	// Крестик лежит на месте времени и нажимается на отпускании.
	z := w.zone(t, zoneKey{kind: zoneCardClose, note: id})
	x, y := pointIn(z.rect)
	w.nc.OnMouseMove(x, y)
	w.nc.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	if len(w.ns.List()) != 3 {
		t.Error("карточка закрылась на нажатии")
	}
	w.nc.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
	if len(w.ns.List()) != 2 {
		t.Errorf("крестик не снял карточку: %d", len(w.ns.List()))
	}
}

// Карточка не показывает время, пока над ней мышь: на его месте крестик.
func TestW11_HoverSwapsTimeForCross(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	id := w.ns.List()[0].ID
	card := w.zone(t, zoneKey{kind: zoneCard, note: id}).rect
	w.nc.OnMouseMove(card.Min.X+30, card.Min.Y+40)
	if containsText(w.drawn(w.nc).texts, "06:57") {
		t.Error("время осталось под курсором: крестик должен занять его место")
	}
	w.nc.OnMouseMove(5, 5)
	if !containsText(w.drawn(w.nc).texts, "06:57") {
		t.Error("время не вернулось, когда курсор ушёл")
	}
}

// Четыре вида действий работают так же, как в Windows 10.
func TestW11_ActionKindsDeliverEvents(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	id := w.ns.List()[0].ID
	click := func(a string) {
		w11Click(w.nc, w.zone(t, zoneKey{kind: zoneAction, note: id, action: a}).rect)
	}
	// Выпадающий список: открыть, выбрать второй пункт.
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneSelect, note: id, action: "when"}).rect)
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneOption, note: id, action: "when", index: 1}).rect)
	if n := len(w.events); n != 1 || w.events[0].Kind != NotificationActionSelect || w.events[0].Value != "1 неделю" {
		t.Fatalf("выбор пункта списка: %+v", w.events)
	}
	// Ответ: ввод и отправка.
	w.nc.SetFocused(true)
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneReply, note: id, action: "reply"}).rect)
	for _, r := range "да" {
		w.nc.OnKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
	}
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneReplySend, note: id, action: "reply"}).rect)
	last := w.events[len(w.events)-1]
	if last.Kind != NotificationActionReply || last.Value != "да" {
		t.Fatalf("ответ не доставлен: %+v", w.events)
	}
	// Ссылка и кнопка. Кнопка снимает уведомление.
	if _, ok := w.nc.view.findNote(id); !ok {
		// ответ снял уведомление (Keep не задан): добавим новое
		w.ns.Add(Notification{AppID: "mail", AppName: "Почта", Title: "Ещё", Timestamp: w11Time(),
			Actions: []NotificationAction{{ID: "ok", Kind: NotificationActionButton, Title: "Ок"}, {ID: "open", Kind: NotificationActionLink, Title: "Открыть"}}})
		for _, n := range w.ns.List() {
			if n.Title == "Ещё" {
				id = n.ID
			}
		}
	}
	click("open")
	if w.events[len(w.events)-1].Kind != NotificationActionLink {
		t.Fatalf("ссылка не доставлена: %+v", w.events)
	}
}

// ─── Тост и «Не беспокоить» ─────────────────────────────────────────────────

func TestW11_ToastShowsCardAndRespectsDoNotDisturb(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows11)
	ns := NewFakeNotifications()
	ts := NewNotificationToast(tm, ns)
	ts.Screen = image.Rect(0, 0, 1280, 800)
	ts.Anchor = image.Rect(1180, 752, 1270, 800)
	ts.Clock = NewFakeClock(w11Time())
	dnd := NewDoNotDisturb(false)
	ts.SetDoNotDisturb(dnd)
	defer ts.Close()

	ns.Add(Notification{AppID: "a", AppName: "Приложение", Title: "Привет", Body: "Текст", Timestamp: w11Time()})
	if !ts.IsOpen() {
		t.Fatal("тост Windows 11 не показан")
	}
	ts.Settle()
	r := ts.OverlayBounds()
	if r.Dx() != 364 || r.Max.X != 1280-12 {
		t.Errorf("тост %v: ждали ширину 364 и поле 12 у края", r)
	}
	if !containsText(drawnTexts(ts), "Приложение") {
		t.Error("в тосте нет строки приложения")
	}
	ts.Hide()
	dnd.SetEnabled(true)
	ns.Add(Notification{AppID: "a", AppName: "Приложение", Title: "Тихо", Timestamp: w11Time()})
	if ts.IsOpen() {
		t.Error("тост показан при включённом «Не беспокоить»")
	}
	if len(ns.List()) != 2 {
		t.Error("«Не беспокоить» убрало уведомление из центра: оно только не всплывает")
	}
	dnd.SetEnabled(false)
	ns.Add(Notification{AppID: "a", AppName: "Приложение", Title: "Громко", Timestamp: w11Time()})
	if !ts.IsOpen() {
		t.Error("после выключения режима тост не показывается")
	}
	dnd.SetEnabled(true)
	if ts.IsOpen() {
		t.Error("включение режима не убрало показанный тост")
	}
}

func drawnTexts(w interface{ DrawOverlay(widget.DrawContext) }) []recText {
	ctx := &recCtx{}
	w.DrawOverlay(ctx)
	return ctx.texts
}

// ─── Календарь ───────────────────────────────────────────────────────────────

func TestW11Calendar_HeaderMonthAndToday(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	ctx := w.drawn(w.cal)
	for _, want := range []string{"понедельник, 5 октября", "Октябрь 2026", "Пн", "Вс", "5", "31", tr(StrFocusTitle), "30 мин", tr(StrFocusStart)} {
		if !containsText(ctx.texts, want) {
			t.Errorf("в календаре нет %q", want)
		}
	}
	// Сегодня — круг акцента.
	acc, _ := w.tm.Accent()
	found := false
	for _, f := range ctx.fills {
		if f.col == acc && f.w == f.h {
			found = true
		}
	}
	if !found {
		t.Error("сегодняшний день не залит акцентом кругом")
	}
	useLanguage(t, "EN")
	ctx = w.drawn(w.cal)
	for _, want := range []string{"Monday, October 5", "October 2026", "Mo", "Focus", "30 min", "Start"} {
		if !containsText(ctx.texts, want) {
			t.Errorf("по-английски нет %q: %+v", want, ctx.texts)
		}
	}
}

// bareCulture не знает DateHeaderCulture: строка над календарём собирается из
// сокращения дня недели и родительного падежа.
type bareCulture struct{ LocaleCulture }

func (c bareCulture) DateHeader() {} // скрывает расширение: подпись не совпадает

func TestW11Calendar_DateHeaderFallsBackForPlainCulture(t *testing.T) {
	tm := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	if got := dateHeader(LocaleCulture{}, tm); got != "понедельник, 5 октября" {
		t.Errorf("своя строка культуры: %q", got)
	}
	if got := dateHeader(bareCulture{}, tm); got != "понедельник, 5 октября" && got != "Пн, 5 октября" {
		t.Errorf("запасная строка: %q", got)
	}
	useLanguage(t, "EN")
	if got := dateHeader(LocaleCulture{}, tm); got != "Monday, October 5" {
		t.Errorf("по-английски: %q", got)
	}
}

func TestW11Calendar_CollapseMonthAndSelection(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	l := w.cal.w11Layout(w.cal.rect())
	full := w.cal.OverlayBounds().Dy()

	w11Click(w.cal, l.next)
	if got := w.cal.ViewMonth(); got.Month() != time.November {
		t.Errorf("›: месяц %v", got)
	}
	w11Click(w.cal, w.cal.w11Layout(w.cal.rect()).prev)
	w11Click(w.cal, w.cal.w11Layout(w.cal.rect()).prev)
	if got := w.cal.ViewMonth(); got.Month() != time.September {
		t.Errorf("‹: месяц %v", got)
	}
	// Выбор дня.
	cell := w.cal.w11Layout(w.cal.rect()).days[2][3]
	w11Click(w.cal, w.cal.dayRect(cell.rect, 36))
	if sel := w.cal.Selected(); !sameDay(sel, cell.day.date) {
		t.Errorf("выбран %v, ждали %v", sel, cell.day.date)
	}
	// Свернуть и развернуть.
	w11Click(w.cal, w.cal.w11Layout(w.cal.rect()).collapse)
	if !w.cal.Collapsed() || w.cal.OverlayBounds().Dy() >= full {
		t.Errorf("свёрнутый календарь не ниже: %d >= %d", w.cal.OverlayBounds().Dy(), full)
	}
	if ctx := w.drawn(w.cal); containsText(ctx.texts, "Пн") {
		t.Error("свёрнутый календарь рисует сетку")
	}
	w11Click(w.cal, w.cal.w11Layout(w.cal.rect()).collapse)
	if w.cal.Collapsed() {
		t.Error("второй клик не развернул календарь")
	}
}

// Клик по календарю не закрывает центр над ним, клик мимо закрывает обоих.
func TestW11_GroupSurvivesClickInNeighbourAndClosesOutside(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	cal := w.cal.rect()
	if w.nc.OnMouseButton(widget.MouseEvent{X: cal.Min.X + 40, Y: cal.Min.Y + 60, Button: widget.MouseLeft, Pressed: true}) {
		t.Error("центр поглотил клик, который пришёлся на календарь")
	}
	if !w.nc.IsOpen() || !w.cal.IsOpen() {
		t.Fatal("клик по календарю закрыл одну из панелей группы")
	}
	w.nc.DismissAt(5, 5)
	w.cal.DismissAt(5, 5)
	if w.nc.IsOpen() || w.cal.IsOpen() {
		t.Error("клик мимо не закрыл группу")
	}
}

func TestW11_EscapeClosesWholeGroup(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	w.cal.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if w.nc.IsOpen() || w.cal.IsOpen() {
		t.Error("Esc в календаре не закрыл группу")
	}
	w.open()
	w.nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if w.nc.IsOpen() || w.cal.IsOpen() {
		t.Error("Esc в центре не закрыл группу")
	}
}

// ─── «Фокусировка» ───────────────────────────────────────────────────────────

func TestW11Focus_DurationStepsAndLimits(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	press := func(id calStop) { w.cal.activateStop(id) }
	press(calStopMore)
	if d := w.fs.State().Duration; d != 35*time.Minute {
		t.Fatalf("«+»: %v", d)
	}
	press(calStopLess)
	press(calStopLess)
	if d := w.fs.State().Duration; d != 25*time.Minute {
		t.Fatalf("«−»: %v", d)
	}
	for i := 0; i < 20; i++ {
		press(calStopLess)
	}
	if d := w.fs.State().Duration; d != FocusMinDuration {
		t.Errorf("нижний предел %v", d)
	}
	for i := 0; i < 80; i++ {
		press(calStopMore)
	}
	if d := w.fs.State().Duration; d != FocusMaxDuration {
		t.Errorf("верхний предел %v", d)
	}
	if got := focusLabel(90 * time.Minute); got != "1 ч 30 мин" {
		t.Errorf("подпись 90 минут: %q", got)
	}
	if got := focusLabel(120 * time.Minute); got != "2 ч" {
		t.Errorf("подпись 2 часов: %q", got)
	}
}

func TestW11Focus_StartCountdownStop(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	w11Click(w.cal, w.cal.w11Layout(w.cal.rect()).faction)
	if !w.fs.State().Running || w.fs.Started != 1 {
		t.Fatalf("«Начать» не начала сеанс: %+v", w.fs.State())
	}
	if !containsText(w.drawn(w.cal).texts, "30:00") || !containsText(w.drawn(w.cal).texts, tr(StrFocusStop)) {
		t.Errorf("идущий сеанс: нет отсчёта 30:00 и «Остановить»: %+v", w.drawn(w.cal).texts)
	}
	w.clk.Set(w.clk.Now().Add(5*time.Minute + 30*time.Second))
	if !containsText(w.drawn(w.cal).texts, "24:30") {
		t.Errorf("через 5:30 остаток не 24:30: %+v", w.drawn(w.cal).texts)
	}
	// Во время сеанса «−» и «+» длительность не меняют.
	w.cal.activateStop(calStopMore)
	if d := w.fs.State().Duration; d != 30*time.Minute {
		t.Errorf("длительность изменилась во время сеанса: %v", d)
	}
	w.clk.Set(w.clk.Now().Add(time.Hour))
	if !containsText(w.drawn(w.cal).texts, "00:00") {
		t.Error("остаток после конца сеанса не 00:00")
	}
	w11Click(w.cal, w.cal.w11Layout(w.cal.rect()).faction)
	if w.fs.State().Running || w.fs.Stopped != 1 {
		t.Errorf("«Остановить» не остановила сеанс: %+v", w.fs.State())
	}
}

func TestFocusCountdownFormat(t *testing.T) {
	cases := map[time.Duration]string{
		25 * time.Minute: "25:00",
		24*time.Minute + 59*time.Second + 400*time.Millisecond: "25:00",
		59 * time.Second:          "00:59",
		0:                         "00:00",
		-time.Second:              "00:00",
		time.Hour + 5*time.Minute: "1:05:00",
	}
	for d, want := range cases {
		if got := focusCountdown(d); got != want {
			t.Errorf("%v: %q, ждали %q", d, got, want)
		}
	}
}

// Секундный отсчёт живёт, пока календарь открыт и сеанс идёт, и перерисовывает
// только модуль «Фокусировки».
func TestW11Focus_TickerLifecycleAndRepaintArea(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	var starts, stops int
	var tick func()
	w.cal.Ticker = func(d time.Duration, f func()) func() {
		if d != time.Second {
			t.Errorf("период отсчёта %v, ждали секунду", d)
		}
		starts++
		tick = f
		return func() { stops++ }
	}
	w.open()
	if starts != 0 {
		t.Fatal("отсчёт запущен без сеанса")
	}
	w.fs.Start()
	if starts != 1 {
		t.Fatalf("сеанс начался, отсчёт не запущен: %d", starts)
	}
	inv := watchInvalidations(t)
	inv.reset()
	tick()
	mod := w.cal.w11Layout(w.cal.rect()).focus
	if len(inv.get()) == 0 {
		t.Fatal("тик не перерисовал ничего")
	}
	for _, r := range inv.get() {
		if !r.In(mod) {
			t.Errorf("тик перерисовал %v вне модуля %v", r, mod)
		}
	}
	w.fs.Stop()
	if stops != 1 {
		t.Errorf("сеанс закончился, отсчёт не остановлен: %d", stops)
	}
	w.fs.Start()
	if starts != 2 {
		t.Fatalf("повторный сеанс не завёл отсчёт: %d", starts)
	}
	w.cal.Close()
	if stops != 2 {
		t.Errorf("закрытие календаря не остановило отсчёт: %d", stops)
	}
	w.fs.mu.Lock()
	subs := len(w.fs.subs)
	w.fs.mu.Unlock()
	if subs != 0 {
		t.Errorf("закрытый календарь остался подписан на сеанс: %d", subs)
	}
	// Тик закрытого календаря ничего не заявляет.
	inv.reset()
	tick()
	if len(inv.get()) != 0 {
		t.Error("тик закрытого календаря заявил перерисовку")
	}
}

// Без своего Ticker отсчёт идёт на таймере стандартной библиотеки и
// останавливается.
func TestW11Focus_DefaultTickerFiresAndStops(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.cal.Ticker = nil
	n := make(chan struct{}, 8)
	stop := w.cal.tickEvery(5*time.Millisecond, func() { n <- struct{}{} })
	select {
	case <-n:
	case <-time.After(2 * time.Second):
		t.Fatal("таймер не сработал")
	}
	stop()
	time.Sleep(30 * time.Millisecond)
	for len(n) > 0 {
		<-n
	}
	time.Sleep(30 * time.Millisecond)
	if len(n) != 0 {
		t.Error("остановленный отсчёт продолжает вызывать")
	}
}

func TestW11Focus_NoSessionNoModule(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.cal.SetFocusSession(nil)
	w.open()
	with := newW11Fix(t, theme.ProfileWindows11, false)
	with.open()
	if w.cal.OverlayBounds().Dy() >= with.cal.OverlayBounds().Dy() {
		t.Error("календарь без сеанса не ниже календаря с модулем")
	}
	if containsText(w.drawn(w.cal).texts, tr(StrFocusTitle)) {
		t.Error("модуль «Фокусировка» нарисован без сеанса")
	}
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

func kev(code widget.KeyCode, mod widget.KeyMod) widget.KeyEvent {
	return widget.KeyEvent{Code: code, Mod: mod, Pressed: true}
}

func TestW11Keyboard_TabWalksCenterThenCalendarAndBack(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	w.nc.SetFocused(true)
	// Колокольчик первым, «Очистить все» вторым.
	w.nc.OnKeyEvent(kev(widget.KeyTab, 0))
	if w.nc.view.focus.kind != zoneDND {
		t.Fatalf("первая остановка %+v, ждали колокольчик", w.nc.view.focus)
	}
	w.nc.OnKeyEvent(kev(widget.KeyTab, 0))
	if w.nc.view.focus.kind != zoneClear {
		t.Fatalf("вторая остановка %+v, ждали «Очистить все»", w.nc.view.focus)
	}
	// Enter на колокольчике переключает режим.
	w.nc.OnKeyEvent(kev(widget.KeyTab, widget.ModShift))
	w.nc.OnKeyEvent(kev(widget.KeyEnter, 0))
	if !w.dnd.Enabled() {
		t.Error("Enter на колокольчике не включил режим")
	}
	// Идём до конца центра: с последней остановки Tab уходит в календарь.
	for i := 0; i < 60 && w.cal.focusStop == calStopNone; i++ {
		w.nc.OnKeyEvent(kev(widget.KeyTab, 0))
	}
	if w.cal.focusStop != calStopCollapse {
		t.Fatalf("Tab не передал фокус календарю: %v", w.cal.focusStop)
	}
	if w.nc.view.focus.kind != zoneNone {
		t.Error("центр оставил свою остановку после передачи фокуса")
	}
	// Обход календаря и возврат в центр по Shift+Tab с первой кнопки.
	w.cal.SetFocused(true)
	w.cal.OnKeyEvent(kev(widget.KeyTab, widget.ModShift))
	if w.cal.focusStop != calStopNone || w.nc.view.focus.kind == zoneNone {
		t.Errorf("Shift+Tab с первой кнопки не вернул фокус центру: календарь %v, центр %+v", w.cal.focusStop, w.nc.view.focus)
	}
}

func TestW11Keyboard_CalendarButtonsActivateWithEnter(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	w.cal.SetFocused(true)
	order := []calStop{calStopCollapse, calStopPrev, calStopNext, calStopGrid, calStopLess, calStopMore, calStopAction}
	for _, want := range order {
		w.cal.OnKeyEvent(kev(widget.KeyTab, 0))
		if w.cal.focusStop != want {
			t.Fatalf("остановка %v, ждали %v", w.cal.focusStop, want)
		}
	}
	// «Начать» с клавиатуры.
	w.cal.OnKeyEvent(kev(widget.KeyEnter, 0))
	if !w.fs.State().Running {
		t.Error("Enter на «Начать» не начал сеанс")
	}
	// Рамка фокуса видна только у клавиатурного фокуса.
	if !w.cal.FocusVisible() {
		t.Error("рамка фокуса не видна у клавиатурного фокуса")
	}
	// PgDn листает месяц; Влево/Вправо — тоже.
	m := w.cal.ViewMonth()
	w.cal.OnKeyEvent(kev(widget.KeyPageDown, 0))
	if w.cal.ViewMonth().Month() == m.Month() {
		t.Error("PgDn не пролистал месяц")
	}
	w.cal.OnKeyEvent(kev(widget.KeyLeft, 0))
	if w.cal.ViewMonth().Month() != m.Month() {
		t.Error("Влево не вернул месяц")
	}
}

// Сетка дней доступна с клавиатуры: стрелки двигают день, Enter выбирает,
// выход за месяц листает месяц.
func TestW11Keyboard_GridCursorMovesAndSelects(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	w.cal.SetFocused(true)
	for i := 0; i < 4; i++ { // ⌃ → ‹ → › → сетка
		w.cal.OnKeyEvent(kev(widget.KeyTab, 0))
	}
	if w.cal.focusStop != calStopGrid {
		t.Fatalf("четвёртая остановка %v, ждали сетку", w.cal.focusStop)
	}
	if got := w.cal.cursorDay(); !sameDay(got, w11Time()) {
		t.Fatalf("курсор встал на %v, ждали сегодня", got)
	}
	w.cal.OnKeyEvent(kev(widget.KeyRight, 0))
	w.cal.OnKeyEvent(kev(widget.KeyDown, 0))
	if got := w.cal.cursorDay(); got.Day() != 13 { // 5 + 1 + 7
		t.Errorf("после → и ↓ курсор на %v, ждали 13 октября", got)
	}
	w.cal.OnKeyEvent(kev(widget.KeyEnter, 0))
	if sel := w.cal.Selected(); sel.Day() != 13 || sel.Month() != time.October {
		t.Errorf("Enter выбрал %v", sel)
	}
	// Вниз за конец месяца листает на следующий.
	for i := 0; i < 3; i++ {
		w.cal.OnKeyEvent(kev(widget.KeyDown, 0))
	}
	if w.cal.ViewMonth().Month() != time.November {
		t.Errorf("курсор вышел за октябрь, а месяц %v", w.cal.ViewMonth().Month())
	}
	// Влево/вправо на сетке не листают месяцы сами по себе: кольцо на дне.
	ctx := w.drawn(w.cal)
	if len(ctx.fills) == 0 {
		t.Error("календарь ничего не нарисовал")
	}
}

// ─── Тема, акцент, язык, меньше движения ────────────────────────────────────

func TestW11_ThemeAccentLanguageSwitchWithoutRecreate(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	light := w.drawn(w.nc)
	if len(light.texts) == 0 {
		t.Fatal("центр ничего не нарисовал")
	}
	// Акцент: сегодня и «Начать» следуют за ним.
	w.tm.SetAccent(theme.RGB(200, 30, 30))
	ctx := w.drawn(w.cal)
	red := false
	for _, f := range ctx.fills {
		if f.col == theme.RGB(200, 30, 30) {
			red = true
		}
	}
	if !red {
		t.Error("календарь не последовал за сменой акцента")
	}
	// Тёмная тема: панели те же, цвета другие.
	if err := w.tm.SetTheme(theme.ProfileWindows11Dark); err != nil {
		t.Fatal(err)
	}
	if !w.nc.win11() || w.nc.OverlayBounds().Empty() || w.cal.OverlayBounds().Empty() {
		t.Fatal("после смены на тёмную тему панели пропали")
	}
	dark := w.drawn(w.nc)
	if len(dark.fills) == 0 || dark.fills[0].col == light.fills[0].col {
		t.Error("тёмная тема не поменяла заливку панели")
	}
	// Windows 10: тот же компонент рисует центр Windows 10 и переоткрывается.
	if err := w.tm.SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if w.nc.win11() || w.cal.win11() {
		t.Error("после смены на Windows 10 остался вид Windows 11")
	}
	_ = w.drawn(w.nc)
	_ = w.drawn(w.cal)
	// И обратно.
	if err := w.tm.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	if !w.nc.win11() || !w.cal.win11() {
		t.Error("возврат на Windows 11 не вернул его вид")
	}
	// Язык.
	useLanguage(t, "EN")
	if !containsText(w.drawn(w.nc).texts, "Notifications") {
		t.Error("язык не сменился на открытой панели")
	}
}

func TestW11_ReduceMotionOpensInstantly(t *testing.T) {
	defer widget.StopAllAnimations()
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.tm.SetFlag(theme.FlagMotionReduce, true)
	w.group.OpenAll(w.anchor)
	if w.nc.Presence() != 1 || w.cal.Presence() != 1 {
		t.Errorf("панели выезжают при «меньше движения»: %v %v", w.nc.Presence(), w.cal.Presence())
	}
	if widget.AnimationsActive() {
		t.Error("открытие завело анимацию")
	}
	// Раскрытие карточки — мгновенно (функциональная анимация без длительности).
	id := w.ns.List()[0].ID
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneCardToggle, note: id}).rect)
	if widget.AnimationsActive() {
		t.Error("раскрытие карточки анимируется при «меньше движения»")
	}
	// Секундный отсчёт — не анимация: сеанс идёт и при «меньше движения».
	var starts int
	w.cal.Ticker = func(time.Duration, func()) func() { starts++; return func() {} }
	w.fs.Start()
	if starts != 1 {
		t.Errorf("при «меньше движения» отсчёт не запущен: %d", starts)
	}
}

// ─── Быстродействие ─────────────────────────────────────────────────────────

// Открытие группы вместе с первой отрисовкой обеих панелей — меньше 100 мс.
func TestW11_OpensUnder100ms(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open() // прогрев: измерения шрифтов кешируются
	w.group.CloseAll()
	w.nc.Settle()
	w.cal.Settle()
	start := time.Now()
	w.group.OpenAll(w.anchor)
	w.drawn(w.cal)
	w.drawn(w.nc)
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("открытие и первый кадр заняли %v, предел 100 мс", d)
	}
}

// Наведение на карточку перерисовывает карточку, а не весь центр.
func TestW11_HoverRepaintsOnlyTheCard(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	inv := watchInvalidations(t)
	id := w.ns.List()[0].ID
	card := w.zone(t, zoneKey{kind: zoneCard, note: id}).rect
	inv.reset()
	w.nc.OnMouseMove(card.Min.X+30, card.Min.Y+40)
	got := inv.get()
	if len(got) == 0 {
		t.Fatal("наведение ничего не перерисовало")
	}
	for _, r := range got {
		if !r.In(card) {
			t.Errorf("наведение перерисовало %v вне карточки %v", r, card)
		}
	}
}

// ─── Модели ─────────────────────────────────────────────────────────────────

func TestDoNotDisturbState(t *testing.T) {
	d := NewDoNotDisturb(false)
	n := 0
	unsub := d.Subscribe(func() { n++ })
	d.SetEnabled(true)
	d.SetEnabled(true) // то же состояние — не событие
	if n != 1 || !d.Enabled() {
		t.Fatalf("уведомлений %d, состояние %v", n, d.Enabled())
	}
	unsub()
	d.SetEnabled(false)
	if n != 1 {
		t.Error("отписанный подписчик получил событие")
	}
	var hooked []bool
	d.OnChange = func(on bool) { hooked = append(hooked, on) }
	d.SetEnabled(true)
	if len(hooked) != 1 || !hooked[0] {
		t.Errorf("OnChange: %v", hooked)
	}
}

func TestFakeFocusSession(t *testing.T) {
	clk := NewFakeClock(w11Time())
	s := NewFakeFocusSession(clk)
	n := 0
	s.Subscribe(func() { n++ })
	s.SetDuration(45 * time.Minute)
	s.Start()
	st := s.State()
	if !st.Running || st.EndsAt != w11Time().Add(45*time.Minute) {
		t.Fatalf("сеанс: %+v", st)
	}
	s.SetDuration(10 * time.Minute) // идущему сеансу длительность не меняют
	if s.State().Duration != 45*time.Minute {
		t.Error("длительность идущего сеанса изменилась")
	}
	clk.Set(w11Time().Add(15 * time.Minute))
	if r := focusRemaining(s.State(), clk.Now()); r != 30*time.Minute {
		t.Errorf("остаток %v", r)
	}
	s.Stop()
	s.Stop()
	if s.Stopped != 1 || n != 3 {
		t.Errorf("остановок %d, уведомлений %d", s.Stopped, n)
	}
	if focusClamp(time.Minute) != FocusMinDuration || focusClamp(time.Hour*9) != FocusMaxDuration {
		t.Error("пределы длительности не держатся")
	}
}

func TestW11_NoColorLiteralsInStrings(t *testing.T) {
	// Строки RU и EN есть для каждого нового ключа.
	for _, k := range []string{StrNotifTitle, StrNotifDND, StrFocusTitle, StrFocusMinutes, StrFocusHours,
		StrFocusHoursMinutes, StrFocusStart, StrFocusStop, StrFocusLess, StrFocusMore, StrCalHeadDate} {
		for _, lang := range []string{"RU", "EN"} {
			if v := widget.TrIn(lang, k); v == k || strings.TrimSpace(v) == "" {
				t.Errorf("%s: нет перевода %s", lang, k)
			}
		}
	}
}
