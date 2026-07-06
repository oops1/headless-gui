// shapes.go — векторные фигуры WPF (Ellipse, Rectangle, Line, Polygon, Polyline).
//
// Рисуются поверх существующих примитивов DrawContext (FillRect/SetPixel), без
// изменения интерфейса. Поддерживают Fill, Stroke, StrokeThickness.
package widget

import (
	"image"
	"image/color"
	"strings"
)

// ─── Общие помощники рисования ──────────────────────────────────────────────
//
// Каждый помощник сначала проверяет, поддерживает ли DrawContext сглаженные
// примитивы (AAShapes — реализует engine.Canvas); при отсутствии — прежняя
// ступенчатая отрисовка (Bresenham/scanline).

// drawThickLine рисует отрезок (x0,y0)-(x1,y1) толщиной t.
func drawThickLine(ctx DrawContext, x0, y0, x1, y1, t int, col color.RGBA) {
	if col.A == 0 {
		return
	}
	if t < 1 {
		t = 1
	}
	if aa, ok := ctx.(AAShapes); ok {
		aa.DrawLineAA(x0, y0, x1, y1, float64(t), col)
		return
	}
	dx := absInt(x1 - x0)
	dy := -absInt(y1 - y0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	half := t / 2
	for {
		ctx.FillRect(x0-half, y0-half, t, t, col) // «жирная» точка
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// fillEllipse заливает эллипс с центром (cx,cy) и полуосями rx,ry.
func fillEllipse(ctx DrawContext, cx, cy, rx, ry int, col color.RGBA) {
	if col.A == 0 || rx <= 0 || ry <= 0 {
		return
	}
	if aa, ok := ctx.(AAShapes); ok {
		aa.FillEllipseAA(cx, cy, rx, ry, col)
		return
	}
	rx2 := float64(rx) * float64(rx)
	for dy := -ry; dy <= ry; dy++ {
		f := 1.0 - float64(dy*dy)/(float64(ry)*float64(ry))
		if f < 0 {
			continue
		}
		w := int(float64(rx) * sqrtApprox(f))
		if w <= 0 {
			continue
		}
		_ = rx2
		ctx.FillRect(cx-w, cy+dy, 2*w+1, 1, col)
	}
}

// drawEllipseOutline рисует контур эллипса толщиной t (через заливку «кольца»).
func drawEllipseOutline(ctx DrawContext, cx, cy, rx, ry, t int, col color.RGBA) {
	if col.A == 0 || rx <= 0 || ry <= 0 {
		return
	}
	if t < 1 {
		t = 1
	}
	if aa, ok := ctx.(AAShapes); ok {
		aa.StrokeEllipseAA(cx, cy, rx, ry, float64(t), col)
		return
	}
	for dy := -ry; dy <= ry; dy++ {
		fo := 1.0 - float64(dy*dy)/(float64(ry)*float64(ry))
		if fo < 0 {
			continue
		}
		wo := int(float64(rx) * sqrtApprox(fo))
		// внутренний радиус
		iry := ry - t
		wi := 0
		if iry > 0 {
			fi := 1.0 - float64(dy*dy)/(float64(iry)*float64(iry))
			if fi > 0 {
				wi = int(float64(rx-t) * sqrtApprox(fi))
			}
		}
		if wi <= 0 {
			ctx.FillRect(cx-wo, cy+dy, 2*wo+1, 1, col)
		} else {
			ctx.FillRect(cx-wo, cy+dy, wo-wi+1, 1, col)
			ctx.FillRect(cx+wi, cy+dy, wo-wi+1, 1, col)
		}
	}
}

// fillPolygon заливает многоугольник (scanline).
func fillPolygon(ctx DrawContext, pts []image.Point, col color.RGBA) {
	if col.A == 0 || len(pts) < 3 {
		return
	}
	if aa, ok := ctx.(AAShapes); ok {
		aa.FillPolygonAA(pts, col)
		return
	}
	minY, maxY := pts[0].Y, pts[0].Y
	for _, p := range pts {
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	for y := minY; y <= maxY; y++ {
		var xs []int
		n := len(pts)
		for i := 0; i < n; i++ {
			a, b := pts[i], pts[(i+1)%n]
			if (a.Y <= y && b.Y > y) || (b.Y <= y && a.Y > y) {
				x := a.X + (y-a.Y)*(b.X-a.X)/(b.Y-a.Y)
				xs = append(xs, x)
			}
		}
		if len(xs) < 2 {
			continue
		}
		// сортировка пересечений
		for i := 0; i < len(xs); i++ {
			for j := i + 1; j < len(xs); j++ {
				if xs[j] < xs[i] {
					xs[i], xs[j] = xs[j], xs[i]
				}
			}
		}
		for i := 0; i+1 < len(xs); i += 2 {
			ctx.FillRect(xs[i], y, xs[i+1]-xs[i]+1, 1, col)
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// sqrtApprox — sqrt без импорта math (Ньютон).
func sqrtApprox(x float64) float64 {
	if x <= 0 {
		return 0
	}
	g := x
	for i := 0; i < 16; i++ {
		g = 0.5 * (g + x/g)
	}
	return g
}

// ─── Ellipse ────────────────────────────────────────────────────────────────

// Ellipse — эллипс/круг (WPF Ellipse).
type Ellipse struct {
	Base
	Fill            color.RGBA
	Stroke          color.RGBA
	StrokeThickness int
}

// NewEllipse создаёт эллипс с заданной заливкой.
func NewEllipse(fill color.RGBA) *Ellipse { return &Ellipse{Fill: fill} }

func (e *Ellipse) Draw(ctx DrawContext) {
	b := e.bounds
	if b.Empty() {
		return
	}
	cx := b.Min.X + b.Dx()/2
	cy := b.Min.Y + b.Dy()/2
	rx := b.Dx() / 2
	ry := b.Dy() / 2
	if e.Fill.A > 0 {
		fillEllipse(ctx, cx, cy, rx, ry, e.Fill)
	}
	if e.Stroke.A > 0 && e.StrokeThickness > 0 {
		drawEllipseOutline(ctx, cx, cy, rx, ry, e.StrokeThickness, e.Stroke)
	}
}

func (e *Ellipse) ApplyTheme(t *Theme) {}

// ─── Rectangle (фигура, не путать с Separator) ──────────────────────────────

// RectangleShape — прямоугольник (WPF Rectangle) с заливкой/обводкой/скруглением.
type RectangleShape struct {
	Base
	Fill            color.RGBA
	Stroke          color.RGBA
	StrokeThickness int
	RadiusX         int
}

// NewRectangleShape создаёт прямоугольник с заданной заливкой.
func NewRectangleShape(fill color.RGBA) *RectangleShape { return &RectangleShape{Fill: fill} }

func (r *RectangleShape) Draw(ctx DrawContext) {
	b := r.bounds
	if b.Empty() {
		return
	}
	if r.Fill.A > 0 {
		if r.RadiusX > 0 {
			ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), r.RadiusX, r.Fill)
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), r.Fill)
		}
	}
	if r.Stroke.A > 0 && r.StrokeThickness > 0 {
		if r.RadiusX > 0 {
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), r.RadiusX, r.Stroke)
		} else {
			for i := 0; i < r.StrokeThickness; i++ {
				ctx.DrawBorder(b.Min.X+i, b.Min.Y+i, b.Dx()-2*i, b.Dy()-2*i, r.Stroke)
			}
		}
	}
}

func (r *RectangleShape) ApplyTheme(t *Theme) {}

// ─── Line ───────────────────────────────────────────────────────────────────

// Line — отрезок (WPF Line). Координаты абсолютные (canvas).
type Line struct {
	Base
	X1, Y1, X2, Y2  int
	Stroke          color.RGBA
	StrokeThickness int
}

func (l *Line) Draw(ctx DrawContext) {
	th := l.StrokeThickness
	if th < 1 {
		th = 1
	}
	drawThickLine(ctx, l.X1, l.Y1, l.X2, l.Y2, th, l.Stroke)
}

func (l *Line) ApplyTheme(t *Theme) {}

// ─── Polygon / Polyline ─────────────────────────────────────────────────────

// Polygon — замкнутый многоугольник (WPF Polygon).
type Polygon struct {
	Base
	Points          []image.Point
	Fill            color.RGBA
	Stroke          color.RGBA
	StrokeThickness int
}

func (p *Polygon) Draw(ctx DrawContext) {
	if len(p.Points) < 2 {
		return
	}
	if p.Fill.A > 0 {
		fillPolygon(ctx, p.Points, p.Fill)
	}
	if p.Stroke.A > 0 {
		th := p.StrokeThickness
		if th < 1 {
			th = 1
		}
		for i := 0; i < len(p.Points); i++ {
			a, b := p.Points[i], p.Points[(i+1)%len(p.Points)]
			drawThickLine(ctx, a.X, a.Y, b.X, b.Y, th, p.Stroke)
		}
	}
}

func (p *Polygon) ApplyTheme(t *Theme) {}

// Polyline — незамкнутая ломаная (WPF Polyline).
type Polyline struct {
	Base
	Points          []image.Point
	Stroke          color.RGBA
	StrokeThickness int
}

func (p *Polyline) Draw(ctx DrawContext) {
	if len(p.Points) < 2 {
		return
	}
	th := p.StrokeThickness
	if th < 1 {
		th = 1
	}
	for i := 0; i+1 < len(p.Points); i++ {
		a, b := p.Points[i], p.Points[i+1]
		drawThickLine(ctx, a.X, a.Y, b.X, b.Y, th, p.Stroke)
	}
}

func (p *Polyline) ApplyTheme(t *Theme) {}

// ─── XAML-построители фигур ─────────────────────────────────────────────────

func buildXAMLRectangleShape(el xElement) Widget {
	r := &RectangleShape{}
	applyColor(&r.Fill, el, "Fill", "Background")
	applyColor(&r.Stroke, el, "Stroke", "BorderBrush")
	r.StrokeThickness = xatoi(el.attr("StrokeThickness"))
	if r.StrokeThickness == 0 && r.Stroke.A > 0 {
		r.StrokeThickness = 1
	}
	r.RadiusX = xatoi(el.attr("RadiusX", "CornerRadius"))
	return r
}

func buildXAMLEllipse(el xElement) Widget {
	e := &Ellipse{}
	applyColor(&e.Fill, el, "Fill", "Background")
	applyColor(&e.Stroke, el, "Stroke", "BorderBrush")
	e.StrokeThickness = xatoi(el.attr("StrokeThickness"))
	if e.StrokeThickness == 0 && e.Stroke.A > 0 {
		e.StrokeThickness = 1
	}
	return e
}

func buildXAMLLine(el xElement, reg map[string]Widget, off image.Point) Widget {
	l := &Line{
		X1: xatoi(el.attr("X1")) + off.X, Y1: xatoi(el.attr("Y1")) + off.Y,
		X2: xatoi(el.attr("X2")) + off.X, Y2: xatoi(el.attr("Y2")) + off.Y,
	}
	applyColor(&l.Stroke, el, "Stroke")
	l.StrokeThickness = xatoi(el.attr("StrokeThickness"))
	if l.StrokeThickness < 1 {
		l.StrokeThickness = 1
	}
	l.SetBounds(image.Rect(min(l.X1, l.X2), min(l.Y1, l.Y2), max(l.X1, l.X2)+1, max(l.Y1, l.Y2)+1))
	if id := el.name(); id != "" {
		reg[id] = l
	}
	return l
}

func buildXAMLPolygon(el xElement, reg map[string]Widget, off image.Point) Widget {
	p := &Polygon{Points: parsePoints(el.attr("Points"), off)}
	applyColor(&p.Fill, el, "Fill")
	applyColor(&p.Stroke, el, "Stroke")
	p.StrokeThickness = xatoi(el.attr("StrokeThickness"))
	if p.StrokeThickness < 1 && p.Stroke.A > 0 {
		p.StrokeThickness = 1
	}
	p.SetBounds(pointsBounds(p.Points))
	if id := el.name(); id != "" {
		reg[id] = p
	}
	return p
}

func buildXAMLPolyline(el xElement, reg map[string]Widget, off image.Point) Widget {
	p := &Polyline{Points: parsePoints(el.attr("Points"), off)}
	applyColor(&p.Stroke, el, "Stroke")
	p.StrokeThickness = xatoi(el.attr("StrokeThickness"))
	if p.StrokeThickness < 1 {
		p.StrokeThickness = 1
	}
	p.SetBounds(pointsBounds(p.Points))
	if id := el.name(); id != "" {
		reg[id] = p
	}
	return p
}

// pointsBounds возвращает охватывающий прямоугольник набора точек.
func pointsBounds(pts []image.Point) image.Rectangle {
	if len(pts) == 0 {
		return image.Rectangle{}
	}
	minx, miny, maxx, maxy := pts[0].X, pts[0].Y, pts[0].X, pts[0].Y
	for _, p := range pts {
		minx, miny = min(minx, p.X), min(miny, p.Y)
		maxx, maxy = max(maxx, p.X), max(maxy, p.Y)
	}
	return image.Rect(minx, miny, maxx+1, maxy+1)
}

// parsePoints разбирает "x1,y1 x2,y2 ..." со смещением off.
func parsePoints(s string, off image.Point) []image.Point {
	var pts []image.Point
	// Разбиваем по пробелам/запятым на числа, берём попарно (x, y).
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	for i := 0; i+1 < len(fields); i += 2 {
		x := xatoi(strings.TrimSpace(fields[i]))
		y := xatoi(strings.TrimSpace(fields[i+1]))
		pts = append(pts, image.Pt(x+off.X, y+off.Y))
	}
	return pts
}
