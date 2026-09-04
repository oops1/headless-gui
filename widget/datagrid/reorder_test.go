package datagrid

import (
	"image"
	"testing"
	"time"
)

// Перетаскивание колонок и щелчок по заголовку — запросы GG-23 и GG-24.
//
// Раньше единственной реакцией на нажатие в заголовке была сортировка, а
// поменять колонки местами было нечем ни событием, ни поведением. Приложение
// выключало сортировку целиком и разбирало мышь в полосе заголовка само —
// вместе с различением «щелчок» / «потянули за границу» / «потащили колонку».

type reoRow struct{ Name string }

func reoGrid(t *testing.T) *DataGrid {
	t.Helper()
	dg := New()
	dg.RowHeight = 20
	dg.HeaderHeight = 20
	for _, h := range []string{"A", "B", "C"} {
		c := NewTextColumn(h, "Name")
		c.SetActualWidth(100)
		dg.AddColumn(c)
	}
	oc := NewObservableCollection()
	for i := 0; i < 3; i++ {
		oc.Add(&reoRow{Name: "x"})
	}
	dg.SetItemsSource(oc)
	dg.SetBounds(image.Rect(0, 0, 300, 100))
	return dg
}

func headers(dg *DataGrid) []string {
	out := make([]string, 0, len(dg.Columns()))
	for _, c := range dg.Columns() {
		out = append(out, c.Header())
	}
	return out
}

func eq(a []string, b ...string) bool {
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

// MoveColumn переставляет колонку и сообщает об этом.
func TestReorder_MoveColumn(t *testing.T) {
	dg := reoGrid(t)
	var from, to = -1, -1
	dg.OnColumnsReordered = func(f, tt int) { from, to = f, tt }

	dg.MoveColumn(0, 2)
	if got := headers(dg); !eq(got, "B", "C", "A") {
		t.Errorf("после переноса первой в конец порядок %v", got)
	}
	if from != 0 || to != 2 {
		t.Errorf("о переносе сообщили как (%d,%d), ждали (0,2)", from, to)
	}

	dg.MoveColumn(2, 0)
	if got := headers(dg); !eq(got, "A", "B", "C") {
		t.Errorf("обратный перенос дал %v", got)
	}
}

// Заведомо неверные индексы ничего не делают и не роняют.
func TestReorder_MoveColumnIgnoresBadIndexes(t *testing.T) {
	dg := reoGrid(t)
	fired := false
	dg.OnColumnsReordered = func(int, int) { fired = true }

	for _, pair := range [][2]int{{-1, 0}, {0, -1}, {5, 0}, {0, 5}, {1, 1}} {
		dg.MoveColumn(pair[0], pair[1])
	}
	if got := headers(dg); !eq(got, "A", "B", "C") {
		t.Errorf("негодные индексы переставили колонки: %v", got)
	}
	if fired {
		t.Error("о несостоявшемся переносе сообщили как о состоявшемся")
	}
}

// Мышью: нажали на заголовке, увели вправо за середину соседа, отпустили.
func TestReorder_DragMovesColumn(t *testing.T) {
	dg := reoGrid(t)
	dg.CanUserReorderColumns = true

	dg.OnMouseButton(50, 10, 0, true) // взяли колонку A
	dg.OnMouseMove(160, 10)           // за середину B
	dg.OnMouseButton(160, 10, 0, false)

	if got := headers(dg); !eq(got, "B", "A", "C") {
		t.Errorf("перетаскивание дало порядок %v, ждали B A C", got)
	}
}

// Дрожь руки — не перетаскивание: порог в несколько пикселей не пройден.
func TestReorder_TinyMoveIsAClick(t *testing.T) {
	dg := reoGrid(t)
	dg.CanUserReorderColumns = true
	clicks := 0
	dg.OnHeaderClick = func(col Column, idx, x, y int) bool { clicks++; return true }

	dg.OnMouseButton(50, 10, 0, true)
	dg.OnMouseMove(52, 10)
	dg.OnMouseButton(52, 10, 0, false)

	if got := headers(dg); !eq(got, "A", "B", "C") {
		t.Errorf("дрожь переставила колонки: %v", got)
	}
	if clicks != 1 {
		t.Errorf("щелчок по заголовку сработал %d раз, ждали один", clicks)
	}
}

// OnHeaderClick вызывается независимо от сортировки и умеет её отменить.
func TestHeaderClick_OverridesSorting(t *testing.T) {
	dg := reoGrid(t)
	dg.CanUserSortColumns = true
	var gotCol Column
	var gotIdx int
	dg.OnHeaderClick = func(col Column, idx, x, y int) bool {
		gotCol, gotIdx = col, idx
		return true // клик разобран — сортировать не надо
	}

	dg.OnMouseButton(150, 10, 0, true) // вторая колонка

	if gotIdx != 1 || gotCol == nil || gotCol.Header() != "B" {
		t.Errorf("обработчику досталась колонка %d (%v)", gotIdx, gotCol)
	}
	for _, c := range dg.Columns() {
		if c.GetSortDirection() != SortNone {
			t.Errorf("колонка %q отсортирована, хотя клик разобран обработчиком", c.Header())
		}
	}
}

// Обработчик, вернувший false, сортировку не отменяет.
func TestHeaderClick_FalseLetsSortingHappen(t *testing.T) {
	dg := reoGrid(t)
	dg.CanUserSortColumns = true
	dg.OnHeaderClick = func(Column, int, int, int) bool { return false }

	dg.OnMouseButton(50, 10, 0, true)

	if dg.Columns()[0].GetSortDirection() == SortNone {
		t.Error("сортировка не сработала, хотя обработчик её не отменял")
	}
}

// Обработчик щелчка зовётся вне внутреннего замка: он вправе обратиться к
// самой таблице, и это не должно вешать поток.
func TestHeaderClick_NoDeadlock(t *testing.T) {
	dg := reoGrid(t)
	done := make(chan struct{})
	dg.OnHeaderClick = func(Column, int, int, int) bool {
		dg.ScrollX()
		dg.Columns()
		dg.MoveColumn(0, 1)
		return true
	}
	go func() {
		defer close(done)
		dg.OnMouseButton(50, 10, 0, true)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("обработчик щелчка по заголовку заклинил поток")
	}
}

// Кромка resize имеет приоритет: тянут границу, а не колонку.
func TestReorder_ResizeEdgeWins(t *testing.T) {
	dg := reoGrid(t)
	dg.CanUserReorderColumns = true
	dg.CanUserResizeColumns = true

	// Правая граница первой колонки — 100.
	dg.OnMouseButton(99, 10, 0, true)
	dg.OnMouseMove(160, 10)
	dg.OnMouseButton(160, 10, 0, false)

	if got := headers(dg); !eq(got, "A", "B", "C") {
		t.Errorf("тянули границу, а переставились колонки: %v", got)
	}
	if w := dg.Columns()[0].ActualWidth(); w <= 100 {
		t.Errorf("ширина первой колонки %d — граница не потянулась", w)
	}
}
