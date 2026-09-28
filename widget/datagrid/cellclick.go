package datagrid

import "image"

// cellclick.go — щелчок по конкретной ячейке.
//
// Таблица сообщала о строке (OnSelectionChanged) и об её активации двойным
// щелчком (OnRowActivated), а про ячейку молчала. Приложению, которое рисует
// в ячейке ссылку — номер коммита, путь, адрес, — поймать одиночный щелчок
// было нечем: разбор координат целиком приватный.
//
// Здесь и событие, и публичный разбор координат: приложение вправе не
// подписываться, а спросить само — например, из своего обработчика мыши.

// CellClickedEvent — щелчок по ячейке области данных.
type CellClickedEvent struct {
	// RowIndex — индекс строки в ТЕКУЩЕМ порядке: тот же, что приходит в
	// OnRowActivated и принимает ItemAtRow. Сам элемент — в Item, так что
	// пересчитывать индекс в исходную коллекцию обычно не нужно.
	RowIndex int
	// ColumnIndex — индекс колонки в текущем порядке колонок.
	ColumnIndex int
	// Column — сама колонка: по ней приложение узнаёт, куда щёлкнули, не
	// пересчитывая индексы после перестановки колонок.
	Column Column
	// Item — элемент модели, стоящий в этой строке.
	Item interface{}
	// X, Y — координаты щелчка ВНУТРИ ячейки, от её левого верхнего угла.
	// Нужны тому, кто рисует в ячейке несколько активных мест.
	X, Y int
	// Button — кнопка мыши: 0 — левая, 1 — правая, 2 — средняя.
	Button int
	// DoubleClick — щелчок был вторым в паре (двойной).
	DoubleClick bool
}

// CellAt возвращает строку и колонку под точкой в области данных.
//
// row — индекс в текущем порядке строк (как у ItemAtRow и OnRowActivated),
// col — в текущем порядке колонок. ok=false — точка вне данных: заголовок,
// полоса прокрутки, пустое место под последней строкой. Потокобезопасно.
func (dg *DataGrid) CellAt(x, y int) (row, col int, ok bool) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	return dg.cellAtLocked(x, y)
}

// cellAtLocked — то же под уже взятым замком.
func (dg *DataGrid) cellAtLocked(x, y int) (row, col int, ok bool) {
	if dg.needsScrollbar() && image.Pt(x, y).In(dg.scrollbarRect()) {
		return -1, -1, false
	}
	r := dg.rowIndexAtY(y)
	c := dg.colIndexAtX(x)
	if r < 0 || c < 0 {
		return -1, -1, false
	}
	return r, c, true
}

// CellRect возвращает прямоугольник ячейки на экране (абсолютные координаты).
//
// row — индекс в текущем порядке строк, как его отдаёт CellAt. Пустой
// прямоугольник — такой ячейки нет. Прямоугольник отдаётся и для строки,
// уехавшей за край видимой области: приложение, которое хочет что-то там
// показать, само решит, нужно ли сперва прокрутить. Потокобезопасно.
func (dg *DataGrid) CellRect(row, col int) image.Rectangle {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	if col < 0 || col >= len(dg.columns) || row < 0 || row >= dg.rowCount() {
		return image.Rectangle{}
	}
	dr := dg.dataRect()
	y := dr.Min.Y + row*dg.RowHeight - dg.scrollY
	x := dg.colLeftX(col)
	return image.Rect(x, y, x+dg.columns[col].ActualWidth(), y+dg.RowHeight)
}

// queueCellClickedLocked откладывает OnCellClicked (колбэки выполняются вне
// dg.mu — см. firePending).
func (dg *DataGrid) queueCellClickedLocked(x, y, button int, double bool) {
	cb := dg.OnCellClicked
	if cb == nil {
		return
	}
	row, col, ok := dg.cellAtLocked(x, y)
	if !ok {
		return
	}
	var item interface{}
	if row < len(dg.sortedIdx) && dg.itemsSource != nil {
		item = dg.itemsSource.Get(dg.sortedIdx[row])
	}
	ev := CellClickedEvent{
		RowIndex:    row,
		ColumnIndex: col,
		Column:      dg.columns[col],
		Item:        item,
		X:           x - dg.colLeftX(col),
		Y:           (y - dg.dataRect().Min.Y + dg.scrollY) % dg.RowHeight,
		Button:      button,
		DoubleClick: double,
	}
	dg.pending = append(dg.pending, func() { cb(ev) })
}

// CursorAt — форма курсора над точкой: значения widget.Cursor.
//
// Ссылку в ячейке принято показывать рукой, иначе о том, что по ней можно
// щёлкнуть, никто не догадается. Пакет datagrid не знает про widget (тот
// импортирует его, не наоборот), поэтому курсор здесь — число; обёртка
// DataGridWidget переводит его в widget.Cursor. Потокобезопасно.
func (dg *DataGrid) CursorAt(x, y int) int {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	if dg.CanUserResizeColumns && image.Pt(x, y).In(dg.headerRect()) &&
		dg.resizeColumnAt(x) >= 0 {
		return CursorSizeWE
	}
	_, col, ok := dg.cellAtLocked(x, y)
	if !ok {
		return CursorArrow
	}
	if c, has := dg.columns[col].(interface{ Cursor() int }); has {
		return c.Cursor()
	}
	return CursorArrow
}

// Формы курсора, которые таблица просит у хоста. Значения совпадают с
// widget.Cursor: пакет widget импортирует этот, и обратной ссылки быть не
// может, а держать два несовпадающих перечисления — верный способ однажды
// показать над ссылкой курсор изменения размера.
const (
	CursorArrow  = 0
	CursorIBeam  = 1
	CursorHand   = 2
	CursorSizeWE = 3
)
