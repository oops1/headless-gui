package theme

import "testing"

// Полоса важности: у Windows 10 (светлой и тёмной панели) предупреждение и
// ошибка имеют свои цвета, различимые между собой.
func TestWindows10NotificationSeverityParts(t *testing.T) {
	for _, name := range []string{ProfileWindows10, ProfileWindows10Dark} {
		for _, light := range []bool{false, true} {
			m := notifyManager(t, name)
			m.SetFlag(KeyTaskbarLight, light)
			w := m.GetStyle("notificationcenter", "severity.warning", StateNormal).Fill
			e := m.GetStyle("notificationcenter", "severity.error", StateNormal).Fill
			if w.A != 255 || e.A != 255 || w == e {
				t.Errorf("%s light=%v: цвета важности %v и %v", name, light, w, e)
			}
		}
		if m := notifyManager(t, name); m.GetMetric("notificationcenter.severity.width") <= 0 {
			t.Errorf("%s: нет метрики ширины полосы важности", name)
		}
	}
	// Светлая панель получает тёмные оттенки: жёлтый на белом не читается.
	m := notifyManager(t, ProfileWindows10)
	m.SetFlag(KeyTaskbarLight, false)
	dark := m.GetStyle("notificationcenter", "severity.warning", StateNormal).Fill
	m.SetFlag(KeyTaskbarLight, true)
	light := m.GetStyle("notificationcenter", "severity.warning", StateNormal).Fill
	if dark == light {
		t.Error("светлая и тёмная панели красят предупреждение одним цветом")
	}
}

// Анимация раскрытия объявлена у Windows 10 и его тёмной разновидности и не
// появляется у плоских тем (там элементов, которые раскрываются, нет).
func TestWindows10NotificationExpandAnimation(t *testing.T) {
	for _, name := range []string{ProfileWindows10, ProfileWindows10Dark} {
		a := notifyManager(t, name).GetAnimation("notification.expand")
		if a.Duration <= 0 || a.Curve == "" {
			t.Errorf("%s: анимация раскрытия %+v", name, a)
		}
		if a.Curve == "out-back" {
			t.Errorf("%s: кривая с отскоком на высоте даёт дрожание краёв", name)
		}
	}
	for _, name := range []string{ProfileWindows2000, ProfileMacOS} {
		if a := notifyManager(t, name).GetAnimation("notification.expand"); a.Duration != 0 {
			t.Errorf("%s: у плоского центра появилась анимация раскрытия %+v", name, a)
		}
	}
}
