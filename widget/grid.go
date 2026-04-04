// Package widget — Grid layout container (аналог WPF Grid).
//
// Grid размещает дочерние виджеты по ячейкам таблицы (строки × столбцы).
// Позиция каждого потомка задаётся через attached-свойства:
//
//	Grid.Row, Grid.Column       — ячейка (0-based)
//	Grid.RowSpan, Grid.ColumnSpan — объединение ячеек (по умолчанию 1)
//
// Размеры строк и столбцов задаются через RowDefinitions / ColumnDefinitions.
// Поддерживаемые режимы (как в WPF):
//
//	Pixel  — фиксированный размер в пикселях (напр. "100")
//	Star   — пропорциональный размер (напр. "*", "2*")
//	Auto   — размер по содержимому (минимальный)
//
// Если RowDefinitions/ColumnDefinitions пусты — одна строка/столбец на весь Grid.
package widget

import (
	"image"
	"image/color"
)

// GridSizeMode определяет тип размера строки/столбца.
type GridSizeMode int

const (
	GridSizePixel GridSizeMode = iota // фиксированные пиксели
	GridSizeStar                      // пропорциональный (Star)
	GridSizeAuto                      // по содержимому
)

// GridDefinition описывает одну строку или столбец.
type GridDefinition struct {
	Mode  GridSizeMode
	Value float64 // пиксели для Pixel, коэффициент для Star (1.0 = "*", 2.0 = "2*")
	Min   int     // MinHeight / MinWidth (0 = не ограничено)
	Max   int     // MaxHeight / MaxWidth (0 = не ограничено)
}

// Grid — контейнер с табличной раскладкой (аналог WPF Grid).
type Grid struct {
	Base
	Background color.RGBA
	UseAlpha   bool

	ShowGridLines bool       // отладочная отрисовка линий сетки
	GridLineColor color.RGBA // цвет линий (по умолчанию серый)

	RowDefs []GridDefinition
	ColDefs []GridDefinition

	// Кэш рассчитанных позиций (обновляется при SetBounds / layout).
	rowOffsets []int // len = rows+1, rowOffsets[0]=0, rowOffsets[rows]=totalH
	colOffsets []int // len = cols+1
}

// NewGrid создаёт пустой Grid.
func NewGrid() *Grid {
	return &Grid{
		GridLineColor: color.RGBA{R: 100, G: 100, B: 100, A: 80},
	}
}

// Rows возвращает количество строк (минимум 1).
func (g *Grid) Rows() int {
	if len(g.RowDefs) == 0 {
		return 1
	}
	return len(g.RowDefs)
}

// Cols возвращает количество столбцов (минимум 1).
func (g *Grid) Cols() int {
	if len(g.ColDefs) == 0 {
		return 1
	}
	return len(g.ColDefs)
}

// SetBounds задаёт bounds и пересчитывает layout.
func (g *Grid) SetBounds(r image.Rectangle) {
	g.bounds = r
	g.layout()
}

// layout пересчитывает позиции строк и столбцов.
func (g *Grid) layout() {
	b := g.bounds
	// Для Auto строк/столбцов: измеряем содержимое дочерних виджетов.
	g.measureAutoSizes()
	g.rowOffsets = resolveDefinitions(g.RowDefs, b.Dy())
	g.colOffsets = resolveDefinitions(g.ColDefs, b.Dx())

	// Расставляем дочерние виджеты по ячейкам.
	for _, child := range g.children {
		row, col, rowSpan, colSpan := g.childCell(child)
		rows := g.Rows()
		cols := g.Cols()

		// Clamp
		if row >= rows {
			row = rows - 1
		}
		if col >= cols {
			col = cols - 1
		}
		endRow := row + rowSpan
		if endRow > rows {
			endRow = rows
		}
		endCol := col + colSpan
		if endCol > cols {
			endCol = cols
		}

		x0 := b.Min.X + g.colOffsets[col]
		y0 := b.Min.Y + g.rowOffsets[row]
		x1 := b.Min.X + g.colOffsets[endCol]
		y1 := b.Min.Y + g.rowOffsets[endRow]

		// Учитываем Margin ребёнка.
		type marginGetter interface {
			GetMargin() Margin
		}
		if mg, ok := child.(marginGetter); ok {
			m := mg.GetMargin()
			x0 += m.Left
			y0 += m.Top
			x1 -= m.Right
			y1 -= m.Bottom
		}

		cellRect := image.Rect(x0, y0, x1, y1)
		child.SetBounds(applyAlignmentRect(child, cellRect))
	}
}

// childCell возвращает Grid.Row, Grid.Column, RowSpan, ColSpan для потомка.
func (g *Grid) childCell(w Widget) (row, col, rowSpan, colSpan int) {
	// Пытаемся получить через Base (все наши виджеты встраивают Base).
	type gridProps interface {
		GetGridRow() int
		GetGridColumn() int
		GetGridRowSpan() int
		GetGridColSpan() int
	}
	if gp, ok := w.(gridProps); ok {
		row = gp.GetGridRow()
		col = gp.GetGridColumn()
		rowSpan = gp.GetGridRowSpan()
		colSpan = gp.GetGridColSpan()
	}
	if rowSpan < 1 {
		rowSpan = 1
	}
	if colSpan < 1 {
		colSpan = 1
	}
	return
}

// Draw рисует фон, потомков и опционально линии сетки.
func (g *Grid) Draw(ctx DrawContext) {
	b := g.bounds

	// Фон
	if g.UseAlpha && g.Background.A < 255 {
		if g.Background.A > 0 {
			if ac, ok := ctx.(DrawContextAlpha); ok {
				ac.FillRectAlpha(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), g.Background)
			}
		}
	} else if g.Background.A > 0 {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), g.Background)
	}

	// Потомки
	g.drawChildren(ctx)

	// Линии сетки (отладка)
	if g.ShowGridLines && len(g.rowOffsets) > 0 && len(g.colOffsets) > 0 {
		for _, off := range g.rowOffsets {
			y := b.Min.Y + off
			ctx.DrawHLine(b.Min.X, y, b.Dx(), g.GridLineColor)
		}
		for _, off := range g.colOffsets {
			x := b.Min.X + off
			ctx.DrawVLine(x, b.Min.Y, b.Dy(), g.GridLineColor)
		}
	}
}

// ApplyTheme — Grid не имеет темизируемых элементов.
func (g *Grid) ApplyTheme(t *Theme) {}

// measureAutoSizes проходит по дочерним виджетам и устанавливает Min для Auto строк/столбцов
// на основе размеров дочерних виджетов (bounds, которые были заданы XAML-парсером).
//
// WPF Grid Auto: строка/столбец получает размер наибольшего дочернего виджета в этой строке/столбце.
// Если у ребёнка нет размера — используется дефолтный (30px для строк, 80px для столбцов).
func (g *Grid) measureAutoSizes() {
	// Сбрасываем Min для Auto определений
	for i := range g.RowDefs {
		if g.RowDefs[i].Mode == GridSizeAuto {
			g.RowDefs[i].Min = 0
		}
	}
	for i := range g.ColDefs {
		if g.ColDefs[i].Mode == GridSizeAuto {
			g.ColDefs[i].Min = 0
		}
	}

	for _, child := range g.children {
		row, col, _, _ := g.childCell(child)
		rows := g.Rows()
		cols := g.Cols()
		if row >= rows {
			row = rows - 1
		}
		if col >= cols {
			col = cols - 1
		}

		cb := child.Bounds()

		// Margin
		var m Margin
		type marginGetter interface {
			GetMargin() Margin
		}
		if mg, ok := child.(marginGetter); ok {
			m = mg.GetMargin()
		}

		// Auto строка: берём высоту ребёнка + margin.
		// Для виджетов без явного размера (bounds пуст) — используем desiredHeight.
		if row < len(g.RowDefs) && g.RowDefs[row].Mode == GridSizeAuto {
			h := cb.Dy() + m.Top + m.Bottom
			if h <= 0 {
				h = desiredHeight(child)
			}
			if h > g.RowDefs[row].Min {
				g.RowDefs[row].Min = h
			}
		}

		// Auto столбец: берём ширину ребёнка + margin
		if col < len(g.ColDefs) && g.ColDefs[col].Mode == GridSizeAuto {
			w := cb.Dx() + m.Left + m.Right
			if w <= 0 {
				w = desiredWidth(child)
			}
			if w > g.ColDefs[col].Min {
				g.ColDefs[col].Min = w
			}
		}
	}
}

// ─── Resolve definitions ────────────────────────────────────────────────────

// resolveDefinitions вычисляет offsets (len = n+1) для набора определений.
// totalPx — доступное пространство в пикселях.
func resolveDefinitions(defs []GridDefinition, totalPx int) []int {
	n := len(defs)
	if n == 0 {
		// Одна неявная строка/столбец = всё пространство.
		return []int{0, totalPx}
	}

	sizes := make([]int, n)
	remaining := totalPx
	var totalStar float64

	// 1-й проход: Pixel и Auto.
	for i, d := range defs {
		switch d.Mode {
		case GridSizePixel:
			px := int(d.Value)
			px = clampDef(px, d)
			sizes[i] = px
			remaining -= px
		case GridSizeAuto:
			// Auto без измерения содержимого — используем Min или 0.
			px := d.Min
			sizes[i] = px
			remaining -= px
		case GridSizeStar:
			v := d.Value
			if v <= 0 {
				v = 1
			}
			totalStar += v
		}
	}

	if remaining < 0 {
		remaining = 0
	}

	// 2-й проход: Star.
	if totalStar > 0 {
		for i, d := range defs {
			if d.Mode == GridSizeStar {
				v := d.Value
				if v <= 0 {
					v = 1
				}
				px := int(float64(remaining) * v / totalStar)
				px = clampDef(px, d)
				sizes[i] = px
			}
		}
	}

	// Строим offsets.
	offsets := make([]int, n+1)
	for i := 0; i < n; i++ {
		offsets[i+1] = offsets[i] + sizes[i]
	}
	return offsets
}

func clampDef(px int, d GridDefinition) int {
	if d.Min > 0 && px < d.Min {
		px = d.Min
	}
	if d.Max > 0 && px > d.Max {
		px = d.Max
	}
	return px
}
