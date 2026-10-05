package desktop_test

import (
	"crypto/sha1"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Снимки панелей оболочки на цветных обоях.
//
// Кадры сохраняются при MAT_OUT / SNAP_OUT (каталог) — смотреть глазами.
// SNAP_OUT пишет кадры панелей ВСЕХ встроенных профилей без флагов вместе со
// списком хешей sums.txt: запустить тест до и после изменения и сравнить файлы —
// вид прежних тем не должен измениться. Файл не использует ничего, чего не было
// до материалов Mica и токенов тени, и потому запускается на прежнем коде как есть.

const matW, matH = 900, 560

// matWallpaper — цветные «обои»: диагональный градиент с крупными пятнами,
// чтобы размытие под панелью было видно.
func matWallpaper(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	blobs := []struct {
		cx, cy, r float64
		c         color.RGBA
	}{
		{0.25, 0.3, 0.30, color.RGBA{230, 90, 60, 255}},
		{0.7, 0.35, 0.28, color.RGBA{40, 170, 230, 255}},
		{0.5, 0.85, 0.35, color.RGBA{120, 70, 200, 255}},
		{0.9, 0.9, 0.22, color.RGBA{250, 200, 60, 255}},
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)
			r := 40 + 80*fx
			g := 70 + 90*fy
			b := 150 - 60*fx
			for _, bl := range blobs {
				d := math.Hypot((fx-bl.cx)*float64(w)/float64(h), fy-bl.cy) / bl.r
				if d < 1 {
					k := (1 - d) * (1 - d)
					r += (float64(bl.c.R) - r) * k
					g += (float64(bl.c.G) - g) * k
					b += (float64(bl.c.B) - b) * k
				}
			}
			img.SetRGBA(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), 255})
		}
	}
	return img
}

type matPanel string

const (
	matStart    matPanel = "start"
	matQuick    matPanel = "quick"
	matNotify   matPanel = "notify"
	matCalendar matPanel = "calendar"
	matMenu     matPanel = "menu"
	matWindow   matPanel = "window"
)

// matWindowHook донастраивает окно сцены по менеджеру тем (материал Mica из
// стиля "window"); задаётся тестами материалов.
var matWindowHook func(w *widget.Window, m *theme.Manager)

// matScene рисует панель задач и открытую панель на обоях. tune правит менеджер
// тем (флаги) до применения.
func matScene(t *testing.T, profile string, panel matPanel, tune func(m *theme.Manager)) (img *image.RGBA, area image.Rectangle) {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(profile); err != nil {
		t.Fatal(err)
	}
	if tune != nil {
		tune(m)
	}
	icons := widget.BuiltinIcons()
	m.SetIconResolver(icons)

	eng := engine.New(matW, matH, 30)
	registerFonts(t, eng)
	if err := eng.SetBackground(matWallpaper(matW, matH)); err != nil {
		t.Fatal(err)
	}
	if err := eng.ApplyThemeProfile(m); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Глобальная палитра виджетов вернулась к умолчанию для соседних тестов.
		m2 := theme.NewManager()
		_ = theme.RegisterBuiltinProfiles(m2)
		_ = m2.SetTheme(theme.ProfileWindows10)
		_ = eng.ApplyThemeProfile(m2)
		widget.StopAllAnimations()
	})

	root := widget.NewCanvas()
	root.SetBounds(image.Rect(0, 0, matW, matH))

	wm := desktop.NewFakeWindowModel(
		desktop.WindowInfo{ID: 1, AppID: "files", Title: "Проводник", Icon: icons.ResolveIcon(theme.IconRef{Name: "start"}, 24)},
		desktop.WindowInfo{ID: 2, AppID: "term", Title: "Терминал", Active: true, Icon: icons.ResolveIcon(theme.IconRef{Name: "network.ethernet"}, 24)},
	)
	status := desktop.NewFakeSystemStatus()
	clk := desktop.NewFakeClock(time.Date(2026, 10, 5, 7, 35, 0, 0, time.UTC))
	bar := desktop.NewTaskbar(m)
	bar.AddItem(desktop.SlotStart, desktop.NewStartButton(m))
	bar.AddItem(desktop.SlotApps, desktop.NewRunningApplications(m, wm))
	bar.AddItem(desktop.SlotTray, desktop.NewNetworkStatus(m, status))
	bar.AddItem(desktop.SlotTray, desktop.NewVolumeStatus(m, status))
	bar.AddItem(desktop.SlotTray, desktop.NewPowerStatus(m, status))
	bar.AddItem(desktop.SlotTray, desktop.NewClock(m, clk))
	barH := bar.Height()
	bar.SetBounds(image.Rect(0, matH-barH, matW, matH))
	root.AddChild(bar)
	t.Cleanup(bar.Close)

	screen := image.Rect(0, 0, matW, matH)
	anchorStart := image.Rect(matW/2-24, matH-barH, matW/2+24, matH)
	anchorTray := image.Rect(matW-110, matH-barH, matW-60, matH)

	switch panel {
	case matStart:
		cat := desktop.NewStaticAppCatalog(
			desktop.AppInfo{ID: "edge", Title: "Microsoft Edge"},
			desktop.AppInfo{ID: "store", Title: "Microsoft Store"},
			desktop.AppInfo{ID: "calc", Title: "Калькулятор"},
			desktop.AppInfo{ID: "term", Title: "Терминал"},
		)
		p := desktop.NewStartMenu(m, cat)
		p.Screen = screen
		root.AddChild(p)
		p.Open(anchorStart)
		p.Settle()
		area = p.OverlayBounds()
		t.Cleanup(p.Close)
	case matQuick:
		p := desktop.NewQuickSettings(m, status)
		p.Screen = screen
		root.AddChild(p)
		p.Open(anchorTray)
		p.Settle()
		area = p.OverlayBounds()
		t.Cleanup(p.Close)
	case matNotify:
		when := time.Date(2026, 10, 5, 7, 37, 0, 0, time.Local)
		p := desktop.NewNotificationCenter(m, ncSampleNotes(when))
		p.Clock = desktop.NewFakeClock(when)
		p.SetQuickActions(ncSampleQuick())
		p.Screen = screen
		root.AddChild(p)
		p.Open(anchorTray)
		p.Settle()
		area = p.OverlayBounds()
		t.Cleanup(p.Close)
	case matCalendar:
		p := desktop.NewCalendarFlyout(m, clk)
		p.Screen = screen
		root.AddChild(p)
		p.Open(anchorTray)
		p.Settle()
		area = p.OverlayBounds()
		t.Cleanup(p.Close)
	case matMenu:
		pm := widget.NewPopupMenu()
		pm.AddItem("Открыть", func() {})
		pm.AddItem("Закрепить на панели задач", func() {})
		pm.AddSeparator()
		pm.AddItem("Свойства", func() {})
		root.AddChild(pm)
		pm.Show(300, 200)
		area = pm.OverlayBounds()
	case matWindow:
		w := widget.NewWindow("Окно", 360, 220)
		w.MainWindow = false
		w.SetBounds(image.Rect(280, 140, 640, 360))
		root.AddChild(w)
		area = w.Bounds()
		if matWindowHook != nil {
			matWindowHook(w, m)
		}
	}
	eng.SetRoot(root)
	eng.Invalidate()
	return eng.RenderOnce(), area
}

func matSave(t *testing.T, dirEnv string, img *image.RGBA, name string) {
	dir := os.Getenv(dirEnv)
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func meanLum(img *image.RGBA, r image.Rectangle) float64 {
	r = r.Intersect(img.Bounds())
	var sum float64
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			sum += 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func equalImages(a, b *image.RGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		ra := a.Pix[a.PixOffset(0, y) : a.PixOffset(0, y)+a.Bounds().Dx()*4]
		rb := b.Pix[b.PixOffset(0, y) : b.PixOffset(0, y)+b.Bounds().Dx()*4]
		for i := range ra {
			if ra[i] != rb[i] {
				return false
			}
		}
	}
	return true
}

// Кадры панелей всех встроенных профилей без флагов — для сравнения «до и
// после» (SNAP_OUT). Тест без каталога проверяет только, что кадр стабилен.
func TestSnapshots_AllProfilesDefault(t *testing.T) {
	profiles := []string{
		theme.ProfileWindows2000, theme.ProfileWindows2000Blue, theme.ProfileWindows10, theme.ProfileWindows10Dark,
		theme.ProfileWindows11, theme.ProfileWindows11Dark, theme.ProfileMacOS, theme.ProfileMacOSDark,
	}
	var sums []string
	for _, p := range profiles {
		for _, panel := range []matPanel{matStart, matQuick, matNotify, matCalendar, matMenu, matWindow} {
			img, _ := matScene(t, p, panel, nil)
			again, _ := matScene(t, p, panel, nil)
			if !equalImages(img, again) {
				t.Errorf("%s/%s: два кадра подряд отличаются", p, panel)
			}
			name := fmt.Sprintf("%s__%s", p, panel)
			matSave(t, "SNAP_OUT", img, name)
			sums = append(sums, fmt.Sprintf("%s %x", name, sha1.Sum(img.Pix)))
		}
	}
	if dir := os.Getenv("SNAP_OUT"); dir != "" {
		out := ""
		for _, s := range sums {
			out += s + "\n"
		}
		_ = os.WriteFile(filepath.Join(dir, "sums.txt"), []byte(out), 0o644)
	}
}
