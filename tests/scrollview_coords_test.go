package tests

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Дети прокрутки обязаны вести себя так, будто стоят там, где их видно.
//
// ScrollView рисует содержимое через транслирующую обёртку контекста (PERF-12),
// а Bounds() детей остаются в координатах СОДЕРЖИМОГО. Всё остальное, что
// смотрело на эти Bounds, видело несдвинутую картину: щелчок попадал в
// невидимую кнопку, прокрученную за верх, подсветка загоралась не у той
// кнопки, перерисовывалось не то место экрана, а меню поля ввода открывалось
// не у курсора.
//
// Каждый тест здесь идёт через настоящий движок: хит-тест, доставка событий,
// кадр. Проверять вызовы виджетов по отдельности нельзя — ошибка жила именно
// на стыке движка и прокрутки.

// svKid — кнопка с учётом нажатий.
type svKid struct {
	btn    *widget.Button
	clicks int
}

func newSvKid(name string, r image.Rectangle) *svKid {
	k := &svKid{btn: widget.NewButton(name)}
	k.btn.SetBounds(r)
	k.btn.AddClickHandler(func() { k.clicks++ })
	return k
}

// svCoordsEnv — движок, корень и прокрутка со сценой из описания дефекта.
type svCoordsEnv struct {
	eng        *engine.Engine
	root       *widget.Panel
	sv         *widget.ScrollView
	a, b, c, d *svKid
}

// newVScrollScene: прокрутка высотой 100, ContentHeight=400, внутри кнопки
// A (y=0), B (y=40), C (y=200), D (y=240) высотой по 30. Холст выше прокрутки,
// чтобы несдвинутые координаты содержимого тоже оказывались на холсте — иначе
// ошибка маскировалась бы выходом за экран.
func newVScrollScene(t *testing.T) *svCoordsEnv {
	t.Helper()
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 500))

	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 100))
	sv.ContentHeight = 400
	root.AddChild(sv)

	env := &svCoordsEnv{root: root, sv: sv}
	env.a = newSvKid("A", image.Rect(10, 0, 200, 30))
	env.b = newSvKid("B", image.Rect(10, 40, 200, 70))
	env.c = newSvKid("C", image.Rect(10, 200, 200, 230))
	env.d = newSvKid("D", image.Rect(10, 240, 200, 270))
	for _, k := range []*svKid{env.a, env.b, env.c, env.d} {
		sv.AddChild(k.btn)
	}

	env.eng = engine.New(300, 500, 30)
	env.eng.SetRoot(root)
	env.eng.RenderOnce()
	return env
}

func (e *svCoordsEnv) click(x, y int) {
	e.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	e.eng.SendMouseButton(x, y, widget.MouseLeft, false)
}

func clicksOf(ks ...*svKid) []int {
	out := make([]int, len(ks))
	for i, k := range ks {
		out[i] = k.clicks
	}
	return out
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Без прокрутки всё по-старому: ориентир, что сцена собрана верно.
func TestScrollViewCoords_ClickUnscrolled(t *testing.T) {
	env := newVScrollScene(t)
	env.click(100, 15)
	env.click(100, 55)
	if got := clicksOf(env.a, env.b, env.c, env.d); !sameInts(got, []int{1, 1, 0, 0}) {
		t.Fatalf("клики A,B,C,D = %v, ждали [1 1 0 0]", got)
	}
}

// Дефект из отчёта: после SetScrollY(200) на экране видны C и D. Щелчок в
// (100,15) нажимал A — невидимую, прокрученную за верх.
func TestScrollViewCoords_ClickVertical(t *testing.T) {
	env := newVScrollScene(t)
	env.sv.SetScrollY(200)
	env.eng.RenderOnce()

	env.click(100, 15) // экранное место C
	if got := clicksOf(env.a, env.b, env.c, env.d); !sameInts(got, []int{0, 0, 1, 0}) {
		t.Fatalf("после щелчка на месте C клики A,B,C,D = %v, ждали [0 0 1 0]", got)
	}
	env.click(100, 55) // экранное место D
	if got := clicksOf(env.a, env.b, env.c, env.d); !sameInts(got, []int{0, 0, 1, 1}) {
		t.Fatalf("после щелчка на месте D клики A,B,C,D = %v, ждали [0 0 1 1]", got)
	}
	// Пустое место между кнопками и под ними не нажимает никого.
	env.click(100, 36)
	env.click(100, 90)
	if got := clicksOf(env.a, env.b, env.c, env.d); !sameInts(got, []int{0, 0, 1, 1}) {
		t.Fatalf("щелчок мимо кнопок что-то нажал: %v", got)
	}
}

// Ниже прокрутки лежит экранное место, куда несдвинутые координаты B попали
// бы мимо прокрутки: щелчок вне прокрутки её содержимого не касается вовсе.
func TestScrollViewCoords_ClickOutsideViewIgnoresContent(t *testing.T) {
	env := newVScrollScene(t)
	env.sv.SetScrollY(200)
	env.eng.RenderOnce()

	// Экранная y=215 лежит под прокруткой, а в координатах содержимого там
	// стоит кнопка C (200..230). Её нажимать нельзя: C с экрана не видна.
	env.click(100, 215)
	if got := clicksOf(env.a, env.b, env.c, env.d); !sameInts(got, []int{0, 0, 0, 0}) {
		t.Fatalf("щелчок под прокруткой нажал кнопку из её содержимого: %v", got)
	}
}

// Горизонтальная прокрутка: те же правила для scrollX.
func TestScrollViewCoords_ClickHorizontal(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 600, 200))

	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 100))
	sv.ContentWidth = 600
	root.AddChild(sv)

	// Три кнопки в ряд: 0..80, 300..380, 500..580.
	a := newSvKid("A", image.Rect(0, 10, 80, 40))
	b := newSvKid("B", image.Rect(300, 10, 380, 40))
	c := newSvKid("C", image.Rect(500, 10, 580, 40))
	for _, k := range []*svKid{a, b, c} {
		sv.AddChild(k.btn)
	}
	eng := engine.New(600, 200, 30)
	eng.SetRoot(root)
	eng.RenderOnce()

	sv.SetScrollX(300)
	if sv.ScrollX() != 300 {
		t.Fatalf("ScrollX = %d, ждали 300", sv.ScrollX())
	}
	eng.RenderOnce()

	// B теперь на экране в 0..80: щелчок в (40,25) нажимает её, а не A.
	eng.SendMouseButton(40, 25, widget.MouseLeft, true)
	eng.SendMouseButton(40, 25, widget.MouseLeft, false)
	if got := clicksOf(a, b, c); !sameInts(got, []int{0, 1, 0}) {
		t.Fatalf("клики A,B,C = %v, ждали [0 1 0]", got)
	}
	// C — в 200..280, за правым краем прокрутки (ширина 200): не видна.
	eng.SendMouseButton(240, 25, widget.MouseLeft, true)
	eng.SendMouseButton(240, 25, widget.MouseLeft, false)
	if got := clicksOf(a, b, c); !sameInts(got, []int{0, 1, 0}) {
		t.Fatalf("щелчок за краем прокрутки что-то нажал: %v", got)
	}
}

// Подсветка загорается у той кнопки, что под курсором.
func TestScrollViewCoords_Hover(t *testing.T) {
	env := newVScrollScene(t)
	env.sv.SetScrollY(200)
	env.eng.RenderOnce()

	env.eng.SendMouseMove(100, 15)
	if !env.c.btn.IsHovered() || env.a.btn.IsHovered() {
		t.Fatalf("над экранным местом C: hover C=%v A=%v, ждали true/false",
			env.c.btn.IsHovered(), env.a.btn.IsHovered())
	}
	env.eng.SendMouseMove(100, 55)
	if !env.d.btn.IsHovered() || env.c.btn.IsHovered() || env.b.btn.IsHovered() {
		t.Fatalf("над экранным местом D: hover D=%v C=%v B=%v, ждали true/false/false",
			env.d.btn.IsHovered(), env.c.btn.IsHovered(), env.b.btn.IsHovered())
	}
	// Курсор ушёл под прокрутку: кнопка, чьи координаты содержимого там
	// лежат, невидима и подсвечиваться не должна, а прежняя подсветка гаснет.
	env.eng.SendMouseMove(100, 215)
	if env.c.btn.IsHovered() || env.d.btn.IsHovered() {
		t.Fatalf("курсор под прокруткой, а кнопки подсвечены: C=%v D=%v",
			env.c.btn.IsHovered(), env.d.btn.IsHovered())
	}
}

// Курсор над невидимой частью содержимого (прокрутка кончилась выше него) не
// должен подсвечивать то, что под ним лежит в координатах содержимого.
func TestScrollViewCoords_HoverBelowViewIgnoresContent(t *testing.T) {
	env := newVScrollScene(t)
	e := newSvKid("E", image.Rect(10, 300, 200, 330))
	env.sv.AddChild(e.btn)
	env.sv.SetScrollY(200)
	env.eng.RenderOnce()

	// Экранная y=115 ниже прокрутки (0..100); в координатах содержимого это 315,
	// то есть кнопка E. Она обрезана клипом и с экрана не видна.
	env.eng.SendMouseMove(100, 115)
	if e.btn.IsHovered() {
		t.Fatalf("E подсвечена курсором, который стоит вне прокрутки")
	}
}

// pixelsEqual сравнивает два кадра целиком и возвращает первую разницу.
func firstDiff(a, b *image.RGBA) (image.Point, bool) {
	if a.Rect != b.Rect {
		return image.Point{}, true
	}
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return image.Pt(x, y), true
			}
		}
	}
	return image.Point{}, false
}

// Инвалидация: ребёнок прокрутки сообщает о перерисовке своих координат
// содержимого, а перерисоваться должно место, где его видно. Проверяем
// пикселями: кадр, полученный частичной перерисовкой, обязан совпасть с кадром,
// нарисованным целиком.
func TestScrollViewCoords_InvalidateRedrawsScreenPlace(t *testing.T) {
	env := newVScrollScene(t)
	env.eng.SetRenderOnDemand(true)
	env.sv.SetScrollY(200)
	env.eng.RenderOnce() // сдвиг доехал до кадра

	before := env.eng.RenderOnce()

	env.c.btn.SetHovered(true) // меняет вид кнопки, которая на экране в 0..30
	partial := env.eng.RenderOnce()

	// Кнопка действительно изменилась на экране — иначе тест ничего не проверял.
	probe := image.Pt(20, 5)
	if before.RGBAAt(probe.X, probe.Y) == partial.RGBAAt(probe.X, probe.Y) {
		// Допустим, подсветка не затронула эту точку: ищем любое отличие в её полосе.
		changed := false
		for y := 0; y < 30 && !changed; y++ {
			for x := 10; x < 200; x++ {
				if before.RGBAAt(x, y) != partial.RGBAAt(x, y) {
					changed = true
					break
				}
			}
		}
		if !changed {
			t.Fatalf("после SetHovered(true) на месте кнопки C ничего не перерисовалось")
		}
	}

	env.eng.Invalidate()
	full := env.eng.RenderOnce()
	if p, bad := firstDiff(partial, full); bad {
		t.Fatalf("частичная перерисовка разошлась с полной в точке %v: %v против %v",
			p, partial.RGBAAt(p.X, p.Y), full.RGBAAt(p.X, p.Y))
	}
}

// Контекстное меню поля ввода внутри прокрутки открывается у курсора и
// принимает щелчки по пунктам.
func TestScrollViewCoords_TextBoxContextMenuAtCursor(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 300))

	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 100))
	sv.ContentHeight = 500
	root.AddChild(sv)

	tb := widget.NewTextBox("")
	tb.SetText("hello")
	tb.SetBounds(image.Rect(10, 200, 250, 230))
	sv.AddChild(tb)

	eng := engine.New(300, 300, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	sv.SetScrollY(200)
	before := eng.RenderOnce()

	// Правая кнопка по экранному месту поля (оно в 0..30).
	eng.SendMouseButton(100, 15, widget.MouseRight, true)
	eng.SendMouseButton(100, 15, widget.MouseRight, false)

	if !tb.HasOverlay() {
		t.Fatalf("контекстное меню не открылось")
	}
	img := eng.RenderOnce()

	// Меню нарисовано у курсора: ниже точки (100,15), в пределах прокрутки
	// высотой 100 его верхняя часть видна. Раньше оно уезжало на y=215+ (в
	// координаты содержимого) либо прижималось к низу холста.
	changed := 0
	for y := 18; y < 90; y++ {
		for x := 104; x < 150; x++ {
			if img.RGBAAt(x, y) != before.RGBAAt(x, y) {
				changed++
			}
		}
	}
	if changed < 100 {
		t.Fatalf("у курсора (100,15) меню не нарисовано: изменилось всего %d точек", changed)
	}
	// Тем более нет «призрака» там, куда меню попало бы без сдвига.
	ghost := 0
	for y := 215; y < 290; y++ {
		for x := 104; x < 150; x++ {
			if img.RGBAAt(x, y) != before.RGBAAt(x, y) {
				ghost++
			}
		}
	}
	if ghost != 0 {
		t.Fatalf("меню нарисовано ещё и в координатах содержимого: %d точек", ghost)
	}

	// Пункт «Select All» — последний; щёлкаем по нему на ЭКРАНЕ.
	ob := tb.OverlayBounds().Sub(image.Pt(0, sv.ScrollY()))
	eng.SendMouseButton(ob.Min.X+10, ob.Max.Y-8, widget.MouseLeft, true)
	eng.SendMouseButton(ob.Min.X+10, ob.Max.Y-8, widget.MouseLeft, false)
	if got := tb.SelectedText(); got != "hello" {
		t.Fatalf("после щелчка по «Select All» выделено %q, ждали \"hello\"", got)
	}
}

// Каретка для редактора метода ввода (IME) — в экранных координатах.
func TestScrollViewCoords_IMECaretRect(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 300))
	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 100))
	sv.ContentHeight = 500
	root.AddChild(sv)
	tb := widget.NewTextBox("")
	tb.SetText("hello")
	tb.SetBounds(image.Rect(10, 200, 250, 230))
	sv.AddChild(tb)

	eng := engine.New(300, 300, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	sv.SetScrollY(200)
	eng.RenderOnce()
	eng.SetFocus(tb)

	r, ok := eng.CaretRect()
	if !ok {
		t.Fatalf("место каретки неизвестно")
	}
	if r.Min.Y < 0 || r.Max.Y > 30 {
		t.Fatalf("каретка %v вне экранной полосы поля (0..30)", r)
	}
}

// Скринридер видит границы там, где элемент на экране.
func TestScrollViewCoords_AccessibilityBounds(t *testing.T) {
	env := newVScrollScene(t)
	env.sv.SetScrollY(200)
	env.eng.RenderOnce()

	tree := env.eng.AccessibilityTree()
	var find func(n *widget.AccessNode) *widget.AccessNode
	find = func(n *widget.AccessNode) *widget.AccessNode {
		if n.Widget == widget.Widget(env.c.btn) {
			return n
		}
		for _, ch := range n.Children {
			if r := find(ch); r != nil {
				return r
			}
		}
		return nil
	}
	n := find(tree)
	if n == nil {
		t.Fatalf("кнопка C не найдена в дереве доступности")
	}
	if want := image.Rect(10, 0, 200, 30); n.Bounds != want {
		t.Fatalf("границы C для скринридера %v, ждали %v", n.Bounds, want)
	}
}

// Перетаскивание с захватом мыши: выделение в поле внутри прокрутки идёт
// от того же места, что и без прокрутки.
func TestScrollViewCoords_CaptureDrag(t *testing.T) {
	selected := func(scrollY int) string {
		root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, 300, 300))
		sv := widget.NewScrollView()
		sv.SetBounds(image.Rect(0, 0, 300, 100))
		sv.ContentHeight = 500
		root.AddChild(sv)
		tb := widget.NewTextBox("")
		tb.SetText("abcdefghijklmnop")
		tb.SetBounds(image.Rect(10, scrollY, 250, scrollY+30))
		sv.AddChild(tb)
		eng := engine.New(300, 300, 30)
		eng.SetRoot(root)
		eng.RenderOnce()
		sv.SetScrollY(scrollY)
		eng.RenderOnce()

		// Нажатие у левого края текста, затем тащим вправо (мышь захвачена).
		eng.SendMouseButton(14, 15, widget.MouseLeft, true)
		eng.SendMouseMove(60, 15)
		eng.SendMouseMove(80, 16)
		eng.SendMouseButton(80, 16, widget.MouseLeft, false)
		return tb.SelectedText()
	}
	ref := selected(0)
	if ref == "" {
		t.Fatalf("эталон без прокрутки ничего не выделил")
	}
	if got := selected(200); got != ref {
		t.Fatalf("с прокруткой выделено %q, без неё %q", got, ref)
	}
}

// Выпадающий список внутри прокрутки: раскрывается под полем на экране и
// принимает выбор пункта.
func TestScrollViewCoords_DropdownList(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 400))
	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 200))
	sv.ContentHeight = 600
	root.AddChild(sv)
	dd := widget.NewDropdown("один", "два", "три")
	dd.SetBounds(image.Rect(10, 300, 200, 330))
	sv.AddChild(dd)

	eng := engine.New(300, 400, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	sv.SetScrollY(250) // поле на экране: 50..80
	eng.RenderOnce()

	eng.SendMouseButton(100, 65, widget.MouseLeft, true)
	eng.SendMouseButton(100, 65, widget.MouseLeft, false)
	if !dd.HasOverlay() {
		t.Fatalf("список не раскрылся")
	}
	// Список под полем: пункты 80..110, 110..140, 140..170. Щёлкаем по второму.
	eng.RenderOnce()
	eng.SendMouseButton(100, 125, widget.MouseLeft, true)
	eng.SendMouseButton(100, 125, widget.MouseLeft, false)
	if got := dd.Selected(); got != 1 {
		t.Fatalf("выбран пункт %d, ждали 1", got)
	}
}

// Вложенная прокрутка: прокрутка в прокрутке.
type nestedEnv struct {
	eng      *engine.Engine
	outer    *widget.ScrollView
	inner    *widget.ScrollView
	n1, n2   *svKid
	n3, n4   *svKid
	outerTop *svKid
}

// Внешняя 300×150 (ContentHeight 600) прокручена на 300: внутренняя, стоящая
// в содержимом на 300..400, видна на экране в 0..100. Внутренняя прокручена на
// 120: её кнопки N1 (300), N2 (340), N3 (450), N4 (490) показаны на экране
// в -120.., то есть видны только N3 (30..60) и N4 (70..100).
func newNestedScene(t *testing.T) *nestedEnv {
	t.Helper()
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 400))

	outer := widget.NewScrollView()
	outer.SetBounds(image.Rect(0, 0, 300, 150))
	outer.ContentHeight = 600
	root.AddChild(outer)

	env := &nestedEnv{outer: outer}
	env.outerTop = newSvKid("T", image.Rect(10, 0, 200, 30))
	outer.AddChild(env.outerTop.btn)

	inner := widget.NewScrollView()
	inner.SetBounds(image.Rect(10, 300, 280, 400))
	inner.ContentHeight = 220
	outer.AddChild(inner)
	env.inner = inner

	env.n1 = newSvKid("N1", image.Rect(20, 300, 200, 330))
	env.n2 = newSvKid("N2", image.Rect(20, 340, 200, 370))
	env.n3 = newSvKid("N3", image.Rect(20, 450, 200, 480))
	env.n4 = newSvKid("N4", image.Rect(20, 490, 200, 520))
	for _, k := range []*svKid{env.n1, env.n2, env.n3, env.n4} {
		inner.AddChild(k.btn)
	}

	env.eng = engine.New(300, 400, 30)
	env.eng.SetRoot(root)
	env.eng.RenderOnce()
	outer.SetScrollY(300)
	inner.SetScrollY(120)
	if outer.ScrollY() != 300 || inner.ScrollY() != 120 {
		t.Fatalf("смещения: внешняя %d, внутренняя %d, ждали 300 и 120", outer.ScrollY(), inner.ScrollY())
	}
	env.eng.RenderOnce()
	return env
}

func TestScrollViewCoords_NestedClick(t *testing.T) {
	env := newNestedScene(t)
	click := func(x, y int) {
		env.eng.SendMouseButton(x, y, widget.MouseLeft, true)
		env.eng.SendMouseButton(x, y, widget.MouseLeft, false)
	}
	click(100, 45) // N3
	if got := clicksOf(env.n1, env.n2, env.n3, env.n4, env.outerTop); !sameInts(got, []int{0, 0, 1, 0, 0}) {
		t.Fatalf("клики N1..N4,T = %v, ждали [0 0 1 0 0]", got)
	}
	click(100, 85) // N4
	if got := clicksOf(env.n1, env.n2, env.n3, env.n4, env.outerTop); !sameInts(got, []int{0, 0, 1, 1, 0}) {
		t.Fatalf("клики N1..N4,T = %v, ждали [0 0 1 1 0]", got)
	}
}

func TestScrollViewCoords_NestedHover(t *testing.T) {
	env := newNestedScene(t)
	env.eng.SendMouseMove(100, 45)
	if !env.n3.btn.IsHovered() || env.n1.btn.IsHovered() || env.n2.btn.IsHovered() || env.n4.btn.IsHovered() {
		t.Fatalf("hover N1..N4 = %v %v %v %v, ждали только N3",
			env.n1.btn.IsHovered(), env.n2.btn.IsHovered(), env.n3.btn.IsHovered(), env.n4.btn.IsHovered())
	}
	env.eng.SendMouseMove(100, 85)
	if !env.n4.btn.IsHovered() || env.n3.btn.IsHovered() {
		t.Fatalf("hover N3=%v N4=%v, ждали false/true", env.n3.btn.IsHovered(), env.n4.btn.IsHovered())
	}
}

// Колесо над внутренней прокруткой листает ЕЁ, а не внешнюю.
func TestScrollViewCoords_NestedWheel(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()
	env := newNestedScene(t)
	outerBefore := env.outer.ScrollY()
	innerBefore := env.inner.ScrollY()
	env.eng.SendMouseWheelPixels(100, 50, 0, -30)
	// Плавная прокрутка идёт на часах движка: продвигаем их руками.
	stepInertia(time.Now(), 60)
	if env.outer.ScrollY() != outerBefore {
		t.Fatalf("колесо над внутренней прокруткой сдвинуло внешнюю: %d → %d",
			outerBefore, env.outer.ScrollY())
	}
	if got := env.inner.ScrollY(); got >= innerBefore {
		t.Fatalf("внутренняя прокрутка не приняла колесо: %d → %d", innerBefore, got)
	}
}

// Инвалидация во вложенной прокрутке: кадр из частичной перерисовки совпадает
// с полным.
func TestScrollViewCoords_NestedInvalidate(t *testing.T) {
	env := newNestedScene(t)
	env.eng.SetRenderOnDemand(true)
	env.eng.Invalidate()
	env.eng.RenderOnce()

	env.n3.btn.SetHovered(true)
	partial := env.eng.RenderOnce()
	env.eng.Invalidate()
	full := env.eng.RenderOnce()
	if p, bad := firstDiff(partial, full); bad {
		t.Fatalf("частичная перерисовка разошлась с полной в точке %v: %v против %v",
			p, partial.RGBAAt(p.X, p.Y), full.RGBAAt(p.X, p.Y))
	}
}
