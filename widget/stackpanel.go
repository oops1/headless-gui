// Package widget — StackPanel: контейнер с автоматической раскладкой дочерних виджетов.
//
// В отличие от Panel (который не управляет позициями потомков), StackPanel
// расставляет дочерние виджеты последовательно: по горизонтали или по вертикали.
// Аналог WPF StackPanel.
//
// XAML:
//
//	<StackPanel Orientation="Horizontal" Background="#2D2D30" Spacing="4" Padding="5">
//	    <Button Content="OK" Width="80" Height="28"/>
//	    <Button Content="Cancel" Width="80" Height="28"/>
//	</StackPanel>
package widget

import (
	"image"
	"image/color"
)

// Orientation определяет направление раскладки.
type Orientation int

const (
	// OrientationVertical — дочерние виджеты размещаются сверху вниз (по умолчанию).
	OrientationVertical Orientation = iota
	// OrientationHorizontal — дочерние виджеты размещаются слева направо.
	OrientationHorizontal
)

// StackPanel — контейнер с автоматической последовательной раскладкой.
type StackPanel struct {
	Base
	Orientation Orientation
	Background  color.RGBA
	UseAlpha    bool
	Spacing     int // расстояние между дочерними виджетами (px)
	Padding     int // внутренний отступ (px)
}

// NewStackPanel создаёт StackPanel с заданным направлением.
func NewStackPanel(orient Orientation) *StackPanel {
	return &StackPanel{
		Orientation: orient,
		Background:  color.RGBA{},
		Spacing:     0,
		Padding:     0,
	}
}

// SetBounds задаёт bounds и пересчитывает раскладку дочерних виджетов.
func (sp *StackPanel) SetBounds(r image.Rectangle) {
	sp.Base.SetBounds(r)
	sp.layout()
}

// AddChild добавляет виджет и пересчитывает раскладку.
func (sp *StackPanel) AddChild(w Widget) {
	sp.Base.AddChild(w)
	sp.layout()
}

// layout расставляет дочерние виджеты последовательно.
//
// Для каждого ребёнка берётся его текущий Dx/Dy как желаемый размер.
// Если размер равен 0 — используется размер по умолчанию.
// Направление, противоположное Orientation, растягивается до размера панели.
func (sp *StackPanel) layout() {
	b := sp.Bounds()
	if b.Empty() {
		return
	}

	pad := sp.Padding
	offset := pad

	for _, child := range sp.children {
		cb := child.Bounds()
		cw := cb.Dx()
		ch := cb.Dy()

		// Margin — внешний отступ ребёнка (WPF Thickness).
		var m Margin
		type marginGetter interface {
			GetMargin() Margin
		}
		if mg, ok := child.(marginGetter); ok {
			m = mg.GetMargin()
		}

		if sp.Orientation == OrientationHorizontal {
			if cw <= 0 {
				cw = 80 // default width
			}
			if ch <= 0 {
				ch = b.Dy() - 2*pad - m.Top - m.Bottom
			}
			if ch <= 0 {
				ch = 30
			}

			offset += m.Left
			cy := b.Min.Y + (b.Dy()-ch)/2
			child.SetBounds(image.Rect(
				b.Min.X+offset,
				cy,
				b.Min.X+offset+cw,
				cy+ch,
			))
			offset += cw + m.Right + sp.Spacing

		} else { // Vertical
			if ch <= 0 {
				ch = 30 // default height
			}
			if cw <= 0 {
				cw = b.Dx() - 2*pad - m.Left - m.Right
			}
			if cw <= 0 {
				cw = 80
			}

			offset += m.Top
			cx := b.Min.X + pad + m.Left
			child.SetBounds(image.Rect(
				cx,
				b.Min.Y+offset,
				cx+cw,
				b.Min.Y+offset+ch,
			))
			offset += ch + m.Bottom + sp.Spacing
		}
	}
}

// Draw рисует фон и дочерние виджеты.
func (sp *StackPanel) Draw(ctx DrawContext) {
	b := sp.Bounds()

	if sp.Background.A > 0 {
		if sp.UseAlpha && sp.Background.A < 255 {
			if ac, ok := ctx.(DrawContextAlpha); ok {
				ac.FillRectAlpha(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), sp.Background)
			} else {
				ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), sp.Background)
			}
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), sp.Background)
		}
	}

	sp.drawChildren(ctx)
}

// ApplyTheme обновляет цвета StackPanel.
func (sp *StackPanel) ApplyTheme(t *Theme) {
	// StackPanel обычно прозрачный; если задан фон — не меняем
}
