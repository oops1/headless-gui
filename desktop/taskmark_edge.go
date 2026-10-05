// taskmark_edge.go — метка открытого окна у бокового края.
//
// В ряду метка лежит под кнопкой. В столбце боковой панели «снизу ячейки»
// значило бы посреди столбца, между соседями: Windows 10 ставит её на сторону
// кнопки, обращённую к краю экрана, — слева у левой панели, справа у правой.
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// EdgeAware — элемент панели, которому важно, у какого края экрана она стоит.
// Необязателен и дополняет VerticalItem: панель сообщает край при добавлении
// элемента и при каждой перекладке (смена края, монитора).
type EdgeAware interface {
	// SetBarEdge сообщает край, к которому прижата панель.
	SetBarEdge(e Edge)
}

// markEdgeOf — сторона, на которой рисуется метка: нижняя в ряду, а в столбце
// сторона к краю экрана (у левой панели — левая, у правой — правая).
func markEdgeOf(vertical bool, bar Edge) Edge {
	switch {
	case !vertical:
		return EdgeBottom
	case bar == EdgeRight:
		return EdgeRight
	default:
		return EdgeLeft
	}
}

// drawTaskMarkAt рисует метку открытого окна на стороне edge ячейки r. Для
// EdgeBottom — прежняя метка под кнопкой (drawTaskMark).
func drawTaskMarkAt(ctx widget.DrawContext, tm *theme.Manager, r image.Rectangle, active bool, s *theme.Style, edge Edge) {
	if edge == EdgeBottom {
		drawTaskMark(ctx, tm, r, active, s)
		return
	}
	thick := int(tmMetric(tm, KeyTaskButtonUnderline))
	if idle := tmMetric(tm, KeyTaskButtonUnderlineIdleLen); !active && idle > 0 {
		if t := int(tmMetric(tm, KeyTaskButtonUnderlineIdle)); t > 0 {
			thick = t
		}
		drawMarkBarAt(ctx, r, thick, idle, textOnly(s), edge)
		return
	}
	ratio := tmMetric(tm, KeyTaskButtonUnderlineLen)
	if ratio <= 0 {
		ratio = 1
	}
	if ratio >= 1 && !active {
		return // полоса во всю кнопку — примета активного окна
	}
	k := ratio
	if !active {
		k /= 2
	}
	drawMarkBarAt(ctx, r, thick, k, s, edge)
}

// drawMarkBarAt рисует полосу толщиной thickness вдоль стороны edge ячейки r,
// длиной в долю k стороны, по центру. Цвет — рамка стиля, а без неё текст.
func drawMarkBarAt(ctx widget.DrawContext, r image.Rectangle, thickness int, k float64, s *theme.Style, edge Edge) {
	if edge == EdgeBottom {
		drawMarkBar(ctx, r, thickness, k, s)
		return
	}
	if thickness <= 0 || s == nil || r.Empty() {
		return
	}
	col := s.Border
	if col.A == 0 {
		col = s.Text
	}
	if col.A == 0 {
		return
	}
	l := int(float64(r.Dy()) * k)
	if l < thickness {
		l = thickness
	}
	if l > r.Dy() {
		l = r.Dy()
	}
	y := r.Min.Y + (r.Dy()-l)/2
	x := r.Min.X
	if edge == EdgeRight {
		x = r.Max.X - thickness
	}
	ctx.FillRect(x, y, thickness, l, col)
}
