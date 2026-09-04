package datagrid

// perf_bench_test.go — бенчмарки аудита PERF-3 (кэш текста ячеек) и
// PERF-4 (decorate-sort-undecorate). Меряют ровно то, что просил аудит:
// отрисовку окна 30×8 ячеек и сортировку 10k строк.

import (
	"fmt"
	"image"
	"image/color"
	"testing"
)

// ─── Заглушка DrawContextBridge ────────────────────────────────────────────

// nopDrawCtx — контекст рисования, ничего не рисующий: бенчмарк меряет
// стоимость подготовки данных (reflect/format), а не растеризацию.
type nopDrawCtx struct{ sink int }

func (c *nopDrawCtx) FillRect(x, y, w, h int, col color.RGBA)      { c.sink += w }
func (c *nopDrawCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) { c.sink += w }
func (c *nopDrawCtx) DrawBorder(x, y, w, h int, col color.RGBA)    { c.sink += w }
func (c *nopDrawCtx) DrawText(text string, x, y int, col color.RGBA) {
	c.sink += len(text)
}
func (c *nopDrawCtx) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.sink += len(text)
}
func (c *nopDrawCtx) MeasureText(text string, sizePt float64) int { return len(text) * 7 }
func (c *nopDrawCtx) SetClip(r image.Rectangle)                   { c.sink += r.Dx() }
func (c *nopDrawCtx) ClearClip()                                  {}
func (c *nopDrawCtx) DrawHLine(x, y, length int, col color.RGBA)  { c.sink += length }
func (c *nopDrawCtx) DrawVLine(x, y, length int, col color.RGBA)  { c.sink += length }
func (c *nopDrawCtx) DrawImage(src image.Image, x, y int)         { c.sink += x }
func (c *nopDrawCtx) DrawImageScaled(src image.Image, x, y, w, h int) {
	c.sink += w
}

// ─── Модель для бенчмарков ─────────────────────────────────────────────────

type benchRow struct {
	Name  string
	Age   int
	City  string
	Email string
	Score float64
	Tag   string
	Note  string
	Flag  bool
}

func benchCollection(n int) *ObservableCollection {
	items := make([]interface{}, n)
	for i := 0; i < n; i++ {
		items[i] = &benchRow{
			Name:  fmt.Sprintf("Person %06d", (i*7919)%n),
			Age:   (i * 31) % 90,
			City:  fmt.Sprintf("City %02d", i%50),
			Email: fmt.Sprintf("user%06d@example.com", i),
			Score: float64((i*13)%1000) / 3.0,
			Tag:   fmt.Sprintf("tag-%03d", i%997),
			Note:  "note",
			Flag:  i%2 == 0,
		}
	}
	return NewObservableCollectionFrom(items)
}

// benchGrid — грид на 8 колонок с окном ровно в 30 видимых строк.
func benchGrid(rows int) *DataGrid {
	dg := New()
	dg.RowHeight = 20
	dg.HeaderHeight = 20
	dg.AddColumn(NewTextColumn("Name", "Name"))
	dg.AddColumn(NewTextColumn("Age", "Age"))
	dg.AddColumn(NewTextColumn("City", "City"))
	dg.AddColumn(NewTextColumn("Email", "Email"))
	dg.AddColumn(NewTextColumn("Score", "Score"))
	dg.AddColumn(NewTextColumn("Tag", "Tag"))
	dg.AddColumn(NewTextColumn("Note", "Note"))
	dg.AddColumn(NewCheckBoxColumn("Flag", "Flag"))
	dg.SetItemsSource(benchCollection(rows))
	// 30 строк по 20px + заголовок 20px.
	dg.SetBounds(image.Rect(0, 0, 1200, 20+30*20))
	return dg
}

// BenchmarkDataGridDraw_30x8 — отрисовка видимого окна 30×8 ячеек.
func BenchmarkDataGridDraw_30x8(b *testing.B) {
	dg := benchGrid(5000)
	ctx := &nopDrawCtx{}
	dg.Draw(ctx) // прогрев layout
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dg.Draw(ctx)
	}
}

// BenchmarkDataGridDraw_30x8_Scrolling — то же, но с прокруткой на строку
// каждый кадр: проверяет, что кэш не деградирует на скролле.
func BenchmarkDataGridDraw_30x8_Scrolling(b *testing.B) {
	dg := benchGrid(5000)
	ctx := &nopDrawCtx{}
	dg.Draw(ctx)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dg.ScrollBy(20)
		if i%200 == 199 {
			dg.ScrollBy(-200 * 20)
		}
		dg.Draw(ctx)
	}
}

// BenchmarkDataGridSort_10k — сортировка 10k строк по строковой колонке.
func BenchmarkDataGridSort_10k(b *testing.B) {
	dg := benchGrid(10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dg.mu.Lock()
		dg.columns[0].SetSortDirection(SortAscending)
		if i%2 == 1 {
			dg.columns[0].SetSortDirection(SortDescending)
		}
		dg.applyCurrentSort()
		dg.mu.Unlock()
	}
}

// BenchmarkDataGridSort_10k_Numeric — сортировка 10k строк по числовой колонке.
func BenchmarkDataGridSort_10k_Numeric(b *testing.B) {
	dg := benchGrid(10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dg.mu.Lock()
		dg.columns[0].SetSortDirection(SortNone)
		dg.columns[4].SetSortDirection(SortAscending)
		if i%2 == 1 {
			dg.columns[4].SetSortDirection(SortDescending)
		}
		dg.applyCurrentSort()
		dg.mu.Unlock()
	}
}
