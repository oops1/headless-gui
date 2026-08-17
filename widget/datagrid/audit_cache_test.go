package datagrid

// audit_cache_test.go — PERF-3: кэш текста ячеек.
//
// Проверяем ровно два свойства: (1) кэш не врёт — текст совпадает с тем,
// что дал бы GetCellValue, и обновляется по всем сигналам; (2) кэш реально
// экономит — на кадр приходится одно вычисление на ячейку (у CheckBox тоже
// одно, а не два).

import (
	"image"
	"image/color"
	"sync/atomic"
	"testing"
)

// ─── Модель с уведомлениями ────────────────────────────────────────────────

type cacheItem struct {
	PropertyNotifier
	Name string
	Flag bool
}

func (it *cacheItem) SetName(v string) {
	it.Name = v
	it.NotifyPropertyChanged(it, "Name")
}

// ─── Колонки-счётчики ──────────────────────────────────────────────────────

// countingTextColumn считает, сколько раз DataGrid спросил значение ячейки.
type countingTextColumn struct {
	*DataGridTextColumn
	calls *int32
}

func (c *countingTextColumn) GetCellValue(item interface{}) string {
	atomic.AddInt32(c.calls, 1)
	return c.DataGridTextColumn.GetCellValue(item)
}

type countingCheckColumn struct {
	*DataGridCheckBoxColumn
	calls *int32
}

func (c *countingCheckColumn) GetCellValue(item interface{}) string {
	atomic.AddInt32(c.calls, 1)
	return c.DataGridCheckBoxColumn.GetCellValue(item)
}

// cacheGrid — грид на 4 строки с одной текстовой и одной чекбокс-колонкой.
func cacheGrid() (*DataGrid, *ObservableCollection, *int32, *int32) {
	var textCalls, checkCalls int32
	dg := New()
	dg.RowHeight = 20
	dg.HeaderHeight = 20

	tc := NewTextColumn("Name", "Name")
	tc.SetActualWidth(100)
	cc := NewCheckBoxColumn("Flag", "Flag")
	cc.SetActualWidth(60)
	dg.AddColumn(&countingTextColumn{DataGridTextColumn: tc, calls: &textCalls})
	dg.AddColumn(&countingCheckColumn{DataGridCheckBoxColumn: cc, calls: &checkCalls})

	oc := NewObservableCollection()
	for i := 0; i < 4; i++ {
		oc.Add(&cacheItem{Name: "row", Flag: i%2 == 0})
	}
	dg.SetItemsSource(oc)
	dg.SetBounds(image.Rect(0, 0, 200, 20+4*20))
	return dg, oc, &textCalls, &checkCalls
}

// drawnTexts собирает текст, реально ушедший в контекст рисования.
type textRecorder struct {
	nopDrawCtx
	texts []string
}

func (r *textRecorder) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	r.texts = append(r.texts, text)
}

// TestCellCache_OneEvaluationPerCellPerFrame — на кадр приходится ровно одно
// вычисление на ячейку, а повторный кадр без изменений не считает ничего.
func TestCellCache_OneEvaluationPerCellPerFrame(t *testing.T) {
	dg, _, textCalls, checkCalls := cacheGrid()
	ctx := &nopDrawCtx{}

	atomic.StoreInt32(textCalls, 0)
	atomic.StoreInt32(checkCalls, 0)
	dg.Draw(ctx)

	if got := atomic.LoadInt32(textCalls); got != 4 {
		t.Errorf("текстовая колонка: %d вычислений на 4 строки, ожидалось 4", got)
	}
	// PERF-3: раньше здесь было 8 — GetCellValue звался и на «галочку», и
	// на проверку состояния.
	if got := atomic.LoadInt32(checkCalls); got != 4 {
		t.Errorf("чекбокс-колонка: %d вычислений на 4 строки, ожидалось 4", got)
	}

	// Второй кадр — всё из кэша.
	atomic.StoreInt32(textCalls, 0)
	atomic.StoreInt32(checkCalls, 0)
	dg.Draw(ctx)
	if got := atomic.LoadInt32(textCalls) + atomic.LoadInt32(checkCalls); got != 0 {
		t.Errorf("второй кадр пересчитал %d ячеек — кэш не работает", got)
	}
}

// TestCellCache_InvalidatedByCollectionChange — изменение коллекции сбрасывает
// кэш, и таблица показывает новый текст.
func TestCellCache_InvalidatedByCollectionChange(t *testing.T) {
	dg, oc, _, _ := cacheGrid()
	rec := &textRecorder{}
	dg.Draw(rec)

	oc.Set(0, &cacheItem{Name: "changed"})

	rec.texts = nil
	dg.Draw(rec)
	if !containsText(rec.texts, "changed") {
		t.Fatalf("после Set() ячейка показывает старый текст: %v", rec.texts)
	}
}

// TestCellCache_InvalidatedByPropertyChanged — элемент с INotifyPropertyChanged
// сам сбрасывает кэш при изменении свойства.
func TestCellCache_InvalidatedByPropertyChanged(t *testing.T) {
	dg, oc, _, _ := cacheGrid()
	rec := &textRecorder{}
	dg.Draw(rec)

	oc.Get(1).(*cacheItem).SetName("notified")

	rec.texts = nil
	dg.Draw(rec)
	if !containsText(rec.texts, "notified") {
		t.Fatalf("PropertyChanged не сбросил кэш ячейки: %v", rec.texts)
	}
}

// TestCellCache_InvalidatedByColumnsAndSort — смена колонок и сортировка тоже
// обесценивают кэш (иначе текст «уезжает» на чужие строки).
func TestCellCache_InvalidatedByColumnsAndSort(t *testing.T) {
	oc := NewObservableCollection()
	oc.Add(&cacheItem{Name: "b"})
	oc.Add(&cacheItem{Name: "a"})

	dg := New()
	dg.RowHeight, dg.HeaderHeight = 20, 20
	col := NewTextColumn("Name", "Name")
	col.SetActualWidth(100)
	dg.AddColumn(col)
	dg.SetItemsSource(oc)
	dg.SetBounds(image.Rect(0, 0, 200, 20+2*20))

	rec := &textRecorder{}
	dg.Draw(rec)
	if len(rec.texts) < 2 || rec.texts[len(rec.texts)-2] != "b" {
		t.Fatalf("исходный порядок строк не b,a: %v", rec.texts)
	}

	// Сортировка по возрастанию: клик по заголовку.
	dg.OnMouseButton(10, 5, 0, true)

	rec.texts = nil
	dg.Draw(rec)
	// Последние две «текстовые» записи — содержимое строк.
	got := lastTwo(rec.texts)
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("после сортировки кэш отдал прежний порядок: %v", rec.texts)
	}

	// Смена колонок — новый набор, кэш прежних индексов недействителен.
	other := NewTextColumn("Same", "Name")
	other.SetActualWidth(100)
	dg.SetColumns([]Column{other})
	rec.texts = nil
	dg.Draw(rec)
	if !containsText(rec.texts, "a") || !containsText(rec.texts, "b") {
		t.Fatalf("после SetColumns текст ячеек потерян: %v", rec.texts)
	}
}

// TestCellCache_BoundedBySize — кэш не растёт на всю коллекцию: после
// прокрутки 100k строк в нём остаётся только окно с запасом.
func TestCellCache_BoundedBySize(t *testing.T) {
	dg := New()
	dg.RowHeight, dg.HeaderHeight = 10, 10
	col := NewTextColumn("Name", "Name")
	col.SetActualWidth(100)
	dg.AddColumn(col)

	items := make([]interface{}, 100000)
	for i := range items {
		items[i] = dgPlainRow{Name: "x"}
	}
	dg.SetItemsSource(NewObservableCollectionFrom(items))
	dg.SetBounds(image.Rect(0, 0, 200, 10+30*10))

	ctx := &nopDrawCtx{}
	for i := 0; i < 400; i++ {
		dg.ScrollBy(10 * 30)
		dg.Draw(ctx)
	}

	dg.cacheMu.Lock()
	n := len(dg.cellCache)
	dg.cacheMu.Unlock()
	if n > 4000 {
		t.Fatalf("кэш разросся до %d записей — окно не удерживается", n)
	}
}

// dgPlainRow — модель без уведомлений (кэшируется всегда).
type dgPlainRow struct{ Name string }

func containsText(texts []string, want string) bool {
	for _, s := range texts {
		if s == want {
			return true
		}
	}
	return false
}

func lastTwo(texts []string) [2]string {
	var out [2]string
	if n := len(texts); n >= 2 {
		out[0], out[1] = texts[n-2], texts[n-1]
	}
	return out
}
