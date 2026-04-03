package widget

import "image"

// DockSide определяет сторону прикрепления в DockPanel (WPF DockPanel.Dock).
type DockSide int

const (
	DockTop    DockSide = iota // по умолчанию
	DockBottom
	DockLeft
	DockRight
	DockFill // последний элемент заполняет оставшееся пространство
)

// Margin — отступы виджета (WPF Thickness: Left, Top, Right, Bottom).
type Margin struct {
	Left, Top, Right, Bottom int
}

// Base — общие поля и тривиальные реализации интерфейса Widget.
// Встраивается во все конкретные виджеты.
type Base struct {
	bounds   image.Rectangle
	children []Widget

	// Grid layout (attached properties, как в WPF).
	GridRow     int // Grid.Row     (0-based)
	GridColumn  int // Grid.Column  (0-based)
	GridRowSpan int // Grid.RowSpan (по умолчанию 1)
	GridColSpan int // Grid.ColumnSpan (по умолчанию 1)

	// DockPanel attached property.
	Dock DockSide

	// Margin — внешние отступы (WPF Margin).
	WidgetMargin Margin
}

func (b *Base) Bounds() image.Rectangle     { return b.bounds }
func (b *Base) SetBounds(r image.Rectangle) { b.bounds = r }
func (b *Base) Children() []Widget          { return b.children }
func (b *Base) AddChild(w Widget)           { b.children = append(b.children, w) }

// ── Grid attached properties ────────────────────────────────────────────────

func (b *Base) SetGridProps(row, col, rowSpan, colSpan int) {
	b.GridRow = row
	b.GridColumn = col
	b.GridRowSpan = rowSpan
	b.GridColSpan = colSpan
}

func (b *Base) GetGridRow() int     { return b.GridRow }
func (b *Base) GetGridColumn() int  { return b.GridColumn }
func (b *Base) GetGridRowSpan() int {
	if b.GridRowSpan < 1 {
		return 1
	}
	return b.GridRowSpan
}
func (b *Base) GetGridColSpan() int {
	if b.GridColSpan < 1 {
		return 1
	}
	return b.GridColSpan
}

// ── DockPanel attached property ─────────────────────────────────────────────

func (b *Base) GetDock() DockSide    { return b.Dock }
func (b *Base) SetDock(d DockSide)   { b.Dock = d }

// ── Margin ──────────────────────────────────────────────────────────────────

func (b *Base) GetMargin() Margin      { return b.WidgetMargin }
func (b *Base) SetMargin(m Margin)     { b.WidgetMargin = m }

// drawChildren рендерит всех потомков в тот же контекст.
// Вызывается конкретными виджетами в конце своего Draw.
func (b *Base) drawChildren(ctx DrawContext) {
	for _, child := range b.children {
		child.Draw(ctx)
	}
}
