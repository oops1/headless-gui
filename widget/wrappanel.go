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

	// auto — размер, выставленный ребёнку по содержимому на прошлой
	// раскладке (см. measure).
	auto map[Widget]image.Point
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

// Relayout пересчитывает раскладку панели — когда содержимое ребёнка
// сменилось само по себе (см. DockPanel.Relayout).
func (wp *WrapPanel) Relayout() {
	wp.layout()
	wp.Invalidate()
}

// measure возвращает размер ребёнка в раскладке. Размер, выставленный
// панелью по содержимому на прошлой раскладке, перемеряется: подпись могла
// смениться (GG-74). Заданный извне — остаётся.
func (wp *WrapPanel) measure(child Widget, prev map[Widget]image.Point) (int, int) {
	cb := child.Bounds()
	cw, ch := cb.Dx(), cb.Dy()
	p := prev[child]
	var auto image.Point
	if cw <= 0 || p.X == cw {
		cw = desiredWidth(child)
		auto.X = cw
	}
	if ch <= 0 || p.Y == ch {
		ch = desiredHeight(child)
		auto.Y = ch
	}
	if auto != (image.Point{}) {
		if wp.auto == nil {
			wp.auto = make(map[Widget]image.Point)
		}
		wp.auto[child] = auto
	}
	return cw, ch
}

func (wp *WrapPanel) layout() {
	b := wp.Bounds()
	if b.Empty() {
		return
	}
	prev := wp.auto
	wp.auto = nil
	pad := wp.Padding
	if wp.Orientation == OrientationVertical {
		wp.layoutVertical(b, pad, prev)
		return
	}
	// Horizontal: строки слева направо, перенос вниз.
	x := pad
	y := pad
	lineH := 0
	maxW := b.Dx() - pad
	for _, child := range wp.children {
		cw, ch := wp.measure(child, prev)
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

func (wp *WrapPanel) layoutVertical(b image.Rectangle, pad int, prev map[Widget]image.Point) {
	x := pad
	y := pad
	colW := 0
	maxH := b.Dy() - pad
	for _, child := range wp.children {
		cw, ch := wp.measure(child, prev)
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
