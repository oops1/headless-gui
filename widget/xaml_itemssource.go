package widget

// xaml_itemssource.go — ItemsSource="{Binding …}" у DataGrid и TreeView.
//
// Привязки XAML передают виджету строку: для текста, флажка или ползунка
// этого довольно. Источник строк таблицы или узлов дерева — коллекция, а не
// строка, и раньше такая привязка тихо превращалась в ничто: разметка
// `<DataGrid ItemsSource="{Binding Items}">` давала пустую таблицу, хотя
// модель была на месте. Коллекцию приходилось подавать кодом.
//
// Здесь привязка ItemsSource у этих двух виджетов получает сам объект.
// Источник ставится только когда он сменился: Refresh зовут на ЛЮБОЕ
// изменение модели, и повторная установка того же источника сбрасывала бы
// выделение и сортировку таблицы.

import (
	"strings"

	dgridPkg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// applyItemsSource ставит коллекцию источником строк или узлов. Возвращает
// true, если привязка — ItemsSource таблицы или дерева: тогда строкового
// пути для неё нет, и звать setWidgetProperty не нужно.
func (s *BindingScope) applyItemsSource(w Widget, prop string, v interface{}) bool {
	if !strings.EqualFold(prop, "ItemsSource") {
		return false
	}
	switch w.(type) {
	case *DataGridWidget, *TreeViewWidget:
	default:
		return false
	}
	// Не коллекция (CollectionView, срез, опечатка в пути) — источник не
	// трогаем: таблица с фильтром и сортировкой представления, подменённая
	// голой коллекцией, показывала бы не то, что задумано.
	oc, ok := v.(*dgridPkg.ObservableCollection)
	if !ok && v != nil {
		return true
	}
	switch t := w.(type) {
	case *DataGridWidget:
		if t.Grid.ItemsSource() != oc {
			t.Grid.SetItemsSource(oc)
		}
		return true
	case *TreeViewWidget:
		s.mu.Lock()
		if s.treeSrc == nil {
			s.treeSrc = map[*TreeViewWidget]*dgridPkg.ObservableCollection{}
		}
		prev, seen := s.treeSrc[t]
		s.treeSrc[t] = oc
		s.mu.Unlock()
		if !seen || prev != oc {
			t.Tree.SetItemsSource(oc)
		}
		return true
	}
	return false
}
