package desktop

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Локализация компонентов рабочего стола: строки пакета были русскими
// литералами («Пуск», «Закреплено», «Очистить все», месяцы календаря). Теперь
// это ключи widget.Tr с русским и английским переводом, а язык берётся при
// отрисовке — смена widget.SetLanguage видна без пересоздания компонентов.

// useLanguage выставляет язык интерфейса на время теста и возвращает русский,
// с которого рабочий стол и начинает (DefaultLanguage).
func useLanguage(t *testing.T, code string) {
	t.Helper()
	widget.SetLanguage(code)
	t.Cleanup(func() { widget.SetLanguage("RU") })
}

// Приложение, не выбравшее язык, видит рабочий стол русским, как и раньше.
// Тест обязан идти до всех, кто зовёт SetLanguage, — после первого вызова
// язык считается выбранным.
func TestLocale_DefaultIsRussianUntilAppChoosesLanguage(t *testing.T) {
	if widget.LanguageExplicit() {
		t.Skip("язык уже выбран другим тестом пакета")
	}
	if got := uiLanguage(); got != "RU" {
		t.Fatalf("язык по умолчанию %q, ждал RU", got)
	}
	if got := tr(StrStart); got != "Пуск" {
		t.Errorf("кнопка без выбранного языка: %q, ждал «Пуск»", got)
	}
}

func TestLocale_EveryKeyHasBothTranslations(t *testing.T) {
	keys := []string{
		StrStart, StrStartPinned, StrStartAllApps, StrNotifEmpty, StrNotifClearAll,
		StrNetNone, StrNetConnected, StrNetNamed, StrSoundMuted, StrSoundLevel,
		StrPowerAC, StrPowerBattery, StrClockTimeFormat, StrClockDateFormat, StrCalLongDate,
	}
	for m := 1; m <= 12; m++ {
		keys = append(keys, strCalMonthGenPref+widgetItoa(m))
	}
	for _, lang := range []string{"RU", "EN"} {
		for _, k := range keys {
			if v, ok := widget.Translation(lang, k); !ok || v == "" {
				t.Errorf("нет перевода %s для ключа %s", lang, k)
			}
		}
	}
	// Русский и английский различаются там, где это надписи, а не форматы.
	for _, k := range []string{StrStart, StrStartPinned, StrStartAllApps, StrNotifEmpty, StrNotifClearAll} {
		ru, _ := widget.Translation("RU", k)
		en, _ := widget.Translation("EN", k)
		if ru == en {
			t.Errorf("ключ %s: русский и английский совпадают (%q)", k, ru)
		}
	}
}

func widgetItoa(n int) string {
	if n >= 10 {
		return string(rune('0'+n/10)) + string(rune('0'+n%10))
	}
	return string(rune('0' + n))
}

func TestLocale_StartButtonLabelFollowsLanguage(t *testing.T) {
	tm := buildTestTheme()
	sb := NewStartButton(tm)
	sb.SetBounds(image.Rect(0, 0, 120, 32))

	draw := func() []itemRecText {
		ctx := newItemRecCtx()
		sb.Draw(ctx)
		return ctx.texts
	}
	has := func(texts []itemRecText, s string) bool {
		for _, x := range texts {
			if x.text == s {
				return true
			}
		}
		return false
	}

	useLanguage(t, "RU")
	if !has(draw(), "Пуск") {
		t.Errorf("по-русски нет подписи «Пуск»: %+v", draw())
	}
	// Тот же экземпляр кнопки, без пересоздания.
	widget.SetLanguage("EN")
	got := draw()
	if !has(got, "Start") || has(got, "Пуск") {
		t.Errorf("после SetLanguage(EN) подпись %+v, ждал «Start»", got)
	}
}

func TestLocale_StartMenuSectionsFollowLanguage(t *testing.T) {
	m, cat := startMenuFixture(t)
	defer m.Close()
	cat.Pin("term")

	labels := func() (pinned, all bool, ru bool) {
		for _, r := range m.buildRows() {
			if r.kind != startMenuRowSection {
				continue
			}
			switch r.label {
			case "Pinned", "All apps":
				if r.label == "Pinned" {
					pinned = true
				} else {
					all = true
				}
			case "Закреплено", "Все приложения":
				ru = true
			}
		}
		return
	}

	useLanguage(t, "RU")
	if _, _, ru := labels(); !ru {
		t.Error("по-русски разделы меню не «Закреплено»/«Все приложения»")
	}
	widget.SetLanguage("EN")
	pinned, all, ru := labels()
	if !pinned || !all || ru {
		t.Errorf("по-английски разделы: Pinned=%v All apps=%v осталось русское=%v", pinned, all, ru)
	}
}

func TestLocale_NotificationCenterFollowsLanguage(t *testing.T) {
	// Пустой центр.
	empty := NewNotificationCenter(flatNotifTheme(t), NewFakeNotifications())
	empty.Screen = panelScreen()
	empty.Open(panelAnchor())
	defer empty.Close()

	useLanguage(t, "RU")
	if got := empty.EmptyLabel(); got != "Новых уведомлений нет" {
		t.Errorf("подпись пустого центра %q", got)
	}
	widget.SetLanguage("EN")
	if got := empty.EmptyLabel(); got != "No new notifications" {
		t.Errorf("по-английски подпись пустого центра %q", got)
	}
	empty.EmptyText = "своя подпись"
	if got := empty.EmptyLabel(); got != "своя подпись" {
		t.Errorf("заданный EmptyText не побеждает: %q", got)
	}

	// Кнопка «Очистить все» при нескольких уведомлениях.
	nc, _ := notifFixture(t)
	defer nc.Close()
	ctx := &recCtx{}
	nc.DrawOverlay(ctx)
	if !containsText(ctx.texts, "Clear all") || containsText(ctx.texts, "Очистить все") {
		t.Errorf("по-английски кнопка очистки нарисована как %+v", ctx.texts)
	}
	widget.SetLanguage("RU")
	ctx = &recCtx{}
	nc.DrawOverlay(ctx)
	if !containsText(ctx.texts, "Очистить все") {
		t.Errorf("по-русски кнопка очистки нарисована как %+v", ctx.texts)
	}
}

func TestLocale_NotificationTimeFollowsCulture(t *testing.T) {
	nc, _ := notifFixture(t)
	defer nc.Close()

	useLanguage(t, "EN")
	ctx := &recCtx{}
	nc.DrawOverlay(ctx)
	if !containsText(ctx.texts, "3:09 PM") {
		t.Errorf("по-английски время карточки не «3:09 PM»: %+v", ctx.texts)
	}
	widget.SetLanguage("RU")
	ctx = &recCtx{}
	nc.DrawOverlay(ctx)
	if !containsText(ctx.texts, "15:09") {
		t.Errorf("по-русски время карточки не «15:09»: %+v", ctx.texts)
	}
}

// ─── Календарь ───────────────────────────────────────────────────────────────

func calendarTexts(c *CalendarFlyout) []string {
	ctx := &recCtx{}
	c.DrawOverlay(ctx)
	out := make([]string, 0, len(ctx.texts))
	for _, tx := range ctx.texts {
		out = append(out, tx.text)
	}
	return out
}

func TestLocale_CalendarRussianByDefaultLooksAsBefore(t *testing.T) {
	useLanguage(t, "RU")
	c, _ := newTestCalendar(t, time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC))
	defer c.Close()

	texts := strings.Join(calendarTexts(c), "|")
	if !strings.Contains(texts, "Август 2026") {
		t.Errorf("нет заголовка «Август 2026»: %s", texts)
	}
	if !strings.Contains(texts, "Пн|Вт|Ср|Чт|Пт|Сб|Вс") {
		t.Errorf("шапка не «Пн…Вс»: %s", texts)
	}
	if got := c.dateTitle(); got != "27 августа 2026" {
		t.Errorf("дата свёрнутой панели %q", got)
	}
	// Первый столбец сетки августа 2026 — понедельник 27 июля.
	grid := c.gridSnapshot()
	if d := grid[0][0].date; d.Weekday() != time.Monday || d.Day() != 27 {
		t.Errorf("первая ячейка %v, ждал понедельник 27 июля", d)
	}
}

func TestLocale_CalendarEnglishFollowsLanguageWithoutRecreate(t *testing.T) {
	useLanguage(t, "RU")
	c, _ := newTestCalendar(t, time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC))
	defer c.Close()

	widget.SetLanguage("EN")
	texts := strings.Join(calendarTexts(c), "|")
	if !strings.Contains(texts, "August 2026") {
		t.Errorf("нет заголовка «August 2026»: %s", texts)
	}
	if !strings.Contains(texts, "Su|Mo|Tu|We|Th|Fr|Sa") {
		t.Errorf("неделя не начинается с воскресенья: %s", texts)
	}
	if got := c.dateTitle(); got != "August 27, 2026" {
		t.Errorf("дата свёрнутой панели %q", got)
	}
	// Первый столбец сетки августа 2026 — воскресенье 26 июля.
	grid := c.gridSnapshot()
	if d := grid[0][0].date; d.Weekday() != time.Sunday || d.Day() != 26 {
		t.Errorf("первая ячейка %v, ждал воскресенье 26 июля", d)
	}
	// Клик по ячейке выбирает ту дату, что нарисована в ней.
	layout := c.computeLayout(c.contentRect())
	cell := layout.days[0][0]
	x, y := pointIn(cell.rect)
	c.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	if got := c.Selected(); !sameDay(got, cell.day.date) {
		t.Errorf("клик выбрал %v, ждал %v", got, cell.day.date)
	}
}

// ruKZ — культура потребителя: понедельник первый, свои названия и формат
// «2006-01-02». Встраивает LocaleCulture и переопределяет только своё.
type ruKZ struct{ LocaleCulture }

func (ruKZ) MonthName(m time.Month) string     { return "М" + string(rune('0'+int(m)%10)) }
func (ruKZ) MonthGenitive(m time.Month) string { return "м" + string(rune('0'+int(m)%10)) }
func (ruKZ) WeekdayShort(d time.Weekday) string {
	return []string{"в", "п", "в", "с", "ч", "п", "с"}[d] + string(rune('0'+int(d)))
}
func (ruKZ) FirstWeekday() time.Weekday { return time.Saturday }
func (ruKZ) TimeFormat() string         { return "15.04" }
func (ruKZ) DateFormat() string         { return "2006-01-02" }
func (c ruKZ) LongDate(t time.Time) string {
	return expandLongDate("{y}/{M}/{d}", t, c.MonthGenitive(t.Month()))
}

func TestLocale_CalendarUsesConsumerCulture(t *testing.T) {
	useLanguage(t, "EN") // язык не должен влиять, когда культуру дал потребитель
	c, _ := newTestCalendar(t, time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC))
	defer c.Close()
	c.Culture = ruKZ{}

	texts := strings.Join(calendarTexts(c), "|")
	if !strings.Contains(texts, "М8 2026") {
		t.Errorf("заголовок не из культуры потребителя: %s", texts)
	}
	// Неделя начинается с субботы: с (6), в (0), п (1)…
	if !strings.Contains(texts, "с6|в0|п1|в2|с3|ч4|п5") {
		t.Errorf("шапка не от субботы: %s", texts)
	}
	if got := c.dateTitle(); got != "2026/м8/27" {
		t.Errorf("дата свёрнутой панели %q", got)
	}
	if d := c.gridSnapshot()[0][0].date; d.Weekday() != time.Saturday {
		t.Errorf("первый столбец сетки %v, ждал субботу", d.Weekday())
	}
}

func TestLocale_CultureFallsBackWhenLanguageHasNoTables(t *testing.T) {
	useLanguage(t, "DE") // перевода нет: запасной английский, а не голые ключи
	var cul LocaleCulture
	if got := cul.MonthName(time.March); got != "March" {
		t.Errorf("месяц для языка без таблиц: %q", got)
	}
	if got := cul.MonthGenitive(time.March); got != "March" {
		t.Errorf("месяц в родительном: %q", got)
	}
	if got := cul.FirstWeekday(); got != time.Sunday {
		t.Errorf("первый день недели: %v", got)
	}
	if got := cul.TimeFormat(); got != "3:04 PM" {
		t.Errorf("формат времени: %q", got)
	}
}

// ─── Часы ────────────────────────────────────────────────────────────────────

func TestLocale_ClockFormatFollowsLanguageAndOverrides(t *testing.T) {
	tm := testThemeManager(t)
	clk := NewFakeClock(time.Date(2026, 8, 27, 14, 5, 0, 0, time.UTC))
	c := NewClock(tm, clk)
	defer c.Close()
	c.SetBounds(image.Rect(0, 0, 200, 44))

	render := func() []recText {
		ctx := &recCtx{}
		c.Draw(ctx)
		return ctx.texts
	}

	useLanguage(t, "RU")
	if got := render(); !containsText(got, "14:05") || !containsText(got, "27.08.2026") {
		t.Errorf("по-русски часы показывают %+v", got)
	}
	widget.SetLanguage("EN")
	if got := render(); !containsText(got, "2:05 PM") || !containsText(got, "8/27/2026") {
		t.Errorf("по-английски часы показывают %+v", got)
	}

	// Формат потребителя из культуры.
	c.Culture = ruKZ{}
	if got := render(); !containsText(got, "14.05") || !containsText(got, "2026-08-27") {
		t.Errorf("культура потребителя: %+v", got)
	}
	// Явные поля побеждают культуру.
	c.TimeFormat = "04:05"
	if got := render(); !containsText(got, "05:00") {
		t.Errorf("явный TimeFormat не победил: %+v", got)
	}
}

// Ширина часов зависит от формата, а значит и от языка: панель обязана
// переложить элементы сама, иначе новая строка не поместится в старую ширину.
func TestLocale_TaskbarRelayoutsOnLanguageChange(t *testing.T) {
	useLanguage(t, "RU")
	p := theme.NewProfile("TaskbarLoc")
	p.SetMetric(KeyTaskbarHeight, 40)
	tm2 := theme.NewManager()
	p.SetStyle(ComponentClock, "", theme.StateNormal, theme.StyleDelta{
		PadX: theme.N(4), Font: &theme.FontSpec{Size: 10},
	})
	if err := tm2.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := tm2.SetTheme("TaskbarLoc"); err != nil {
		t.Fatal(err)
	}

	clk := NewFakeClock(time.Date(2026, 8, 27, 14, 5, 0, 0, time.UTC))
	clock := NewClock(tm2, clk)
	tb := NewTaskbar(tm2)
	tb.AddItem(SlotTray, clock)
	tb.SetBounds(image.Rect(0, 0, 800, 40))
	defer tb.Close()
	defer clock.Close()

	ruW := clock.Bounds().Dx()
	if ruW == 0 {
		t.Fatal("часы не получили места")
	}
	widget.SetLanguage("EN") // «8/27/2026» уже «27.08.2026»
	enW := clock.Bounds().Dx()
	if enW == ruW {
		t.Errorf("после смены языка ширина часов осталась %d: панель не переложила элементы", enW)
	}
	if want := clock.PreferredSize(image.Pt(800, 40)).X; enW != want {
		t.Errorf("ширина часов %d, желаемая %d", enW, want)
	}
}

// ─── Подсказки трея ──────────────────────────────────────────────────────────

func TestLocale_TrayTooltipsFollowLanguageWithoutStateChange(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus() // сеть Wi-Fi «Сеть», звук, питание от сети
	net := NewNetworkStatus(tm, st)
	vol := NewVolumeStatus(tm, st)
	pw := NewPowerStatus(tm, st)
	defer net.Close()
	defer vol.Close()
	defer pw.Close()

	useLanguage(t, "RU")
	if got := pw.GetToolTip(); got != "Питание от сети" {
		t.Fatalf("по-русски подсказка питания %q", got)
	}
	widget.SetLanguage("EN")
	if got := pw.GetToolTip(); got != "Plugged in" {
		t.Errorf("по-английски подсказка питания %q", got)
	}
	if got := net.GetToolTip(); got != "Network: Сеть, connected" {
		t.Errorf("по-английски подсказка сети %q", got)
	}
	st.SetVolume(VolState{Level: 0.4})
	if got := vol.GetToolTip(); got != "Sound: 40%" {
		t.Errorf("по-английски подсказка звука %q", got)
	}
	st.SetVolume(VolState{Muted: true})
	if got := vol.GetToolTip(); got != "Sound: muted" {
		t.Errorf("по-английски подсказка приглушённого звука %q", got)
	}

	// Своя подсказка пользователя языку не подчиняется.
	pw.SetToolTip("мой текст")
	widget.SetLanguage("RU")
	if got := pw.GetToolTip(); got != "мой текст" {
		t.Errorf("явный SetToolTip перезаписан сменой языка: %q", got)
	}
}

// Псевдонимы ключей (widget.AliasStrings) подхватываются: приложение связывает
// «desktop.start» со своим «Start» и получает свой текст.
func TestLocale_AppAliasWins(t *testing.T) {
	useLanguage(t, "RU")
	widget.RegisterString("RU", "app.Start", "ПУСК")
	widget.AliasString(StrStart, "app.Start")
	t.Cleanup(widget.ClearStringAliases)
	if got := tr(StrStart); got != "ПУСК" {
		t.Errorf("псевдоним не подхвачен: %q", got)
	}
}
