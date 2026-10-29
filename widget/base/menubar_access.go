package base

// menubar_access.go — строка меню в дереве доступности: пункты верхнего
// уровня и пункты раскрытого подменю.
//
// До этого файла строка меню отдавалась скринридеру пустой полосой без детей:
// ни названий меню («Файл», «Правка»), ни пунктов открытого подменю в дереве не
// было, а нажать «Файл» программно можно было только кликом в центр всей
// полосы.

import (
	"image"
	"strconv"
	"strings"
	"sync/atomic"
)

// AccessInfo — контракт Accessible: полоса меню. Границы охватывают и
// раскрытое подменю: оно рисуется за пределами полосы, а поиск по точке
// спускается только в узлы, которые точку содержат.
func (mb *MenuBar) AccessInfo() AccessInfo {
	info := AccessInfo{Role: RoleMenuBar, Description: mb.GetToolTip(), Bounds: mb.Bounds()}
	if !mb.IsEnabled() {
		info.States = append(info.States, StateDisabled)
	}
	return info
}

// AccessChildren — контракт AccessChildrenProvider: пункты верхнего уровня
// (идентификатор «t0», «t1», …) и, пока подменю открыто, его пункты с
// идентификаторами «t1/0», «t1/2/0» (путь из PopupMenu.AccessChildren).
func (mb *MenuBar) AccessChildren() []AccessInfo {
	mb.mu.RLock()
	items := make([]MenuBarItem, len(mb.items))
	copy(items, mb.items)
	rects := append([]image.Rectangle(nil), mb.itemRects...)
	mb.mu.RUnlock()
	active := int(atomic.LoadInt32(&mb.activeIdx))

	out := make([]AccessInfo, 0, len(items))
	for i, it := range items {
		if i >= len(rects) {
			break
		}
		info := AccessInfo{
			Role:   RoleMenuItem,
			Name:   mnemonicLabel(it.Text, mb.UseMnemonics),
			Bounds: rects[i],
			ID:     "t" + strconv.Itoa(i),
		}
		if len(it.Items) > 0 {
			info.States = append(info.States, StateHasPopup)
			if i == active {
				info.States = append(info.States, StateExpanded)
			}
		}
		// Фокус — только у открытого меню: пока полоса просто лежит под
		// курсором, фокус клавиатуры остаётся там, где был.
		if i == active {
			info.States = append(info.States, StateFocused)
		}
		out = append(out, info)
	}
	if active >= 0 && mb.popup.IsOpen() {
		out = append(out, mb.popup.accessItems("t"+strconv.Itoa(active)+"/")...)
	}
	return out
}

// AccessAction — контракт AccessActionHandler.
//
// Пункт верхнего уровня («tN»): AccessInvoke нажимает его, как мышь, — меню
// раскрывается, раскрытое закрывается, пункт без подменю выполняется;
// AccessExpand и AccessCollapse открывают и закрывают подменю; AccessFocus и
// AccessSelect подсвечивают пункт, не раскрывая его. Пункты подменю
// («tN/…») передаются меню.
func (mb *MenuBar) AccessAction(id string, action AccessActionKind) bool {
	if !mb.IsEnabled() || !strings.HasPrefix(id, "t") {
		return false
	}
	top, rest, nested := strings.Cut(id[1:], "/")
	idx, err := strconv.Atoi(top)
	if err != nil || idx < 0 {
		return false
	}
	mb.mu.RLock()
	var item MenuBarItem
	valid := idx < len(mb.items)
	if valid {
		item = mb.items[idx]
	}
	mb.mu.RUnlock()
	if !valid {
		return false
	}
	active := int(atomic.LoadInt32(&mb.activeIdx))

	if nested {
		if active != idx || !mb.popup.IsOpen() {
			return false // подменю уже закрыли: путь устарел
		}
		return mb.popup.AccessAction(rest, action)
	}
	switch action {
	case AccessInvoke, AccessToggle:
		if active == idx {
			mb.closeSubmenu()
		} else {
			mb.openSubmenu(idx)
		}
	case AccessExpand:
		if len(item.Items) == 0 {
			return false
		}
		mb.openSubmenu(idx)
	case AccessCollapse:
		if active != idx {
			return false
		}
		mb.closeSubmenu()
	case AccessFocus, AccessSelect:
		mb.setHoverIdx(idx)
	default:
		return false
	}
	return true
}
