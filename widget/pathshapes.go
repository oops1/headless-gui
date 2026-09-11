// pathshapes.go — сглаженные пути с дробными координатами.
//
// AAShapes принимал только image.Point: гладкую кривую (S-коннектор между
// блоками сравнения) приходилось выбирать точками и округлять до целых, и
// погрешность ±0,5 точки на линии толщиной в полторы-две точки была видна
// глазом — «лесенка» и изломы. Приложение обходило это своим растеризатором и
// блитом картинки, заодно протаскивая в виджет масштаб HiDPI снаружи.
package widget

import (
	"image"
	"image/color"
	"math"
)

// Point2F — точка с дробными координатами в логических единицах.
//
// Координаты те же, что у целочисленных примитивов: целое значение — центр
// точки. Иначе путь, собранный из дробных точек, и отрезок DrawLineAA между
// теми же целыми точками разошлись бы на полпикселя.
type Point2F struct{ X, Y float64 }

// PathShapes — опциональные сглаженные пути с дробными координатами.
// Реализуется engine.Canvas (масштаб HiDPI холст учитывает сам).
//
// Ломаная обводится ОДНИМ контуром с круглыми сопряжениями во внутренних
// вершинах — на изгибе нет ни щели, ни зубца. Концы у незамкнутого пути
// прямые, как у DrawLineAA.
type PathShapes interface {
	// StrokeCubicAA — кубическая кривая Безье (x0,y0)…(x3,y3) толщиной thickness.
	StrokeCubicAA(x0, y0, x1, y1, x2, y2, x3, y3, thickness float64, col color.RGBA)
	// StrokePathAA — ломаная толщиной thickness (closed замыкает контур).
	StrokePathAA(pts []Point2F, thickness float64, closed bool, col color.RGBA)
	// FillPathAA — залитый многоугольник.
	FillPathAA(pts []Point2F, col color.RGBA)
}

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

// strokeCubic рисует кривую Безье: на холсте с PathShapes — одним гладким
// контуром, иначе — той же ломаной, округлённой до точек.
func strokeCubic(ctx DrawContext, x0, y0, x1, y1, x2, y2, x3, y3, thickness float64, col color.RGBA) {
	if ps, ok := ctx.(PathShapes); ok {
		ps.StrokeCubicAA(x0, y0, x1, y1, x2, y2, x3, y3, thickness, col)
		return
	}
	strokePath(ctx, FlattenCubic(x0, y0, x1, y1, x2, y2, x3, y3), thickness, false, col)
}

// strokePath рисует дробную ломаную; без PathShapes округляет её до точек и
// отдаёт прежнему пути (AAShapes или ступеньки).
func strokePath(ctx DrawContext, pts []Point2F, thickness float64, closed bool, col color.RGBA) {
	if ps, ok := ctx.(PathShapes); ok {
		ps.StrokePathAA(pts, thickness, closed, col)
		return
	}
	strokePolyline(ctx, roundPoints(pts), thickness, closed, col)
}

// fillPath заливает дробный многоугольник; без PathShapes — округлённый.
func fillPath(ctx DrawContext, pts []Point2F, col color.RGBA) {
	if ps, ok := ctx.(PathShapes); ok {
		ps.FillPathAA(pts, col)
		return
	}
	fillPolygon(ctx, roundPoints(pts), col)
}

// roundPoints округляет дробные точки, выбрасывая совпавшие подряд: после
// округления соседние точки плотной кривой часто падают в одну, а отрезок
// нулевой длины ступенчатому откату рисовать нечего.
func roundPoints(pts []Point2F) []image.Point {
	out := make([]image.Point, 0, len(pts))
	for _, p := range pts {
		q := image.Pt(int(math.Round(p.X)), int(math.Round(p.Y)))
		if n := len(out); n > 0 && out[n-1] == q {
			continue
		}
		out = append(out, q)
	}
	return out
}
