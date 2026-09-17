package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// GG-77/78 через движок: меню кнопки выбирается щелчками, отданными движку;
// значок в заголовке панели перекрашен в цвет заголовка.

func TestMenuButton_EngineClickSelectsItem(t *testing.T) {
	eng := engine.New(300, 200, 20)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 200))
	fetched := 0
	split := widget.NewSplitButton("Pull", nil)
	split.Items = []widget.MenuItem{{Text: "Fetch", OnClick: func() { fetched++ }}}
	split.SetBounds(image.Rect(10, 10, 110, 40))
	root.AddChild(split)
	eng.SetRoot(root)
	eng.RenderOnce()

	closed := eng.RenderOnce()
	eng.SendMouseButton(100, 25, widget.MouseLeft, true)
	eng.SendMouseButton(100, 25, widget.MouseLeft, false)
	if !split.IsMenuOpen() {
		t.Fatal("щелчок по стрелке через движок не открыл меню")
	}
	open := eng.RenderOnce()
	if sameRect(closed, open, image.Rect(94, 10, 110, 40)) {
		t.Fatal("стрелка не показана нажатой при открытом меню")
	}

	r := split.OverlayBounds()
	x, y := r.Min.X+20, r.Min.Y+8
	eng.SendMouseButton(x, y, widget.MouseLeft, true)
	eng.SendMouseButton(x, y, widget.MouseLeft, false)
	if fetched != 1 || split.IsMenuOpen() {
		t.Fatalf("щелчок по пункту через движок: выполнен %d раз, меню открыто %v", fetched, split.IsMenuOpen())
	}
}

func TestDockPaneTitleButtons_EngineTintAndTooltip(t *testing.T) {
	eng := engine.New(300, 200, 20)
	p := widget.NewDockPane("branches", "Ветки", nil)
	p.TitleBG = color.RGBA{R: 30, G: 30, B: 30, A: 255}
	p.TitleText = color.RGBA{R: 200, G: 40, B: 40, A: 255}
	white := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range white.Pix {
		white.Pix[i] = 255
	}
	p.SetBounds(image.Rect(0, 0, 300, 200))
	p.SetTitleButtons([]widget.DockPaneButton{
		{Tooltip: "GitHub", Icon: white},
		{Tooltip: "Вид", Menu: []widget.MenuItem{{Text: "Сортировать"}}},
	})
	eng.SetRoot(p)
	frame := eng.RenderOnce()

	// Первая кнопка — 195..213 по X, 3..21 по Y; значок 14×14 по центру.
	got := frame.RGBAAt(204, 12)
	want := p.TitleText
	if absDiff(got.R, want.R) > 3 || absDiff(got.G, want.G) > 3 || absDiff(got.B, want.B) > 3 {
		t.Fatalf("значок в заголовке %v, ждал цвет заголовка %v", got, want)
	}
}

func sameRect(a, b *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
