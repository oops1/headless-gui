package svg

import (
	"image"
	"math"
	"strings"

	"golang.org/x/image/vector"
)

// LineJoin — stroke-linejoin: как сходятся два сегмента обводки.
type LineJoin uint8

const (
	// JoinMiter — острый угол (по умолчанию); срезается, если длиннее
	// stroke-miterlimit.
	JoinMiter LineJoin = iota
	// JoinRound — скруглённый стык.
	JoinRound
	// JoinBevel — срезанный стык.
	JoinBevel
)

// LineCap — stroke-linecap: что рисуется на концах незамкнутой линии.
type LineCap uint8

const (
	// CapButt — обрыв ровно в конце (по умолчанию).
	CapButt LineCap = iota
	// CapRound — полукруг радиуса в половину толщины.
	CapRound
	// CapSquare — продолжение на половину толщины.
	CapSquare
)

// defaultMiterLimit — stroke-miterlimit по умолчанию (SVG).
const defaultMiterLimit = 4

// maxDashPieces — предел числа штрихов одной линии: пунктир мельче пикселя на
// длинном контуре превратил бы маленький файл в миллионы многоугольников.
const maxDashPieces = 100000

// fpt — точка в пикселях устройства.
type fpt struct{ x, y float64 }

// strokeParams — параметры обводки в пикселях устройства.
type strokeParams struct {
	half   float64 // половина толщины
	join   LineJoin
	cap    LineCap
	miter  float64
	dash   []float64
	dashAt float64
}

// stroker складывает контуры обводки (замкнутые многоугольники одного
// направления обхода) в растеризатор. Одинаковое направление обязательно:
// vector.Rasterizer считает покрытие знаковой суммой, и встречные
// многоугольники вычли бы друг друга там, где обводка должна быть сплошной.
type stroker struct {
	z   *vector.Rasterizer
	p   strokeParams
	buf []fpt // рабочий многоугольник
}

// rasterStrokeJoins — обводка по-настоящему: стыки, концы, пунктир. Толщина
// sw — в пикселях устройства; mapPt переводит точку viewBox в пиксели.
func rasterStrokeJoins(w, h int, contours []Contour, p strokeParams, mapPt func(Point) (float32, float32)) *image.Alpha {
	st := &stroker{z: vector.NewRasterizer(w, h), p: p}
	if p.miter < 1 {
		st.p.miter = defaultMiterLimit
	}
	var pts []fpt
	for _, c := range contours {
		pts = pts[:0]
		for _, q := range c.Points {
			x, y := mapPt(q)
			pts = append(pts, fpt{float64(x), float64(y)})
		}
		st.contour(pts, c.Closed)
	}
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	st.z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// contour обводит один контур: сначала пунктир (если задан), затем стык/концы.
func (s *stroker) contour(pts []fpt, closed bool) {
	pts = dedupe(pts, closed)
	if len(pts) == 0 {
		return
	}
	if len(s.p.dash) == 0 {
		s.polyline(pts, closed)
		return
	}
	pieces := dashPolyline(pts, closed, s.p.dash, s.p.dashAt)
	if pieces == nil { // слишком мелкий пунктир — рисуем сплошной линией
		s.polyline(pts, closed)
		return
	}
	for _, pc := range pieces {
		s.polyline(dedupe(pc, false), false)
	}
}

// dedupe убирает подряд идущие совпадающие точки (и замыкающую, равную
// первой): нулевые сегменты не имеют направления.
func dedupe(pts []fpt, closed bool) []fpt {
	const eps = 1e-7
	out := make([]fpt, 0, len(pts))
	for _, p := range pts {
		if n := len(out); n > 0 && math.Abs(p.x-out[n-1].x) < eps && math.Abs(p.y-out[n-1].y) < eps {
			continue
		}
		out = append(out, p)
	}
	if closed && len(out) > 1 {
		a, b := out[0], out[len(out)-1]
		if math.Abs(a.x-b.x) < eps && math.Abs(a.y-b.y) < eps {
			out = out[:len(out)-1]
		}
	}
	return out
}

// polyline обводит ломаную без пунктира.
func (s *stroker) polyline(pts []fpt, closed bool) {
	n := len(pts)
	switch {
	case n == 1:
		s.dot(pts[0])
	case closed && n >= 2:
		s.emit(s.leftChain(pts, true))
		rev := make([]fpt, n)
		for i, p := range pts {
			rev[n-1-i] = p
		}
		s.emit(s.leftChain(rev, true))
	case n >= 2:
		fwd := s.leftChain(pts, false)
		rev := make([]fpt, n)
		for i, p := range pts {
			rev[n-1-i] = p
		}
		back := s.leftChain(rev, false)
		poly := make([]fpt, 0, len(fwd)+len(back)+8)
		poly = append(poly, fwd...)
		d := unit(pts[n-1].x-pts[n-2].x, pts[n-1].y-pts[n-2].y)
		poly = s.capPoints(poly, pts[n-1], d)
		poly = append(poly, back...)
		d0 := unit(pts[0].x-pts[1].x, pts[0].y-pts[1].y)
		poly = s.capPoints(poly, pts[0], d0)
		s.emit(poly)
	}
}

// dot — вырожденный подконтур из одной точки: видим только при круглых и
// квадратных концах (как в SVG). Обход — того же направления, что у остальных
// многоугольников обводки (см. emit).
func (s *stroker) dot(p fpt) {
	h := s.p.half
	switch s.p.cap {
	case CapRound:
		steps := arcSteps(2*math.Pi, h)
		if steps < 8 {
			steps = 8
		}
		poly := make([]fpt, 0, steps)
		for i := 0; i < steps; i++ {
			a := -2 * math.Pi * float64(i) / float64(steps)
			poly = append(poly, fpt{p.x + math.Cos(a)*h, p.y + math.Sin(a)*h})
		}
		s.emit(poly)
	case CapSquare:
		s.emit([]fpt{{p.x - h, p.y - h}, {p.x - h, p.y + h}, {p.x + h, p.y + h}, {p.x + h, p.y - h}})
	}
}

// emit отдаёт многоугольник растеризатору как есть. Направление обхода у всех
// многоугольников обводки одно (отрицательная площадь): по левой стороне
// вперёд, по правой назад; у замкнутого контура внешнее кольцо и внутреннее
// идут навстречу, поэтому внутри остаётся дыра. Нормализовать направление по
// знаку площади нельзя — дыра заполнилась бы.
func (s *stroker) emit(poly []fpt) {
	if len(poly) < 3 {
		return
	}
	s.z.MoveTo(float32(poly[0].x), float32(poly[0].y))
	for _, p := range poly[1:] {
		s.z.LineTo(float32(p.x), float32(p.y))
	}
	s.z.ClosePath()
}

// unit — единичный вектор (нулевой остаётся нулевым).
func unit(dx, dy float64) fpt {
	l := math.Hypot(dx, dy)
	if l == 0 {
		return fpt{}
	}
	return fpt{dx / l, dy / l}
}

// leftNormal — нормаль слева от направления d (поворот на +90°).
func leftNormal(d fpt) fpt { return fpt{-d.y, d.x} }

// leftChain — смещение ломаной на half влево с соединениями на вершинах.
// Для замкнутой ломаной цепочка замкнута (стык есть и на первой вершине).
func (s *stroker) leftChain(pts []fpt, closed bool) []fpt {
	n := len(pts)
	h := s.p.half
	segs := n - 1
	if closed {
		segs = n
	}
	dirs := make([]fpt, segs)
	lens := make([]float64, segs)
	for i := 0; i < segs; i++ {
		a, b := pts[i], pts[(i+1)%n]
		lens[i] = math.Hypot(b.x-a.x, b.y-a.y)
		dirs[i] = unit(b.x-a.x, b.y-a.y)
	}
	out := make([]fpt, 0, n*2+4)
	if !closed {
		l0 := leftNormal(dirs[0])
		out = append(out, fpt{pts[0].x + l0.x*h, pts[0].y + l0.y*h})
		for i := 1; i < n-1; i++ {
			out = s.join(out, pts[i], dirs[i-1], dirs[i], lens[i-1], lens[i])
		}
		l1 := leftNormal(dirs[segs-1])
		out = append(out, fpt{pts[n-1].x + l1.x*h, pts[n-1].y + l1.y*h})
		return out
	}
	for i := 0; i < n; i++ {
		prev := (i + segs - 1) % segs
		out = s.join(out, pts[i], dirs[prev], dirs[i], lens[prev], lens[i])
	}
	return out
}

// join добавляет точки левого смещения в вершине p между сегментами с
// направлениями d0 → d1 и длинами l0, l1.
func (s *stroker) join(out []fpt, p, d0, d1 fpt, l0, l1 float64) []fpt {
	h := s.p.half
	n0 := leftNormal(d0)
	n1 := leftNormal(d1)
	cross := d0.x*d1.y - d0.y*d1.x
	dot := d0.x*d1.x + d0.y*d1.y
	pt := func(n fpt) fpt { return fpt{p.x + n.x*h, p.y + n.y*h} }

	// Почти прямо: одной точки достаточно (кривые разбиты на мелкие отрезки,
	// и таких вершин большинство).
	if math.Abs(cross) < 1e-9 && dot > 0 {
		return append(out, pt(n0))
	}
	// miter-вектор: (n0+n1)/(1+dot) — до пересечения смещённых прямых.
	mv := func() fpt {
		k := 1 / (1 + dot)
		return fpt{(n0.x + n1.x) * k, (n0.y + n1.y) * k}
	}
	if cross > 0 {
		// Левая сторона внутренняя. Смещённые прямые пересекаются в одной
		// точке, если она лежит на обоих сегментах; иначе идём через вершину —
		// петля лишь утолщает обводку внутри самого контура.
		if t := h * math.Abs(cross) / (1 + dot); t <= l0 && t <= l1 {
			return append(out, pt(mv()))
		}
		return append(out, pt(n0), p, pt(n1))
	}
	// Левая сторона внешняя.
	switch s.p.join {
	case JoinRound:
		phi := math.Atan2(cross, dot)
		if cross == 0 {
			phi = -math.Pi
		}
		steps := arcSteps(math.Abs(phi), h)
		a0 := math.Atan2(n0.y, n0.x)
		out = append(out, pt(n0))
		for i := 1; i < steps; i++ {
			a := a0 + phi*float64(i)/float64(steps)
			out = append(out, fpt{p.x + math.Cos(a)*h, p.y + math.Sin(a)*h})
		}
		return append(out, pt(n1))
	case JoinMiter:
		// длина среза относится к толщине как 1/sin(φ/2), φ — угол между
		// сегментами; через 1+dot это sqrt(2/(1+dot)).
		if 1+dot > 1e-12 && 2 <= s.p.miter*s.p.miter*(1+dot) {
			return append(out, pt(mv()))
		}
	}
	return append(out, pt(n0), pt(n1)) // bevel (и miter сверх лимита)
}

// arcSteps — число отрезков для дуги в angle радиан радиуса r пикселей так,
// чтобы стрелка прогиба не превышала ~0,05 px.
func arcSteps(angle, r float64) int {
	if r < 0.05 {
		return 1
	}
	step := 2 * math.Acos(1-math.Min(0.05/r, 0.5))
	n := int(math.Ceil(angle / step))
	if n < 1 {
		n = 1
	}
	if n > 256 {
		n = 256
	}
	return n
}

// capPoints добавляет в poly точки конца линии в точке p (направление d —
// наружу от линии). Вызывать надо, когда последняя точка poly — смещение p на
// +half слева от d; цепочка продолжится смещением на −half.
func (s *stroker) capPoints(poly []fpt, p, d fpt) []fpt {
	h := s.p.half
	l := leftNormal(d)
	switch s.p.cap {
	case CapSquare:
		poly = append(poly,
			fpt{p.x + (l.x+d.x)*h, p.y + (l.y+d.y)*h},
			fpt{p.x + (-l.x+d.x)*h, p.y + (-l.y+d.y)*h})
	case CapRound:
		steps := arcSteps(math.Pi, h)
		a0 := math.Atan2(l.y, l.x)
		for i := 1; i < steps; i++ {
			a := a0 - math.Pi*float64(i)/float64(steps)
			poly = append(poly, fpt{p.x + math.Cos(a)*h, p.y + math.Sin(a)*h})
		}
	}
	return poly
}

// dashPolyline режет ломаную по пунктиру pattern (длины штрих/пробел, уже в
// пикселях; нечётный список здесь уже удвоен) со смещением offset. nil — штрихов
// слишком много. У замкнутой линии штрих, идущий через начальную вершину,
// склеивается в один (со стыком, а не с двумя концами).
func dashPolyline(pts []fpt, closed bool, pattern []float64, offset float64) [][]fpt {
	total := 0.0
	for _, v := range pattern {
		total += v
	}
	if !(total > 0) || math.IsInf(total, 0) {
		return nil
	}
	path := pts
	if closed {
		path = append(append([]fpt(nil), pts...), pts[0])
	}
	var plen float64
	for i := 1; i < len(path); i++ {
		plen += math.Hypot(path[i].x-path[i-1].x, path[i].y-path[i-1].y)
	}
	if plen/total*float64(len(pattern)) > maxDashPieces {
		return nil
	}

	// Начальное положение в узоре.
	off := math.Mod(offset, total)
	if off < 0 {
		off += total
	}
	idx := 0
	for off > 0 && off >= pattern[idx] {
		off -= pattern[idx]
		idx = (idx + 1) % len(pattern)
	}
	remain := pattern[idx] - off // сколько осталось до конца текущего элемента
	on := idx%2 == 0
	startsOn := on

	var pieces [][]fpt
	var cur []fpt
	if on {
		cur = []fpt{path[0]}
	}
	flush := func(end fpt, dir fpt) {
		if len(cur) == 0 {
			return
		}
		if len(cur) == 1 && cur[0] == end {
			end = fpt{end.x + dir.x*1e-4, end.y + dir.y*1e-4} // штрих нулевой длины
		}
		cur = append(cur, end)
		pieces = append(pieces, cur)
		cur = nil
	}
	for i := 1; i < len(path); i++ {
		a, b := path[i-1], path[i]
		segLen := math.Hypot(b.x-a.x, b.y-a.y)
		if segLen == 0 {
			continue
		}
		dir := fpt{(b.x - a.x) / segLen, (b.y - a.y) / segLen}
		pos := 0.0
		for segLen-pos > remain {
			pos += remain
			p := fpt{a.x + dir.x*pos, a.y + dir.y*pos}
			if on {
				flush(p, dir)
			} else {
				cur = []fpt{p}
			}
			on = !on
			idx = (idx + 1) % len(pattern)
			remain = pattern[idx]
			if len(pieces) > maxDashPieces {
				return nil
			}
		}
		remain -= segLen - pos
		if on {
			cur = append(cur, b)
		}
	}
	last := path[len(path)-1]
	if on && len(cur) > 0 {
		switch {
		case closed && startsOn && len(pieces) == 0:
			return nil // весь контур — один штрих: обводим сплошным кольцом
		case closed && startsOn:
			// Хвост продолжает первый штрих через начальную вершину.
			pieces[0] = append(cur, pieces[0][1:]...)
		default:
			cur = append(cur[:len(cur):len(cur)], last)
			if len(cur) > 1 {
				pieces = append(pieces, cur)
			}
		}
	}
	return pieces
}

// parseDashArray разбирает stroke-dasharray: «none» и пустое значение — нет
// пунктира; отрицательные значения, нулевая сумма — ошибка (пунктира нет);
// нечётное число значений повторяется дважды, как требует SVG.
func parseDashArray(v string) []float64 {
	v = strings.TrimSpace(v)
	if v == "" || strings.EqualFold(v, "none") {
		return nil
	}
	vals := parseLengthList(v)
	if len(vals) == 0 {
		return nil
	}
	sum := 0.0
	for _, f := range vals {
		if f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		sum += f
	}
	if sum <= 0 {
		return nil
	}
	if len(vals)%2 == 1 {
		vals = append(vals[:len(vals):len(vals)], vals...)
	}
	return vals
}

// parseLengthList — числа через запятую/пробел; единицы px и % отбрасываются
// (как в parseLength).
func parseLengthList(s string) []float64 {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		out = append(out, parseLength(f))
	}
	return out
}

func parseLineJoin(v string) (LineJoin, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "miter", "miter-clip", "arcs":
		return JoinMiter, true
	case "round":
		return JoinRound, true
	case "bevel":
		return JoinBevel, true
	}
	return JoinMiter, false
}

func parseLineCap(v string) (LineCap, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "butt":
		return CapButt, true
	case "round":
		return CapRound, true
	case "square":
		return CapSquare, true
	}
	return CapButt, false
}
