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
	"time"
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

	// itemsSubID — дескриптор подписки на itemsSource.CollectionChanged
	// (SEC-11). Снимается при перебиндовке источника и в Dispose; без этого
	// N вызовов SetItemsSource = N живых замыканий, каждое из которых держит
	// грид и перестраивает индекс на каждое изменение старой коллекции.
	itemsSubID int

	// ── Свойства (WPF-совместимые) ───────────────────────────────────────
	AutoGenerateColumns bool
	IsReadOnly          bool
	CanUserSortColumns  bool

	// CanUserReorderColumns — можно ли менять порядок колонок, перетаскивая
	// заголовок мышью (reorder.go).
	//
	// По умолчанию ВЫКЛЮЧЕНО, хотя в WPF включено. Включение меняет момент
	// срабатывания щелчка по заголовку: сортировка и OnHeaderClick уходят с
	// нажатия на ОТПУСКАНИЕ — иначе нельзя отличить щелчок от захвата, и
	// всякая попытка потащить колонку заодно пересортировывала бы таблицу.
	// Менять это молча у всех, кто уже полагается на прежний момент, нельзя.
	CanUserReorderColumns bool

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
	scrollY  int
	scrollX  int // горизонтальный скролл (для широких таблиц)
	hoverRow int

	// notifiedScrollY — положение прокрутки, о котором уже сообщили в
	// OnScroll: без него событие сыпалось бы на каждый клампинг.
	notifiedScrollY int

	// Перетаскивание колонки за заголовок (reorder.go). dragCol == -1 —
	// нажатия на заголовке нет.
	dragCol    int
	dragStartX int
	dragX      int
	dragging   bool

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

	// ZebraStripes — чередовать ли фон нечётных строк (AlternateBG).
	//
	// По умолчанию да — так таблица выглядела всегда. Отключить это одним
	// лишь приравниванием AlternateBG к Background было нельзя: ApplyTheme
	// вычисляет AlternateBG из фона темы заново на каждую смену темы и
	// затирает всё, что приложение туда положило. Цветом по-прежнему владеет
	// тема, а признаком «полосы нужны» — приложение.
	ZebraStripes bool

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

	// RowToolTip — текст всплывающей подсказки для строки под курсором.
	//
	// У Base.ToolTip один текст на весь виджет, а строке нужен свой:
	// состояние файла, причина конфликта, полный путь. Колбэк, а не поле в
	// модели, по той же причине, что и CellRenderer, — текст берётся из
	// приложения и может быть локализован на лету.
	//
	// Пустая строка означает «подсказки нет»: тогда показывается общий
	// ToolTip виджета, если он задан.
	RowToolTip func(item interface{}, row int) string

	// OnHeaderClick — щелчок по заголовку колонки.
	//
	// Вызывается ДО сортировки и НЕЗАВИСИМО от CanUserSortColumns: клик по
	// заголовку — это не обязательно «сортируй», это может быть и меню
	// выбора видимых колонок. Раньше две эти реакции нельзя было развести:
	// единственным способом отобрать клик у сортировки было выключить её
	// целиком и разбирать мышь в полосе заголовка самому — вместе с
	// различением «клик» / «начало resize» / «начало перетаскивания».
	//
	// Возврат true означает «клик разобран»: сортировка не выполняется.
	// Возврат false оставляет прежнее поведение.
	//
	// Вызывается ВНЕ внутреннего замка — обработчик может звать методы
	// таблицы, не рискуя взаимной блокировкой.
	OnHeaderClick func(col Column, colIndex, x, y int) bool

	// OnColumnsReordered — колонку переставили перетаскиванием или вызовом
	// MoveColumn. from и to — индексы до и после перестановки.
	OnColumnsReordered func(from, to int)

	// OnScroll — прокрутка сдвинулась: first — первая видимая строка,
	// count — сколько строк помещается в окне.
	//
	// Нужно подгрузке следующей порции данных: без события её приходилось
	// вешать на выбор строки рядом с концом списка, то есть требовать от
	// человека щёлкнуть там, где он просто хотел прокрутить.
	//
	// Зовётся ВНЕ внутреннего замка — обработчик волен добавить строки в
	// коллекцию прямо из него.
	OnScroll func(first, count int)

	// RowStyleSelector — условная раскраска строк по значению модели (BUG-3).
	// Вызывается для каждой видимой строки перед отрисовкой её содержимого.
	// item — элемент модели, rowIndex — индекс в исходной коллекции.
	// Если ok=true, bg используется как фон строки (вместо AlternatingRowBackground);
	// выделение и hover рисуются поверх. Аналог WPF RowStyle/RowBackground.
	RowStyleSelector func(item interface{}, rowIndex int) (bg color.RGBA, ok bool)

	// OnRowActivated — двойной клик / Enter по строке.
	//
	// Срабатывает ВСЕГДА, независимо от IsReadOnly и состояния
	// редактирования. Используется для UX-сценариев типа
	// «открыть детали», «toggle breakpoint в read-only грид
	// дизассемблера». Если дополнительно стартует inline-edit
	// (грид и колонка editable) — OnRowActivated вызывается ДО
	// beginEdit, чтобы дать обработчику возможность выполнить
	// своё действие до перехода в режим редактирования.
	OnRowActivated func(rowIndex int, item interface{})

	// ── Внутреннее состояние ─────────────────────────────────────────────
	mu      sync.Mutex
	focused bool
	dirty   bool // layout нужно пересчитать

	// pending — колбэки, накопленные под dg.mu; вызываются синхронно ПОСЛЕ
	// освобождения мьютекса (см. firePending) — обработчик может безопасно
	// дёргать методы DataGrid, не рискуя дедлоком.
	pending []func()

	// ── Точечная инвалидация (damage) ────────────────────────────────────
	// Интерактивные обработчики (выбор строки, hover, скролл, сортировка)
	// накапливают сюда изменившиеся АБСОЛЮТНЫЕ прямоугольники. Обёртка
	// (widget.DataGridWidget) забирает их через TakeDirty и транслирует в
	// точечный InvalidateRect вместо инвалидации всего виджета. dirtyFull —
	// когда сдвигается весь контент (скролл/сортировка/resize колонок).
	dirtyRects []image.Rectangle
	dirtyFull  bool

	// ── Кэш текста ячеек (PERF-3) ────────────────────────────────────────
	// Живёт под собственным мьютексом: Draw работает вне dg.mu (PERF-8) и
	// не должен блокировать обработчики мыши, а инвалидация из-под dg.mu
	// не должна ждать кадр. Порядок захвата всегда dg.mu → cacheMu,
	// обратный запрещён.
	cacheMu    sync.Mutex
	cellCache  map[cellKey]string
	cacheGen   uint64              // поколение; инвалидация = ++
	itemSubs   map[interface{}]int // item → id подписки на PropertyChanged
	cacheLoRow int                 // удерживаемое окно строк [lo..hi]
	cacheHiRow int
}

// cellKey — координата ячейки в кэше текста (row — индекс в sortedIdx).
type cellKey struct{ row, col int }

// maxItemSubs ограничивает число подписок на PropertyChanged элементов:
// кэшируются только видимые ячейки, так что окна с запасом хватает.
// Сверх лимита новые элементы не кэшируются вовсе — лучше медленно, чем
// показать устаревший текст.
const maxItemSubs = 512

// notifierWithHandle — элемент модели, умеющий уведомлять об изменении
// свойств с отпиской по дескриптору (реализуется PropertyNotifier).
type notifierWithHandle interface {
	AddPropertyChangedHandle(PropertyChangedHandler) int
	RemovePropertyChangedHandle(int)
}

// ─── Кэш текста ячеек (PERF-3) ─────────────────────────────────────────────

// InvalidateCells сбрасывает кэш отформатированного текста ячеек.
//
// Вызывается автоматически при изменении коллекции, источника, колонок,
// сортировки и при коммите редактирования, а также по PropertyChanged
// элементов, реализующих INotifyPropertyChanged. Если модель меняется
// «молча» (обычная структура без уведомлений), вызовите метод вручную —
// иначе таблица продолжит показывать прежний текст.
func (dg *DataGrid) InvalidateCells() { dg.invalidateCellCache() }

// invalidateCellCache полностью сбрасывает кэш и снимает подписки на
// PropertyChanged. Безопасно вызывать как под dg.mu, так и без него.
func (dg *DataGrid) invalidateCellCache() {
	dg.cacheMu.Lock()
	dg.cacheGen++
	dg.cellCache = nil
	subs := dg.itemSubs
	dg.itemSubs = nil
	dg.cacheMu.Unlock()

	// Отписка — вне cacheMu: обработчик уведомителя сам берёт свой лок.
	for item, id := range subs {
		if n, ok := item.(notifierWithHandle); ok {
			n.RemovePropertyChangedHandle(id)
		}
	}
}

// invalidateCellRow сбрасывает кэш одной строки (после коммита правки).
func (dg *DataGrid) invalidateCellRow(row int) {
	dg.cacheMu.Lock()
	for k := range dg.cellCache {
		if k.row == row {
			delete(dg.cellCache, k)
		}
	}
	dg.cacheMu.Unlock()
}

// cellText возвращает отформатированный текст ячейки, беря его из кэша.
// gen — поколение кэша, снятое в начале кадра: если за время отрисовки
// данные сменились, результат не оседает в кэше.
//
// Значение вычисляется ВНЕ cacheMu (reflect + Sprintf — самая дорогая
// часть кадра), под локом только карта.
func (dg *DataGrid) cellText(gen uint64, row, col int, c Column, item interface{}) string {
	k := cellKey{row: row, col: col}

	dg.cacheMu.Lock()
	if dg.cacheGen == gen {
		if s, ok := dg.cellCache[k]; ok {
			dg.cacheMu.Unlock()
			return s
		}
	}
	dg.cacheMu.Unlock()

	s := c.GetCellValue(item)

	dg.cacheMu.Lock()
	if dg.cacheGen != gen {
		dg.cacheMu.Unlock()
		return s
	}
	// Элемент с уведомлениями — подписываемся, чтобы сбросить кэш при
	// изменении свойства. Не подписались (лимит/несравнимый ключ) — не кэшируем.
	if n, ok := item.(notifierWithHandle); ok {
		if !dg.trackNotifierLocked(item, n) {
			dg.cacheMu.Unlock()
			return s
		}
	}
	if dg.cellCache == nil {
		dg.cellCache = make(map[cellKey]string, 256)
	}
	dg.cellCache[k] = s
	dg.cacheMu.Unlock()
	return s
}

// trackNotifierLocked подписывается на PropertyChanged элемента (idempotent).
// Возвращает false, если подписаться нельзя — тогда значение не кэшируется.
// Вызывать под cacheMu.
func (dg *DataGrid) trackNotifierLocked(item interface{}, n notifierWithHandle) bool {
	// Ключ карты обязан быть сравнимым: структура со слайсом внутри
	// уронила бы приложение при вставке.
	if t := reflect.TypeOf(item); t == nil || !t.Comparable() {
		return false
	}
	if _, ok := dg.itemSubs[item]; ok {
		return true
	}
	if len(dg.itemSubs) >= maxItemSubs {
		return false
	}
	if dg.itemSubs == nil {
		dg.itemSubs = make(map[interface{}]int, 64)
	}
	dg.itemSubs[item] = n.AddPropertyChangedHandle(func(interface{}, string) {
		dg.invalidateCellCache()
	})
	return true
}

// trimCellCache выбрасывает записи вне удерживаемого окна строк и возвращает
// актуальное поколение кэша. Кэш держит только видимое окно с запасом — на
// 100k строк в памяти всё равно остаётся пара сотен записей.
//
// Если подписок на PropertyChanged накопилось слишком много (долгая прокрутка
// по модели с уведомлениями), кэш сбрасывается целиком — это дешевле, чем
// вести обратный индекс «элемент → ячейки».
func (dg *DataGrid) trimCellCache(lo, hi int) uint64 {
	dg.cacheMu.Lock()
	dg.cacheLoRow, dg.cacheHiRow = lo, hi
	tooManySubs := len(dg.itemSubs) > maxItemSubs/2
	if !tooManySubs && len(dg.cellCache) > 4*(hi-lo+1)*8+256 {
		for k := range dg.cellCache {
			if k.row < lo || k.row > hi {
				delete(dg.cellCache, k)
			}
		}
	}
	gen := dg.cacheGen
	dg.cacheMu.Unlock()

	if tooManySubs {
		dg.invalidateCellCache()
		dg.cacheMu.Lock()
		gen = dg.cacheGen
		dg.cacheMu.Unlock()
	}
	return gen
}

// ─── Damage tracking (точечная инвалидация) ────────────────────────────────

// markFullDirty помечает, что нужна полная перерисовка виджета.
func (dg *DataGrid) markFullDirty() { dg.dirtyFull = true }

// markRectDirty добавляет абсолютный прямоугольник в накопитель damage
// (пересекается с bounds, дубликаты отбрасываются). Вызывать под dg.mu.
func (dg *DataGrid) markRectDirty(r image.Rectangle) {
	r = r.Intersect(dg.bounds)
	if r.Empty() {
		return
	}
	for _, e := range dg.dirtyRects {
		if e == r {
			return
		}
	}
	dg.dirtyRects = append(dg.dirtyRects, r)
}

// rowRectAbs возвращает абсолютный прямоугольник видимой части строки row
// (пустой, если строка вне области данных / прокручена за пределы).
func (dg *DataGrid) rowRectAbs(row int) image.Rectangle {
	if row < 0 || row >= dg.rowCount() {
		return image.Rectangle{}
	}
	dr := dg.dataRect()
	rowY := dr.Min.Y + row*dg.RowHeight - dg.scrollY
	return image.Rect(dr.Min.X, rowY, dr.Max.X, rowY+dg.RowHeight).Intersect(dr)
}

// markRowDirty помечает область строки row как изменившуюся. Вызывать под dg.mu.
func (dg *DataGrid) markRowDirty(row int) { dg.markRectDirty(dg.rowRectAbs(row)) }

// TakeDirty возвращает накопленные области изменения (абсолютные координаты) и
// сбрасывает накопитель. full=true — требуется полная перерисовка виджета.
// Потокобезопасно. Обёртка вызывает его после каждого обработчика ввода.
func (dg *DataGrid) TakeDirty() (rects []image.Rectangle, full bool) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	rects, full = dg.dirtyRects, dg.dirtyFull
	dg.dirtyRects = nil
	dg.dirtyFull = false
	return
}

// IsEditing сообщает, идёт ли сейчас редактирование ячейки (мигает каретка).
func (dg *DataGrid) IsEditing() bool {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.isEditing
}

// firePending вызывает накопленные колбэки вне dg.mu. Регистрируется через
// defer ДО dg.mu.Lock() в публичных обработчиках событий (LIFO: Unlock
// выполнится раньше).
func (dg *DataGrid) firePending() {
	dg.mu.Lock()
	fs := dg.pending
	dg.pending = nil
	dg.mu.Unlock()
	for _, f := range fs {
		f()
	}
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
		dragCol:              -1,
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
		ZebraStripes:    true,
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

// Bounds возвращает прямоугольник виджета.
func (dg *DataGrid) Bounds() image.Rectangle {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.bounds
}

// SetBounds задаёт прямоугольник виджета и помечает layout грязным.
func (dg *DataGrid) SetBounds(r image.Rectangle) {
	dg.mu.Lock()
	dg.bounds = r
	dg.dirty = true
	dg.mu.Unlock()
}

// ─── Columns ───────────────────────────────────────────────────────────────

// AddColumn добавляет колонку.
func (dg *DataGrid) AddColumn(col Column) {
	if col == nil {
		return
	}
	dg.mu.Lock()
	dg.columns = append(dg.columns, col)
	dg.dirty = true
	dg.markFullDirty()
	dg.mu.Unlock()
	dg.invalidateCellCache()
}

// Columns возвращает копию среза колонок.
//
// Копия, а не сам срез: вызывающий код не должен уметь подменить колонку
// «под» гридом мимо мьютекса (SEC-5) — сами объекты колонок остаются общими.
func (dg *DataGrid) Columns() []Column {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	out := make([]Column, len(dg.columns))
	copy(out, dg.columns)
	return out
}

// SetColumns заменяет все колонки.
//
// Смена набора колонок обесценивает индексы editingCol/resizingCol/focusCol:
// если их не сбросить, следующий commitEdit/resize залезет за границу нового
// среза и уронит приложение (SEC-5). Режим редактирования отменяется БЕЗ
// коммита — редактируемой колонки больше не существует.
func (dg *DataGrid) SetColumns(cols []Column) {
	dg.mu.Lock()
	dg.columns = cols
	dg.dirty = true
	dg.markFullDirty()

	if dg.isEditing {
		dg.isEditing = false
		dg.editingRow, dg.editingCol = -1, -1
		dg.editingValue, dg.editCursorPos = "", 0
	}
	dg.resizingCol = -1
	if dg.focusCol >= len(cols) {
		dg.focusCol = len(cols) - 1
	}
	if dg.focusCol < 0 {
		dg.focusCol = 0
	}
	dg.mu.Unlock()
	dg.invalidateCellCache()
}

// Dispose снимает подписку на источник данных и освобождает кэш.
// Вызывать при выбрасывании грида из дерева виджетов (SEC-11).
func (dg *DataGrid) Dispose() {
	dg.mu.Lock()
	if dg.itemsSource != nil && dg.itemsSubID > 0 {
		dg.itemsSource.RemoveCollectionChanged(dg.itemsSubID)
	}
	dg.itemsSubID = 0
	dg.itemsSource = nil
	dg.sortedIdx = nil
	dg.selectedRows = make(map[int]bool)
	dg.focusRow = -1
	dg.isEditing = false
	dg.editingRow, dg.editingCol = -1, -1
	dg.mu.Unlock()
	dg.invalidateCellCache()
}

// ─── Data Source ────────────────────────────────────────────────────────────

// SetItemsSource задаёт источник данных.
//
// Со СТАРОГО источника подписка снимается (SEC-11): иначе каждая
// перебиндовка оставляла живое замыкание, которое держит грид и продолжает
// перестраивать его индекс на каждое изменение уже ненужной коллекции.
func (dg *DataGrid) SetItemsSource(oc *ObservableCollection) {
	dg.mu.Lock()

	if dg.itemsSource != nil && dg.itemsSubID > 0 {
		dg.itemsSource.RemoveCollectionChanged(dg.itemsSubID)
	}
	dg.itemsSubID = 0

	dg.itemsSource = oc
	dg.rebuildSortedIdx()
	dg.selectedRows = make(map[int]bool)
	dg.focusRow = -1
	dg.scrollY = 0
	// Источник сменился — редактирование прежней строки бессмысленно.
	dg.isEditing = false
	dg.editingRow, dg.editingCol = -1, -1
	dg.editingValue, dg.editCursorPos = "", 0
	dg.markFullDirty()

	if oc != nil {
		// Подписка на изменения
		dg.itemsSubID = oc.AddCollectionChanged(func(event CollectionChangedEvent) {
			dg.mu.Lock()
			dg.rebuildSortedIdx()
			dg.mu.Unlock()
			dg.invalidateCellCache()
		})

		// Авто-генерация колонок
		if dg.AutoGenerateColumns && len(dg.columns) == 0 && oc.Count() > 0 {
			dg.autoGenerateColumns(oc.Get(0))
		}
	}
	dg.mu.Unlock()

	dg.invalidateCellCache()
}

// ItemsSource возвращает источник данных.
func (dg *DataGrid) ItemsSource() *ObservableCollection {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.itemsSource
}

// rebuildSortedIdx пересоздаёт индексный массив (без сортировки).
// Вызывать под dg.mu.
func (dg *DataGrid) rebuildSortedIdx() {
	if dg.itemsSource == nil {
		dg.sortedIdx = nil
		dg.clampStateToRows()
		return
	}
	n := dg.itemsSource.Count()
	if cap(dg.sortedIdx) >= n {
		dg.sortedIdx = dg.sortedIdx[:n]
	} else {
		dg.sortedIdx = make([]int, n)
	}
	for i := 0; i < n; i++ {
		dg.sortedIdx[i] = i
	}
	// Переприменяем текущую сортировку
	dg.applyCurrentSort()
	dg.clampStateToRows()
}

// clampStateToRows приводит индексы состояния к новому числу строк (SEC-5).
//
// Коллекция может сжаться из фоновой горутины прямо посреди редактирования:
// без коррекции следующий commitEdit/Draw уходит за границу sortedIdx и
// роняет приложение. Исчезнувшая строка = отмена редактирования БЕЗ коммита
// (записывать значение уже некуда). Вызывать под dg.mu.
func (dg *DataGrid) clampStateToRows() {
	n := len(dg.sortedIdx)

	if dg.isEditing && (dg.editingRow < 0 || dg.editingRow >= n ||
		dg.editingCol < 0 || dg.editingCol >= len(dg.columns)) {
		dg.isEditing = false
		dg.editingRow, dg.editingCol = -1, -1
		dg.editingValue, dg.editCursorPos = "", 0
	}
	if dg.focusRow >= n {
		dg.focusRow = n - 1
	}
	if dg.anchorRow >= n {
		dg.anchorRow = n - 1
	}
	if dg.hoverRow >= n {
		dg.hoverRow = -1
	}
	for r := range dg.selectedRows {
		if r < 0 || r >= n {
			delete(dg.selectedRows, r)
		}
	}
	dg.clampScrollY()
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
// ItemAtRow возвращает элемент модели, показанный в строке row.
//
// Строка — номер в ТЕКУЩЕМ порядке (с учётом сортировки), тот же, что отдают
// RowIndexAtY и HoverRow. Без этого метода снаружи оставалось только повторять
// пересчёт через sortedIdx, которого наружу нет.
func (dg *DataGrid) ItemAtRow(row int) interface{} {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	if row < 0 || row >= len(dg.sortedIdx) || dg.itemsSource == nil {
		return nil
	}
	return dg.itemsSource.Get(dg.sortedIdx[row])
}

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
	dg.queueScrollNotify()
}

// queueScrollNotify откладывает вызов OnScroll, если прокрутка действительно
// сдвинулась. Вызывается под dg.mu.
//
// Живёт в clampScrollY, а не в каждом месте, где меняется scrollY: клампинг —
// единственное, что делают ВСЕ такие места, и это гарантирует, что ни один
// путь прокрутки не забудет сообщить о себе.
func (dg *DataGrid) queueScrollNotify() {
	if dg.OnScroll == nil || dg.scrollY == dg.notifiedScrollY {
		return
	}
	dg.notifiedScrollY = dg.scrollY
	cb := dg.OnScroll
	first, count := dg.firstVisibleRowLocked(), dg.visibleRowCountLocked()
	dg.pending = append(dg.pending, func() { cb(first, count) })
}

// FirstVisibleRow возвращает индекс первой видимой строки.
//
// Виртуализация в таблице есть с самого начала — рисуются только видимые
// строки, — но снаружи об этом нельзя было спросить: подгрузке следующей
// порции нужно знать, докуда человек долистал.
func (dg *DataGrid) FirstVisibleRow() int {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.firstVisibleRowLocked()
}

// VisibleRowCount возвращает, сколько строк помещается в окне таблицы.
func (dg *DataGrid) VisibleRowCount() int {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.visibleRowCountLocked()
}

func (dg *DataGrid) firstVisibleRowLocked() int {
	if dg.RowHeight <= 0 {
		return 0
	}
	return dg.scrollY / dg.RowHeight
}

func (dg *DataGrid) visibleRowCountLocked() int {
	if dg.RowHeight <= 0 {
		return 0
	}
	n := dg.viewHeight() / dg.RowHeight
	if n < 0 {
		return 0
	}
	return n
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

	// Сортировка меняет порядок строк и индикатор в заголовке — полная перерисовка.
	dg.markFullDirty()

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

	// Callback — строго ВНЕ dg.mu (SEC-4): sync.Mutex нерекурсивен, а
	// обработчик сортировки почти всегда лезет в SelectedItem()/ScrollBy().
	// Сортировку применяем в том же отложенном вызове — обработчику надо
	// дать шанс выставить Handled и отсортировать данные самому.
	if dg.OnSorting != nil {
		cb := dg.OnSorting
		evt := &SortingEvent{Column: col, Direction: dir}
		dg.pending = append(dg.pending, func() {
			cb(evt)
			if evt.Handled {
				return
			}
			dg.mu.Lock()
			dg.applyCurrentSort()
			dg.mu.Unlock()
			dg.invalidateCellCache()
		})
		return
	}

	dg.applyCurrentSort()
	dg.invalidateCellCache()
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

	n := len(dg.sortedIdx)
	if n < 2 {
		return
	}

	// Decorate-sort-undecorate (PERF-4): раньше КАЖДОЕ сравнение делало два
	// GetPropertyValue (reflect + strings.Split + FieldByName), Sprintf и
	// ToLower — то есть 2·n·log₂n рефлексивных обходов на сортировку.
	// Теперь ключи извлекаются один раз (n обходов), а сравниваются уже
	// типизированные значения.
	src := dg.itemsSource
	keys := make([]sortKey, n)
	for i, idx := range dg.sortedIdx {
		v, ok := GetPropertyValue(src.Get(idx), path)
		if !ok {
			v = nil
		}
		keys[i] = makeSortKey(v)
	}
	kind := commonSortKind(keys)

	// Сортируем перестановку позиций, а не сам sortedIdx: сортировка
	// переставляет элементы, а ключи привязаны к исходным позициям.
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}
	desc := dir == SortDescending
	sort.SliceStable(perm, func(a, b int) bool {
		cmp := compareSortKeys(&keys[perm[a]], &keys[perm[b]], kind)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})

	old := make([]int, n)
	copy(old, dg.sortedIdx)
	for i, p := range perm {
		dg.sortedIdx[i] = old[p]
	}
}

// ─── Ключи сортировки (decorate-sort-undecorate) ───────────────────────────

// sortKind — тип ключа сортировки колонки.
type sortKind uint8

const (
	kindNil    sortKind = iota // значение отсутствует
	kindNumber                 // int/uint/float — сравнение по float64
	kindBool                   // false < true
	kindTime                   // time.Time — сравнение по моменту времени
	kindString                 // строки и всё прочее — по нижнему регистру
	kindMixed                  // в колонке разные типы — общий compareValues
)

// sortKey — предизвлечённый ключ одной строки.
type sortKey struct {
	kind sortKind
	num  float64     // kindNumber; kindBool: 0/1
	unix int64       // kindTime — UnixNano (float64 потерял бы наносекунды)
	str  string      // kindString — уже в нижнем регистре
	raw  interface{} // сырое значение — только для kindMixed
}

// makeSortKey раскладывает значение свойства в типизированный ключ.
func makeSortKey(v interface{}) sortKey {
	if v == nil {
		return sortKey{kind: kindNil}
	}
	switch t := v.(type) {
	case string:
		return sortKey{kind: kindString, str: strings.ToLower(t), raw: v}
	case time.Time:
		return sortKey{kind: kindTime, unix: t.UnixNano(), raw: v}
	case bool:
		var f float64
		if t {
			f = 1
		}
		return sortKey{kind: kindBool, num: f, raw: v}
	}
	rv := reflect.ValueOf(v)
	if isNumeric(rv) {
		return sortKey{kind: kindNumber, num: toFloat64(rv), raw: v}
	}
	if rv.Kind() == reflect.Bool {
		var f float64
		if rv.Bool() {
			f = 1
		}
		return sortKey{kind: kindBool, num: f, raw: v}
	}
	return sortKey{kind: kindString, str: strings.ToLower(valToString(v)), raw: v}
}

// commonSortKind определяет, однородна ли колонка. Неоднородная (kindMixed)
// сравнивается прежним compareValues — семантика не меняется, но значения
// уже извлечены и рефлексия по пути больше не повторяется.
func commonSortKind(keys []sortKey) sortKind {
	k := kindNil
	for i := range keys {
		if keys[i].kind == kindNil {
			continue
		}
		if k == kindNil {
			k = keys[i].kind
			continue
		}
		if keys[i].kind != k {
			return kindMixed
		}
	}
	return k
}

// compareSortKeys сравнивает два ключа: -1 / 0 / 1. nil всегда меньше любого
// значения — как и в прежнем compareValues.
func compareSortKeys(a, b *sortKey, kind sortKind) int {
	if a.kind == kindNil || b.kind == kindNil {
		switch {
		case a.kind == kindNil && b.kind == kindNil:
			return 0
		case a.kind == kindNil:
			return -1
		default:
			return 1
		}
	}
	switch kind {
	case kindNumber, kindBool:
		switch {
		case a.num < b.num:
			return -1
		case a.num > b.num:
			return 1
		}
		return 0
	case kindTime:
		switch {
		case a.unix < b.unix:
			return -1
		case a.unix > b.unix:
			return 1
		}
		return 0
	case kindString:
		return strings.Compare(a.str, b.str)
	}
	return compareValues(a.raw, b.raw)
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

// drawSnapshot — согласованный слепок состояния для одного кадра (PERF-8).
//
// Раньше dg.mu держался на всю отрисовку: медленный кадр заставлял мышь и
// фоновые изменения коллекции ждать. Теперь под локом снимается только
// слепок (десятки байт + видимое окно строк), а рисование идёт без лока.
type drawSnapshot struct {
	bounds     image.Rectangle
	headerRect image.Rectangle
	dataRect   image.Rectangle
	dataW      int // ширина области данных (без скроллбара)
	rowH       int
	headerH    int
	fontSize   float64
	scrollX    int
	scrollY    int

	cols     []Column
	widths   []int
	headers  []string
	sortDirs []SortDirection

	needScroll  bool
	scrollbar   image.Rectangle
	thumb       image.Rectangle
	thumbActive bool

	// Видимое окно строк: startRow..endRow включительно (rows == nil — пусто).
	startRow int
	rows     []rowSnapshot

	isEditing  bool
	editRow    int
	editCol    int
	editValue  string
	editCursor int
	focused    bool

	rowStyle func(interface{}, int) (color.RGBA, bool)
	gen      uint64

	bg, headerBG, headerText, textColor       color.RGBA
	borderColor, selectColor, hoverColor      color.RGBA
	alternateBG, gridLine                     color.RGBA
	scrollTrack, scrollThumb, scrollThumbHigh color.RGBA
	editBG, editBorder                        color.RGBA
	zebra                                     bool

	// Перетаскивание колонки: что тащат и куда встанет (reorder.go).
	dragging bool
	dragCol  int
	dropX    int
}

// rowSnapshot — данные одной видимой строки на кадр.
type rowSnapshot struct {
	dataIdx  int
	item     interface{}
	selected bool
	hovered  bool
}

// snapshotForDraw снимает слепок состояния под dg.mu (короткий лок).
// ok=false — рисовать нечего.
func (dg *DataGrid) snapshotForDraw() (s drawSnapshot, ok bool) {
	dg.mu.Lock()

	b := dg.bounds
	if b.Empty() || len(dg.columns) == 0 {
		dg.mu.Unlock()
		return s, false
	}

	// Пересчёт ширин колонок — состояние, поэтому под локом.
	if dg.dirty {
		dg.layoutColumns()
		dg.dirty = false
	}

	s.bounds = b
	s.headerRect = dg.headerRect()
	s.dataRect = dg.dataRect()
	s.rowH = dg.RowHeight
	s.headerH = dg.HeaderHeight
	s.fontSize = dg.FontSize
	s.scrollX = dg.scrollX
	s.scrollY = dg.scrollY

	n := len(dg.columns)
	s.cols = make([]Column, n)
	s.widths = make([]int, n)
	s.headers = make([]string, n)
	s.sortDirs = make([]SortDirection, n)
	copy(s.cols, dg.columns)
	for i, c := range dg.columns {
		s.widths[i] = c.ActualWidth()
		s.headers[i] = c.Header()
		s.sortDirs[i] = c.GetSortDirection()
	}

	s.needScroll = dg.needsScrollbar()
	s.dataW = b.Dx()
	if s.needScroll {
		s.dataW -= scrollbarWidth
		s.scrollbar = dg.scrollbarRect()
		s.thumb = dg.thumbRect()
		s.thumbActive = dg.thumbHovered || dg.thumbDragging
	}

	rc := dg.rowCount()
	if rc > 0 && dg.itemsSource != nil {
		startRow := dg.scrollY / dg.RowHeight
		if startRow < 0 {
			startRow = 0
		}
		endRow := (dg.scrollY + dg.viewHeight()) / dg.RowHeight
		if endRow >= rc {
			endRow = rc - 1
		}
		if endRow >= startRow {
			s.startRow = startRow
			s.rows = make([]rowSnapshot, endRow-startRow+1)
			for row := startRow; row <= endRow; row++ {
				di := dg.sortedIdx[row]
				s.rows[row-startRow] = rowSnapshot{
					dataIdx:  di,
					item:     dg.itemsSource.Get(di),
					selected: dg.selectedRows[row],
					hovered:  row == dg.hoverRow,
				}
			}
		}
	}

	s.isEditing = dg.isEditing
	s.editRow, s.editCol = dg.editingRow, dg.editingCol
	s.editValue = dg.editingValue
	s.editCursor = dg.editCursorPos
	s.focused = dg.focused
	s.rowStyle = dg.RowStyleSelector

	s.bg, s.headerBG, s.headerText = dg.Background, dg.HeaderBG, dg.HeaderText
	s.textColor, s.borderColor = dg.TextColor, dg.BorderColor
	s.selectColor, s.hoverColor = dg.SelectColor, dg.HoverColor
	s.alternateBG, s.gridLine = dg.AlternateBG, dg.GridLineColor
	s.zebra = dg.ZebraStripes
	dg.headerDragSnapshot(&s)
	s.scrollTrack, s.scrollThumb = dg.ScrollTrackBG, dg.ScrollThumbBG
	s.scrollThumbHigh = dg.ScrollThumbHover
	s.editBG, s.editBorder = dg.EditBG, dg.EditBorder

	dg.mu.Unlock()
	return s, true
}

// Draw отрисовывает DataGrid.
//
// dg.mu держится только на снятие слепка; сама отрисовка (включая
// пользовательские RowStyleSelector и DrawCell) идёт без лока — медленный
// кадр больше не блокирует мышь и фоновые изменения коллекции (PERF-8).
//
// Следствие: чтение полей элементов модели (GetCellValue) происходит ВНЕ
// dg.mu. Состав коллекции менять из любой горутины безопасно — за это
// отвечает ObservableCollection; а поля самих элементов, как и в WPF,
// правит только UI-поток (либо модель синхронизируется сама).
func (dg *DataGrid) Draw(ctx DrawContextBridge) {
	s, ok := dg.snapshotForDraw()
	if !ok {
		return
	}

	// Кэш текста ячеек держим по видимому окну с запасом (PERF-3).
	lo, hi := s.startRow, s.startRow+len(s.rows)-1
	pad := len(s.rows) + 8
	s.gen = dg.trimCellCache(lo-pad, hi+pad)

	b := s.bounds

	// Глобальный клип по bounds — ничто не выйдет за пределы DataGrid.
	ctx.SetClip(b)
	defer ctx.ClearClip()

	// Фон
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), s.bg)

	// Заголовок
	dg.drawHeader(ctx, &s)
	dg.drawHeaderDrag(ctx, &s)

	// Строки данных (с виртуализацией) — собственный клип внутри
	dg.drawRows(ctx, &s)

	// Скроллбар
	if s.needScroll {
		dg.drawScrollbar(ctx, &s)
	}

	// Внешняя рамка
	ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), s.borderColor)
}

// drawHeader рисует заголовки колонок.
func (dg *DataGrid) drawHeader(ctx DrawContextBridge, s *drawSnapshot) {
	hr := s.headerRect
	ctx.FillRect(hr.Min.X, hr.Min.Y, hr.Dx(), hr.Dy(), s.headerBG)

	// Клиппинг по области заголовка (без скроллбара)
	clipRect := image.Rect(hr.Min.X, hr.Min.Y, hr.Min.X+s.dataW, hr.Max.Y)
	ctx.SetClip(clipRect)

	x := s.bounds.Min.X - s.scrollX
	for i := range s.cols {
		w := s.widths[i]
		if x+w > hr.Min.X && x < hr.Min.X+s.dataW {
			// Текст заголовка
			textX := x + 6
			textY := hr.Min.Y + (s.headerH-14)/2
			ctx.DrawTextSize(s.headers[i], textX, textY, s.fontSize, s.headerText)

			// Индикатор сортировки
			if s.sortDirs[i] != SortNone {
				arrow := "▲"
				if s.sortDirs[i] == SortDescending {
					arrow = "▼"
				}
				arrowX := x + w - 16
				ctx.DrawTextSize(arrow, arrowX, textY, s.fontSize, s.headerText)
			}

			// Разделитель колонки
			ctx.DrawVLine(x+w-1, hr.Min.Y, s.headerH, s.gridLine)
		}
		x += w
	}

	// Восстанавливаем глобальный клип по bounds
	ctx.SetClip(s.bounds)

	// Горизонтальная линия под заголовком
	ctx.DrawHLine(hr.Min.X, hr.Max.Y-1, hr.Dx(), s.borderColor)
}

// drawRows рисует видимые строки (виртуализация).
func (dg *DataGrid) drawRows(ctx DrawContextBridge, s *drawSnapshot) {
	if len(s.rows) == 0 {
		return
	}
	dr := s.dataRect
	clipRect := image.Rect(dr.Min.X, dr.Min.Y, dr.Min.X+s.dataW, dr.Max.Y)
	ctx.SetClip(clipRect)

	for i := range s.rows {
		row := s.startRow + i
		rs := &s.rows[i]
		rowY := dr.Min.Y + row*s.rowH - s.scrollY
		if rowY+s.rowH < dr.Min.Y || rowY >= dr.Max.Y {
			continue
		}

		// Базовый фон строки: пользовательский RowStyleSelector (BUG-3) имеет
		// приоритет над стандартным чередованием AlternatingRowBackground.
		drewBase := false
		if s.rowStyle != nil {
			if bg, ok := s.rowStyle(rs.item, rs.dataIdx); ok {
				ctx.FillRect(dr.Min.X, rowY, s.dataW, s.rowH, bg)
				drewBase = true
			}
		}
		if !drewBase && s.zebra && row%2 == 1 {
			ctx.FillRect(dr.Min.X, rowY, s.dataW, s.rowH, s.alternateBG)
		}
		// Выделение / hover рисуются поверх базового фона.
		if rs.selected {
			ctx.FillRectAlpha(dr.Min.X, rowY, s.dataW, s.rowH, s.selectColor)
		} else if rs.hovered {
			ctx.FillRect(dr.Min.X, rowY, s.dataW, s.rowH, s.hoverColor)
		}

		// Ячейки
		cellX := s.bounds.Min.X - s.scrollX
		for colIdx, col := range s.cols {
			w := s.widths[colIdx]
			cellRect := image.Rect(cellX, rowY, cellX+w, rowY+s.rowH)

			// Per-cell clip = пересечение ячейки с областью данных,
			// чтобы текст не вылезал ни за пределы ячейки, ни за хедер/нижнюю границу.
			cellClip := cellRect.Intersect(clipRect)
			if !cellClip.Empty() {
				ctx.SetClip(cellClip)
			}

			// Режим редактирования?
			if s.isEditing && s.editRow == row && s.editCol == colIdx {
				dg.drawEditCell(ctx, s, cellRect)
			} else {
				cdc := CellDrawContext{
					Rect:       cellRect,
					Item:       rs.item,
					RowIndex:   rs.dataIdx,
					IsSelected: rs.selected,
					IsHovered:  rs.hovered,
					IsEditing:  false,
					DrawCtx:    ctx,
					TextColor:  s.textColor,
					FontSize:   s.fontSize,
				}
				// PERF-3: текст ячейки считается один раз и кладётся в кэш —
				// без него каждый кадр на каждую видимую ячейку приходился
				// reflect-обход пути + fmt.Sprintf (а у CheckBox — дважды).
				if ctc, ok := col.(cachedTextColumn); ok && ctc.UsesCachedText() {
					cdc.CachedText = dg.cellText(s.gen, row, colIdx, col, rs.item)
					cdc.HasCachedText = true
				}
				col.DrawCell(cdc)
			}

			// Вертикальная линия ячейки
			ctx.DrawVLine(cellX+w-1, rowY, s.rowH, s.gridLine)
			cellX += w
		}

		// Восстанавливаем data-area clip после ячеек строки
		ctx.SetClip(clipRect)

		// Горизонтальная линия строки
		ctx.DrawHLine(dr.Min.X, rowY+s.rowH-1, s.dataW, s.gridLine)
	}

	// Восстанавливаем глобальный клип по bounds
	ctx.SetClip(s.bounds)
}

// drawEditCell рисует ячейку в режиме редактирования.
func (dg *DataGrid) drawEditCell(ctx DrawContextBridge, s *drawSnapshot, r image.Rectangle) {
	// Фон и рамка
	ctx.FillRect(r.Min.X+1, r.Min.Y+1, r.Dx()-2, r.Dy()-2, s.editBG)
	ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), s.editBorder)

	// Текст
	textX := r.Min.X + 6
	textY := r.Min.Y + (r.Dy()-14)/2
	ctx.DrawTextSize(s.editValue, textX, textY, s.fontSize, s.textColor)

	// Каретка
	if s.focused {
		// SEC-5: позицию курсора клэмпим — редактор мог быть сброшен
		// параллельным изменением модели, а срез рун по «старой» позиции
		// уронил бы кадр.
		runes := []rune(s.editValue)
		pos := s.editCursor
		if pos < 0 {
			pos = 0
		}
		if pos > len(runes) {
			pos = len(runes)
		}
		caretX := textX + ctx.MeasureText(string(runes[:pos]), s.fontSize)
		ctx.FillRect(caretX, r.Min.Y+4, 1, r.Dy()-8, s.textColor)
	}
}

// drawScrollbar рисует вертикальный скроллбар.
func (dg *DataGrid) drawScrollbar(ctx DrawContextBridge, s *drawSnapshot) {
	sr := s.scrollbar
	ctx.FillRect(sr.Min.X, sr.Min.Y, sr.Dx(), sr.Dy(), s.scrollTrack)

	tr := s.thumb
	tc := s.scrollThumb
	if s.thumbActive {
		tc = s.scrollThumbHigh
	}
	ctx.FillRect(tr.Min.X+2, tr.Min.Y+1, tr.Dx()-4, tr.Dy()-2, tc)
}

// ─── Mouse Events ──────────────────────────────────────────────────────────

// OnMouseButton обрабатывает нажатие/отпускание кнопки мыши.
// Возвращает true, если событие поглощено.
//
// Без модификаторов: выделение ведёт себя как одиночное даже в режиме
// SelectionExtended. Для Ctrl+Click и Shift+Click — OnMouseButtonMod.
func (dg *DataGrid) OnMouseButton(x, y int, button int, pressed bool) bool {
	return dg.OnMouseButtonMod(x, y, button, pressed, false, false)
}

// OnMouseButtonMod — то же, но с модификаторами клавиатуры.
//
// shift выделяет диапазон от опорной строки, ctrl добавляет и снимает строку
// по одной — ровно то, что selectRow умел с самого начала и чего нельзя было
// попросить снаружи.
func (dg *DataGrid) OnMouseButtonMod(x, y int, button int, pressed bool, shift, ctrl bool) bool {
	// Заголовок разбирается ДО общего замка: OnHeaderClick отвечает
	// «разобрал/не разобрал» здесь и сейчас, отложить его через firePending
	// нельзя — ответ нужен раньше, чем решение о сортировке. А звать чужой
	// колбэк под замком нельзя тем более: обработчик почти наверняка позовёт
	// методы этой же таблицы.
	if button == 0 {
		if pressed {
			if dg.fireHeaderClick(x, y) {
				return true
			}
		} else if handled, onHeader := dg.finishHeaderPress(x, y); onHeader {
			return handled
		}
	}

	defer dg.firePending() // LIFO: выполнится ПОСЛЕ Unlock — колбэки вне dg.mu
	dg.mu.Lock()
	defer dg.mu.Unlock()

	if button != 0 { // только LeftButton
		return false
	}

	if !pressed {
		// Отпускание: завершаем drag операции
		if dg.thumbDragging {
			dg.thumbDragging = false
			dg.markRectDirty(dg.scrollbarRect()) // ползунок гаснет
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
			dg.markRectDirty(dg.scrollbarRect()) // ползунок подсвечивается
			return true
		}
		sr := dg.scrollbarRect()
		if pt.In(sr) {
			ratio := float64(y-sr.Min.Y) / float64(sr.Dy())
			dg.scrollY = int(ratio * float64(dg.contentHeight()))
			dg.clampScrollY()
			dg.markFullDirty() // прыжок скролла — весь вьюпорт
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

	// Заголовок. С включённым перетаскиванием нажатие лишь ЗАПОМИНАЕТСЯ:
	// щелчок это или захват колонки, станет ясно только по движению мыши.
	// Разбор такого нажатия — в finishHeaderPress на отпускании.
	if pt.In(dg.headerRect()) {
		if dg.CanUserReorderColumns && dg.beginHeaderPress(x) {
			return true
		}
		if dg.CanUserSortColumns {
			colIdx := dg.colIndexAtX(x)
			if colIdx >= 0 {
				dg.sortByColumn(colIdx)
				return true
			}
		}
	}

	// Область данных: выделение
	row := dg.rowIndexAtY(y)
	if row >= 0 {
		// Завершаем текущее редактирование
		if dg.isEditing {
			dg.commitEdit()
		}
		dg.selectRow(row, shift, ctrl)
		return true
	}

	return false
}

// fireHeaderClick вызывает OnHeaderClick, если нажатие пришлось на заголовок
// колонки. Сообщает, разобран ли клик.
//
// Кромка resize и ползунок прокрутки проверяются ЗДЕСЬ и отдаются своим
// обработчикам: приложению, которое хочет всего лишь меню по щелчку на
// заголовке, незачем самому отличать «клик» от «потянули за границу» —
// именно на это уходило больше всего чужого кода.
func (dg *DataGrid) fireHeaderClick(x, y int) bool {
	dg.mu.Lock()
	fn := dg.OnHeaderClick
	// С перетаскиванием щелчок разбирается на отпускании (finishHeaderPress):
	// на нажатии ещё неизвестно, щелчок это или захват колонки.
	if fn == nil || dg.CanUserReorderColumns || !image.Pt(x, y).In(dg.headerRect()) {
		dg.mu.Unlock()
		return false
	}
	if dg.needsScrollbar() && image.Pt(x, y).In(dg.scrollbarRect()) {
		dg.mu.Unlock()
		return false
	}
	if dg.CanUserResizeColumns && dg.resizeColumnAt(x) >= 0 {
		dg.mu.Unlock()
		return false
	}
	idx := dg.colIndexAtX(x)
	var col Column
	if idx >= 0 && idx < len(dg.columns) {
		col = dg.columns[idx]
	}
	dg.mu.Unlock()

	return fn(col, idx, x, y)
}

// HoverRowToolTip возвращает текст подсказки для строки под курсором.
//
// Пустая строка — подсказки нет: либо колбэк RowToolTip не задан, либо
// курсор не над строкой, либо приложение вернуло пустой текст.
//
// Колбэк зовётся ВНЕ замка: подсказку спрашивает движок посреди своей работы,
// и обработчик волен обратиться к таблице за данными строки.
func (dg *DataGrid) HoverRowToolTip() string {
	dg.mu.Lock()
	fn := dg.RowToolTip
	row := dg.hoverRow
	var item interface{}
	if fn != nil && row >= 0 && row < len(dg.sortedIdx) && dg.itemsSource != nil {
		item = dg.itemsSource.Get(dg.sortedIdx[row])
	}
	dg.mu.Unlock()

	if fn == nil || row < 0 {
		return ""
	}
	return fn(item, row)
}

// OnMouseDoubleClick обрабатывает двойной клик.
//
// Поведение:
//  1. Если клик попал на валидную строку — вызывается OnRowActivated
//     (даже если грид/колонка read-only). Это даёт UX-крючок на
//     «активацию строки» — стандартное действие WPF DataGrid для
//     read-only сценариев (открыть детали / toggle breakpoint).
//  2. Затем — попытка beginEdit(row, col). beginEdit сам решит,
//     допустимо ли редактирование с учётом IsReadOnly грида и колонки.
//
// Снапшот item для callback снимается ПОД мьютексом, сам callback
// вызывается ПОСЛЕ Unlock (firePending) — обработчик может безопасно дёргать
// SetItemsSource / SelectRow / Refresh, не вызывая deadlock.
func (dg *DataGrid) OnMouseDoubleClick(x, y int) bool {
	defer dg.firePending() // LIFO: выполнится ПОСЛЕ Unlock — колбэки вне dg.mu
	dg.mu.Lock()
	defer dg.mu.Unlock()

	row := dg.rowIndexAtY(y)
	col := dg.colIndexAtX(x)
	if row < 0 {
		return false
	}

	var activatedItem interface{}
	if row < len(dg.sortedIdx) && dg.itemsSource != nil {
		activatedItem = dg.itemsSource.Get(dg.sortedIdx[row])
	}
	cb := dg.OnRowActivated

	if col >= 0 {
		dg.beginEdit(row, col)
	}

	if cb != nil {
		dg.pending = append(dg.pending, func() { cb(row, activatedItem) })
	}
	return true
}

// OnMouseMove обрабатывает перемещение мыши.
func (dg *DataGrid) OnMouseMove(x, y int) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	// Drag скроллбара
	if dg.thumbDragging {
		old := dg.scrollY
		sr := dg.scrollbarRect()
		tr := dg.thumbRect()
		trackUsable := sr.Dy() - tr.Dy()
		if trackUsable > 0 {
			dy := y - dg.thumbDragStartY
			scrollDelta := int(float64(dy) / float64(trackUsable) * float64(dg.maxScrollY()))
			dg.scrollY = dg.thumbDragStartS + scrollDelta
			dg.clampScrollY()
		}
		if dg.scrollY != old {
			dg.markFullDirty()
		}
		return
	}

	// Перетаскивание колонки за заголовок.
	if dg.dragCol >= 0 {
		dg.dragHeaderTo(x)
		return
	}

	// Resize колонки. SEC-5: индекс мог протухнуть после SetColumns —
	// проверяем по фактической длине среза, а не только на >= 0.
	if dg.resizingCol >= len(dg.columns) {
		dg.resizingCol = -1
	}
	if dg.resizingCol >= 0 {
		dx := x - dg.resizingStartX
		newW := dg.resizingStartW + dx
		if newW < minColumnWidth {
			newW = minColumnWidth
		}
		dg.columns[dg.resizingCol].SetActualWidth(newW)
		dg.markFullDirty() // ширина колонки сдвигает весь контент
		return
	}

	oldHover := dg.hoverRow
	oldThumb := dg.thumbHovered

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

	// Точечная инвалидация: подсветка hover меняется только на прежней и новой
	// строках; подсветка ползунка — в области скроллбара.
	if dg.hoverRow != oldHover {
		dg.markRowDirty(oldHover)
		dg.markRowDirty(dg.hoverRow)
	}
	if dg.thumbHovered != oldThumb {
		dg.markRectDirty(dg.scrollbarRect())
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
	defer dg.firePending() // LIFO: выполнится ПОСЛЕ Unlock — колбэки вне dg.mu
	dg.mu.Lock()
	defer dg.mu.Unlock()
	dg.scrollY += delta
	dg.clampScrollY()
}

// ScrollY возвращает текущее вертикальное смещение прокрутки (в пикселях).
func (dg *DataGrid) ScrollY() int {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.scrollY
}

// ScrollX возвращает горизонтальную прокрутку в пикселях.
//
// Нужна снаружи по той же причине, что и ScrollY: посчитать абсолютный X
// ячейки, когда таблица уехала вбок. Без неё формулу было не составить —
// величина есть, а спросить её нечем.
func (dg *DataGrid) ScrollX() int {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.scrollX
}

// HoverRow возвращает индекс строки под курсором или -1.
//
// Нужен подсказке на строку: у Base.ToolTip один текст на весь виджет, а
// строке нужен свой. См. также RowToolTip — с ним обёртка не нужна вовсе.
func (dg *DataGrid) HoverRow() int {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.hoverRow
}

// RowIndexAtY возвращает индекс строки по координате Y (или -1).
//
// Формула не сложная — высота заголовка, RowHeight, прокрутка, — но она
// ЗДЕСЬ, и повторять её снаружи значит копировать то, что может измениться.
func (dg *DataGrid) RowIndexAtY(y int) int {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.rowIndexAtY(y)
}

// WheelScroll прокручивает таблицу колесом мыши на 3 строки за тик
// (up=true — вверх, иначе вниз). Возвращает true, если прокрутка
// фактически сдвинулась — обёртка использует это, чтобы поглощать
// событие ТОЛЬКО когда есть что прокручивать (иначе колесо всплывает
// к родительскому ScrollView).
func (dg *DataGrid) WheelScroll(up bool) bool {
	defer dg.firePending() // LIFO: выполнится ПОСЛЕ Unlock — колбэки вне dg.mu
	dg.mu.Lock()
	defer dg.mu.Unlock()
	if dg.maxScrollY() == 0 {
		return false
	}
	old := dg.scrollY
	step := 3 * dg.RowHeight
	if up {
		dg.scrollY -= step
	} else {
		dg.scrollY += step
	}
	dg.clampScrollY()
	return dg.scrollY != old
}

// ─── Selection helpers ─────────────────────────────────────────────────────

func (dg *DataGrid) selectRow(row int, shift, ctrl bool) {
	// Снимок прежнего выделения и скролла для точечной инвалидации.
	oldRows := make([]int, 0, len(dg.selectedRows))
	for r := range dg.selectedRows {
		oldRows = append(oldRows, r)
	}
	oldScroll := dg.scrollY

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

	// Точечная инвалидация: скролл не сдвинулся → перерисовываем только прежде
	// и ныне выделенные строки; ensureVisible прокрутил → весь вьюпорт.
	if dg.scrollY != oldScroll {
		dg.markFullDirty()
	} else {
		for _, r := range oldRows {
			dg.markRowDirty(r)
		}
		for r := range dg.selectedRows {
			dg.markRowDirty(r)
		}
	}

	// Callback — откладываем до выхода из-под dg.mu (см. firePending).
	if dg.OnSelectionChanged != nil {
		var item interface{}
		if row >= 0 && row < len(dg.sortedIdx) {
			item = dg.itemsSource.Get(dg.sortedIdx[row])
		}
		cb := dg.OnSelectionChanged
		ev := SelectionChangedEvent{SelectedIndex: row, SelectedItem: item}
		dg.pending = append(dg.pending, func() { cb(ev) })
	}
}

// ─── Editing ───────────────────────────────────────────────────────────────

func (dg *DataGrid) beginEdit(row, col int) {
	if col < 0 || col >= len(dg.columns) {
		return
	}
	column := dg.columns[col]

	// WPF-совместимая семантика IsReadOnly:
	//   - per-column IsReadOnly, выставленный ЯВНО, перекрывает grid.IsReadOnly
	//     в обе стороны (column=true → RO даже если grid editable;
	//     column=false → editable даже если grid RO).
	//   - если у колонки IsReadOnly не выставлен явно, она наследует
	//     значение grid.IsReadOnly.
	var effectiveRO bool
	if column.IsReadOnlyExplicit() {
		effectiveRO = column.IsReadOnly()
	} else {
		effectiveRO = dg.IsReadOnly
	}
	if effectiveRO {
		return
	}
	if row < 0 || row >= len(dg.sortedIdx) || dg.itemsSource == nil {
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
	dg.markRowDirty(row) // строка перешла в режим редактирования
}

// commitEdit завершает редактирование ячейки. Вызывать под dg.mu.
//
// SEC-5: editingRow/editingCol проверяются по ФАКТИЧЕСКИМ границам. Фоновый
// RemoveAt/Clear или SetColumns посреди редактирования оставляли здесь
// индексы за границей срезов — грид падал на первом же клике.
//
// SEC-4: OnCellEditEnding и запись значения уходят в pending и выполняются
// уже вне dg.mu — обработчик волен звать SelectedItem()/ScrollBy().
func (dg *DataGrid) commitEdit() {
	if !dg.isEditing {
		return
	}

	row, colIdx := dg.editingRow, dg.editingCol
	value := dg.editingValue

	// Режим редактирования снимаем сразу и синхронно: дальше по стеку
	// (selectRow, обработка клавиш) состояние должно быть уже согласованным.
	dg.isEditing = false
	dg.editingRow, dg.editingCol = -1, -1
	dg.editingValue, dg.editCursorPos = "", 0
	dg.markRowDirty(row)

	if colIdx < 0 || colIdx >= len(dg.columns) ||
		row < 0 || row >= len(dg.sortedIdx) || dg.itemsSource == nil {
		return // редактируемой ячейки больше нет — коммитить некуда
	}

	col := dg.columns[colIdx]
	dataIdx := dg.sortedIdx[row]
	item := dg.itemsSource.Get(dataIdx)
	if item == nil {
		return
	}

	cbEnd := dg.OnCellEditEnding
	cbRow := dg.OnRowEditEnding
	dg.pending = append(dg.pending, func() {
		if cbEnd != nil {
			evt := &CellEditEndingEvent{
				RowIndex: dataIdx,
				Column:   col,
				Item:     item,
				NewValue: value,
			}
			cbEnd(evt)
			if evt.Cancel {
				return
			}
		}
		// Записываем значение в модель
		col.SetCellValue(item, value)
		dg.invalidateCellRow(row)
		if cbRow != nil {
			cbRow(dataIdx, item)
		}
	})
}

// cancelEdit отменяет редактирование без записи в модель. Вызывать под dg.mu.
func (dg *DataGrid) cancelEdit() {
	if !dg.isEditing {
		return
	}
	dg.markRowDirty(dg.editingRow) // ячейка выходит из режима редактирования
	dg.isEditing = false
	dg.editingRow = -1
	dg.editingCol = -1
	dg.editingValue, dg.editCursorPos = "", 0
}

// ─── Keyboard ──────────────────────────────────────────────────────────────

// OnKeyEvent обрабатывает клавиатурный ввод.
func (dg *DataGrid) OnKeyEvent(code int, char rune, pressed bool, shift, ctrl bool) {
	if !pressed {
		return
	}

	defer dg.firePending() // LIFO: выполнится ПОСЛЕ Unlock — колбэки вне dg.mu
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
	case 13: // Enter — активация строки + (если editable) начать редактирование
		if dg.focusRow >= 0 {
			// SEC-4: item снимается под мьютексом, а callback уходит в
			// pending и выполняется уже вне dg.mu (firePending). Прежде он
			// звался прямо здесь, под нерекурсивным sync.Mutex: обработчик,
			// дёрнувший SelectedItem()/ScrollBy(), намертво вешал UI-поток.
			if dg.OnRowActivated != nil {
				var item interface{}
				if dg.focusRow < len(dg.sortedIdx) && dg.itemsSource != nil {
					item = dg.itemsSource.Get(dg.sortedIdx[dg.focusRow])
				}
				cb := dg.OnRowActivated
				row := dg.focusRow
				if dg.focusCol >= 0 {
					dg.beginEdit(dg.focusRow, dg.focusCol)
				}
				dg.pending = append(dg.pending, func() { cb(row, item) })
				return
			}
			if dg.focusCol >= 0 {
				dg.beginEdit(dg.focusRow, dg.focusCol)
			}
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
			dg.markFullDirty() // выделены все строки
		}
	}
}

// handleEditKey обрабатывает ввод в режиме редактирования.
func (dg *DataGrid) handleEditKey(code int, char rune, ctrl bool) {
	// Любое изменение внутри редактируемой ячейки перерисовывает её строку.
	// (commit/cancel уже пометили строку и обнулили editingRow — тогда no-op.)
	defer func() { dg.markRowDirty(dg.editingRow) }()

	// SEC-5: позиция каретки могла остаться от прежнего значения (модель
	// сменилась под редактором) — срез рун по ней паникует.
	if n := len([]rune(dg.editingValue)); dg.editCursorPos > n {
		dg.editCursorPos = n
	} else if dg.editCursorPos < 0 {
		dg.editCursorPos = 0
	}

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
	// Под локом: цвета читаются в слепок кадра (Draw работает вне dg.mu).
	dg.mu.Lock()
	defer dg.mu.Unlock()
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
