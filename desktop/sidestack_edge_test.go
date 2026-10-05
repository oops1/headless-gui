package desktop

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Список окон раскрывается вбок от боковой панели: у левой — вправо от кнопки,
// у правой — влево, по высоте напротив кнопки, целиком на экране. Раньше список
// был рассчитан на нижний край: на экране, где ряд не помещался правее панели,
// его сдвигало под саму панель.
func TestSideStack_ListOpensBesideBar(t *testing.T) {
	for _, edge := range []Edge{EdgeLeft, EdgeRight} {
		for _, screenW := range []int{960, 640} {
			g := sideScene(t, edge, false)
			g.prev.Screen = image.Rect(0, 0, screenW, 420)
			if edge == EdgeRight {
				// Панель у правого края меньшего экрана.
				g.bar.SetBounds(image.Rect(screenW-g.bar.Thickness(), 0, screenW, 420))
			}
			g.area.layout()
			btn := g.area.ButtonRect(3)
			clickAt(g.area, btn)
			if !g.prev.IsOpen() {
				t.Fatalf("край %v, экран %d: список не открылся", edge, screenW)
			}
			r := g.prev.Bounds()
			bar := g.bar.Bounds()
			if r.Overlaps(bar) {
				t.Errorf("край %v, экран %d: список %v налез на панель %v", edge, screenW, r, bar)
			}
			if edge == EdgeLeft && r.Min.X < btn.Max.X || edge == EdgeRight && r.Max.X > btn.Min.X {
				t.Errorf("край %v, экран %d: список %v не по ту сторону кнопки %v", edge, screenW, r, btn)
			}
			if !r.In(g.prev.Screen) {
				t.Errorf("край %v, экран %d: список %v вышел за экран", edge, screenW, r)
			}
			if cy := (btn.Min.Y + btn.Max.Y) / 2; cy < r.Min.Y || cy > r.Max.Y {
				t.Errorf("край %v, экран %d: список %v не напротив кнопки %v", edge, screenW, r, btn)
			}
			if screenW == 640 && !g.prev.listVertical(3) {
				t.Errorf("край %v: три миниатюры не помещаются рядом с панелью, а список рядом", edge)
			}
		}
	}
}

// Метка открытого окна в столбце стоит на стороне ячейки, обращённой к краю
// экрана, а не снизу ячейки (посреди столбца, между соседями).
func TestSideStack_MarksOnScreenEdgeSide(t *testing.T) {
	for _, edge := range []Edge{EdgeLeft, EdgeRight} {
		g := sideScene(t, edge, false)
		g.area.layout()
		img := g.renderSize(960, 420)
		accent := g.tm.GetStyle(ComponentTaskButton, "", theme.StateActive).Border
		active := g.area.ButtonRect(2) // code: активное окно
		idle := g.area.ButtonRect(1)   // web: запущено
		none := g.area.ButtonRect(0)   // files: не запущено
		cy := func(r image.Rectangle) int { return r.Min.Y + r.Dy()/2 }
		side := func(r image.Rectangle) int {
			if edge == EdgeRight {
				return r.Max.X - 1
			}
			return r.Min.X
		}
		far := func(r image.Rectangle) int { // противоположная сторона
			if edge == EdgeRight {
				return r.Min.X
			}
			return r.Max.X - 1
		}
		if got := img.RGBAAt(side(active), cy(active)); got != accent {
			t.Errorf("край %v: у активного нет полосы со стороны края экрана: %v, ждали %v", edge, got, accent)
		}
		for _, y := range []int{active.Min.Y + 1, active.Max.Y - 2} {
			if got := img.RGBAAt(side(active), y); got != accent {
				t.Errorf("край %v: полоса активного не во всю высоту ячейки (y=%d): %v", edge, y, got)
			}
		}
		if got := img.RGBAAt(far(active), cy(active)); got == accent {
			t.Errorf("край %v: полоса активного и с противоположной стороны", edge)
		}
		// Снизу ячейки полосы нет: там, где её рисовала нижняя панель.
		if got := img.RGBAAt(active.Min.X+active.Dx()/2, active.Max.Y-1); got == accent {
			t.Errorf("край %v: полоса активного осталась снизу ячейки", edge)
		}
		if got := img.RGBAAt(side(none), cy(none)); got == accent {
			t.Errorf("край %v: у незапущенного есть полоса", edge)
		}
		// Запущенное неактивное: короткая метка по центру стороны, не до углов.
		light := func(c color.RGBA) bool { return c.R > 200 && c.G > 200 && c.B > 200 }
		if got := img.RGBAAt(side(idle), cy(idle)); !light(got) {
			t.Errorf("край %v: у запущенного нет метки на стороне края: %v", edge, got)
		}
		if got := img.RGBAAt(side(idle), idle.Min.Y+1); light(got) {
			t.Errorf("край %v: метка запущенного идёт до угла ячейки: %v", edge, got)
		}
	}
}

// Значок в ячейке столбца стоит по центру, а не прижат к левому краю: ячейка
// шире значка на всю толщину панели.
func TestSideStack_IconCentered(t *testing.T) {
	g := sideScene(t, EdgeLeft, false)
	g.area.layout()
	img := g.renderSize(960, 420)
	r := g.area.ButtonRect(0) // files: жёлтый значок
	minX, maxX := 1<<30, -1
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if c := img.RGBAAt(x, y); c.R > 200 && c.G > 150 && c.B < 80 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
			}
		}
	}
	if maxX < 0 {
		t.Fatal("значок не найден")
	}
	if left, right := minX-r.Min.X, r.Max.X-1-maxX; left-right > 1 || right-left > 1 {
		t.Errorf("значок не по центру ячейки: слева %d, справа %d", left, right)
	}
}

// Полоса кнопок окон (RunningApplications) в столбце — столбец квадратов с
// метками у края экрана, а не ряд, втиснутый в квадрат панели.
func TestSideStack_RunningApplicationsColumn(t *testing.T) {
	tm := win10Fast(t)
	wm := NewFakeWindowModel(
		WindowInfo{ID: 1, AppID: "a", Title: "one", Icon: solidIcon(color.RGBA{R: 240, G: 180, B: 40, A: 255})},
		WindowInfo{ID: 2, AppID: "b", Title: "two", Active: true, Icon: solidIcon(color.RGBA{R: 40, G: 140, B: 230, A: 255})},
		WindowInfo{ID: 3, AppID: "c", Title: "three", Icon: solidIcon(color.RGBA{R: 60, G: 190, B: 120, A: 255})},
	)
	ra := NewRunningApplications(tm, wm)
	bar := NewTaskbar(tm)
	bar.SetEdge(EdgeLeft)
	bar.AddItem(SlotApps, ra)
	bar.SetBounds(image.Rect(0, 0, bar.Thickness(), 300))
	t.Cleanup(func() { bar.Close(); ra.Close() })

	if len(ra.btns) != 3 {
		t.Fatalf("кнопок %d, ждали 3", len(ra.btns))
	}
	for i, b := range ra.btns {
		if b.rect.Dx() != bar.Thickness() {
			t.Errorf("кнопка %d: ширина %d, ждали всю толщину панели %d", i, b.rect.Dx(), bar.Thickness())
		}
		if i > 0 && b.rect.Min.Y < ra.btns[i-1].rect.Max.Y {
			t.Errorf("кнопка %d наезжает на предыдущую: %v и %v", i, b.rect, ra.btns[i-1].rect)
		}
		if b.showLabel {
			t.Errorf("кнопка %d с подписью в столбце", i)
		}
	}
	if ra.btns[1].rect.Min.Y <= ra.btns[0].rect.Min.Y {
		t.Error("кнопки идут не сверху вниз")
	}

	root := widget.NewPanel(color.RGBA{R: 31, G: 31, B: 31, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 300))
	root.AddChild(bar)
	eng := engine.New(200, 300, 30)
	eng.SetRoot(root)
	img := eng.RenderOnce()
	accent := tm.GetStyle(ComponentTaskButton, "", theme.StateActive).Border
	act := ra.btns[1].rect
	if got := img.RGBAAt(act.Min.X, act.Min.Y+act.Dy()/2); got != accent {
		t.Errorf("метка активного окна не на стороне края: %v, ждали %v", got, accent)
	}
	if got := img.RGBAAt(act.Min.X+act.Dx()/2, act.Max.Y-1); got == accent {
		t.Error("метка активного окна осталась снизу кнопки")
	}
}

// Меню команд кнопки у боковой панели раскрывается вбок от неё, а не над
// кнопкой поверх соседних.
func TestSideStack_CommandMenuOpensBeside(t *testing.T) {
	for _, edge := range []Edge{EdgeLeft, EdgeRight} {
		g := sideScene(t, edge, false)
		g.area.layout()
		eng := engine.New(960, 420, 30)
		eng.SetRoot(g.root)
		eng.RenderOnce()
		btn := g.area.ButtonRect(1)
		g.area.ShowCommands(1)
		m := g.area.menu.overlayBounds()
		if m.Empty() {
			t.Fatalf("край %v: меню не открылось", edge)
		}
		if edge == EdgeLeft && m.Min.X < btn.Max.X || edge == EdgeRight && m.Max.X > btn.Min.X {
			t.Errorf("край %v: меню %v не по ту сторону кнопки %v", edge, m, btn)
		}
		if m.Min.Y < btn.Min.Y-1 && m.Max.Y > btn.Max.Y {
			t.Errorf("край %v: меню %v не у кнопки %v", edge, m, btn)
		}
	}
}
