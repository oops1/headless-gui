package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

type person struct {
	Name string
	Age  int
	City string
}

func names(items []interface{}) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.(*person).Name
	}
	return out
}

func sampleOC() *datagrid.ObservableCollection {
	return datagrid.NewObservableCollectionFrom([]interface{}{
		&person{"Charlie", 30, "NYC"},
		&person{"alice", 17, "LA"},
		&person{"Bob", 25, "NYC"},
		&person{"Dave", 16, "LA"},
	})
}

func eqStr(a, b []string) bool {
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

func TestCollectionView_Filter(t *testing.T) {
	v := widget.NewCollectionView(sampleOC())
	v.SetFilter(func(it interface{}) bool { return it.(*person).Age >= 18 })
	got := names(v.Items())
	want := []string{"Charlie", "Bob"} // порядок исходный
	if !eqStr(got, want) {
		t.Fatalf("filter: got %v want %v", got, want)
	}
}

func TestCollectionView_SortAscDesc(t *testing.T) {
	v := widget.NewCollectionView(sampleOC())
	v.SetSort(widget.SortDescription{Property: "Name"}) // регистронезависимо
	if got := names(v.Items()); !eqStr(got, []string{"alice", "Bob", "Charlie", "Dave"}) {
		t.Fatalf("asc: got %v", got)
	}
	v.SetSort(widget.SortDescription{Property: "Age", Direction: widget.Descending})
	if got := names(v.Items()); !eqStr(got, []string{"Charlie", "Bob", "alice", "Dave"}) {
		t.Fatalf("desc by age: got %v", got)
	}
}

func TestCollectionView_MultiSort(t *testing.T) {
	v := widget.NewCollectionView(sampleOC())
	// Сначала по City (asc), затем по Age (desc) внутри города.
	v.SetSort(
		widget.SortDescription{Property: "City"},
		widget.SortDescription{Property: "Age", Direction: widget.Descending},
	)
	// LA: Dave(16),alice(17) → desc: alice,Dave ; NYC: Charlie(30),Bob(25)
	if got := names(v.Items()); !eqStr(got, []string{"alice", "Dave", "Charlie", "Bob"}) {
		t.Fatalf("multisort: got %v", got)
	}
}

func TestCollectionView_FilterPlusSort(t *testing.T) {
	v := widget.NewCollectionView(sampleOC())
	v.SetFilter(func(it interface{}) bool { return it.(*person).City == "NYC" })
	v.AddSort(widget.SortDescription{Property: "Age"})
	if got := names(v.Items()); !eqStr(got, []string{"Bob", "Charlie"}) {
		t.Fatalf("filter+sort: got %v", got)
	}
}

func TestCollectionView_Group(t *testing.T) {
	v := widget.NewCollectionView(sampleOC())
	v.SetGroup("City")
	groups := v.Groups()
	if len(groups) != 2 {
		t.Fatalf("groups: got %d want 2", len(groups))
	}
	// Порядок появления: NYC (Charlie), LA (alice)
	if groups[0].Name != "NYC" || len(groups[0].Items) != 2 {
		t.Fatalf("group0: %+v", groups[0])
	}
	if groups[1].Name != "LA" || len(groups[1].Items) != 2 {
		t.Fatalf("group1: %+v", groups[1])
	}
}

func TestCollectionView_LiveRefreshOnSourceChange(t *testing.T) {
	oc := sampleOC()
	v := widget.NewCollectionView(oc)
	v.SetFilter(func(it interface{}) bool { return it.(*person).Age >= 18 })
	if v.Count() != 2 {
		t.Fatalf("before add: %d", v.Count())
	}
	oc.Add(&person{"Eve", 40, "SF"}) // подходит под фильтр
	if v.Count() != 3 {
		t.Fatalf("after add: %d want 3", v.Count())
	}
}

func TestCollectionView_ViewChangedFires(t *testing.T) {
	v := widget.NewCollectionView(sampleOC())
	fired := 0
	v.AddViewChanged(func() { fired++ })
	v.SetFilter(func(it interface{}) bool { return true })
	v.AddSort(widget.SortDescription{Property: "Name"})
	if fired < 2 {
		t.Fatalf("view-changed fired %d times, want >=2", fired)
	}
}
