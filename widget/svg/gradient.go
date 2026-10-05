package svg

import (
	"image/color"
	"math"
	"strings"
)

// GradientKind — вид градиента.
type GradientKind uint8

const (
	// GradientLinear — <linearGradient>.
	GradientLinear GradientKind = iota
	// GradientRadial — <radialGradient>.
	GradientRadial
)

// SpreadMethod — spreadMethod: что рисовать за пределами вектора градиента.
type SpreadMethod uint8

const (
	// SpreadPad — крайние цвета тянутся дальше (по умолчанию).
	SpreadPad SpreadMethod = iota
	// SpreadReflect — градиент отражается туда-обратно.
	SpreadReflect
	// SpreadRepeat — градиент повторяется.
	SpreadRepeat
)

// GradientStop — одна опорная точка градиента.
type GradientStop struct {
	Offset float64    // 0..1, не убывает по списку
	Color  color.RGBA // не премультиплицированный; stop-opacity уже в A
	// Current — stop-color="currentColor": RGB берётся из current при
	// растеризации (A остаётся от stop-opacity).
	Current bool
}

// Gradient — градиент, уже привязанный к конкретной фигуре: Transform
// переводит систему координат самого градиента (в которой заданы X1…FY) в
// систему viewBox документа, т.е. учитывает и transform элемента, и bbox
// (gradientUnits=objectBoundingBox), и gradientTransform.
type Gradient struct {
	Kind   GradientKind
	Stops  []GradientStop
	Spread SpreadMethod

	// Линейный: вектор (X1,Y1)→(X2,Y2).
	X1, Y1, X2, Y2 float64
	// Радиальный: окружность (CX,CY,R) и фокус (FX,FY).
	CX, CY, R, FX, FY float64

	Transform Matrix
}

// MeanColor — «средний» цвет градиента: стопы, взвешенные непрозрачностью (чтобы
// невидимый стоп не тянул цвет к чёрному); A — средняя непрозрачность. Нужен
// тем, кто красит монохромно или не умеет градиенты (Shape.Fill для фигур с
// градиентом хранит именно его). Стоп currentColor даёт чёрный RGB.
func (g *Gradient) MeanColor() color.RGBA {
	if g == nil || len(g.Stops) == 0 {
		return color.RGBA{}
	}
	var r, gr, b, a float64
	for _, s := range g.Stops {
		w := float64(s.Color.A) / 255
		r += float64(s.Color.R) * w
		gr += float64(s.Color.G) * w
		b += float64(s.Color.B) * w
		a += w
	}
	if a == 0 {
		return color.RGBA{}
	}
	n := float64(len(g.Stops))
	return color.RGBA{
		R: uint8(r/a + 0.5),
		G: uint8(gr/a + 0.5),
		B: uint8(b/a + 0.5),
		A: uint8(a/n*255 + 0.5),
	}
}

// ── разбор из дерева ─────────────────────────────────────────────────────────

// gradDef — разобранный, но ещё не привязанный к фигуре градиент.
type gradDef struct {
	g   Gradient // геометрия — в единицах самого градиента (Transform пуст)
	obb bool     // gradientUnits=objectBoundingBox
	tr  Matrix   // gradientTransform
}

// maxHrefHops — глубина цепочки xlink:href у градиента (защита от циклов).
const maxHrefHops = 16

func isGradientTag(n *xnode) bool {
	t := strings.ToLower(n.XMLName.Local)
	return t == "lineargradient" || t == "radialgradient"
}

// gradientDef разбирает градиент n (с наследованием через href); nil — градиент
// непригоден (нет стопов не считается ошибкой: решает вызывающий).
func (b *builder) gradientDef(n *xnode) *gradDef {
	if d, ok := b.shared.grads[n]; ok {
		return d
	}
	d := b.parseGradient(n)
	b.shared.grads[n] = d
	return d
}

// chain — сам градиент и цепочка его href-предков (без циклов).
func (b *builder) gradientChain(n *xnode) []*xnode {
	chain := []*xnode{n}
	seen := map[*xnode]bool{n: true}
	cur := n
	for len(chain) < maxHrefHops {
		id := hrefID(cur)
		if id == "" {
			break
		}
		next := b.ids[id]
		if next == nil || seen[next] || !isGradientTag(next) {
			break
		}
		seen[next] = true
		chain = append(chain, next)
		cur = next
	}
	return chain
}

func (b *builder) parseGradient(n *xnode) *gradDef {
	chain := b.gradientChain(n)
	kind := GradientLinear
	if strings.EqualFold(n.XMLName.Local, "radialGradient") {
		kind = GradientRadial
	}

	// attr — первое значение по цепочке; own=true — только у градиентов того же
	// вида (геометрия не наследуется между linear и radial).
	attr := func(name string, own bool) (string, bool) {
		for _, c := range chain {
			if own && !strings.EqualFold(c.XMLName.Local, n.XMLName.Local) {
				continue
			}
			if v, ok := c.attr(name); ok {
				return v, true
			}
		}
		return "", false
	}

	d := &gradDef{g: Gradient{Kind: kind}, tr: Identity()}
	if v, ok := attr("gradientUnits", false); ok {
		d.obb = !strings.EqualFold(strings.TrimSpace(v), "userSpaceOnUse")
	} else {
		d.obb = true
	}
	if v, ok := attr("gradientTransform", false); ok {
		d.tr = ParseTransform(v)
	}
	if v, ok := attr("spreadMethod", false); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "reflect":
			d.g.Spread = SpreadReflect
		case "repeat":
			d.g.Spread = SpreadRepeat
		}
	}

	// Единицы процентов: bbox — доли, userSpaceOnUse — от viewport.
	coord := func(name string, def string, size float64, own bool) float64 {
		v, ok := attr(name, own)
		if !ok {
			v = def
		}
		return b.coordUnits(v, d.obb, size)
	}
	if kind == GradientLinear {
		d.g.X1 = coord("x1", "0%", b.vpW, true)
		d.g.Y1 = coord("y1", "0%", b.vpH, true)
		d.g.X2 = coord("x2", "100%", b.vpW, true)
		d.g.Y2 = coord("y2", "0%", b.vpH, true)
	} else {
		diag := math.Sqrt((b.vpW*b.vpW + b.vpH*b.vpH) / 2)
		d.g.CX = coord("cx", "50%", b.vpW, true)
		d.g.CY = coord("cy", "50%", b.vpH, true)
		d.g.R = coord("r", "50%", diag, true)
		d.g.FX, d.g.FY = d.g.CX, d.g.CY
		if _, ok := attr("fx", true); ok {
			d.g.FX = coord("fx", "50%", b.vpW, true)
		}
		if _, ok := attr("fy", true); ok {
			d.g.FY = coord("fy", "50%", b.vpH, true)
		}
	}

	// Стопы — у первого в цепочке градиента, у которого они есть.
	for _, c := range chain {
		stops := b.parseStops(c)
		if len(stops) > 0 {
			d.g.Stops = stops
			break
		}
	}
	return d
}

// coordUnits разбирает координату градиента/маски/клипа: «50%» → доля (obb)
// или доля размера viewport; число — как есть.
func (b *builder) coordUnits(s string, obb bool, size float64) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		f := parseLength(strings.TrimSuffix(s, "%")) / 100
		if obb {
			return f
		}
		return f * size
	}
	return parseLength(s)
}

// parseStops собирает <stop> градиента n.
func (b *builder) parseStops(n *xnode) []GradientStop {
	var stops []GradientStop
	prev := 0.0
	for i := range n.Nodes {
		c := &n.Nodes[i]
		if !strings.EqualFold(c.XMLName.Local, "stop") {
			continue
		}
		pg := b.props(c)
		off := 0.0
		if v, ok := pg.get("offset"); ok {
			off = clampUnit(parseOpacity(v))
		}
		if off < prev {
			off = prev
		}
		prev = off

		st := GradientStop{Offset: off, Color: color.RGBA{0, 0, 0, 255}}
		if v, ok := pg.get("stop-color"); ok {
			p := ParsePaint(v)
			switch p.Kind {
			case PaintColor:
				st.Color = p.Color
			case PaintCurrent:
				st.Current = true
			}
		}
		if v, ok := pg.get("stop-opacity"); ok {
			// Цвет остаётся: при stop-opacity:0 он всё равно участвует в
			// интерполяции соседних стопов (applyOpacity обнулил бы и RGB).
			st.Color.A = uint8(float64(st.Color.A)*clampUnit(parseOpacity(v)) + 0.5)
		}
		stops = append(stops, st)
	}
	return stops
}

// ── растеризация ─────────────────────────────────────────────────────────────

// lutSize — число ячеек таблицы цветов градиента.
const lutSize = 512

// gradPainter красит пиксели по градиенту.
type gradPainter struct {
	lut    []color.RGBA // straight RGBA
	spread SpreadMethod

	solid bool // вырожденный градиент — один цвет (lut[lutSize-1])

	radial bool
	// линейный: t = ta*x + tb*y + tc в пикселях устройства
	ta, tb, tc float64
	// радиальный
	inv               Matrix // устройство → пространство градиента
	fx, fy, ex, ey, a float64
}

// newGradPainter готовит краску для градиента g. dev2vb — переход пиксель
// устройства → система viewBox. nil — рисовать нечего (вырожденная матрица).
func newGradPainter(g *Gradient, dev2vb Matrix, current color.RGBA, tint bool, cm *[20]float64) *gradPainter {
	if len(g.Stops) == 0 {
		return nil
	}
	inv, ok := g.Transform.inverse()
	if !ok {
		return nil
	}
	p := &gradPainter{lut: buildLUT(g.Stops, current, tint, cm), spread: g.Spread}
	if len(g.Stops) == 1 {
		p.solid = true
		return p
	}
	// устройство → пространство градиента
	dev := inv.Mul(dev2vb)
	if g.Kind == GradientLinear {
		dx, dy := g.X2-g.X1, g.Y2-g.Y1
		l2 := dx*dx + dy*dy
		if l2 == 0 || math.IsNaN(l2) {
			p.solid = true
			return p
		}
		p.ta = (dev.A*dx + dev.B*dy) / l2
		p.tb = (dev.C*dx + dev.D*dy) / l2
		p.tc = ((dev.E-g.X1)*dx + (dev.F-g.Y1)*dy) / l2
		return p
	}
	r := g.R
	if r <= 0 || math.IsNaN(r) {
		p.solid = true
		return p
	}
	fx, fy := g.FX, g.FY
	// Фокус обязан лежать внутри окружности (SVG 1.1): иначе прижимаем к краю.
	if d := math.Hypot(fx-g.CX, fy-g.CY); d >= r {
		k := r * 0.999 / d
		fx = g.CX + (fx-g.CX)*k
		fy = g.CY + (fy-g.CY)*k
	}
	p.radial = true
	p.inv = dev
	p.fx, p.fy = fx, fy
	p.ex, p.ey = g.CX-fx, g.CY-fy
	p.a = p.ex*p.ex + p.ey*p.ey - r*r // < 0, пока фокус внутри
	return p
}

// at возвращает цвет пикселя (x,y) устройства (по центру пикселя).
func (p *gradPainter) at(x, y int) color.RGBA {
	if p.solid {
		return p.lut[lutSize-1]
	}
	px, py := float64(x)+0.5, float64(y)+0.5
	var t float64
	if !p.radial {
		t = p.ta*px + p.tb*py + p.tc
	} else {
		gx := p.inv.A*px + p.inv.C*py + p.inv.E
		gy := p.inv.B*px + p.inv.D*py + p.inv.F
		dx, dy := gx-p.fx, gy-p.fy
		bq := -2 * (dx*p.ex + dy*p.ey)
		cq := dx*dx + dy*dy
		disc := bq*bq - 4*p.a*cq
		if disc < 0 {
			disc = 0
		}
		t = (-bq - math.Sqrt(disc)) / (2 * p.a)
	}
	if math.IsNaN(t) {
		t = 0
	}
	return p.lut[lutIndex(t, p.spread)]
}

func lutIndex(t float64, m SpreadMethod) int {
	switch m {
	case SpreadRepeat:
		t -= math.Floor(t)
	case SpreadReflect:
		u := math.Mod(math.Abs(t), 2)
		if u > 1 {
			u = 2 - u
		}
		t = u
	default:
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
	}
	return int(t*(lutSize-1) + 0.5)
}

// buildLUT строит таблицу цветов по стопам (интерполяция в sRGB без
// премультипликации, как в спецификации SVG).
func buildLUT(stops []GradientStop, current color.RGBA, tint bool, cm *[20]float64) []color.RGBA {
	col := make([]color.RGBA, len(stops))
	for i, s := range stops {
		c := s.Color
		switch {
		case tint:
			// монохромная перекраска: RGB — current, форму даёт альфа стопа
			c = color.RGBA{current.R, current.G, current.B, uint8((uint32(c.A)*uint32(current.A) + 127) / 255)}
		case s.Current:
			c.R, c.G, c.B = current.R, current.G, current.B
		}
		col[i] = c
	}
	lut := make([]color.RGBA, lutSize)
	j := 0
	for i := range lut {
		t := float64(i) / (lutSize - 1)
		for j < len(stops) && stops[j].Offset <= t {
			j++
		}
		var c color.RGBA
		switch {
		case j == 0:
			c = col[0]
		case j == len(stops):
			c = col[len(stops)-1]
		default:
			o0, o1 := stops[j-1].Offset, stops[j].Offset
			f := 0.0
			if o1 > o0 {
				f = (t - o0) / (o1 - o0)
			}
			c = lerpRGBA(col[j-1], col[j], f)
		}
		if cm != nil {
			c = applyColorMatrix(c, cm)
		}
		lut[i] = c
	}
	return lut
}

func lerpRGBA(a, b color.RGBA, f float64) color.RGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f + 0.5) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), l(a.A, b.A)}
}
