// dragdrop.go — перетаскивание узлов мышью.
//
// Порядок и вложенность узлов задавались только из кода: перетащить ветку в
// другую группу или переставить репозиторий выше было нечем. Приложение,
// которому это нужно, оставалось либо без функции, либо со своим разбором
// мыши поверх дерева — вместе с формулой «какой узел под этим Y», подсветкой
// места вставки и проверкой, что узел не тащат в собственного потомка.
package treeview

// DropPosition — куда встанет перетаскиваемый узел относительно цели.
type DropPosition int

const (
	// DropInside — внутрь цели, последним ребёнком.
	DropInside DropPosition = iota
	// DropBefore — перед целью, к тому же родителю.
	DropBefore
	// DropAfter — после цели, к тому же родителю.
	DropAfter
)

// dragThresholdY — на сколько нужно увести курсор, чтобы нажатие стало
// перетаскиванием, а не щелчком.
//
// Порог нужен, потому что мышь дрожит: без него всякий щелчок по узлу
// заканчивался бы микроперестановкой дерева.
const dragThresholdY = 4

// String — читаемое имя позиции (для сообщений и отладки).
func (p DropPosition) String() string {
	switch p {
	case DropBefore:
		return "перед"
	case DropAfter:
		return "после"
	default:
		return "внутрь"
	}
}

// CanDrop сообщает, допустим ли перенос узла к цели.
//
// Внутрь самого себя и внутрь собственного потомка перетащить нельзя: дерево
// перестало бы быть деревом, а поддерево — потерялось бы вместе с узлом.
// Проверка живёт ЗДЕСЬ, а не у приложения: ошибиться в ней легко, а
// последствия у неё не «неудобно», а «часть данных исчезла с экрана».
func (tv *TreeView) CanDrop(dragged, target *TreeViewItem, pos DropPosition) bool {
	if dragged == nil || target == nil || dragged == target {
		return false
	}
	if isDescendant(target, dragged) {
		return false
	}
	if tv.CanDropNode != nil {
		return tv.CanDropNode(dragged, target, pos)
	}
	return true
}

// isDescendant сообщает, лежит ли node внутри поддерева root.
func isDescendant(node, root *TreeViewItem) bool {
	for p := node; p != nil; p = p.parent {
		if p == root {
			return true
		}
	}
	return false
}

// MoveNode переносит узел к цели и возвращает, состоялся ли перенос.
//
// Публичный: приложение, которое ведёт свою модель, вызывает его само после
// того, как переставило данные, — или не вызывает вовсе, если перестроит
// дерево заново.
func (tv *TreeView) MoveNode(dragged, target *TreeViewItem, pos DropPosition) bool {
	if !tv.CanDrop(dragged, target, pos) {
		return false
	}
	tv.detach(dragged)

	switch pos {
	case DropInside:
		target.AddChild(dragged)
		target.Expanded = true // иначе узел «исчезнет» внутрь свёрнутой ветки
	default:
		tv.insertNear(dragged, target, pos == DropAfter)
	}

	tv.mu.Lock()
	tv.dirty = true // состав видимых строк изменился
	tv.mu.Unlock()
	tv.markFullDirty()
	return true
}

// detach вынимает узел из его нынешнего места.
func (tv *TreeView) detach(item *TreeViewItem) {
	if p := item.parent; p != nil {
		p.RemoveChild(item)
		return
	}
	for i, r := range tv.roots {
		if r == item {
			tv.roots = append(tv.roots[:i], tv.roots[i+1:]...)
			break
		}
	}
}

// insertNear ставит узел рядом с целью — до или после неё.
func (tv *TreeView) insertNear(item, target *TreeViewItem, after bool) {
	if p := target.parent; p != nil {
		idx := 0
		for i, c := range p.Children {
			if c == target {
				idx = i
				break
			}
		}
		if after {
			idx++
		}
		p.InsertChild(idx, item)
		return
	}

	// Цель — корневой узел: встаём рядом с ней в списке корней.
	idx := len(tv.roots)
	for i, r := range tv.roots {
		if r == target {
			idx = i
			break
		}
	}
	if after {
		idx++
	}
	if idx > len(tv.roots) {
		idx = len(tv.roots)
	}
	item.parent = nil
	tv.roots = append(tv.roots, nil)
	copy(tv.roots[idx+1:], tv.roots[idx:])
	tv.roots[idx] = item
	item.setOwnerRecursive(tv)
}

// dropTargetAt определяет цель и место вставки по координате Y.
//
// Верхняя и нижняя трети строки означают «перед» и «после», середина —
// «внутрь». Трети, а не половины: без середины узел нельзя было бы положить
// ВНУТРЬ ветки, а именно это и нужно, чтобы перенести репозиторий в группу.
func (tv *TreeView) dropTargetAt(y int) (*TreeViewItem, DropPosition, int) {
	b := tv.bounds
	ih := tv.itemH()
	if b.Empty() || ih <= 0 {
		return nil, DropInside, -1
	}
	flat := tv.visibleNodes()
	rel := y - b.Min.Y + tv.scrollY
	idx := rel / ih
	if idx < 0 || idx >= len(flat) {
		return nil, DropInside, -1
	}
	within := rel - idx*ih
	switch {
	case within < ih/3:
		return flat[idx].item, DropBefore, idx
	case within > ih-ih/3:
		return flat[idx].item, DropAfter, idx
	default:
		return flat[idx].item, DropInside, idx
	}
}

// beginNodeDrag запоминает нажатие как возможное начало переноса.
func (tv *TreeView) beginNodeDrag(item *TreeViewItem, y int) {
	if !tv.CanUserDragNodes || item == nil {
		return
	}
	tv.dragItem = item
	tv.dragStartY = y
	tv.dragging = false
	tv.dropIdx = -1
}

// dragNodeTo двигает начатый перенос; сообщает, идёт ли он.
func (tv *TreeView) dragNodeTo(y int) bool {
	if tv.dragItem == nil {
		return false
	}
	if !tv.dragging {
		d := y - tv.dragStartY
		if d < 0 {
			d = -d
		}
		if d < dragThresholdY {
			return false
		}
		tv.dragging = true
	}
	target, pos, idx := tv.dropTargetAt(y)
	tv.dropTarget, tv.dropPos, tv.dropIdx = target, pos, idx
	if target == nil || !tv.canDropLocked(tv.dragItem, target, pos) {
		tv.dropIdx = -1
	}
	tv.markFullDirty()
	return true
}

// canDropLocked — CanDrop без взятия замка (колбэк приложения зовётся ниже,
// уже вне его; здесь только структурные проверки).
func (tv *TreeView) canDropLocked(dragged, target *TreeViewItem, pos DropPosition) bool {
	if dragged == nil || target == nil || dragged == target {
		return false
	}
	return !isDescendant(target, dragged)
}

// finishNodeDrag завершает перенос; сообщает, был ли он.
func (tv *TreeView) finishNodeDrag() bool {
	item, target, pos := tv.dragItem, tv.dropTarget, tv.dropPos
	dragging := tv.dragging
	tv.dragItem, tv.dropTarget, tv.dragging, tv.dropIdx = nil, nil, false, -1

	if !dragging {
		return false
	}
	tv.markFullDirty()
	if item == nil || target == nil || !tv.CanDrop(item, target, pos) {
		return true // перенос был, но окончился ничем — щелчком его считать нельзя
	}

	// Колбэк первым: приложение, ведущее свою модель, скажет «справился сам»,
	// и дерево не станет переставлять узлы у него за спиной.
	if tv.OnNodeDrop != nil && tv.OnNodeDrop(item, target, pos) {
		return true
	}
	tv.MoveNode(item, target, pos)
	return true
}

// drawDropIndicator показывает, куда встанет перетаскиваемый узел.
//
// Полоса между строками для «перед» и «после», двойная рамка для «внутрь».
// Три вида места вставки обязаны различаться с одного взгляда: одной полосой
// снизу «после» не отличить от «внутрь», а одинарной рамкой — «внутрь» от
// обычной подсветки строки.
func (tv *TreeView) drawDropIndicator(ctx DrawContextBridge, ih int) {
	if !tv.dragging || tv.dropIdx < 0 {
		return
	}
	b := tv.bounds
	w := b.Dx()
	if tv.needsScrollbar() {
		w -= scrollbarWidth
	}
	y := b.Min.Y + tv.dropIdx*ih - tv.scrollY
	col := tv.Theme.SelectColor

	switch tv.dropPos {
	case DropBefore:
		ctx.FillRect(b.Min.X, y, w, 2, col)
	case DropAfter:
		ctx.FillRect(b.Min.X, y+ih-2, w, 2, col)
	default:
		// Двойная рамка, а не заливка: заливка легла бы поверх подписи и
		// сделала бы её нечитаемой, а одинарная рамка не отличалась бы от
		// обычной подсветки строки.
		ctx.DrawBorder(b.Min.X, y, w, ih, col)
		ctx.DrawBorder(b.Min.X+1, y+1, w-2, ih-2, col)
	}
}
