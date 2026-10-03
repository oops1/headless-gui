package widget

// window_tabs_drag.go — прокрутка полосы вкладок заголовка и перестановка
// вкладок перетаскиванием.
//
// Вкладки сжимались до предела, а дальше просто не рисовались: при десятке
// открытых документов часть становилась невидимой И недостижимой — до неё
// нельзя было ни дотянуться мышью, ни узнать, что она есть. Переставить
// вкладки местами тоже было нечем, хотя в браузере и в терминале это первое,
// что человек пробует сделать.
//
// Полоса теперь прокручивается: колесом над заголовком и сама — к активной
// вкладке, когда та уходит за край. Перетаскивание меняет вкладки местами
// на лету, как в Windows Terminal: пока тащат, соседи расступаются, и
// отпускание ничего больше не двигает.

import "image"

// titleTabDragThreshold — на сколько точек нужно увести мышь, чтобы нажатие
// стало перетаскиванием. Без порога любой щелчок по вкладке дрожанием руки
// превращался бы в перестановку.
const titleTabDragThreshold = 5

// titleTabWheelStep — на сколько точек прокручивает полосу один щелчок
// колеса.
const titleTabWheelStep = 60

// MoveTitleTab переставляет вкладку из позиции from в позицию to.
//
// Активной остаётся та же вкладка (её индекс меняется вслед за ней).
// Возвращает false, если индексы неверны или двигать нечего.
func (w *Window) MoveTitleTab(from, to int) bool {
	tt := w.titleTabs
	if tt == nil {
		return false
	}
	tt.mu.Lock()
	n := len(tt.tabs)
	if from < 0 || from >= n || to < 0 || to >= n || from == to {
		tt.mu.Unlock()
		return false
	}
	tab := tt.tabs[from]
	tt.tabs = append(tt.tabs[:from], tt.tabs[from+1:]...)
	rest := make([]TabItem, len(tt.tabs[to:]))
	copy(rest, tt.tabs[to:])
	tt.tabs = append(append(tt.tabs[:to], tab), rest...)

	// Активная вкладка — та же самая, но её номер изменился.
	switch {
	case tt.active == from:
		tt.active = to
	case from < tt.active && to >= tt.active:
		tt.active--
	case from > tt.active && to <= tt.active:
		tt.active++
	}
	tt.mu.Unlock()

	if w.OnTitleTabMoved != nil {
		w.OnTitleTabMoved(from, to)
	}
	w.Invalidate()
	return true
}

// ScrollTitleTabs сдвигает полосу вкладок на dx точек (положительное —
// вправо). Предел считается при отрисовке, поэтому сдвиг применяется
// «мягко»: лишнее срежется на ближайшем кадре.
func (w *Window) ScrollTitleTabs(dx int) {
	tt := w.titleTabs
	if tt == nil || dx == 0 {
		return
	}
	tt.mu.Lock()
	tt.scrollX = clampTabScroll(tt.scrollX+dx, tt.maxScroll)
	tt.mu.Unlock()
	w.Invalidate()
}

// clampTabScroll держит сдвиг в допустимых пределах.
func clampTabScroll(v, max int) int {
	if v < 0 || max <= 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}

// OnMouseWheelPixels прокручивает полосу вкладок, когда курсор над
// заголовком и вкладки не помещаются.
//
// Возвращает false во всех остальных случаях: колесо над содержимым окна
// принадлежит содержимому, и перехватывать его окно не вправе.
func (w *Window) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	tt := w.titleTabs
	if tt == nil || !w.titleTabsActive() {
		return false
	}
	if !image.Pt(x, y).In(w.titleBarRect()) {
		return false
	}
	tt.mu.Lock()
	max := tt.maxScroll
	tt.mu.Unlock()
	if max <= 0 {
		return false
	}
	// Полоса горизонтальная, поэтому вертикальное колесо тоже двигает её
	// вбок: так ведут себя вкладки в браузере.
	step := dx
	if step == 0 {
		step = dy
	}
	if step == 0 {
		return false
	}
	move := titleTabWheelStep
	if step < 0 {
		move = -move
	}
	w.ScrollTitleTabs(move)
	return true
}

// ─── Перетаскивание ─────────────────────────────────────────────────────────

// titleTabDragStart запоминает нажатие на вкладке: перетаскиванием оно
// станет, только если мышь уведут дальше порога.
func (w *Window) titleTabDragStart(idx int, pt image.Point) {
	tt := w.titleTabs
	if tt == nil {
		return
	}
	tt.mu.Lock()
	tt.pressIdx = idx
	tt.pressAt = pt
	tt.dragging = false
	tt.dragDX = 0
	tt.mu.Unlock()
}

// titleTabDragMove ведёт перетаскивание. Возвращает true, если полосу надо
// перерисовать.
func (w *Window) titleTabDragMove(pt image.Point) bool {
	tt := w.titleTabs
	if tt == nil {
		return false
	}
	tt.mu.Lock()
	idx := tt.pressIdx
	if idx < 0 || idx >= len(tt.tabRects) {
		tt.mu.Unlock()
		return false
	}
	dx := pt.X - tt.pressAt.X
	if !tt.dragging {
		if dx < titleTabDragThreshold && dx > -titleTabDragThreshold {
			tt.mu.Unlock()
			return false
		}
		tt.dragging = true
	}
	tt.dragDX = dx

	// Куда вкладка метит: по середине её смещённого прямоугольника.
	cur := tt.tabRects[idx]
	center := cur.Min.X + dx + cur.Dx()/2
	target := idx
	for i, r := range tt.tabRects {
		if r.Empty() || i == idx {
			continue
		}
		if i < idx && center < r.Min.X+r.Dx()/2 {
			target = i
			break
		}
		if i > idx && center > r.Min.X+r.Dx()/2 {
			target = i
		}
	}
	tt.mu.Unlock()

	if target != idx {
		// Соседи расступаются сразу, а не по отпусканию: человек видит
		// будущий порядок до того, как отпустит кнопку.
		if w.MoveTitleTab(idx, target) {
			tt.mu.Lock()
			tt.pressIdx = target
			// Точка отсчёта переезжает вместе со вкладкой — иначе смещение
			// посчиталось бы дважды и вкладка «убежала» бы от курсора.
			if target < len(tt.tabRects) && idx < len(tt.tabRects) {
				tt.pressAt.X += tt.tabRects[target].Min.X - tt.tabRects[idx].Min.X
				tt.dragDX = pt.X - tt.pressAt.X
			}
			tt.mu.Unlock()
		}
	}
	return true
}

// titleTabDragEnd завершает перетаскивание. Возвращает true, если оно шло:
// тогда отпускание кнопки уже израсходовано и не должно толковаться как
// щелчок.
func (w *Window) titleTabDragEnd() bool {
	tt := w.titleTabs
	if tt == nil {
		return false
	}
	tt.mu.Lock()
	was := tt.dragging
	tt.pressIdx = -1
	tt.dragging = false
	tt.dragDX = 0
	tt.mu.Unlock()
	if was {
		w.Invalidate()
	}
	return was
}

// titleTabDragState — снимок перетаскивания для отрисовки.
func (tt *titleTabsState) dragState() (idx, dx int) {
	if !tt.dragging {
		return -1, 0
	}
	return tt.pressIdx, tt.dragDX
}
