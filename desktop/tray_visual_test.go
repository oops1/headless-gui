package desktop_test

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Снимки трея: панель с треем в разных темах (Windows 10 тёмная и светлая — с
// наведением и горящей кнопкой уведомлений) и классика с изменённым акцентом.
// Кадры сохраняются при TRAY_OUT (каталог) — смотреть глазами. Без неё тест
// проверяет, что кадры рисуются.

type trayParts struct {
	root   *widget.Panel
	bar    *desktop.Taskbar
	volume *desktop.VolumeItem
	notify *desktop.NotificationButton
}

func trayWallpaper(w, h int) *widget.Panel {
	root := widget.NewPanel(color.RGBA{R: 30, G: 60, B: 110, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	const cell = 24
	for y := 0; y < h; y += cell {
		for x := 0; x < w; x += cell {
			shade := uint8(70)
			if (x/cell+y/cell)%2 == 0 {
				shade = 150
			}
			p := widget.NewPanel(color.RGBA{R: shade, G: uint8(90 + int(shade)/4), B: 170, A: 255})
			p.ShowHeader = false
			p.SetBounds(image.Rect(x, y, x+cell, y+cell))
			root.AddChild(p)
		}
	}
	return root
}

// trayScene — панель с «Пуском», треем (сеть, звук, питание, значок), часами,
// кнопкой центра уведомлений со счётчиком и полоской «Показать рабочий стол».
func trayScene(t *testing.T, tm *theme.Manager, w, h int) *trayParts {
	t.Helper()
	tm.SetIconResolver(widget.BuiltinIcons())
	root := trayWallpaper(w, h)

	st := desktop.NewFakeSystemStatus()
	bar := desktop.NewTaskbar(tm)
	bar.AddItem(desktop.SlotStart, desktop.NewStartButton(tm))
	tray := desktop.NewSystemTray(tm)
	tray.AddItem(desktop.NewNetworkStatus(tm, st))
	vol := desktop.NewVolumeStatus(tm, st)
	tray.AddItem(vol)
	tray.AddItem(desktop.NewPowerStatus(tm, st))
	bar.AddItem(desktop.SlotTray, tray)
	bar.AddItem(desktop.SlotTray, desktop.NewClock(tm, desktop.NewFakeClock(time.Date(2026, 10, 5, 14, 30, 0, 0, time.UTC))))
	nb := desktop.NewNotificationButton(tm, nil)
	nb.SetCount(3)
	bar.AddItem(desktop.SlotTray, nb)
	bar.AddItem(desktop.SlotTray, desktop.NewShowDesktopButton(tm))
	bar.SetBounds(image.Rect(0, h-bar.Height(), w, h))
	root.AddChild(bar)
	t.Cleanup(func() { bar.Close(); widget.StopAllAnimations() })
	return &trayParts{root: root, bar: bar, volume: vol, notify: nb}
}

func renderTray(root *widget.Panel, w, h int, scale float64) *image.RGBA {
	eng := engine.New(w, h, 30)
	if scale != 1 {
		eng.SetScale(scale)
	}
	eng.SetRoot(root)
	eng.RenderOnce()
	time.Sleep(10 * time.Millisecond)
	return eng.RenderOnce()
}

func saveTrayPNG(t *testing.T, name string, img *image.RGBA) {
	t.Helper()
	dir := os.Getenv("TRAY_OUT")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func trayTheme(t *testing.T, name string) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	return m
}

// Панель с треем во всех темах: для сравнения до и после (Windows 11, macOS и
// классика не должны измениться).
func TestVisual_TrayAllThemes(t *testing.T) {
	const w, h = 460, 80
	// Порядок фиксирован, сцена строится заново на каждый кадр: раскладка часов
	// зависит от измерителя текста последнего созданного движка, и случайный
	// порядок обхода карты давал разные кадры от запуска к запуску.
	// Движок регистрирует измеритель текста: без него первая же сцена строила бы
	// раскладку часов по запасным метрикам и отличалась бы от остальных.
	engine.New(w, h, 30)
	for _, tc := range []struct{ file, name string }{
		{"win2000", theme.ProfileWindows2000}, {"win2000blue", theme.ProfileWindows2000Blue},
		{"win11", theme.ProfileWindows11}, {"win11dark", theme.ProfileWindows11Dark},
		{"macos", theme.ProfileMacOS}, {"win10", theme.ProfileWindows10},
	} {
		for _, scale := range []float64{1, 2} {
			p := trayScene(t, trayTheme(t, tc.name), w, h)
			img := renderTray(p.root, w, h, scale)
			suffix := ""
			if scale != 1 {
				suffix = "_x2"
			}
			saveTrayPNG(t, "tray_"+tc.file+suffix+".png", img)
		}
	}
}

// Windows 10: наведение на значок звука подсвечивает всю полосу, кнопка центра
// уведомлений горит, пока центр открыт.
func TestVisual_Win10TrayHoverAndActive(t *testing.T) {
	const w, h = 460, 80
	var frames []*image.RGBA
	for _, light := range []bool{false, true} {
		name := "dark"
		if light {
			name = "light"
		}
		tm := trayTheme(t, theme.ProfileWindows10)
		tm.SetFlag(theme.KeyTaskbarLight, light)
		p := trayScene(t, tm, w, h)
		vb := p.volume.Bounds()
		if vb.Dy() != 40 {
			t.Fatalf("%s: значок звука высотой %d, ждали 40", name, vb.Dy())
		}
		p.volume.OnMouseMove(vb.Min.X+3, vb.Min.Y+3)
		p.notify.SetActive(true)
		img := renderTray(p.root, w, h, 1)
		saveTrayPNG(t, "tray_win10_"+name+"_hover.png", img)
		saveTrayPNG(t, "tray_win10_"+name+"_hover_x2.png", renderTray(p.root, w, h, 2))
		frames = append(frames, img)

		// Подсветка наведения доходит до верха и низа полосы.
		top, bottom := vb.Min.Y, vb.Max.Y-1
		mid := vb.Min.X + 2
		base := renderTray(trayScene(t, func() *theme.Manager {
			m := trayTheme(t, theme.ProfileWindows10)
			m.SetFlag(theme.KeyTaskbarLight, light)
			return m
		}(), w, h).root, w, h, 1)
		for _, y := range []int{top + 1, bottom - 1} {
			if img.RGBAAt(mid, y) == base.RGBAAt(mid, y) {
				t.Errorf("%s: подсветка наведения не дошла до y=%d", name, y)
			}
		}
	}
	if frames[0].Bounds() == (image.Rectangle{}) {
		t.Fatal("кадр пуст")
	}
}

// Классика следует SetAccent: календарь с выбранным днём (и меню «Пуск»)
// перекрашиваются, серая палитра остаётся.
func TestVisual_ClassicWithAccent(t *testing.T) {
	const w, h = 460, 300
	var frames []*image.RGBA
	for _, accent := range []*color.RGBA{nil, {R: 16, G: 124, B: 16, A: 255}} {
		tm := trayTheme(t, theme.ProfileWindows2000)
		if accent != nil {
			tm.SetAccent(*accent)
		}
		p := trayScene(t, tm, w, h)
		cal := desktop.NewCalendarFlyout(tm, desktop.NewFakeClock(time.Date(2026, 10, 5, 14, 30, 0, 0, time.UTC)))
		cal.Screen = image.Rect(0, 0, w, h)
		p.root.AddChild(cal)
		cal.Open(image.Rect(w-120, h-28, w-60, h))
		img := renderTray(p.root, w, h, 1)
		name := "tray_win2000_accent_default.png"
		if accent != nil {
			name = "tray_win2000_accent_green.png"
		}
		saveTrayPNG(t, name, img)
		frames = append(frames, img)
	}
	same := true
	for i := range frames[0].Pix {
		if frames[0].Pix[i] != frames[1].Pix[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("SetAccent не изменил кадр классической темы")
	}
}
