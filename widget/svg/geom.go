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
//   - групповая opacity (приближённо — умножается в альфу заливки/обводки);
//   - градиенты: fill/stroke="url(#id) [запасной цвет]" на <linearGradient> и
//     <radialGradient> — x1/y1/x2/y2, cx/cy/r/fx/fy, gradientUnits
//     (objectBoundingBox по умолчанию и userSpaceOnUse), gradientTransform,
//     spreadMethod (pad/reflect/repeat), <stop offset stop-color stop-opacity>
//     (атрибутами, через style или CSS-класс; stop-color=currentColor),
//     наследование стопов и атрибутов по цепочке xlink:href/href. Ссылка на
//     отсутствующий объект даёт запасной цвет, а без него — «не рисовать»
//     (как в браузерах), но не чёрный. Градиент из одного стопа — сплошной
//     цвет; Shape.Fill для градиентной фигуры хранит средний цвет;
//   - <defs>, <symbol>, <clipPath>, <mask>, <pattern>, <marker>, <filter>,
//     <style>, <title>, <desc>, <metadata> напрямую не рисуются; чужие
//     пространства имён (inkscape:, sodipodi:) пропускаются;
//   - <use> (href/xlink:href на фигуру, группу, <symbol> или вложенный <svg>;
//     x, y, width, height, transform; viewBox и preserveAspectRatio символа).
//     Циклы ссылок отсекаются, раскрытие ограничено (maxUseVisits,
//     maxUsePoints);
//   - вложенный <svg> со своими x/y/width/height/viewBox;
//   - clip-path="url(#id)" (clipPathUnits, clip-rule, вложенные clip-path,
//     clip-path на самом clipPath; границы сглажены, покрытия перемножаются);
//   - mask="url(#id)" (maskUnits, maskContentUnits, область x/y/width/height,
//     яркость×альфа содержимого, mask-type:alpha; маска сама может быть
//     градиентной);
//   - display:none и visibility:hidden|collapse (атрибутом, в style и в CSS);
//   - <style>: селекторы тега, .класса, #id и их сочетания (rect.st0, g#a),
//     «*», списки через запятую; приоритет — атрибут < таблица стилей < style="";
//   - <image> с data:-ссылкой на PNG/JPEG/GIF (preserveAspectRatio, transform,
//     opacity; сильное уменьшение усредняется по блокам);
//   - filter="url(#id)": feGaussianBlur (размытие покрытия фигуры) и
//     feColorMatrix (matrix/saturate/hueRotate/luminanceToAlpha над цветом
//     фигуры); остальные примитивы игнорируются, элемент рисуется без них.
//     Фильтр группы применяется к каждой её фигуре отдельно;
//   - все именованные цвета CSS.
//
// # Ограничения
//
//   - stroke рисуется квадами по сегментам, БЕЗ линейных стыков (join),
//     скруглённых/квадратных капов, штриховки (dash) и masking. Пригодно для
//     тонких иконочных линий; острые углы могут иметь микрозазор;
//   - нет <text> (его не рисуем; маска из текста даст сплошную фигуру),
//     <pattern> (fill="url(#pattern)" берёт запасной цвет или не рисуется),
//     внешних картинок и вложенных SVG в <image>, внешних таблиц стилей и
//     сложных CSS-селекторов (потомки, дочерние, атрибутные, псевдоклассы),
//     marker, единиц кроме px и процентов в градиентах/масках;
//   - групповая opacity не изолирует группу (перекрывающиеся потомки
//     просвечивают друг через друга); filter/mask/clip группы действуют на
//     каждую её фигуру отдельно, а не на склеенную картинку группы;
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

// inverse возвращает обратную матрицу; ok=false для вырожденной.
func (m Matrix) inverse() (Matrix, bool) {
	det := m.Det()
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return Matrix{}, false
	}
	id := 1 / det
	return Matrix{
		A: m.D * id,
		B: -m.B * id,
		C: -m.C * id,
		D: m.A * id,
		E: (m.C*m.F - m.D*m.E) * id,
		F: (m.B*m.E - m.A*m.F) * id,
	}, true
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
