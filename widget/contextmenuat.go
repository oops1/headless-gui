// contextmenuat.go — контекстное меню, собираемое под точкой курсора.
//
// SetContextMenu вешает на виджет ОДНО готовое меню. Списку и таблице этого
// мало: действия над файлом, коммитом и веткой зависят от того, по какой
// строке щёлкнули, а строка виджету заранее не известна. Приложение обходило
// это, разбирая правую кнопку самостоятельно и повторяя формулу «какая строка
// под этим Y».
//
// Здесь виджет получает возможность собрать меню НА МЕСТЕ, зная точку.
package widget

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

// rowMenuHost — общее поведение виджета, который отдаёт собранное меню движку.
//
// Меню, отданное из ContextMenuAt, показывает движок, а рисует его тот виджет,
// которому оно принадлежит: оверлеи собираются обходом дерева, и меню, ни за
// кем не закреплённое, не рисуется вовсе. Значит виджет обязан его запомнить и
// реализовать весь контракт оверлея — отрисовку, границы, гашение и разбор
// ввода. Всё это одинаково у таблицы и у дерева, поэтому лежит здесь, а не
// дважды в двух файлах.
type rowMenuHost struct {
	menu *PopupMenu
}

// build собирает меню из пунктов и запоминает его.
//
// Пустой список даёт nil, а не пустое меню: пустая рамка на экране — это не
// «меню без пунктов», это ошибка, которую пользователь не может исправить.
func (h *rowMenuHost) build(items []MenuItem) *PopupMenu {
	if len(items) == 0 {
		return nil
	}
	m := NewPopupMenu()
	m.SetItems(items)
	h.menu = m
	return m
}

func (h *rowMenuHost) open() bool { return h.menu != nil && h.menu.IsOpen() }

func (h *rowMenuHost) drawOverlay(ctx DrawContext) {
	if h.open() {
		h.menu.DrawOverlay(ctx)
	}
}

func (h *rowMenuHost) overlayBounds() image.Rectangle {
	if h.open() {
		return h.menu.OverlayBounds()
	}
	return image.Rectangle{}
}

func (h *rowMenuHost) dismiss() {
	if h.open() {
		h.menu.Close()
	}
}

// routeMouse отдаёт событие открытому меню.
//
// Второе значение — «событие разобрано меню, дальше не идти». Отпускание
// правой кнопки поглощается: это та же кнопка, которой меню открыли, и без
// этого оно закрылось бы мгновенно. Щелчок мимо меню гасит его и НЕ
// поглощается — клик должен отработать как обычный.
func (h *rowMenuHost) routeMouse(e MouseEvent) bool {
	if !h.open() {
		return false
	}
	if e.Button == MouseRight && !e.Pressed {
		return true
	}
	if image.Pt(e.X, e.Y).In(h.menu.Bounds()) {
		return h.menu.OnMouseButton(e)
	}
	h.menu.Close()
	return false
}

// routeMove ведёт подсветку пунктов; сообщает, что курсор над меню.
func (h *rowMenuHost) routeMove(x, y int) bool {
	if !h.open() {
		return false
	}
	h.menu.OnMouseMove(x, y)
	return image.Pt(x, y).In(h.menu.Bounds())
}

// routeKey отдаёт клавишу открытому меню (стрелки, Enter, Escape).
func (h *rowMenuHost) routeKey(e KeyEvent) bool {
	if !h.open() {
		return false
	}
	h.menu.OnKeyEvent(e)
	return true
}
