package desktop

import (
	"strconv"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Стопки уведомлений Windows 11 и модуль «Фокусировка» как у настоящей
// Windows 11 (v3.34).

func stackGroup(t *testing.T, w *w11Fix, app AppID) richGroup {
	t.Helper()
	l := w.nc.view.layout(w.nc.rect())
	for _, g := range l.groups {
		if g.app == app {
			return g
		}
	}
	t.Fatalf("группы %q нет в раскладке", app)
	return richGroup{}
}

// Несколько уведомлений одного приложения сворачиваются в стопку: самое свежее
// и кнопка «+N уведомл.».
func TestW11Stack_CollapsedByDefault(t *testing.T) {
	testreset.Globals(t)
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	g := stackGroup(t, w, "vpn")
	if !g.stack || g.stackOpen || len(g.cards) != 1 {
		t.Fatalf("группа из двух уведомлений: stack=%v open=%v карточек %d", g.stack, g.stackOpen, len(g.cards))
	}
	c := g.cards[0]
	if c.n.Title != "Подключено" {
		t.Errorf("в стопке не самое свежее уведомление: %q", c.n.Title)
	}
	if c.stackRect.Empty() || c.stackText != "+1 уведомл." {
		t.Fatalf("кнопка стопки %v %q", c.stackRect, c.stackText)
	}
	if !c.stackRect.In(c.rect) {
		t.Errorf("кнопка %v выходит за карточку %v", c.stackRect, c.rect)
	}
	// Одиночное уведомление в стопку не сворачивается.
	if m := stackGroup(t, w, "mail"); m.stack || !m.cards[0].stackRect.Empty() {
		t.Errorf("одиночная карточка со стопкой: %+v", m.cards[0].stackRect)
	}
	ctx := w.drawn(w.nc)
	if !containsText(ctx.texts, "+1 уведомл.") {
		t.Errorf("кнопка стопки не нарисована: %+v", ctx.texts)
	}
	if containsText(ctx.texts, "Отключено") {
		t.Error("в свёрнутой стопке видна карточка из глубины")
	}
	// Счётчика в заголовке нет: его роль у кнопки.
	if containsText(ctx.texts, "2") {
		t.Error("в заголовке стопки остался счётчик группы")
	}
}

func TestW11Stack_ClickExpandsAndHeaderCollapses(t *testing.T) {
	testreset.Globals(t)
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	before := w.nc.OverlayBounds().Dy()
	z := w.zone(t, zoneKey{kind: zoneStack, app: "vpn"})
	w11Click(w.nc, z.rect)
	finishAnimations() // стопка раскрывается плавно (v3.34): ждём конца движения
	g := stackGroup(t, w, "vpn")
	if !g.stackOpen || len(g.cards) != 2 {
		t.Fatalf("после щелчка: open=%v карточек %d", g.stackOpen, len(g.cards))
	}
	for _, c := range g.cards {
		if !c.stackRect.Empty() {
			t.Error("у развёрнутой стопки осталась кнопка")
		}
	}
	if after := w.nc.OverlayBounds().Dy(); after <= before {
		t.Errorf("панель не выросла: %d → %d", before, after)
	}
	if !containsText(w.drawn(w.nc).texts, "Отключено") {
		t.Error("развёрнутая стопка не показала вторую карточку")
	}
	// Заголовок группы сворачивает обратно.
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneGroup, app: "vpn"}).rect)
	finishAnimations()
	if g := stackGroup(t, w, "vpn"); g.stackOpen || len(g.cards) != 1 {
		t.Errorf("заголовок не свернул стопку: open=%v карточек %d", g.stackOpen, len(g.cards))
	}
	// Шеврон заголовка есть только у развёрнутой стопки.
	l := w.nc.view.layout(w.nc.rect())
	if _, ok := l.find(zoneKey{kind: zoneGroupToggle, app: "vpn"}); ok {
		t.Error("у свёрнутой стопки есть шеврон")
	}
}

func TestW11Stack_CountFollowsNotifications(t *testing.T) {
	testreset.Globals(t)
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.ns.Add(Notification{AppID: "vpn", AppName: "VPN", Title: "Ещё", Timestamp: w11Time().Add(-80 * time.Hour)})
	w.open()
	if g := stackGroup(t, w, "vpn"); g.cards[0].stackText != "+2 уведомл." {
		t.Errorf("три уведомления: %q", g.cards[0].stackText)
	}
	// Закрыли два из трёх — остаётся одиночная карточка без стопки.
	for _, n := range w.ns.List() {
		if n.AppID == "vpn" && n.Title != "Подключено" {
			w.ns.Dismiss(n.ID)
		}
	}
	g := stackGroup(t, w, "vpn")
	if g.stack || len(g.cards) != 1 || !g.cards[0].stackRect.Empty() {
		t.Errorf("одно уведомление осталось стопкой: stack=%v", g.stack)
	}
}

// Закрытие стопки целиком — крестиком заголовка.
func TestW11Stack_GroupCloseDismissesAll(t *testing.T) {
	testreset.Globals(t)
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	h := w.zone(t, zoneKey{kind: zoneGroup, app: "vpn"}).rect
	x, y := pointIn(h)
	w.nc.OnMouseMove(x, y)
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneGroupClose, app: "vpn"}).rect)
	for _, n := range w.ns.List() {
		if n.AppID == "vpn" {
			t.Errorf("уведомление %q осталось после закрытия стопки", n.Title)
		}
	}
}

func TestW11Stack_Keyboard(t *testing.T) {
	testreset.Globals(t)
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	w.nc.SetFocused(true)
	for i := 0; i < 30; i++ {
		w.nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true})
		if w.nc.view.focus.kind == zoneStack {
			break
		}
	}
	if w.nc.view.focus.kind != zoneStack {
		t.Fatal("Tab не дошёл до кнопки стопки")
	}
	w.nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if g := stackGroup(t, w, "vpn"); !g.stackOpen {
		t.Fatal("Enter не развернул стопку")
	}
	if f := w.nc.view.focus; f.kind != zoneGroup || f.app != "vpn" {
		t.Errorf("фокус после раскрытия %+v, ждали заголовок группы", f)
	}
	w.nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyLeft, Pressed: true})
	if g := stackGroup(t, w, "vpn"); g.stackOpen {
		t.Error("← на заголовке не свернула стопку")
	}
	w.nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyRight, Pressed: true})
	if g := stackGroup(t, w, "vpn"); !g.stackOpen {
		t.Error("→ на заголовке не развернула стопку")
	}
}

// Строка «+N уведомл.» берёт форму по числу и языку.
func TestW11Stack_LabelPlural(t *testing.T) {
	testreset.Globals(t)
	for _, n := range []int{1, 2, 5, 11, 21, 100} {
		if got := stackLabel(n); got != "+"+strconv.Itoa(n)+" уведомл." {
			t.Errorf("RU %d: %q", n, got)
		}
	}
	useLanguage(t, "EN")
	for n, want := range map[int]string{1: "+1 notification", 2: "+2 notifications", 7: "+7 notifications"} {
		if got := stackLabel(n); got != want {
			t.Errorf("EN %d: %q, ждали %q", n, got, want)
		}
	}
	for _, k := range []string{StrNotifStackMore + ".one", StrNotifStackMore + ".few", StrNotifStackMore + ".many", StrNotifStackMore + ".other"} {
		if widget.TrIn("RU", k) == k {
			t.Errorf("нет русской формы %s", k)
		}
	}
}

// Прежний вид (флаг погашен): заголовок со счётчиком и все карточки, стопки нет.
func TestW11Stack_FlagOffKeepsGroup(t *testing.T) {
	testreset.Globals(t)
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.tm.SetFlag(theme.KeyNotifyStack, false)
	w.open()
	g := stackGroup(t, w, "vpn")
	if g.stack || len(g.cards) != 2 {
		t.Errorf("при погашенном флаге: stack=%v карточек %d", g.stack, len(g.cards))
	}
	for _, name := range []string{theme.ProfileWindows10} {
		o := newW11Fix(t, name, true)
		o.open()
		if og := stackGroup(t, o, "vpn"); og.stack {
			t.Errorf("%s: стопка в чужом презентере", name)
		}
	}
}

// ─── «Фокусировка» ───────────────────────────────────────────────────────────

func TestW11Focus_CompactLikeRealWindows11(t *testing.T) {
	testreset.Globals(t)
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	l := w.cal.w11Layout(w.cal.rect())
	if !l.ftitle.Empty() {
		t.Error("у компактного модуля есть заголовок")
	}
	if l.fless.Empty() || l.fmore.Empty() || l.faction.Empty() {
		t.Fatalf("нет «−», «+» или кнопки: %v %v %v", l.fless, l.fmore, l.faction)
	}
	// Одна строка: кнопки на одной высоте, «−» слева, «▶ Фокусировка» справа.
	if l.fless.Min.Y != l.faction.Min.Y || l.fless.Min.X >= l.faction.Min.X {
		t.Errorf("не одна строка: %v и %v", l.fless, l.faction)
	}
	if l.focus.Dy() != 60 {
		t.Errorf("высота модуля %d, ждали 60", l.focus.Dy())
	}
	ctx := w.drawn(w.cal)
	if !containsText(ctx.texts, tr(StrFocusTitle)) || containsText(ctx.texts, tr(StrFocusStart)) {
		t.Errorf("подпись кнопки: %+v", ctx.texts[len(ctx.texts)-3:])
	}
	// «+» двигает длительность; кнопка запускает сеанс и становится «Остановить».
	w11Click(w.cal, l.fmore)
	if d := w.fs.State().Duration; d != 35*time.Minute {
		t.Errorf("«+»: %v", d)
	}
	w11Click(w.cal, l.faction)
	if w.fs.Started != 1 || !w.fs.State().Running {
		t.Fatalf("кнопка не запустила сеанс: %+v", w.fs.State())
	}
	ctx = w.drawn(w.cal)
	if !containsText(ctx.texts, tr(StrFocusStop)) {
		t.Error("у идущего сеанса нет «Остановить»")
	}
	w11Click(w.cal, w.cal.w11Layout(w.cal.rect()).faction)
	if w.fs.Stopped != 1 {
		t.Error("«Остановить» не остановила сеанс")
	}
}

// Прежний вид — флагом: заголовок и «Начать», модуль выше.
func TestW11Focus_LegacyByFlag(t *testing.T) {
	testreset.Globals(t)
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	compact := w.cal.OverlayBounds().Dy()
	w.tm.SetFlag(theme.KeyCalendarFocusCompact, false)
	w.cal.Close()
	w.cal.Settle()
	w.open()
	l := w.cal.w11Layout(w.cal.rect())
	if l.ftitle.Empty() || w.cal.OverlayBounds().Dy() <= compact {
		t.Errorf("прежний вид без заголовка или не выше: %d и %d", w.cal.OverlayBounds().Dy(), compact)
	}
	ctx := w.drawn(w.cal)
	if !containsText(ctx.texts, tr(StrFocusTitle)) || !containsText(ctx.texts, tr(StrFocusStart)) {
		t.Errorf("прежний вид без «Фокусировка» и «Начать»")
	}
}

// Одна кнопка без выбора длительности.
func TestW11Focus_SingleButton(t *testing.T) {
	testreset.Globals(t)
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.cal.FocusSingleButton = true
	w.open()
	l := w.cal.w11Layout(w.cal.rect())
	if !l.fless.Empty() || !l.fmore.Empty() || !l.fvalue.Empty() {
		t.Errorf("при одной кнопке остались «−»/«+»: %v %v", l.fless, l.fmore)
	}
	if l.faction.Dx() < l.panel.Dx()-2*l.m.pad-1 {
		t.Errorf("кнопка не на всю ширину: %v", l.faction)
	}
	for _, s := range l.stops() {
		if s.id == calStopLess || s.id == calStopMore {
			t.Error("«−»/«+» в обходе клавиатуры")
		}
	}
	ctx := w.drawn(w.cal)
	if containsText(ctx.texts, "30 мин") {
		t.Error("длительность нарисована при одной кнопке")
	}
	w11Click(w.cal, l.faction)
	if !w.fs.State().Running || w.fs.State().Duration != 30*time.Minute {
		t.Errorf("кнопка запустила сеанс не на длительность по умолчанию: %+v", w.fs.State())
	}
	if !containsText(w.drawn(w.cal).texts, tr(StrFocusStop)) {
		t.Error("после запуска нет «Остановить»")
	}
}
