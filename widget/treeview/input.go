package treeview

import "image"

// ─── Mouse Events ──────────────────────────────────────────────────────────

// OnMouseButton обрабатывает нажатие/отпускание кнопки мыши.
// Возвращает true, если событие поглощено.
func (tv *TreeView) OnMouseButton(x, y int, button, pressed int) bool {
	if button != 0 { // только левая кнопка
		return false
	}

	b := tv.bounds
	pt := image.Pt(x, y)

	if !pt.In(b) {
		return false
	}

	isPressed := pressed != 0

	// ── Скроллбар ───────────────────────────────────────────────────────
	if tv.needsScrollbar() {
		sr := tv.scrollbarRect()

		if isPressed && pt.In(sr) {
			tr := tv.thumbRect()
			if pt.In(tr) {
				// Начало перетаскивания ползунка
				tv.thumbDragging = true
				tv.thumbDragStartY = y
				tv.thumbDragStartS = tv.scrollY
			} else {
				// Клик по треку — page up/down
				if y < tr.Min.Y {
					tv.scrollY -= b.Dy()
				} else {
					tv.scrollY += b.Dy()
				}
				tv.clampScroll()
			}
			return true
		}

		if !isPressed && tv.thumbDragging {
			tv.thumbDragging = false
			return true
		}
	}

	if !isPressed {
		return false
	}

	// ── Определяем строку под курсором ──────────────────────────────────
	ih := tv.itemH()
	flat := tv.visibleNodes()

	relY := y - b.Min.Y + tv.scrollY
	idx := relY / ih

	if idx < 0 || idx >= len(flat) {
		return true
	}

	fi := flat[idx]
	item := fi.item

	// ── Клик по стрелке → toggle expand ─────────────────────────────────
	arrowX := b.Min.X + 6 + fi.depth*tv.indentW()
	if x >= arrowX && x < arrowX+arrowZone && item.HasChildren() {
		tv.ToggleExpand(item)
		tv.dirty = true
		return true
	}

	// ── Выбор узла ──────────────────────────────────────────────────────
	old := tv.selectedItem
	tv.selectedItem = item
	if old != nil {
		old.IsSelected = false
	}
	item.IsSelected = true

	if old != item {
		tv.fireSelectedItemChanged(old, item)
	}

	// ── Двойной клик → expand + invoke ──────────────────────────────────
	now := nowMs()
	if tv.lastClickIdx == idx && now-tv.lastClickTime < 400 {
		if item.HasChildren() {
			tv.ToggleExpand(item)
		}
		if tv.OnItemInvoked != nil {
			tv.OnItemInvoked(ItemInvokedEvent{Item: item})
		}
		tv.lastClickTime = 0
	} else {
		tv.lastClickTime = now
		tv.lastClickIdx = idx
	}

	return true
}

// OnMouseMove обрабатывает перемещение мыши.
func (tv *TreeView) OnMouseMove(x, y int) {
	b := tv.bounds
	pt := image.Pt(x, y)

	// Перетаскивание ползунка скроллбара
	if tv.thumbDragging {
		sr := tv.scrollbarRect()
		trackH := sr.Dy()
		tr := tv.thumbRect()
		thumbH := tr.Dy()
		maxS := tv.maxScrollY()

		if trackH > thumbH && maxS > 0 {
			dy := y - tv.thumbDragStartY
			tv.scrollY = tv.thumbDragStartS + dy*maxS/(trackH-thumbH)
			tv.clampScroll()
		}
		return
	}

	if !pt.In(b) {
		tv.hoverIdx = -1
		tv.thumbHovered = false
		return
	}

	// Hover на скроллбаре
	if tv.needsScrollbar() {
		sr := tv.scrollbarRect()
		if pt.In(sr) {
			tr := tv.thumbRect()
			tv.thumbHovered = pt.In(tr)
			tv.hoverIdx = -1
			return
		}
	}
	tv.thumbHovered = false

	// Hover на строке
	ih := tv.itemH()
	relY := y - b.Min.Y + tv.scrollY
	idx := relY / ih

	flat := tv.visibleNodes()
	if idx >= 0 && idx < len(flat) {
		tv.hoverIdx = idx
	} else {
		tv.hoverIdx = -1
	}
}

// ─── Keyboard Events ───────────────────────────────────────────────────────

// KeyCode — аппаратно-независимый код клавиши (дублируется из widget для
// избежания циклического импорта).
const (
	keyUp     = 38
	keyDown   = 40
	keyLeft   = 37
	keyRight  = 39
	keyHome   = 36
	keyEnd    = 35
	keyEnter  = 13
	keySpace  = 32
	keyPageUp = 33
	keyPageDn = 34
)

// OnKeyEvent обрабатывает клавиатурный ввод.
func (tv *TreeView) OnKeyEvent(keyCode int, r rune, pressed bool, shift, ctrl bool) {
	if !pressed {
		return
	}

	flat := tv.visibleNodes()
	if len(flat) == 0 {
		return
	}

	curIdx := -1
	if tv.selectedItem != nil {
		curIdx = tv.indexOfItem(tv.selectedItem, flat)
	}

	switch keyCode {
	case keyDown:
		// Следующий элемент
		newIdx := curIdx + 1
		if newIdx >= len(flat) {
			newIdx = len(flat) - 1
		}
		if newIdx >= 0 {
			tv.selectByIndex(newIdx, flat)
			tv.ensureVisible(newIdx)
		}

	case keyUp:
		// Предыдущий элемент
		newIdx := curIdx - 1
		if newIdx < 0 {
			newIdx = 0
		}
		tv.selectByIndex(newIdx, flat)
		tv.ensureVisible(newIdx)

	case keyRight:
		// Раскрыть узел или перейти к первому ребёнку
		if tv.selectedItem != nil {
			if tv.selectedItem.HasChildren() && !tv.selectedItem.Expanded {
				tv.ExpandItem(tv.selectedItem)
			} else if tv.selectedItem.Expanded && tv.selectedItem.HasChildren() {
				// Перейти к первому ребёнку
				flat2 := tv.visibleNodes()
				newIdx := tv.indexOfItem(tv.selectedItem, flat2) + 1
				if newIdx < len(flat2) {
					tv.selectByIndex(newIdx, flat2)
					tv.ensureVisible(newIdx)
				}
			}
		}

	case keyLeft:
		// Свернуть узел или перейти к родителю
		if tv.selectedItem != nil {
			if tv.selectedItem.Expanded && tv.selectedItem.HasChildren() {
				tv.CollapseItem(tv.selectedItem)
			} else if tv.selectedItem.parent != nil {
				// Перейти к родителю
				flat2 := tv.visibleNodes()
				parentIdx := tv.indexOfItem(tv.selectedItem.parent, flat2)
				if parentIdx >= 0 {
					tv.selectByIndex(parentIdx, flat2)
					tv.ensureVisible(parentIdx)
				}
			}
		}

	case keyHome:
		// Первый элемент
		if len(flat) > 0 {
			tv.selectByIndex(0, flat)
			tv.ensureVisible(0)
		}

	case keyEnd:
		// Последний элемент
		last := len(flat) - 1
		if last >= 0 {
			tv.selectByIndex(last, flat)
			tv.ensureVisible(last)
		}

	case keyPageUp:
		// На страницу вверх
		pageSize := tv.bounds.Dy() / tv.itemH()
		newIdx := curIdx - pageSize
		if newIdx < 0 {
			newIdx = 0
		}
		tv.selectByIndex(newIdx, flat)
		tv.ensureVisible(newIdx)

	case keyPageDn:
		// На страницу вниз
		pageSize := tv.bounds.Dy() / tv.itemH()
		newIdx := curIdx + pageSize
		if newIdx >= len(flat) {
			newIdx = len(flat) - 1
		}
		if newIdx >= 0 {
			tv.selectByIndex(newIdx, flat)
			tv.ensureVisible(newIdx)
		}

	case keyEnter, keySpace:
		// Toggle expand
		if tv.selectedItem != nil && tv.selectedItem.HasChildren() {
			tv.ToggleExpand(tv.selectedItem)
		}
		// Invoke
		if tv.selectedItem != nil && tv.OnItemInvoked != nil {
			tv.OnItemInvoked(ItemInvokedEvent{Item: tv.selectedItem})
		}
	}
}

// selectByIndex выбирает элемент по индексу в flat-списке.
func (tv *TreeView) selectByIndex(idx int, flat []flatItem) {
	if idx < 0 || idx >= len(flat) {
		return
	}
	item := flat[idx].item
	old := tv.selectedItem
	tv.selectedItem = item
	if old != nil {
		old.IsSelected = false
	}
	item.IsSelected = true

	if old != item {
		tv.fireSelectedItemChanged(old, item)
	}
}
