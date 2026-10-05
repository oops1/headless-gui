package desktop

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Трей Windows 10 по замерам референса (100 %, панель 40 px): шаг значков 24,
// «Пуск» 48 со значком 16, кнопка центра уведомлений 40, полоска «Показать
// рабочий стол» 5 px с линией 1 px слева, подсветка значков на всю полосу.

func TestWin10Geometry_MatchesMeasurements(t *testing.T) {
	tm := win10Fast(t)
	if got := tm.GetMetric(KeyTaskbarGap); got != 0 {
		t.Errorf("taskbar.gap = %v, ждали 0", got)
	}
	if got := tm.GetMetric(KeyStartButtonIconSize); got != 16 {
		t.Errorf("значок «Пуск» %v, ждали 16", got)
	}
	if got := NewStartButton(tm).PreferredSize(image.Pt(500, 40)).X; got != 48 {
		t.Errorf("ширина «Пуск» %d, ждали 48", got)
	}
	st := NewFakeSystemStatus()
	for name, w := range map[string]int{
		"сеть":    NewNetworkStatus(tm, st).PreferredSize(image.Pt(500, 40)).X,
		"звук":    NewVolumeStatus(tm, st).PreferredSize(image.Pt(500, 40)).X,
		"питание": NewPowerStatus(tm, st).PreferredSize(image.Pt(500, 40)).X,
		"значок":  NewTrayIcon(tm).PreferredSize(image.Pt(500, 40)).X,
	} {
		if w != 24 {
			t.Errorf("шаг значка трея (%s) %d, ждали 24", name, w)
		}
	}
	if got := NewNotificationButton(tm, nil).PreferredSize(image.Pt(500, 40)).X; got != 40 {
		t.Errorf("кнопка центра уведомлений %d, ждали 40", got)
	}
	if got := NewShowDesktopButton(tm).PreferredSize(image.Pt(500, 40)); got != image.Pt(5, 40) {
		t.Errorf("полоска «Показать рабочий стол» %v, ждали 5×40", got)
	}
}

// ширина полоски и линии слева: линия рисуется цветом части "line".
func TestWin10ShowDesktop_DrawsLeftLine(t *testing.T) {
	tm := win10Fast(t)
	b := NewShowDesktopButton(tm)
	b.SetBounds(image.Rect(100, 0, 105, 40))
	ctx := &recCtx{}
	b.Draw(ctx)
	var line *recFill
	for i := range ctx.fills {
		if f := ctx.fills[i]; f.w == 1 && f.h == 40 && f.x == 100 {
			line = &ctx.fills[i]
		}
	}
	if line == nil {
		t.Fatalf("линия 1×40 слева не нарисована: %+v", ctx.fills)
	}
	if line.col.A == 0 {
		t.Error("линия прозрачна")
	}
	// Другие профили линии не рисуют.
	ctx = &recCtx{}
	nb := NewShowDesktopButton(managerFor(t, theme.ProfileWindows11))
	nb.SetBounds(image.Rect(100, 0, 104, 40))
	nb.Draw(ctx)
	for _, f := range ctx.fills {
		if f.w == 1 && f.h == 40 {
			t.Errorf("у Windows 11 появилась линия слева: %+v", f)
		}
	}
}

// trayStripScene — панель с треем (сеть, звук) и кнопкой уведомлений.
func trayStripScene(tm *theme.Manager) (*Taskbar, *NetworkItem, *NotificationButton) {
	bar := NewTaskbar(tm)
	tray := NewSystemTray(tm)
	st := NewFakeSystemStatus()
	net := NewNetworkStatus(tm, st)
	tray.AddItem(net)
	tray.AddItem(NewVolumeStatus(tm, st))
	bar.AddItem(SlotTray, tray)
	nb := NewNotificationButton(tm, nil)
	bar.AddItem(SlotTray, nb)
	bar.SetBounds(image.Rect(0, 0, 400, tmHeight(tm)))
	return bar, net, nb
}

func tmHeight(tm *theme.Manager) int { return int(tm.GetMetric(KeyTaskbarHeight)) }

// hoverFills — заливки, которыми значок рисуется под курсором.
func hoverFills(it Item) []recFill {
	b := it.Bounds()
	it.(interface{ OnMouseMove(x, y int) }).OnMouseMove(b.Min.X+2, b.Min.Y+2)
	ctx := &recCtx{}
	it.Draw(ctx)
	return ctx.fills
}

// Подсветка наведения значков трея Windows 10 занимает всю высоту полосы (40),
// а не квадрат значка 24×16; сам значок остаётся по центру. У остальных тем
// значок подсвечивается своим квадратом.
func TestWin10Tray_HoverFillsWholeStrip(t *testing.T) {
	tm := win10Fast(t)
	_, net, nb := trayStripScene(tm)
	if b := net.Bounds(); b.Dy() != 40 || b.Dx() != 24 {
		t.Fatalf("границы значка сети %v, ждали 24×40", b)
	}
	if b := nb.Bounds(); b.Dy() != 40 || b.Dx() != 40 {
		t.Fatalf("границы кнопки уведомлений %v, ждали 40×40", b)
	}
	for name, it := range map[string]Item{"сеть": net, "уведомления": nb} {
		full := false
		for _, f := range hoverFills(it) {
			if f.h == 40 && f.w == it.Bounds().Dx() {
				full = true
			}
		}
		if !full {
			t.Errorf("%s: подсветка наведения не на всю высоту полосы", name)
		}
	}

	// Значок (фигуры) стоит по центру полосы, а не растянут на неё.
	net.OnMouseMove(-10, -10) // курсор ушёл: подложки наведения нет
	ctx := &recCtx{}
	net.Draw(ctx)
	for _, f := range ctx.fills {
		if f.h == 40 {
			continue // подложка полосы (угасающее наведение)
		}
		if f.h > 16 {
			t.Errorf("фигура значка выше значка 16: %+v", f)
		}
		if f.y < 12 || f.y+f.h > 28 {
			t.Errorf("фигура значка вне центра полосы (12..28): %+v", f)
		}
	}

	// Windows 11: плашка не во всю полосу, а своей высоты из темы (tray.item.height
	// — 40 в панели 48); сам значок остаётся своего размера по центру плашки.
	tm11 := managerFor(t, theme.ProfileWindows11)
	_, net11, _ := trayStripScene(tm11)
	if got, want := net11.Bounds().Dy(), int(tm11.GetMetric(KeyTrayItemHeight)); got != want || got >= tmHeight(tm11) {
		t.Errorf("Windows 11: высота плашки %d, ждали %d (меньше полосы %d)", got, want, tmHeight(tm11))
	}
	// Тема без tray.item.height и без заливки полосы (macOS): значок своего размера.
	tmMac := managerFor(t, theme.ProfileMacOS)
	_, netMac, _ := trayStripScene(tmMac)
	if got, want := netMac.Bounds().Dy(), int(tmMac.GetMetric(KeyTrayIconSize)); got != want {
		t.Errorf("macOS: высота значка %d, ждали %d (без растяжки)", got, want)
	}
}

// Условные стили трея наследуются: на светлой панели значок, кнопка уведомлений
// и полоска не остаются белыми.
func TestWin10Tray_LightStylesInherited(t *testing.T) {
	tm := win10Fast(t)
	tm.SetFlag(theme.KeyTaskbarLight, true)
	vol := tm.GetStyle(ComponentVolume, "", theme.StateNormal)
	for _, c := range []string{ComponentTrayIcon, ComponentTrayNotifications, ComponentTrayShowDesktop} {
		got := tm.GetStyle(c, "", theme.StateNormal)
		if got.Text != vol.Text {
			t.Errorf("%s на светлой панели: текст %v, у звука %v", c, got.Text, vol.Text)
		}
		if h, vh := tm.GetStyle(c, "", theme.StateHover), tm.GetStyle(ComponentVolume, "", theme.StateHover); h.Fill != vh.Fill {
			t.Errorf("%s на светлой панели: наведение %v, у звука %v", c, h.Fill, vh.Fill)
		}
	}
	tm.SetFlag(theme.KeyTaskbarLight, false)
	dark := tm.GetStyle(ComponentTrayIcon, "", theme.StateNormal).Text
	if dark == vol.Text {
		t.Error("светлый и тёмный текст значка совпали — флаг ничего не меняет")
	}
}

// Кнопка центра уведомлений горит, пока центр открыт.
func TestNotificationButton_ActiveWhileCenterOpen(t *testing.T) {
	tm := win10Fast(t)
	b := NewNotificationButton(tm, nil)
	b.SetBounds(image.Rect(0, 0, 40, 40))
	idle := &recCtx{}
	b.Draw(idle)

	f := NewFlyout(tm, "notificationcenter")
	f.Size = func() image.Point { return image.Pt(300, 300) }
	f.Screen = image.Rect(0, 0, 800, 600)
	untrack := b.Track(f)
	defer untrack()

	f.Open(image.Rect(760, 560, 800, 600))
	if !b.Active() {
		t.Fatal("центр открыт, а кнопка не горит")
	}
	lit := &recCtx{}
	b.Draw(lit)
	if len(lit.fills) <= len(idle.fills) {
		t.Errorf("горящая кнопка не получила подложки: %d → %d заливок", len(idle.fills), len(lit.fills))
	}
	f.Close()
	if b.Active() {
		t.Error("центр закрыт, а кнопка горит")
	}
	// Без источника — Track не падает.
	b.Track(nil)()
}

// Формы множественного числа счётчика.
func TestPluralForm(t *testing.T) {
	ru := map[int]string{
		0: PluralMany, 1: PluralOne, 2: PluralFew, 4: PluralFew, 5: PluralMany,
		11: PluralMany, 12: PluralMany, 14: PluralMany, 21: PluralOne, 22: PluralFew,
		25: PluralMany, 101: PluralOne, 111: PluralMany,
	}
	for n, want := range ru {
		if got := PluralForm("ru", n); got != want {
			t.Errorf("RU %d: %s, ждали %s", n, got, want)
		}
	}
	for n, want := range map[int]string{0: PluralOther, 1: PluralOne, 2: PluralOther, 21: PluralOther} {
		if got := PluralForm("EN", n); got != want {
			t.Errorf("EN %d: %s, ждали %s", n, got, want)
		}
	}
}

func TestNotificationButton_PluralTooltip(t *testing.T) {
	prev := widget.Language()
	defer widget.SetLanguage(prev)
	b := NewNotificationButton(nil, nil)

	widget.SetLanguage("RU")
	want := map[int]string{
		1: "1 новое уведомление", 2: "2 новых уведомления", 5: "5 новых уведомлений",
		21: "21 новое уведомление", 11: "11 новых уведомлений",
	}
	for n, w := range want {
		b.SetCount(n)
		if got := b.GetToolTip(); got != w {
			t.Errorf("RU %d: %q, ждали %q", n, got, w)
		}
	}
	widget.SetLanguage("EN")
	for n, w := range map[int]string{1: "1 new notification", 3: "3 new notifications"} {
		b.SetCount(n)
		if got := b.GetToolTip(); got != w {
			t.Errorf("EN %d: %q, ждали %q", n, got, w)
		}
	}
}

// Новые ключи desktop.tray.* и прежние как алиасы: приложение, переопределившее
// прежний ключ, получает свою строку; новый ключ побеждает прежний.
func TestTrayStrings_LegacyKeysStayAliases(t *testing.T) {
	prev := widget.Language()
	defer widget.SetLanguage(prev)
	widget.SetLanguage("EN")
	b := NewShowDesktopButton(nil)
	nb := NewNotificationButton(nil, nil)

	if got := b.GetToolTip(); got != "Show desktop" {
		t.Errorf("встроенная строка: %q", got)
	}
	// Прежние ключи по-прежнему зарегистрированы встроенными переводами.
	if got := widget.TrIn("EN", StrShowDesktop); got != "Show desktop" {
		t.Errorf("прежний ключ потерян: %q", got)
	}

	widget.RegisterString("EN", StrShowDesktop, "Peek at desktop")
	defer widget.RegisterString("EN", StrShowDesktop, "Show desktop")
	if got := b.GetToolTip(); got != "Peek at desktop" {
		t.Errorf("переопределение прежнего ключа не учтено: %q", got)
	}
	widget.RegisterString("EN", StrTrayShowDesktop, "Desktop")
	defer widget.RegisterString("EN", StrTrayShowDesktop, "Show desktop")
	if got := b.GetToolTip(); got != "Desktop" {
		t.Errorf("новый ключ должен побеждать прежний: %q", got)
	}

	// Счётчик: прежний ключ с одним шаблоном уважается, если приложение его задало.
	widget.RegisterString("EN", StrNewNotifications, "Unread: %d")
	defer widget.RegisterString("EN", StrNewNotifications, "New notifications: %d")
	nb.SetCount(3)
	if got := nb.GetToolTip(); got != "Unread: 3" {
		t.Errorf("переопределение прежнего счётчика не учтено: %q", got)
	}
}

// Подсказки берутся тем же tr() с языком по умолчанию, что у остальных
// компонентов: до явного SetLanguage языки не расходятся.
func TestTrayStrings_SameLanguageAsDesktop(t *testing.T) {
	b := NewNotificationButton(nil, nil)
	if got, want := b.GetToolTip(), tr(StrTrayNoNewNotifications); got != want {
		t.Errorf("подсказка кнопки %q, а tr() даёт %q", got, want)
	}
	if got, want := NewShowDesktopButton(nil).GetToolTip(), tr(StrTrayShowDesktop); got != want {
		t.Errorf("подсказка полоски %q, а tr() даёт %q", got, want)
	}
}
