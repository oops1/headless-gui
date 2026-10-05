package svg

import (
	"image"
	"image/color"
	"math"
	"strings"
)

// FilterGraph — граф примитивов <filter>, который не сводится к одному
// размытию или матрице цвета: feOffset, feFlood, feComposite, feMerge, feBlend,
// feDropShadow и цепочки с именованными result/in (типичная «тень» из
// feGaussianBlur in="SourceAlpha" + feOffset + feFlood + feComposite +
// feMerge). Выполняется над слоем группы целиком (Group.Filter), поэтому
// фигура с таким фильтром получает свою группу-слой.
//
// Граф строит разбор; значения в единицах viewBox. Фильтр с примитивом,
// которого граф не знает (feTurbulence, feMorphology, feDisplacementMap…), в
// граф не попадает: элемент рисуется без фильтра.
type FilterGraph struct {
	prims []filterPrim
}

type filterPrim struct {
	kind    string // blur, offset, flood, composite, merge, blend, matrix, dropshadow
	in, in2 string // "" — результат предыдущего примитива (у первого — SourceGraphic)
	result  string

	sx, sy float64    // blur, dropshadow: отклонение, viewBox
	dx, dy float64    // offset, dropshadow: сдвиг, viewBox
	col    color.RGBA // flood, dropshadow: цвет, A уже умножена на flood-opacity
	op     string     // composite: over|in|out|atop|xor|arithmetic; blend: режим
	k      [4]float64 // composite arithmetic
	cmat   *[20]float64
	merge  []string // merge: входы по порядку, снизу вверх
}

// primKind — вид примитива графа по имени элемента; пусто — графу неизвестен.
func primKind(local string) string {
	switch strings.ToLower(local) {
	case "fegaussianblur":
		return "blur"
	case "feoffset":
		return "offset"
	case "feflood":
		return "flood"
	case "fecomposite":
		return "composite"
	case "femerge":
		return "merge"
	case "feblend":
		return "blend"
	case "fecolormatrix":
		return "matrix"
	case "fedropshadow":
		return "dropshadow"
	}
	return ""
}

// isPrimTag — это элемент-примитив фильтра (fe*), известный или нет.
func isPrimTag(local string) bool {
	l := strings.ToLower(local)
	return strings.HasPrefix(l, "fe") && l != "fe"
}

// buildFilterGraph строит граф из примитивов filter-узла f. ok=false — в
// фильтре есть то, чего граф не умеет (неизвестный примитив, вход вроде
// BackgroundImage, ссылка на несуществующий result, primitiveUnits=
// objectBoundingBox): вызывающий пропускает фильтр целиком. t — преобразование
// элемента (сдвиги и отклонения идут в единицах viewBox).
func (b *builder) buildFilterGraph(f *xnode, t Matrix) (*FilterGraph, bool) {
	if v, ok := f.attr("primitiveUnits"); ok && strings.EqualFold(strings.TrimSpace(v), "objectBoundingBox") {
		return nil, false
	}
	g := &FilterGraph{}
	defined := map[string]bool{}
	inputOK := func(in string) bool {
		switch in {
		case "", "SourceGraphic", "SourceAlpha":
			return true
		}
		return defined[in]
	}
	for i := range f.Nodes {
		p := &f.Nodes[i]
		if !isPrimTag(p.XMLName.Local) {
			continue // <desc>, <title>, <animate>… фильтру не мешают
		}
		kind := primKind(p.XMLName.Local)
		if kind == "" {
			return nil, false
		}
		fp := filterPrim{kind: kind}
		fp.in, _ = p.attr("in")
		fp.in = strings.TrimSpace(fp.in)
		fp.in2, _ = p.attr("in2")
		fp.in2 = strings.TrimSpace(fp.in2)
		fp.result, _ = p.attr("result")
		fp.result = strings.TrimSpace(fp.result)
		if !inputOK(fp.in) {
			return nil, false
		}
		switch kind {
		case "blur":
			fp.sx, fp.sy = b.devStd(p, t, 0, 0)
		case "offset":
			fp.dx, fp.dy = devShift(p, t, 0, 0)
		case "dropshadow":
			fp.sx, fp.sy = b.devStd(p, t, 2, 2)
			fp.dx, fp.dy = devShift(p, t, 2, 2)
			fp.col = b.floodColor(p)
		case "flood":
			fp.col = b.floodColor(p)
		case "matrix":
			fp.cmat = parseColorMatrix(p)
		case "composite", "blend":
			if !inputOK(fp.in2) {
				return nil, false
			}
			fp.op = "over"
			if kind == "blend" {
				fp.op = "normal"
			}
			if v, ok := p.attr("operator"); ok && kind == "composite" {
				fp.op = strings.ToLower(strings.TrimSpace(v))
			}
			if v, ok := p.attr("mode"); ok && kind == "blend" {
				fp.op = strings.ToLower(strings.TrimSpace(v))
			}
			switch fp.op {
			case "over", "in", "out", "atop", "xor", "arithmetic",
				"normal", "multiply", "screen", "darken", "lighten":
			default:
				return nil, false
			}
			if kind == "composite" && fp.op == "arithmetic" {
				for j, n := range [4]string{"k1", "k2", "k3", "k4"} {
					if v, ok := p.attr(n); ok {
						if fl := parseFloats(v); len(fl) > 0 {
							fp.k[j] = fl[0]
						}
					}
				}
			}
		case "merge":
			for j := range p.Nodes {
				mn := &p.Nodes[j]
				if !strings.EqualFold(mn.XMLName.Local, "feMergeNode") {
					continue
				}
				in, _ := mn.attr("in")
				in = strings.TrimSpace(in)
				if !inputOK(in) {
					return nil, false
				}
				fp.merge = append(fp.merge, in)
			}
		}
		g.prims = append(g.prims, fp)
		if fp.result != "" {
			defined[fp.result] = true
		}
	}
	if len(g.prims) == 0 {
		return nil, false
	}
	return g, true
}

// devStd читает stdDeviation в единицах viewBox при преобразовании t; def —
// значение по умолчанию (feDropShadow: 2, feGaussianBlur: 0).
func (b *builder) devStd(p *xnode, t Matrix, defX, defY float64) (sx, sy float64) {
	sx, sy = defX, defY
	if v, ok := p.attr("stdDeviation"); ok {
		f := parseFloats(v)
		if len(f) > 0 {
			sx, sy = f[0], f[0]
			if len(f) > 1 {
				sy = f[1]
			}
		}
	}
	if sx < 0 || sy < 0 {
		return 0, 0
	}
	return sx * math.Hypot(t.A, t.B), sy * math.Hypot(t.C, t.D)
}

// devShift читает dx/dy (по умолчанию def) и переводит вектор через линейную
// часть t в единицы viewBox.
func devShift(p *xnode, t Matrix, defX, defY float64) (dx, dy float64) {
	dx, dy = defX, defY
	if v, ok := p.attr("dx"); ok {
		if f := parseFloats(v); len(f) > 0 {
			dx = f[0]
		}
	}
	if v, ok := p.attr("dy"); ok {
		if f := parseFloats(v); len(f) > 0 {
			dy = f[0]
		}
	}
	return t.A*dx + t.C*dy, t.B*dx + t.D*dy
}

// floodColor — flood-color и flood-opacity примитива (атрибутом или в style);
// по умолчанию чёрный, непрозрачный.
func (b *builder) floodColor(p *xnode) color.RGBA {
	pg := b.props(p)
	col := color.RGBA{A: 255}
	if v, ok := pg.get("flood-color"); ok {
		if c, ok := ParseColor(v); ok {
			col = c
		}
	}
	if v, ok := pg.get("flood-opacity"); ok {
		col.A = uint8(float64(col.A)*clampUnit(parseOpacity(v)) + 0.5)
	}
	return col
}

// ── выполнение ───────────────────────────────────────────────────────────────

// run выполняет граф над слоем src (premultiplied) и возвращает результат
// последнего примитива. s — масштаб viewBox → пиксели. src не меняется.
func (g *FilterGraph) run(src *image.RGBA, s float64) *image.RGBA {
	named := map[string]*image.RGBA{}
	var alpha, last *image.RGBA
	resolve := func(in string) *image.RGBA {
		switch in {
		case "":
			if last == nil {
				return src
			}
			return last
		case "SourceGraphic":
			return src
		case "SourceAlpha":
			if alpha == nil {
				alpha = alphaOnly(src)
			}
			return alpha
		}
		if im := named[in]; im != nil {
			return im
		}
		if last == nil {
			return src
		}
		return last
	}
	for i := range g.prims {
		p := &g.prims[i]
		var out *image.RGBA
		switch p.kind {
		case "blur":
			out = blurRGBA(resolve(p.in), p.sx*s, p.sy*s)
		case "offset":
			out = shiftRGBA(resolve(p.in), p.dx*s, p.dy*s)
		case "flood":
			out = floodRGBA(src.Bounds(), p.col)
		case "matrix":
			out = cloneRGBA(resolve(p.in))
			if p.cmat != nil {
				matrixRGBA(out, p.cmat)
			}
		case "composite":
			out = compositeRGBA(resolve(p.in), resolve(p.in2), p.op, p.k)
		case "blend":
			out = blendRGBA(resolve(p.in), resolve(p.in2), p.op)
		case "merge":
			out = image.NewRGBA(src.Bounds())
			for _, in := range p.merge {
				compositeOver(out, resolve(in))
			}
		case "dropshadow":
			in := resolve(p.in)
			sh := blurRGBA(alphaOnly(in), p.sx*s, p.sy*s)
			sh = shiftRGBA(sh, p.dx*s, p.dy*s)
			sh = compositeRGBA(floodRGBA(src.Bounds(), p.col), sh, "in", [4]float64{})
			compositeOver(sh, in)
			out = sh
		}
		last = out
		if p.result != "" {
			named[p.result] = out
		}
	}
	if last == nil {
		return src
	}
	return last
}

func cloneRGBA(src *image.RGBA) *image.RGBA {
	out := image.NewRGBA(src.Bounds())
	copy(out.Pix, src.Pix)
	return out
}

// alphaOnly — SourceAlpha: покрытие слоя, цвет чёрный.
func alphaOnly(src *image.RGBA) *image.RGBA {
	out := image.NewRGBA(src.Bounds())
	for i := 3; i < len(src.Pix); i += 4 {
		out.Pix[i] = src.Pix[i]
	}
	return out
}

// floodRGBA заливает прямоугольник цветом (A уже учитывает flood-opacity).
func floodRGBA(r image.Rectangle, c color.RGBA) *image.RGBA {
	out := image.NewRGBA(r)
	a := uint32(c.A)
	pr := uint8((uint32(c.R)*a + 127) / 255)
	pg := uint8((uint32(c.G)*a + 127) / 255)
	pb := uint8((uint32(c.B)*a + 127) / 255)
	for i := 0; i+3 < len(out.Pix); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = pr, pg, pb, c.A
	}
	return out
}

// shiftRGBA сдвигает слой на dx, dy пикселей. Дробный сдвиг делается
// линейной интерполяцией между соседними пикселями: тень на полпикселя не
// «прыгает» на целый.
func shiftRGBA(src *image.RGBA, dx, dy float64) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(b)
	ix, iy := int(math.Floor(dx)), int(math.Floor(dy))
	fx, fy := dx-float64(ix), dy-float64(iy)
	px := func(x, y, k int) float64 {
		if x < 0 || y < 0 || x >= w || y >= h {
			return 0
		}
		return float64(src.Pix[y*src.Stride+x*4+k])
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx, sy := x-ix, y-iy // пиксель-источник (левый-верхний из четырёх)
			for k := 0; k < 4; k++ {
				v := px(sx, sy, k)*(1-fx)*(1-fy) +
					px(sx-1, sy, k)*fx*(1-fy) +
					px(sx, sy-1, k)*(1-fx)*fy +
					px(sx-1, sy-1, k)*fx*fy
				out.Pix[y*out.Stride+x*4+k] = uint8(v + 0.5)
			}
		}
	}
	return out
}

// compositeRGBA — feComposite: a — in, b — in2 (оба premultiplied).
func compositeRGBA(a, b *image.RGBA, op string, k [4]float64) *image.RGBA {
	out := image.NewRGBA(a.Bounds())
	for i := 0; i+3 < len(a.Pix); i += 4 {
		aa, ba := float64(a.Pix[i+3])/255, float64(b.Pix[i+3])/255
		var fa, fb float64 // доли a и b в результате (Porter–Duff)
		switch op {
		case "over":
			fa, fb = 1, 1-aa
		case "in":
			fa, fb = ba, 0
		case "out":
			fa, fb = 1-ba, 0
		case "atop":
			fa, fb = ba, 1-aa
		case "xor":
			fa, fb = 1-ba, 1-aa
		case "arithmetic":
			var res [4]float64
			for c := 0; c < 4; c++ {
				x, y := float64(a.Pix[i+c])/255, float64(b.Pix[i+c])/255
				res[c] = k[0]*x*y + k[1]*x + k[2]*y + k[3]
			}
			ra := clamp01(res[3])
			for c := 0; c < 3; c++ {
				out.Pix[i+c] = uint8(math.Min(clamp01(res[c]), ra)*255 + 0.5)
			}
			out.Pix[i+3] = uint8(ra*255 + 0.5)
			continue
		}
		for c := 0; c < 4; c++ {
			v := float64(a.Pix[i+c])*fa + float64(b.Pix[i+c])*fb
			if v > 255 {
				v = 255
			}
			out.Pix[i+c] = uint8(v + 0.5)
		}
	}
	return out
}

// blendRGBA — feBlend: a — in (верхний), b — in2 (нижний), premultiplied.
func blendRGBA(a, b *image.RGBA, mode string) *image.RGBA {
	out := image.NewRGBA(a.Bounds())
	for i := 0; i+3 < len(a.Pix); i += 4 {
		qa, qb := float64(a.Pix[i+3])/255, float64(b.Pix[i+3])/255
		for c := 0; c < 3; c++ {
			ca, cb := float64(a.Pix[i+c])/255, float64(b.Pix[i+c])/255
			var r float64
			switch mode {
			case "multiply":
				r = (1-qa)*cb + (1-qb)*ca + ca*cb
			case "screen":
				r = cb + ca - ca*cb
			case "darken":
				r = math.Min((1-qa)*cb+ca, (1-qb)*ca+cb)
			case "lighten":
				r = math.Max((1-qa)*cb+ca, (1-qb)*ca+cb)
			default: // normal
				r = (1-qa)*cb + ca
			}
			out.Pix[i+c] = uint8(clamp01(r)*255 + 0.5)
		}
		out.Pix[i+3] = uint8((1-(1-qa)*(1-qb))*255 + 0.5)
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
