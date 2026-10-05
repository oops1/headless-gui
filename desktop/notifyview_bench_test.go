package desktop

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

// Раскладка считается на каждое движение мыши и каждый кадр: она должна быть
// дешёвой и при сотне карточек.
func BenchmarkNotificationCenterLayout(b *testing.B) {
	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		b.Fatal(err)
	}
	if err := tm.SetTheme(theme.ProfileWindows10); err != nil {
		b.Fatal(err)
	}
	ns := NewFakeNotifications()
	for i := 0; i < 100; i++ {
		ns.Add(Notification{AppID: AppID("a" + string(rune('a'+i%5))), AppName: "Приложение", Title: "Заголовок уведомления",
			Body: "Достаточно длинный текст уведомления, который переносится на несколько строк карточки", Time: sampleTime()})
	}
	nc := NewNotificationCenter(tm, ns)
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	nc.Settle()
	defer nc.Close()
	panel := nc.rect()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nc.view.onMove(panel, image.Pt(500, 100+i%300), false)
	}
}
