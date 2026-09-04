package datagrid

import (
	"image"
	"image/color"
	"testing"
	"time"
)

// Запросы GG-25 (полосатость), GG-26 (картинки в шаблонной колонке и ScrollX),
// GG-27 (подсказка на строку).

// recorderCtx — контекст отрисовки, запоминающий то, что в нём рисовали.
type recorderCtx struct {
	nopDrawCtx
	fills  []color.RGBA
	images int
}

func (c *recorderCtx) FillRect(x, y, w, h int, col color.RGBA) {
	c.fills = append(c.fills, col)
}
func (c *recorderCtx) DrawImage(src image.Image, x, y int)             { c.images++ }
func (c *recorderCtx) DrawImageScaled(src image.Image, x, y, w, h int) { c.images++ }

func (c *recorderCtx) hasFill(want color.RGBA) bool {
	for _, f := range c.fills {
		if f == want {
			return true
		}
	}
	return false
}

type reqRow struct{ Name string }

func reqGrid(t *testing.T, rows int) *DataGrid {
	t.Helper()
	dg := New()
	dg.RowHeight = 20
	dg.HeaderHeight = 20
	c := NewTextColumn("Имя", "Name")
	c.SetActualWidth(100)
	dg.AddColumn(c)
	oc := NewObservableCollection()
	for i := 0; i < rows; i++ {
		oc.Add(&reqRow{Name: "строка"})
	}
	dg.SetItemsSource(oc)
	dg.SetBounds(image.Rect(0, 0, 200, 20+rows*20))
	return dg
}

// Полосы можно выключить, и смена темы их не возвращает.
func TestZebra_CanBeTurnedOff(t *testing.T) {
	dg := reqGrid(t, 4)
	stripe := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	dg.AlternateBG = stripe

	ctx := &recorderCtx{}
	dg.Draw(ctx)
	if !ctx.hasFill(stripe) {
		t.Fatal("полос нет с самого начала — проверять нечего")
	}

	dg.ZebraStripes = false
	ctx = &recorderCtx{}
	dg.Draw(ctx)
	if ctx.hasFill(stripe) {
		t.Error("полосы остались после ZebraStripes = false")
	}

	// Смена темы переписывает AlternateBG — но не решение приложения.
	dg.ApplyTheme(&DataGridTheme{
		Background:  color.RGBA{R: 10, G: 10, B: 10, A: 255},
		AlternateBG: color.RGBA{R: 20, G: 20, B: 20, A: 255},
	})
	ctx = &recorderCtx{}
	dg.Draw(ctx)
	if ctx.hasFill(color.RGBA{R: 20, G: 20, B: 20, A: 255}) {
		t.Error("смена темы вернула полосы, выключенные приложением")
	}
}

// Шаблонная колонка умеет рисовать картинки.
func TestTemplateColumn_CanDrawImages(t *testing.T) {
	dg := reqGrid(t, 2)
	icon := image.NewRGBA(image.Rect(0, 0, 16, 16))

	col := NewTemplateColumn("Значок", func(cdc CellDrawContext) {
		cdc.DrawCtx.DrawImageScaled(icon, cdc.Rect.Min.X, cdc.Rect.Min.Y, 16, 16)
		cdc.DrawCtx.DrawImage(icon, cdc.Rect.Min.X, cdc.Rect.Min.Y)
	})
	col.SetActualWidth(60)
	dg.AddColumn(col)

	ctx := &recorderCtx{}
	dg.Draw(ctx)
	if ctx.images != 4 { // две строки × два вызова
		t.Errorf("шаблонная колонка нарисовала %d картинок, ждали 4", ctx.images)
	}
}

// ScrollX отдаётся наружу — иначе абсолютный X ячейки не посчитать.
func TestScrollX_IsExported(t *testing.T) {
	dg := reqGrid(t, 2)
	if got := dg.ScrollX(); got != 0 {
		t.Errorf("свежая таблица уехала вбок на %d", got)
	}
}

// Подсказка на строку: своя для каждой строки под курсором.
func TestRowToolTip_FollowsHoverRow(t *testing.T) {
	dg := reqGrid(t, 4)
	dg.RowToolTip = func(item interface{}, row int) string {
		if r, ok := item.(*reqRow); ok {
			return r.Name + "!"
		}
		return ""
	}

	if got := dg.HoverRowToolTip(); got != "" {
		t.Errorf("без курсора над строками подсказка %q", got)
	}

	dg.OnMouseMove(10, 20+10) // первая строка
	if got := dg.HoverRow(); got != 0 {
		t.Fatalf("курсор над строкой %d, ждали нулевую", got)
	}
	if got := dg.HoverRowToolTip(); got != "строка!" {
		t.Errorf("подсказка строки %q", got)
	}

	dg.OnMouseMove(10, 5) // заголовок — строки под курсором нет
	if got := dg.HoverRowToolTip(); got != "" {
		t.Errorf("над заголовком подсказка строки %q", got)
	}
}

// RowIndexAtY считает то же, что и внутренняя формула: снаружи её больше
// незачем повторять.
func TestRowIndexAtY_MatchesGeometry(t *testing.T) {
	dg := reqGrid(t, 4)
	for row := 0; row < 4; row++ {
		y := 20 + row*20 + 10
		if got := dg.RowIndexAtY(y); got != row {
			t.Errorf("y=%d дало строку %d, ждали %d", y, got, row)
		}
	}
	if got := dg.RowIndexAtY(5); got != -1 {
		t.Errorf("заголовок дал строку %d, ждали -1", got)
	}
}

// Событие прокрутки и первая видимая строка — запрос GG-12.
//
// Виртуализация в таблице была с самого начала, но спросить, докуда человек
// долистал, было нечем: подгрузку следующей порции журнала приходилось вешать
// на выбор строки рядом с концом списка — то есть требовать щелчка там, где
// человек просто крутил колесо.
func TestScroll_ReportsFirstVisibleRow(t *testing.T) {
	dg := reqGrid(t, 100)
	dg.SetBounds(image.Rect(0, 0, 200, 20+10*20)) // окно на 10 строк

	var firsts, counts []int
	dg.OnScroll = func(first, count int) {
		firsts = append(firsts, first)
		counts = append(counts, count)
	}

	if got := dg.FirstVisibleRow(); got != 0 {
		t.Errorf("свежая таблица начинается со строки %d", got)
	}
	if got := dg.VisibleRowCount(); got != 10 {
		t.Errorf("в окне помещается %d строк, ждали 10", got)
	}

	dg.ScrollBy(20 * 20) // на двадцать строк вниз
	if got := dg.FirstVisibleRow(); got != 20 {
		t.Errorf("после прокрутки первая видимая строка %d, ждали 20", got)
	}
	if len(firsts) != 1 || firsts[0] != 20 || counts[0] != 10 {
		t.Errorf("о прокрутке сообщили как %v/%v, ждали [20]/[10]", firsts, counts)
	}

	// Повторная прокрутка на то же место события не порождает.
	dg.ScrollBy(0)
	if len(firsts) != 1 {
		t.Errorf("прокрутка на ноль породила событие: %v", firsts)
	}

	// Колесо тоже сообщает.
	dg.WheelScroll(false)
	if len(firsts) != 2 {
		t.Errorf("колесо не сообщило о прокрутке: %v", firsts)
	}
}

// Обработчик прокрутки зовётся вне замка: он вправе долить строк в коллекцию
// прямо из себя — ради этого событие и заводилось.
func TestScroll_HandlerMayTouchTheGrid(t *testing.T) {
	dg := reqGrid(t, 100)
	dg.SetBounds(image.Rect(0, 0, 200, 20+10*20))

	done := make(chan struct{})
	dg.OnScroll = func(first, count int) {
		dg.ItemsSource().Add(&reqRow{Name: "дозагрузка"})
		dg.FirstVisibleRow()
	}
	go func() {
		defer close(done)
		dg.ScrollBy(400)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("обработчик прокрутки заклинил поток")
	}
}
