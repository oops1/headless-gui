package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// frames_test.go — перевод координат на границе «экран ↔ содержимое прокрутки»
// там, где его делает движок: подсказка, попапы в окнах ОС, кадр виджета вне
// дерева. Хит-тест, захват и поиск оверлея — core/input/frames_test.go;
// сквозные проверки через SendMouse* и кадр — tests/scrollview_coords_test.go.

// frameScene — корень 300×400, в нём прокрутка 300×100 (ContentHeight 400),
// прокрученная на scrollY; в содержимом — кнопка на y=200..230 (при scrollY=200
// она на экране в 0..30).
func frameScene(t *testing.T, scrollY int) (root *widget.Panel, sv *widget.ScrollView, btn *widget.Button) {
	t.Helper()
	root = widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 400))
	sv = widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 100))
	sv.ContentHeight = 400
	root.AddChild(sv)
	btn = widget.NewButton("B")
	btn.SetBounds(image.Rect(10, 200, 200, 230))
	sv.AddChild(btn)
	sv.SetScrollY(scrollY)
	if sv.ScrollY() != scrollY {
		t.Fatalf("ScrollY = %d, ждали %d", sv.ScrollY(), scrollY)
	}
	return root, sv, btn
}

// Подсказка берётся у кнопки, которую видно под курсором.
func TestTooltipAt_InScroll(t *testing.T) {
	testreset.Globals(t)
	root, sv, btn := frameScene(t, 200)
	btn.SetToolTip("видимая")
	other := widget.NewButton("A")
	other.SetBounds(image.Rect(10, 0, 200, 30))
	other.SetToolTip("прокрученная за верх")
	sv.AddChild(other)

	if got := tooltipAt(root, 100, 15); got != "видимая" {
		t.Fatalf("подсказка %q, ждали \"видимая\"", got)
	}
}

// Попап, вынесенный в окно ОС, стоит на экране, а оверлей рисуется в координатах
// поля: прямоугольник — экранный, содержимое — то же, что рисует холст.
func TestPopupItems_ScreenRectInScroll(t *testing.T) {
	testreset.Globals(t)
	e := New(300, 400, 20)
	e.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 400))
	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 200))
	sv.ContentHeight = 600
	root.AddChild(sv)
	dd := widget.NewDropdown("Alpha", "Beta", "Gamma")
	dd.SetBounds(image.Rect(40, 340, 200, 370))
	sv.AddChild(dd)
	e.SetRoot(root)
	sv.SetScrollY(250) // поле на экране 90..120, список 120..210
	dd.SetOpen(true)

	items := collectPopups(nil, root, 0)
	if len(items) != 1 {
		t.Fatalf("попапов %d, ждали 1", len(items))
	}
	want := dd.OverlayBounds().Sub(image.Pt(0, 250))
	if items[0].rect != want {
		t.Fatalf("прямоугольник попапа %v, ждали экранный %v", items[0].rect, want)
	}

	// Картинка попапа совпадает с тем, что холст рисует на этом месте.
	c := e.canvas
	c.BlitBackground()
	drawOverlays(root, c, false)
	shifted := renderOverlayShifted(c.CloneForSize(want.Dx(), want.Dy(), c.Scale(), nil), dd, want, items[0].off)
	for y := 0; y < want.Dy(); y++ {
		for x := 0; x < want.Dx(); x++ {
			if in, out := c.Back().RGBAAt(want.Min.X+x, want.Min.Y+y), shifted.RGBAAt(x, y); in != out {
				t.Fatalf("пиксель (%d,%d): холст=%v попап=%v", x, y, in, out)
			}
		}
	}
}

// Виджет вне дерева не ломает поиск кадра: его координаты считаются экранными.
func TestFrameOffsetOf_NotInTree(t *testing.T) {
	testreset.Globals(t)
	e := New(100, 100, 20)
	root, _, _ := frameScene(t, 200)
	e.SetRoot(root)
	stray := widget.NewButton("вне дерева")
	if got := e.in.FrameOffsetOf(stray); got != (image.Point{}) {
		t.Fatalf("кадр виджета вне дерева %v, ждали нулевой", got)
	}
	if got := e.in.FrameOffsetOf(nil); got != (image.Point{}) {
		t.Fatalf("кадр nil-виджета %v, ждали нулевой", got)
	}
}
