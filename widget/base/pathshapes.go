// pathshapes.go — сглаженные пути с дробными координатами.
//
// AAShapes принимал только image.Point: гладкую кривую (S-коннектор между
// блоками сравнения) приходилось выбирать точками и округлять до целых, и
// погрешность ±0,5 точки на линии толщиной в полторы-две точки была видна
// глазом — «лесенка» и изломы. Приложение обходило это своим растеризатором и
// блитом картинки, заодно протаскивая в виджет масштаб HiDPI снаружи.
package base

import (
	hgdraw "github.com/oops1/headless-gui/v4/core/draw"
	"image"
	"image/color"
	"math"

	geom "github.com/oops1/headless-gui/v4/core/geom"
)

// Point2F — точка с дробными координатами в логических единицах. Тип живёт в
// core/geom; здесь алиас.
type Point2F = geom.Point2F

// PathShapes — см. draw.PathShapes (core/draw); алиас: тот же тип.
type PathShapes = hgdraw.PathShapes

// FlattenCubic раскладывает кубическую кривую Безье в ломаную; см.
// geom.FlattenCubic.
func FlattenCubic(x0, y0, x1, y1, x2, y2, x3, y3 float64) []Point2F {
	return geom.FlattenCubic(x0, y0, x1, y1, x2, y2, x3, y3)
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
	FillPolygon(ctx, roundPoints(pts), col)
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
