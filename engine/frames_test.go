package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// frames_test.go — перевод координат на границе «экран ↔ содержимое прокрутки»
// (frames.go) на уровне внутренних функций движка. Сквозные проверки через
// SendMouse* и кадр — в tests/scrollview_coords_test.go.

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

func TestHitPath_ShiftsFramesThroughScroll(t *testing.T) {
	root, sv, btn := frameScene(t, 200)

	path := hitPath(root, 100, 15)
	if len(path) != 3 {
		t.Fatalf("длина пути %d, ждали 3 (корень, прокрутка, кнопка): %v", len(path), path)
	}
	if path[0].w != widget.Widget(root) || path[1].w != widget.Widget(sv) || path[2].w != widget.Widget(btn) {
		t.Fatalf("состав пути неверен: %v", path)
	}
	// Кадр корня и самой прокрутки — экранный; кадр кнопки сдвинут на прокрутку.
	if path[0].off != (image.Point{}) || path[1].off != (image.Point{}) {
		t.Errorf("кадры корня и прокрутки %v, %v — ждали нулевые", path[0].off, path[1].off)
	}
	if want := image.Pt(0, 200); path[2].off != want {
		t.Errorf("кадр кнопки %v, ждали %v", path[2].off, want)
	}
}

// Точка вне прокрутки не задевает её содержимое, даже если в координатах
// содержимого там что-то лежит.
func TestHitPath_OutsideScrollMissesContent(t *testing.T) {
	root, _, _ := frameScene(t, 200)
	path := hitPath(root, 100, 215) // под прокруткой (0..100); в содержимом там кнопка
	if len(path) != 1 {
		t.Fatalf("путь %v, ждали только корень", path)
	}
}

// Вложенная прокрутка: кадры складываются.
func TestHitPath_NestedFramesAdd(t *testing.T) {
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 400))
	outer := widget.NewScrollView()
	outer.SetBounds(image.Rect(0, 0, 300, 150))
	outer.ContentHeight = 600
	root.AddChild(outer)
	inner := widget.NewScrollView()
	inner.SetBounds(image.Rect(10, 300, 280, 400))
	inner.ContentHeight = 220
	outer.AddChild(inner)
	btn := widget.NewButton("N")
	btn.SetBounds(image.Rect(20, 450, 200, 480))
	inner.AddChild(btn)
	outer.SetScrollY(300)
	inner.SetScrollY(120)

	path := hitPath(root, 100, 45) // кнопка: 450-120-300 = 30..60 на экране
	if len(path) != 4 || path[3].w != widget.Widget(btn) {
		t.Fatalf("путь %v, ждали корень, обе прокрутки и кнопку", path)
	}
	if want := image.Pt(0, 420); path[3].off != want {
		t.Errorf("кадр кнопки %v, ждали %v (300 внешней + 120 внутренней)", path[3].off, want)
	}
	if want := image.Pt(0, 300); path[2].off != want {
		t.Errorf("кадр внутренней прокрутки %v, ждали %v", path[2].off, want)
	}
	if got := e2eFrameOf(root, btn); got != path[3].off {
		t.Errorf("frameOffsetIn %v расходится с путём hit-теста %v", got, path[3].off)
	}
}

func e2eFrameOf(root, w widget.Widget) image.Point {
	off, _ := frameOffsetIn(root, w, image.Point{}, 0)
	return off
}

// Подсказка берётся у кнопки, которую видно под курсором.
func TestTooltipAt_InScroll(t *testing.T) {
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

// dismissSpy запоминает точку, с которой его попросили закрыться.
type dismissSpy struct {
	widget.Base
	got   []image.Point
	calls int
}

func (d *dismissSpy) Draw(widget.DrawContext) {}
func (d *dismissSpy) DismissAt(x, y int) {
	d.calls++
	d.got = append(d.got, image.Pt(x, y))
}

// DismissAt получает точку в кадре виджета: виджет, сверяющий её со своими
// Bounds, внутри прокрутки иначе ошибался бы на величину прокрутки.
func TestDismissOutside_PointInWidgetFrame(t *testing.T) {
	root, sv, _ := frameScene(t, 200)
	spy := &dismissSpy{}
	spy.SetBounds(image.Rect(0, 250, 50, 280))
	sv.AddChild(spy)
	outside := &dismissSpy{}
	outside.SetBounds(image.Rect(0, 300, 50, 330))
	root.AddChild(outside)

	dismissOutside(root, nil, 40, 15)

	if spy.calls != 1 || spy.got[0] != image.Pt(40, 215) {
		t.Errorf("виджет в прокрутке получил %v, ждали [(40,215)]", spy.got)
	}
	if outside.calls != 1 || outside.got[0] != image.Pt(40, 15) {
		t.Errorf("виджет вне прокрутки получил %v, ждали [(40,15)]", outside.got)
	}
}

// Оверлей виджета внутри прокрутки находится по экранной точке; а площадь
// самого виджета, обрезанная клипом, оверлей не «ловит».
func TestFindOverlayStep_InScroll(t *testing.T) {
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 400))
	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 100))
	sv.ContentHeight = 600
	root.AddChild(sv)
	// Выпадающий список у поля на y=300..330 в содержимом; сам список лежит под
	// полем, на 330..420.
	dd := widget.NewDropdown("a", "b", "c")
	dd.SetBounds(image.Rect(10, 300, 200, 330))
	sv.AddChild(dd)
	sv.SetScrollY(260) // поле на экране 40..70, список 70..160
	dd.SetOpen(true)

	// Точка на поле, видимом в прокрутке.
	w, off := findOverlayStep(root, 50, 55)
	if w != widget.Widget(dd) || off != (image.Point{Y: 260}) {
		t.Fatalf("на поле: нашли %v с кадром %v", w, off)
	}
	// Точка на списке за нижним краем прокрутки (экранные 100..160).
	if w, _ := findOverlayStep(root, 50, 120); w != widget.Widget(dd) {
		t.Fatalf("на открытом списке вне прокрутки оверлей не найден: %v", w)
	}
	// Сам виджет стоит в содержимом на 300..330, то есть на экране 40..70, и под
	// прокруткой, в экранных 300..330, его «собственной площади» нет.
	if w, _ := findOverlayStep(root, 50, 310); w != nil {
		t.Fatalf("оверлей найден по несуществующей экранной точке: %v", w)
	}
}

// Попап, вынесенный в окно ОС, стоит на экране, а оверлей рисуется в координатах
// поля: прямоугольник — экранный, содержимое — то же, что рисует холст.
func TestPopupItems_ScreenRectInScroll(t *testing.T) {
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
	c.blitBackground()
	drawOverlays(root, c, false)
	shifted := renderOverlayShifted(c.cloneForSize(want.Dx(), want.Dy(), c.scale, nil), dd, want, items[0].off)
	for y := 0; y < want.Dy(); y++ {
		for x := 0; x < want.Dx(); x++ {
			if in, out := c.back.RGBAAt(want.Min.X+x, want.Min.Y+y), shifted.RGBAAt(x, y); in != out {
				t.Fatalf("пиксель (%d,%d): холст=%v попап=%v", x, y, in, out)
			}
		}
	}
}

// capSpy — виджет, запоминающий координаты движения мыши.
type capSpy struct {
	widget.Base
	moves []image.Point
}

func (c *capSpy) Draw(widget.DrawContext) {}
func (c *capSpy) OnMouseMove(x, y int)    { c.moves = append(c.moves, image.Pt(x, y)) }

// Захватчик внутри прокрутки получает движение в своём кадре, и кадр следует за
// прокруткой, которая сдвинулась посреди перетаскивания.
func TestCapture_FrameFollowsScroll(t *testing.T) {
	e := New(300, 400, 20)
	e.SetTooltipsEnabled(false)
	root, sv, _ := frameScene(t, 100)
	spy := &capSpy{}
	spy.SetBounds(image.Rect(0, 150, 50, 180))
	sv.AddChild(spy)
	e.SetRoot(root)

	e.SetCapture(spy)
	e.SendMouseMove(20, 30)
	sv.SetScrollY(200) // колесо во время перетаскивания
	e.SendMouseMove(20, 30)

	want := []image.Point{{X: 20, Y: 130}, {X: 20, Y: 230}}
	if len(spy.moves) != 2 || spy.moves[0] != want[0] || spy.moves[1] != want[1] {
		t.Fatalf("захватчик получил %v, ждали %v", spy.moves, want)
	}

	// После снятия захвата цепочка сбрасывается: новый захват ищет её заново.
	e.ReleaseCapture()
	if e.capChainOK {
		t.Fatalf("цепочка кадра пережила снятие захвата")
	}
}

// Виджет вне дерева не ломает поиск кадра: его координаты считаются экранными.
func TestFrameOffsetOf_NotInTree(t *testing.T) {
	e := New(100, 100, 20)
	root, _, _ := frameScene(t, 200)
	e.SetRoot(root)
	stray := widget.NewButton("вне дерева")
	if got := e.frameOffsetOf(stray); got != (image.Point{}) {
		t.Fatalf("кадр виджета вне дерева %v, ждали нулевой", got)
	}
	if got := e.frameOffsetOf(nil); got != (image.Point{}) {
		t.Fatalf("кадр nil-виджета %v, ждали нулевой", got)
	}
}
