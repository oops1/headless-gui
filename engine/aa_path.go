// aa_path.go — сглаженные пути с дробными координатами (widget.PathShapes) и
// общая обводка ломаной для целочисленного и дробного вариантов.
//
// Ломаная раньше обводилась суммой прямоугольников по отрезкам, без
// сопряжений: на изгибе снаружи оставалась щель-зубец, и кривая, собранная из
// коротких отрезков, выглядела изломанной. Координаты вдобавок были только
// целыми — округление ±0,5 точки на тонкой линии давало «лесенку».
package engine

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"

	"github.com/oops1/headless-gui/v3/widget"
)

// fpt — точка в ФИЗИЧЕСКИХ координатах холста.
type fpt struct{ x, y float64 }

// joinMinSin — изгиб, начиная с которого вершине нужно сопряжение.
//
// На плотной кривой соседние отрезки почти сонаправлены: щель снаружи изгиба
// там — сотые доли точки, и диск в каждой из сотен вершин был бы работой ради
// невидимого. Порог — синус угла около 3°.
const joinMinSin = 0.05

// toPhys переводит логическую точку в физическую. +0.5 — та же договорённость,
// что у всех примитивов холста: целая координата — центр точки.
func (c *Canvas) toPhys(x, y float64) fpt {
	return fpt{(x + 0.5) * c.scale, (y + 0.5) * c.scale}
}

// strokePhys обводит ломаную шириной width (физические точки) одним путём:
// прямоугольник на каждый отрезок и диск в каждой изогнутой внутренней вершине
// (у замкнутой — во всех). Концы незамкнутой ломаной прямые, как и раньше, —
// круглые удлинили бы каждую уже нарисованную линию на полтолщины.
//
// Все подпути одной ориентации: растеризатор складывает покрытие со знаком, и
// диск, обходимый в другую сторону, чем прямоугольник, вырезал бы в стыке дыру.
// Прямоугольники ниже обходятся против часовой (в экранных координатах) —
// поэтому и диски берутся ellipsePathCCW.
func (c *Canvas) strokePhys(pts []fpt, width float64, closed bool, col color.RGBA) {
	if len(pts) < 2 || width <= 0 || col.A == 0 {
		return
	}
	half := width / 2
	minX, minY, maxX, maxY := pts[0].x, pts[0].y, pts[0].x, pts[0].y
	for _, p := range pts[1:] {
		minX, maxX = math.Min(minX, p.x), math.Max(maxX, p.x)
		minY, maxY = math.Min(minY, p.y), math.Max(maxY, p.y)
	}
	pad := half + 2
	bx, by := int(math.Floor(minX-pad)), int(math.Floor(minY-pad))
	bw, bh := int(math.Ceil(maxX+pad))-bx, int(math.Ceil(maxY+pad))-by
	h := float32(half)

	c.fillPath(bx, by, bw, bh, col, func(z *vector.Rasterizer, ox, oy float32) {
		n := len(pts)
		segs := n - 1
		if closed {
			segs = n
		}
		at := func(i int) (float32, float32) {
			p := pts[(i+n)%n]
			return float32(p.x) - ox, float32(p.y) - oy
		}
		for i := 0; i < segs; i++ {
			ax, ay := at(i)
			bx2, by2 := at(i + 1)
			dx, dy := bx2-ax, by2-ay
			l := float32(math.Hypot(float64(dx), float64(dy)))
			if l == 0 {
				continue
			}
			px, py := -dy/l*h, dx/l*h
			z.MoveTo(ax+px, ay+py)
			z.LineTo(bx2+px, by2+py)
			z.LineTo(bx2-px, by2-py)
			z.LineTo(ax-px, ay-py)
			z.ClosePath()
		}

		first, last := 1, n-1
		if closed {
			first, last = 0, n
		}
		for i := first; i < last; i++ {
			if !needsJoin(pts, i, closed) {
				continue
			}
			x, y := at(i)
			ellipsePathCCW(z, x, y, h, h, 0, 0)
			z.ClosePath()
		}
	})
}

// needsJoin — вершина i достаточно изогнута, чтобы щель снаружи была видна.
func needsJoin(pts []fpt, i int, closed bool) bool {
	n := len(pts)
	if !closed && (i <= 0 || i >= n-1) {
		return false
	}
	a, p, b := pts[(i-1+n)%n], pts[i], pts[(i+1)%n]
	ux, uy := p.x-a.x, p.y-a.y
	vx, vy := b.x-p.x, b.y-p.y
	lu, lv := math.Hypot(ux, uy), math.Hypot(vx, vy)
	if lu == 0 || lv == 0 {
		return true // вырожденный отрезок: направление неизвестно, сопрягаем
	}
	cross := (ux*vy - uy*vx) / (lu * lv)
	dot := (ux*vx + uy*vy) / (lu * lv)
	// Разворот (dot < 0) — щель есть при любом «синусе».
	return dot < 0 || math.Abs(cross) > joinMinSin
}

// StrokePathAA рисует сглаженную ломаную с дробными координатами (логические
// единицы; HiDPI учитывается здесь). Реализует widget.PathShapes.
func (c *Canvas) StrokePathAA(pts []widget.Point2F, thickness float64, closed bool, col color.RGBA) {
	if len(pts) < 2 || thickness <= 0 {
		return
	}
	phys := make([]fpt, len(pts))
	for i, p := range pts {
		phys[i] = c.toPhys(p.X, p.Y)
	}
	c.strokePhys(phys, thickness*c.scale, closed, col)
}

// StrokeCubicAA рисует сглаженную кубическую кривую Безье. Реализует
// widget.PathShapes: кривая раскладывается той же функцией, что и в откате для
// контекста без PathShapes, — на разных контекстах она выглядит одинаково.
func (c *Canvas) StrokeCubicAA(x0, y0, x1, y1, x2, y2, x3, y3, thickness float64, col color.RGBA) {
	c.StrokePathAA(widget.FlattenCubic(x0, y0, x1, y1, x2, y2, x3, y3), thickness, false, col)
}

// FillPathAA заливает многоугольник с дробными координатами. Реализует
// widget.PathShapes.
func (c *Canvas) FillPathAA(pts []widget.Point2F, col color.RGBA) {
	if len(pts) < 3 || col.A == 0 {
		return
	}
	phys := make([]fpt, len(pts))
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for i, p := range pts {
		q := c.toPhys(p.X, p.Y)
		phys[i] = q
		minX, maxX = math.Min(minX, q.x), math.Max(maxX, q.x)
		minY, maxY = math.Min(minY, q.y), math.Max(maxY, q.y)
	}
	bx, by := int(math.Floor(minX))-1, int(math.Floor(minY))-1
	bw, bh := int(math.Ceil(maxX))+1-bx, int(math.Ceil(maxY))+1-by
	c.fillPath(bx, by, bw, bh, col, func(z *vector.Rasterizer, ox, oy float32) {
		z.MoveTo(float32(phys[0].x)-ox, float32(phys[0].y)-oy)
		for _, p := range phys[1:] {
			z.LineTo(float32(p.x)-ox, float32(p.y)-oy)
		}
	})
}

// intPhys переводит целые логические точки в физические для strokePhys.
func (c *Canvas) intPhys(pts []image.Point) []fpt {
	out := make([]fpt, len(pts))
	for i, p := range pts {
		out[i] = c.toPhys(float64(p.X), float64(p.Y))
	}
	return out
}

// Проверка на этапе сборки: холст реализует дробные пути.
var _ widget.PathShapes = (*Canvas)(nil)
