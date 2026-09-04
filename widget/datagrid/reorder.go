// reorder.go — перетаскивание колонок за заголовок.
//
// Колонки можно было только растягивать: поменять их местами не давало ни
// событие, ни готовое поведение. Приложение делало это само — захватывало
// мышь, копило смещение, по отпусканию считало целевую колонку и пересобирало
// список через SetColumns. Кода на это уходит больше, чем на саму панель, и
// сложность там не в перестановке, а в различении «клик» / «потянули за
// границу» / «потащили колонку».
//
// Здесь это различение делает сама таблица.
package datagrid

import "image"

// dragThresholdX — на сколько нужно увести курсор вбок, чтобы нажатие на
// заголовке стало перетаскиванием, а не щелчком.
//
// Порог нужен, потому что мышь дрожит: без него всякий щелчок по заголовку
// заканчивался бы микроперестановкой колонок. Несколько пикселей человек не
// замечает, а дрожь в них укладывается.
const dragThresholdX = 4

// MoveColumn переставляет колонку from на позицию to.
//
// Индексы — в текущем порядке колонок; to означает место в списке ПОСЛЕ
// изъятия колонки from (обычная семантика «переставить на N-е место»).
// Выход за границы и to == from ничего не делают.
func (dg *DataGrid) MoveColumn(from, to int) {
	dg.mu.Lock()
	moved := dg.moveColumnLocked(from, to)
	dg.mu.Unlock()
	if moved && dg.OnColumnsReordered != nil {
		dg.OnColumnsReordered(from, to)
	}
}

// moveColumnLocked переставляет колонку; вызывается под dg.mu.
func (dg *DataGrid) moveColumnLocked(from, to int) bool {
	n := len(dg.columns)
	if from < 0 || from >= n || to < 0 || to >= n || from == to {
		return false
	}
	col := dg.columns[from]
	rest := append(dg.columns[:from:from], dg.columns[from+1:]...)
	out := make([]Column, 0, n)
	out = append(out, rest[:to]...)
	out = append(out, col)
	out = append(out, rest[to:]...)
	dg.columns = out
	// Порядок колонок сдвигает всё содержимое строк, а не только заголовок.
	dg.markFullDirty()
	return true
}

// dropIndexAt возвращает место вставки для курсора в точке x — номер колонки,
// ПЕРЕД которой встанет перетаскиваемая.
//
// Считается по серединам колонок, а не по их краям: пока курсор не перешёл
// середину соседа, вставлять перед ним рано — иначе колонки прыгали бы от
// первого же пикселя движения.
func (dg *DataGrid) dropIndexAt(x int) int {
	cx := dg.bounds.Min.X - dg.scrollX
	for i, col := range dg.columns {
		w := col.ActualWidth()
		if x < cx+w/2 {
			return i
		}
		cx += w
	}
	return len(dg.columns)
}

// dropLineX возвращает X линии вставки для места idx.
func (dg *DataGrid) dropLineX(idx int) int {
	x := dg.bounds.Min.X - dg.scrollX
	for i := 0; i < idx && i < len(dg.columns); i++ {
		x += dg.columns[i].ActualWidth()
	}
	return x
}

// beginHeaderPress запоминает нажатие на заголовке как ВОЗМОЖНОЕ начало
// перетаскивания. Сообщает, взято ли нажатие в работу.
//
// Вызывается под dg.mu.
func (dg *DataGrid) beginHeaderPress(x int) bool {
	idx := dg.colIndexAtX(x)
	if idx < 0 {
		return false
	}
	dg.dragCol = idx
	dg.dragStartX = x
	dg.dragX = x
	dg.dragging = false
	return true
}

// dragHeaderTo двигает начатое перетаскивание; сообщает, идёт ли оно.
// Вызывается под dg.mu.
func (dg *DataGrid) dragHeaderTo(x int) bool {
	if dg.dragCol < 0 {
		return false
	}
	// Индекс мог протухнуть после SetColumns — та же осторожность, что и у
	// resizingCol.
	if dg.dragCol >= len(dg.columns) {
		dg.dragCol = -1
		dg.dragging = false
		return false
	}
	dg.dragX = x
	if !dg.dragging && abs(x-dg.dragStartX) >= dragThresholdX {
		dg.dragging = true
	}
	if dg.dragging {
		dg.markRectDirty(dg.headerRect())
	}
	return dg.dragging
}

// finishHeaderPress завершает нажатие на заголовке: либо переставляет
// колонку, либо разбирает щелчок (OnHeaderClick, затем сортировка).
//
// Возвращает (поглощено, было ли вообще нажатие на заголовке).
//
// Колбэки зовутся ВНЕ замка: OnHeaderClick отвечает «разобрал/не разобрал»
// здесь и сейчас, и отложить его нельзя — ответ нужен раньше, чем решение о
// сортировке. Обработчик при этом почти наверняка обратится к самой таблице.
func (dg *DataGrid) finishHeaderPress(x, y int) (bool, bool) {
	dg.mu.Lock()
	if dg.dragCol < 0 {
		dg.mu.Unlock()
		return false, false
	}
	from, dragging := dg.dragCol, dg.dragging
	dg.dragCol, dg.dragging = -1, false
	dg.markRectDirty(dg.headerRect())

	if dragging {
		to := dg.dropIndexAt(x)
		if to > from {
			to--
		}
		moved := dg.moveColumnLocked(from, to)
		cb := dg.OnColumnsReordered
		dg.mu.Unlock()
		if moved && cb != nil {
			cb(from, to)
		}
		return true, true
	}

	// Не тянули — это щелчок по заголовку.
	fn := dg.OnHeaderClick
	idx := dg.colIndexAtX(x)
	var col Column
	if idx >= 0 && idx < len(dg.columns) {
		col = dg.columns[idx]
	}
	canSort := dg.CanUserSortColumns
	dg.mu.Unlock()

	if fn != nil && fn(col, idx, x, y) {
		return true, true
	}
	if canSort && idx >= 0 {
		dg.mu.Lock()
		dg.sortByColumn(idx)
		dg.mu.Unlock()
		dg.firePending()
	}
	return true, true
}

// headerDragSnapshot переносит состояние перетаскивания в снимок отрисовки.
// Вызывается под dg.mu.
func (dg *DataGrid) headerDragSnapshot(s *drawSnapshot) {
	s.dragging = dg.dragging
	if dg.dragging {
		s.dragCol = dg.dragCol
		s.dropX = dg.dropLineX(dg.dropIndexAt(dg.dragX))
	}
}

// drawHeaderDrag рисует место вставки перетаскиваемой колонки.
//
// Линия, а не призрак колонки под курсором: место вставки — единственное, что
// человеку нужно знать, а призрак требует ещё одного буфера на кадр.
func (dg *DataGrid) drawHeaderDrag(ctx DrawContextBridge, s *drawSnapshot) {
	if !s.dragging {
		return
	}
	hr := s.headerRect
	ctx.SetClip(image.Rect(hr.Min.X, hr.Min.Y, hr.Min.X+s.dataW, hr.Max.Y))
	// Заголовок колонки, которую тащат, — приглушённым: видно, ЧТО едет.
	if s.dragCol >= 0 && s.dragCol < len(s.widths) {
		x := s.bounds.Min.X - s.scrollX
		for i := 0; i < s.dragCol; i++ {
			x += s.widths[i]
		}
		ctx.FillRectAlpha(x, hr.Min.Y, s.widths[s.dragCol], s.headerH, s.hoverColor)
	}
	// Линия вставки — в два пикселя: одна на тёмном фоне почти не видна.
	ctx.FillRect(s.dropX-1, hr.Min.Y, 2, s.headerH, s.scrollThumbHigh)
	ctx.SetClip(s.bounds)
}

// abs — модуль целого.
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
