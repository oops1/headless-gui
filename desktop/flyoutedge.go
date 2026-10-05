// flyoutedge.go — привязка всплывающей панели к краю панели задач и к краю
// монитора.
//
// Нижняя и верхняя панели раскрывают окно вверх и вниз, боковые (EdgeLeft,
// EdgeRight) — вправо и влево. Кроме привязки к значку у окна есть второй
// способ встать: прижаться к краю рабочей области монитора, как центр
// уведомлений, который живёт у правого края своего экрана, где бы ни стояла
// панель задач и какой бы значок его ни открыл.
package desktop

import "image"

// SetMonitor сообщает панели, на каком мониторе она показывается: границы
// экрана (Screen), в которые окно вписывается, и рабочая область, к краю
// которой окно прижимается (PinToEdge). Пустые границы монитора оставляют
// Screen как есть.
//
// Панели второго монитора стоят в его координатах: окно не уедет на соседний
// экран, даже если значок у края.
func (f *Flyout) SetMonitor(m Monitor) {
	var was image.Rectangle
	if f.visible() {
		was = f.dirtyRect()
	}
	f.monitor = m
	if !m.Bounds.Empty() {
		f.Screen = m.Bounds
	}
	if f.visible() {
		f.invalidateOverlay(was)
	}
}

// Monitor возвращает монитор, назначенный панели (нулевой — не назначен).
func (f *Flyout) Monitor() Monitor { return f.monitor }

// PinToEdge прижимает окно к краю рабочей области монитора вместо привязки к
// значку. Значок по-прежнему нужен как якорь повторного клика (Toggle,
// DismissAt), но положение окна от него больше не зависит.
//
// Вдоль края окно ставится по Align: у правого и левого края AlignStart — к
// верху области, AlignEnd — к низу, AlignCenter — по середине; у нижнего и
// верхнего — к левому краю, правому и по центру. Margin — зазор от края.
// Откуда окно выезжает, при SlideAuto выводится из края (правый край — справа).
//
// Область берётся из Monitor.Work() монитора (SetMonitor), а если монитор не
// назначен — из Screen. Закреплённая за панелью панель задач вычитается из
// рабочей области автоматически (Taskbar.BindFlyouts).
func (f *Flyout) PinToEdge(e Edge) {
	var was image.Rectangle
	if f.visible() {
		was = f.dirtyRect()
	}
	f.pinned, f.pinEdge = true, e
	if f.visible() {
		f.invalidateOverlay(was)
	}
}

// Unpin возвращает привязку к значку.
func (f *Flyout) Unpin() {
	var was image.Rectangle
	if f.visible() {
		was = f.dirtyRect()
	}
	f.pinned = false
	if f.visible() {
		f.invalidateOverlay(was)
	}
}

// Pinned сообщает, прижата ли панель к краю монитора, и к какому.
func (f *Flyout) Pinned() (Edge, bool) { return f.pinEdge, f.pinned }

// workArea — область, в которой окно прижимается к краю: рабочая область
// монитора, а без монитора — границы экрана.
func (f *Flyout) workArea() image.Rectangle {
	if w := f.monitor.Work(); !w.Empty() {
		return w
	}
	return f.Screen
}

// sideRect — положение окна рядом с боковой панелью: по горизонтали от значка
// в сторону от края, по вертикали по Align.
func (f *Flyout) sideRect(sz image.Point) image.Rectangle {
	var x int
	if f.Edge == EdgeLeft {
		x = f.Anchor.Max.X + f.Margin
	} else {
		x = f.Anchor.Min.X - f.Margin - sz.X
	}
	var y int
	switch f.Align {
	case AlignCenter:
		y = f.Anchor.Min.Y + (f.Anchor.Dy()-sz.Y)/2
	case AlignEnd:
		y = f.Anchor.Max.Y - sz.Y
	default:
		y = f.Anchor.Min.Y
	}
	return image.Rect(x, y, x+sz.X, y+sz.Y)
}

// SetPinMargins разводит два зазора прижатой панели (PinToEdge): edge — до
// края экрана, panel — до панели задач (тех сторон рабочей области, которые
// вырезала панель). Без вызова оба зазора равны Margin, как раньше.
//
// В Windows 10 центр уведомлений стоит вплотную к краю экрана и всё же с
// зазором над панелью задач; одним Margin этого не выразить, и потребителю
// приходилось уменьшать рабочую область самому.
func (f *Flyout) SetPinMargins(edge, panel int) {
	var was image.Rectangle
	if f.visible() {
		was = f.dirtyRect()
	}
	f.pinEdgeGap, f.pinPanelGap, f.pinSplit = edge, panel, true
	if f.visible() {
		f.invalidateOverlay(was)
	}
}

// PinMargins возвращает зазоры прижатой панели: до края экрана и до панели
// задач. Без SetPinMargins оба равны Margin.
func (f *Flyout) PinMargins() (edge, panel int) {
	if f.pinSplit {
		return f.pinEdgeGap, f.pinPanelGap
	}
	return f.Margin, f.Margin
}

// pinGap — зазор до стороны area: до края экрана, если сторона совпала с
// границей экрана, и до панели задач, если область здесь вырезана ею.
func (f *Flyout) pinGap(area image.Rectangle, side Edge) int {
	if !f.pinSplit {
		return f.Margin
	}
	scr := f.monitor.Bounds
	if scr.Empty() {
		scr = f.Screen
	}
	if scr.Empty() {
		return f.pinEdgeGap
	}
	cut := false
	switch side {
	case EdgeLeft:
		cut = area.Min.X > scr.Min.X
	case EdgeRight:
		cut = area.Max.X < scr.Max.X
	case EdgeTop:
		cut = area.Min.Y > scr.Min.Y
	default:
		cut = area.Max.Y < scr.Max.Y
	}
	if cut {
		return f.pinPanelGap
	}
	return f.pinEdgeGap
}

// pinnedRect — положение окна, прижатого к краю рабочей области.
func (f *Flyout) pinnedRect(sz image.Point) image.Rectangle {
	area := f.workArea()
	if area.Empty() {
		// Некуда прижимать: ведём себя как неприжатая, чтобы окно появилось.
		area = f.Anchor
	}
	var x, y int
	switch f.pinEdge {
	case EdgeLeft, EdgeRight:
		if f.pinEdge == EdgeLeft {
			x = area.Min.X + f.pinGap(area, EdgeLeft)
		} else {
			x = area.Max.X - f.pinGap(area, EdgeRight) - sz.X
		}
		switch f.Align {
		case AlignCenter:
			y = area.Min.Y + (area.Dy()-sz.Y)/2
		case AlignEnd:
			y = area.Max.Y - f.pinGap(area, EdgeBottom) - sz.Y
		default:
			y = area.Min.Y + f.pinGap(area, EdgeTop)
		}
	default:
		if f.pinEdge == EdgeTop {
			y = area.Min.Y + f.pinGap(area, EdgeTop)
		} else {
			y = area.Max.Y - f.pinGap(area, EdgeBottom) - sz.Y
		}
		switch f.Align {
		case AlignCenter:
			x = area.Min.X + (area.Dx()-sz.X)/2
		case AlignEnd:
			x = area.Max.X - f.pinGap(area, EdgeRight) - sz.X
		default:
			x = area.Min.X + f.pinGap(area, EdgeLeft)
		}
	}
	return image.Rect(x, y, x+sz.X, y+sz.Y)
}
