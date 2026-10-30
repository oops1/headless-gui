package geom

// shapes.go — раскладка кривых и обрезка областей изображения: общие для
// холста движка (core/raster) и откатов виджетов (widget), чтобы на разных
// контекстах фигура и картинка выглядели одинаково.

import (
	"image"
	"math"
)

// FlattenCubic раскладывает кубическую кривую Безье в ломаную.
//
// Число отрезков — по длине контрольного многоугольника, примерно отрезок на
// две точки: короче — лишняя работа, длиннее — излом виден глазом на кривой с
// сильным изгибом. Экспортирован, потому что одна раскладка нужна и холсту, и
// откату для контекста без PathShapes: разложи они кривую по-разному — на
// разных контекстах она и выглядела бы по-разному.
func FlattenCubic(x0, y0, x1, y1, x2, y2, x3, y3 float64) []Point2F {
	l := math.Hypot(x1-x0, y1-y0) + math.Hypot(x2-x1, y2-y1) + math.Hypot(x3-x2, y3-y2)
	n := int(math.Ceil(l / 2))
	if n < 8 {
		n = 8
	}
	if n > 512 {
		n = 512
	}
	pts := make([]Point2F, n+1)
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		u := 1 - t
		a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
		pts[i] = Point2F{
			X: a*x0 + b*x1 + c*x2 + d*x3,
			Y: a*y0 + b*y1 + c*y2 + d*y3,
		}
	}
	return pts
}

// ClampImageRect обрезает sr по границам источника bounds и уменьшает dr
// пропорционально, чтобы оставшаяся часть картинки не растянулась. Любой из
// результатов пуст — рисовать нечего.
func ClampImageRect(bounds, sr, dr image.Rectangle) (image.Rectangle, image.Rectangle) {
	if sr.Empty() || dr.Empty() {
		return image.Rectangle{}, image.Rectangle{}
	}
	c := sr.Intersect(bounds)
	if c.Empty() {
		return image.Rectangle{}, image.Rectangle{}
	}
	if c == sr {
		return sr, dr
	}
	sw, sh := int64(sr.Dx()), int64(sr.Dy())
	dw, dh := int64(dr.Dx()), int64(dr.Dy())
	// Доля источника, отрезанная с каждой стороны, переводится в пиксели dr с
	// округлением к ближайшему.
	scaleTo := func(n, d, s int64) int {
		return int((n*d + s/2) / s)
	}
	out := image.Rect(
		dr.Min.X+scaleTo(int64(c.Min.X-sr.Min.X), dw, sw),
		dr.Min.Y+scaleTo(int64(c.Min.Y-sr.Min.Y), dh, sh),
		dr.Max.X-scaleTo(int64(sr.Max.X-c.Max.X), dw, sw),
		dr.Max.Y-scaleTo(int64(sr.Max.Y-c.Max.Y), dh, sh),
	)
	return c, out
}
