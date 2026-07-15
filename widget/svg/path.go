package svg

import (
	"math"
	"strconv"
)

// Contour — один контур (subpath) как плоская полилиния в float64.
// Closed=true — контур замкнут (fill замыкает всегда; флаг важен для stroke).
type Contour struct {
	Points []Point
	Closed bool
}

// flattenTol — допуск аппроксимации кривых прямыми (в единицах пользователя
// SVG, до применения масштаба viewBox→пиксели). Мелкий, чтобы иконки 16..256px
// выглядели гладко. Очень сильное увеличение (>~30×) может дать лёгкую огранку.
const flattenTol = 0.03

// ─── Тонкий разбор чисел из атрибута d ──────────────────────────────────────

// dScanner — потоковый разбор строки d: команды (буквы) и числа.
type dScanner struct {
	s   string
	pos int
}

func (sc *dScanner) skipSep() {
	for sc.pos < len(sc.s) {
		c := sc.s[sc.pos]
		if c == ' ' || c == ',' || c == '\t' || c == '\n' || c == '\r' {
			sc.pos++
			continue
		}
		break
	}
}

// nextCmd возвращает следующую командную букву (или 0, если далее число/конец).
func (sc *dScanner) nextCmd() byte {
	sc.skipSep()
	if sc.pos >= len(sc.s) {
		return 0
	}
	c := sc.s[sc.pos]
	if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
		sc.pos++
		return c
	}
	return 0
}

// nextNum читает следующее число (float). ok=false — чисел больше нет.
func (sc *dScanner) nextNum() (float64, bool) {
	sc.skipSep()
	start := sc.pos
	n := len(sc.s)
	if sc.pos < n && (sc.s[sc.pos] == '+' || sc.s[sc.pos] == '-') {
		sc.pos++
	}
	seenDigit := false
	seenDot := false
	for sc.pos < n {
		c := sc.s[sc.pos]
		switch {
		case c >= '0' && c <= '9':
			seenDigit = true
			sc.pos++
		case c == '.' && !seenDot:
			seenDot = true
			sc.pos++
		case (c == 'e' || c == 'E') && seenDigit:
			// экспонента
			sc.pos++
			if sc.pos < n && (sc.s[sc.pos] == '+' || sc.s[sc.pos] == '-') {
				sc.pos++
			}
		default:
			goto done
		}
	}
done:
	if !seenDigit {
		sc.pos = start
		return 0, false
	}
	f, err := strconv.ParseFloat(sc.s[start:sc.pos], 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// nextFlag читает флаг дуги (одна цифра 0/1, без разделителя обязательного).
func (sc *dScanner) nextFlag() (bool, bool) {
	sc.skipSep()
	if sc.pos >= len(sc.s) {
		return false, false
	}
	c := sc.s[sc.pos]
	if c == '0' {
		sc.pos++
		return false, true
	}
	if c == '1' {
		sc.pos++
		return true, true
	}
	return false, false
}

// ─── Парсер d → контуры ─────────────────────────────────────────────────────

// pathBuilder накапливает контуры, аппроксимируя кривые прямыми.
type pathBuilder struct {
	contours []Contour
	cur      []Point
	closed   bool
	started  bool

	startX, startY float64 // начало текущего subpath
	curX, curY     float64 // текущая точка
}

func (b *pathBuilder) moveTo(x, y float64) {
	b.flush()
	b.cur = []Point{{X: x, Y: y}}
	b.closed = false
	b.started = true
	b.startX, b.startY = x, y
	b.curX, b.curY = x, y
}

func (b *pathBuilder) lineTo(x, y float64) {
	if !b.started {
		b.moveTo(x, y)
		return
	}
	b.cur = append(b.cur, Point{X: x, Y: y})
	b.curX, b.curY = x, y
}

func (b *pathBuilder) cubicTo(x1, y1, x2, y2, x, y float64) {
	if !b.started {
		b.moveTo(b.curX, b.curY)
	}
	p0 := Point{b.curX, b.curY}
	flattenCubic(p0, Point{x1, y1}, Point{x2, y2}, Point{x, y}, flattenTol, &b.cur, 0)
	b.curX, b.curY = x, y
}

func (b *pathBuilder) quadTo(x1, y1, x, y float64) {
	// Квадратичная → кубическая.
	p0 := Point{b.curX, b.curY}
	c1 := Point{p0.X + 2.0/3.0*(x1-p0.X), p0.Y + 2.0/3.0*(y1-p0.Y)}
	c2 := Point{x + 2.0/3.0*(x1-x), y + 2.0/3.0*(y1-y)}
	b.cubicTo(c1.X, c1.Y, c2.X, c2.Y, x, y)
}

func (b *pathBuilder) close() {
	if !b.started {
		return
	}
	b.closed = true
	b.flush()
	// Следующий subpath (без явного M) начинается из точки закрытия.
	b.curX, b.curY = b.startX, b.startY
	b.started = false
}

func (b *pathBuilder) flush() {
	if len(b.cur) >= 2 {
		b.contours = append(b.contours, Contour{Points: b.cur, Closed: b.closed})
	}
	b.cur = nil
	b.closed = false
}

// ParsePathData разбирает атрибут d в набор контуров.
// Поддержаны команды: M m L l H h V v C c S s Q q T t A a Z z.
func ParsePathData(d string) []Contour {
	sc := &dScanner{s: d}
	b := &pathBuilder{}

	var (
		prevCmd  byte
		lastCX   float64 // контрольная точка предыдущего C/S для S
		lastCY   float64
		lastQX   float64 // контрольная точка предыдущего Q/T для T
		lastQY   float64
		hadCubic bool
		hadQuad  bool
	)

	readXY := func(rel bool) (float64, float64, bool) {
		x, ok1 := sc.nextNum()
		y, ok2 := sc.nextNum()
		if !ok1 || !ok2 {
			return 0, 0, false
		}
		if rel {
			x += b.curX
			y += b.curY
		}
		return x, y, true
	}

	for {
		cmd := sc.nextCmd()
		if cmd == 0 {
			// Нет буквы: либо конец, либо продолжение предыдущей команды
			// с новым набором чисел (implicit repeat). hasNumberNext не
			// потребляет число — его прочитает обработчик команды.
			if !hasNumberNext(sc) {
				break
			}
			cmd = implicitRepeat(prevCmd)
			if cmd == 0 {
				break
			}
		}

		switch cmd {
		case 'M', 'm':
			rel := cmd == 'm'
			x, y, ok := readXY(rel)
			if !ok {
				break
			}
			b.moveTo(x, y)
			// Последующие пары в M трактуются как L.
			for {
				sc.skipSep()
				if !hasNumberNext(sc) {
					break
				}
				lx, ly, ok := readXY(rel)
				if !ok {
					break
				}
				b.lineTo(lx, ly)
			}
			hadCubic, hadQuad = false, false
		case 'L', 'l':
			rel := cmd == 'l'
			for {
				if !hasNumberNext(sc) {
					break
				}
				x, y, ok := readXY(rel)
				if !ok {
					break
				}
				b.lineTo(x, y)
			}
			hadCubic, hadQuad = false, false
		case 'H', 'h':
			rel := cmd == 'h'
			for {
				x, ok := sc.nextNum()
				if !ok {
					break
				}
				if rel {
					x += b.curX
				}
				b.lineTo(x, b.curY)
			}
			hadCubic, hadQuad = false, false
		case 'V', 'v':
			rel := cmd == 'v'
			for {
				y, ok := sc.nextNum()
				if !ok {
					break
				}
				if rel {
					y += b.curY
				}
				b.lineTo(b.curX, y)
			}
			hadCubic, hadQuad = false, false
		case 'C', 'c':
			rel := cmd == 'c'
			for {
				x1, y1, ok := readXY(rel)
				if !ok {
					break
				}
				x2, y2, ok2 := readXY(rel)
				x, y, ok3 := readXY(rel)
				if !ok2 || !ok3 {
					break
				}
				b.cubicTo(x1, y1, x2, y2, x, y)
				lastCX, lastCY = x2, y2
				hadCubic, hadQuad = true, false
			}
		case 'S', 's':
			rel := cmd == 's'
			for {
				x2, y2, ok := readXY(rel)
				if !ok {
					break
				}
				x, y, ok3 := readXY(rel)
				if !ok3 {
					break
				}
				// Первая контрольная точка = отражение предыдущей.
				x1, y1 := b.curX, b.curY
				if hadCubic {
					x1 = 2*b.curX - lastCX
					y1 = 2*b.curY - lastCY
				}
				b.cubicTo(x1, y1, x2, y2, x, y)
				lastCX, lastCY = x2, y2
				hadCubic, hadQuad = true, false
			}
		case 'Q', 'q':
			rel := cmd == 'q'
			for {
				x1, y1, ok := readXY(rel)
				if !ok {
					break
				}
				x, y, ok2 := readXY(rel)
				if !ok2 {
					break
				}
				b.quadTo(x1, y1, x, y)
				lastQX, lastQY = x1, y1
				hadQuad, hadCubic = true, false
			}
		case 'T', 't':
			rel := cmd == 't'
			for {
				x, y, ok := readXY(rel)
				if !ok {
					break
				}
				x1, y1 := b.curX, b.curY
				if hadQuad {
					x1 = 2*b.curX - lastQX
					y1 = 2*b.curY - lastQY
				}
				b.quadTo(x1, y1, x, y)
				lastQX, lastQY = x1, y1
				hadQuad, hadCubic = true, false
			}
		case 'A', 'a':
			rel := cmd == 'a'
			for {
				rx, ok1 := sc.nextNum()
				ry, ok2 := sc.nextNum()
				rot, ok3 := sc.nextNum()
				large, ok4 := sc.nextFlag()
				sweep, ok5 := sc.nextFlag()
				ex, ok6 := sc.nextNum()
				ey, ok7 := sc.nextNum()
				if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6 && ok7) {
					break
				}
				if rel {
					ex += b.curX
					ey += b.curY
				}
				arcTo(b, rx, ry, rot, large, sweep, ex, ey)
				hadCubic, hadQuad = false, false
			}
		case 'Z', 'z':
			b.close()
			hadCubic, hadQuad = false, false
		default:
			// Неизвестная команда — прекратить (безопасно).
			b.flush()
			return b.contours
		}
		prevCmd = cmd
	}

	b.flush()
	return b.contours
}

// hasNumberNext сообщает, идёт ли далее число (не команда/конец).
func hasNumberNext(sc *dScanner) bool {
	save := sc.pos
	_, ok := sc.nextNum()
	sc.pos = save
	return ok
}

// implicitRepeat возвращает команду для неявного повтора координатных наборов.
// После M — L; после m — l; остальные повторяют себя.
func implicitRepeat(prev byte) byte {
	switch prev {
	case 'M':
		return 'L'
	case 'm':
		return 'l'
	case 0:
		return 0
	default:
		return prev
	}
}

// ─── Аппроксимация кривых ────────────────────────────────────────────────────

// flattenCubic рекурсивно разбивает кубическую кривую Безье на отрезки.
// Первая точка (p0) уже добавлена вызывающим; функция добавляет промежуточные
// и конечную точки.
func flattenCubic(p0, p1, p2, p3 Point, tol float64, out *[]Point, depth int) {
	if depth > 20 {
		*out = append(*out, p3)
		return
	}
	// Мера «плоскости»: отклонение контрольных точек от хорды p0→p3.
	d1 := distToLine(p1, p0, p3)
	d2 := distToLine(p2, p0, p3)
	if d1+d2 <= tol {
		*out = append(*out, p3)
		return
	}
	// Деление в середине (де Кастельжо, t=0.5).
	p01 := mid(p0, p1)
	p12 := mid(p1, p2)
	p23 := mid(p2, p3)
	p012 := mid(p01, p12)
	p123 := mid(p12, p23)
	pm := mid(p012, p123)
	flattenCubic(p0, p01, p012, pm, tol, out, depth+1)
	flattenCubic(pm, p123, p23, p3, tol, out, depth+1)
}

func mid(a, b Point) Point { return Point{(a.X + b.X) / 2, (a.Y + b.Y) / 2} }

// distToLine — расстояние от точки p до прямой через a,b.
func distToLine(p, a, b Point) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	// |cross((p-a),(b-a))| / |b-a|
	cross := (p.X-a.X)*dy - (p.Y-a.Y)*dx
	return math.Abs(cross) / math.Sqrt(l2)
}

// arcTo аппроксимирует эллиптическую дугу SVG кубическими Безье и добавляет их
// в builder. Конечная точка (ex,ey) — абсолютная.
func arcTo(b *pathBuilder, rx, ry, xRotDeg float64, large, sweep bool, ex, ey float64) {
	x1, y1 := b.curX, b.curY
	if rx == 0 || ry == 0 {
		b.lineTo(ex, ey)
		return
	}
	rx = math.Abs(rx)
	ry = math.Abs(ry)
	phi := xRotDeg * math.Pi / 180
	cosP, sinP := math.Cos(phi), math.Sin(phi)

	// Шаг 1: приводим к системе окружности.
	dx := (x1 - ex) / 2
	dy := (y1 - ey) / 2
	x1p := cosP*dx + sinP*dy
	y1p := -sinP*dx + cosP*dy

	// Коррекция радиусов при недостаточности.
	lambda := (x1p*x1p)/(rx*rx) + (y1p*y1p)/(ry*ry)
	if lambda > 1 {
		s := math.Sqrt(lambda)
		rx *= s
		ry *= s
	}

	// Шаг 2: центр в приведённой системе.
	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	var co float64
	if den != 0 {
		v := num / den
		if v < 0 {
			v = 0
		}
		co = math.Sqrt(v)
	}
	if large == sweep {
		co = -co
	}
	cxp := co * rx * y1p / ry
	cyp := -co * ry * x1p / rx

	// Шаг 3: центр в исходной системе.
	cx := cosP*cxp - sinP*cyp + (x1+ex)/2
	cy := sinP*cxp + cosP*cyp + (y1+ey)/2

	// Шаг 4: углы.
	theta1 := angleBetween(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	dTheta := angleBetween((x1p-cxp)/rx, (y1p-cyp)/ry, (-x1p-cxp)/rx, (-y1p-cyp)/ry)
	if !sweep && dTheta > 0 {
		dTheta -= 2 * math.Pi
	} else if sweep && dTheta < 0 {
		dTheta += 2 * math.Pi
	}

	// Разбиваем на сегменты ≤90° и приближаем каждый кубической Безье.
	segs := int(math.Ceil(math.Abs(dTheta) / (math.Pi / 2)))
	if segs == 0 {
		segs = 1
	}
	delta := dTheta / float64(segs)
	t := 4.0 / 3.0 * math.Tan(delta/4)

	theta := theta1
	for i := 0; i < segs; i++ {
		cosT1, sinT1 := math.Cos(theta), math.Sin(theta)
		theta2 := theta + delta
		cosT2, sinT2 := math.Cos(theta2), math.Sin(theta2)

		// Точки в системе эллипса (до поворота/сдвига).
		e1x, e1y := cosT1, sinT1
		e2x, e2y := cosT2, sinT2
		c1x := e1x - t*sinT1
		c1y := e1y + t*cosT1
		c2x := e2x + t*sinT2
		c2y := e2y - t*cosT2

		map0 := func(px, py float64) (float64, float64) {
			// масштаб радиусами, поворот, сдвиг центра
			sx := px * rx
			sy := py * ry
			return cosP*sx - sinP*sy + cx, sinP*sx + cosP*sy + cy
		}
		c1X, c1Y := map0(c1x, c1y)
		c2X, c2Y := map0(c2x, c2y)
		eX, eY := map0(e2x, e2y)
		b.cubicTo(c1X, c1Y, c2X, c2Y, eX, eY)
		theta = theta2
	}
}

// angleBetween возвращает угол между векторами (ux,uy) и (vx,vy) со знаком.
func angleBetween(ux, uy, vx, vy float64) float64 {
	dot := ux*vx + uy*vy
	lu := math.Hypot(ux, uy)
	lv := math.Hypot(vx, vy)
	if lu == 0 || lv == 0 {
		return 0
	}
	c := dot / (lu * lv)
	if c > 1 {
		c = 1
	} else if c < -1 {
		c = -1
	}
	ang := math.Acos(c)
	if ux*vy-uy*vx < 0 {
		ang = -ang
	}
	return ang
}
