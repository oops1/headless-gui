// Package widget — GridSplitter: перетаскиваемый разделитель столбцов/строк Grid.
//
// В отличие от прежней реализации (визуальный Separator), этот сплиттер реально
// изменяет размеры соседних столбцов/строк родительского Grid мышью.
package widget

import (
	"image"
	"image/color"
)

// GridSplitter — разделитель, изменяющий размеры соседних ячеек Grid.
type GridSplitter struct {
	Base
	Background color.RGBA
	HoverColor color.RGBA

	grid     *Grid           // родительский Grid (выставляется при сборке)
	capMgr   CaptureManager
	dragging bool
	startX   int
	startY   int
	hovered  bool
}

// NewGridSplitter создаёт сплиттер.
func NewGridSplitter() *GridSplitter {
	return &GridSplitter{
		Background: win10.Border,
		HoverColor: win10.Accent,
	}
}

// SetGrid задаёт родительский Grid (вызывается XAML-сборщиком Grid).
func (s *GridSplitter) SetGrid(g *Grid) { s.grid = g }

// vertical=true → тонкий вертикальный сплиттер (двигает столбцы, тянется по X).
func (s *GridSplitter) vertical() bool {
	b := s.bounds
	return b.Dx() <= b.Dy()
}

// Draw рисует полосу разделителя.
func (s *GridSplitter) Draw(ctx DrawContext) {
	b := s.bounds
	if b.Empty() {
		return
	}
	col := s.Background
	if s.hovered || s.dragging {
		col = s.HoverColor
	}
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), col)
}

// WantsCapture захватывает мышь при нажатии ЛКМ (для перетаскивания).
func (s *GridSplitter) WantsCapture(e MouseEvent) bool {
	return s.IsEnabled() && e.Button == MouseLeft && e.Pressed
}

// SetCaptureManager сохраняет менеджер захвата мыши.
func (s *GridSplitter) SetCaptureManager(cm CaptureManager) { s.capMgr = cm }

// OnMouseButton начинает/заканчивает перетаскивание.
func (s *GridSplitter) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	if e.Pressed {
		if !s.dragging {
			s.dragging = true
			s.Invalidate() // подсветка активного сплиттера
		}
		s.startX = e.X
		s.startY = e.Y
		return true
	}
	if s.dragging {
		s.dragging = false
		s.Invalidate() // подсветка гаснет
	}
	if s.capMgr != nil {
		s.capMgr.ReleaseCapture()
	}
	return true
}

// OnMouseMove перетаскивает границу и обновляет hover.
func (s *GridSplitter) OnMouseMove(x, y int) {
	if s.dragging {
		if s.grid == nil {
			return
		}
		if s.vertical() {
			dx := x - s.startX
			if dx != 0 {
				s.grid.ResizeColumnsAround(s.GetGridColumn(), dx)
				s.startX = x
				s.grid.Invalidate() // перекладка ячеек — перерисовать весь Grid
			}
		} else {
			dy := y - s.startY
			if dy != 0 {
				s.grid.ResizeRowsAround(s.GetGridRow(), dy)
				s.startY = y
				s.grid.Invalidate() // перекладка ячеек — перерисовать весь Grid
			}
		}
		return
	}
	hovered := image.Pt(x, y).In(s.bounds)
	if hovered != s.hovered {
		s.hovered = hovered
		s.Invalidate()
	}
}

// Cursor возвращает курсор изменения размера (для CursorProvider).
func (s *GridSplitter) Cursor(x, y int) Cursor {
	if s.vertical() {
		return CursorSizeWE
	}
	return CursorSizeNS
}

// ApplyTheme обновляет цвета.
func (s *GridSplitter) ApplyTheme(t *Theme) {
	s.Background = t.Border
	s.HoverColor = t.Accent
}
