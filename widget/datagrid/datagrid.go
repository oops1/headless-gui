// Package datagrid — полноценный DataGrid, совместимый с WPF.
//
// Поддерживает:
//   - Виртуализация строк (рисуются только видимые)
//   - Сортировка по клику на заголовок
//   - Выделение (Single/Extended)
//   - Редактирование (двойной клик / Enter → Enter/Esc)
//   - Клавиатурная навигация (стрелки, Tab, Home/End, PageUp/PageDown)
//   - Resize колонок мышью
//   - Скроллбар
//   - Data Binding с ObservableCollection / INotifyPropertyChanged
package datagrid

import (
	"fmt"
	"image"
	"image/color"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// ─── SelectionMode ─────────────────────────────────────────────────────────

// SelectionMode определяет режим выделения строк.
type SelectionMode int

const (
	// SelectionSingle — только одна строка.
	SelectionSingle SelectionMode = iota
	// SelectionExtended — множественное выделение (Ctrl+Click, Shift+Click).
	SelectionExtended
)

// ─── Events ────────────────────────────────────────────────────────────────

// SelectionChangedEvent — событие смены выделения.
type SelectionChangedEvent struct {
	SelectedIndex int
	SelectedItem  interface{}
}

// SortingEvent — событие сортировки.
type SortingEvent struct {
	Column    Column
	Direction SortDirection
	Handled   bool // если true, DataGrid не выполняет стандартную сортировку
}

// CellEditEndingEvent — событие завершения редактирования ячейки.
type CellEditEndingEvent struct {
	RowIndex    int
	Column      Column
	Item        interface{}
	NewValue    string
	Cancel      bool // если true, изменение отменяется
}

// ─── Константы ─────────────────────────────────────────────────────────────

const (
	defaultRowHeight    = 28
	defaultHeaderHeight = 30
	defaultFontSize     = 10.0
	scrollbarWidth      = 12
	resizeHitZone       = 5 // зона ±px для resize колонки
	minColumnWidth      = 30
)

// ─── DataGrid ──────────────────────────────────────────────────────────────

// DataGrid — табличный виджет, совместимый с WPF DataGrid.
type DataGrid struct {
	// Bounds — прямоугольник виджета (абсолютные координаты).
	bounds image.Rectangle

	// ── Колонки ──────────────────────────────────────────────────────────
	columns []Column

	// ── Данные ───────────────────────────────────────────────────────────
	itemsSource *ObservableCollection // наблюдаемая коллекция
	sortedIdx   []int                 // индексы в исходной коллекции после сортировки

	// ── Свойства (WPF-совместимые) ───────────────────────────────────────
	AutoGenerateColumns bool
	IsReadOnly          bool
	CanUserSortColumns  bool
	CanUserResizeColumns bool
	SelectionMode       SelectionMode
	RowHeight           int
	HeaderHeight        int
	FontSize            float64

	// ── Выделение ────────────────────────────────────────────────────────
	selectedRows map[int]bool // множество выделенных индексов (в sortedIdx)
	anchorRow    int          // якорь для Shift+Click
	focusRow     int          // строка с фокусом
	focusCol     int          // колонка с фокусом

	// ── Редактирование ───────────────────────────────────────────────────
	editingRow   int
	editingCol   int
	editingValue string // текущее значение в редакторе
	isEditing    bool
	editCursorPos int  // позиция курсора в редакторе

	// ── Скроллинг ────────────────────────────────────────────────────────
	scrollY      int
	scrollX      int // горизонтальный скролл (для широких таблиц)
	hoverRow     int
	thumbDragging   bool
	thumbDragStartY int
	thumbDragStartS int
	thumbHovered    bool

	// ── Resize колонок ───────────────────────────────────────────────────
	resizingCol     int  // индекс колонки (-1 = нет)
	resizingStartX  int
	resizingStartW  int
	resizeHover     bool // курсор в зоне resize

	// ── Цвета (из темы) ──────────────────────────────────────────────────
	Background      color.RGBA
	HeaderBG        color.RGBA
	HeaderText      color.RGBA
	TextColor       color.RGBA
	BorderColor     color.RGBA
	SelectColor     color.RGBA
	HoverColor      color.RGBA
	AlternateBG     color.RGBA
	GridLineColor   color.RGBA
	ScrollTrackBG   color.RGBA
	ScrollThumbBG   color.RGBA
	ScrollThumbHover color.RGBA
	EditBG          color.RGBA
	EditBorder      color.RGBA

	// ── Callbacks ────────────────────────────────────────────────────────
	OnSelectionChanged func(e SelectionChangedEvent)
	OnSorting          func(e *SortingEvent)
	OnCellEditEnding   func(e *CellEditEndingEvent)
	OnRowEditEnding    func(rowIndex int, item interface{})

	// ── Внутреннее состояние ─────────────────────────────────────────────
	mu      sync.Mutex
	focused bool
	dirty   bool // layout нужно пересчитать
}

// ─── Конструктор ───────────────────────────────────────────────────────────

// New создаёт DataGrid с настройками по умолчанию.
func New() *DataGrid {
	dg := &DataGrid{
		RowHeight:            defaultRowHeight,
		HeaderHeight:         defaultHeaderHeight,
		FontSize:             defaultFontSize,
		CanUserSortColumns:   true,
		CanUserResizeColumns: true,
		SelectionMode:        SelectionSingle,
		selectedRows:         make(map[int]bool),
		focusRow:             -1,
		focusCol:             0,
		anchorRow:            -1,
		editingRow:           -1,
		editingCol:           -1,
		hoverRow:             -1,
		resizingCol:          -1,
		// Цвета по умолчанию (Dark theme)
		Background:      color.RGBA{R: 30, G: 30, B: 30, A: 255},
		HeaderBG:        color.RGBA{R: 45, G: 45, B: 48, A: 255},
		HeaderText:      color.RGBA{R: 212, G: 212, B: 212, A: 255},
		TextColor:       color.RGBA{R: 204, G: 204, B: 204, A: 255},
		BorderColor:     color.RGBA{R: 63, G: 63, B: 70, A: 255},
		SelectColor:     color.RGBA{R: 0, G: 120, B: 215, A: 80},
		HoverColor:      color.RGBA{R: 62, G: 62, B: 66, A: 255},
		AlternateBG:     color.RGBA{R: 37, G: 37, B: 38, A: 255},
		GridLineColor:   color.RGBA{R: 50, G: 50, B: 52, A: 255},
		ScrollTrackBG:   color.RGBA{R: 46, G: 46, B: 48, A: 255},
		ScrollThumbBG:   color.RGBA{R: 77, G: 77, B: 80, A: 255},
		ScrollThumbHover: color.RGBA{R: 0, G: 120, B: 215, A: 255},
		EditBG:          color.RGBA{R: 60, G: 60, B: 60, A: 255},
		EditBorder:      color.RGBA{R: 0, G: 120, B: 215, A: 255},
		dirty:           true,
	}
	return dg
}

// ─── Widget interface (compatible with widget.Widget) ──────────────────────

func (dg *DataGrid) Bounds() image.Rectangle    { return dg.bounds }
func (dg *DataGrid) SetBounds(r image.Rectangle) { dg.bounds = r; dg.dirty = true }

// ─── Columns ───────────────────────────────────────────────────────────────

// AddColumn добавляет колонку.
func (dg *DataGrid) AddColumn(col Column) {
	dg.columns = append(dg.columns, col)
	dg.dirty = true
}

// Columns возвращает колонки.
func (dg *DataGrid) Columns() []Column {
	return dg.columns
}

// SetColumns заменяет все колонки.
func (dg *DataGrid) SetColumns(cols []Column) {
	dg.columns = cols
	dg.dirty = true
}

// ─── Data Source ────────────────────────────────────────────────────────────

// SetItemsSource задаёт источник данных.
func (dg *DataGrid) SetItemsSource(oc *ObservableCollection) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	dg.itemsSource = oc
	dg.rebuildSortedIdx()
	dg.selectedRows = make(map[int]bool)
	dg.focusRow = -1
	dg.scrollY = 0

	// Подписка на изменения
	oc.AddCollectionChanged(func(event CollectionChangedEvent) {
		dg.mu.Lock()
		dg.rebuildSortedIdx()
		dg.mu.Unlock()
	})

	// Авто-генерация колонок
	if dg.AutoGenerateColumns && len(dg.columns) == 0 && oc.Count() > 0 {
		dg.autoGenerateColumns(oc.Get(0))
	}
}

// ItemsSource возвращает источник данных.
func (dg *DataGrid) ItemsSource() *ObservableCollection {
	return dg.itemsSource
}

// rebuildSortedIdx пересоздаёт индексный массив (без сортировки).
func (dg *DataGrid) rebuildSortedIdx() {
	if dg.itemsSource == nil {
		dg.sortedIdx = nil
		return
	}
	n := dg.itemsSource.Count()
	dg.sortedIdx = make([]int, n)
	for i := 0; i < n; i++ {
		dg.sortedIdx[i] = i
	}
	// Переприменяем текущую сортировку
	dg.applyCurrentSort()
}

// autoGenerateColumns генерирует колонки из полей первого элемента.
func (dg *DataGrid) autoGenerateColumns(sample interface{}) {
	if sample == nil {
		return
	}
	t := reflect.TypeOf(sample)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		switch field.Type.Kind() {
		case reflect.Bool:
			dg.columns = append(dg.columns, NewCheckBoxColumn(field.Name, field.Name))
		default:
			dg.columns = append(dg.columns, NewTextColumn(field.Name, field.Name))
		}
	}
	dg.dirty = true
}

// ─── Selection ─────────────────────────────────────────────────────────────

// SelectedItem возвращает первый выделенный элемент.
func (dg *DataGrid) SelectedItem() interface{} {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	if dg.focusRow >= 0 && dg.focusRow < len(dg.sortedIdx) {
		return dg.itemsSource.Get(dg.sortedIdx[dg.focusRow])
	}
	return nil
}

// SelectedItems возвращает все выделенные элементы.
func (dg *DataGrid) SelectedItems() []interface{} {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	var result []interface{}
	for idx := range dg.selectedRows {
		if idx >= 0 && idx < len(dg.sortedIdx) {
			result = append(result, dg.itemsSource.Get(dg.sortedIdx[idx]))
		}
	}
	return result
}

// SetSelectedIndex задаёт выделенную строку.
func (dg *DataGrid) SetSelectedIndex(idx int) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	dg.selectedRows = map[int]bool{idx: true}
	dg.focusRow = idx
	dg.anchorRow = idx
	dg.ensureVisible(idx)
}

// ─── Layout ────────────────────────────────────────────────────────────────

// layoutColumns вычисляет ActualWidth для каждой колонки.
func (dg *DataGrid) layoutColumns() {
	if len(dg.columns) == 0 {
		return
	}

	totalW := dg.bounds.Dx()
	if dg.needsScrollbar() {
		totalW -= scrollbarWidth
	}

	// Первый проход: фиксированные и Auto колонки
	usedW := 0
	var starCols []int
	totalStars := 0.0

	for i, col := range dg.columns {
		cw := col.Width()
		switch cw.Mode {
		case ColumnWidthPixel:
			w := int(cw.Value)
			if w < minColumnWidth {
				w = minColumnWidth
			}
			col.SetActualWidth(w)
			usedW += w
		case ColumnWidthAuto:
			// Вычисляем по заголовку (приблизительно)
			headerW := len(col.Header())*8 + 20
			if headerW < minColumnWidth {
				headerW = minColumnWidth
			}
			col.SetActualWidth(headerW)
			usedW += headerW
		case ColumnWidthStar:
			starCols = append(starCols, i)
			totalStars += cw.Value
		}
	}

	// Второй проход: Star колонки получают оставшееся пространство
	remaining := totalW - usedW
	if remaining < 0 {
		remaining = 0
	}
	if len(starCols) > 0 && totalStars > 0 {
		for _, i := range starCols {
			cw := dg.columns[i].Width()
			w := int(float64(remaining) * cw.Value / totalStars)
			if w < minColumnWidth {
				w = minColumnWidth
			}
			dg.columns[i].SetActualWidth(w)
		}
	}
}

// ─── Geometry helpers ──────────────────────────────────────────────────────

func (dg *DataGrid) rowCount() int {
	if dg.sortedIdx == nil {
		return 0
	}
	return len(dg.sortedIdx)
}

func (dg *DataGrid) contentHeight() int {
	return dg.rowCount() * dg.RowHeight
}

func (dg *DataGrid) viewHeight() int {
	return dg.bounds.Dy() - dg.HeaderHeight
}

func (dg *DataGrid) needsScrollbar() bool {
	return dg.contentHeight() > dg.viewHeight()
}

func (dg *DataGrid) maxScrollY() int {
	m := dg.contentHeight() - dg.viewHeight()
	if m < 0 {
		return 0
	}
	return m
}

func (dg *DataGrid) clampScrollY() {
	if dg.scrollY < 0 {
		dg.scrollY = 0
	}
	if max := dg.maxScrollY(); dg.scrollY > max {
		dg.scrollY = max
	}
}

func (dg *DataGrid) totalColumnsWidth() int {
	w := 0
	for _, col := range dg.columns {
		w += col.ActualWidth()
	}
	return w
}

func (dg *DataGrid) ensureVisible(row int) {
	if row < 0 || row >= dg.rowCount() {
		return
	}
	top := row * dg.RowHeight
	bot := top + dg.RowHeight
	vh := dg.viewHeight()

	if top < dg.scrollY {
		dg.scrollY = top
	}
	if bot > dg.scrollY+vh {
		dg.scrollY = bot - vh
	}
	dg.clampScrollY()
}

// headerRect возвращает прямоугольник заголовка.
func (dg *DataGrid) headerRect() image.Rectangle {
	b := dg.bounds
	return image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+dg.HeaderHeight)
}

// dataRect возвращает прямоугольник области данных (под заголовком).
func (dg *DataGrid) dataRect() image.Rectangle {
	b := dg.bounds
	return image.Rect(b.Min.X, b.Min.Y+dg.HeaderHeight, b.Max.X, b.Max.Y)
}

// rowIndexAtY возвращает индекс строки по Y-координате (в области данных).
func (dg *DataGrid) rowIndexAtY(y int) int {
	dr := dg.dataRect()
	if y < dr.Min.Y || y >= dr.Max.Y {
		return -1
	}
	idx := (y - dr.Min.Y + dg.scrollY) / dg.RowHeight
	if idx >= 0 && idx < dg.rowCount() {
		return idx
	}
	return -1
}

// colIndexAtX возвращает индекс колонки по X-координате.
func (dg *DataGrid) colIndexAtX(x int) int {
	bx := dg.bounds.Min.X - dg.scrollX
	for i, col := range dg.columns {
		w := col.ActualWidth()
		if x >= bx && x < bx+w {
			return i
		}
		bx += w
	}
	return -1
}

// colLeftX возвращает X левого края колонки (абсолютные координаты).
func (dg *DataGrid) colLeftX(colIdx int) int {
	x := dg.bounds.Min.X - dg.scrollX
	for i := 0; i < colIdx && i < len(dg.columns); i++ {
		x += dg.columns[i].ActualWidth()
	}
	return x
}

// ─── Scrollbar ─────────────────────────────────────────────────────────────

func (dg *DataGrid) scrollbarRect() image.Rectangle {
	b := dg.bounds
	return image.Rect(b.Max.X-scrollbarWidth, b.Min.Y+dg.HeaderHeight, b.Max.X, b.Max.Y)
}

func (dg *DataGrid) thumbRect() image.Rectangle {
	if !dg.needsScrollbar() {
		return image.Rectangle{}
	}
	sr := dg.scrollbarRect()
	vh := sr.Dy()
	ch := dg.contentHeight()
	ratio := float64(vh) / float64(ch)
	thumbH := int(ratio * float64(vh))
	if thumbH < 20 {
		thumbH = 20
	}
	maxS := dg.maxScrollY()
	var thumbY int
	if maxS > 0 {
		thumbY = int(float64(dg.scrollY) / float64(maxS) * float64(vh-thumbH))
	}
	return image.Rect(sr.Min.X, sr.Min.Y+thumbY, sr.Max.X, sr.Min.Y+thumbY+thumbH)
}

// ─── Sorting ───────────────────────────────────────────────────────────────

// sortByColumn сортирует данные по заданной колонке.
func (dg *DataGrid) sortByColumn(colIdx int) {
	if colIdx < 0 || colIdx >= len(dg.columns) {
		return
	}
	col := dg.columns[colIdx]

	// Переключаем направление
	dir := col.GetSortDirection()
	switch dir {
	case SortNone, SortDescending:
		dir = SortAscending
	case SortAscending:
		dir = SortDescending
	}

	// Сбрасываем направление всех колонок
	for _, c := range dg.columns {
		c.SetSortDirection(SortNone)
	}
	col.SetSortDirection(dir)

	// Вызываем callback
	if dg.OnSorting != nil {
		evt := &SortingEvent{Column: col, Direction: dir}
		dg.OnSorting(evt)
		if evt.Handled {
			return
		}
	}

	dg.applyCurrentSort()
}

// applyCurrentSort применяет текущую сортировку.
func (dg *DataGrid) applyCurrentSort() {
	if dg.itemsSource == nil {
		return
	}

	// Находим колонку с активной сортировкой
	var sortCol Column
	for _, c := range dg.columns {
		if c.GetSortDirection() != SortNone {
			sortCol = c
			break
		}
	}
	if sortCol == nil {
		return
	}

	dir := sortCol.GetSortDirection()
	path := sortCol.SortMemberPath()
	if path == "" && sortCol.GetBinding() != nil {
		path = sortCol.GetBinding().Path
	}
	if path == "" {
		return
	}

	src := dg.itemsSource
	sort.SliceStable(dg.sortedIdx, func(i, j int) bool {
		a, _ := GetPropertyValue(src.Get(dg.sortedIdx[i]), path)
		b, _ := GetPropertyValue(src.Get(dg.sortedIdx[j]), path)
		cmp := compareValues(a, b)
		if dir == SortDescending {
			return cmp > 0
		}
		return cmp < 0
	})
}

// compareValues сравнивает два значения произвольного типа.
func compareValues(a, b interface{}) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Приводим к строкам для сравнения через reflect
	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)

	// Числа
	if isNumeric(va) && isNumeric(vb) {
		fa := toFloat64(va)
		fb := toFloat64(vb)
		if fa < fb {
			return -1
		}
		if fa > fb {
			return 1
		}
		return 0
	}

	// Bool
	if va.Kind() == reflect.Bool && vb.Kind() == reflect.Bool {
		ba := va.Bool()
		bb := vb.Bool()
		if ba == bb {
			return 0
		}
		if !ba {
			return -1
		}
		return 1
	}

	// Строки (fallback)
	sa := strings.ToLower(valToString(a))
	sb := strings.ToLower(valToString(b))
	return strings.Compare(sa, sb)
}

func isNumeric(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func toFloat64(v reflect.Value) float64 {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		return v.Float()
	}
	return 0
}

func valToString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// ─── Draw ──────────────────────────────────────────────────────────────────

// Draw отрисовывает DataGrid.
func (dg *DataGrid) Draw(ctx DrawContextBridge) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	b := dg.bounds
	if b.Empty() || len(dg.columns) == 0 {
		return
	}

	// Глобальный клип по bounds — ничто не выйдет за пределы DataGrid.
	ctx.SetClip(b)
	defer ctx.ClearClip()

	// Пересчитываем ширину колонок
	if dg.dirty {
		dg.layoutColumns()
		dg.dirty = false
	}

	// Фон
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), dg.Background)

	// Заголовок
	dg.drawHeader(ctx)

	// Строки данных (с виртуализацией) — собственный клип внутри
	dg.drawRows(ctx)

	// Скроллбар
	if dg.needsScrollbar() {
		dg.drawScrollbar(ctx)
	}

	// Внешняя рамка
	ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), dg.BorderColor)
}

// drawHeader рисует заголовки колонок.
func (dg *DataGrid) drawHeader(ctx DrawContextBridge) {
	hr := dg.headerRect()
	ctx.FillRect(hr.Min.X, hr.Min.Y, hr.Dx(), hr.Dy(), dg.HeaderBG)

	// Клиппинг по области заголовка (без скроллбара)
	dataW := dg.bounds.Dx()
	if dg.needsScrollbar() {
		dataW -= scrollbarWidth
	}
	clipRect := image.Rect(hr.Min.X, hr.Min.Y, hr.Min.X+dataW, hr.Max.Y)
	ctx.SetClip(clipRect)

	x := dg.bounds.Min.X - dg.scrollX
	for _, col := range dg.columns {
		w := col.ActualWidth()
		if x+w > hr.Min.X && x < hr.Min.X+dataW {
			// Текст заголовка
			textX := x + 6
			textY := hr.Min.Y + (dg.HeaderHeight-14)/2
			ctx.DrawTextSize(col.Header(), textX, textY, dg.FontSize, dg.HeaderText)

			// Индикатор сортировки
			if col.GetSortDirection() != SortNone {
				arrow := "▲"
				if col.GetSortDirection() == SortDescending {
					arrow = "▼"
				}
				arrowX := x + w - 16
				ctx.DrawTextSize(arrow, arrowX, textY, dg.FontSize, dg.HeaderText)
			}

			// Разделитель колонки
			ctx.DrawVLine(x+w-1, hr.Min.Y, dg.HeaderHeight, dg.GridLineColor)
		}
		x += w
	}

	// Восстанавливаем глобальный клип по bounds
	ctx.SetClip(dg.bounds)

	// Горизонтальная линия под заголовком
	ctx.DrawHLine(hr.Min.X, hr.Max.Y-1, hr.Dx(), dg.BorderColor)
}

// drawRows рисует видимые строки (виртуализация).
func (dg *DataGrid) drawRows(ctx DrawContextBridge) {
	dr := dg.dataRect()
	if dg.rowCount() == 0 || dg.itemsSource == nil {
		return
	}

	// Вычисляем диапазон видимых строк
	startRow := dg.scrollY / dg.RowHeight
	if startRow < 0 {
		startRow = 0
	}
	endRow := (dg.scrollY + dg.viewHeight()) / dg.RowHeight
	if endRow >= dg.rowCount() {
		endRow = dg.rowCount() - 1
	}

	dataW := dg.bounds.Dx()
	if dg.needsScrollbar() {
		dataW -= scrollbarWidth
	}
	clipRect := image.Rect(dr.Min.X, dr.Min.Y, dr.Min.X+dataW, dr.Max.Y)
	ctx.SetClip(clipRect)

	for row := startRow; row <= endRow; row++ {
		rowY := dr.Min.Y + row*dg.RowHeight - dg.scrollY
		if rowY+dg.RowHeight < dr.Min.Y || rowY >= dr.Max.Y {
			continue
		}

		// Получаем элемент данных
		dataIdx := dg.sortedIdx[row]
		item := dg.itemsSource.Get(dataIdx)

		// Фон строки: чередование, hover, выделение
		isSelected := dg.selectedRows[row]
		isHovered := row == dg.hoverRow

		if isSelected {
			ctx.FillRectAlpha(dr.Min.X, rowY, dataW, dg.RowHeight, dg.SelectColor)
		} else if isHovered {
			ctx.FillRect(dr.Min.X, rowY, dataW, dg.RowHeight, dg.HoverColor)
		} else if row%2 == 1 {
			ctx.FillRect(dr.Min.X, rowY, dataW, dg.RowHeight, dg.AlternateBG)
		}

		// Ячейки
		cellX := dg.bounds.Min.X - dg.scrollX
		for colIdx, col := range dg.columns {
			w := col.ActualWidth()
			cellRect := image.Rect(cellX, rowY, cellX+w, rowY+dg.RowHeight)

			// Per-cell clip = пересечение ячейки с областью данных,
			// чтобы текст не вылезал ни за пределы ячейки, ни за хедер/нижнюю границу.
			cellClip := cellRect.Intersect(clipRect)
			if !cellClip.Empty() {
				ctx.SetClip(cellClip)
			}

			// Режим редактирования?
			if dg.isEditing && dg.editingRow == row && dg.editingCol == colIdx {
				dg.drawEditCell(ctx, cellRect)
			} else {
				cdc := CellDrawContext{
					Rect:       cellRect,
					Item:       item,
					RowIndex:   dataIdx,
					IsSelected: isSelected,
					IsHovered:  isHovered,
					IsEditing:  false,
					DrawCtx:    ctx,
					TextColor:  dg.TextColor,
					FontSize:   dg.FontSize,
				}
				col.DrawCell(cdc)
			}

			// Вертикальная линия ячейки
			ctx.DrawVLine(cellX+w-1, rowY, dg.RowHeight, dg.GridLineColor)
			cellX += w
		}

		// Восстанавливаем data-area clip после ячеек строки
		ctx.SetClip(clipRect)

		// Горизонтальная линия строки
		ctx.DrawHLine(dr.Min.X, rowY+dg.RowHeight-1, dataW, dg.GridLineColor)
	}

	// Восстанавливаем глобальный клип по bounds
	ctx.SetClip(dg.bounds)
}

// drawEditCell рисует ячейку в режиме редактирования.
func (dg *DataGrid) drawEditCell(ctx DrawContextBridge, r image.Rectangle) {
	// Фон и рамка
	ctx.FillRect(r.Min.X+1, r.Min.Y+1, r.Dx()-2, r.Dy()-2, dg.EditBG)
	ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), dg.EditBorder)

	// Текст
	textX := r.Min.X + 6
	textY := r.Min.Y + (r.Dy()-14)/2
	ctx.DrawTextSize(dg.editingValue, textX, textY, dg.FontSize, dg.TextColor)

	// Каретка
	if dg.focused {
		caretText := string([]rune(dg.editingValue)[:dg.editCursorPos])
		caretX := textX + ctx.MeasureText(caretText, dg.FontSize)
		ctx.FillRect(caretX, r.Min.Y+4, 1, r.Dy()-8, dg.TextColor)
	}
}

// drawScrollbar рисует вертикальный скроллбар.
func (dg *DataGrid) drawScrollbar(ctx DrawContextBridge) {
	sr := dg.scrollbarRect()
	ctx.FillRect(sr.Min.X, sr.Min.Y, sr.Dx(), sr.Dy(), dg.ScrollTrackBG)

	tr := dg.thumbRect()
	tc := dg.ScrollThumbBG
	if dg.thumbHovered || dg.thumbDragging {
		tc = dg.ScrollThumbHover
	}
	ctx.FillRect(tr.Min.X+2, tr.Min.Y+1, tr.Dx()-4, tr.Dy()-2, tc)
}

// ─── Mouse Events ──────────────────────────────────────────────────────────

// OnMouseButton обрабатывает нажатие/отпускание кнопки мыши.
// Возвращает true, если событие поглощено.
func (dg *DataGrid) OnMouseButton(x, y int, button int, pressed bool) bool {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	if button != 0 { // только LeftButton
		return false
	}

	if !pressed {
		// Отпускание: завершаем drag операции
		if dg.thumbDragging {
			dg.thumbDragging = false
			return true
		}
		if dg.resizingCol >= 0 {
			dg.resizingCol = -1
			return true
		}
		return false
	}

	// ── Pressed ────────────────────────────────────────────────────────
	pt := image.Pt(x, y)

	// Скроллбар
	if dg.needsScrollbar() {
		tr := dg.thumbRect()
		if pt.In(tr) {
			dg.thumbDragging = true
			dg.thumbDragStartY = y
			dg.thumbDragStartS = dg.scrollY
			return true
		}
		sr := dg.scrollbarRect()
		if pt.In(sr) {
			ratio := float64(y-sr.Min.Y) / float64(sr.Dy())
			dg.scrollY = int(ratio * float64(dg.contentHeight()))
			dg.clampScrollY()
			return true
		}
	}

	// Resize колонок (на границе заголовка)
	if dg.CanUserResizeColumns && pt.In(dg.headerRect()) {
		colIdx := dg.resizeColumnAt(x)
		if colIdx >= 0 {
			dg.resizingCol = colIdx
			dg.resizingStartX = x
			dg.resizingStartW = dg.columns[colIdx].ActualWidth()
			return true
		}
	}

	// Заголовок: сортировка
	if dg.CanUserSortColumns && pt.In(dg.headerRect()) {
		colIdx := dg.colIndexAtX(x)
		if colIdx >= 0 {
			dg.sortByColumn(colIdx)
			return true
		}
	}

	// Область данных: выделение
	row := dg.rowIndexAtY(y)
	if row >= 0 {
		// Завершаем текущее редактирование
		if dg.isEditing {
			dg.commitEdit()
		}
		dg.selectRow(row, false, false) // TODO: Shift/Ctrl из события
		return true
	}

	return false
}

// OnMouseDoubleClick обрабатывает двойной клик (вход в редактирование).
func (dg *DataGrid) OnMouseDoubleClick(x, y int) bool {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	row := dg.rowIndexAtY(y)
	col := dg.colIndexAtX(x)
	if row >= 0 && col >= 0 {
		dg.beginEdit(row, col)
		return true
	}
	return false
}

// OnMouseMove обрабатывает перемещение мыши.
func (dg *DataGrid) OnMouseMove(x, y int) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	// Drag скроллбара
	if dg.thumbDragging {
		sr := dg.scrollbarRect()
		tr := dg.thumbRect()
		trackUsable := sr.Dy() - tr.Dy()
		if trackUsable > 0 {
			dy := y - dg.thumbDragStartY
			scrollDelta := int(float64(dy) / float64(trackUsable) * float64(dg.maxScrollY()))
			dg.scrollY = dg.thumbDragStartS + scrollDelta
			dg.clampScrollY()
		}
		return
	}

	// Resize колонки
	if dg.resizingCol >= 0 {
		dx := x - dg.resizingStartX
		newW := dg.resizingStartW + dx
		if newW < minColumnWidth {
			newW = minColumnWidth
		}
		dg.columns[dg.resizingCol].SetActualWidth(newW)
		return
	}

	// Hover строки
	dg.hoverRow = dg.rowIndexAtY(y)

	// Cursor для resize (определяем зону)
	if dg.CanUserResizeColumns && image.Pt(x, y).In(dg.headerRect()) {
		dg.resizeHover = dg.resizeColumnAt(x) >= 0
	} else {
		dg.resizeHover = false
	}

	// Hover скроллбара
	if dg.needsScrollbar() {
		tr := dg.thumbRect()
		dg.thumbHovered = image.Pt(x, y).In(tr)
	}
}

// resizeColumnAt возвращает индекс колонки, если X попадает в зону resize.
func (dg *DataGrid) resizeColumnAt(x int) int {
	colX := dg.bounds.Min.X - dg.scrollX
	for i, col := range dg.columns {
		w := col.ActualWidth()
		rightEdge := colX + w
		if x >= rightEdge-resizeHitZone && x <= rightEdge+resizeHitZone {
			return i
		}
		colX += w
	}
	return -1
}

// ScrollBy прокручивает на delta пикселей.
func (dg *DataGrid) ScrollBy(delta int) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	dg.scrollY += delta
	dg.clampScrollY()
}

// ─── Selection helpers ─────────────────────────────────────────────────────

func (dg *DataGrid) selectRow(row int, shift, ctrl bool) {
	if dg.SelectionMode == SelectionSingle || (!shift && !ctrl) {
		// Простой клик — одна строка
		dg.selectedRows = map[int]bool{row: true}
		dg.anchorRow = row
	} else if ctrl {
		// Ctrl+Click — toggle
		if dg.selectedRows[row] {
			delete(dg.selectedRows, row)
		} else {
			dg.selectedRows[row] = true
		}
	} else if shift {
		// Shift+Click — диапазон
		dg.selectedRows = make(map[int]bool)
		from, to := dg.anchorRow, row
		if from > to {
			from, to = to, from
		}
		for i := from; i <= to; i++ {
			dg.selectedRows[i] = true
		}
	}

	dg.focusRow = row
	dg.ensureVisible(row)

	// Callback
	if dg.OnSelectionChanged != nil {
		var item interface{}
		if row >= 0 && row < len(dg.sortedIdx) {
			item = dg.itemsSource.Get(dg.sortedIdx[row])
		}
		go dg.OnSelectionChanged(SelectionChangedEvent{
			SelectedIndex: row,
			SelectedItem:  item,
		})
	}
}

// ─── Editing ───────────────────────────────────────────────────────────────

func (dg *DataGrid) beginEdit(row, col int) {
	if dg.IsReadOnly {
		return
	}
	if col < 0 || col >= len(dg.columns) || dg.columns[col].IsReadOnly() {
		return
	}
	if row < 0 || row >= len(dg.sortedIdx) {
		return
	}

	dataIdx := dg.sortedIdx[row]
	item := dg.itemsSource.Get(dataIdx)

	dg.isEditing = true
	dg.editingRow = row
	dg.editingCol = col
	dg.editingValue = dg.columns[col].GetCellValue(item)
	dg.editCursorPos = len([]rune(dg.editingValue))
	dg.focusRow = row
	dg.focusCol = col
}

func (dg *DataGrid) commitEdit() {
	if !dg.isEditing {
		return
	}

	col := dg.columns[dg.editingCol]
	dataIdx := dg.sortedIdx[dg.editingRow]
	item := dg.itemsSource.Get(dataIdx)

	// Callback
	if dg.OnCellEditEnding != nil {
		evt := &CellEditEndingEvent{
			RowIndex: dataIdx,
			Column:   col,
			Item:     item,
			NewValue: dg.editingValue,
		}
		dg.OnCellEditEnding(evt)
		if evt.Cancel {
			dg.cancelEdit()
			return
		}
	}

	// Записываем значение в модель
	col.SetCellValue(item, dg.editingValue)

	// Уведомляем о завершении редактирования строки
	if dg.OnRowEditEnding != nil {
		go dg.OnRowEditEnding(dataIdx, item)
	}

	dg.isEditing = false
	dg.editingRow = -1
	dg.editingCol = -1
}

func (dg *DataGrid) cancelEdit() {
	dg.isEditing = false
	dg.editingRow = -1
	dg.editingCol = -1
}

// ─── Keyboard ──────────────────────────────────────────────────────────────

// OnKeyEvent обрабатывает клавиатурный ввод.
func (dg *DataGrid) OnKeyEvent(code int, char rune, pressed bool, shift, ctrl bool) {
	if !pressed {
		return
	}

	dg.mu.Lock()
	defer dg.mu.Unlock()

	// Редактирование — специальная обработка
	if dg.isEditing {
		dg.handleEditKey(code, char, ctrl)
		return
	}

	rc := dg.rowCount()
	if rc == 0 {
		return
	}

	switch code {
	case 38: // Up
		if dg.focusRow > 0 {
			dg.selectRow(dg.focusRow-1, shift, ctrl)
		}
	case 40: // Down
		if dg.focusRow < rc-1 {
			dg.selectRow(dg.focusRow+1, shift, ctrl)
		}
	case 37: // Left
		if dg.focusCol > 0 {
			dg.focusCol--
		}
	case 39: // Right
		if dg.focusCol < len(dg.columns)-1 {
			dg.focusCol++
		}
	case 36: // Home
		dg.selectRow(0, shift, ctrl)
	case 35: // End
		dg.selectRow(rc-1, shift, ctrl)
	case 33: // PageUp
		page := dg.viewHeight() / dg.RowHeight
		if page < 1 {
			page = 1
		}
		newRow := dg.focusRow - page
		if newRow < 0 {
			newRow = 0
		}
		dg.selectRow(newRow, shift, ctrl)
	case 34: // PageDown
		page := dg.viewHeight() / dg.RowHeight
		if page < 1 {
			page = 1
		}
		newRow := dg.focusRow + page
		if newRow >= rc {
			newRow = rc - 1
		}
		dg.selectRow(newRow, shift, ctrl)
	case 9: // Tab
		if shift {
			if dg.focusCol > 0 {
				dg.focusCol--
			} else if dg.focusRow > 0 {
				dg.focusRow--
				dg.focusCol = len(dg.columns) - 1
				dg.selectRow(dg.focusRow, false, false)
			}
		} else {
			if dg.focusCol < len(dg.columns)-1 {
				dg.focusCol++
			} else if dg.focusRow < rc-1 {
				dg.focusRow++
				dg.focusCol = 0
				dg.selectRow(dg.focusRow, false, false)
			}
		}
	case 13: // Enter — начать редактирование
		if dg.focusRow >= 0 && dg.focusCol >= 0 {
			dg.beginEdit(dg.focusRow, dg.focusCol)
		}
	case 27: // Escape
		if dg.isEditing {
			dg.cancelEdit()
		}
	case 65: // A (Ctrl+A — выделить всё)
		if ctrl && dg.SelectionMode == SelectionExtended {
			dg.selectedRows = make(map[int]bool)
			for i := 0; i < rc; i++ {
				dg.selectedRows[i] = true
			}
		}
	}
}

// handleEditKey обрабатывает ввод в режиме редактирования.
func (dg *DataGrid) handleEditKey(code int, char rune, ctrl bool) {
	switch code {
	case 13: // Enter — commit
		dg.commitEdit()
	case 27: // Escape — cancel
		dg.cancelEdit()
	case 8: // Backspace
		if dg.editCursorPos > 0 {
			runes := []rune(dg.editingValue)
			runes = append(runes[:dg.editCursorPos-1], runes[dg.editCursorPos:]...)
			dg.editingValue = string(runes)
			dg.editCursorPos--
		}
	case 46: // Delete
		runes := []rune(dg.editingValue)
		if dg.editCursorPos < len(runes) {
			runes = append(runes[:dg.editCursorPos], runes[dg.editCursorPos+1:]...)
			dg.editingValue = string(runes)
		}
	case 37: // Left
		if dg.editCursorPos > 0 {
			dg.editCursorPos--
		}
	case 39: // Right
		if dg.editCursorPos < len([]rune(dg.editingValue)) {
			dg.editCursorPos++
		}
	case 36: // Home
		dg.editCursorPos = 0
	case 35: // End
		dg.editCursorPos = len([]rune(dg.editingValue))
	default:
		// Печатаемый символ
		if char > 0 && !ctrl {
			runes := []rune(dg.editingValue)
			newRunes := make([]rune, 0, len(runes)+1)
			newRunes = append(newRunes, runes[:dg.editCursorPos]...)
			newRunes = append(newRunes, char)
			newRunes = append(newRunes, runes[dg.editCursorPos:]...)
			dg.editingValue = string(newRunes)
			dg.editCursorPos++
		}
	}
}

// ─── Focus ─────────────────────────────────────────────────────────────────

// SetFocused устанавливает фокус.
func (dg *DataGrid) SetFocused(v bool) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	dg.focused = v
}

// IsFocused возвращает состояние фокуса.
func (dg *DataGrid) IsFocused() bool {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.focused
}

// ─── Theme ─────────────────────────────────────────────────────────────────

// DataGridTheme — набор цветов для DataGrid.
type DataGridTheme struct {
	Background       color.RGBA
	HeaderBG         color.RGBA
	HeaderText       color.RGBA
	TextColor        color.RGBA
	BorderColor      color.RGBA
	SelectColor      color.RGBA
	HoverColor       color.RGBA
	AlternateBG      color.RGBA
	GridLineColor    color.RGBA
	ScrollTrackBG    color.RGBA
	ScrollThumbBG    color.RGBA
	ScrollThumbHover color.RGBA
	EditBG           color.RGBA
	EditBorder       color.RGBA
}

// ApplyTheme применяет тему к DataGrid.
func (dg *DataGrid) ApplyTheme(t *DataGridTheme) {
	dg.Background = t.Background
	dg.HeaderBG = t.HeaderBG
	dg.HeaderText = t.HeaderText
	dg.TextColor = t.TextColor
	dg.BorderColor = t.BorderColor
	dg.SelectColor = t.SelectColor
	dg.HoverColor = t.HoverColor
	dg.AlternateBG = t.AlternateBG
	dg.GridLineColor = t.GridLineColor
	dg.ScrollTrackBG = t.ScrollTrackBG
	dg.ScrollThumbBG = t.ScrollThumbBG
	dg.ScrollThumbHover = t.ScrollThumbHover
	dg.EditBG = t.EditBG
	dg.EditBorder = t.EditBorder
}
