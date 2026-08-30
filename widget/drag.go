package widget

import "image"

// ─── Drag support для Panel ─────────────────────────────────────────────────

// DragState хранит состояние перетаскивания панели.
// Встраивается в Panel через поле Drag.
type DragState struct {
	// Enabled включает возможность перетаскивания панели мышью.
	Enabled bool

	// HandleHeight — высота области-«ручки» от верхнего края панели (px).
	// Клик/перетаскивание работает только в этой зоне.
	// 0 означает всю панель (drag за любую точку).
	HandleHeight int

	capMgr   CaptureManager // инжектится движком через SetCaptureManager
	dragging bool
	startX   int // позиция курсора при начале drag
	startY   int
	panelX   int // позиция панели при начале drag
	panelY   int
}

// initDrag вызывается из Panel.OnMouseButton при нажатии в drag-зоне.
func (d *DragState) initDrag(e MouseEvent, panelBounds image.Rectangle) {
	d.dragging = true
	d.startX = e.X
	d.startY = e.Y
	d.panelX = panelBounds.Min.X
	d.panelY = panelBounds.Min.Y
}

// inDragHandle проверяет, попадает ли точка (x, y) в зону перетаскивания.
func (d *DragState) inDragHandle(x, y int, panelBounds image.Rectangle) bool {
	if !image.Pt(x, y).In(panelBounds) {
		return false
	}
	if d.HandleHeight <= 0 {
		return true // вся панель — drag handle
	}
	return y < panelBounds.Min.Y+d.HandleHeight
}

// ─── Dismissable ────────────────────────────────────────────────────────────

// Dismissable реализуется виджетами с раскрывающимся состоянием
// (dropdown, popup, menu), которые нужно закрыть при внешнем событии (drag и т.п.).
type Dismissable interface {
	Dismiss()
}

// DismissableAt — виджет, которому для решения о закрытии нужна точка клика.
//
// Движок предпочитает его обычному Dismissable: закрывать себя по факту
// «клик пришёлся не в меня» умеет и Dismissable, но не всякий виджет считает
// чужой площадью всё, что вне его собственной. Всплывающие панели рабочего
// стола, стоящие одна над другой, — набор, в котором площадь соседки своя
// (desktop.FlyoutGroup).
type DismissableAt interface {
	DismissAt(x, y int)
}

// DismissAll рекурсивно закрывает все Dismissable-виджеты в поддереве w.
func DismissAll(w Widget) {
	if d, ok := w.(Dismissable); ok {
		d.Dismiss()
	}
	for _, child := range w.Children() {
		DismissAll(child)
	}
}

// ─── Рекурсивное смещение виджетов ──────────────────────────────────────────

// BaseBoundsProvider реализуется виджетами, которые переопределяют Bounds()
// (например, Dropdown расширяет bounds при открытом списке). ShiftWidget
// использует BaseBounds() чтобы сдвигать именно базовый прямоугольник,
// не раздувая его от кадра к кадру.
type BaseBoundsProvider interface {
	BaseBounds() image.Rectangle
}

// ShiftWidget сдвигает bounds виджета и всех его потомков на (dx, dy).
// Если виджет реализует BaseBoundsProvider — использует базовые bounds,
// иначе берёт Bounds().
func ShiftWidget(w Widget, dx, dy int) {
	var b image.Rectangle
	if bp, ok := w.(BaseBoundsProvider); ok {
		b = bp.BaseBounds()
	} else {
		b = w.Bounds()
	}
	w.SetBounds(image.Rect(b.Min.X+dx, b.Min.Y+dy, b.Max.X+dx, b.Max.Y+dy))
	// Контейнеры с собственным layout (Canvas, Grid, TabControl…) уже
	// переложили потомков внутри SetBounds — повторный сдвиг задваивал бы
	// смещение (тот же класс бага, что в Canvas.layoutChild).
	if HasOwnLayout(w) {
		return
	}
	for _, child := range w.Children() {
		ShiftWidget(child, dx, dy)
	}
}
