package widget

import "slices"

// textbox_undo.go — отмена и возврат правок многострочного редактора.
//
// Раньше каждая правка клала в стек снимок ВСЕГО текста, глубина — 200. На
// документе в мегабайты это двести копий документа (сотни мегабайт) и копия
// целого текста на каждое нажатие клавиши, даже стрелки. Теперь хранится сама
// правка — где, что убрали и что вставили: её размер зависит от правки, а не
// от документа.

// tbUndoDepth — сколько правок помнит редактор.
const tbUndoDepth = 200

// tbEdit — одна замена: на месте [pos, pos+len(removed)) стояли руны removed,
// теперь стоят inserted.
type tbEdit struct {
	pos      int
	removed  []rune
	inserted []rune
}

// tbUndoEntry — одно действие пользователя: одна и более замен, выполненных
// подряд (выделение удалили и вставили на его место). Откат — с конца.
//
// Каретка хранится дважды — как хранили снимки: куда вернуть при отмене и
// куда при возврате. Второе заполняется в момент отмены текущим положением
// каретки, а не «концом правки»: так положение каретки после цепочки
// отмена-возврат осталось тем же, что было у снимков, и привычное поведение не
// поменялось.
type tbUndoEntry struct {
	edits     []tbEdit
	caretUndo int
	caretRedo int
}

// splice заменяет n рун с позиции pos на ins и запоминает замену для отмены.
// Единственный путь правки текста, который попадает в историю: всё, что
// меняет t.runes мимо него (IME, откат), либо временное, либо само история.
// Вызывать под t.mu.
func (t *TextBox) splice(pos, n int, ins []rune) {
	if n == 0 && len(ins) == 0 {
		return
	}
	t.pending = append(t.pending, tbEdit{
		pos:      pos,
		removed:  slices.Clone(t.runes[pos : pos+n]),
		inserted: slices.Clone(ins),
	})
	t.spliceRaw(pos, n, ins)
}

// spliceRaw — замена без записи в историю. Правит срез на месте: вставка
// посреди документа раньше выделяла новый срез размером с документ на каждое
// нажатие клавиши. Вызывать под t.mu.
func (t *TextBox) spliceRaw(pos, n int, ins []rune) {
	t.runes = slices.Replace(t.runes, pos, pos+n, ins...)
	t.dirty = true
}

// commitUndo закрывает действие пользователя: замены, накопленные splice,
// становятся одной записью истории. caretBefore — каретка до действия.
// Вызывать под t.mu.
func (t *TextBox) commitUndo(caretBefore int) {
	if len(t.pending) == 0 {
		return
	}
	e := tbUndoEntry{edits: t.pending, caretUndo: caretBefore}
	t.pending = nil
	t.undoStack = append(t.undoStack, e)
	if n := len(t.undoStack); n > tbUndoDepth {
		copy(t.undoStack, t.undoStack[n-tbUndoDepth:])
		clear(t.undoStack[tbUndoDepth:])
		t.undoStack = t.undoStack[:tbUndoDepth]
	}
	t.redoStack = nil
}

// resetUndo забывает историю: после замены всего текста снаружи записанные
// позиции указывают в чужой документ, и откат испортил бы текст. Вызывать под
// t.mu.
func (t *TextBox) resetUndo() {
	t.undoStack, t.redoStack, t.pending = nil, nil, nil
}

// editsFit — каждая замена из цепочки применима к тексту, длина которого
// растёт и убывает по ходу цепочки.
func editsFit(edits []tbEdit, length int) bool {
	for i := range edits {
		e := &edits[i]
		if e.pos < 0 || e.pos+len(e.removed) > length {
			return false
		}
		length += len(e.inserted) - len(e.removed)
	}
	return true
}

// undo отменяет последнее действие. Вызывать под t.mu.
func (t *TextBox) undo() {
	if len(t.undoStack) == 0 || t.ime.active {
		// Во время набора текст композиции лежит в runes, но в историю не
		// записан: откат на нём применил бы правки к чужим позициям.
		return
	}
	e := t.undoStack[len(t.undoStack)-1]
	t.undoStack = t.undoStack[:len(t.undoStack)-1]

	// Отменять в обратном порядке: каждая замена записана относительно
	// текста, каким он стал после предыдущих.
	rev := make([]tbEdit, len(e.edits))
	for i, ed := range e.edits {
		rev[len(e.edits)-1-i] = tbEdit{pos: ed.pos, removed: ed.inserted, inserted: ed.removed}
	}
	if !editsFit(rev, len(t.runes)) {
		t.resetUndo() // история разошлась с текстом — лучше потерять её, чем текст
		return
	}
	for _, ed := range rev {
		t.spliceRaw(ed.pos, len(ed.removed), ed.inserted)
	}
	e.caretRedo = t.caret
	t.caret = e.caretUndo
	t.selAnchor = -1
	t.clampCaret()
	t.redoStack = append(t.redoStack, e)
}

// redo возвращает последнее отменённое действие. Вызывать под t.mu.
func (t *TextBox) redo() {
	if len(t.redoStack) == 0 || t.ime.active {
		return
	}
	e := t.redoStack[len(t.redoStack)-1]
	t.redoStack = t.redoStack[:len(t.redoStack)-1]

	if !editsFit(e.edits, len(t.runes)) {
		t.resetUndo()
		return
	}
	for _, ed := range e.edits {
		t.spliceRaw(ed.pos, len(ed.removed), ed.inserted)
	}
	e.caretUndo = t.caret
	t.caret = e.caretRedo
	t.selAnchor = -1
	t.clampCaret()
	t.undoStack = append(t.undoStack, e)
}
