// Package widget — UniformGrid: равномерная сетка (аналог WPF UniformGrid).
//
// Размещает дочерние виджеты в ячейки одинакового размера. Число столбцов и
// строк задаётся Columns/Rows; если не заданы — вычисляются автоматически
// (квадратная сетка по числу элементов).
package widget

import (
	"image"
	"image/color"
	"math"
)

// UniformGrid — равномерная сетка одинаковых ячеек.
type UniformGrid struct {
	Base
	Rows       int // 0 = авто
	Columns    int // 0 = авто
	Background color.RGBA
	UseAlpha   bool
	Spacing    int // зазор между ячейками (px)
}

// NewUniformGrid создаёт UniformGrid.
func NewUniformGrid() *UniformGrid {
	return &UniformGrid{UseAlpha: true}
}

// SetBounds задаёт bounds и пересчитывает раскладку.
func (ug *UniformGrid) SetBounds(r image.Rectangle) {
	ug.Base.SetBounds(r)
	ug.layout()
}

// AddChild добавляет виджет и пересчитывает раскладку.
func (ug *UniformGrid) AddChild(w Widget) {
	ug.Base.AddChild(w)
	ug.layout()
}

// dims вычисляет (rows, cols) с учётом авто-режима.
func (ug *UniformGrid) dims() (int, int) {
	n := len(ug.children)
	rows, cols := ug.Rows, ug.Columns
	switch {
	case cols > 0 && rows > 0:
		// заданы оба
	case cols > 0:
		rows = (n + cols - 1) / cols
	case rows > 0:
		cols = (n + rows - 1) / rows
	default:
		if n == 0 {
			return 1, 1
		}
		cols = int(math.Ceil(math.Sqrt(float64(n))))
		rows = (n + cols - 1) / cols
	}
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	return rows, cols
}

func (ug *UniformGrid) layout() {
	b := ug.Bounds()
	if b.Empty() || len(ug.children) == 0 {
		return
	}
	rows, cols := ug.dims()
	sp := ug.Spacing
	cellW := (b.Dx() - sp*(cols-1)) / cols
	cellH := (b.Dy() - sp*(rows-1)) / rows
	if cellW < 0 {
		cellW = 0
	}
	if cellH < 0 {
		cellH = 0
	}
	for i, child := range ug.children {
		r := i / cols
		c := i % cols
		x := b.Min.X + c*(cellW+sp)
		y := b.Min.Y + r*(cellH+sp)
		child.SetBounds(image.Rect(x, y, x+cellW, y+cellH))
	}
}

// Draw рисует фон и дочерние виджеты.
func (ug *UniformGrid) Draw(ctx DrawContext) {
	b := ug.Bounds()
	if b.Empty() {
		return
	}
	if ug.Background.A > 0 {
		if ug.UseAlpha && ug.Background.A < 255 {
			if ac, ok := ctx.(DrawContextAlpha); ok {
				ac.FillRectAlpha(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), ug.Background)
			} else {
				ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), ug.Background)
			}
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), ug.Background)
		}
	}
	ug.drawChildren(ctx)
}

// ApplyTheme — UniformGrid обычно прозрачный.
func (ug *UniformGrid) ApplyTheme(t *Theme) {}
