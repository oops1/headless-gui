package xaml

// shapes.go — XAML-строители фигур (Rectangle, Ellipse, Line, Polygon,
// Polyline) и разбор списка точек. Сами фигуры — в widget/base/shapes.go.

import (
	"image"
	"strings"

	"github.com/oops1/headless-gui/v3/widget/base"
)

func buildXAMLRectangleShape(el Element) base.Widget {
	r := &base.RectangleShape{}
	ApplyColor(&r.Fill, el, "Fill", "Background")
	ApplyColor(&r.Stroke, el, "Stroke", "BorderBrush")
	r.StrokeThickness = ParseInt(el.Attr("StrokeThickness"))
	if r.StrokeThickness == 0 && r.Stroke.A > 0 {
		r.StrokeThickness = 1
	}
	r.RadiusX = ParseInt(el.Attr("RadiusX", "CornerRadius"))
	return r
}

func buildXAMLEllipse(el Element) base.Widget {
	e := &base.Ellipse{}
	ApplyColor(&e.Fill, el, "Fill", "Background")
	ApplyColor(&e.Stroke, el, "Stroke", "BorderBrush")
	e.StrokeThickness = ParseInt(el.Attr("StrokeThickness"))
	if e.StrokeThickness == 0 && e.Stroke.A > 0 {
		e.StrokeThickness = 1
	}
	return e
}

func buildXAMLLine(el Element, reg map[string]base.Widget, off image.Point) base.Widget {
	l := &base.Line{
		X1: ParseInt(el.Attr("X1")) + off.X, Y1: ParseInt(el.Attr("Y1")) + off.Y,
		X2: ParseInt(el.Attr("X2")) + off.X, Y2: ParseInt(el.Attr("Y2")) + off.Y,
	}
	ApplyColor(&l.Stroke, el, "Stroke")
	l.StrokeThickness = ParseInt(el.Attr("StrokeThickness"))
	if l.StrokeThickness < 1 {
		l.StrokeThickness = 1
	}
	l.SetBounds(image.Rect(min(l.X1, l.X2), min(l.Y1, l.Y2), max(l.X1, l.X2)+1, max(l.Y1, l.Y2)+1))
	if id := el.Name(); id != "" {
		reg[id] = l
	}
	return l
}

func buildXAMLPolygon(el Element, reg map[string]base.Widget, off image.Point) base.Widget {
	p := &base.Polygon{Points: parsePoints(el.Attr("Points"), off)}
	ApplyColor(&p.Fill, el, "Fill")
	ApplyColor(&p.Stroke, el, "Stroke")
	p.StrokeThickness = ParseInt(el.Attr("StrokeThickness"))
	if p.StrokeThickness < 1 && p.Stroke.A > 0 {
		p.StrokeThickness = 1
	}
	p.SetBounds(base.PointsBounds(p.Points))
	if id := el.Name(); id != "" {
		reg[id] = p
	}
	return p
}

func buildXAMLPolyline(el Element, reg map[string]base.Widget, off image.Point) base.Widget {
	p := &base.Polyline{Points: parsePoints(el.Attr("Points"), off)}
	ApplyColor(&p.Stroke, el, "Stroke")
	p.StrokeThickness = ParseInt(el.Attr("StrokeThickness"))
	if p.StrokeThickness < 1 {
		p.StrokeThickness = 1
	}
	p.SetBounds(base.PointsBounds(p.Points))
	if id := el.Name(); id != "" {
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
		x := ParseInt(strings.TrimSpace(fields[i]))
		y := ParseInt(strings.TrimSpace(fields[i+1]))
		pts = append(pts, image.Pt(x+off.X, y+off.Y))
	}
	return pts
}
