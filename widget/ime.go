package widget

// ime.go — незавершённый ввод (композиция) в текстовых полях.
//
// Китайский, японский и корейский набираются не по букве: человек печатает
// слоги, система показывает список кандидатов, и только выбранный вариант
// становится текстом. Пока он не выбран, набранное — КОМПОЗИЦИЯ: она видна в
// поле, подчёркнута, но ещё не введена.
//
// Движок этого не знал вовсе: до поля доходили только готовые символы, и
// набрать иероглиф в нём было нельзя — ни одного. Теперь система передаёт
// композицию сюда, а поле показывает её на месте каретки и заменяет на
// следующую, пока ввод не завершится.
//
// Текст композиции хранится прямо в содержимом поля, а не рядом: так его
// видно без правок в отрисовке, каретка стоит там, где её ждут, а раскладка
// строк считается с учётом набираемого. Отдельно помнится лишь ДИАПАЗОН —
// чтобы заменить его следующей композицией и подчеркнуть при отрисовке.

import "image"

// IMEComposer — виджет, принимающий незавершённый ввод.
//
// Смещения и длины — в рунах. Пустая композиция означает «ничего не
// набрано»: так система сообщает, что прежнюю надо убрать.
type IMEComposer interface {
	// IMESetComposition показывает промежуточный текст; caret — позиция
	// курсора ВНУТРИ него (в рунах).
	IMESetComposition(text string, caret int)
	// IMECommit завершает ввод: текст становится содержимым поля.
	IMECommit(text string)
	// IMECancel отменяет ввод: набранное убирается без следа.
	IMECancel()
	// IMECaretRect — место каретки в координатах холста (логические
	// пиксели). По нему система ставит окно кандидатов, иначе оно появится
	// в углу экрана, далеко от набираемого слова.
	IMECaretRect() image.Rectangle
}

// imeState — диапазон композиции внутри текста поля.
type imeState struct {
	active bool
	start  int // начало в рунах
	length int // длина в рунах
}

// clear забывает композицию (её текст к этому времени либо принят, либо
// удалён вызывающим).
func (s *imeState) clear() { *s = imeState{} }

// set запоминает новый диапазон.
func (s *imeState) set(start, length int) {
	if length <= 0 {
		s.clear()
		return
	}
	*s = imeState{active: true, start: start, length: length}
}

// imeRange — диапазон композиции, если он есть.
func (s *imeState) imeRange() (int, int, bool) {
	if !s.active || s.length <= 0 {
		return 0, 0, false
	}
	return s.start, s.start + s.length, true
}

// ─── TextBox (много строк) ──────────────────────────────────────────────────

// IMESetComposition показывает набираемый текст на месте каретки.
func (t *TextBox) IMESetComposition(text string, caret int) {
	if t.ReadOnly || !t.IsEnabled() {
		return
	}
	rs := []rune(text)
	t.mu.Lock()
	t.imeReplaceLocked(rs, caret)
	t.mu.Unlock()
	t.Invalidate()
}

// IMECommit завершает ввод: набранное становится обычным текстом.
func (t *TextBox) IMECommit(text string) {
	if t.ReadOnly || !t.IsEnabled() {
		return
	}
	rs := []rune(text)
	t.mu.Lock()
	t.imeReplaceLocked(rs, len(rs))
	from := t.caret
	if f, _, ok := t.ime.imeRange(); ok {
		// Набранное стало обычным текстом — это правка, и её можно отменить.
		// Пока шла композиция, она в историю не писалась: текст менялся на
		// каждом нажатии, и запись на каждое была бы мусором.
		from = f
		t.pending = append(t.pending, tbEdit{pos: f, inserted: append([]rune(nil), rs...)})
	}
	t.commitUndo(from)
	t.ime.clear() // текст уже на месте — он больше не композиция
	t.mu.Unlock()
	t.Invalidate()
}

// IMECancel убирает набранное без следа.
func (t *TextBox) IMECancel() {
	t.mu.Lock()
	t.imeReplaceLocked(nil, 0)
	t.commitUndo(t.caret) // выделение, замещённое набором, — настоящая правка: её можно отменить
	t.mu.Unlock()
	t.Invalidate()
}

// imeReplaceLocked заменяет прежнюю композицию на новую. Вызывать под t.mu.
//
// Текст композиции меняется мимо истории правок (spliceRaw): он временный, и
// записывать каждый его промежуточный вид значило бы засорить откат. Исключение
// — выделение, которое набор заместил: его удаление настоящее и записывается.
func (t *TextBox) imeReplaceLocked(rs []rune, caret int) {
	if from, to, ok := t.ime.imeRange(); ok {
		// Прежняя композиция уходит целиком: система всегда присылает
		// набранное заново, а не добавку к нему.
		if to > len(t.runes) {
			to = len(t.runes)
		}
		if from <= to {
			t.spliceRaw(from, to-from, nil)
			t.caret = from
		}
		t.ime.clear()
		t.dirty = true
	}
	t.clampCaret()
	if len(rs) > 0 {
		t.deleteSel()
	}
	start := t.caret
	if len(rs) > 0 {
		t.spliceRaw(start, 0, rs)
		t.caret = start + len(rs)
		t.ime.set(start, len(rs))
		if caret >= 0 && caret <= len(rs) {
			t.caret = start + caret
		}
	}
	t.selAnchor = -1
	t.ensureLayout()
	t.ensureCaretVisible()
}

// IMECaretRect — место каретки в координатах холста: по нему система ставит
// окно кандидатов под набираемым словом, а не в углу экрана.
func (t *TextBox) IMECaretRect() image.Rectangle {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureLayout()
	li, cx := t.caretPoint()
	lh := t.lineHeight()
	b := t.bounds
	x := b.Min.X + t.PaddingX + cx - t.scrollX
	y := b.Min.Y + t.PaddingY + li*lh - t.scrollY
	return image.Rect(x, y, x+1, y+lh)
}

// imeUnderline — диапазон композиции для отрисовки (подчёркивание).
func (t *TextBox) imeUnderline() (int, int, bool) { return t.ime.imeRange() }

// ─── TextInput (одна строка) ────────────────────────────────────────────────

func (t *TextInput) IMESetComposition(text string, caret int) {
	if !t.IsEnabled() {
		return
	}
	rs := []rune(text)
	t.mu.Lock()
	t.imeReplaceLocked(rs, caret)
	t.mu.Unlock()
	t.Invalidate()
}

func (t *TextInput) IMECommit(text string) {
	if !t.IsEnabled() {
		return
	}
	rs := []rune(text)
	t.mu.Lock()
	t.imeReplaceLocked(rs, len(rs))
	t.ime.clear()
	t.mu.Unlock()
	t.Invalidate()
}

func (t *TextInput) IMECancel() {
	t.mu.Lock()
	t.imeReplaceLocked(nil, 0)
	t.mu.Unlock()
	t.Invalidate()
}

// imeReplaceLocked — то же, что у TextBox: прежняя композиция уходит целиком,
// на её место встаёт новая. Вызывать под t.mu.
func (t *TextInput) imeReplaceLocked(rs []rune, caret int) {
	if from, to, ok := t.ime.imeRange(); ok {
		if to > len(t.runes) {
			to = len(t.runes)
		}
		if from <= to {
			t.runes = append(t.runes[:from], t.runes[to:]...)
			t.caretPos = from
		}
		t.ime.clear()
	}
	if t.caretPos < 0 {
		t.caretPos = 0
	}
	if t.caretPos > len(t.runes) {
		t.caretPos = len(t.runes)
	}
	start := t.caretPos
	if len(rs) > 0 {
		ins := make([]rune, 0, len(t.runes)+len(rs))
		ins = append(ins, t.runes[:start]...)
		ins = append(ins, rs...)
		ins = append(ins, t.runes[start:]...)
		t.runes = ins
		t.ime.set(start, len(rs))
		t.caretPos = start + len(rs)
		if caret >= 0 && caret <= len(rs) {
			t.caretPos = start + caret
		}
	}
	t.selStart, t.selEnd = -1, -1
}

// IMECaretRect — место каретки в координатах холста.
func (t *TextInput) IMECaretRect() image.Rectangle {
	b := t.Bounds()
	return image.Rect(b.Min.X+t.PaddingX, b.Min.Y+4, b.Min.X+t.PaddingX+1, b.Max.Y-4)
}

// imeUnderline — диапазон композиции для отрисовки.
func (t *TextInput) imeUnderline() (int, int, bool) { return t.ime.imeRange() }
