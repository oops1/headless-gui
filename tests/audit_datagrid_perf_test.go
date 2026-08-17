package tests

// audit_datagrid_perf_test.go — бенчмарки PERF-4 для CollectionView
// (decorate-sort-undecorate) и PERF-8 (Refresh вне лока).

import (
	"fmt"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

type cvBenchRow struct {
	Name  string
	Age   int
	City  string
	Score float64
}

func cvBenchSource(n int) *datagrid.ObservableCollection {
	items := make([]interface{}, n)
	for i := 0; i < n; i++ {
		items[i] = &cvBenchRow{
			Name:  fmt.Sprintf("Person %06d", (i*7919)%n),
			Age:   (i * 31) % 90,
			City:  fmt.Sprintf("City %02d", i%50),
			Score: float64((i*13)%1000) / 3.0,
		}
	}
	return datagrid.NewObservableCollectionFrom(items)
}

// BenchmarkCollectionViewSort_10k — сортировка 10k строк по строковому ключу.
func BenchmarkCollectionViewSort_10k(b *testing.B) {
	src := cvBenchSource(10000)
	v := widget.NewCollectionView(src)
	v.SetSort(widget.SortDescription{Property: "Name"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Refresh()
	}
}

// BenchmarkCollectionViewSort_10k_Multi — двухключевая сортировка 10k строк
// (строка + число): худший случай для «сравнение через reflect на каждый вызов».
func BenchmarkCollectionViewSort_10k_Multi(b *testing.B) {
	src := cvBenchSource(10000)
	v := widget.NewCollectionView(src)
	v.SetSort(
		widget.SortDescription{Property: "City"},
		widget.SortDescription{Property: "Age", Direction: widget.Descending},
	)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Refresh()
	}
}
