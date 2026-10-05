package desktop_test

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Элементы панели в столбце бокового края.

// Дата, которая не помещается по ширине столбца, не показывается: часы
// сводятся к одному времени, а не вылезают за панель.
func TestEdges_VerticalClockDropsWideDate(t *testing.T) {
	tm, _ := edgeManager(t, theme.ProfileWindows10)
	clk := desktop.NewClock(tm, desktop.NewFakeClock(time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC)))
	clk.SetVertical(true)
	wide := clk.PreferredSize(image.Pt(200, 400))
	narrow := clk.PreferredSize(image.Pt(30, 400))
	if narrow.Y >= wide.Y {
		t.Errorf("в узком столбце дата осталась: высота %d против %d", narrow.Y, wide.Y)
	}
	if wide.X != 200 || narrow.X != 30 {
		t.Errorf("часы должны занимать всю толщину: %v %v", wide, narrow)
	}
	// В ряду ничего не меняется: ширина по строке.
	clk.SetVertical(false)
	row := clk.PreferredSize(image.Pt(1000, 40))
	if row.X >= 200 {
		t.Errorf("в ряду часы раздулись: %v", row)
	}
}

// Значки трея в столбце идут сеткой, а что не поместилось, прячется за шеврон.
func TestEdges_VerticalTrayHidesWhatDoesNotFit(t *testing.T) {
	tm, _ := edgeManager(t, theme.ProfileWindows10)
	status := desktop.NewFakeSystemStatus()
	tray := desktop.NewSystemTray(tm)
	for i := 0; i < 5; i++ {
		tray.AddItem(desktop.NewNetworkStatus(tm, status))
	}
	tray.SetVertical(true)

	tray.SetBounds(image.Rect(0, 0, 62, 400))
	if len(tray.Hidden()) != 0 {
		t.Errorf("при месте для всех спрятано %d", len(tray.Hidden()))
	}
	for i, it := range tray.Items() {
		if b := it.Bounds(); b.Empty() || !b.In(tray.Bounds()) {
			t.Errorf("значок %d: %v", i, b)
		}
	}
	// Сетка в несколько колонок, когда ширина позволяет: нижний значок выше,
	// чем если бы все лежали в одну колонку.
	last := len(tray.Items()) - 1
	wideY := tray.Items()[last].Bounds().Min.Y
	tray.SetBounds(image.Rect(0, 0, 24, 400))
	narrowY := tray.Items()[last].Bounds().Min.Y
	if wideY >= narrowY {
		t.Errorf("в широком столбце значки не легли сеткой: %d против %d", wideY, narrowY)
	}

	tray.SetBounds(image.Rect(0, 0, 24, 60))
	if len(tray.Hidden()) == 0 {
		t.Error("в тесном столбце ничего не спрятано")
	}
	for _, it := range tray.Hidden() {
		if !it.Bounds().Empty() {
			t.Errorf("спрятанный значок имеет границы %v", it.Bounds())
		}
	}
}

// Область, занятая панелью у бокового края, — её столбец.
func TestEdges_ReservedAreaOfSideBar(t *testing.T) {
	_, _, s := oneScreen(t, theme.ProfileWindows10, desktop.EdgeLeft, 900, 600)
	if got := s.bar.ReservedArea(); got != s.bar.Bounds() || got.Dx() != s.bar.Thickness() {
		t.Errorf("зарезервировано %v", got)
	}
}

// Элемент, не умеющий лежать в столбце, получает квадрат в толщину панели и не
// вылезает за неё.
type rowOnlyItem struct {
	widget.Base
	want image.Point
}

func newRowOnly(want image.Point) *rowOnlyItem { return &rowOnlyItem{want: want} }

func (r *rowOnlyItem) PreferredSize(image.Point) image.Point { return r.want }
func (r *rowOnlyItem) Draw(widget.DrawContext)               {}

func TestEdges_RowOnlyItemGetsSquare(t *testing.T) {
	tm, _ := edgeManager(t, theme.ProfileWindows10)
	bar := desktop.NewTaskbar(tm)
	defer bar.Close()
	it := newRowOnly(image.Pt(300, 40))
	bar.AddItem(desktop.SlotStart, it)
	bar.SetEdge(desktop.EdgeLeft)
	bar.SetBounds(image.Rect(0, 0, 62, 500))
	if b := it.Bounds(); b.Dx() != 62 || b.Dy() != 40 || b.Min.Y != 0 {
		t.Errorf("элемент ряда в столбце: %v, ждали 62×40 сверху (ширина — вся толщина)", b)
	}
	// Просит больше стороны — получает сторону: из столбца не вылезает.
	it2 := newRowOnly(image.Pt(300, 300))
	bar.SetItems(desktop.SlotStart, it2)
	bar.SetBounds(image.Rect(0, 0, 62, 500))
	if b := it2.Bounds(); b.Dx() != 62 || b.Dy() != 62 {
		t.Errorf("слишком большой элемент: %v, ждали 62×62", b)
	}
}
