// Package widget — WrapPanel: контейнер с переносом (аналог WPF WrapPanel).
//
// Размещает дочерние виджеты последовательно по строкам (Horizontal) или
// столбцам (Vertical), перенося на новую линию при выходе за границу.
package widget

import (
	"image"
	"image/color"
)

// WrapPanel — контейнер с авто-переносом дочерних виджетов.
type WrapPanel struct {
	Base
	Orientation Orientation // Horizontal (default) | Vertical
	Background  color.RGBA
	UseAlpha    bool
	Spacing     int // зазор между элементами в линии (px)
	LineSpacing int // зазор между линиями (px)
	Padding     int
}

// NewWrapPanel создаёт WrapPanel.
func NewWrapPanel(orient Orientation) *WrapPanel {
	return &WrapPanel{Orientation: orient, UseAlpha: true}
}

// SetBounds задаёт bounds и пересчитывает раскладку.
func (wp *WrapPanel) SetBounds(r image.Rectangle) {
	wp.Base.SetBounds(r)
	wp.layout()
}

// AddChild добавляет виджет и пересчитывает раскладку.
func (wp *WrapPanel) AddChild(w Widget) {
	wp.Base.AddChild(w)
	wp.layout()
}

func (wp *WrapPanel) layout() {
	b := wp.Bounds()
	if b.Empty() {
		return
	}
	pad := wp.Padding
	if wp.Orientation == OrientationVertical {
		wp.layoutVertical(b, pad)
		return
	}
	// Horizontal: строки слева направо, перенос вниз.
	x := pad
	y := pad
	lineH := 0
	maxW := b.Dx() - pad
	for _, child := range wp.children {
		cb := child.Bounds()
		cw, ch := cb.Dx(), cb.Dy()
		if cw <= 0 {
			cw = desiredWidth(child)
		}
		if ch <= 0 {
			ch = desiredHeight(child)
		}
		if x > pad && x+cw > maxW {
			// перенос на новую строку
			x = pad
			y += lineH + wp.LineSpacing
			lineH = 0
		}
		child.SetBounds(image.Rect(b.Min.X+x, b.Min.Y+y, b.Min.X+x+cw, b.Min.Y+y+ch))
		x += cw + wp.Spacing
		if ch > lineH {
			lineH = ch
		}
	}
}

func (wp *WrapPanel) layoutVertical(b image.Rectangle, pad int) {
	x := pad
	y := pad
	colW := 0
	maxH := b.Dy() - pad
	for _, child := range wp.children {
		cb := child.Bounds()
		cw, ch := cb.Dx(), cb.Dy()
		if cw <= 0 {
			cw = desiredWidth(child)
		}
		if ch <= 0 {
			ch = desiredHeight(child)
		}
		if y > pad && y+ch > maxH {
			y = pad
			x += colW + wp.Spacing
			colW = 0
		}
		child.SetBounds(image.Rect(b.Min.X+x, b.Min.Y+y, b.Min.X+x+cw, b.Min.Y+y+ch))
		y += ch + wp.LineSpacing
		if cw > colW {
			colW = cw
		}
	}
}

// Draw рисует фон и дочерние виджеты.
func (wp *WrapPanel) Draw(ctx DrawContext) {
	b := wp.Bounds()
	if b.Empty() {
		return
	}
	if wp.Background.A > 0 {
		if wp.UseAlpha && wp.Background.A < 255 {
			if ac, ok := ctx.(DrawContextAlpha); ok {
				ac.FillRectAlpha(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), wp.Background)
			} else {
				ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), wp.Background)
			}
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), wp.Background)
		}
	}
	wp.drawChildren(ctx)
}

// ApplyTheme — WrapPanel обычно прозрачный.
func (wp *WrapPanel) ApplyTheme(t *Theme) {}
