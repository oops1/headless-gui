package widget

import (
	"image"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/widget/diffview"
)

// mergeview_edit.go — история, правка итога, буфер обмена, клавиатура и мышь.

// ─── История ────────────────────────────────────────────────────────────────

func (m *MergeView) snapshotLocked() mvHist {
	r := m.docs[MergeResult]
	return mvHist{
		lines:  slices.Clone(r.text.Lines),
		res:    slices.Clone(m.res),
		spans:  slices.Clone(m.rspan),
		caret:  r.caret,
		anchor: r.anchor,
	}
}

func (m *MergeView) restoreLocked(e mvHist) {
	r := m.docs[MergeResult]
	r.text.Lines = slices.Clone(e.lines)
	r.caret, r.anchor, r.wantCol = e.caret, e.anchor, -1
	m.res = slices.Clone(e.res)
	m.rspan = slices.Clone(e.spans)
	r.rev = m.nextRev()
}

// pushUndoLocked сохраняет состояние до правки. Подряд идущий набор сливается
// в один шаг: отмена слова по букве была бы пыткой.
func (m *MergeView) pushUndoLocked(kind dvEditKind) {
	now := time.Now()
	if (kind == dvEditType || kind == dvEditDelete) && kind == m.lastEdit &&
		now.Sub(m.lastEditAt) < 1500*time.Millisecond {
		m.lastEditAt = now
		return
	}
	m.undo = append(m.undo, m.snapshotLocked())
	if len(m.undo) > dvUndoLimit {
		m.undo = m.undo[1:]
	}
	m.redo = nil
	m.lastEdit, m.lastEditAt = kind, now
}

func (m *MergeView) resetHistoryLocked() {
	m.undo, m.redo = nil, nil
	m.lastEdit = dvEditNone
	m.current = -1
}

// Undo отменяет последнюю правку итога или решение по конфликту.
func (m *MergeView) Undo() {
	m.do(func() {
		if len(m.undo) == 0 {
			return
		}
		m.redo = append(m.redo, m.snapshotLocked())
		m.restoreLocked(m.undo[len(m.undo)-1])
		m.undo = m.undo[:len(m.undo)-1]
		m.afterEditLocked()
		m.resolvedEvent = true
	})
}

// Redo повторяет отменённое.
func (m *MergeView) Redo() {
	m.do(func() {
		if len(m.redo) == 0 {
			return
		}
		m.undo = append(m.undo, m.snapshotLocked())
		m.restoreLocked(m.redo[len(m.redo)-1])
		m.redo = m.redo[:len(m.redo)-1]
		m.afterEditLocked()
		m.resolvedEvent = true
	})
}

// CanUndo сообщает, есть ли что отменять.
func (m *MergeView) CanUndo() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.undo) > 0
}

// CanRedo сообщает, есть ли что повторять.
func (m *MergeView) CanRedo() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.redo) > 0
}

// ─── Правка итога ───────────────────────────────────────────────────────────

func (m *MergeView) afterEditLocked() {
	m.lastEditAt = time.Now()
	m.rebuildLocked()
	m.ensureResultVisibleLocked()
}

// editLocked — общий вход правки: правится только итог, и строки, добавленные
// или убранные руками, достаются блоку, внутри которого стояла каретка.
func (m *MergeView) editLocked(kind dvEditKind, fn func(s *dvDoc)) bool {
	r := m.docs[MergeResult]
	if r.readOnly {
		return false
	}
	m.pushUndoLocked(kind)
	// Правят итог — он и ведёт прокрутку: подгонять его под верх значило бы
	// сдвигать итог на строку при каждом Enter прямо под пишущим.
	m.resultDrives = true
	at, before := r.caret.line, len(r.text.Lines)
	fn(r)
	if len(r.text.Lines) == 0 {
		r.text.Lines = []string{""}
	}
	m.adjustSpansLocked(at, len(r.text.Lines)-before)
	r.caret = r.clamp(r.caret)
	r.anchor = r.caret
	r.wantCol = -1
	r.rev = m.nextRev()
	m.editsEvent = true
	m.afterEditLocked()
	return true
}

func (m *MergeView) deleteSelLocked(s *dvDoc) {
	if s.hasSel() {
		a, b := s.sel()
		s.remove(a, b)
		s.caret = a
	}
}

func (m *MergeView) insertTextLocked(text string, kind dvEditKind) {
	m.editLocked(kind, func(s *dvDoc) {
		m.deleteSelLocked(s)
		s.caret = s.insert(s.caret, text)
	})
}

// InsertText вставляет текст в итог на место каретки.
func (m *MergeView) InsertText(text string) { m.do(func() { m.insertTextLocked(text, dvEditOther) }) }

func (m *MergeView) newlineLocked() {
	s := m.docs[MergeResult]
	ind := diffview.IndentOf(s.text.Lines[s.caret.line])
	if a, _ := s.sel(); s.hasSel() {
		ind = diffview.IndentOf(s.text.Lines[a.line])
	}
	m.insertTextLocked("\n"+ind, dvEditOther)
}

func (m *MergeView) backspaceLocked(word bool) {
	s := m.docs[MergeResult]
	if !s.hasSel() && s.caret == (dvPos{}) {
		return
	}
	m.editLocked(dvEditDelete, func(s *dvDoc) {
		if s.hasSel() {
			m.deleteSelLocked(s)
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

func (m *MergeView) deleteLocked(word bool) {
	s := m.docs[MergeResult]
	if !s.hasSel() && s.caret == s.end() {
		return
	}
	m.editLocked(dvEditDelete, func(s *dvDoc) {
		if s.hasSel() {
			m.deleteSelLocked(s)
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

// DeleteSelection удаляет выделенное в итоге.
func (m *MergeView) DeleteSelection() {
	m.do(func() {
		if m.docs[MergeResult].hasSel() {
			m.editLocked(dvEditDelete, m.deleteSelLocked)
		}
	})
}

// ─── Буфер обмена ───────────────────────────────────────────────────────────

// selOrLineLocked — выделение или (без него) вся строка с переводом: так
// копируют строку в редакторах кода.
func (m *MergeView) selOrLineLocked(s *dvDoc) (a, b dvPos) {
	if s.hasSel() {
		return s.sel()
	}
	if s.caret.line+1 < len(s.text.Lines) {
		return dvPos{s.caret.line, 0}, dvPos{s.caret.line + 1, 0}
	}
	return dvPos{s.caret.line, 0}, dvPos{s.caret.line, s.lineLen(s.caret.line)}
}

func (m *MergeView) copyLocked() {
	s := m.docs[m.active]
	a, b := m.selOrLineLocked(s)
	text := s.textRange(a, b)
	// Переводы строк — как в файле итога: вставка в другой редактор не должна
	// приносить чужой EOL.
	if eol := m.docs[MergeResult].text.EOL; eol != "" && eol != "\n" {
		text = strings.ReplaceAll(text, "\n", eol)
	}
	ClipboardSetText(text)
}

// Copy копирует выделенное (или строку) из активной панели.
func (m *MergeView) Copy() { m.do(m.copyLocked) }

// Cut вырезает выделенное из итога.
func (m *MergeView) Cut() {
	m.do(func() {
		if m.docs[MergeResult].readOnly || m.active != MergeResult {
			m.copyLocked()
			return
		}
		m.copyLocked()
		s := m.docs[MergeResult]
		a, b := m.selOrLineLocked(s)
		m.editLocked(dvEditOther, func(s *dvDoc) {
			s.remove(a, b)
			s.caret = a
		})
	})
}

// Paste вставляет текст из буфера в итог.
func (m *MergeView) Paste() {
	m.do(func() {
		text := ClipboardGetText()
		if text == "" || m.docs[MergeResult].readOnly {
			return
		}
		m.active = MergeResult
		m.insertTextLocked(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), dvEditOther)
	})
}

// SelectAll выделяет всю активную панель.
func (m *MergeView) SelectAll() {
	m.do(func() {
		s := m.docs[m.active]
		s.anchor = dvPos{}
		s.caret = s.end()
	})
}

// SelectedText — выделенное в активной панели.
func (m *MergeView) SelectedText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.docs[m.active]
	if !s.hasSel() {
		return ""
	}
	a, b := s.sel()
	return s.textRange(a, b)
}

// ─── Каретка ────────────────────────────────────────────────────────────────

func (m *MergeView) moveLocked(to dvPos, sel bool) {
	s := m.docs[m.active]
	s.caret = s.clamp(to)
	if !sel {
		s.anchor = s.caret
	}
	s.wantCol = -1
	if m.active == MergeResult {
		m.ensureResultVisibleLocked()
		m.current = m.resultChunkAtLocked(s.caret.line)
	} else {
		m.ensureSideVisibleLocked()
	}
}

// moveRowsLocked двигает каретку на dr экранных строк, сохраняя колонку.
func (m *MergeView) moveRowsLocked(dr int, sel bool) {
	s := m.docs[m.active]
	if s.wantCol < 0 {
		s.wantCol = diffview.DisplayCol([]rune(s.text.Lines[s.caret.line]), s.caret.col)
	}
	want := s.wantCol
	line := min(max(0, s.caret.line+dr), len(s.text.Lines)-1)
	s.caret = s.clamp(dvPos{line, diffview.RawCol([]rune(s.text.Lines[line]), want)})
	if !sel {
		s.anchor = s.caret
	}
	s.wantCol = want
	if m.active == MergeResult {
		m.ensureResultVisibleLocked()
		m.current = m.resultChunkAtLocked(s.caret.line)
	} else {
		m.ensureSideVisibleLocked()
	}
}

// ensureSideVisibleLocked подкручивает верхние панели к каретке стороны.
func (m *MergeView) ensureSideVisibleLocked() {
	s := m.docs[m.active]
	if m.active == MergeResult || s.caret.line >= len(s.lineRow) {
		return
	}
	row := s.lineRow[s.caret.line]
	if row < 0 {
		return
	}
	viewH := m.topViewH()
	y := float64(dvTopPad + row*dvLineH)
	switch {
	case y < m.scroll:
		m.setScrollLocked(y)
	case y+dvLineH > m.scroll+viewH:
		m.setScrollLocked(y + dvLineH - viewH)
	}
}

// homeLocked — умный Home: сначала к первому непробельному, потом к краю.
func (m *MergeView) homeLocked(sel bool) {
	s := m.docs[m.active]
	raw := []rune(s.text.Lines[s.caret.line])
	ind := len([]rune(diffview.IndentOf(string(raw))))
	to := dvPos{s.caret.line, ind}
	if s.caret.col == ind {
		to.col = 0
	}
	m.moveLocked(to, sel)
}

// ─── Клавиатура ─────────────────────────────────────────────────────────────

// AcceptsTab — контракт TabAcceptor: в итоге Tab вставляет табуляцию, а не
// уводит фокус. В панелях сторон Tab остаётся навигацией: там не правят.
func (m *MergeView) AcceptsTab() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active == MergeResult && !m.docs[MergeResult].readOnly
}

// OnKeyEvent — клавиатура контрола.
func (m *MergeView) OnKeyEvent(e KeyEvent) {
	if !e.Pressed {
		return
	}
	ctrl, shift, alt := e.Mod&ModCtrl != 0, e.Mod&ModShift != 0, e.Mod&ModAlt != 0

	// Ctrl+S и решения по конфликту — вне общей ветки: у них свои события.
	if ctrl && e.Code == KeyS {
		if f := m.OnSaveRequest; f != nil {
			f()
		}
		m.do(func() { m.emitCommandLocked("SaveCommand", nil) })
		return
	}
	if alt && !ctrl {
		switch e.Code {
		case Key1:
			m.ResolveCurrent(MergeTakeOurs)
			return
		case Key2:
			m.ResolveCurrent(MergeTakeBase)
			return
		case Key3:
			m.ResolveCurrent(MergeTakeTheirs)
			return
		case Key0:
			m.ResolveCurrent(MergeUnresolved)
			return
		}
	}
	m.do(func() { m.keyLocked(e, ctrl, shift) })
}

func (m *MergeView) keyLocked(e KeyEvent, ctrl, shift bool) {
	s := m.docs[m.active]
	editable := m.active == MergeResult && !s.readOnly

	switch e.Code {
	case KeyF7:
		if shift {
			m.goToChunkLocked(m.prevConflictFromLocked())
		} else {
			m.goToChunkLocked(m.nextConflictLocked(m.current, +1))
		}
		return
	case KeyLeft:
		if ctrl {
			m.moveLocked(s.wordLeft(s.caret), shift)
		} else if s.caret.col == 0 && s.caret.line > 0 {
			m.moveLocked(dvPos{s.caret.line - 1, s.lineLen(s.caret.line - 1)}, shift)
		} else {
			m.moveLocked(dvPos{s.caret.line, s.caret.col - 1}, shift)
		}
		return
	case KeyRight:
		if ctrl {
			m.moveLocked(s.wordRight(s.caret), shift)
		} else if s.caret.col >= s.lineLen(s.caret.line) && s.caret.line+1 < len(s.text.Lines) {
			m.moveLocked(dvPos{s.caret.line + 1, 0}, shift)
		} else {
			m.moveLocked(dvPos{s.caret.line, s.caret.col + 1}, shift)
		}
		return
	case KeyUp:
		m.moveRowsLocked(-1, shift)
		return
	case KeyDown:
		m.moveRowsLocked(+1, shift)
		return
	case KeyPageUp:
		m.moveRowsLocked(-m.pageRowsLocked(), shift)
		return
	case KeyPageDown:
		m.moveRowsLocked(+m.pageRowsLocked(), shift)
		return
	case KeyHome:
		if ctrl {
			m.moveLocked(dvPos{}, shift)
			return
		}
		m.homeLocked(shift)
		return
	case KeyEnd:
		if ctrl {
			m.moveLocked(s.end(), shift)
			return
		}
		m.moveLocked(dvPos{s.caret.line, s.lineLen(s.caret.line)}, shift)
		return
	case KeyA:
		if ctrl {
			s.anchor, s.caret = dvPos{}, s.end()
			return
		}
	case KeyC:
		if ctrl {
			m.copyLocked()
			return
		}
	case KeyInsert:
		switch {
		case ctrl:
			m.copyLocked()
			return
		case shift && editable:
			m.pasteLocked()
			return
		}
	}

	if !editable {
		return
	}

	switch e.Code {
	case KeyZ:
		if ctrl {
			m.undoRedoLocked(shift)
			return
		}
	case KeyY:
		if ctrl {
			m.undoRedoLocked(true)
			return
		}
	case KeyX:
		if ctrl {
			m.cutLocked()
			return
		}
	case KeyV:
		if ctrl {
			m.pasteLocked()
			return
		}
	case KeyBackspace:
		m.backspaceLocked(ctrl)
		return
	case KeyDelete:
		if shift {
			m.cutLocked()
			return
		}
		m.deleteLocked(ctrl)
		return
	case KeyEnter:
		m.newlineLocked()
		return
	case KeyTab:
		m.insertTextLocked("\t", dvEditType)
		return
	}
	if e.Rune >= ' ' && !ctrl {
		m.insertTextLocked(string(e.Rune), dvEditType)
	}
}

// prevConflictFromLocked — конфликт перед текущим (или последний).
func (m *MergeView) prevConflictFromLocked() int {
	from := m.current
	if from < 0 {
		from = len(m.chunks)
	}
	return m.nextConflictLocked(from, -1)
}

func (m *MergeView) pageRowsLocked() int {
	h := m.topViewH()
	if m.active == MergeResult {
		g := m.geom()
		h = float64(max(0, g.ry1-g.ry0))
	}
	return max(1, int(h/dvLineH)-1)
}

// undoRedoLocked — общая ветка Ctrl+Z / Ctrl+Shift+Z / Ctrl+Y под замком.
func (m *MergeView) undoRedoLocked(redo bool) {
	if redo {
		if len(m.redo) == 0 {
			return
		}
		m.undo = append(m.undo, m.snapshotLocked())
		m.restoreLocked(m.redo[len(m.redo)-1])
		m.redo = m.redo[:len(m.redo)-1]
	} else {
		if len(m.undo) == 0 {
			return
		}
		m.redo = append(m.redo, m.snapshotLocked())
		m.restoreLocked(m.undo[len(m.undo)-1])
		m.undo = m.undo[:len(m.undo)-1]
	}
	m.resolvedEvent, m.editsEvent = true, true
	m.afterEditLocked()
}

func (m *MergeView) cutLocked() {
	m.copyLocked()
	s := m.docs[MergeResult]
	a, b := m.selOrLineLocked(s)
	m.editLocked(dvEditOther, func(s *dvDoc) {
		s.remove(a, b)
		s.caret = a
	})
}

func (m *MergeView) pasteLocked() {
	text := ClipboardGetText()
	if text == "" {
		return
	}
	m.insertTextLocked(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), dvEditOther)
}

// ─── Мышь ───────────────────────────────────────────────────────────────────

// paneAtLocked — панель под точкой и её текстовая область.
func (m *MergeView) paneAtLocked(x, y int) (MergeSide, bool) {
	g := m.geom()
	p := image.Pt(x, y)
	if p.In(g.resultBounds()) {
		return MergeResult, true
	}
	for i := 0; i < 3; i++ {
		if i == int(MergeBase) && !m.showBase {
			continue
		}
		if p.In(image.Rect(g.px[i][0], g.ty0, g.px[i][1], g.ty1)) {
			return MergeSide(i), true
		}
	}
	return MergeResult, false
}

// paneCodeXLocked — левый край кода панели (после номеров строк).
func (m *MergeView) paneCodeXLocked(g mvGeom, side MergeSide) (x0, x1, codeX int) {
	if side == MergeResult {
		r := g.resultBounds()
		x0, x1 = r.Min.X, r.Max.X
	} else {
		x0, x1 = g.px[side][0], g.px[side][1]
	}
	n := len(m.docs[side].text.Lines)
	digits := max(2, len(strconv.Itoa(max(1, n))))
	return x0, x1, x0 + int(float64(digits)*m.charW) + 18
}

// hitPosLocked — позиция в тексте панели под точкой.
func (m *MergeView) hitPosLocked(side MergeSide, x, y int) dvPos {
	s := m.docs[side]
	if len(s.rows) == 0 || len(s.text.Lines) == 0 {
		return dvPos{}
	}
	g := m.geom()
	var row int
	if side == MergeResult {
		row = int((float64(y-g.ry0) + m.rscroll - dvTopPad) / dvLineH)
	} else {
		row = int((float64(y-g.ty0) + m.scroll - dvTopPad) / dvLineH)
	}
	row = max(0, min(row, len(s.rows)-1))
	if s.rows[row].kind == dvRowPad {
		// Пустое место чужого блока: ближайшая настоящая строка выше.
		for row > 0 && s.rows[row].kind == dvRowPad {
			row--
		}
	}
	line := s.rows[row].line
	if line >= len(s.text.Lines) {
		return s.clamp(dvPos{})
	}
	_, _, codeX := m.paneCodeXLocked(g, side)
	dc := int(math.Round((float64(x-codeX) + m.hscroll) / m.charW))
	return s.clamp(dvPos{line, diffview.RawCol([]rune(s.text.Lines[line]), max(0, dc))})
}

// WantsCapture — контрол берёт мышь на выделение и перетаскивание разделителя.
func (m *MergeView) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.onRulerLocked(e.X, e.Y) || m.onSplitterLocked(e.Y) {
		return true
	}
	_, ok := m.paneAtLocked(e.X, e.Y)
	return ok && m.hitButtonLocked(e.X, e.Y) < 0
}

func (m *MergeView) onSplitterLocked(y int) bool {
	g := m.geom()
	return y >= g.splitY-mvSplitH && y <= g.splitY+mvSplitH
}

// OnMouseButton — клики: выбор панели, каретка, кнопки решения, разделитель.
func (m *MergeView) OnMouseButton(e MouseEvent) bool {
	handled := false
	m.do(func() {
		switch {
		case e.Button == MouseLeft && e.Pressed:
			handled = m.leftPressLocked(e)
		case e.Button == MouseLeft && !e.Pressed:
			m.dragSel, m.splitDrag, m.rulerDrag = false, false, false
			handled = true
		case e.Button == MouseRight && e.Pressed:
			handled = m.rightPressLocked(e)
		}
	})
	return handled
}

func (m *MergeView) leftPressLocked(e MouseEvent) bool {
	// Полоса-обзор — раньше разделителя: она пересекает его по высоте, и
	// нажатие на неё иначе начинало бы двигать границу панелей.
	if m.onRulerLocked(e.X, e.Y) {
		m.rulerPressLocked(e.Y)
		return true
	}
	if m.onSplitterLocked(e.Y) {
		m.splitDrag = true
		return true
	}
	if btn := m.hitButtonLocked(e.X, e.Y); btn >= 0 {
		ci, how := m.buttonActionLocked(btn)
		m.resolveLocked(ci, how)
		return true
	}
	side, ok := m.paneAtLocked(e.X, e.Y)
	if !ok {
		return false
	}
	m.active = side
	m.focused = true
	s := m.docs[side]

	now := time.Now()
	pt := image.Pt(e.X, e.Y)
	if now.Sub(m.lastClickAt) < 400*time.Millisecond && abs(pt.X-m.lastClickPt.X) < 4 && abs(pt.Y-m.lastClickPt.Y) < 4 {
		m.clicks++
	} else {
		m.clicks = 1
	}
	m.lastClickAt, m.lastClickPt = now, pt

	pos := m.hitPosLocked(side, e.X, e.Y)
	switch m.clicks {
	case 2:
		a, b := s.wordAt(pos)
		s.anchor, s.caret = a, b
	case 3:
		s.anchor = dvPos{pos.line, 0}
		s.caret = s.clamp(dvPos{pos.line + 1, 0})
	default:
		s.caret = pos
		if e.Mod&ModShift == 0 {
			s.anchor = pos
		}
		m.dragSel = true
		m.dragSide = side
	}
	s.wantCol = -1
	if side == MergeResult {
		m.current = m.resultChunkAtLocked(s.caret.line)
	} else if row := s.lineRow[s.caret.line]; row >= 0 {
		m.current = s.rows[row].chunk
	}
	return true
}

func (m *MergeView) rightPressLocked(e MouseEvent) bool {
	side, ok := m.paneAtLocked(e.X, e.Y)
	if !ok {
		return false
	}
	m.active = side
	return true
}

// OnMouseMove — выделение протяжкой, перетаскивание разделителя, подсветка
// кнопок под курсором.
func (m *MergeView) OnMouseMove(x, y int) {
	m.do(func() {
		if m.rulerDrag {
			m.rulerDragLocked(y)
			return
		}
		if m.splitDrag {
			g := m.geom()
			inner := float64(g.b.Dy() - 2*dvOuterPad)
			if inner > 0 {
				m.split = min(max(0.2, (float64(y-g.b.Min.Y-dvOuterPad))/inner), 0.85)
				m.clampScrollLocked()
			}
			return
		}
		if m.dragSel {
			s := m.docs[m.dragSide]
			s.caret = m.hitPosLocked(m.dragSide, x, y)
			s.wantCol = -1
			return
		}
		btn := m.hitButtonLocked(x, y)
		chunk := -1
		if side, ok := m.paneAtLocked(x, y); ok {
			chunk = m.chunkAtPointLocked(side, y)
		}
		if btn != m.hoverBtn || chunk != m.hoverChunk {
			m.hoverBtn, m.hoverChunk = btn, chunk
		}
	})
}

// chunkAtPointLocked — блок под точкой в панели.
func (m *MergeView) chunkAtPointLocked(side MergeSide, y int) int {
	g := m.geom()
	if side == MergeResult {
		row := int((float64(y-g.ry0) + m.rscroll - dvTopPad) / dvLineH)
		return m.resultChunkAtLocked(row)
	}
	row := int((float64(y-g.ty0) + m.scroll - dvTopPad) / dvLineH)
	for ci, sp := range m.spans {
		if row >= sp.from && row < sp.to {
			return ci
		}
	}
	return -1
}

// OnMouseWheelPixels — колесо: прокручивается часть под курсором, вторая при
// синхронной прокрутке идёт следом (SetSyncScroll).
func (m *MergeView) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	handled := false
	m.do(func() {
		side, ok := m.paneAtLocked(x, y)
		if !ok {
			return
		}
		if side == MergeResult {
			before := m.rscroll
			m.setResultScrollLocked(m.rscroll - dy)
			handled = m.rscroll != before
			return
		}
		before := m.scroll
		m.setScrollLocked(m.scroll - dy)
		handled = m.scroll != before
	})
	return handled
}

// Cursor — курсор над контролом: текст в панелях, «север-юг» над разделителем.
func (m *MergeView) Cursor(x, y int) Cursor {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.onRulerLocked(x, y) {
		return CursorArrow
	}
	if m.onSplitterLocked(y) {
		return CursorSizeNS
	}
	if m.hitButtonLocked(x, y) >= 0 {
		return CursorHand
	}
	if _, ok := m.paneAtLocked(x, y); ok {
		return CursorIBeam
	}
	return CursorArrow
}

// SetFocused — контракт фокуса: каретка мигает только у контрола в фокусе.
func (m *MergeView) SetFocused(v bool) {
	m.do(func() { m.focused = v })
}

// IsFocused сообщает, в фокусе ли контрол.
func (m *MergeView) IsFocused() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.focused
}

// ContextMenuAt — контракт ContextMenuProvider: меню решений по конфликту и
// правки итога.
func (m *MergeView) ContextMenuAt(x, y int) *PopupMenu {
	m.mu.Lock()
	ci := -1
	if side, ok := m.paneAtLocked(x, y); ok {
		ci = m.chunkAtPointLocked(side, y)
	}
	conflict := ci >= 0 && ci < len(m.chunks) && m.chunks[ci].Conflict
	readOnly := m.docs[MergeResult].readOnly
	m.mu.Unlock()

	var items []MenuItem
	if conflict {
		items = append(items,
			MenuItem{Text: Tr("merge.ctx.ours"), OnClick: func() { m.Resolve(ci, MergeTakeOurs) }},
			MenuItem{Text: Tr("merge.ctx.theirs"), OnClick: func() { m.Resolve(ci, MergeTakeTheirs) }},
			MenuItem{Text: Tr("merge.ctx.both"), OnClick: func() { m.Resolve(ci, MergeTakeOursThenTheirs) }},
			MenuItem{Text: Tr("merge.ctx.base"), OnClick: func() { m.Resolve(ci, MergeTakeBase) }},
			MenuItem{Text: Tr("merge.ctx.unresolve"), OnClick: func() { m.Resolve(ci, MergeUnresolved) }},
			MenuItem{Separator: true},
		)
	}
	items = append(items,
		MenuItem{Text: Tr("merge.ctx.copy"), OnClick: m.Copy},
		MenuItem{Text: Tr("merge.ctx.paste"), Disabled: readOnly, OnClick: m.Paste},
		MenuItem{Separator: true},
		MenuItem{Text: Tr("merge.ctx.next"), OnClick: m.NextConflict},
		MenuItem{Text: Tr("merge.ctx.prev"), OnClick: m.PrevConflict},
	)
	m.menu.SetItems(items)
	return m.menu
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
