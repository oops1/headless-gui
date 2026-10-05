package desktop

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Кнопки приложений и список окон стопки у бокового края: список раскрывается
// вбок от панели, а линия «запущено» и «активно» стоит на стороне ячейки,
// обращённой к краю экрана.

// sideScene — сцена Windows 10 с панелью у бокового края. Панель, область
// приложений и список окон — те же, что у нижней; отличается край и привязка
// всплывающих панелей (BindFlyouts).
func sideScene(t *testing.T, edge Edge, light bool) *goldenParts {
	t.Helper()
	const w, h = 960, 420
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
	bar.SetEdge(edge)
	bar.AddItem(SlotStart, start)
	bar.AddItem(SlotApps, area)
	bar.AddItem(SlotTray, NewClock(tm, NewFakeClock(time.Date(2026, 10, 5, 14, 30, 0, 0, time.UTC))))
	th := bar.Thickness()
	if edge == EdgeLeft {
		bar.SetBounds(image.Rect(0, 0, th, h))
	} else {
		bar.SetBounds(image.Rect(w-th, 0, w, h))
	}
	root.AddChild(bar)
	prev := NewWindowPreview(tm, wm)
	prev.Screen = image.Rect(0, 0, w, h)
	prev.Track(area)
	bar.BindFlyouts(prev)
	root.AddChild(prev)
	t.Cleanup(func() { prev.Close(); bar.Close(); widget.StopAllAnimations() })
	return &goldenParts{root: root, bar: bar, area: area, start: start, prev: prev, wm: wm, tm: tm}
}

func (g *goldenParts) renderSize(w, h int) *image.RGBA {
	eng := engine.New(w, h, 30)
	eng.SetRoot(g.root)
	eng.RenderOnce()
	finishAnimations()
	return eng.RenderOnce()
}

func TestSideStack_Shots(t *testing.T) {
	for _, tc := range []struct {
		edge  Edge
		light bool
		name  string
	}{{EdgeLeft, false, "left_dark"}, {EdgeLeft, true, "left_light"}, {EdgeRight, false, "right_dark"}} {
		g := sideScene(t, tc.edge, tc.light)
		g.area.layout()
		// Список открыт клавишей, выбрано второе окно: видна рамка фокуса.
		g.prev.groupKeyed(3)
		g.prev.OnKeyEvent(widget.KeyEvent{Code: widget.KeyRight, Pressed: true})
		if !g.prev.IsOpen() {
			t.Fatal("список не открылся")
		}
		savePNG(t, "win10_side_list_"+tc.name+".png", g.renderSize(960, 420))
	}
}
