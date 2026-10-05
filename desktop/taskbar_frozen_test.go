package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"os"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Кадры панели профилей, которые работа над панелью Windows 11 трогать не
// должна (Windows 2000 с синим вариантом, Windows 10 тёмная и светлая, macOS).
// Хэши сняты ДО изменений; расхождение значит, что новое оформление просочилось
// в чужой профиль. Windows 11 здесь намеренно нет — её панель менялась.
//
// FROZEN_PRINT=1 печатает хэши вместо сверки (для осознанного обновления).

// frozenBar — панель с «Пуском», поиском, областью приложений, треем и часами.
func frozenBar(t *testing.T, tm *theme.Manager, w, h int) *widget.Panel {
	t.Helper()
	root := widget.NewPanel(color.RGBA{R: 30, G: 60, B: 110, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	cat := NewStaticAppCatalog(
		AppInfo{ID: "files", Title: "Files", Icon: solidIcon(color.RGBA{R: 240, G: 180, B: 40, A: 255})},
		AppInfo{ID: "web", Title: "Browser", Icon: solidIcon(color.RGBA{R: 40, G: 140, B: 230, A: 255})},
		AppInfo{ID: "mail", Title: "Mail", Icon: solidIcon(color.RGBA{R: 220, G: 70, B: 80, A: 255})},
		AppInfo{ID: "term", Title: "Terminal", Icon: solidIcon(color.RGBA{R: 150, G: 90, B: 210, A: 255})},
	)
	for _, id := range []AppID{"files", "web", "mail", "term"} {
		cat.Pin(id)
	}
	wm := NewFakeWindowModel(
		WindowInfo{ID: 1, AppID: "web", Title: "Browser", Active: true},
		WindowInfo{ID: 2, AppID: "mail", Title: "Inbox"},
		WindowInfo{ID: 3, AppID: "mail", Title: "Draft", Minimized: true},
		WindowInfo{ID: 4, AppID: "term", Title: "bash"},
	)
	status := NewFakeSystemStatus()
	status.SetNetwork(NetState{Kind: NetWiFi, Quality: 0.7, Name: "home"})
	status.SetVolume(VolState{Level: 0.5})
	status.SetPower(PowerState{Charge: 0.8})
	notes := NewFakeNotifications()
	notes.Add(Notification{Title: "a"})
	notes.Add(Notification{Title: "b"})

	bar := NewTaskbar(tm)
	start := NewStartButton(tm)
	box := NewSearchBox(tm, NewFakeSearchProvider())
	area := NewApplicationArea(tm, cat, wm)
	tray := NewSystemTray(tm)
	tray.AddItem(NewNetworkStatus(tm, status))
	tray.AddItem(NewVolumeStatus(tm, status))
	tray.AddItem(NewPowerStatus(tm, status))
	tray.AddItem(NewNotificationButton(tm, notes))
	bar.AddItem(SlotStart, start)
	bar.AddItem(SlotStart, box)
	bar.AddItem(SlotApps, area)
	bar.AddItem(SlotTray, tray)
	bar.AddItem(SlotTray, NewClock(tm, NewFakeClock(time.Date(2026, 10, 5, 14, 30, 0, 0, time.UTC))))
	bar.SetBounds(image.Rect(0, h-bar.Height(), w, h))
	area.layout()
	// Наведение на вторую кнопку — состояние Hover тоже в кадре.
	hover := area.ButtonRect(1)
	area.OnMouseMove(hover.Min.X+4, hover.Min.Y+4)
	root.AddChild(bar)
	t.Cleanup(func() { bar.Close(); tray.Close(); widget.StopAllAnimations() })
	return root
}

func frameHash(img *image.RGBA) string {
	sum := sha256.Sum256(img.Pix)
	return hex.EncodeToString(sum[:8])
}

func TestTaskbar_OtherProfilesFrozen(t *testing.T) {
	want := map[string]string{}
	for k, v := range frozenHashes {
		want[k] = v
	}
	type variant struct {
		name  string
		light bool // флаг taskbar.light (светлая панель Windows 10)
	}
	variants := []variant{
		{theme.ProfileWindows2000, false}, {theme.ProfileWindows2000Blue, false},
		{theme.ProfileWindows10, false}, {theme.ProfileWindows10, true}, {theme.ProfileWindows10Dark, false},
		{theme.ProfileMacOS, false}, {theme.ProfileMacOSDark, false},
	}
	for _, v := range variants {
		name := v.name
		for _, scale := range []float64{1, 2} {
			tm := managerFor(t, name)
			if v.light {
				tm.SetFlag(theme.KeyTaskbarLight, true)
			}
			root := frozenBar(t, tm, 640, 120)
			eng := engine.New(640, 120, 30)
			if scale != 1 {
				eng.SetScale(scale)
			}
			// Глобальный стиль виджетов и кегль по умолчанию принадлежат процессу, а
			// не панели: без явного применения кадр зависел бы от соседних тестов.
			if err := eng.SetThemeProfile(tm, name); err != nil {
				t.Fatal(err)
			}
			eng.ApplyThemeProfile(tm)
			eng.SetRoot(root)
			eng.RenderOnce()
			finishAnimations()
			got := frameHash(eng.RenderOnce())
			key := fmt.Sprintf("%s light=%v @%v", name, v.light, scale)
			if os.Getenv("FROZEN_PRINT") != "" {
				fmt.Printf("\t%q: %q,\n", key, got)
				continue
			}
			if want[key] != got {
				t.Errorf("%s: кадр панели изменился (%s, было %s)", key, got, want[key])
			}
		}
	}
}
