package base

import (
	"image"
	"image/color"
	"math"
	"time"
)

// diffview_collapse.go — свёрнутая сторона и граница между сторонами (GG-95).
//
// Свёрнутая сторона — узкая полоса с кнопкой «развернуть»; вторая сторона
// занимает всю освободившуюся ширину, коннекторы и кнопки переноса блоков не
// рисуются. Подсветка изменений, полоса-обзор, переход по изменениям, правка и
// сохранение работают как прежде. Свернуть можно не больше одной стороны.
// Граница между развёрнутыми сторонами тянется мышью за промежуток с
// коннекторами; двойной щелчок по нему возвращает 50/50.

const (
	dvStripW    = 28   // ширина полосы свёрнутой стороны
	dvStripGap  = 10   // зазор между полосой и развёрнутой стороной
	dvFoldBtnW  = 24   // кнопка-шеврон «свернуть» в шапке
	dvPaneMinW  = 80   // самая узкая сторона при перетаскивании границы
	dvSplitHalf = 0.5  // доля по умолчанию
	dvSplitMin  = 0.15 // пределы доли левой стороны
	dvSplitMax  = 0.85
)

// SetCollapsed сворачивает или разворачивает сторону. Свёрнутой может быть
// только одна: сворачивание второй разворачивает первую. Если свернули
// активную сторону, активной становится видимая (каретка и
// OnActiveSideChanged). Событие OnCollapsedChanged приходит на каждую сторону,
// чьё состояние изменилось.
func (d *DiffView) SetCollapsed(side DiffSide, v bool) {
	if side != DiffLeft && side != DiffRight {
		return
	}
	d.do(func() { d.setCollapsedLocked(side, v) })
}

func (d *DiffView) setCollapsedLocked(side DiffSide, v bool) {
	if d.coll[side] == v {
		return
	}
	other := 1 - side
	d.coll[side] = v
	if v {
		d.coll[other] = false
		if d.active == side {
			d.active = other
		}
	}
	d.hoverChunk, d.hoverBtn, d.hoverFold = -1, -1, -1
	d.dragSel, d.splitDrag = false, false
	// Ширина кода изменилась — прокрутка вбок могла выйти за край.
	d.setHScrollLocked(d.hscroll)
	d.setScrollLocked(d.scroll)
}

// Collapsed сообщает, свёрнута ли сторона.
func (d *DiffView) Collapsed(side DiffSide) bool {
	if side != DiffLeft && side != DiffRight {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.coll[side]
}

// SetShowCollapseButtons показывает или прячет кнопки «свернуть» в шапках
// сторон — для приложения, которое ставит свои кнопки. Полоса свёрнутой
// стороны разворачивается щелчком и без них.
func (d *DiffView) SetShowCollapseButtons(v bool) {
	d.do(func() { d.foldBtns = v })
}

// ShowCollapseButtons сообщает, показаны ли кнопки «свернуть».
func (d *DiffView) ShowCollapseButtons() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.foldBtns
}

// SetSplit задаёт долю ширины левой стороны, когда развёрнуты обе: 0.5 —
// поровну. Значение зажимается в 0.15…0.85, нечисловое — 0.5. Событие
// OnSplitChanged приходит, если доля изменилась.
func (d *DiffView) SetSplit(ratio float64) {
	d.do(func() { d.setSplitLocked(ratio) })
}

func (d *DiffView) setSplitLocked(ratio float64) {
	if math.IsNaN(ratio) {
		ratio = dvSplitHalf
	}
	ratio = math.Max(dvSplitMin, math.Min(dvSplitMax, ratio))
	if ratio == d.split {
		return
	}
	d.split = ratio
	d.splitEvent = true
	d.setHScrollLocked(d.hscroll)
}

// Split — доля ширины левой стороны, когда развёрнуты обе.
func (d *DiffView) Split() float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.split
}

// paneWidths делит ширину avail (обе стороны и промежуток) по доле. При доле
// 0.5 — прежней формулой: кадры сравнения по умолчанию не меняются ни на
// пиксель.
func (d *DiffView) paneWidths(avail int) (lw, rw int) {
	both := avail - dvGutterW
	if d.split == dvSplitHalf || both < 2*dvPaneMinW {
		pw := max(both/2, dvPaneMinW)
		return pw, pw
	}
	lw = int(math.Round(float64(both) * d.split))
	lw = max(dvPaneMinW, min(lw, both-dvPaneMinW))
	return lw, both - lw
}

// layoutSides раскладывает стороны по ширине: свёрнутая — полоса у своего
// края, развёрнутая — всё остальное; обе развёрнуты — по доле split.
func (d *DiffView) layoutSides(g *dvGeom) {
	b := g.b
	g.lx0 = b.Min.X + dvOuterPad
	right := g.rulerX - dvOuterPad // правый край правой стороны
	g.fold = d.coll
	switch {
	case d.coll[DiffLeft]:
		g.lx1 = g.lx0 + dvStripW
		g.rx0 = g.lx1 + dvStripGap
		g.rx1 = max(right, g.rx0+dvPaneMinW)
	case d.coll[DiffRight]:
		g.lx1 = max(right-dvStripW-dvStripGap, g.lx0+dvPaneMinW)
		g.rx0 = g.lx1 + dvStripGap
		g.rx1 = g.rx0 + dvStripW
	default:
		lw, rw := d.paneWidths(right - g.lx0)
		g.lx1 = g.lx0 + lw
		g.rx0 = g.lx1 + dvGutterW
		g.rx1 = g.rx0 + rw
	}
}

// sideX — края стороны в геометрии g.
func (g dvGeom) sideX(side DiffSide) (x0, x1 int) {
	if side == DiffRight {
		return g.rx0, g.rx1
	}
	return g.lx0, g.lx1
}

// codeWLocked — ширина области кода, по которой считается прокрутка вбок:
// самая узкая из развёрнутых сторон (ей нужно увести код дальше всех).
func (d *DiffView) codeWLocked(g dvGeom) float64 {
	w := math.Inf(1)
	for _, side := range []DiffSide{DiffLeft, DiffRight} {
		if g.fold[side] {
			continue
		}
		_, _, codeX, codeR := d.paneGeom(g, side)
		w = math.Min(w, float64(codeR-codeX))
	}
	if math.IsInf(w, 1) {
		return 0
	}
	return w
}

// stripRect — полоса свёрнутой стороны (пусто — сторона развёрнута).
func (d *DiffView) stripRect(g dvGeom, side DiffSide) image.Rectangle {
	if !g.fold[side] {
		return image.Rectangle{}
	}
	x0, x1 := g.sideX(side)
	y0 := g.cy0 + 4
	if d.headers {
		y0 = g.b.Min.Y + 8
	}
	return image.Rect(x0, y0, x1, max(g.cy1-4, y0+dvFoldBtnW))
}

// foldBtnRect — кнопка «свернуть» в шапке развёрнутой стороны (пусто — её нет).
func (d *DiffView) foldBtnRect(g dvGeom, side DiffSide) image.Rectangle {
	if !d.headers || !d.foldBtns || g.fold[side] {
		return image.Rectangle{}
	}
	_, x1 := g.sideX(side)
	y := g.b.Min.Y + 8 + (34-dvFoldBtnW)/2
	return image.Rect(x1-8-dvFoldBtnW, y, x1-8, y+dvFoldBtnW)
}

// hitFoldLocked — сторона, чью кнопку «свернуть» или полосу «развернуть»
// задевает точка; -1 — мимо.
func (d *DiffView) hitFoldLocked(x, y int) DiffSide {
	g := d.geom()
	pt := image.Pt(x, y)
	for _, side := range []DiffSide{DiffLeft, DiffRight} {
		if pt.In(d.stripRect(g, side)) || pt.In(d.foldBtnRect(g, side)) {
			return side
		}
	}
	return -1
}

// gutterHitLocked — точка над промежутком между развёрнутыми сторонами, но не
// над кнопкой переноса: здесь тянут границу.
func (d *DiffView) gutterHitLocked(x, y int) bool {
	g := d.geom()
	if g.fold[DiffLeft] || g.fold[DiffRight] {
		return false
	}
	if x < g.lx1 || x >= g.rx0 || y < g.b.Min.Y || y >= g.cy1 {
		return false
	}
	return d.hitButtonLocked(x, y) < 0
}

// splitPressLocked — нажатие на промежуток: двойной щелчок возвращает 50/50,
// одинарный начинает перетаскивание без скачка.
func (d *DiffView) splitPressLocked(x int) {
	now := time.Now()
	if now.Sub(d.splitClickAt) < 400*time.Millisecond {
		d.splitClickAt = time.Time{}
		d.setSplitLocked(dvSplitHalf)
		return
	}
	d.splitClickAt = now
	g := d.geom()
	d.splitDrag = true
	d.splitGrab = x - (g.lx1+g.rx0)/2
}

// splitDragToLocked ведёт границу за курсором.
func (d *DiffView) splitDragToLocked(x int) {
	g := d.geom()
	both := g.rulerX - dvOuterPad - g.lx0 - dvGutterW
	if both <= 0 {
		return
	}
	lw := x - d.splitGrab - dvGutterW/2 - g.lx0
	before := d.split
	d.setSplitLocked(float64(lw) / float64(both))
	if d.split != before {
		// Границу протянули: нажатие, начавшее перетаскивание, — не первый
		// щелчок двойного, и быстрое повторное нажатие долю не сбрасывает.
		d.splitClickAt = time.Time{}
	}
}

// drawStrip рисует свёрнутую сторону: полоса-карточка с цветной меткой
// стороны и шевроном «развернуть».
func (d *DiffView) drawStrip(ctx DrawContext, g dvGeom, side DiffSide) {
	r := d.stripRect(g, side)
	if r.Empty() {
		return
	}
	p := &d.pal
	fill := p.card
	if d.hoverFold == side {
		fill = p.track
	}
	d.roundRect(ctx, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), fill, p.cardEdge)
	cx := (r.Min.X + r.Max.X) / 2
	dot := p.del
	dir := 1 // развернуть левую — шеврон вправо
	if side == DiffRight {
		dot, dir = p.add, -1
	}
	cy := r.Min.Y + 17
	dvChevron(ctx, cx, cy, dir, p.text)
	fillEllipse(ctx, cx, cy+18, 4, 4, dot)
}

// drawFoldButton рисует кнопку «свернуть» в шапке стороны.
func (d *DiffView) drawFoldButton(ctx DrawContext, g dvGeom, side DiffSide) {
	r := d.foldBtnRect(g, side)
	if r.Empty() {
		return
	}
	p := &d.pal
	if d.hoverFold == side {
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, p.track)
	}
	dir := -1 // свернуть левую — к левому краю
	if side == DiffRight {
		dir = 1
	}
	dvChevron(ctx, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, dir, p.muted)
}

// dvChevron — шеврон «‹» (dir < 0) или «›» (dir > 0) с центром (cx, cy).
func dvChevron(ctx DrawContext, cx, cy, dir int, col color.RGBA) {
	tip := cx + dir*2
	back := cx - dir*2
	drawThickLine(ctx, back, cy-4, tip, cy, 2, col)
	drawThickLine(ctx, tip, cy, back, cy+4, 2, col)
}

// dvChevronV — шеврон «вверх» (dir < 0) или «вниз» (dir > 0).
func dvChevronV(ctx DrawContext, cx, cy, dir int, col color.RGBA) {
	tip := cy + dir*2
	back := cy - dir*2
	drawThickLine(ctx, cx-4, back, cx, tip, 2, col)
	drawThickLine(ctx, cx, tip, cx+4, back, 2, col)
}
