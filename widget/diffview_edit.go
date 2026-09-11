package widget

import (
	"image"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/widget/diffview"
)

// diffview_edit.go — правка, история, буфер обмена, клавиатура и мышь.

// ─── История ────────────────────────────────────────────────────────────────

func (d *DiffView) snapshotLocked() dvHist {
	var e dvHist
	for i, s := range d.docs {
		e.lines[i] = slices.Clone(s.text.Lines)
		e.caret[i], e.anchor[i], e.rev[i] = s.caret, s.anchor, s.rev
	}
	e.active = d.active
	return e
}

func (d *DiffView) restoreLocked(e dvHist) {
	for i, s := range d.docs {
		s.text.Lines, s.caret, s.anchor, s.rev = e.lines[i], e.caret[i], e.anchor[i], e.rev[i]
		s.wantCol = -1
	}
	d.active = e.active
}

// pushUndoLocked сохраняет состояние до правки. Подряд идущий набор одной
// стороны сливается в один шаг: отмена слова по букве была бы пыткой.
func (d *DiffView) pushUndoLocked(kind dvEditKind, side DiffSide) {
	now := time.Now()
	if (kind == dvEditType || kind == dvEditDelete) && kind == d.lastEdit &&
		side == d.lastEditSide && now.Sub(d.lastEditAt) < 1500*time.Millisecond {
		d.lastEditAt = now
		return
	}
	d.undo = append(d.undo, d.snapshotLocked())
	if len(d.undo) > dvUndoLimit {
		d.undo = d.undo[1:]
	}
	d.redo = nil
	d.lastEdit, d.lastEditSide, d.lastEditAt = kind, side, now
}

func (d *DiffView) resetHistoryLocked() {
	d.undo, d.redo = nil, nil
	d.lastEdit = dvEditNone
	d.current = -1
	d.expanded = map[[2]int]bool{}
}

func (d *DiffView) undoLocked() {
	if len(d.undo) == 0 {
		return
	}
	d.redo = append(d.redo, d.snapshotLocked())
	d.restoreLocked(d.undo[len(d.undo)-1])
	d.undo = d.undo[:len(d.undo)-1]
	d.afterEditLocked()
}

func (d *DiffView) redoLocked() {
	if len(d.redo) == 0 {
		return
	}
	d.undo = append(d.undo, d.snapshotLocked())
	d.restoreLocked(d.redo[len(d.redo)-1])
	d.redo = d.redo[:len(d.redo)-1]
	d.afterEditLocked()
}

func (d *DiffView) Undo() { d.do(d.undoLocked) }
func (d *DiffView) Redo() { d.do(d.redoLocked) }

func (d *DiffView) CanUndo() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.undo) > 0
}

func (d *DiffView) CanRedo() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.redo) > 0
}

// ─── Правка ─────────────────────────────────────────────────────────────────

func (d *DiffView) afterEditLocked() {
	d.lastEditAt = time.Now()
	d.rebuildLocked()
	d.current = d.chunkAtLocked(d.active, d.docs[d.active].caret.line)
	d.ensureCaretVisibleLocked()
}

// editLocked — общий вход правки активной стороны.
func (d *DiffView) editLocked(kind dvEditKind, fn func(s *dvDoc)) bool {
	s := d.docs[d.active]
	if s.readOnly {
		return false
	}
	d.pushUndoLocked(kind, d.active)
	fn(s)
	if len(s.text.Lines) == 0 {
		s.text.Lines = []string{""}
	}
	s.caret = s.clamp(s.caret)
	s.anchor = s.caret
	s.wantCol = -1
	s.rev = d.nextRev()
	d.afterEditLocked()
	return true
}

func (d *DiffView) deleteSelLocked(s *dvDoc) {
	if s.hasSel() {
		a, b := s.sel()
		s.remove(a, b)
		s.caret = a
	}
}

func (d *DiffView) insertTextLocked(text string, kind dvEditKind) {
	d.editLocked(kind, func(s *dvDoc) {
		d.deleteSelLocked(s)
		s.caret = s.insert(s.caret, text)
	})
}

// newlineLocked — перевод строки с отступом текущей строки.
func (d *DiffView) newlineLocked() {
	s := d.docs[d.active]
	ind := diffview.IndentOf(s.text.Lines[s.caret.line])
	if a, _ := s.sel(); s.hasSel() {
		ind = diffview.IndentOf(s.text.Lines[a.line])
	}
	d.insertTextLocked("\n"+ind, dvEditOther)
}

func (d *DiffView) backspaceLocked(word bool) {
	s := d.docs[d.active]
	if !s.hasSel() && s.caret == (dvPos{}) {
		return
	}
	d.editLocked(dvEditDelete, func(s *dvDoc) {
		if s.hasSel() {
			d.deleteSelLocked(s)
			return
		}
		from := dvPos{s.caret.line, s.caret.col - 1}
		switch {
		case word:
			from = s.wordLeft(s.caret)
		case s.caret.col == 0:
			from = dvPos{s.caret.line - 1, s.lineLen(s.caret.line - 1)}
		}
		s.remove(from, s.caret)
		s.caret = from
	})
}

func (d *DiffView) deleteLocked(word bool) {
	s := d.docs[d.active]
	if !s.hasSel() && s.caret == s.end() {
		return
	}
	d.editLocked(dvEditDelete, func(s *dvDoc) {
		if s.hasSel() {
			d.deleteSelLocked(s)
			return
		}
		to := dvPos{s.caret.line, s.caret.col + 1}
		switch {
		case word:
			to = s.wordRight(s.caret)
		case s.caret.col >= s.lineLen(s.caret.line):
			to = dvPos{s.caret.line + 1, 0}
		}
		s.remove(s.caret, to)
	})
}

// ─── Буфер обмена ───────────────────────────────────────────────────────────

// selOrLineLocked — выделение или (без него) вся строка с переводом: так
// копируют строку в редакторах кода.
func (d *DiffView) selOrLineLocked(s *dvDoc) (a, b dvPos) {
	if s.hasSel() {
		return s.sel()
	}
	if s.caret.line+1 < len(s.text.Lines) {
		return dvPos{s.caret.line, 0}, dvPos{s.caret.line + 1, 0}
	}
	return dvPos{s.caret.line, 0}, dvPos{s.caret.line, s.lineLen(s.caret.line)}
}

func (d *DiffView) copyLocked() {
	s := d.docs[d.active]
	a, b := d.selOrLineLocked(s)
	if txt := s.textRange(a, b); txt != "" {
		eol := s.text.EOL
		if eol == "" {
			eol = "\n"
		}
		ClipboardSetText(strings.ReplaceAll(txt, "\n", eol))
	}
}

func (d *DiffView) cutLocked() {
	s := d.docs[d.active]
	if s.readOnly {
		d.copyLocked()
		return
	}
	a, b := d.selOrLineLocked(s)
	d.copyLocked()
	s.anchor, s.caret = a, b
	d.editLocked(dvEditOther, d.deleteSelLocked)
}

func (d *DiffView) pasteLocked() {
	if txt := ClipboardGetText(); txt != "" {
		d.insertTextLocked(txt, dvEditOther)
	}
}

func (d *DiffView) selectAllLocked() {
	s := d.docs[d.active]
	s.anchor, s.caret = dvPos{}, s.end()
}

func (d *DiffView) Cut()       { d.do(d.cutLocked) }
func (d *DiffView) Copy()      { d.do(d.copyLocked) }
func (d *DiffView) Paste()     { d.do(d.pasteLocked) }
func (d *DiffView) SelectAll() { d.do(d.selectAllLocked) }

func (d *DiffView) DeleteSelection() {
	d.do(func() {
		if d.docs[d.active].hasSel() {
			d.editLocked(dvEditOther, d.deleteSelLocked)
		}
	})
}

// InsertText вставляет текст в каретку активной стороны.
func (d *DiffView) InsertText(text string) {
	d.do(func() { d.insertTextLocked(text, dvEditOther) })
}

// SelectedText — выделенный текст активной стороны.
func (d *DiffView) SelectedText() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.docs[d.active]
	a, b := s.sel()
	return s.textRange(a, b)
}

// ─── Перенос блоков ─────────────────────────────────────────────────────────

func dvSplice(dst []string, from, to int, src []string) []string {
	out := make([]string, 0, len(dst)-(to-from)+len(src))
	out = append(out, dst[:from]...)
	out = append(out, src...)
	return append(out, dst[to:]...)
}

func (d *DiffView) applyLocked(ci int, toRight bool) {
	if ci < 0 || ci >= len(d.chunks) || d.chunks[ci].Kind == DiffEqual {
		return
	}
	dstSide := DiffLeft
	if toRight {
		dstSide = DiffRight
	}
	dst, src := d.docs[dstSide], d.docs[dstSide.Other()]
	if dst.readOnly {
		return
	}
	c := d.chunks[ci]
	ord := d.ordinalLocked(ci)
	d.pushUndoLocked(dvEditOther, dstSide)
	if toRight {
		dst.text.Lines = dvSplice(dst.text.Lines, c.RightFrom, c.RightTo, src.text.Lines[c.LeftFrom:c.LeftTo])
	} else {
		dst.text.Lines = dvSplice(dst.text.Lines, c.LeftFrom, c.LeftTo, src.text.Lines[c.RightFrom:c.RightTo])
	}
	if len(dst.text.Lines) == 0 {
		dst.text.Lines = []string{""}
	}
	dst.caret, dst.anchor = dst.clamp(dst.caret), dst.clamp(dst.anchor)
	dst.rev = d.nextRev()
	d.lastEdit = dvEditNone
	d.rebuildLocked()
	// Текущим становится следующее отличие: переносят обычно подряд.
	d.current = -1
	for i, nc := range d.chunks {
		if nc.Kind != DiffEqual && nc.LeftFrom >= c.LeftFrom {
			d.current = i
			break
		}
	}
	if f := d.OnBlockCopied; f != nil {
		d.emit(func() { f(ord, toRight) })
	}
}

func (d *DiffView) copyAllLocked(toRight bool) {
	dstSide := DiffLeft
	if toRight {
		dstSide = DiffRight
	}
	dst, src := d.docs[dstSide], d.docs[dstSide.Other()]
	if dst.readOnly || slices.Equal(dst.text.Lines, src.text.Lines) {
		return
	}
	d.pushUndoLocked(dvEditOther, dstSide)
	dst.text.Lines = slices.Clone(src.text.Lines)
	dst.caret, dst.anchor = dst.clamp(dst.caret), dst.clamp(dst.anchor)
	dst.rev = d.nextRev()
	d.lastEdit = dvEditNone
	d.rebuildLocked()
	d.current = -1
}

// ─── Каретка ────────────────────────────────────────────────────────────────

func (d *DiffView) caretMovedLocked() {
	d.lastEdit = dvEditNone
	s := d.docs[d.active]
	if s.lineRow[s.caret.line] < 0 {
		s.caret.line = d.visibleLineLocked(s, s.caret.line, +1)
		s.caret = s.clamp(s.caret)
	}
	d.current = d.chunkAtLocked(d.active, s.caret.line)
	d.ensureCaretVisibleLocked()
}

func (d *DiffView) moveLocked(p dvPos, extend bool) {
	s := d.docs[d.active]
	s.caret = s.clamp(p)
	if !extend {
		s.anchor = s.caret
	}
	d.caretMovedLocked()
}

// moveRowsLocked двигает каретку на delta экранных строк, минуя свёртки и
// сохраняя экранную колонку.
func (d *DiffView) moveRowsLocked(delta int, extend bool) {
	s := d.docs[d.active]
	raw := []rune(s.text.Lines[s.caret.line])
	if s.wantCol < 0 {
		s.wantCol = diffview.DisplayCol(raw, s.caret.col)
	}
	want := s.wantCol
	r := s.lineRow[s.caret.line]
	t := max(0, min(r+delta, len(s.rows)-1))
	step := 1
	if delta < 0 {
		step = -1
	}
	for t >= 0 && t < len(s.rows) && s.rows[t].kind == dvRowFold {
		t += step
	}
	if t < 0 || t >= len(s.rows) {
		t = r
	}
	line := s.rows[t].line
	d.moveLocked(dvPos{line, diffview.RawCol([]rune(s.text.Lines[line]), want)}, extend)
	s.wantCol = want
}

// homeLocked — «умный» Home: к началу текста строки, повторно — к нулевой
// колонке.
func (d *DiffView) homeLocked(extend bool) {
	s := d.docs[d.active]
	ind := len([]rune(diffview.IndentOf(s.text.Lines[s.caret.line])))
	col := ind
	if s.caret.col == ind {
		col = 0
	}
	d.moveLocked(dvPos{s.caret.line, col}, extend)
}

// ensureCaretVisibleLocked прокручивает так, чтобы каретка была видна.
func (d *DiffView) ensureCaretVisibleLocked() {
	s := d.docs[d.active]
	r := s.lineRow[s.caret.line]
	if r < 0 {
		return
	}
	d.stopAnim()
	vh := d.viewH()
	rowTop := float64(dvTopPad + r*dvLineH)
	const m = dvLineH
	in := func() (bool, float64) {
		y := rowTop - d.sideTop(d.active)
		switch {
		case y < m:
			return false, y - m
		case y+dvLineH > vh-m:
			return false, y + dvLineH - (vh - m)
		}
		return true, 0
	}
	for i := 0; i < 3; i++ {
		ok, delta := in()
		if ok {
			break
		}
		if !d.setScrollLocked(d.scroll + delta) {
			break
		}
	}
	if ok, _ := in(); !ok {
		d.setScrollLocked(d.scrollForV(d.vForSide(d.active, rowTop)))
	}

	g := d.geom()
	_, _, codeX, codeR := d.paneGeom(g, d.active)
	w := float64(codeR - codeX)
	x := float64(diffview.DisplayCol([]rune(s.text.Lines[s.caret.line]), s.caret.col)) * d.charW
	switch {
	case x-d.hscroll < 0:
		d.hscroll = math.Max(0, x-4*d.charW)
	case x-d.hscroll > w-2*d.charW:
		d.hscroll = x - w + 4*d.charW
	}
}

// ─── Клавиатура ─────────────────────────────────────────────────────────────

// AcceptsTab — контракт TabAcceptor: Tab вставляет табуляцию, а не уводит
// фокус. У стороны только для чтения Tab снова уходит обходу фокуса — вставить
// его всё равно некуда. Ctrl+Tab остаётся навигацией всегда.
func (d *DiffView) AcceptsTab() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.docs[d.active].readOnly
}

func (d *DiffView) OnKeyEvent(e KeyEvent) {
	if !e.Pressed {
		return
	}
	ctrl := e.Mod&ModCtrl != 0
	shift := e.Mod&ModShift != 0
	alt := e.Mod&ModAlt != 0
	if ctrl && !alt && e.Code == KeyS {
		side := d.ActiveSide()
		if f := d.OnSaveRequest; f != nil {
			f(side)
		}
		d.mu.Lock()
		cmd := d.commands["SaveCommand"]
		d.mu.Unlock()
		if cmd != nil && cmd.CanExecute(side) {
			cmd.Execute(side)
		}
		return
	}
	if ctrl && e.Code == KeyUp && !alt {
		d.scrollBy(-dvLineH)
		return
	}
	if ctrl && e.Code == KeyDown && !alt {
		d.scrollBy(dvLineH)
		return
	}
	d.do(func() { d.keyLocked(e, ctrl, shift, alt) })
}

func (d *DiffView) keyLocked(e KeyEvent, ctrl, shift, alt bool) {
	s := d.docs[d.active]
	switch {
	case e.Code == KeyF7 && shift, alt && e.Code == KeyUp:
		d.navigateLocked(-1)
	case e.Code == KeyF7, alt && e.Code == KeyDown:
		d.navigateLocked(+1)
	case alt && !ctrl && e.Code == KeyRight:
		d.applyLocked(d.current, true)
	case alt && !ctrl && e.Code == KeyLeft:
		d.applyLocked(d.current, false)

	case ctrl && e.Code == KeyZ && shift, ctrl && e.Code == KeyY:
		d.redoLocked()
	case ctrl && e.Code == KeyZ:
		d.undoLocked()
	case ctrl && e.Code == KeyA:
		d.selectAllLocked()
	case ctrl && e.Code == KeyC, ctrl && e.Code == KeyInsert:
		d.copyLocked()
	case ctrl && e.Code == KeyX, shift && e.Code == KeyDelete:
		d.cutLocked()
	case ctrl && e.Code == KeyV, shift && e.Code == KeyInsert:
		d.pasteLocked()

	case e.Code == KeyLeft:
		p := dvPos{s.caret.line, s.caret.col - 1}
		switch {
		case ctrl:
			p = s.wordLeft(s.caret)
		case !shift && s.hasSel():
			p, _ = s.sel()
		case s.caret.col == 0 && s.caret.line > 0:
			p = dvPos{s.caret.line - 1, s.lineLen(s.caret.line - 1)}
		}
		d.moveLocked(p, shift)
	case e.Code == KeyRight:
		p := dvPos{s.caret.line, s.caret.col + 1}
		switch {
		case ctrl:
			p = s.wordRight(s.caret)
		case !shift && s.hasSel():
			_, p = s.sel()
		case s.caret.col >= s.lineLen(s.caret.line) && s.caret.line+1 < len(s.text.Lines):
			p = dvPos{s.caret.line + 1, 0}
		}
		d.moveLocked(p, shift)
	case e.Code == KeyUp:
		d.moveRowsLocked(-1, shift)
	case e.Code == KeyDown:
		d.moveRowsLocked(+1, shift)
	case e.Code == KeyPageUp:
		d.moveRowsLocked(-int(d.viewH()/dvLineH)+1, shift)
	case e.Code == KeyPageDown:
		d.moveRowsLocked(int(d.viewH()/dvLineH)-1, shift)
	case e.Code == KeyHome && ctrl:
		d.moveLocked(dvPos{}, shift)
	case e.Code == KeyEnd && ctrl:
		d.moveLocked(s.end(), shift)
	case e.Code == KeyHome:
		d.homeLocked(shift)
	case e.Code == KeyEnd:
		d.moveLocked(dvPos{s.caret.line, s.lineLen(s.caret.line)}, shift)
	case e.Code == KeyEscape:
		s.anchor = s.caret

	case e.Code == KeyBackspace:
		d.backspaceLocked(ctrl)
	case e.Code == KeyDelete:
		d.deleteLocked(ctrl)
	case e.Code == KeyEnter:
		d.newlineLocked()
	case e.Code == KeyTab && !shift && !ctrl:
		d.insertTextLocked("\t", dvEditType)
	case e.Rune >= 32 && (!ctrl || alt):
		d.insertTextLocked(string(e.Rune), dvEditType)
	}
}

// ─── Мышь ───────────────────────────────────────────────────────────────────

func (d *DiffView) inTextArea(x, y int) (DiffSide, bool) {
	side, ok := d.sideAt(x, y)
	if !ok {
		return 0, false
	}
	x0, _, _, codeR := d.paneGeom(d.geom(), side)
	return side, x >= x0+6 && x < codeR+6
}

func (d *DiffView) OnMouseButton(e MouseEvent) bool {
	switch e.Button {
	case MouseLeft:
	case MouseRight:
		if e.Pressed {
			d.do(func() { d.rightPressLocked(e.X, e.Y) })
		}
		return false
	default:
		return false
	}
	if !e.Pressed {
		// Захват снимет движок: он гарантирует это при отпускании ЛКМ.
		d.mu.Lock()
		was := d.rulerDrag || d.dragSel
		d.rulerDrag, d.dragSel = false, false
		d.mu.Unlock()
		return was
	}
	d.mu.Lock()
	g := d.geom()
	if image.Pt(e.X, e.Y).In(d.rulerTrack(g).Inset(-4)) {
		d.rulerDrag = true
		d.scrollToRulerLocked(e.Y)
		d.mu.Unlock()
		d.Invalidate()
		return true
	}
	btn := d.hitButtonLocked(e.X, e.Y)
	d.mu.Unlock()
	if btn >= 0 {
		d.do(func() { d.applyLocked(btn/2, btn%2 == 0) })
		return true
	}
	d.do(func() { d.leftPressLocked(e) })
	return true
}

func (d *DiffView) leftPressLocked(e MouseEvent) {
	side, ok := d.inTextArea(e.X, e.Y)
	if !ok {
		return
	}
	s := d.docs[side]
	r := d.rowAtLocked(side, e.Y)
	if r >= 0 && r < len(s.rows) && s.rows[r].kind == dvRowFold {
		c := d.chunks[s.rows[r].chunk]
		d.expanded[[2]int{c.LeftFrom, c.RightFrom}] = true
		d.rebuildLocked()
		return
	}
	now := time.Now()
	near := math.Abs(float64(e.X-d.lastClickPt.X)) <= 4 && math.Abs(float64(e.Y-d.lastClickPt.Y)) <= 4
	if near && now.Sub(d.lastClickAt) < 400*time.Millisecond {
		d.clicks++
	} else {
		d.clicks = 1
	}
	d.lastClickAt, d.lastClickPt = now, image.Pt(e.X, e.Y)

	d.active = side
	p := d.hitPosLocked(side, e.X, e.Y)
	_, _, codeX, _ := d.paneGeom(d.geom(), side)
	switch {
	case e.X < codeX-6 || d.clicks >= 3: // номер строки или тройной клик — строка целиком
		s.anchor = dvPos{p.line, 0}
		s.caret = s.clamp(dvPos{p.line + 1, 0})
		if p.line+1 >= len(s.text.Lines) {
			s.caret = dvPos{p.line, s.lineLen(p.line)}
		}
	case d.clicks == 2:
		s.anchor, s.caret = s.wordAt(p)
	case e.Mod&ModShift != 0:
		s.caret = p
	default:
		s.caret, s.anchor = p, p
	}
	s.wantCol = -1
	d.dragSel, d.dragSide = true, side
	d.lastEdit = dvEditNone
	d.current = d.chunkAtLocked(side, s.caret.line)
}

func (d *DiffView) rightPressLocked(x, y int) {
	side, ok := d.inTextArea(x, y)
	if !ok {
		return
	}
	d.active = side
	s := d.docs[side]
	p := d.hitPosLocked(side, x, y)
	// Правый клик внутри выделения его не сбрасывает — меню работает с ним.
	if a, b := s.sel(); s.hasSel() && !p.before(a) && p.before(b) {
		return
	}
	s.caret, s.anchor = p, p
	d.current = d.chunkAtLocked(side, p.line)
}

func (d *DiffView) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if image.Pt(e.X, e.Y).In(d.rulerTrack(d.geom()).Inset(-4)) {
		return true
	}
	_, ok := d.inTextArea(e.X, e.Y)
	return ok && d.hitButtonLocked(e.X, e.Y) < 0
}

func (d *DiffView) OnMouseMove(x, y int) {
	d.mu.Lock()
	if d.rulerDrag {
		d.scrollToRulerLocked(y)
		d.mu.Unlock()
		d.Invalidate()
		return
	}
	if d.dragSel {
		d.mu.Unlock()
		d.do(func() {
			g := d.geom()
			switch {
			case y < g.cy0:
				d.setScrollLocked(d.scroll - float64(g.cy0-y))
			case y > g.cy1:
				d.setScrollLocked(d.scroll + float64(y-g.cy1))
			}
			s := d.docs[d.dragSide]
			if d.clicks < 2 {
				s.caret = d.hitPosLocked(d.dragSide, x, y)
			}
		})
		return
	}
	btn, ch := -1, -1
	if !CursorIsNowhere(x, y) {
		btn = d.hitButtonLocked(x, y)
		if btn >= 0 {
			ch = btn / 2
		} else {
			ch = d.hitChunkLocked(x, y)
		}
	}
	changed := btn != d.hoverBtn || ch != d.hoverChunk
	d.hoverBtn, d.hoverChunk = btn, ch
	d.mu.Unlock()
	if changed {
		d.Invalidate()
	}
}

func (d *DiffView) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	if dx != 0 {
		d.hscrollBy(dx)
	}
	if dy != 0 {
		d.scrollBy(dy)
	}
	return true
}

func (d *DiffView) Cursor(x, y int) Cursor {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hitButtonLocked(x, y) >= 0 || image.Pt(x, y).In(d.rulerTrack(d.geom()).Inset(-4)) {
		return CursorHand
	}
	if side, ok := d.inTextArea(x, y); ok {
		s := d.docs[side]
		if r := d.rowAtLocked(side, y); r >= 0 && r < len(s.rows) && s.rows[r].kind == dvRowFold {
			return CursorHand
		}
		return CursorIBeam
	}
	return CursorArrow
}

func (d *DiffView) SetFocused(v bool) {
	d.mu.Lock()
	d.focused = v
	d.mu.Unlock()
	d.Invalidate()
}

func (d *DiffView) IsFocused() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.focused
}

// ─── Контекстное меню ───────────────────────────────────────────────────────

// ContextMenuAt — контракт ContextMenuProvider: меню под точкой.
func (d *DiffView) ContextMenuAt(x, y int) *PopupMenu {
	d.mu.Lock()
	side, ok := d.inTextArea(x, y)
	if !ok {
		d.mu.Unlock()
		return nil
	}
	s := d.docs[side]
	ro := s.readOnly
	sel := s.hasSel()
	canUndo, canRedo := len(d.undo) > 0, len(d.redo) > 0
	ci := d.chunkAtLocked(side, s.caret.line)
	roL, roR := d.docs[0].readOnly, d.docs[1].readOnly
	d.mu.Unlock()

	items := []MenuItem{
		{Text: Tr("diff.ctx.undo"), Disabled: !canUndo, OnClick: d.Undo},
		{Text: Tr("diff.ctx.redo"), Disabled: !canRedo, OnClick: d.Redo},
		{Separator: true},
		{Text: Tr("diff.ctx.cut"), Disabled: ro, OnClick: d.Cut},
		{Text: Tr("diff.ctx.copy"), OnClick: d.Copy},
		{Text: Tr("diff.ctx.paste"), Disabled: ro, OnClick: d.Paste},
		{Text: Tr("diff.ctx.delete"), Disabled: ro || !sel, OnClick: d.DeleteSelection},
		{Separator: true},
		{Text: Tr("diff.ctx.selall"), OnClick: d.SelectAll},
		{Separator: true},
		{Text: Tr("diff.ctx.toRight"), Disabled: ci < 0 || roR, OnClick: func() { d.do(func() { d.applyLocked(ci, true) }) }},
		{Text: Tr("diff.ctx.toLeft"), Disabled: ci < 0 || roL, OnClick: func() { d.do(func() { d.applyLocked(ci, false) }) }},
		{Text: Tr("diff.ctx.next"), OnClick: d.NextChange},
		{Text: Tr("diff.ctx.prev"), OnClick: d.PrevChange},
		{Separator: true},
		{Text: Tr("diff.ctx.allRight"), Disabled: roR, OnClick: func() { d.CopyAll(true) }},
		{Text: Tr("diff.ctx.allLeft"), Disabled: roL, OnClick: func() { d.CopyAll(false) }},
	}
	d.menu.SetItems(items)
	return d.menu
}
