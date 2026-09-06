// Package datagrid — определения колонок DataGrid.
//
// Реализованы три WPF-совместимых типа:
//   - DataGridTextColumn     — текстовая ячейка
//   - DataGridCheckBoxColumn — ячейка с чекбоксом
//   - DataGridTemplateColumn — ячейка с пользовательским отрисовщиком
package datagrid

import (
	"image"
	"image/color"
)

// ─── ColumnWidthMode ───────────────────────────────────────────────────────

// ColumnWidthMode определяет режим ширины колонки.
type ColumnWidthMode int

const (
	// ColumnWidthPixel — фиксированная ширина в пикселях.
	ColumnWidthPixel ColumnWidthMode = iota
	// ColumnWidthAuto — ширина по содержимому.
	ColumnWidthAuto
	// ColumnWidthStar — пропорциональная ширина (заполняет оставшееся пространство).
	ColumnWidthStar
)

// ─── ColumnWidth ───────────────────────────────────────────────────────────

// ColumnWidth описывает ширину колонки (аналог WPF DataGridLength).
type ColumnWidth struct {
	Mode  ColumnWidthMode
	Value float64 // Pixel: пиксели; Star: пропорция; Auto: не используется
}

// PixelWidth создаёт фиксированную ширину.
func PixelWidth(px float64) ColumnWidth {
	return ColumnWidth{Mode: ColumnWidthPixel, Value: px}
}

// StarWidth создаёт пропорциональную ширину.
func StarWidth(factor float64) ColumnWidth {
	return ColumnWidth{Mode: ColumnWidthStar, Value: factor}
}

// AutoWidth создаёт авто-ширину.
func AutoWidth() ColumnWidth {
	return ColumnWidth{Mode: ColumnWidthAuto}
}

// ─── SortDirection ─────────────────────────────────────────────────────────

// SortDirection — направление сортировки.
type SortDirection int

const (
	SortNone       SortDirection = iota
	SortAscending                // ▲
	SortDescending               // ▼
)

// ─── CellDrawContext ───────────────────────────────────────────────────────

// CellDrawContext предоставляется колонке для отрисовки ячейки.
type CellDrawContext struct {
	// Rect — прямоугольник ячейки (абсолютные координаты).
	Rect image.Rectangle
	// Item — объект строки (модель данных).
	Item interface{}
	// RowIndex — индекс строки в данных.
	RowIndex int
	// IsSelected — строка выделена.
	IsSelected bool
	// IsHovered — курсор над строкой.
	IsHovered bool
	// IsEditing — ячейка в режиме редактирования.
	IsEditing bool
	// DrawCtx — контекст рисования движка.
	DrawCtx DrawContextBridge
	// TextColor — цвет текста из темы.
	TextColor color.RGBA
	// FontSize — размер шрифта.
	FontSize float64

	// CachedText — заранее вычисленное значение ячейки (GetCellValue),
	// действительно только при HasCachedText. DataGrid считает его один раз
	// и держит в кэше видимого окна (PERF-3): без этого каждая ячейка на
	// каждом кадре повторяла reflect-обход пути привязки и fmt.Sprintf,
	// а чекбокс-колонка делала это дважды.
	CachedText string
	// HasCachedText сообщает, что CachedText заполнен.
	HasCachedText bool
}

// cachedTextColumn — колонка, для которой DataGrid готовит CachedText.
// Реализуется через ColumnBase.UsesCachedText: текст готовится только для
// колонок с привязкой, шаблонные колонки рисуют себя сами.
type cachedTextColumn interface {
	UsesCachedText() bool
}

// cellValue возвращает текст ячейки, предпочитая заранее вычисленный.
func (cdc *CellDrawContext) cellValue(c Column) string {
	if cdc.HasCachedText {
		return cdc.CachedText
	}
	return c.GetCellValue(cdc.Item)
}

// ─── DrawContextBridge ─────────────────────────────────────────────────────

// DrawContextBridge — минимальный интерфейс для рисования, не импортирующий widget.
// Реализуется адаптером, оборачивающим widget.DrawContext.
type DrawContextBridge interface {
	FillRect(x, y, w, h int, col color.RGBA)
	FillRectAlpha(x, y, w, h int, col color.RGBA)
	DrawBorder(x, y, w, h int, col color.RGBA)
	DrawText(text string, x, y int, col color.RGBA)
	DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA)
	MeasureText(text string, sizePt float64) int
	SetClip(r image.Rectangle)
	ClearClip()
	DrawHLine(x, y, length int, col color.RGBA)
	DrawVLine(x, y, length int, col color.RGBA)

	// DrawImage и DrawImageScaled — растровые картинки: значок статуса,
	// миниатюра, флаг страны.
	//
	// Раньше их здесь не было, и шаблонная колонка могла рисовать только
	// примитивами. Значок в ячейке приходилось складывать в список и рисовать
	// вторым проходом уже своим контекстом — приём рабочий, но держащийся на
	// том, что Draw колонки и Draw обёртки идут в одном кадре подряд.
	DrawImage(src image.Image, x, y int)
	DrawImageScaled(src image.Image, x, y, w, h int)

	// FillEllipseAA и FillRoundRect — фигуры, которыми движок рисует сам
	// (кружок радиокнопки, значок диалога, подсветка строки).
	//
	// Без них цветной кружок статуса в ячейке приходилось заранее
	// растеризовать из SVG в картинку каждого цвета — то есть держать набор
	// картинок ради фигуры, которую движок умеет рисовать одной строкой.
	FillEllipseAA(cx, cy, rx, ry int, col color.RGBA)
	FillRoundRect(x, y, w, h, r int, col color.RGBA)
}

// ─── Column интерфейс ──────────────────────────────────────────────────────

// Column — интерфейс колонки DataGrid.
type Column interface {
	// Header возвращает текст заголовка.
	Header() string
	// SetHeader устанавливает текст заголовка.
	SetHeader(header string)

	// Width возвращает ширину колонки.
	Width() ColumnWidth
	// SetWidth устанавливает ширину.
	SetWidth(w ColumnWidth)

	// ActualWidth возвращает вычисленную ширину в пикселях (после layout).
	ActualWidth() int
	// SetActualWidth задаёт вычисленную ширину.
	SetActualWidth(px int)

	// MinWidth/MaxWidth — ограничения ширины.
	MinWidth() int
	MaxWidth() int

	// IsReadOnly возвращает текущее значение признака read-only колонки.
	// Если IsReadOnlyExplicit() == false, это значение не должно
	// учитываться отдельно — колонка наследует DataGrid.IsReadOnly.
	IsReadOnly() bool

	// IsReadOnlyExplicit возвращает true, если IsReadOnly был выставлен
	// на колонке ЯВНО. В этом случае значение IsReadOnly() перекрывает
	// DataGrid.IsReadOnly в обе стороны (WPF-совместимая семантика).
	IsReadOnlyExplicit() bool

	// SortMemberPath возвращает путь к свойству для сортировки.
	SortMemberPath() string

	// Binding возвращает привязку данных колонки.
	GetBinding() *Binding

	// GetCellValue возвращает строковое значение ячейки для item.
	GetCellValue(item interface{}) string

	// SetCellValue записывает значение в модель.
	SetCellValue(item interface{}, value string) bool

	// DrawCell рисует ячейку.
	DrawCell(cdc CellDrawContext)

	// SortDirection/SetSortDirection — текущее направление сортировки.
	GetSortDirection() SortDirection
	SetSortDirection(dir SortDirection)
}

// ─── ColumnBase ────────────────────────────────────────────────────────────

// ColumnBase — общая реализация базовых свойств колонки.
//
// Поле readOnly реализует tri-state с readOnlyExplicit:
//
//	readOnlyExplicit=false → колонка наследует IsReadOnly грида
//	readOnlyExplicit=true,  readOnly=true  → жёстко RO (даже при grid.IsReadOnly=false)
//	readOnlyExplicit=true,  readOnly=false → жёстко editable (даже при grid.IsReadOnly=true)
//
// Это совпадает с поведением WPF DataGrid: per-column IsReadOnly
// перекрывает DataGrid.IsReadOnly в обе стороны.
type ColumnBase struct {
	header           string
	width            ColumnWidth
	actualWidth      int
	minWidth         int
	maxWidth         int
	readOnly         bool
	readOnlyExplicit bool // true — IsReadOnly выставлено явно (см. SetReadOnly / SetReadOnlyExplicit)
	sortPath         string
	binding          *Binding
	sortDirection    SortDirection
}

func (c *ColumnBase) Header() string                   { return c.header }
func (c *ColumnBase) SetHeader(h string)               { c.header = h }
func (c *ColumnBase) Width() ColumnWidth               { return c.width }
func (c *ColumnBase) SetWidth(w ColumnWidth)           { c.width = w }
func (c *ColumnBase) ActualWidth() int                 { return c.actualWidth }
func (c *ColumnBase) SetActualWidth(px int)            { c.actualWidth = px }
func (c *ColumnBase) MinWidth() int                    { return c.minWidth }
func (c *ColumnBase) MaxWidth() int                    { return c.maxWidth }
func (c *ColumnBase) IsReadOnly() bool                 { return c.readOnly }
func (c *ColumnBase) SortMemberPath() string           { return c.sortPath }
func (c *ColumnBase) GetBinding() *Binding             { return c.binding }
func (c *ColumnBase) GetSortDirection() SortDirection  { return c.sortDirection }
func (c *ColumnBase) SetSortDirection(d SortDirection) { c.sortDirection = d }

// SetReadOnly выставляет признак read-only ЯВНО.
// После вызова значение перекрывает DataGrid.IsReadOnly в обе стороны
// (WPF-семантика). Если нужно сбросить колонку обратно в режим
// «наследую от грида» — используйте ResetReadOnly.
func (c *ColumnBase) SetReadOnly(v bool) {
	c.readOnly = v
	c.readOnlyExplicit = true
}

// ResetReadOnly сбрасывает явную установку IsReadOnly для колонки —
// после этого колонка снова наследует значение DataGrid.IsReadOnly.
func (c *ColumnBase) ResetReadOnly() {
	c.readOnly = false
	c.readOnlyExplicit = false
}

// IsReadOnlyExplicit сообщает, было ли IsReadOnly выставлено
// на колонке явно (через SetReadOnly). DataGrid.beginEdit использует
// это для решения «брать значение колонки или наследовать от грида».
func (c *ColumnBase) IsReadOnlyExplicit() bool { return c.readOnlyExplicit }

// UsesCachedText сообщает DataGrid, что колонке нужен текст ячейки
// (GetCellValue) — значит, его стоит посчитать один раз и закэшировать.
// Колонка без привязки рисует себя сама, считать нечего.
func (c *ColumnBase) UsesCachedText() bool { return c.binding != nil }

func (c *ColumnBase) SetSortPath(path string)          { c.sortPath = path }
func (c *ColumnBase) SetMinWidth(px int)               { c.minWidth = px }
func (c *ColumnBase) SetMaxWidth(px int)               { c.maxWidth = px }

func (c *ColumnBase) GetCellValue(item interface{}) string {
	return ResolveBinding(c.binding, item)
}

func (c *ColumnBase) SetCellValue(item interface{}, value string) bool {
	if c.binding == nil || c.readOnly {
		return false
	}
	return SetPropertyValue(item, c.binding.Path, value)
}

// ─── DataGridTextColumn ────────────────────────────────────────────────────

// DataGridTextColumn — текстовая колонка (WPF DataGridTextColumn).
type DataGridTextColumn struct {
	ColumnBase
}

// NewTextColumn создаёт текстовую колонку.
func NewTextColumn(header string, bindingPath string) *DataGridTextColumn {
	col := &DataGridTextColumn{}
	col.header = header
	col.width = StarWidth(1)
	col.binding = &Binding{Path: bindingPath, Mode: TwoWay}
	col.sortPath = bindingPath
	return col
}

// DrawCell рисует текстовую ячейку.
func (c *DataGridTextColumn) DrawCell(cdc CellDrawContext) {
	r := cdc.Rect
	text := cdc.cellValue(c)
	if text == "" {
		return
	}

	// Клиппинг по ячейке устанавливается вызывающей стороной (drawRows), но
	// одного клиппинга мало: обрезанный им текст обрывается посреди буквы, и
	// отличить короткое значение от урезанного нельзя. Многоточие об этом
	// сообщает.
	textX := r.Min.X + 6
	textY := r.Min.Y + (r.Dy()-14)/2
	text = EllipsizeText(cdc.DrawCtx, text, r.Dx()-12, cdc.FontSize)
	cdc.DrawCtx.DrawTextSize(text, textX, textY, cdc.FontSize, cdc.TextColor)
}

// ─── DataGridCheckBoxColumn ────────────────────────────────────────────────

// DataGridCheckBoxColumn — колонка с чекбоксом (WPF DataGridCheckBoxColumn).
type DataGridCheckBoxColumn struct {
	ColumnBase
}

// NewCheckBoxColumn создаёт колонку с чекбоксом.
func NewCheckBoxColumn(header string, bindingPath string) *DataGridCheckBoxColumn {
	col := &DataGridCheckBoxColumn{}
	col.header = header
	col.width = PixelWidth(60)
	col.binding = &Binding{Path: bindingPath, Mode: TwoWay}
	col.sortPath = bindingPath
	return col
}

// GetCellValue возвращает "true" или "false".
func (c *DataGridCheckBoxColumn) GetCellValue(item interface{}) string {
	if c.binding == nil {
		return "false"
	}
	val, ok := GetPropertyValue(item, c.binding.Path)
	if !ok {
		return "false"
	}
	if b, ok := val.(bool); ok && b {
		return "true"
	}
	return "false"
}

// SetCellValue переключает bool в модели.
func (c *DataGridCheckBoxColumn) SetCellValue(item interface{}, value string) bool {
	if c.readOnly || c.binding == nil {
		return false
	}
	return SetPropertyValue(item, c.binding.Path, value == "true")
}

// DrawCell рисует чекбокс.
func (c *DataGridCheckBoxColumn) DrawCell(cdc CellDrawContext) {
	r := cdc.Rect

	// Центрируем чекбокс (16x16)
	boxSize := 16
	cx := r.Min.X + (r.Dx()-boxSize)/2
	cy := r.Min.Y + (r.Dy()-boxSize)/2

	// Фон чекбокса
	bgColor := color.RGBA{R: 50, G: 50, B: 50, A: 255}
	cdc.DrawCtx.FillRect(cx, cy, boxSize, boxSize, bgColor)
	cdc.DrawCtx.DrawBorder(cx, cy, boxSize, boxSize, color.RGBA{R: 100, G: 100, B: 100, A: 255})

	// Галочка. PERF-3: значение берём из подготовленного текста — раньше
	// GetCellValue вызывался здесь ВТОРОЙ раз за ту же ячейку.
	checked := cdc.cellValue(c) == "true"
	if checked {
		// Рисуем ✓ — две линии
		checkColor := color.RGBA{R: 0, G: 200, B: 100, A: 255}
		// Простая V-образная галочка
		for i := 0; i < 4; i++ {
			cdc.DrawCtx.FillRect(cx+3+i, cy+8+i, 1, 1, checkColor)
		}
		for i := 0; i < 6; i++ {
			cdc.DrawCtx.FillRect(cx+7+i, cy+11-i, 1, 1, checkColor)
		}
	}
}

// ─── DataGridTemplateColumn ────────────────────────────────────────────────

// CellRenderer — пользовательская функция отрисовки ячейки.
type CellRenderer func(cdc CellDrawContext)

// DataGridTemplateColumn — колонка с пользовательским шаблоном (WPF DataGridTemplateColumn).
type DataGridTemplateColumn struct {
	ColumnBase
	CellTemplate CellRenderer
}

// NewTemplateColumn создаёт шаблонную колонку.
func NewTemplateColumn(header string, renderer CellRenderer) *DataGridTemplateColumn {
	col := &DataGridTemplateColumn{
		CellTemplate: renderer,
	}
	col.header = header
	col.width = StarWidth(1)
	return col
}

// UsesCachedText — шаблонная колонка рисует ячейку сама, готовить для неё
// текст незачем даже при заданной привязке.
func (c *DataGridTemplateColumn) UsesCachedText() bool { return false }

// DrawCell вызывает пользовательский шаблон.
func (c *DataGridTemplateColumn) DrawCell(cdc CellDrawContext) {
	if c.CellTemplate != nil {
		c.CellTemplate(cdc)
	}
}
