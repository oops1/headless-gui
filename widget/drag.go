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
	for _, child := range w.Children() {
		ShiftWidget(child, dx, dy)
	}
}
