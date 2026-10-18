package widget

// popupmenu_access.go — всплывающее меню в дереве доступности.
//
// Меню рисуется оверлеем поверх всего UI и не входит в Children() виджета,
// поэтому скринридер раньше не видел ни открытого меню, ни его пунктов: поиск
// элемента под курсором (UIA ElementFromPoint, AT-SPI GetAccessibleAtPoint)
// возвращал то, что лежит ПОД меню, а клавиатурная подсветка пункта до него не
// доходила. Теперь открытое меню — узел RoleMenu, а его пункты и пункты
// открытых подменю — дочерние узлы RoleMenuItem с границами на экране.
//
// Пункт под курсором или выбранный клавиатурой (плашка наведения) отдаётся с
// состоянием focused: у меню фокус и есть эта плашка. Действия над узлами
// (нажать, подсветить, раскрыть подменю) доставляет AccessAction.

import (
	"image"
	"strconv"
	"strings"
	"sync/atomic"
)

// AccessInfo — контракт Accessible: роль «меню». Закрытое меню остаётся в
// дереве узлом без границ с состоянием offscreen (так делают и GTK, и Qt: меню
// существует всегда, показано оно или нет); открытое охватывает себя и все
// открытые подменю.
func (m *PopupMenu) AccessInfo() AccessInfo {
	info := AccessInfo{Role: RoleMenu, Description: m.GetToolTip()}
	if !m.IsEnabled() {
		info.States = append(info.States, StateDisabled)
	}
	if m.IsOpen() {
		info.Bounds = m.Bounds()
		info.States = append(info.States, StatePopup)
	} else {
		info.Bounds = m.Base.Bounds()
		info.States = append(info.States, StateOffscreen)
	}
	return info
}

// AccessChildren — контракт AccessChildrenProvider: пункты открытого меню и
// всех его открытых подменю подряд, сверху вниз, уровень за уровнем. Идентификатор
// пункта — путь индексов: «2» — третий пункт меню, «2/0» — первый пункт
// подменю, открытого из третьего. Разделители в список не входят, но
// занимают номер: путь пункта не зависит от них.
func (m *PopupMenu) AccessChildren() []AccessInfo {
	if !m.IsOpen() {
		return nil
	}
	return m.accessItems("")
}

// accessItems собирает узлы пунктов этого уровня и открытого ниже подменю.
func (m *PopupMenu) accessItems(prefix string) []AccessInfo {
	l := m.layout()
	view := image.Rect(l.x, l.viewTop, l.x+l.w, l.viewBottom)
	hover := int(atomic.LoadInt32(&m.hoverIdx))
	child, childIdx := m.openChildOf()

	m.mu.RLock()
	items := make([]MenuItem, len(m.items))
	copy(items, m.items)
	itemH, sepH, mnemonics := m.ItemHeight, m.SeparatorH, m.UseMnemonics
	m.mu.RUnlock()

	out := make([]AccessInfo, 0, len(items))
	y := l.itemsTop
	for i, it := range items {
		h := itemH
		if it.Separator {
			h = sepH
		}
		row := image.Rect(l.x, y, l.x+l.w, y+h)
		y += h
		if it.Separator {
			continue
		}
		info := AccessInfo{
			Role:        RoleMenuItem,
			Name:        mnemonicLabel(it.Text, mnemonics),
			Description: it.Shortcut,
			Bounds:      row,
			ID:          prefix + strconv.Itoa(i),
			States:      []string{StatePopup},
		}
		if vis := row.Intersect(view); vis.Empty() {
			info.States = append(info.States, StateOffscreen)
		} else if vis != row {
			info.Visible = vis
		}
		if it.Disabled {
			info.States = append(info.States, StateDisabled)
		}
		if it.Checkable {
			info.States = append(info.States, StateCheckable)
			if it.Checked {
				info.States = append(info.States, StateChecked)
			}
		}
		if len(it.SubItems) > 0 {
			info.States = append(info.States, StateHasPopup)
			if child != nil && childIdx == i {
				info.States = append(info.States, StateExpanded)
			}
		}
		if i == hover {
			info.States = append(info.States, StateFocused)
		}
		out = append(out, info)
	}
	if child != nil {
		out = append(out, child.accessItems(prefix+strconv.Itoa(childIdx)+"/")...)
	}
	return out
}

// AccessAction — контракт AccessActionHandler: действие скринридера над
// пунктом меню. id — путь из AccessChildren.
//
//   - AccessInvoke и AccessToggle выполняют пункт так же, как Enter: обычный
//     пункт закрывает меню и вызывает OnClick, пункт с подменю раскрывает его;
//   - AccessFocus и AccessSelect ставят на пункт плашку наведения, как
//     стрелка клавиатуры, — меню остаётся открытым;
//   - AccessExpand и AccessCollapse раскрывают и сворачивают подменю пункта.
//
// Недоступный пункт и пункт, которого уже нет в меню, действий не принимают.
func (m *PopupMenu) AccessAction(id string, action AccessActionKind) bool {
	if !m.IsOpen() || !m.IsEnabled() {
		return false
	}
	lvl, idx, ok := m.accessLevel(id)
	if !ok {
		return false
	}
	lvl.mu.RLock()
	it := lvl.items[idx]
	lvl.mu.RUnlock()
	if it.Separator || it.Disabled {
		return false
	}
	switch action {
	case AccessInvoke, AccessToggle:
		lvl.activateItem(idx)
	case AccessFocus, AccessSelect:
		lvl.ensureVisible(idx)
		lvl.setHoverIdx(idx)
	case AccessExpand:
		if len(it.SubItems) == 0 {
			return false
		}
		lvl.setHoverIdx(idx)
		lvl.openChild(idx)
	case AccessCollapse:
		if c, ci := lvl.openChildOf(); c == nil || ci != idx {
			return false
		}
		lvl.closeChild()
	default:
		return false
	}
	return true
}

// accessLevel разбирает путь пункта и возвращает уровень меню и индекс пункта
// в нём. Открытое подменю обязано существовать: путь, по которому его уже
// закрыли, — устаревший.
func (m *PopupMenu) accessLevel(id string) (lvl *PopupMenu, idx int, ok bool) {
	lvl = m
	parts := strings.Split(id, "/")
	for n, p := range parts {
		i, err := strconv.Atoi(p)
		if err != nil || i < 0 {
			return nil, 0, false
		}
		lvl.mu.RLock()
		valid := i < len(lvl.items)
		lvl.mu.RUnlock()
		if !valid {
			return nil, 0, false
		}
		if n == len(parts)-1 {
			return lvl, i, true
		}
		c, ci := lvl.openChildOf()
		if c == nil || ci != i {
			return nil, 0, false
		}
		lvl = c
	}
	return nil, 0, false
}
