package datagrid

import "testing"

// GG-81: выделение переживало замену строк и молчало о программной смене.
//
// В журнале клиента git обновление репозитория очищает коллекцию и заполняет её
// заново: подсветка оставалась на прежнем индексе и показывала другой коммит, а
// приложение об этом не знало — событие слал только клик.

type selRow struct{ N int }

func selGrid(t *testing.T) (*DataGrid, *ObservableCollection, *[]SelectionChangedEvent) {
	t.Helper()
	dg := New()
	oc := NewObservableCollection()
	for i := 0; i < 5; i++ {
		oc.Add(&selRow{N: i})
	}
	dg.SetItemsSource(oc)
	var events []SelectionChangedEvent
	dg.OnSelectionChanged = func(e SelectionChangedEvent) { events = append(events, e) }
	return dg, oc, &events
}

func lastEvent(t *testing.T, events *[]SelectionChangedEvent) SelectionChangedEvent {
	t.Helper()
	if len(*events) == 0 {
		t.Fatal("событие OnSelectionChanged не пришло")
	}
	return (*events)[len(*events)-1]
}

// Программная смена выделения шлёт событие, как клик (и как WPF).
func TestSelection_SetSelectedIndexNotifies(t *testing.T) {
	dg, _, events := selGrid(t)

	dg.SetSelectedIndex(3)
	if e := lastEvent(t, events); e.SelectedIndex != 3 {
		t.Fatalf("событие: строка %d, ждал 3", e.SelectedIndex)
	}
	if e := lastEvent(t, events); e.SelectedItem.(*selRow).N != 3 {
		t.Fatalf("в событии другой элемент: %+v", e.SelectedItem)
	}

	// Повторная установка того же — без события.
	n := len(*events)
	dg.SetSelectedIndex(3)
	if len(*events) != n {
		t.Fatalf("повтор той же строки прислал событие: %v", *events)
	}

	// Тихая установка — без события.
	dg.SetSelectedIndexQuiet(1)
	if len(*events) != n {
		t.Fatalf("SetSelectedIndexQuiet прислал событие: %v", *events)
	}
	if got := dg.SelectedItems(); len(got) != 1 || got[0].(*selRow).N != 1 {
		t.Fatalf("тихая установка не сменила выделение: %+v", got)
	}
}

// Сброс коллекции снимает выделение и сообщает об этом.
func TestSelection_ClearedOnCollectionReset(t *testing.T) {
	dg, oc, events := selGrid(t)
	dg.SetSelectedIndex(4)
	*events = nil

	oc.Clear()

	if got := dg.SelectedItems(); len(got) != 0 {
		t.Fatalf("после сброса коллекции выделение осталось: %+v", got)
	}
	if e := lastEvent(t, events); e.SelectedIndex != -1 || e.SelectedItem != nil {
		t.Fatalf("событие сброса: %+v, ждал пустой выбор", e)
	}

	// Заполнили заново — выделения по-прежнему нет.
	*events = nil
	for i := 0; i < 3; i++ {
		oc.Add(&selRow{N: 10 + i})
	}
	if got := dg.SelectedItems(); len(got) != 0 {
		t.Fatalf("после перезаполнения выделение появилось само: %+v", got)
	}
	if len(*events) != 0 {
		t.Fatalf("добавление строк прислало событие выделения: %v", *events)
	}
}

// Удаление выделенной строки снимает выделение; удаление чужой — нет.
func TestSelection_RemovedRow(t *testing.T) {
	dg, oc, events := selGrid(t)
	dg.SetSelectedIndex(2)
	*events = nil

	oc.RemoveAt(0) // не выделенная
	if got := dg.SelectedItems(); len(got) != 1 {
		t.Fatalf("удаление чужой строки сняло выделение: %+v", got)
	}
	if len(*events) != 0 {
		t.Fatalf("удаление чужой строки прислало событие: %v", *events)
	}

	// Теперь выделенная (после сдвига — элемент N=2 стоит в строке 1).
	dg.SetSelectedIndex(1)
	*events = nil
	oc.RemoveAt(1)
	if got := dg.SelectedItems(); len(got) != 0 {
		t.Fatalf("после удаления выделенной строки выделение осталось: %+v", got)
	}
	if e := lastEvent(t, events); e.SelectedIndex != -1 {
		t.Fatalf("событие удаления: %+v, ждал пустой выбор", e)
	}
}
