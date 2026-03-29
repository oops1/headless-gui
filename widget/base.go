package widget

import "image"

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

// drawChildren рендерит всех потомков в тот же контекст.
// Вызывается конкретными виджетами в конце своего Draw.
func (b *Base) drawChildren(ctx DrawContext) {
	for _, child := range b.children {
		child.Draw(ctx)
	}
}
