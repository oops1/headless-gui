package svg

import (
	"image"
	"image/color"
	"math"
	"strings"
)

// Pattern — узор fill|stroke="url(#pattern)", уже привязанный к конкретной
// фигуре: габариты объекта (patternUnits=objectBoundingBox) и transform
// элемента в нём учтены.
type Pattern struct {
	// Tile — плитка (x, y, ширина, высота) в системе координат узора.
	Tile [4]float64
	// M переводит систему координат узора в систему viewBox документа:
	// transform элемента и patternTransform.
	M Matrix
	// Shapes — содержимое плитки. Координаты — в единицах узора, начало (0,0)
	// — левый верхний угол плитки (viewBox и patternContentUnits уже учтены).
	Shapes []Shape
}

type patKey struct {
	n  *xnode
	m  Matrix
	bb [4]float64
}

// maxPatternTile — предел стороны растрированной плитки, пикселей. Площадь
// ограничена отдельно (maxPatternTilePixels).
const (
	maxPatternTile       = 2048
	maxPatternTilePixels = 4 << 20
)

// patternChainAttr ищет атрибут у первого в цепочке href-ссылок узора, у
// кого он есть.
func patternChainAttr(chain []*xnode, name string) (string, bool) {
	for _, c := range chain {
		if v, ok := c.attr(name); ok {
			return v, true
		}
	}
	return "", false
}

// patternChain — узор и узоры, на которые он ссылается (xlink:href); цикл и
// ссылки не на pattern обрываются.
func (b *builder) patternChain(n *xnode) []*xnode {
	chain := []*xnode{n}
	seen := map[*xnode]bool{n: true}
	for len(chain) < 16 {
		id := hrefID(chain[len(chain)-1])
		next := b.ids[id]
		if id == "" || next == nil || seen[next] || !strings.EqualFold(next.XMLName.Local, "pattern") {
			break
		}
		seen[next] = true
		chain = append(chain, next)
	}
	return chain
}

// patternFor строит узор для фигуры с контурами contours (в локальных
// координатах) и состоянием st. nil — узор ничего не рисует (вырожденная
// плитка, нет габаритов, цикл).
func (b *builder) patternFor(n *xnode, st inherited, contours []Contour) *Pattern {
	if b.shared.activeRef[n] || b.clipMode {
		return nil
	}
	chain := b.patternChain(n)
	unitsOBB := true // patternUnits по умолчанию objectBoundingBox
	if v, ok := patternChainAttr(chain, "patternUnits"); ok {
		unitsOBB = !strings.EqualFold(strings.TrimSpace(v), "userSpaceOnUse")
	}
	contentOBB := false // patternContentUnits по умолчанию userSpaceOnUse
	if v, ok := patternChainAttr(chain, "patternContentUnits"); ok {
		contentOBB = strings.EqualFold(strings.TrimSpace(v), "objectBoundingBox")
	}
	vbStr, hasVB := patternChainAttr(chain, "viewBox")
	var vb [4]float64
	if hasVB {
		f := parseFloats(vbStr)
		if len(f) == 4 && f[2] > 0 && f[3] > 0 {
			vb = [4]float64{f[0], f[1], f[2], f[3]}
		} else {
			hasVB = false
		}
	}

	var bb bboxRect
	if unitsOBB || (contentOBB && !hasVB) {
		x0, y0, x1, y1, ok := contoursBBox(contours)
		if !ok || x1-x0 <= 0 || y1-y0 <= 0 {
			return nil
		}
		bb = bboxRect{x0, y0, x1 - x0, y1 - y0}
	}
	key := patKey{n: n, m: st.transform, bb: bb.key()}
	if p, ok := b.shared.pats[key]; ok {
		return p
	}

	geom := func(name string, size, bbOff, bbSize float64) float64 {
		v, _ := patternChainAttr(chain, name)
		f := b.coordUnits(v, unitsOBB, size)
		if unitsOBB {
			return bbOff + f*bbSize
		}
		return f
	}
	x := geom("x", b.vpW, bb.x, bb.w)
	y := geom("y", b.vpH, bb.y, bb.h)
	var w, h float64
	if unitsOBB {
		w = geom("width", b.vpW, 0, bb.w)
		h = geom("height", b.vpH, 0, bb.h)
	} else {
		w = geom("width", b.vpW, 0, 0)
		h = geom("height", b.vpH, 0, 0)
	}
	if !(w > 0) || !(h > 0) || math.IsInf(w, 0) || math.IsInf(h, 0) {
		return nil
	}

	pt := Identity()
	if v, ok := patternChainAttr(chain, "patternTransform"); ok {
		pt = ParseTransform(v)
	}
	pat := &Pattern{Tile: [4]float64{x, y, w, h}, M: st.transform.Mul(pt)}

	content := Identity()
	switch {
	case hasVB:
		par, _ := patternChainAttr(chain, "preserveAspectRatio")
		content = viewBoxMatrix(vb, par, 0, 0, w, h)
	case contentOBB:
		content = ScaleM(bb.w, bb.h)
	}

	// Содержимое — у первого узора цепочки, у которого оно есть.
	var src *xnode
	for _, c := range chain {
		if len(c.Nodes) > 0 {
			src = c
			break
		}
	}
	if b.shared.pats == nil {
		b.shared.pats = map[patKey]*Pattern{}
	}
	b.shared.pats[key] = pat // до разбора: ссылка узора на себя не зациклится
	if src == nil {
		return pat
	}
	b.shared.activeRef[n] = true
	defer delete(b.shared.activeRef, n)

	sub := b.sub()
	base := defaultInherited()
	base.transform = content
	st0 := sub.resolveState(src, base, sub.props(src))
	st0.transform = content // transform самого узора не бывает: только patternTransform
	for i := range src.Nodes {
		sub.walk(&src.Nodes[i], st0, 1)
	}
	pat.Shapes = sub.doc.Shapes
	return pat
}

// patTile — растрированная плитка: пиксели и масштаб «единица узора → пиксель».
type patTile struct {
	img    *image.RGBA // premultiplied
	kx, ky float64
}

// patPainter красит пиксели устройства выборкой из плитки с повтором.
type patPainter struct {
	t      *patTile
	d2p    Matrix  // устройство → система узора
	x0, y0 float64 // начало плитки
	w, h   float64 // размер плитки в единицах узора
	tw, th int
}

// newPatPainter готовит краску для узора; nil — рисовать нечего.
func (c *rctx) newPatPainter(p *Pattern) *patPainter {
	if p == nil {
		return nil
	}
	inv, ok := p.M.inverse()
	if !ok {
		return nil
	}
	t := c.tile(p)
	if t == nil {
		return nil
	}
	b := t.img.Bounds()
	return &patPainter{
		t: t, d2p: inv.Mul(c.dev2vb),
		x0: p.Tile[0], y0: p.Tile[1], w: p.Tile[2], h: p.Tile[3],
		tw: b.Dx(), th: b.Dy(),
	}
}

// tile растрирует плитку узора (раз за растеризацию).
func (c *rctx) tile(p *Pattern) *patTile {
	if t, ok := c.tiles[p]; ok {
		return t
	}
	if c.tiles == nil {
		c.tiles = map[*Pattern]*patTile{}
	}
	var t *patTile
	defer func() { c.tiles[p] = t }()

	// Масштаб единица узора → пиксель устройства.
	vb2dev := Matrix{A: c.s, D: c.s, E: c.tx, F: c.ty}
	dm := vb2dev.Mul(p.M)
	sx := math.Hypot(dm.A, dm.B)
	sy := math.Hypot(dm.C, dm.D)
	w, h := p.Tile[2], p.Tile[3]
	tw, th := math.Ceil(w*sx), math.Ceil(h*sy)
	if !(tw >= 1) {
		tw = 1
	}
	if !(th >= 1) {
		th = 1
	}
	if tw > maxPatternTile {
		tw = maxPatternTile
	}
	if th > maxPatternTile {
		th = maxPatternTile
	}
	if tw*th > maxPatternTilePixels {
		k := math.Sqrt(maxPatternTilePixels / (tw * th))
		tw, th = math.Max(1, math.Floor(tw*k)), math.Max(1, math.Floor(th*k))
	}
	iw, ih := int(tw), int(th)
	kx, ky := tw/w, th/h
	img := image.NewRGBA(image.Rect(0, 0, iw, ih))
	sub := &rctx{
		w: iw, h: ih, s: (kx + ky) / 2,
		current: c.current, tint: c.tint, opts: c.opts, div: 1,
		mapPt: func(pt Point) (float32, float32) {
			return float32(pt.X * kx), float32(pt.Y * ky)
		},
		dev2vb: Matrix{A: 1 / kx, D: 1 / ky},
	}
	sub.drawShapes(img, p.Shapes)
	t = &patTile{img: img, kx: kx, ky: ky}
	return t
}

// at возвращает не премультиплицированный цвет пикселя устройства (x,y):
// билинейная выборка из плитки с заворотом по краям.
func (p *patPainter) at(x, y int) color.RGBA {
	px, py := float64(x)+0.5, float64(y)+0.5
	u := p.d2p.A*px + p.d2p.C*py + p.d2p.E - p.x0
	v := p.d2p.B*px + p.d2p.D*py + p.d2p.F - p.y0
	u -= math.Floor(u/p.w) * p.w
	v -= math.Floor(v/p.h) * p.h
	fu := u*p.t.kx - 0.5
	fv := v*p.t.ky - 0.5
	x0, y0 := int(math.Floor(fu)), int(math.Floor(fv))
	ax, ay := fu-float64(x0), fv-float64(y0)
	var acc [4]float64
	for dy := 0; dy < 2; dy++ {
		wy := ay
		if dy == 0 {
			wy = 1 - ay
		}
		yy := wrapInt(y0+dy, p.th)
		for dx := 0; dx < 2; dx++ {
			wx := ax
			if dx == 0 {
				wx = 1 - ax
			}
			xx := wrapInt(x0+dx, p.tw)
			o := p.t.img.PixOffset(xx, yy)
			wgt := wx * wy
			acc[0] += float64(p.t.img.Pix[o]) * wgt
			acc[1] += float64(p.t.img.Pix[o+1]) * wgt
			acc[2] += float64(p.t.img.Pix[o+2]) * wgt
			acc[3] += float64(p.t.img.Pix[o+3]) * wgt
		}
	}
	a := acc[3]
	if a < 0.5 {
		return color.RGBA{}
	}
	return color.RGBA{
		R: clampByte(acc[0] * 255 / a),
		G: clampByte(acc[1] * 255 / a),
		B: clampByte(acc[2] * 255 / a),
		A: clampByte(a),
	}
}

func wrapInt(v, n int) int {
	v %= n
	if v < 0 {
		v += n
	}
	return v
}
