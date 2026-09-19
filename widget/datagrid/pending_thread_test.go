package datagrid

import (
	"sync"
	"testing"
)

// Отложенные колбэки (dg.pending) принадлежат UI-потоку: там лежит, среди
// прочего, запись отредактированной ячейки в модель. Обработчик изменения
// коллекции работает в горутине, которая коллекцию и меняла, — фоновой.
// Раньше он звал firePending и выполнял чужие отложенные действия у себя:
// фоновая горутина писала поля элемента, пока UI-поток читал их в Draw
// (гонка ловилась тестом SEC-5 под -race).

func TestPending_CollectionChangeDoesNotRunUIQueue(t *testing.T) {
	dg := New()
	oc := NewObservableCollection()
	dg.SetItemsSource(oc)

	var mu sync.Mutex
	ran := 0
	dg.mu.Lock()
	dg.pending = append(dg.pending, func() {
		mu.Lock()
		ran++
		mu.Unlock()
	})
	dg.mu.Unlock()

	// Фоновое изменение коллекции: очередь UI-потока трогать нельзя.
	done := make(chan struct{})
	go func() {
		defer close(done)
		oc.Add(struct{ Name string }{"строка"})
	}()
	<-done

	mu.Lock()
	got := ran
	mu.Unlock()
	if got != 0 {
		t.Fatalf("изменение коллекции выполнило %d отложенных колбэков UI-потока", got)
	}

	// UI-поток забирает свою очередь сам.
	dg.firePending()
	mu.Lock()
	got = ran
	mu.Unlock()
	if got != 1 {
		t.Errorf("после firePending выполнено %d колбэков, ждал 1", got)
	}
}

// Своё уведомление обработчик коллекции по-прежнему шлёт: сброс коллекции
// снимает выделение, и приложение обязано об этом узнать (GG-81).
func TestPending_CollectionResetStillNotifiesSelection(t *testing.T) {
	dg := New()
	oc := NewObservableCollection()
	oc.Add(struct{ Name string }{"раз"})
	oc.Add(struct{ Name string }{"два"})
	dg.SetItemsSource(oc)

	var mu sync.Mutex
	var events []int
	dg.OnSelectionChanged = func(ev SelectionChangedEvent) {
		mu.Lock()
		events = append(events, ev.SelectedIndex)
		mu.Unlock()
	}
	dg.SetSelectedIndex(1)

	mu.Lock()
	events = nil
	mu.Unlock()

	oc.Clear()

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 || events[0] != -1 {
		t.Errorf("после сброса коллекции события: %v, ждал одно с индексом -1", events)
	}
}
