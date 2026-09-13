package widget

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/oops1/headless-gui/v3/widget/diffview"
)

// diffview_draw.go — отрисовка контрола сравнения.

func (d *DiffView) Draw(ctx DrawContext) {
	d.mu.Lock()
	defer d.mu.Unlock()
	b := d.Bounds()
	if b.Empty() {
		return
	}
	outer := ctx.Clip()
	defer ctx.SetClip(outer)
	ctx.SetClip(b.Intersect(outer))

	p := &d.pal
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), p.bg)
	if w := ctx.MeasureTextFont("MMMMMMMMMM", d.fontSize, d.monoFont); w > 0 {
		d.charW = float64(w) / 10
	}
	g := d.geom()
	viewH := float64(g.cy1 - g.cy0)
	lt, rt := d.offsets(viewH)

	if d.headers {
		d.drawHeader(ctx, g, DiffLeft)
		d.drawHeader(ctx, g, DiffRight)
	}

	content := image.Rect(b.Min.X, g.cy0, b.Max.X, g.cy1).Intersect(outer)
	d.drawPane(ctx, g, DiffLeft, lt, content)
	d.drawPane(ctx, g, DiffRight, rt, content)
	d.drawConnectors(ctx, g, lt, rt, content)
	d.drawRuler(ctx, g, viewH)
	ctx.SetClip(outer)
	d.drawChildren(ctx)
}

func (d *DiffView) roundRect(ctx DrawContext, x, y, w, h int, fill, edge color.RGBA) {
	dvRoundRect(ctx, &d.pal, x, y, w, h, fill, edge)
}

// dvRoundRect — карточка палитры: скругление и тень из неё же. Свободная
// функция: тем же приёмом рисует панели контрол слияния.
func dvRoundRect(ctx DrawContext, p *dvPalette, x, y, w, h int, fill, edge color.RGBA) {
	if p.shadow {
		if sd, ok := ctx.(ShadowDrawer); ok {
			sd.DrawSoftShadow(image.Rect(x, y, x+w, y+h), p.radius, 1, color.RGBA{A: 18})
		}
	}
	if p.radius > 0 {
		ctx.FillRoundRect(x, y, w, h, p.radius, fill)
		ctx.DrawRoundBorder(x, y, w, h, p.radius, edge)
		return
	}
	ctx.FillRect(x, y, w, h, fill)
	ctx.DrawBorder(x, y, w, h, edge)
}

func (d *DiffView) drawHeader(ctx DrawContext, g dvGeom, side DiffSide) {
	p := &d.pal
	s := d.docs[side]
	x0, x1 := g.lx0, g.lx1
	dot, band := p.del, p.delBand
	if side == DiffRight {
		x0, x1 = g.rx0, g.rx1
		dot, band = p.add, p.addBand
	}
	y, h := g.b.Min.Y+8, 34
	edge := p.cardEdge
	if d.focused && d.active == side {
		edge = p.accent
	}
	d.roundRect(ctx, x0, y, x1-x0, h, p.card, edge)
	fillEllipse(ctx, x0+18, y+h/2, 5, 5, dot)

	name := s.title
	if name == "" {
		name = Tr("diff.empty")
	}
	ty := y + (h-17)/2
	tx := x0 + 32
	ctx.DrawTextFont(name, tx, ty, d.fontSize, d.boldFont, p.text)
	tx += ctx.MeasureTextFont(name, d.fontSize, d.boldFont) + 10

	n := 0
	for _, c := range d.chunks {
		switch {
		case c.Kind == DiffEqual:
		case side == DiffRight:
			n += c.RightTo - c.RightFrom
		default:
			n += c.LeftTo - c.LeftFrom
		}
	}
	badge := fmt.Sprintf("−%d", n)
	if side == DiffRight {
		badge = fmt.Sprintf("+%d", n)
	}
	bw := ctx.MeasureText(badge, d.fontSize) + 16
	bx := x1 - bw - 10
	ctx.FillRoundRect(bx, y+7, bw, h-14, 10, band)
	ctx.DrawText(badge, bx+8, ty, dot)

	right := bx - 10
	mark := func(text string, col color.RGBA) {
		w := ctx.MeasureText(text, d.fontSize)
		right -= w
		ctx.DrawText(text, right, ty, col)
		right -= 10
	}
	if s.modified() {
		mark(Tr("diff.modified"), p.dirty)
	}
	if s.readOnly && d.showRO {
		mark(Tr("diff.readonly"), p.muted)
	}
	if s.note != "" && tx < right {
		prev := ctx.Clip()
		ctx.SetClip(image.Rect(tx, y, right, y+h).Intersect(prev))
		ctx.DrawText(s.note, tx, ty, p.muted)
		ctx.SetClip(prev)
	}
}

func (d *DiffView) drawPane(ctx DrawContext, g dvGeom, side DiffSide, top float64, content image.Rectangle) {
	p := &d.pal
	s := d.docs[side]
	x0, x1, codeX, codeR := d.paneGeom(g, side)
	clip := image.Rect(x0-2, g.cy0, x1+2, g.cy1).Intersect(content)
	if clip.Empty() {
		return
	}
	ctx.SetClip(clip)
	cw := d.charW
	rowY := func(r int) int { return g.cy0 + dvTopPad + r*dvLineH - int(top) }
	r0 := max(0, (int(top)-dvTopPad)/dvLineH-1)
	r1 := min(len(s.rows), r0+(g.cy1-g.cy0)/dvLineH+3)

	for _, c := range s.cards {
		if c[1] < r0 || c[0] > r1 {
			continue
		}
		y0, y1 := rowY(c[0])-dvCardExt, rowY(c[1])+dvCardExt
		d.roundRect(ctx, x0, y0, x1-x0, y1-y0, p.card, p.cardEdge)
	}

	band, strong := p.delBand, p.delStrong
	if side == DiffRight {
		band, strong = p.addBand, p.addStrong
	}
	codeClip := image.Rect(codeX-2, g.cy0, codeR, g.cy1).Intersect(clip)
	hs := int(d.hscroll)
	selA, selB := s.sel()
	showSel := s.hasSel()
	showCaret := d.focused && d.active == side

	for r := r0; r < r1; r++ {
		row := s.rows[r]
		y := rowY(r)
		if row.kind == dvRowFold {
			txt := Trf("diff.fold", row.line)
			tw := ctx.MeasureText(txt, d.fontSize-1)
			cx, my := (x0+x1)/2, y+dvLineH/2
			ctx.DrawHLine(x0+12, my, max(0, cx-tw/2-10-(x0+12)), p.cardEdge)
			ctx.DrawHLine(cx+tw/2+10, my, max(0, x1-12-(cx+tw/2+10)), p.cardEdge)
			ctx.DrawTextSize(txt, cx-tw/2, y+dvTextDY+1, d.fontSize-1, p.muted)
			continue
		}
		changed := d.chunks[row.chunk].Kind != DiffEqual
		if row.kind == dvRowGap && changed {
			ctx.FillRoundRect(x0+10, y+dvCardExt+2, x1-x0-20, dvLineH-2*dvCardExt-4, 3, strong)
		}
		if row.kind == dvRowLine {
			if changed {
				ctx.FillRect(codeX-8, y, codeR-(codeX-8), dvLineH, band)
			}
			if row.chunk == d.current && changed {
				ctx.FillRect(x0+3, y, 3, dvLineH, p.accent)
			}
			num := strconv.Itoa(row.line + 1)
			nc := p.num
			if changed || (showCaret && row.line == s.caret.line) {
				nc = p.muted
			}
			nw := ctx.MeasureTextFont(num, d.fontSize, d.monoFont)
			ctx.DrawTextFont(num, codeX-14-nw, y+dvTextDY, d.fontSize, d.monoFont, nc)
		}

		ctx.SetClip(codeClip)
		rs := s.disp[row.line]
		if h := s.hl[row.line]; row.kind == dvRowLine && changed && h[0] >= 0 && h[1] > h[0] {
			hx := codeX + int(float64(h[0])*cw) - hs
			ctx.FillRect(hx-1, y+2, int(float64(h[1]-h[0])*cw)+2, dvLineH-4, strong)
		}
		if showSel && row.line >= selA.line && row.line <= selB.line {
			raw := []rune(s.text.Lines[row.line])
			c0, c1 := 0, len(rs)+1
			if row.line == selA.line {
				c0 = diffview.DisplayCol(raw, selA.col)
			}
			if row.line == selB.line {
				c1 = diffview.DisplayCol(raw, selB.col)
			}
			if c1 > c0 {
				ctx.FillRect(codeX+int(float64(c0)*cw)-hs, y+1, int(float64(c1-c0)*cw), dvLineH-2, p.sel)
			}
		}
		if row.kind == dvRowLine {
			d.drawCode(ctx, s, row.line, codeX-hs, y+dvTextDY)
		}
		if showCaret && row.line == s.caret.line {
			cx := codeX + int(float64(diffview.DisplayCol([]rune(s.text.Lines[row.line]), s.caret.col))*cw) - hs
			ctx.FillRect(cx, y+2, 2, dvLineH-4, p.caret)
		}
		ctx.SetClip(clip)
	}

	// Черта на месте вставки, если у блока на этой стороне нет строк.
	for ci, sp := range d.spans {
		if d.chunks[ci].Kind == DiffEqual {
			continue
		}
		a, bnd := sp.lr0, sp.lr1
		if side == DiffRight {
			a, bnd = sp.rr0, sp.rr1
		}
		if a != bnd || a < r0-1 || a > r1+1 {
			continue
		}
		col := premulAlpha(p.accent, 150)
		if ci == d.current || ci == d.hoverChunk {
			col = p.accent
		}
		ctx.FillRect(codeX-8, rowY(a)-1, codeR-(codeX-8), 2, col)
	}
}

func (d *DiffView) drawCode(ctx DrawContext, s *dvDoc, line, x, y int) {
	dvDrawCode(ctx, &d.pal, s, line, x, y, d.syntax, d.charW, d.fontSize, d.monoFont)
}

// dvDrawCode рисует строку кода с подсветкой синтаксиса. Свободная функция:
// тем же кодом рисует строки контрол слияния.
func dvDrawCode(ctx DrawContext, p *dvPalette, s *dvDoc, line, x, y int, syntax bool, charW, fontSize float64, monoFont string) {
	rs := s.disp[line]
	if len(rs) == 0 {
		return
	}
	if !syntax || s.toks[line] == nil {
		ctx.DrawTextFont(string(rs), x, y, fontSize, monoFont, p.text)
		return
	}
	for _, t := range s.toks[line] {
		col := p.text
		switch t.Kind {
		case diffview.TokenKeyword:
			col = p.kw
		case diffview.TokenString:
			col = p.str
		case diffview.TokenComment:
			col = p.com
		case diffview.TokenNumber:
			col = p.numLit
		case diffview.TokenFunc:
			col = p.fn
		}
		ctx.DrawTextFont(string(rs[t.Start:t.End]), x+int(float64(t.Start)*charW), y, fontSize, monoFont, col)
	}
}

func (d *DiffView) drawConnectors(ctx DrawContext, g dvGeom, lt, rt float64, content image.Rectangle) {
	p := &d.pal
	gutter := image.Rect(g.lx1, g.cy0, g.rx0, g.cy1).Intersect(content)
	if gutter.Empty() {
		return
	}
	top, bot := float64(g.cy0-dvCircleR*2), float64(g.cy1+dvCircleR*2)
	visible := func(ci int) (lx, ly, rx, ry float64, ok bool) {
		lx, ly, rx, ry = d.circleCenters(g, ci, lt, rt)
		ok = !((ly < top && ry < top) || (ly > bot && ry > bot))
		return
	}
	// Кнопки сидят на краях карточек: сначала все линии, потом все кнопки.
	curve := func(ci int, active bool) {
		lx, ly, rx, ry, ok := visible(ci)
		if !ok {
			return
		}
		col, thick := premulAlpha(p.accent, 130), 1.5
		if active {
			col, thick = p.accent, 2.2
		}
		// Концы — в центрах кнопок; стык скрыт под кругом.
		c := dvConnector{math.Floor(lx), math.Floor(ly), math.Floor(rx), math.Floor(ry)}
		if pts := c.visible(top, bot); len(pts) >= 2 {
			strokePath(ctx, pts, thick, false, col)
		}
	}
	buttons := func(ci int, active bool) {
		lx, ly, rx, ry, ok := visible(ci)
		if !ok {
			return
		}
		ring := p.card
		if active {
			ring = p.accent
		}
		lr, rr, lc, rc := dvCircleR, dvCircleR, p.del, p.add
		if d.hoverBtn == ci*2 {
			lr, lc = lr+1, p.delHover
		}
		if d.hoverBtn == ci*2+1 {
			rr, rc = rr+1, p.addHover
		}
		dvArrowButton(ctx, int(math.Floor(lx)), int(math.Floor(ly)), lr, lc, ring, true)
		dvArrowButton(ctx, int(math.Floor(rx)), int(math.Floor(ry)), rr, rc, ring, false)
	}
	pass := func(draw func(int, bool)) {
		for ci, c := range d.chunks {
			if c.Kind == DiffEqual || ci == d.current || ci == d.hoverChunk {
				continue
			}
			draw(ci, false)
		}
		if d.hoverChunk >= 0 && d.hoverChunk != d.current {
			draw(d.hoverChunk, true)
		}
		if d.current >= 0 && d.chunks[d.current].Kind != DiffEqual {
			draw(d.current, true)
		}
	}
	ctx.SetClip(gutter)
	pass(curve)
	ctx.SetClip(content)
	pass(buttons)
}

// dvArrowButton — круглая кнопка переноса блока со стрелкой.
func dvArrowButton(ctx DrawContext, cx, cy, r int, col, ring color.RGBA, pointRight bool) {
	fillEllipse(ctx, cx, cy, r+2, r+2, ring)
	fillEllipse(ctx, cx, cy, r, r, col)
	white := dvRGB(0xFFFFFF)
	dir := 1
	if !pointRight {
		dir = -1
	}
	tip, tail := image.Pt(cx+dir*5, cy), image.Pt(cx-dir*5, cy)
	drawThickLine(ctx, tail.X, tail.Y, tip.X-dir*2, tip.Y, 2, white)
	fillPolygon(ctx, []image.Point{
		{X: tip.X + dir, Y: tip.Y},
		{X: tip.X - dir*4, Y: tip.Y - 4},
		{X: tip.X - dir*4, Y: tip.Y + 4},
	}, white)
}

func (d *DiffView) drawRuler(ctx DrawContext, g dvGeom, viewH float64) {
	p := &d.pal
	tr := d.rulerTrack(g)
	if tr.Dy() <= 0 {
		return
	}
	ctx.SetClip(tr.Inset(-2).Intersect(g.b))
	ctx.FillRoundRect(tr.Min.X, tr.Min.Y, tr.Dx(), tr.Dy(), dvRulerW/2, p.track)
	V := d.bps[len(d.bps)-1].v
	scale := float64(tr.Dy()) / V
	half := tr.Dx() / 2
	for ci, c := range d.chunks {
		if c.Kind == DiffEqual {
			continue
		}
		y0 := tr.Min.Y + int(d.bps[ci+1].v*scale)
		h := max(3, int((d.bps[ci+2].v-d.bps[ci+1].v)*scale))
		switch c.Kind {
		case DiffDelete:
			ctx.FillRect(tr.Min.X+2, y0, tr.Dx()-4, h, p.del)
		case DiffInsert:
			ctx.FillRect(tr.Min.X+2, y0, tr.Dx()-4, h, p.add)
		default:
			ctx.FillRect(tr.Min.X+2, y0, half-2, h, p.del)
			ctx.FillRect(tr.Min.X+half, y0, tr.Dx()-half-2, h, p.add)
		}
		if ci == d.current {
			ctx.DrawBorder(tr.Min.X, y0-1, tr.Dx(), h+2, p.accent)
		}
	}
	if d.maxScroll(viewH) > 0 {
		ty := tr.Min.Y + int(d.scroll*scale)
		th := max(18, int(viewH*scale))
		ctx.FillRectAlpha(tr.Min.X, ty, tr.Dx(), min(th, tr.Max.Y-ty), premulAlpha(p.thumb, 110))
	}
}

// ─── S-коннектор ────────────────────────────────────────────────────────────

// dvCurveHandle — жёсткость ручек Безье: доля горизонтального пролёта.
const dvCurveHandle = 0.5

// dvConnector — S-кривая с горизонтальными касательными на концах. По y это
// smoothstep, поэтому она монотонна по высоте и режется по полосе видимости
// точно: блоки, уехавшие за край окна на тысячи строк, не заставляют
// раскладывать кривую целиком.
type dvConnector struct{ x0, y0, x3, y3 float64 }

func (c dvConnector) at(t float64) Point2F {
	h := (c.x3 - c.x0) * dvCurveHandle
	p1, p2 := c.x0+h, c.x3-h
	mt := 1 - t
	return Point2F{
		X: mt*mt*mt*c.x0 + 3*mt*mt*t*p1 + 3*mt*t*t*p2 + t*t*t*c.x3,
		Y: c.y0 + (c.y3-c.y0)*t*t*(3-2*t),
	}
}

// tAtY — параметр, где кривая достигает высоты y (с зажимом в [0,1]).
func (c dvConnector) tAtY(y float64) float64 {
	dy := c.y3 - c.y0
	if dy == 0 {
		return 0
	}
	s := math.Max(0, math.Min(1, (y-c.y0)/dy))
	lo, hi := 0.0, 1.0
	for i := 0; i < 40; i++ {
		m := (lo + hi) / 2
		if m*m*(3-2*m) < s {
			lo = m
		} else {
			hi = m
		}
	}
	return (lo + hi) / 2
}

// visible — точки участка кривой внутри полосы [top, bot]. Точки дробные: их
// обводит холст одним сглаженным контуром (PathShapes), без округления до
// пикселя, которое делало кривую ломаной.
func (c dvConnector) visible(top, bot float64) []Point2F {
	ta, tb := 0.0, 1.0
	if c.y3 != c.y0 {
		ta, tb = c.tAtY(top), c.tAtY(bot)
		if ta > tb {
			ta, tb = tb, ta
		}
	} else if c.y0 < top || c.y0 > bot {
		return nil
	}
	if tb-ta <= 0 {
		return nil
	}
	span := math.Min(math.Abs(c.y3-c.y0), bot-top) + math.Abs(c.x3-c.x0)
	n := max(16, min(600, int(span/2)))
	pts := make([]Point2F, 0, n+1)
	for i := 0; i <= n; i++ {
		pts = append(pts, c.at(ta+(tb-ta)*float64(i)/float64(n)))
	}
	return pts
}
