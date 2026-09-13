package widget

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v3/widget/diffview"
)

// mergeview_draw.go — отрисовка контрола слияния.

// Кнопки решения: у конфликтного блока их две — «взять наше» и «взять их».
// Кодируются одним числом, как стрелки переноса в DiffView: ci*2 + слот.
const (
	mvBtnOurs   = 0
	mvBtnTheirs = 1
	mvBtnSlots  = 2
)

func (m *MergeView) Draw(ctx DrawContext) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.Bounds()
	if b.Empty() {
		return
	}
	outer := ctx.Clip()
	defer ctx.SetClip(outer)
	ctx.SetClip(b.Intersect(outer))

	p := &m.pal
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), p.bg)
	if w := ctx.MeasureTextFont("MMMMMMMMMM", m.fontSize, m.monoFont); w > 0 {
		m.charW = float64(w) / 10
	}
	g := m.geom()

	// Сначала ВСЕ шапки, потом панели: панель рисуется со своим отсечением, и
	// шапка соседа, нарисованная между ними, была бы им обрезана.
	for i := 0; i < 3; i++ {
		if i == int(MergeBase) && !m.showBase {
			continue
		}
		m.drawHeader(ctx, g, MergeSide(i))
	}
	m.drawHeader(ctx, g, MergeResult)
	for i := 0; i < 3; i++ {
		if i == int(MergeBase) && !m.showBase {
			continue
		}
		m.drawPane(ctx, g, MergeSide(i), outer)
		ctx.SetClip(b.Intersect(outer))
	}
	m.drawPane(ctx, g, MergeResult, outer)
	ctx.SetClip(b.Intersect(outer))
	m.drawButtons(ctx, g, outer)
	ctx.SetClip(b.Intersect(outer))
	m.drawSplitter(ctx, g)
	m.drawRuler(ctx, g)
	ctx.SetClip(outer)
	m.drawChildren(ctx)
}

// sideColor — цвет стороны: наше и их разведены так же, как удаление и
// вставка в сравнении, база приглушена, итог — акцент темы.
func (m *MergeView) sideColor(side MergeSide) color.RGBA {
	p := &m.pal
	switch side {
	case MergeOurs:
		return p.del
	case MergeTheirs:
		return p.add
	case MergeBase:
		return p.muted
	}
	return p.accent
}

func (m *MergeView) drawHeader(ctx DrawContext, g mvGeom, side MergeSide) {
	p := &m.pal
	s := m.docs[side]
	x0, x1, _ := m.paneCodeXLocked(g, side)
	y, h := g.headerTopY, mvHeaderH
	if side == MergeResult {
		y = g.headerResY
	}
	edge := p.cardEdge
	if m.focused && m.active == side {
		edge = p.accent
	}
	dvRoundRect(ctx, p, x0, y, x1-x0, h, p.card, edge)
	fillEllipse(ctx, x0+18, y+h/2, 5, 5, m.sideColor(side))

	name := s.title
	if name == "" {
		name = Tr(mvSideKey(side))
	}
	ty := y + (h-17)/2
	tx := x0 + 32
	ctx.DrawTextFont(name, tx, ty, m.fontSize, m.boldFont, p.text)
	tx += ctx.MeasureTextFont(name, m.fontSize, m.boldFont) + 10

	right := x1 - 10
	mark := func(text string, col color.RGBA) {
		w := ctx.MeasureText(text, m.fontSize)
		right -= w
		ctx.DrawText(text, right, ty, col)
		right -= 10
	}
	if side == MergeResult {
		// У итога в шапке главное число: сколько конфликтов ещё не закрыто.
		n := 0
		for i, c := range m.chunks {
			if c.Conflict && !m.res[i].Resolved() {
				n++
			}
		}
		badge, col := Trf("merge.done", mergeConflicts(m.chunks)), p.add
		if n > 0 {
			badge, col = Trf("merge.left", n), p.dirty
		}
		bw := ctx.MeasureText(badge, m.fontSize) + 16
		bx := x1 - bw - 10
		band := p.addBand
		if n > 0 {
			band = p.delBand
		}
		ctx.FillRoundRect(bx, y+7, bw, h-14, 10, band)
		ctx.DrawText(badge, bx+8, ty, col)
		right = bx - 10
		if s.readOnly {
			mark(Tr("merge.readonly"), p.muted)
		}
	}
	if s.note != "" && tx < right {
		prev := ctx.Clip()
		ctx.SetClip(image.Rect(tx, y, right, y+h).Intersect(prev))
		ctx.DrawText(s.note, tx, ty, p.muted)
		ctx.SetClip(prev)
	}
}

func mvSideKey(side MergeSide) string {
	switch side {
	case MergeOurs:
		return "merge.side.ours"
	case MergeBase:
		return "merge.side.base"
	case MergeTheirs:
		return "merge.side.theirs"
	}
	return "merge.side.result"
}

func mergeConflicts(chunks []MergeChunk) int {
	n := 0
	for _, c := range chunks {
		if c.Conflict {
			n++
		}
	}
	return n
}

// chunkStateColor — полоса блока: нерешённый конфликт, решённый (цветом
// выбранной стороны) или правка одной стороны.
func (m *MergeView) chunkBands(ci int) (band, strong color.RGBA, ok bool) {
	p := &m.pal
	if ci < 0 || ci >= len(m.chunks) {
		return band, strong, false
	}
	c := m.chunks[ci]
	switch {
	case c.Conflict && !m.res[ci].Resolved():
		return p.delBand, p.delStrong, true
	case c.Conflict:
		return p.addBand, p.addStrong, true
	case len(c.Merged) > 0 || !mvSameLines(c.Ours, c.Base) || !mvSameLines(c.Theirs, c.Base):
		return p.addBand, p.addStrong, true
	}
	return band, strong, false
}

func mvSameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (m *MergeView) drawPane(ctx DrawContext, g mvGeom, side MergeSide, outer image.Rectangle) {
	p := &m.pal
	s := m.docs[side]
	x0, x1, codeX := m.paneCodeXLocked(g, side)
	y0, y1 := g.ty0, g.ty1
	top := m.scroll
	if side == MergeResult {
		y0, y1 = g.ry0, g.ry1
		top = m.rscroll
	}
	clip := image.Rect(x0-2, y0, x1+2, y1).Intersect(outer)
	if clip.Empty() {
		return
	}
	ctx.SetClip(clip)
	codeR := x1 - 8
	codeClip := image.Rect(codeX-2, y0, codeR, y1).Intersect(clip)
	cw := m.charW
	hs := int(m.hscroll)
	rowY := func(r int) int { return y0 + dvTopPad + r*dvLineH - int(top) }
	r0 := max(0, (int(top)-dvTopPad)/dvLineH-1)
	r1 := min(len(s.rows), r0+(y1-y0)/dvLineH+3)

	for _, c := range s.cards {
		if c[1] < r0 || c[0] > r1 {
			continue
		}
		cy0, cy1 := rowY(c[0])-dvCardExt, rowY(c[1])+dvCardExt
		dvRoundRect(ctx, p, x0, cy0, x1-x0, cy1-cy0, p.card, p.cardEdge)
	}

	selA, selB := s.sel()
	showSel := s.hasSel()
	showCaret := m.focused && m.active == side

	for r := r0; r < r1; r++ {
		row := s.rows[r]
		y := rowY(r)
		band, strong, marked := m.chunkBands(row.chunk)
		if row.kind == dvRowPad {
			// Строки на этой стороне нет: место занято чужой. Полоса блока
			// показывает, что участок к нему относится.
			if marked {
				ctx.FillRectAlpha(codeX-8, y, codeR-(codeX-8), dvLineH, premulAlpha(band, 120))
			}
			continue
		}
		if marked {
			ctx.FillRect(codeX-8, y, codeR-(codeX-8), dvLineH, band)
		}
		if row.chunk == m.current && row.chunk >= 0 {
			ctx.FillRect(x0+3, y, 3, dvLineH, p.accent)
		}
		num := strconv.Itoa(row.line + 1)
		nc := p.num
		if marked || (showCaret && row.line == s.caret.line) {
			nc = p.muted
		}
		nw := ctx.MeasureTextFont(num, m.fontSize, m.monoFont)
		ctx.DrawTextFont(num, codeX-14-nw, y+dvTextDY, m.fontSize, m.monoFont, nc)

		ctx.SetClip(codeClip)
		if showSel && row.line >= selA.line && row.line <= selB.line {
			raw := []rune(s.text.Lines[row.line])
			c0, c1 := 0, len(s.disp[row.line])+1
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
		if isMarkerLine(s.text.Lines[row.line], m.markerSize) {
			// Маркеры git в итоге — цветом конфликта и жирным: их нельзя
			// спутать со строкой кода.
			ctx.FillRect(codeX-8, y, codeR-(codeX-8), dvLineH, strong)
			ctx.DrawTextFont(s.text.Lines[row.line], codeX-hs, y+dvTextDY, m.fontSize, m.boldFont, p.dirty)
		} else {
			dvDrawCode(ctx, p, s, row.line, codeX-hs, y+dvTextDY, m.syntax, m.charW, m.fontSize, m.monoFont)
		}
		if showCaret && row.line == s.caret.line {
			cx := codeX + int(float64(diffview.DisplayCol([]rune(s.text.Lines[row.line]), s.caret.col))*cw) - hs
			ctx.FillRect(cx, y+2, 2, dvLineH-4, p.caret)
		}
		ctx.SetClip(clip)
	}
}

// isMarkerLine — строка маркера конфликта git заданной длины (0 — семь знаков).
// Сравнение точное: восемь «=» при длине семь — это код, а не разделитель, и
// git такую строку маркером тоже не считает.
func isMarkerLine(s string, size int) bool {
	if size <= 0 {
		size = 7
	}
	for _, ch := range []string{"<", "|", "=", ">"} {
		mk := strings.Repeat(ch, size)
		if s == mk || strings.HasPrefix(s, mk+" ") {
			return true
		}
	}
	return false
}

// buttonCenterLocked — центр кнопки решения: у правого края «нашей» панели и у
// левого края панели «их», на первой строке блока.
func (m *MergeView) buttonCenterLocked(g mvGeom, ci, slot int) (image.Point, bool) {
	if ci < 0 || ci >= len(m.chunks) || !m.chunks[ci].Conflict {
		return image.Point{}, false
	}
	sp := m.spans[ci]
	y := g.ty0 + dvTopPad + sp.from*dvLineH - int(m.scroll) + dvLineH/2
	if y < g.ty0+dvCircleR || y > g.ty1-dvCircleR {
		return image.Point{}, false
	}
	if slot == mvBtnOurs {
		return image.Pt(g.px[MergeOurs][1]-dvCircleR-4, y), true
	}
	return image.Pt(g.px[MergeTheirs][0]+dvCircleR+4, y), true
}

// hitButtonLocked — кнопка решения под точкой или -1.
func (m *MergeView) hitButtonLocked(x, y int) int {
	g := m.geom()
	for ci, c := range m.chunks {
		if !c.Conflict {
			continue
		}
		for slot := 0; slot < mvBtnSlots; slot++ {
			pt, ok := m.buttonCenterLocked(g, ci, slot)
			if !ok {
				continue
			}
			dx, dy := x-pt.X, y-pt.Y
			if dx*dx+dy*dy <= (dvCircleR+4)*(dvCircleR+4) {
				return ci*mvBtnSlots + slot
			}
		}
	}
	return -1
}

// buttonActionLocked — что делает кнопка: блок и решение.
func (m *MergeView) buttonActionLocked(btn int) (ci int, how MergeResolution) {
	ci = btn / mvBtnSlots
	if btn%mvBtnSlots == mvBtnOurs {
		return ci, MergeTakeOurs
	}
	return ci, MergeTakeTheirs
}

func (m *MergeView) drawButtons(ctx DrawContext, g mvGeom, outer image.Rectangle) {
	p := &m.pal
	ctx.SetClip(image.Rect(g.b.Min.X, g.ty0, g.b.Max.X, g.ty1).Intersect(outer))
	for ci, c := range m.chunks {
		if !c.Conflict {
			continue
		}
		for slot := 0; slot < mvBtnSlots; slot++ {
			pt, ok := m.buttonCenterLocked(g, ci, slot)
			if !ok {
				continue
			}
			col := p.del
			if slot == mvBtnTheirs {
				col = p.add
			}
			if m.res[ci].Resolved() {
				// Решённый блок: кнопка выбранной стороны подсвечена, вторая
				// приглушена — видно, чем закрыт конфликт.
				chosen := (slot == mvBtnOurs && m.res[ci] == MergeTakeOurs) ||
					(slot == mvBtnTheirs && m.res[ci] == MergeTakeTheirs)
				if !chosen {
					col = premulAlpha(col, 90)
				}
			}
			if m.hoverBtn == ci*mvBtnSlots+slot {
				if slot == mvBtnOurs {
					col = p.delHover
				} else {
					col = p.addHover
				}
			}
			// Стрелка смотрит в сторону итога: от «нашего» — вправо, от «их» —
			// влево, как перенос блока в сравнении.
			dvArrowButton(ctx, pt.X, pt.Y, dvCircleR, col, p.card, slot == mvBtnOurs)
		}
	}
}

func (m *MergeView) drawSplitter(ctx DrawContext, g mvGeom) {
	p := &m.pal
	x0 := g.px[0][0]
	x1 := g.b.Max.X - dvOuterPad - dvRulerW - 6
	y := g.splitY
	ctx.FillRect(x0, y-1, x1-x0, 2, p.cardEdge)
	// Ручка: три точки посередине — за неё двигают границу верха и итога.
	cx := (x0 + x1) / 2
	for i := -1; i <= 1; i++ {
		fillEllipse(ctx, cx+i*8, y, 2, 2, p.muted)
	}
}

// drawRuler — полоса-обзор: где по файлу стоят конфликты и что уже решено, и
// ползунок видимой области верхних панелей. Геометрия — общая с разбором мыши
// (rulerTrackLocked, rulerMarkLocked, rulerThumbLocked): что нарисовано, в то
// и попадает щелчок.
func (m *MergeView) drawRuler(ctx DrawContext, g mvGeom) {
	p := &m.pal
	tr := m.rulerTrackLocked(g)
	if tr.Dy() <= 0 || m.rows == 0 {
		return
	}
	ctx.SetClip(tr.Inset(-2).Intersect(g.b))
	ctx.FillRoundRect(tr.Min.X, tr.Min.Y, tr.Dx(), tr.Dy(), dvRulerW/2, p.track)
	for ci, c := range m.chunks {
		band, _, marked := m.chunkBands(ci)
		if !marked {
			continue
		}
		col := band
		if c.Conflict {
			col = p.dirty
			if m.res[ci].Resolved() {
				col = p.add
			}
		}
		r := m.rulerMarkLocked(g, ci)
		ctx.FillRect(r.Min.X+2, r.Min.Y, r.Dx()-4, r.Dy(), col)
		if ci == m.current {
			ctx.DrawBorder(r.Min.X, r.Min.Y-1, r.Dx(), r.Dy()+2, p.accent)
		}
	}
	if th := m.rulerThumbLocked(g); !th.Empty() {
		alpha := uint8(110)
		if m.rulerDrag {
			alpha = 170 // схваченный ползунок плотнее — видно, что он в руке
		}
		ctx.FillRectAlpha(th.Min.X, th.Min.Y, th.Dx(), th.Dy(), premulAlpha(p.thumb, alpha))
	}
}

// String — для отладочных сообщений теста.
func (s MergeSide) String() string {
	switch s {
	case MergeOurs:
		return "ours"
	case MergeBase:
		return "base"
	case MergeTheirs:
		return "theirs"
	case MergeResult:
		return "result"
	}
	return fmt.Sprintf("MergeSide(%d)", int(s))
}
