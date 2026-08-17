package datagrid

// audit_sort_test.go — PERF-4: decorate-sort-undecorate не должен менять
// результат сортировки. Проверяем числа, строки (регистронезависимо), bool,
// time.Time, nil-значения и смешанные типы.

import (
	"image"
	"testing"
	"time"
)

type sortRow struct {
	S   string
	N   int
	F   float64
	B   bool
	T   time.Time
	Any interface{}
}

// sortedNames возвращает поле S в порядке представления.
func sortedNames(dg *DataGrid) []string {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	out := make([]string, len(dg.sortedIdx))
	for i, idx := range dg.sortedIdx {
		out[i] = dg.itemsSource.Get(idx).(*sortRow).S
	}
	return out
}

// gridSortedBy строит грид по строкам и сортирует по пути path.
func gridSortedBy(rows []*sortRow, path string, dir SortDirection) *DataGrid {
	dg := New()
	dg.RowHeight, dg.HeaderHeight = 20, 20
	col := NewTextColumn(path, path)
	col.SetActualWidth(100)
	col.SetSortDirection(dir)
	dg.AddColumn(col)

	items := make([]interface{}, len(rows))
	for i, r := range rows {
		items[i] = r
	}
	dg.SetItemsSource(NewObservableCollectionFrom(items))
	dg.SetBounds(image.Rect(0, 0, 200, 200))

	dg.mu.Lock()
	dg.applyCurrentSort()
	dg.mu.Unlock()
	return dg
}

func eqNames(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: длина %d, ожидалось %d (%v)", what, len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: получено %v, ожидалось %v", what, got, want)
		}
	}
}

func TestSort_Numbers(t *testing.T) {
	rows := []*sortRow{
		{S: "c", N: 30}, {S: "a", N: -5}, {S: "b", N: 7}, {S: "d", N: 7},
	}
	eqNames(t, "числа asc", sortedNames(gridSortedBy(rows, "N", SortAscending)),
		[]string{"a", "b", "d", "c"}) // стабильность: b перед d
	eqNames(t, "числа desc", sortedNames(gridSortedBy(rows, "N", SortDescending)),
		[]string{"c", "b", "d", "a"})
}

func TestSort_Floats(t *testing.T) {
	rows := []*sortRow{{S: "x", F: 2.5}, {S: "y", F: -1.5}, {S: "z", F: 10}}
	eqNames(t, "float asc", sortedNames(gridSortedBy(rows, "F", SortAscending)),
		[]string{"y", "x", "z"})
}

// TestSort_StringsCaseInsensitive — сравнение строк остаётся без учёта
// регистра, как в прежнем compareValues (ToLower на каждое сравнение).
func TestSort_StringsCaseInsensitive(t *testing.T) {
	rows := []*sortRow{{S: "Charlie"}, {S: "alice"}, {S: "Bob"}, {S: "dave"}}
	eqNames(t, "строки asc", sortedNames(gridSortedBy(rows, "S", SortAscending)),
		[]string{"alice", "Bob", "Charlie", "dave"})
	eqNames(t, "строки desc", sortedNames(gridSortedBy(rows, "S", SortDescending)),
		[]string{"dave", "Charlie", "Bob", "alice"})
}

func TestSort_Bool(t *testing.T) {
	rows := []*sortRow{{S: "t1", B: true}, {S: "f1", B: false}, {S: "t2", B: true}}
	eqNames(t, "bool asc", sortedNames(gridSortedBy(rows, "B", SortAscending)),
		[]string{"f1", "t1", "t2"})
}

// TestSort_Time — time.Time сравнивается как время. Строковое представление
// (прежнее поведение) путало порядок при разной точности долей секунды.
func TestSort_Time(t *testing.T) {
	base := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	rows := []*sortRow{
		{S: "late", T: base.Add(2 * time.Hour)},
		{S: "early", T: base},
		{S: "mid", T: base.Add(time.Hour).Add(500 * time.Millisecond)},
	}
	eqNames(t, "время asc", sortedNames(gridSortedBy(rows, "T", SortAscending)),
		[]string{"early", "mid", "late"})
	eqNames(t, "время desc", sortedNames(gridSortedBy(rows, "T", SortDescending)),
		[]string{"late", "mid", "early"})
}

// TestSort_NilFirst — отсутствующие значения меньше любых прочих (как раньше).
func TestSort_NilFirst(t *testing.T) {
	rows := []*sortRow{
		{S: "has", Any: 5},
		{S: "none", Any: nil},
		{S: "more", Any: 9},
	}
	eqNames(t, "nil asc", sortedNames(gridSortedBy(rows, "Any", SortAscending)),
		[]string{"none", "has", "more"})
}

// TestSort_MixedTypes — колонка с разнотипными значениями сравнивается по
// строковому виду, как и раньше (общий compareValues).
func TestSort_MixedTypes(t *testing.T) {
	rows := []*sortRow{
		{S: "num", Any: 10},
		{S: "str", Any: "banana"},
		{S: "str2", Any: "Apple"},
	}
	got := sortedNames(gridSortedBy(rows, "Any", SortAscending))
	// "10" < "apple" < "banana" по строковому сравнению в нижнем регистре.
	eqNames(t, "смешанные типы", got, []string{"num", "str2", "str"})
}

// TestSort_StableAcrossRebuild — повторная сборка индекса не ломает порядок.
func TestSort_StableAcrossRebuild(t *testing.T) {
	rows := []*sortRow{{S: "c", N: 3}, {S: "a", N: 1}, {S: "b", N: 2}}
	dg := gridSortedBy(rows, "N", SortAscending)
	want := sortedNames(dg)

	dg.mu.Lock()
	dg.rebuildSortedIdx()
	dg.mu.Unlock()

	eqNames(t, "после rebuildSortedIdx", sortedNames(dg), want)
}
