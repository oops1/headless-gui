package treeview

// ─── События TreeView ──────────────────────────────────────────────────────

// SelectedItemChangedEvent — событие смены выбранного узла.
type SelectedItemChangedEvent struct {
	OldItem *TreeViewItem
	NewItem *TreeViewItem
}

// ExpandedEvent — событие раскрытия узла.
type ExpandedEvent struct {
	Item *TreeViewItem
}

// CollapsedEvent — событие свёртывания узла.
type CollapsedEvent struct {
	Item *TreeViewItem
}

// ItemInvokedEvent — событие двойного клика по узлу.
type ItemInvokedEvent struct {
	Item *TreeViewItem
}

// SelectedItemChangedHandler — обработчик смены выделения.
type SelectedItemChangedHandler func(e SelectedItemChangedEvent)

// ExpandedHandler — обработчик раскрытия.
type ExpandedHandler func(e ExpandedEvent)

// CollapsedHandler — обработчик свёртывания.
type CollapsedHandler func(e CollapsedEvent)

// ItemInvokedHandler — обработчик двойного клика.
type ItemInvokedHandler func(e ItemInvokedEvent)
