package datagrid

import "testing"

// GG-93: журнал Go.Git подгружает историю страницами по 500 строк. SetItems
// со всей коллекцией слал Reset и снимал выделение, а Add по одному на каждый
// элемент перестраивал индекс строк целиком.

func rangeRows(from, n int) []interface{} {
	out := make([]interface{}, n)
	for i := range out {
		out[i] = &selRow{N: from + i}
	}
	return out
}

// AddRange — одно событие CollectionAdd с первым индексом, числом и элементами.
func TestAddRange_OneEvent(t *testing.T) {
	oc := NewObservableCollectionFrom(rangeRows(0, 3))
	var events []CollectionChangedEvent
	oc.AddCollectionChanged(func(e CollectionChangedEvent) { events = append(events, e) })

	page := rangeRows(3, 4)
	oc.AddRange(page)
	page[0] = nil // вызывающий переиспользует срез — коллекция не должна это видеть

	if len(events) != 1 {
		t.Fatalf("событий %d, ждал одно", len(events))
	}
	e := events[0]
	if e.Action != CollectionAdd || e.Index != 3 || e.Count != 4 || e.Added() != 4 || len(e.NewItems) != 4 {
		t.Fatalf("событие %+v, ждал Add с Index 3 и Count 4", e)
	}
	if e.NewItem.(*selRow).N != 3 || e.NewItems[3].(*selRow).N != 6 {
		t.Fatalf("элементы события не те: %+v", e.NewItems)
	}
	if oc.Count() != 7 || oc.Get(3) == nil || oc.Get(3).(*selRow).N != 3 {
		t.Fatalf("коллекция после AddRange: %d элементов, [3] = %v", oc.Count(), oc.Get(3))
	}

	oc.AddRange(nil)
	if len(events) != 1 {
		t.Fatal("пустой AddRange прислал событие")
	}
	// Поштучный Add — по-прежнему один элемент: Count 0, Added 1.
	oc.Add(&selRow{N: 7})
	if e := events[len(events)-1]; e.Count != 0 || e.Added() != 1 || e.NewItems != nil {
		t.Fatalf("Add: %+v", e)
	}
}

// Подгрузка страницы не снимает выделение и не пересчитывает ячейки старых строк.
func TestAddRange_KeepsSelectionAndCellCache(t *testing.T) {
	dg, oc, events := selGrid(t)
	dg.AddColumn(NewTextColumn("N", "N"))
	dg.SetSelectedIndex(2)
	n := len(*events)
	gen := dg.cacheGen

	oc.AddRange(rangeRows(5, 500))

	if got := dg.SelectedItems(); len(got) != 1 || got[0].(*selRow).N != 2 {
		t.Fatalf("выделение после подгрузки: %+v, ждал строку 2", got)
	}
	if len(*events) != n {
		t.Fatalf("подгрузка прислала OnSelectionChanged: %v", (*events)[n:])
	}
	if len(dg.sortedIdx) != 505 || dg.sortedIdx[504] != 504 {
		t.Fatalf("индекс строк: %d строк, последняя %d", len(dg.sortedIdx), dg.sortedIdx[len(dg.sortedIdx)-1])
	}
	if dg.cacheGen != gen {
		t.Fatal("кэш ячеек сброшен: дописывание в конец не меняет старые строки")
	}

	// Поштучный Add в конец идёт тем же путём — без полного перестроения.
	oc.Add(&selRow{N: 505})
	if dg.cacheGen != gen || len(dg.sortedIdx) != 506 {
		t.Fatalf("Add в конец: поколение кэша %d (было %d), строк %d", dg.cacheGen, gen, len(dg.sortedIdx))
	}
}

// С сортировкой новые строки встают по порядку: быстрый путь не годится.
func TestAddRange_WithSortRebuilds(t *testing.T) {
	dg := New()
	col := NewTextColumn("N", "N")
	dg.AddColumn(col)
	oc := NewObservableCollectionFrom(rangeRows(10, 3)) // 10, 11, 12
	dg.SetItemsSource(oc)
	col.SetSortDirection(SortDescending)
	dg.mu.Lock()
	dg.rebuildSortedIdx()
	dg.mu.Unlock()

	oc.AddRange([]interface{}{&selRow{N: 20}, &selRow{N: 1}})

	var got []int
	for _, i := range dg.sortedIdx {
		got = append(got, oc.Get(i).(*selRow).N)
	}
	want := []int{20, 12, 11, 10, 1}
	if len(got) != len(want) {
		t.Fatalf("порядок %v, ждал %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("порядок %v, ждал %v", got, want)
		}
	}
}

// Вставка в середину сдвигает строки — это полный путь, как раньше.
func TestAddRange_InsertInMiddleRebuilds(t *testing.T) {
	dg, oc, _ := selGrid(t)
	dg.AddColumn(NewTextColumn("N", "N"))
	gen := dg.cacheGen
	oc.Insert(1, &selRow{N: 99})
	if dg.cacheGen == gen {
		t.Fatal("вставка в середину не сбросила кэш ячеек: строки сдвинулись")
	}
	if len(dg.sortedIdx) != 6 {
		t.Fatalf("строк %d, ждал 6", len(dg.sortedIdx))
	}
}
