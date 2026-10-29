// contextmenuat.go — контекстное меню, собираемое под точкой курсора.
//
// SetContextMenu вешает на виджет ОДНО готовое меню. Списку и таблице этого
// мало: действия над файлом, коммитом и веткой зависят от того, по какой
// строке щёлкнули, а строка виджету заранее не известна. Приложение обходило
// это, разбирая правую кнопку самостоятельно и повторяя формулу «какая строка
// под этим Y».
//
// Здесь виджет получает возможность собрать меню НА МЕСТЕ, зная точку.
package base

import "image"

// ContextMenuProvider — виджет, собирающий контекстное меню под точкой.
//
// Движок спрашивает его ПЕРЕД полем ContextMenu: точка известна ему всегда, а
// готовое меню — частный случай, когда оно от точки не зависит. Возврат nil
// означает «здесь меню нет» — тогда движок идёт дальше вверх по дереву, как и
// с обычным ContextMenu.
type ContextMenuProvider interface {
	ContextMenuAt(x, y int) *PopupMenu
}

// RowMenuHost — общее поведение виджета, который отдаёт собранное меню движку.
//
// Меню, отданное из ContextMenuAt, показывает движок, а рисует его тот виджет,
// которому оно принадлежит: оверлеи собираются обходом дерева, и меню, ни за
// кем не закреплённое, не рисуется вовсе. Значит виджет обязан его запомнить и
// реализовать весь контракт оверлея — отрисовку, границы, гашение и разбор
// ввода. Всё это одинаково у таблицы и у дерева, поэтому лежит здесь, а не
// дважды в двух файлах.
//
// Служебный: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
type RowMenuHost struct {
	menu *PopupMenu
	// openedAt — точка, в которой меню открыли, и признак «отпускание этой же
	// кнопки ещё не пришло». См. routeMouse.
	openedAt image.Point
	armed    bool
}

// Build собирает меню из пунктов и запоминает его. (x, y) — точка, в которой
// меню открывают.
//
// Пустой список даёт nil, а не пустое меню: пустая рамка на экране — это не
// «меню без пунктов», это ошибка, которую пользователь не может исправить.
func (h *RowMenuHost) Build(host Widget, x, y int, items []MenuItem) *PopupMenu {
	if len(items) == 0 {
		return nil
	}
	m := NewPopupMenu()
	m.SetHost(host) // меню виджета находит движок через него (ADR-0008)
	m.SetItems(items)
	h.menu = m
	h.openedAt = image.Pt(x, y)
	h.armed = true
	return m
}

func (h *RowMenuHost) Open() bool { return h.menu != nil && h.menu.IsOpen() }

func (h *RowMenuHost) DrawOverlay(ctx DrawContext) {
	if h.Open() {
		h.menu.DrawOverlay(ctx)
	}
}

func (h *RowMenuHost) OverlayBounds() image.Rectangle {
	if h.Open() {
		return h.menu.OverlayBounds()
	}
	return image.Rectangle{}
}

func (h *RowMenuHost) Dismiss() {
	if h.Open() {
		h.menu.Close()
	}
}

// RouteMouse отдаёт событие открытому меню.
//
// Возвращает «событие разобрано меню, дальше не идти».
//
// Отпускание кнопки, которой меню открыли, поглощается и НЕ гасит меню: оно
// приходит уже после показа, в той же точке, и без этой оговорки меню
// закрывалось бы в то же мгновение, в которое открылось. Различаем по точке,
// а не по номеру кнопки: контекстное меню открывают правой, меню переполнения
// — левой, и обе одинаково присылают своё отпускание следом.
//
// Щелчок мимо меню гасит его и НЕ поглощается — клик должен отработать как
// обычный.
func (h *RowMenuHost) RouteMouse(e MouseEvent) bool {
	if !h.Open() {
		return false
	}
	pt := image.Pt(e.X, e.Y)
	if h.armed && !e.Pressed && nearPoint(pt, h.openedAt) {
		h.armed = false
		return true
	}
	h.armed = false
	if pt.In(h.menu.Bounds()) {
		return h.menu.OnMouseButton(e)
	}
	h.menu.Close()
	return false
}

// nearPoint — та же точка с точностью до дрожи руки между нажатием и
// отпусканием.
func nearPoint(a, b image.Point) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx <= 3 && dy <= 3
}

// RouteMove ведёт подсветку пунктов; сообщает, что курсор над меню.
func (h *RowMenuHost) RouteMove(x, y int) bool {
	if !h.Open() {
		return false
	}
	h.menu.OnMouseMove(x, y)
	return image.Pt(x, y).In(h.menu.Bounds())
}

// RouteKey отдаёт клавишу открытому меню (стрелки, Enter, Escape).
func (h *RowMenuHost) RouteKey(e KeyEvent) bool {
	if !h.Open() {
		return false
	}
	h.menu.OnKeyEvent(e)
	return true
}
