package svg

import (
	"math"
	"strconv"
	"strings"
)

// kappa — коэффициент кубической аппроксимации четверти окружности.
const kappa = 0.5522847498307936

// circleContours строит контур окружности (cx,cy,r) четырьмя дугами Безье.
func circleContours(cx, cy, r float64) []Contour {
	return ellipseContours(cx, cy, r, r)
}

// ellipseContours строит контур эллипса (cx,cy,rx,ry).
func ellipseContours(cx, cy, rx, ry float64) []Contour {
	if rx <= 0 || ry <= 0 {
		return nil
	}
	b := &pathBuilder{}
	kx, ky := kappa*rx, kappa*ry
	b.moveTo(cx, cy-ry)
	b.cubicTo(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy)
	b.cubicTo(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry)
	b.cubicTo(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy)
	b.cubicTo(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry)
	b.close()
	return b.contours
}

// rectContours строит контур прямоугольника (возможно со скруглением rx,ry).
func rectContours(x, y, w, h, rx, ry float64) []Contour {
	if w <= 0 || h <= 0 {
		return nil
	}
	// Нормализация скруглений по правилам SVG.
	if rx < 0 {
		rx = 0
	}
	if ry < 0 {
		ry = 0
	}
	if rx == 0 && ry > 0 {
		rx = ry
	}
	if ry == 0 && rx > 0 {
		ry = rx
	}
	if rx > w/2 {
		rx = w / 2
	}
	if ry > h/2 {
		ry = h / 2
	}
	b := &pathBuilder{}
	if rx == 0 || ry == 0 {
		b.moveTo(x, y)
		b.lineTo(x+w, y)
		b.lineTo(x+w, y+h)
		b.lineTo(x, y+h)
		b.close()
		return b.contours
	}
	kx, ky := kappa*rx, kappa*ry
	// По часовой стрелке, начиная от верхнего левого скругления.
	b.moveTo(x+rx, y)
	b.lineTo(x+w-rx, y)
	b.cubicTo(x+w-rx+kx, y, x+w, y+ry-ky, x+w, y+ry)
	b.lineTo(x+w, y+h-ry)
	b.cubicTo(x+w, y+h-ry+ky, x+w-rx+kx, y+h, x+w-rx, y+h)
	b.lineTo(x+rx, y+h)
	b.cubicTo(x+rx-kx, y+h, x, y+h-ry+ky, x, y+h-ry)
	b.lineTo(x, y+ry)
	b.cubicTo(x, y+ry-ky, x+rx-kx, y, x+rx, y)
	b.close()
	return b.contours
}

// lineContour строит открытый контур отрезка.
func lineContour(x1, y1, x2, y2 float64) []Contour {
	return []Contour{{Points: []Point{{x1, y1}, {x2, y2}}, Closed: false}}
}

// parsePointList разбирает "x1,y1 x2,y2 ..." в список точек.
func parsePointList(s string) []Point {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	var pts []Point
	for i := 0; i+1 < len(fields); i += 2 {
		x, err1 := strconv.ParseFloat(fields[i], 64)
		y, err2 := strconv.ParseFloat(fields[i+1], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		pts = append(pts, Point{X: x, Y: y})
	}
	return pts
}

// polyContours строит контур из списка точек (closed=true — polygon).
func polyContours(pts []Point, closed bool) []Contour {
	if len(pts) < 2 {
		return nil
	}
	return []Contour{{Points: pts, Closed: closed}}
}

// parseLength разбирает длину/координату, отбрасывая единицы px и проценты
// (проценты не поддержаны — трактуются как число). Пустая строка → 0.
func parseLength(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.TrimSuffix(s, "px")
	s = strings.TrimSuffix(s, "%")
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}
