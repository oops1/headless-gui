package theme

import "testing"

func notifyManager(t *testing.T, name string) *Manager {
	t.Helper()
	m := NewManager()
	if err := RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	return m
}

// Презентер центра уведомлений назначен только Windows 10 (и его тёмной
// разновидности, что наследует профиль); остальные темы остаются плоскими.
func TestNotificationCenterPresenterOnlyForWindows10(t *testing.T) {
	for _, name := range []string{ProfileWindows10, ProfileWindows10Dark} {
		m := notifyManager(t, name)
		if got := m.Active().PresenterName("notificationcenter"); got != NotificationCenterPresenter {
			t.Errorf("%s: презентер %q", name, got)
		}
	}
	for _, name := range []string{ProfileWindows11, ProfileWindows11Dark, ProfileWindows2000, ProfileMacOS} {
		m := notifyManager(t, name)
		if got := m.Active().PresenterName("notificationcenter"); got != "" {
			t.Errorf("%s: у плоской темы презентер центра %q", name, got)
		}
		if got := m.GetMetric("notificationcenter.width"); got != 0 {
			t.Errorf("%s: метрика notificationcenter.width = %v появилась у плоской темы", name, got)
		}
	}
}

func TestWindows10NotificationMetrics(t *testing.T) {
	m := notifyManager(t, ProfileWindows10)
	if w := m.GetMetric("notificationcenter.width"); w < 360 || w > 400 {
		t.Errorf("ширина центра %v вне 360–400", w)
	}
	if g := m.GetMetric("notificationcenter.margin"); g != 0 {
		t.Errorf("зазор до панели задач %v, ждал 0", g)
	}
	for _, k := range []Key{
		"notificationcenter.pad", "notificationcenter.card.pad", "notificationcenter.card.icon",
		"notificationcenter.action.height", "notificationcenter.quick.columns", "notificationcenter.quick.height",
		"notificationcenter.toast.width", "notificationcenter.toast.timeout",
	} {
		if m.GetMetric(k) <= 0 {
			t.Errorf("метрика %s не объявлена", k)
		}
	}
}

// Новые части стиля объявлены в тёмном и светлом режимах и следуют за акцентом
// там, где должны.
func TestWindows10NotificationParts(t *testing.T) {
	for _, light := range []bool{false, true} {
		m := notifyManager(t, ProfileWindows10)
		m.SetFlag(KeyTaskbarLight, light)
		for _, part := range []string{"toast", "field", "glyph", "dim", "scrollbar", "panel", "card", "action", "link", "quick.tile", "quick.tile.on"} {
			s := m.GetStyle("notificationcenter", part, StateNormal)
			if s.Fill.A == 0 && s.Text.A == 0 && s.Backdrop.Mode == BackdropNone {
				t.Errorf("light=%v: часть %s пуста", light, part)
			}
		}
		on := m.GetStyle("notificationcenter", "quick.tile.on", StateNormal)
		if acc, _ := m.Accent(); on.Fill != acc {
			t.Errorf("light=%v: включённая плитка %v не равна акценту", light, on.Fill)
		}
		m.SetAccent(RGB(200, 30, 30))
		on = m.GetStyle("notificationcenter", "quick.tile.on", StateNormal)
		if on.Fill != RGB(200, 30, 30) {
			t.Errorf("light=%v: плитка не последовала за сменой акцента: %v", light, on.Fill)
		}
	}
}
