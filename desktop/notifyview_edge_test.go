package desktop

import (
	"image"
	"testing"
	"time"
)

// Уведомления прежней модели (без AppID и AppName) показываются карточками без
// заголовка группы: пустую подпись над ними рисовать нечем.
func TestNotificationCenter_LegacyNotificationsHaveNoGroupHeader(t *testing.T) {
	nc := NewNotificationCenter(richNotifTheme(t), notesFake())
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	nc.Settle()
	defer nc.Close()
	l := nc.testLayout()
	if len(l.groups) != 1 || len(l.groups[0].cards) != 2 {
		t.Fatalf("раскладка: групп %d", len(l.groups))
	}
	if !l.groups[0].rect.Empty() {
		t.Errorf("у безымянной группы есть заголовок %v", l.groups[0].rect)
	}
	for _, z := range l.zones {
		if z.key.kind == zoneGroup || z.key.kind == zoneGroupClose {
			t.Errorf("зона заголовка у безымянной группы: %+v", z.key)
		}
	}
	_ = drawTexts(nc)
}

// Движение мыши над списком не перерисовывает список целиком: бегунок
// появляется один раз полосой, повторные движения в ту же точку ничего не
// заявляют.
func TestNotificationCenter_MouseMoveDoesNotRepaintWholeList(t *testing.T) {
	nc, _, _ := richFixture(t, 14)
	inv := watchInvalidations(t)
	l := nc.testLayout()
	pt := image.Pt(l.viewport.Min.X+100, l.viewport.Min.Y+60)
	nc.OnMouseMove(pt.X, pt.Y)
	for _, r := range inv.get() {
		if r.Dx() >= l.viewport.Dx() && r.Dy() >= l.viewport.Dy() {
			t.Errorf("движение мыши заявило весь список: %v", r)
		}
	}
	inv.reset()
	nc.OnMouseMove(pt.X, pt.Y)
	nc.OnMouseMove(pt.X+1, pt.Y)
	if got := inv.get(); len(got) != 0 {
		t.Errorf("повторные движения в пределах одного элемента заявили %v", got)
	}
}

// Крохотный экран и сотня карточек: раскладка и рисование не падают, зоны не
// выходят за панель.
func TestNotificationCenter_TinyScreenDoesNotBreak(t *testing.T) {
	ns := NewFakeNotifications()
	for i := 0; i < 100; i++ {
		ns.Add(Notification{AppID: AppID("a" + string(rune('a'+i%7))), AppName: "Приложение", Title: "Заголовок",
			Body: "Очень длинный текст уведомления, который переносится на несколько строк", Time: sampleTime(),
			Actions: []NotificationAction{{ID: "x", Kind: NotificationActionButton, Title: "Да"}}})
	}
	nc := NewNotificationCenter(richNotifTheme(t), ns)
	nc.SetQuickActions(NewQuickActionList(
		QuickAction{ID: "1", Title: "Один"}, QuickAction{ID: "2", Title: "Два"}, QuickAction{ID: "3", Title: "Три"},
		QuickAction{ID: "4", Title: "Четыре"}, QuickAction{ID: "5", Title: "Пять"}, QuickAction{ID: "6", Title: "Шесть"},
		QuickAction{ID: "7", Title: "Семь"}, QuickAction{ID: "8", Title: "Восемь"}, QuickAction{ID: "9", Title: "Девять"}))
	nc.SetQuickExpanded(true)
	nc.Clock = NewFakeClock(sampleTime().Add(24 * time.Hour))
	for _, scr := range []image.Rectangle{image.Rect(0, 0, 420, 200), image.Rect(0, 0, 300, 120), image.Rect(0, 0, 800, 600)} {
		nc.Close()
		nc.Screen = scr
		nc.Open(image.Rect(0, scr.Max.Y-40, 48, scr.Max.Y))
		nc.Settle()
		l := nc.testLayout()
		for _, z := range l.zones {
			if !z.hit.In(l.panel) {
				t.Fatalf("экран %v: зона %+v (%v) вышла за панель %v, окно списка %v", scr, z.key, z.hit, l.panel, l.viewport)
			}
		}
		if l.viewport.Dy() > l.panel.Dy() {
			t.Errorf("экран %v: окно списка выше панели", scr)
		}
		_ = drawTexts(nc)
		nc.OnMouseWheelPixels(scr.Max.X-100, 60, 0, 500)
		nc.OnKeyEvent(key(0x22)) // PgDn
		nc.OnKeyEvent(key(0x09)) // Tab
	}
	nc.Close()
}
