// hittest.go — какой узел лежит под точкой.
//
// Формула «строка по Y» жила только внутри обработчиков мыши: высота строки,
// прокрутка, плоский список видимых узлов. Приложению, которому нужно собрать
// контекстное меню по правой кнопке или принять перетаскивание, приходилось
// повторять её у себя — а она меняется вместе с раскладкой дерева.
package treeview

// ItemAtY возвращает узел, показанный в строке под координатой y, или nil.
//
// Координата — та же, в которой приходят события мыши (координаты холста).
// Точка вне дерева или ниже последней строки даёт nil.
func (tv *TreeView) ItemAtY(y int) *TreeViewItem {
	item, _ := tv.itemAndIndexAtY(y)
	return item
}

// IndexAtY возвращает номер видимой строки под координатой y, или -1.
//
// Номер — в плоском списке РАЗВЁРНУТЫХ узлов, тот же, что использует
// отрисовка; свёрнутые ветки в нём не участвуют.
func (tv *TreeView) IndexAtY(y int) int {
	_, idx := tv.itemAndIndexAtY(y)
	return idx
}

func (tv *TreeView) itemAndIndexAtY(y int) (*TreeViewItem, int) {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	b := tv.bounds
	if b.Empty() || y < b.Min.Y || y >= b.Max.Y {
		return nil, -1
	}
	ih := tv.itemH()
	if ih <= 0 {
		return nil, -1
	}
	flat := tv.visibleNodesLocked()
	idx := (y - b.Min.Y + tv.scrollY) / ih
	if idx < 0 || idx >= len(flat) {
		return nil, -1
	}
	return flat[idx].item, idx
}
