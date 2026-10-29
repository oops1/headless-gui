package xaml

// shapes.go — XAML-строители фигур (Rectangle, Ellipse, Line, Polygon,
// Polyline) и разбор списка точек. Сами фигуры — в widget/base/shapes.go.

import (
	"image"
	"strings"

	"github.com/oops1/headless-gui/v3/widget/base"
)

func buildXAMLRectangleShape(el xElement) base.Widget {
	r := &base.RectangleShape{}
	applyColor(&r.Fill, el, "Fill", "Background")
	applyColor(&r.Stroke, el, "Stroke", "BorderBrush")
	r.StrokeThickness = xatoi(el.attr("StrokeThickness"))
	if r.StrokeThickness == 0 && r.Stroke.A > 0 {
		r.StrokeThickness = 1
	}
	r.RadiusX = xatoi(el.attr("RadiusX", "CornerRadius"))
	return r
}

func buildXAMLEllipse(el xElement) base.Widget {
	e := &base.Ellipse{}
	applyColor(&e.Fill, el, "Fill", "Background")
	applyColor(&e.Stroke, el, "Stroke", "BorderBrush")
	e.StrokeThickness = xatoi(el.attr("StrokeThickness"))
	if e.StrokeThickness == 0 && e.Stroke.A > 0 {
		e.StrokeThickness = 1
	}
	return e
}

func buildXAMLLine(el xElement, reg map[string]base.Widget, off image.Point) base.Widget {
	l := &base.Line{
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

func buildXAMLPolygon(el xElement, reg map[string]base.Widget, off image.Point) base.Widget {
	p := &base.Polygon{Points: parsePoints(el.attr("Points"), off)}
	applyColor(&p.Fill, el, "Fill")
	applyColor(&p.Stroke, el, "Stroke")
	p.StrokeThickness = xatoi(el.attr("StrokeThickness"))
	if p.StrokeThickness < 1 && p.Stroke.A > 0 {
		p.StrokeThickness = 1
	}
	p.SetBounds(base.PointsBounds(p.Points))
	if id := el.name(); id != "" {
		reg[id] = p
	}
	return p
}

func buildXAMLPolyline(el xElement, reg map[string]base.Widget, off image.Point) base.Widget {
	p := &base.Polyline{Points: parsePoints(el.attr("Points"), off)}
	applyColor(&p.Stroke, el, "Stroke")
	p.StrokeThickness = xatoi(el.attr("StrokeThickness"))
	if p.StrokeThickness < 1 {
		p.StrokeThickness = 1
	}
	p.SetBounds(base.PointsBounds(p.Points))
	if id := el.name(); id != "" {
		reg[id] = p
	}
	return p
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
