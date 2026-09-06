package datagrid

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

// Запросы GG-41 (многоточие), GG-42 (фигуры в мосте) и GG-43 (пустое
// состояние).

// cellRecorder — контекст, запоминающий нарисованный текст и фигуры.
type cellRecorder struct {
	nopDrawCtx
	texts     []string
	ellipses  int
	roundRect int
	// charW — ширина символа, которой «меряет» этот контекст.
	charW int
}

func (c *cellRecorder) MeasureText(s string, sizePt float64) int {
	w := c.charW
	if w == 0 {
		w = 7
	}
	return len([]rune(s)) * w
}
func (c *cellRecorder) DrawTextSize(s string, x, y int, sizePt float64, col color.RGBA) {
	c.texts = append(c.texts, s)
}
func (c *cellRecorder) DrawText(s string, x, y int, col color.RGBA) {
	c.texts = append(c.texts, s)
}
func (c *cellRecorder) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) { c.ellipses++ }
func (c *cellRecorder) FillRoundRect(x, y, w, h, r int, col color.RGBA)  { c.roundRect++ }

type cellRow struct{ Name string }

// Длинный текст обрезается многоточием, короткий не трогается.
func TestEllipsizeText(t *testing.T) {
	ctx := &cellRecorder{charW: 10} // 10 точек на символ

	if got := EllipsizeText(ctx, "abc", 100, 10); got != "abc" {
		t.Errorf("короткая строка изменилась: %q", got)
	}
	// «abcdefghij» — 100 точек, в 55 влезает 4 символа плюс многоточие (50).
	got := EllipsizeText(ctx, "abcdefghij", 55, 10)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("обрезанная строка без многоточия: %q", got)
	}
	if ctx.MeasureText(got, 10) > 55 {
		t.Errorf("результат %q шире отведённого", got)
	}
	if len([]rune(got)) < 2 {
		t.Errorf("обрезали слишком коротко: %q", got)
	}
}

// В совсем узкой колонке остаётся одно многоточие: пустая ячейка соврала бы,
// что значения нет вовсе.
func TestEllipsizeText_TooNarrowKeepsEllipsis(t *testing.T) {
	ctx := &cellRecorder{charW: 10}
	if got := EllipsizeText(ctx, "abcdef", 12, 10); got != "…" {
		t.Errorf("в узкой колонке получилось %q, ждали одно многоточие", got)
	}
}

// Текстовая колонка сама обрезает значение по ширине ячейки.
func TestTextColumn_EllipsizesLongValue(t *testing.T) {
	dg := New()
	dg.RowHeight = 20
	dg.HeaderHeight = 20
	col := NewTextColumn("Адрес", "Name")
	col.SetActualWidth(60) // 60 - 12 отступов = 48 точек под текст
	dg.AddColumn(col)

	oc := NewObservableCollection()
	oc.Add(&cellRow{Name: "очень-длинный-адрес-репозитория"})
	dg.SetItemsSource(oc)
	dg.SetBounds(image.Rect(0, 0, 60, 40))

	ctx := &cellRecorder{charW: 10}
	dg.Draw(ctx)

	found := ""
	for _, s := range ctx.texts {
		if strings.HasSuffix(s, "…") {
			found = s
			break
		}
	}
	if found == "" {
		t.Fatalf("длинное значение нарисовано без многоточия: %v", ctx.texts)
	}
	if ctx.MeasureText(found, dg.FontSize) > 48 {
		t.Errorf("обрезанное значение %q шире ячейки", found)
	}
}

// Шаблонная колонка рисует фигуры — кружок статуса больше не нужно
// растеризовать из SVG заранее.
func TestTemplateColumn_CanDrawShapes(t *testing.T) {
	dg := New()
	dg.RowHeight = 20
	dg.HeaderHeight = 20

	col := NewTemplateColumn("Статус", func(cdc CellDrawContext) {
		r := cdc.Rect
		cdc.DrawCtx.FillEllipseAA(r.Min.X+8, r.Min.Y+r.Dy()/2, 5, 5, color.RGBA{G: 200, A: 255})
		cdc.DrawCtx.FillRoundRect(r.Min.X+20, r.Min.Y+4, 30, 12, 4, color.RGBA{B: 200, A: 255})
	})
	col.SetActualWidth(80)
	dg.AddColumn(col)

	oc := NewObservableCollection()
	oc.Add(&cellRow{Name: "a"})
	oc.Add(&cellRow{Name: "b"})
	dg.SetItemsSource(oc)
	dg.SetBounds(image.Rect(0, 0, 80, 60))

	ctx := &cellRecorder{}
	dg.Draw(ctx)

	if ctx.ellipses != 2 {
		t.Errorf("нарисовано %d кружков, ждали по одному на строку", ctx.ellipses)
	}
	if ctx.roundRect != 2 {
		t.Errorf("нарисовано %d скруглённых прямоугольников", ctx.roundRect)
	}
}

// Пустая таблица объясняет себя.
func TestEmptyState_TextIsDrawn(t *testing.T) {
	dg := New()
	dg.RowHeight = 20
	dg.HeaderHeight = 20
	col := NewTextColumn("Имя", "Name")
	col.SetActualWidth(200)
	dg.AddColumn(col)
	dg.SetItemsSource(NewObservableCollection())
	dg.SetBounds(image.Rect(0, 0, 200, 100))

	ctx := &cellRecorder{}
	dg.Draw(ctx)
	for _, s := range ctx.texts {
		if s == "Ключей пока нет" {
			t.Fatal("текст пустого состояния нарисован, хотя его не задавали")
		}
	}

	dg.EmptyStateText = "Ключей пока нет"
	ctx = &cellRecorder{}
	dg.Draw(ctx)

	seen := false
	for _, s := range ctx.texts {
		if s == "Ключей пока нет" {
			seen = true
		}
	}
	if !seen {
		t.Errorf("текст пустого состояния не нарисован: %v", ctx.texts)
	}
	// Заголовок остался: по нему видно, что за таблица.
	head := false
	for _, s := range ctx.texts {
		if s == "Имя" {
			head = true
		}
	}
	if !head {
		t.Error("заголовок исчез вместе со строками")
	}
}

// Непустая таблица объясняет себя сама — текста пустого состояния в ней нет.
func TestEmptyState_HiddenWhenRowsExist(t *testing.T) {
	dg := New()
	dg.RowHeight = 20
	dg.HeaderHeight = 20
	dg.EmptyStateText = "Ничего нет"
	col := NewTextColumn("Имя", "Name")
	col.SetActualWidth(200)
	dg.AddColumn(col)

	oc := NewObservableCollection()
	oc.Add(&cellRow{Name: "строка"})
	dg.SetItemsSource(oc)
	dg.SetBounds(image.Rect(0, 0, 200, 100))

	ctx := &cellRecorder{}
	dg.Draw(ctx)
	for _, s := range ctx.texts {
		if s == "Ничего нет" {
			t.Error("текст пустого состояния нарисован поверх непустой таблицы")
		}
	}
}

// Своя отрисовка пустого состояния перекрывает текст.
func TestEmptyState_RendererWins(t *testing.T) {
	dg := New()
	dg.RowHeight = 20
	dg.HeaderHeight = 20
	dg.EmptyStateText = "текст"
	called := 0
	var gotRect image.Rectangle
	dg.EmptyStateRenderer = func(c EmptyStateContext) {
		called++
		gotRect = c.Rect
	}
	col := NewTextColumn("Имя", "Name")
	col.SetActualWidth(200)
	dg.AddColumn(col)
	dg.SetItemsSource(NewObservableCollection())
	dg.SetBounds(image.Rect(0, 0, 200, 100))

	ctx := &cellRecorder{}
	dg.Draw(ctx)

	if called != 1 {
		t.Fatalf("отрисовщик вызван %d раз", called)
	}
	if gotRect.Empty() || gotRect.Min.Y < 20 {
		t.Errorf("отрисовщику досталась область %v — ждали область данных под заголовком", gotRect)
	}
	for _, s := range ctx.texts {
		if s == "текст" {
			t.Error("текст нарисован, хотя задан свой отрисовщик")
		}
	}
}
