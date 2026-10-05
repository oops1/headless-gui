package theme

import "testing"

// Центр уведомлений и календарь Windows 11 — профиль: презентеры, метрики,
// части стилей, токены светлой и тёмной разновидности.

func TestWin11NotifyPresentersOnlyForWindows11(t *testing.T) {
	for _, name := range []string{ProfileWindows11, ProfileWindows11Dark} {
		a := notifyManager(t, name).Active()
		if got := a.PresenterName("notificationcenter"); got != NotificationCenterWin11Presenter {
			t.Errorf("%s: презентер центра %q", name, got)
		}
		if got := a.PresenterName("calendar"); got != CalendarWin11Presenter {
			t.Errorf("%s: презентер календаря %q", name, got)
		}
	}
	for _, name := range []string{ProfileWindows10, ProfileWindows10Dark, ProfileWindows2000, ProfileMacOS} {
		if got := notifyManager(t, name).Active().PresenterName("calendar"); got != "" {
			t.Errorf("%s: у темы презентер календаря %q", name, got)
		}
	}
}

// Метрики по замерам Windows 11 при 100 %: ширина 364 у центра и календаря,
// скругление карточек 4, панелей 8.
func TestWin11NotifyGeometry(t *testing.T) {
	m := notifyManager(t, ProfileWindows11)
	want := map[Key]float64{
		"notificationcenter.width":       364,
		"notificationcenter.toast.width": 364,
		"calendar.w11.width":             364,
		"notificationcenter.stack.gap":   8,
		"notificationcenter.edge":        12,
		"calendar.w11.edge":              12,
	}
	for k, v := range want {
		if got := m.GetMetric(k); got != v {
			t.Errorf("%s = %v, ждали %v", k, got, v)
		}
	}
	for _, k := range []Key{"calendar.w11.pad", "calendar.w11.head", "calendar.w11.cell", "calendar.w11.day",
		"calendar.w11.nav", "calendar.w11.focus", "calendar.w11.button", "notificationcenter.header.button",
		"notificationcenter.card.pad", "notificationcenter.card.slot"} {
		if m.GetMetric(k) <= 0 {
			t.Errorf("метрика %s не объявлена", k)
		}
	}
	if c := m.GetStyle("notificationcenter", "card", StateNormal).Corner; c != 4 {
		t.Errorf("скругление карточки %v, ждали 4", c)
	}
	if c := m.GetStyle("notificationcenter", "", StateNormal).Corner; c != 8 {
		t.Errorf("скругление панели центра %v, ждали 8", c)
	}
	if c := m.GetStyle("calendar", "", StateNormal).Corner; c != 8 {
		t.Errorf("скругление панели календаря %v, ждали 8", c)
	}
	if a := m.GetAnimation("notification.expand"); a.Duration <= 0 || a.Curve == "out-back" {
		t.Errorf("анимация раскрытия карточки %+v", a)
	}
}

// Части объявлены в обоих режимах; тёмный профиль меняет токены, а не стили, и
// остаётся коротким.
func TestWin11NotifyPartsAndDarkTokens(t *testing.T) {
	parts := map[string][]string{
		"notificationcenter": {"card", "action", "field", "glyph", "dim", "group", "pill", "headbtn", "headbtn.on", "link", "scrollbar", "toast"},
		"calendar":           {"date", "month", "nav", "divider", "focus.button", "focus.accent", "progress", "progress.fill"},
	}
	for _, name := range []string{ProfileWindows11, ProfileWindows11Dark} {
		m := notifyManager(t, name)
		for comp, list := range parts {
			for _, part := range list {
				s := m.GetStyle(comp, part, StateNormal)
				if s.Fill.A == 0 && s.Text.A == 0 && s.Border.A == 0 {
					t.Errorf("%s: часть %s.%s пуста", name, comp, part)
				}
			}
		}
	}
	light, dark := notifyManager(t, ProfileWindows11), notifyManager(t, ProfileWindows11Dark)
	lc, dc := light.GetStyle("notificationcenter", "card", StateNormal), dark.GetStyle("notificationcenter", "card", StateNormal)
	if lc.Fill == dc.Fill {
		t.Error("карточка светлой и тёмной темы залита одним цветом")
	}
	if lc.Fill.A == 0 || dc.Fill.A == 0 {
		t.Error("заливка карточки пуста")
	}
	if light.GetStyle("notificationcenter", "dim", StateNormal).Text == dark.GetStyle("notificationcenter", "dim", StateNormal).Text {
		t.Error("приглушённый текст не следует за режимом")
	}
	if n := Windows11DarkProfile().TokenCount(); n > 15 {
		t.Errorf("тёмный Windows 11 объявляет %d токенов, предел 15", n)
	}
}

// Включённое состояние и кнопка «Начать» следуют за акцентом.
func TestWin11NotifyFollowsAccent(t *testing.T) {
	m := notifyManager(t, ProfileWindows11)
	m.SetAccent(RGB(200, 30, 30))
	if got := m.GetStyle("calendar", "focus.accent", StateNormal).Fill; got != RGB(200, 30, 30) {
		t.Errorf("«Начать» %v не последовала за акцентом", got)
	}
	if got := m.GetStyle("calendar", "progress.fill", StateNormal).Fill; got != RGB(200, 30, 30) {
		t.Errorf("полоса прогресса %v не последовала за акцентом", got)
	}
	if got := m.GetStyle("notificationcenter", "headbtn.on", StateNormal).Text; got != RGB(200, 30, 30) {
		t.Errorf("колокольчик «Не беспокоить» %v не последовал за акцентом", got)
	}
	if got := m.GetStyle("calendar", "day", StateActive).Fill; got != RGB(200, 30, 30) {
		t.Errorf("сегодня %v не последовал за акцентом", got)
	}
}

// Число — круг без тени и рамки панели; рамка есть у выбранного дня.
func TestWin11CalendarDayIsFlatCircle(t *testing.T) {
	m := notifyManager(t, ProfileWindows11)
	for _, st := range []State{StateNormal, StateHover, StateActive} {
		s := m.GetStyle("calendar", "day", st)
		if s.Corner != 18 || s.Elevation != 0 || s.Shadow.A != 0 || s.BorderWidth != 0 {
			t.Errorf("день[%v]: скругление %v, подъём %v, тень %v, рамка %v", st, s.Corner, s.Elevation, s.Shadow, s.BorderWidth)
		}
	}
	if s := m.GetStyle("calendar", "day", StateFocused); s.BorderWidth == 0 {
		t.Error("у выбранного дня нет рамки")
	}
}

// Материал Mica ложится на панель центра и календаря, но не на карточку.
func TestWin11NotifyMaterialOnPanelsNotOnCards(t *testing.T) {
	m := notifyManager(t, ProfileWindows11)
	m.SetFlag(FlagBackdropMica, true)
	if got := m.GetStyle("notificationcenter", "", StateNormal).Backdrop.Material; got != MaterialMica {
		t.Errorf("панель центра: материал %v", got)
	}
	if got := m.GetStyle("calendar", "", StateNormal).Backdrop.Material; got != MaterialMica {
		t.Errorf("панель календаря: материал %v", got)
	}
	for _, part := range []string{"card", "group", "action", "toast"} {
		if got := m.GetStyle("notificationcenter", part, StateNormal).Backdrop.Material; got != MaterialDefault {
			t.Errorf("часть %s получила материал %v", part, got)
		}
	}
	m.SetFlag(FlagShadowSoft, true)
	if s := m.GetStyle("notificationcenter", "toast", StateNormal); s.ShadowBlur <= 0 {
		t.Error("у тоста нет мягкой тени под флагом")
	}
	if s := m.GetStyle("notificationcenter", "card", StateNormal); s.ShadowBlur != 0 {
		t.Error("карточка получила тень панели")
	}
}
