package widget

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// Линии и контуры в ячейке таблицы — запрос GG-54.
//
// В мост ячейки (datagrid.DrawContextBridge) из AAShapes пробрасывались только
// FillEllipseAA и FillRoundRect: наклонная линия в шаблонной колонке
// складывалась ступеньками из FillRect, а кольцо (точка слияния в графе
// коммитов) не собиралось вовсе — двумя заливками его не нарисовать, на
// выделенной строке фон другой.

// Мост обязан реализовываться адаптером целиком: проверка на этапе сборки.
var _ datagrid.DrawContextBridge = (*drawContextAdapter)(nil)

// aaRecorder — контекст со сглаженными примитивами, запоминающий вызовы.
type aaRecorder struct {
	DrawContext
	lines     []string
	polylines int
	closed    bool
	ellipses  int
	polygons  int
	thickness float64
	fills     int
}

func (r *aaRecorder) FillRect(int, int, int, int, color.RGBA) { r.fills++ }
func (r *aaRecorder) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) {}
func (r *aaRecorder) StrokeEllipseAA(cx, cy, rx, ry int, t float64, col color.RGBA) {
	r.ellipses++
	r.thickness = t
}
func (r *aaRecorder) FillPolygonAA(pts []image.Point, col color.RGBA) { r.polygons++ }
func (r *aaRecorder) StrokePolylineAA(pts []image.Point, t float64, closed bool, col color.RGBA) {
	r.polylines++
	r.closed = closed
	r.thickness = t
}
func (r *aaRecorder) DrawLineAA(x1, y1, x2, y2 int, t float64, col color.RGBA) {
	r.lines = append(r.lines, "line")
	r.thickness = t
}

// Со сглаживающим холстом мост зовёт примитивы движка, а не рисует ступеньки
// сам: ради этого запрос и подан.
func TestCellBridge_ForwardsAAPrimitives(t *testing.T) {
	rec := &aaRecorder{}
	a := &drawContextAdapter{ctx: rec}
	blue := color.RGBA{B: 200, A: 255}

	a.DrawLineAA(0, 0, 40, 20, 1.5, blue)
	if len(rec.lines) != 1 {
		t.Errorf("отрезок не дошёл до холста: %d вызовов", len(rec.lines))
	}
	if rec.thickness != 1.5 {
		t.Errorf("толщина округлилась до %v — дробная у сглаженной линии осмысленна", rec.thickness)
	}

	a.StrokePolylineAA([]image.Point{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 20, Y: 0}}, 2, true, blue)
	if rec.polylines != 1 || !rec.closed {
		t.Errorf("ломаная: вызовов %d, closed=%v", rec.polylines, rec.closed)
	}

	a.StrokeEllipseAA(10, 10, 5, 5, 2, blue)
	if rec.ellipses != 1 {
		t.Errorf("кольцо не дошло до холста: %d вызовов", rec.ellipses)
	}

	a.FillPolygonAA([]image.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 8}}, blue)
	if rec.polygons != 1 {
		t.Errorf("многоугольник не дошёл до холста: %d вызовов", rec.polygons)
	}

	if rec.fills != 0 {
		t.Errorf("на сглаживающем холсте мост нарисовал %d прямоугольников — "+
			"это откат, он здесь не нужен", rec.fills)
	}
}

// plainCtx — холст БЕЗ сглаженных примитивов: их поддержка опциональна.
type plainCtx struct {
	DrawContext
	fills int
}

func (c *plainCtx) FillRect(int, int, int, int, color.RGBA) { c.fills++ }

// Без AAShapes фигура всё равно ложится — ступеньками, но ложится: контекст
// без сглаживания есть и у печати, и у тестов.
func TestCellBridge_FallsBackWithoutAAShapes(t *testing.T) {
	col := color.RGBA{B: 200, A: 255}
	cases := []struct {
		name string
		draw func(a *drawContextAdapter)
	}{
		{"отрезок", func(a *drawContextAdapter) { a.DrawLineAA(0, 0, 30, 15, 2, col) }},
		{"ломаная", func(a *drawContextAdapter) {
			a.StrokePolylineAA([]image.Point{{X: 0, Y: 0}, {X: 10, Y: 10}}, 2, false, col)
		}},
		{"кольцо", func(a *drawContextAdapter) { a.StrokeEllipseAA(20, 20, 8, 8, 2, col) }},
		{"многоугольник", func(a *drawContextAdapter) {
			a.FillPolygonAA([]image.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 8}}, col)
		}},
	}
	for _, tc := range cases {
		ctx := &plainCtx{}
		tc.draw(&drawContextAdapter{ctx: ctx})
		if ctx.fills == 0 {
			t.Errorf("%s: на холсте без сглаживания не нарисовано ничего", tc.name)
		}
	}
}

// Замыкание ломаной в откате рисует лишний сегмент: контур должен сойтись.
func TestCellBridge_FallbackClosesPolyline(t *testing.T) {
	col := color.RGBA{B: 200, A: 255}
	pts := []image.Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}}

	open := &plainCtx{}
	(&drawContextAdapter{ctx: open}).StrokePolylineAA(pts, 1, false, col)

	closed := &plainCtx{}
	(&drawContextAdapter{ctx: closed}).StrokePolylineAA(pts, 1, true, col)

	if closed.fills <= open.fills {
		t.Errorf("замкнутая ломаная нарисована не длиннее открытой: %d против %d",
			closed.fills, open.fills)
	}
}

// Толщина тоньше точки не исчезает: линия в 0.4 точки на ступенчатом холсте
// всё равно должна быть видна.
func TestCellBridge_ThinLineStaysVisible(t *testing.T) {
	ctx := &plainCtx{}
	(&drawContextAdapter{ctx: ctx}).DrawLineAA(0, 0, 20, 0, 0.4, color.RGBA{A: 255})
	if ctx.fills == 0 {
		t.Error("линия толщиной 0.4 не нарисована вовсе")
	}
}
