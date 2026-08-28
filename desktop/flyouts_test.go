package desktop

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Всплывающие панели под каждой из четырёх тем.
//
// Компонент без метрик темы просит нулевой размер и не открывается вовсе —
// молча, без ошибки. Пропустить такую дыру легко: тесты компонента ходят по
// собственной тестовой теме, где метрики есть, а в настоящей теме панель
// просто не появится. Поэтому проверяется каждая тема и каждая панель.

func allBuiltinThemes(t *testing.T) []string {
	t.Helper()
	return []string{
		theme.ProfileWindows2000, theme.ProfileWindows2000Blue,
		theme.ProfileWindows10, theme.ProfileWindows10Dark,
		theme.ProfileWindows11, theme.ProfileWindows11Dark,
		theme.ProfileMacOS, theme.ProfileMacOSDark,
	}
}

func TestFlyouts_OpenUnderEveryTheme(t *testing.T) {
	screen := image.Rect(0, 0, 800, 600)
	anchor := image.Rect(0, 560, 48, 600)

	for _, name := range allBuiltinThemes(t) {
		name := name
		t.Run(name, func(t *testing.T) {
			tm := managerFor(t, name)
			tm.SetIconResolver(widget.BuiltinIcons())

			panels := map[string]interface {
				Open(image.Rectangle)
				OverlayBounds() image.Rectangle
			}{
				"startmenu":     NewStartMenu(tm, NewStaticAppCatalog(sampleApps()...)),
				"quicksettings": NewQuickSettings(tm, NewFakeSystemStatus()),
				"notifications": NewNotificationCenter(tm, notesFake()),
				"calendar":      NewCalendarFlyout(tm, NewFakeClock(sampleTime())),
			}

			for what, panel := range panels {
				setScreen(panel, screen)
				panel.Open(anchor)
				r := panel.OverlayBounds()
				if r.Empty() {
					t.Errorf("%s: панель не открылась — темой не заданы её метрики", what)
					continue
				}
				if !r.In(screen) {
					t.Errorf("%s: панель %v вышла за экран %v", what, r, screen)
				}
				if r.Dx() < 40 || r.Dy() < 40 {
					t.Errorf("%s: панель вышла вырожденной: %v", what, r)
				}
			}
		})
	}
}

// setScreen задаёт границы экрана панели через встроенный Flyout.
func setScreen(panel any, screen image.Rectangle) {
	switch p := panel.(type) {
	case *StartMenu:
		p.Screen = screen
	case *QuickSettings:
		p.Screen = screen
	case *NotificationCenter:
		p.Screen = screen
	case *CalendarFlyout:
		p.Screen = screen
	}
}

func sampleApps() []AppInfo {
	return []AppInfo{
		{ID: "term", Title: "Терминал"},
		{ID: "files", Title: "Проводник"},
		{ID: "mail", Title: "Почта"},
	}
}

// notesFake — пара уведомлений разной важности.
func notesFake() *FakeNotifications {
	n := NewFakeNotifications()
	n.Add(Notification{Title: "Обновление", Body: "Готово к установке", Time: sampleTime()})
	n.Add(Notification{Title: "Сеть", Body: "Подключено", Time: sampleTime(), Severity: SeverityWarning})
	return n
}

func sampleTime() time.Time { return time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC) }
