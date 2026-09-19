// selection.go — выделение узлов: один или набор (GG-86).
//
// У дерева было ровно одно выделенное состояние — selectedItem, — и команда
// вида «выбрать все устаревшие ветки, чтобы удалить их разом» упиралась в это:
// пометить удавалось только один узел. У таблицы такой режим есть
// (datagrid.SelectionExtended), у дерева не было.
package treeview

// SelectionMode — сколько узлов можно выбрать разом.
type SelectionMode int

const (
	// SelectionSingle — один узел (прежнее и умолчательное поведение).
	SelectionSingle SelectionMode = iota
	// SelectionExtended — несколько: Ctrl добавляет и снимает по одному,
	// Shift выбирает диапазон, Shift+↑/↓ расширяет его с клавиатуры.
	SelectionExtended
)

// SelectionChangedEvent — набор выбранных узлов сменился.
type SelectionChangedEvent struct {
	// Items — выбранные узлы в порядке выбора (пусто — выделения нет).
	Items []*TreeViewItem
	// Current — узел, на котором стоит курсор выбора: последний, которого
	// коснулись мышью или клавишами.
	Current *TreeViewItem
}

// SelectionChangedHandler — обработчик смены набора.
type SelectionChangedHandler func(e SelectionChangedEvent)

// SelectedItems возвращает выбранные узлы (копия, в порядке выбора).
func (tv *TreeView) SelectedItems() []*TreeViewItem {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return append([]*TreeViewItem(nil), tv.selection...)
}

// SetSelectedItems задаёт набор выбранных узлов.
//
// В режиме SelectionSingle берётся последний из списка — набора там быть не
// может. Пустой список снимает выделение.
func (tv *TreeView) SetSelectedItems(items []*TreeViewItem) {
	var current *TreeViewItem
	if len(items) > 0 {
		current = items[len(items)-1]
	}
	if tv.SelectionMode != SelectionExtended && current != nil {
		items = []*TreeViewItem{current}
	}
	tv.applySelection(items, current)
}

// IsItemSelected сообщает, выбран ли узел.
func (tv *TreeView) IsItemSelected(item *TreeViewItem) bool {
	if item == nil {
		return false
	}
	tv.mu.Lock()
	defer tv.mu.Unlock()
	for _, it := range tv.selection {
		if it == item {
			return true
		}
	}
	return false
}

// applySelection — общая механика смены выделения: снять прежние отметки,
// поставить новые, обновить курсор выбора, пометить перерисовку и позвать
// обработчики.
//
// Зовётся без tv.mu — как и весь разбор ввода; публичные методы берут замок
// только на чтение своих полей.
func (tv *TreeView) applySelection(items []*TreeViewItem, current *TreeViewItem) {
	// Уникализируем, сохраняя порядок: Ctrl+клик по уже выбранному не должен
	// давать его в наборе дважды.
	seen := make(map[*TreeViewItem]bool, len(items))
	uniq := make([]*TreeViewItem, 0, len(items))
	for _, it := range items {
		if it == nil || seen[it] {
			continue
		}
		seen[it] = true
		uniq = append(uniq, it)
	}

	old := tv.selection
	oldCurrent := tv.selectedItem
	sameSet := len(old) == len(uniq)
	if sameSet {
		for i := range old {
			if old[i] != uniq[i] {
				sameSet = false
				break
			}
		}
	}
	if sameSet && oldCurrent == current {
		return
	}

	flat := tv.visibleNodes()
	was := make(map[*TreeViewItem]bool, len(old))
	touched := 0
	for _, it := range old {
		was[it] = true
		if !seen[it] {
			it.IsSelected = false
			tv.markRowDirty(tv.indexOfItem(it, flat))
			touched++
		}
	}
	for _, it := range uniq {
		if !was[it] {
			it.IsSelected = true
			tv.markRowDirty(tv.indexOfItem(it, flat))
			touched++
		}
	}
	// Много строк поменяло вид — дешевле перерисовать вьюпорт целиком, чем
	// собирать длинный список прямоугольников.
	if touched > 8 {
		tv.markFullDirty()
	}

	tv.selection = uniq
	tv.selectedItem = current
	if current != nil && !seen[current] {
		// Курсор выбора на узле, которого в наборе нет (Ctrl сняли отметку):
		// подсветки у него быть не должно.
		current.IsSelected = false
	}

	if oldCurrent != current {
		tv.fireSelectedItemChanged(oldCurrent, current)
	}
	if !sameSet {
		tv.fireSelectionChanged()
	}
}

// fireSelectionChanged сообщает о смене набора.
func (tv *TreeView) fireSelectionChanged() {
	if tv.OnSelectionChanged == nil {
		return
	}
	tv.OnSelectionChanged(SelectionChangedEvent{
		Items:   append([]*TreeViewItem(nil), tv.selection...),
		Current: tv.selectedItem,
	})
}

// clickSelect — выбор узла мышью с учётом модификаторов.
func (tv *TreeView) clickSelect(item *TreeViewItem, idx int, flat []flatItem, shift, ctrl bool) {
	if item == nil {
		return
	}
	if tv.SelectionMode != SelectionExtended || (!shift && !ctrl) {
		tv.anchor = item
		tv.applySelection([]*TreeViewItem{item}, item)
		return
	}

	if ctrl {
		// Ctrl переключает узел: был в наборе — выходит из него.
		next := make([]*TreeViewItem, 0, len(tv.selection)+1)
		found := false
		for _, it := range tv.selection {
			if it == item {
				found = true
				continue
			}
			next = append(next, it)
		}
		if !found {
			next = append(next, item)
		}
		tv.anchor = item
		tv.applySelection(next, item)
		return
	}

	// Shift — диапазон от якоря до этой строки.
	tv.applySelection(tv.rangeFrom(tv.anchorIndex(flat), idx, flat), item)
}

// anchorIndex — индекс якоря диапазона в плоском списке (-1, если якоря нет).
func (tv *TreeView) anchorIndex(flat []flatItem) int {
	anchor := tv.anchor
	if anchor == nil {
		anchor = tv.selectedItem
	}
	if anchor == nil {
		return -1
	}
	tv.anchor = anchor
	return tv.indexOfItem(anchor, flat)
}

// rangeFrom — узлы от from до to включительно (в любом порядке индексов).
func (tv *TreeView) rangeFrom(from, to int, flat []flatItem) []*TreeViewItem {
	if to < 0 || to >= len(flat) {
		return nil
	}
	if from < 0 {
		from = to
	}
	if from > to {
		from, to = to, from
	}
	out := make([]*TreeViewItem, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, flat[i].item)
	}
	return out
}

// moveSelection — перевод курсора выбора на строку idx с клавиатуры.
// shift в режиме набора расширяет диапазон от якоря.
func (tv *TreeView) moveSelection(idx int, flat []flatItem, shift bool) {
	if idx < 0 || idx >= len(flat) {
		return
	}
	item := flat[idx].item
	if tv.SelectionMode != SelectionExtended || !shift {
		tv.anchor = item
		tv.applySelection([]*TreeViewItem{item}, item)
		return
	}
	tv.applySelection(tv.rangeFrom(tv.anchorIndex(flat), idx, flat), item)
}

// clearSelection снимает выделение целиком (дерево перестроили).
func (tv *TreeView) clearSelection() {
	tv.anchor = nil
	tv.applySelection(nil, nil)
}
