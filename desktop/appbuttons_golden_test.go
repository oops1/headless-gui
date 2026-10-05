package desktop

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Кадры панели Windows 10 с кнопками приложений: запущено, активно, стопка,
// наведение, закреплённое незапущенное, «Пуск» с открытым меню, список окон и
// меню команд. Сохраняются при GOLDEN_OUT — смотреть глазами; без неё тест
// проверяет, что кадры рисуются и различаются.

// solidIcon — значок-плитка: яркий квадрат со светлой серединой. Настоящие
// значки приложений в движке не лежат, а плитка достаточна, чтобы видеть
// размер значка и линии под ним.
func solidIcon(c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			col := c
			if x >= 14 && x < 34 && y >= 14 && y < 34 {
				col = color.RGBA{R: 245, G: 245, B: 245, A: 255}
			}
			img.SetRGBA(x, y, col)
		}
	}
	return img
}

// colorPreviews — модель окон с цветными миниатюрами.
type colorPreviews struct{ *FakeWindowModel }

func (c colorPreviews) Preview(id WindowID, max image.Point) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 160, 96))
	base := uint8(60 + 30*int(id))
	for y := 0; y < 96; y++ {
		for x := 0; x < 160; x++ {
			img.SetRGBA(x, y, color.RGBA{R: base, G: uint8(90 + y), B: uint8(150 + x/4), A: 255})
		}
	}
	return img
}

type goldenParts struct {
	root  *widget.Panel
	bar   *Taskbar
	area  *ApplicationArea
	start *StartButton
	prev  *WindowPreview
	wm    *colorPreviews
	tm    *theme.Manager
}

func goldenScene(t *testing.T, light bool) *goldenParts {
	t.Helper()
	const w, h = 640, 240
	tm := win10Fast(t)
	tm.SetFlag(theme.KeyTaskbarLight, light)

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

	cat := NewStaticAppCatalog(
		AppInfo{ID: "files", Title: "Files", Icon: solidIcon(color.RGBA{R: 240, G: 180, B: 40, A: 255})},
		AppInfo{ID: "web", Title: "Browser", Icon: solidIcon(color.RGBA{R: 40, G: 140, B: 230, A: 255})},
		AppInfo{ID: "code", Title: "Editor", Icon: solidIcon(color.RGBA{R: 60, G: 190, B: 120, A: 255})},
		AppInfo{ID: "mail", Title: "Mail", Icon: solidIcon(color.RGBA{R: 220, G: 70, B: 80, A: 255})},
		AppInfo{ID: "term", Title: "Terminal", Icon: solidIcon(color.RGBA{R: 150, G: 90, B: 210, A: 255})},
		AppInfo{ID: "calc", Title: "Calc", Icon: solidIcon(color.RGBA{R: 90, G: 100, B: 120, A: 255})},
	)
	for _, id := range []AppID{"files", "web", "code", "mail", "term", "calc"} {
		cat.Pin(id)
	}
	wm := &colorPreviews{NewFakeWindowModel(
		WindowInfo{ID: 1, AppID: "web", Title: "Browser"},
		WindowInfo{ID: 2, AppID: "code", Title: "main.go — Editor", Active: true},
		WindowInfo{ID: 3, AppID: "mail", Title: "Inbox"},
		WindowInfo{ID: 4, AppID: "mail", Title: "New message"},
		WindowInfo{ID: 5, AppID: "mail", Title: "Calendar", Minimized: true},
		WindowInfo{ID: 6, AppID: "term", Title: "bash"},
	)}

	area := NewApplicationArea(tm, cat, wm)
	start := NewStartButton(tm)
	bar := NewTaskbar(tm)
	bar.AddItem(SlotStart, start)
	bar.AddItem(SlotApps, area)
	bar.AddItem(SlotTray, NewClock(tm, NewFakeClock(time.Date(2026, 10, 5, 14, 30, 0, 0, time.UTC))))
	barH := bar.Height()
	bar.SetBounds(image.Rect(0, h-barH, w, h))
	root.AddChild(bar)
	prev := NewWindowPreview(tm, wm)
	prev.Screen = image.Rect(0, 0, w, h)
	prev.Track(area)
	root.AddChild(prev)
	t.Cleanup(func() { prev.Close(); bar.Close(); widget.StopAllAnimations() })
	return &goldenParts{root: root, bar: bar, area: area, start: start, prev: prev, wm: wm, tm: tm}
}

func (g *goldenParts) render(scale float64) *image.RGBA {
	eng := engine.New(640, 240, 30)
	if scale != 1 {
		eng.SetScale(scale)
	}
	eng.SetRoot(g.root)
	eng.RenderOnce()
	finishAnimations()
	return eng.RenderOnce()
}

func savePNG(t *testing.T, name string, img *image.RGBA) {
	t.Helper()
	dir := os.Getenv("GOLDEN_OUT")
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

func TestGolden_Windows10AppButtons(t *testing.T) {
	var frames []*image.RGBA
	for _, light := range []bool{false, true} {
		name := "dark"
		if light {
			name = "light"
		}
		g := goldenScene(t, light)
		// Наведение на запущенный «Terminal» (6-я кнопка) и на закреплённый
		// незапущенный «Calc».
		g.area.layout()
		hover := g.area.ButtonRect(5)
		g.area.OnMouseMove(hover.Min.X+10, hover.Min.Y+10)
		img := g.render(1)
		savePNG(t, "win10_appbuttons_"+name+".png", img)
		savePNG(t, "win10_appbuttons_"+name+"_x2.png", g.render(2))
		frames = append(frames, img)
	}
	if imagesEqualRGBA(frames[0], frames[1]) {
		t.Error("тёмная и светлая панель одинаковы")
	}

	for _, light := range []bool{false, true} {
		name := "dark"
		if light {
			name = "light"
		}
		// «Пуск» горит, по щелчку на стопке «Mail» открыт список окон.
		g := goldenScene(t, light)
		g.start.SetActive(true)
		g.area.layout()
		clickAt(g.area, g.area.ButtonRect(3))
		if !g.prev.IsOpen() {
			t.Fatal("список окон не открылся")
		}
		img := g.render(1)
		savePNG(t, "win10_appbuttons_list_"+name+".png", img)
		savePNG(t, "win10_appbuttons_list_"+name+"_x2.png", g.render(2))
		if imagesEqualRGBA(img, frames[0]) || imagesEqualRGBA(img, frames[1]) {
			t.Error("список окон и «Пуск» Active не изменили кадр")
		}
		g.prev.Close()

		// Меню команд кнопки «Browser».
		g.start.SetActive(false)
		rightClick(g.area, g.area.ButtonRect(1))
		if !g.area.HasOverlay() {
			t.Fatal("меню команд не открыто")
		}
		savePNG(t, "win10_appbuttons_menu_"+name+".png", g.render(1))
		savePNG(t, "win10_appbuttons_menu_"+name+"_x2.png", g.render(2))
	}
}

func imagesEqualRGBA(a, b *image.RGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}
