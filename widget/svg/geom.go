// Package svg реализует парсер ПОДМНОЖЕСТВА SVG и растеризацию векторных
// иконок поверх golang.org/x/image/vector.
//
// Назначение — темизируемые монохромные иконки (Material-подобные): парсер
// строит плоский список контуров (полилиний в float64) с цветами, а
// растеризатор превращает их в *image.RGBA заданного размера и цвета
// (см. Document.Rasterize / RasterizeCached). Виджет widget.SVGIcon рисует
// полученный RGBA через DrawContext.DrawImage.
//
// # Поддерживаемое подмножество SVG
//
//   - <svg>: viewBox="minX minY W H", width/height (как fallback viewBox);
//   - контейнеры <g>, <a>, <switch> с наследованием свойств;
//   - <path> d: команды M m L l H h V v C c S s Q q T t A a Z z
//     (эллиптические дуги приближаются кубическими Безье);
//   - <rect> (в т.ч. rx/ry — скругление), <circle>, <ellipse>, <line>,
//     <polyline>, <polygon>;
//   - transform: translate, scale, rotate(a[,cx,cy]), matrix, skewX, skewY;
//   - fill: #rgb/#rgba/#rrggbb/#rrggbbaa, rgb()/rgba(), именованные цвета,
//     none, currentColor; fill-rule: nonzero | evenodd; fill-opacity;
//   - базовая stroke: сплошной цвет, stroke-width, stroke-opacity;
//   - presentation-атрибуты и style="fill:...;..." (style приоритетнее);
//   - групповая opacity (приближённо — умножается в альфу заливки/обводки).
//
// # Ограничения
//
//   - stroke рисуется квадами по сегментам, БЕЗ линейных стыков (join),
//     скруглённых/квадратных капов, штриховки (dash) и masking. Пригодно для
//     тонких иконочных линий; острые углы могут иметь микрозазор;
//   - нет gradient/pattern/clipPath/mask/filter/text/image/use/symbol,
//     CSS-классов и внешних стилей, единиц кроме px, процентных координат;
//   - контуры хранятся уже сплющенными (полилинии); экстремальное увеличение
//     (>~30× от единиц viewBox) может дать лёгкую огранку кривых;
//   - even-odd объединяет покрытия XOR-формулой попиксельно — корректно для
//     контуров, чьи рёбра не пересекают один и тот же пиксель (типичные
//     кольца/дырки), возможны артефакты на самопересекающихся путях.
package svg

import "math"

// Point — точка в координатах пользователя SVG (система viewBox), float64.
type Point struct{ X, Y float64 }

// Matrix — аффинное преобразование 2×3 (как SVG transform-matrix).
// Отображение точки:
//
//	x' = A*x + C*y + E
//	y' = B*x + D*y + F
type Matrix struct{ A, B, C, D, E, F float64 }

// Identity возвращает единичную матрицу.
func Identity() Matrix { return Matrix{A: 1, B: 0, C: 0, D: 1, E: 0, F: 0} }

// Apply применяет преобразование к точке.
func (m Matrix) Apply(p Point) Point {
	return Point{X: m.A*p.X + m.C*p.Y + m.E, Y: m.B*p.X + m.D*p.Y + m.F}
}

// ApplyVec применяет только линейную часть (без сдвига) — для векторов/дельт.
func (m Matrix) ApplyVec(dx, dy float64) (float64, float64) {
	return m.A*dx + m.C*dy, m.B*dx + m.D*dy
}

// Mul возвращает композицию m∘n: (m.Mul(n)).Apply(p) == m.Apply(n.Apply(p)).
// Используется для накопления вложенных transform (родитель.Mul(потомок)).
func (m Matrix) Mul(n Matrix) Matrix {
	return Matrix{
		A: m.A*n.A + m.C*n.B,
		B: m.B*n.A + m.D*n.B,
		C: m.A*n.C + m.C*n.D,
		D: m.B*n.C + m.D*n.D,
		E: m.A*n.E + m.C*n.F + m.E,
		F: m.B*n.E + m.D*n.F + m.F,
	}
}

// Det возвращает детерминант линейной части (масштаб площади).
func (m Matrix) Det() float64 { return m.A*m.D - m.B*m.C }

// AvgScale возвращает средний масштаб длины (√|det|) — для пересчёта толщины
// обводки при неравномерном/повёрнутом преобразовании (приближённо).
func (m Matrix) AvgScale() float64 {
	d := math.Abs(m.Det())
	if d == 0 {
		return 0
	}
	return math.Sqrt(d)
}

// Translate — матрица сдвига.
func Translate(tx, ty float64) Matrix { return Matrix{A: 1, D: 1, E: tx, F: ty} }

// ScaleM — матрица масштабирования.
func ScaleM(sx, sy float64) Matrix { return Matrix{A: sx, D: sy} }

// RotateDeg — матрица поворота вокруг начала координат (угол в градусах).
func RotateDeg(deg float64) Matrix {
	r := deg * math.Pi / 180
	s, c := math.Sin(r), math.Cos(r)
	return Matrix{A: c, B: s, C: -s, D: c}
}

// RotateAboutDeg — поворот вокруг точки (cx, cy).
func RotateAboutDeg(deg, cx, cy float64) Matrix {
	return Translate(cx, cy).Mul(RotateDeg(deg)).Mul(Translate(-cx, -cy))
}
