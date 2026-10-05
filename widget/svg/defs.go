package svg

import (
	"math"
	"strings"
)

// ClipPart — одна фигура тела clipPath: её покрытие входит в объединение.
type ClipPart struct {
	Paths   []Contour
	EvenOdd bool // clip-rule=evenodd
	// Clips — собственный clip-path этой фигуры внутри clipPath (редкость).
	Clips []*ClipPath
}

// ClipPath — вырезка clip-path, уже в координатах viewBox: объединение Parts,
// ещё пересечённое со всеми Chain (собственный clip-path у clipPath).
// Пустой Parts вырезает всё.
type ClipPath struct {
	Parts []ClipPart
	Chain []*ClipPath
}

// Mask — маска mask="url(#…)": её содержимое (уже в координатах viewBox)
// растеризуется, а яркость×альфа результата умножается на покрытие
// маскируемой фигуры.
type Mask struct {
	Shapes []Shape
	// Region — область маски (x/y/width/height); nil — без ограничения.
	Region *ClipPath
	// Alpha — mask-type:alpha: берётся только альфа, а не яркость.
	Alpha bool
}

type clipKey struct {
	n  *xnode
	m  Matrix
	bb [4]float64
}

type maskKey struct {
	n  *xnode
	m  Matrix
	bb [4]float64
}

// hrefID возвращает id из href/xlink:href="#id" ("" — нет или внешняя ссылка).
func hrefID(n *xnode) string {
	v, ok := n.attr("href")
	if !ok {
		return ""
	}
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "#") {
		return v[1:]
	}
	return ""
}

// bboxRect — габариты элемента в его локальных координатах.
type bboxRect struct{ x, y, w, h float64 }

func (r bboxRect) key() [4]float64 { return [4]float64{r.x, r.y, r.w, r.h} }

func (r bboxRect) matrix() Matrix { return Matrix{A: r.w, D: r.h, E: r.x, F: r.y} }

// localBBox считает габариты содержимого n в его собственной системе координат
// (без его transform, без эффектов); ok=false — у элемента нет геометрии.
func (b *builder) localBBox(n *xnode) (bboxRect, bool) {
	sub := b.sub()
	sub.noEffects = true
	sub.noTransformFor = n
	sub.walk(n, defaultInherited(), 1)
	var all []Contour
	for i := range sub.doc.Shapes {
		all = append(all, sub.doc.Shapes[i].Paths...)
	}
	x0, y0, x1, y1, ok := contoursBBox(all)
	if !ok {
		return bboxRect{}, false
	}
	return bboxRect{x0, y0, x1 - x0, y1 - y0}, true
}

// applyEffects читает clip-path, mask и filter элемента и добавляет их в st.
// Ссылка на отсутствующий объект игнорируется (как в браузерах), элемент
// рисуется без этого эффекта.
func (b *builder) applyEffects(n *xnode, pg propGetter, st inherited) inherited {
	if b.noEffects {
		return st
	}
	if v, ok := pg.get("clip-path"); ok {
		if id, _, isURL := parseURLRef(v); isURL && id != "" {
			if c := b.clipFor(id, n, st); c != nil {
				st.clips = append(st.clips[:len(st.clips):len(st.clips)], c)
			}
		}
	}
	if b.clipMode {
		return st
	}
	if v, ok := pg.get("mask"); ok {
		if id, _, isURL := parseURLRef(v); isURL && id != "" {
			if m := b.maskFor(id, n, st); m != nil {
				st.masks = append(st.masks[:len(st.masks):len(st.masks)], m)
			}
		}
	}
	if v, ok := pg.get("filter"); ok {
		if id, _, isURL := parseURLRef(v); isURL && id != "" {
			b.applyFilter(id, &st)
		}
	}
	return st
}

// clipFor строит вырезку для элемента n, ссылающегося на clipPath id.
func (b *builder) clipFor(id string, n *xnode, st inherited) *ClipPath {
	cp := b.ids[id]
	if cp == nil || !strings.EqualFold(cp.XMLName.Local, "clipPath") || b.shared.activeRef[cp] {
		return nil
	}
	obb := false
	if v, ok := cp.attr("clipPathUnits"); ok {
		obb = strings.EqualFold(strings.TrimSpace(v), "objectBoundingBox")
	}
	var bb bboxRect
	if obb {
		var ok bool
		if bb, ok = b.localBBox(n); !ok || bb.w <= 0 || bb.h <= 0 {
			return &ClipPath{} // нет габаритов — вырезать нечем: элемент скрыт
		}
	}
	key := clipKey{n: cp, m: st.transform}
	if obb {
		key.bb = bb.key()
	}
	if c, ok := b.shared.clips[key]; ok {
		return c
	}
	b.shared.activeRef[cp] = true
	defer delete(b.shared.activeRef, cp)

	sub := b.sub()
	sub.clipMode = true
	base := defaultInherited()
	base.transform = st.transform
	cpg := sub.props(cp)
	st0 := sub.resolveState(cp, base, cpg) // transform самого clipPath
	if obb {
		st0.transform = st0.transform.Mul(bb.matrix())
	}
	clip := &ClipPath{}
	for i := range cp.Nodes {
		sub.walk(&cp.Nodes[i], st0, 1)
	}
	for i := range sub.doc.Shapes {
		sh := &sub.doc.Shapes[i]
		clip.Parts = append(clip.Parts, ClipPart{Paths: sh.Paths, EvenOdd: sh.EvenOdd, Clips: sh.Clips})
	}
	// clip-path на самом clipPath: пересечение.
	if v, ok := cpg.get("clip-path"); ok {
		if cid, _, isURL := parseURLRef(v); isURL && cid != "" {
			if c := b.clipFor(cid, n, st); c != nil {
				clip.Chain = append(clip.Chain, c)
			}
		}
	}
	b.shared.clips[key] = clip
	return clip
}

// maskFor строит маску для элемента n, ссылающегося на mask id.
func (b *builder) maskFor(id string, n *xnode, st inherited) *Mask {
	mn := b.ids[id]
	if mn == nil || !strings.EqualFold(mn.XMLName.Local, "mask") || b.shared.activeRef[mn] {
		return nil
	}
	unitsOBB := true // maskUnits по умолчанию objectBoundingBox
	if v, ok := mn.attr("maskUnits"); ok {
		unitsOBB = !strings.EqualFold(strings.TrimSpace(v), "userSpaceOnUse")
	}
	contentOBB := false // maskContentUnits по умолчанию userSpaceOnUse
	if v, ok := mn.attr("maskContentUnits"); ok {
		contentOBB = strings.EqualFold(strings.TrimSpace(v), "objectBoundingBox")
	}
	var bb bboxRect
	if unitsOBB || contentOBB {
		var ok bool
		if bb, ok = b.localBBox(n); !ok || bb.w <= 0 || bb.h <= 0 {
			return &Mask{Region: &ClipPath{}} // нет габаритов: всё скрыто
		}
	}
	key := maskKey{n: mn, m: st.transform, bb: bb.key()}
	if m, ok := b.shared.masks[key]; ok {
		return m
	}
	b.shared.activeRef[mn] = true
	defer delete(b.shared.activeRef, mn)

	// Область маски (по умолчанию -10%,-10%,120%,120%).
	geom := func(name, def string, size float64, bbOff, bbSize float64) float64 {
		v, ok := mn.attr(name)
		if !ok {
			v = def
		}
		f := b.coordUnits(v, unitsOBB, size)
		if unitsOBB {
			return bbOff + f*bbSize
		}
		return f
	}
	x := geom("x", "-10%", b.vpW, bb.x, bb.w)
	y := geom("y", "-10%", b.vpH, bb.y, bb.h)
	var w, h float64
	if unitsOBB {
		w = geom("width", "120%", b.vpW, 0, bb.w)
		h = geom("height", "120%", b.vpH, 0, bb.h)
	} else {
		w = geom("width", "120%", b.vpW, 0, 0)
		h = geom("height", "120%", b.vpH, 0, 0)
	}
	mask := &Mask{}
	if w > 0 && h > 0 {
		rect := applyTransform(rectContours(x, y, w, h, 0, 0), st.transform)
		mask.Region = &ClipPath{Parts: []ClipPart{{Paths: rect}}}
	} else {
		mask.Region = &ClipPath{}
	}
	mpg := b.props(mn)
	if v, ok := mpg.get("mask-type"); ok {
		mask.Alpha = strings.EqualFold(strings.TrimSpace(v), "alpha")
	}

	sub := b.sub()
	base := defaultInherited()
	base.transform = st.transform
	st0 := sub.resolveState(mn, base, sub.props(mn))
	if contentOBB {
		st0.transform = st0.transform.Mul(bb.matrix())
	}
	for i := range mn.Nodes {
		sub.walk(&mn.Nodes[i], st0, 1)
	}
	mask.Shapes = sub.doc.Shapes
	b.shared.masks[key] = mask
	return mask
}

// applyFilter учитывает из <filter> то, что умеет: feGaussianBlur (размытие
// фигуры) и feColorMatrix (цвет). Остальные примитивы игнорируются — элемент
// рисуется без них.
func (b *builder) applyFilter(id string, st *inherited) {
	f := b.ids[id]
	if f == nil || !strings.EqualFold(f.XMLName.Local, "filter") {
		return
	}
	for i := range f.Nodes {
		p := &f.Nodes[i]
		switch strings.ToLower(p.XMLName.Local) {
		case "fegaussianblur":
			v, _ := p.attr("stdDeviation")
			f := parseFloats(v)
			if len(f) == 0 {
				continue
			}
			sx, sy := f[0], f[0]
			if len(f) > 1 {
				sy = f[1]
			}
			if sx < 0 || sy < 0 {
				continue
			}
			t := st.transform
			sx *= math.Hypot(t.A, t.B)
			sy *= math.Hypot(t.C, t.D)
			st.blurX = math.Hypot(st.blurX, sx)
			st.blurY = math.Hypot(st.blurY, sy)
		case "fecolormatrix":
			if m := parseColorMatrix(p); m != nil {
				st.cmat = m
			}
		}
	}
}

// ── use / symbol / вложенный svg ─────────────────────────────────────────────

// walkUse раскрывает <use>: рисует ссылку с учётом x/y и transform самого use.
// Ссылка на symbol или svg получает ещё и viewport (viewBox → width×height).
func (b *builder) walkUse(n *xnode, st inherited, depth int) {
	id := hrefID(n)
	if id == "" {
		return
	}
	t := b.ids[id]
	if t == nil || b.shared.activeUse[t] || t == n {
		return
	}
	st.transform = st.transform.Mul(Translate(lenAttr(n, "x"), lenAttr(n, "y")))

	b.shared.activeUse[t] = true
	b.shared.useDepth++
	defer func() {
		delete(b.shared.activeUse, t)
		b.shared.useDepth--
	}()

	tag := strings.ToLower(t.XMLName.Local)
	if tag != "symbol" && tag != "svg" {
		b.walk(t, st, depth+1)
		return
	}
	tpg := b.props(t)
	st2 := b.resolveState(t, st, tpg)
	st2 = b.applyEffects(t, tpg, st2)
	w, h := b.vpW, b.vpH
	if v, ok := t.attr("width"); ok {
		w = b.coordUnits(v, false, b.vpW)
	}
	if v, ok := t.attr("height"); ok {
		h = b.coordUnits(v, false, b.vpH)
	}
	if v, ok := n.attr("width"); ok {
		w = b.coordUnits(v, false, b.vpW)
	}
	if v, ok := n.attr("height"); ok {
		h = b.coordUnits(v, false, b.vpH)
	}
	if vb, ok := viewBoxOf(t); ok && w > 0 && h > 0 {
		par, _ := t.attr("preserveAspectRatio")
		st2.transform = st2.transform.Mul(viewBoxMatrix(vb, par, 0, 0, w, h))
	}
	for i := range t.Nodes {
		b.walk(&t.Nodes[i], st2, depth+1)
	}
}

// nestedViewport применяет x/y/viewBox вложенного <svg>.
func (b *builder) nestedViewport(n *xnode, st inherited) inherited {
	x, y := lenAttr(n, "x"), lenAttr(n, "y")
	w, h := b.vpW, b.vpH
	if v, ok := n.attr("width"); ok {
		w = b.coordUnits(v, false, b.vpW)
	}
	if v, ok := n.attr("height"); ok {
		h = b.coordUnits(v, false, b.vpH)
	}
	if vb, ok := viewBoxOf(n); ok && w > 0 && h > 0 {
		par, _ := n.attr("preserveAspectRatio")
		st.transform = st.transform.Mul(viewBoxMatrix(vb, par, x, y, w, h))
	} else if x != 0 || y != 0 {
		st.transform = st.transform.Mul(Translate(x, y))
	}
	return st
}

func viewBoxOf(n *xnode) ([4]float64, bool) {
	v, ok := n.attr("viewBox")
	if !ok {
		return [4]float64{}, false
	}
	f := parseFloats(v)
	if len(f) != 4 || f[2] <= 0 || f[3] <= 0 {
		return [4]float64{}, false
	}
	return [4]float64{f[0], f[1], f[2], f[3]}, true
}

// viewBoxMatrix — преобразование viewBox → прямоугольник (x,y,w,h) с учётом
// preserveAspectRatio (meet/slice, выравнивание; slice без обрезки по краям).
func viewBoxMatrix(vb [4]float64, par string, x, y, w, h float64) Matrix {
	sx, sy := w/vb[2], h/vb[3]
	fields := strings.Fields(par)
	align := "xMidYMid"
	slice := false
	if len(fields) > 0 && fields[0] == "defer" {
		fields = fields[1:]
	}
	if len(fields) > 0 {
		align = fields[0]
	}
	if len(fields) > 1 && fields[1] == "slice" {
		slice = true
	}
	if align == "none" {
		return Translate(x, y).Mul(ScaleM(sx, sy)).Mul(Translate(-vb[0], -vb[1]))
	}
	s := minf(sx, sy)
	if slice {
		s = maxf(sx, sy)
	}
	fx, fy := 0.5, 0.5
	if strings.HasPrefix(align, "xMin") {
		fx = 0
	} else if strings.HasPrefix(align, "xMax") {
		fx = 1
	}
	if strings.HasSuffix(align, "YMin") {
		fy = 0
	} else if strings.HasSuffix(align, "YMax") {
		fy = 1
	}
	tx := x + (w-vb[2]*s)*fx
	ty := y + (h-vb[3]*s)*fy
	return Translate(tx, ty).Mul(ScaleM(s, s)).Mul(Translate(-vb[0], -vb[1]))
}
