// focus_taskbar.go — обход фокуса и рамка фокуса на уровне панели задач.
package desktop

import (
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

var _ FocusNavigator = (*Taskbar)(nil)

// Children возвращает детей панели в порядке областей — виджеты, «Пуск»,
// приложения, трей, — а не в порядке добавления. Именно по Children движок обходит дерево
// при Tab: оболочка, добавившая трей раньше приложений, иначе получила бы
// обход справа налево. Дети, не попавшие ни в одну область, идут в конце.
func (t *Taskbar) Children() []widget.Widget {
	base := t.Base.Children()
	if len(base) < 2 {
		return base
	}
	out := make([]widget.Widget, 0, len(base))
	inSlot := make(map[widget.Widget]struct{}, len(base))
	for _, s := range slotOrder {
		for _, it := range t.slots[s] {
			out = append(out, it)
			inSlot[it] = struct{}{}
		}
	}
	for _, c := range base {
		if _, ok := inSlot[c]; !ok {
			out = append(out, c)
		}
	}
	return out
}

// slotFocusables собирает элементы области, до которых можно дойти: в порядке
// обхода и только те, у кого есть место на панели.
func (t *Taskbar) slotFocusables(slot Slot) []widget.Widget {
	var out []widget.Widget
	for _, it := range t.slots[slot] {
		for _, w := range widget.CollectFocusables(it) {
			if !w.Bounds().Empty() {
				out = append(out, w)
			}
		}
	}
	return out
}

// MoveFocus реализует FocusNavigator: переносит фокус от from к соседу по
// той же области панели. Область не зацикливается: у её края стрелка ничего
// не делает, а Tab вынесет фокус к следующей области.
func (t *Taskbar) MoveFocus(from widget.Widget, move FocusMove) bool {
	for _, slot := range slotOrder {
		list := t.slotFocusables(slot)
		idx := -1
		for i, w := range list {
			if w == from {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		target := idx
		switch move {
		case FocusPrev:
			target = idx - 1
		case FocusNext:
			target = idx + 1
		case FocusFirst:
			target = 0
		case FocusLast:
			target = len(list) - 1
		}
		if target < 0 || target >= len(list) || target == idx {
			return false
		}
		moveFocusTo(from, list[target])
		return true
	}
	return false
}

// drawFocus рисует рамку фокуса поверх элементов. Рамка принадлежит панели, а
// не элементам: у всех она одна и та же, и её толщина и цвет решаются в одном
// месте (PaintFocusRing).
func (t *Taskbar) drawFocus(ctx widget.DrawContext) {
	bar := t.Bounds()
	if bar.Empty() {
		return
	}
	base := t.style("", theme.StateNormal)
	for _, w := range widget.CollectFocusables(t) {
		paintFocusOf(ctx, w, t.tm, base, bar)
	}
}
