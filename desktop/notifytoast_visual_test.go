package desktop_test

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Тост над треем: тот же вид, что и карточка центра. Снимки — при NC_OUT.
func TestVisual_NotificationToastWin10(t *testing.T) {
	for _, light := range []bool{false, true} {
		name := "toast_dark"
		if light {
			name = "toast_light"
		}
		t.Run(name, func(t *testing.T) {
			m := theme.NewManager()
			if err := theme.RegisterBuiltinProfiles(m); err != nil {
				t.Fatal(err)
			}
			if err := m.SetTheme(theme.ProfileWindows10); err != nil {
				t.Fatal(err)
			}
			m.SetFlag(theme.KeyTaskbarLight, light)
			m.SetIconResolver(widget.BuiltinIcons())
			const w, h = 1280, 760
			root, bar := buildScene(t, m, w, h)
			t.Cleanup(bar.Close)

			clock := time.Date(2026, 10, 5, 7, 37, 0, 0, time.Local)
			ns := desktop.NewFakeNotifications()
			ts := desktop.NewNotificationToast(m, ns)
			t.Cleanup(ts.Close)
			ts.Screen = image.Rect(0, 0, w, h)
			ts.Anchor = image.Rect(w-300, bar.Bounds().Min.Y, w, h)
			ts.Clock = desktop.NewFakeClock(clock)
			ts.Timeout = time.Hour
			root.AddChild(ts)

			eng := engine.New(w, h, 30)
			eng.SetRoot(root)
			ns.Add(desktop.Notification{
				AppID: "mail", AppName: "Почта", Icon: ncGlyph(48, color.RGBA{R: 60, G: 170, B: 90, A: 255}, 2),
				Title: "Анна Иванова", Body: "Привет! Посмотри, пожалуйста, договор до конца дня.",
				Timestamp: clock.Add(-time.Minute),
				Actions: []desktop.NotificationAction{
					{ID: "open", Kind: desktop.NotificationActionButton, Title: "Открыть"},
					{ID: "later", Kind: desktop.NotificationActionButton, Title: "Позже"},
				},
			})
			ts.Settle()
			if !ts.IsOpen() || ts.OverlayBounds().Empty() {
				t.Fatal("тост не показан")
			}
			eng.Invalidate()
			ncSave(t, eng.RenderOnce(), "nc_win10_"+name)
		})
	}
}
