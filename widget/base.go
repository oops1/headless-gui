package widget

import "image"

// Base — общие поля и тривиальные реализации интерфейса Widget.
// Встраивается во все конкретные виджеты.
type Base struct {
	bounds   image.Rectangle
	children []Widget
}

func (b *Base) Bounds() image.Rectangle     { return b.bounds }
func (b *Base) SetBounds(r image.Rectangle) { b.bounds = r }
func (b *Base) Children() []Widget          { return b.children }
func (b *Base) AddChild(w Widget)           { b.children = append(b.children, w) }

// drawChildren рендерит всех потомков в тот же контекст.
// Вызывается конкретными виджетами в конце своего Draw.
func (b *Base) drawChildren(ctx DrawContext) {
	for _, child := range b.children {
		child.Draw(ctx)
	}
}
