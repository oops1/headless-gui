package widget

// richtext_ime.go — незавершённый ввод (IME) в RichText: реализация
// IMEComposer по образцу TextBox (см. ime.go).
//
// Где живёт композиция. Её текст лежит прямо в документе, подчёркнутый, — как
// у TextBox: так он виден без правок в отрисовке, каретка стоит там, где её
// ждут, а раскладка считается с учётом набираемого. Отдельно помнится
// диапазон (richIME), чтобы заменить его следующей композицией.
//
// Как это связано с историей отмены. TextBox пишет композицию мимо истории
// (spliceRaw), у RichDocument такого пути нет: каждая правка записывается.
// Поэтому композиция записывается как обычная вставка, а перед каждой
// следующей предыдущая ОТМЕНЯЕТСЯ — откат снимает её запись целиком, вместе с
// замещённым выделением. К коммиту в истории остаётся ровно одна запись: та,
// что вставила итоговый текст. Промежуточные композиции в ней не оседают.
// Стек возврата, который эти откаты затрагивают, запоминается в начале
// композиции и восстанавливается при отмене: иначе набор, который человек
// отменил, лишил бы его возврата прежних правок.

import (
	"slices"
	"unicode/utf8"
)

// richIME — состояние незавершённого ввода. Нулевое значение — композиции нет.
type richIME struct {
	active bool
	// selLo, selHi — выделение, которое композиция замещает (равные — вставка
	// без замещения); start, length — где в документе стоит текст композиции;
	// text — он же.
	selLo, selHi  int
	start, length int
	text          string
	// redo — стек возврата на момент начала композиции.
	redo []richDocEntry
}

// richIMEClean оставляет в тексте композиции только печатаемое: перевод строки
// из композиции разбил бы абзац, а управляющие символы нечем нарисовать.
func richIMEClean(s string) string {
	ok := true
	for _, r := range s {
		if r < 32 || r == 0x7f {
			ok = false
			break
		}
	}
	if ok {
		return s
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 32 && r != 0x7f {
			out = append(out, r)
		}
	}
	return string(out)
}

// imeApplyLocked заменяет прежнюю композицию (если была) на text. final — это
// уже итог ввода (коммит), а не композиция: он без подчёркивания и остаётся
// в истории. Пустой text убирает композицию и возвращает выделение, как было.
// true — в документе появился текст. Вызывать под t.mu; после вызова
// производное состояние сверяет вызывающий (contentChangedLocked).
func (t *RichText) imeApplyLocked(text string, caret int, final bool) bool {
	d := t.rdoc
	var lo, hi int
	if t.ime.active {
		// Откат прежней композиции: её запись — верхняя, пока ввод не
		// завершён (другие правки сначала принимают её, см. edit).
		lo, hi = t.ime.selLo, t.ime.selHi
		d.Undo()
		d.redo = t.ime.redo
		t.syncDocLocked()
	} else {
		lo, hi = t.selOrCaretLocked()
		t.ime.redo = slices.Clone(d.redo)
	}
	text = richIMEClean(text)
	redo := t.ime.redo

	if text == "" {
		t.ime = richIME{}
		if lo < hi {
			t.selAnchor, t.selCaret = lo, hi
		} else {
			t.setCaretEditLocked(lo)
		}
		return false
	}

	style := t.styleForLocked(lo, hi)
	if !final {
		style.Underline = true
	}
	end := t.replaceLocked(lo, hi, func(at int) int { return d.Insert(at, text, style) })
	if final {
		t.ime = richIME{}
		t.setCaretEditLocked(end)
		return true
	}
	n := utf8.RuneCountInString(text)
	t.ime = richIME{active: true, selLo: lo, selHi: hi, start: lo, length: n, text: text, redo: redo}
	pos := lo + n
	if caret >= 0 && caret <= n {
		pos = lo + caret
	}
	t.setCaretEditLocked(pos)
	return true
}

// imeFinishLocked принимает незавершённую композицию как обычный текст: так
// поступают с ней всё, что не IME, — щелчок, клавиша, команда панели,
// потеря фокуса. true — документ изменился. Вызывать под t.mu.
func (t *RichText) imeFinishLocked() bool {
	if !t.ime.active {
		return false
	}
	changed := t.imeApplyLocked(t.ime.text, 0, true)
	t.contentChangedLocked()
	return changed
}

// imeFlush — imeFinishLocked для вызова без замка: щелчок мыши, клавиша,
// потеря фокуса. OnChange вызывается уже без замка.
func (t *RichText) imeFlush() {
	t.mu.Lock()
	changed := t.imeFinishLocked()
	onCh := t.OnChange
	t.mu.Unlock()
	if changed {
		t.Invalidate()
		if onCh != nil {
			onCh()
		}
	}
}

// IMESetComposition показывает набираемый текст на месте каретки (поверх
// выделения — вместо него). caret — позиция курсора внутри композиции, в
// рунах. OnChange не вызывается: текст ещё не введён, а его вид меняется на
// каждое нажатие.
func (t *RichText) IMESetComposition(text string, caret int) {
	t.mu.Lock()
	if !t.Editable || !t.IsEnabled() {
		t.mu.Unlock()
		return
	}
	t.layoutLocked()
	t.imeApplyLocked(text, caret, false)
	t.contentChangedLocked()
	t.mu.Unlock()
	t.Invalidate()
}

// IMECommit завершает ввод: набранное становится обычным текстом — одна запись
// отмены на весь ввод, а не на каждую промежуточную композицию. Коммит без
// предшествующей композиции (часть систем присылает готовый текст сразу)
// вставляется так же.
func (t *RichText) IMECommit(text string) {
	t.mu.Lock()
	if !t.Editable || !t.IsEnabled() {
		t.mu.Unlock()
		return
	}
	t.layoutLocked()
	changed := t.imeApplyLocked(text, 0, true)
	t.contentChangedLocked()
	onCh := t.OnChange
	t.mu.Unlock()
	t.Invalidate()
	if changed && onCh != nil {
		onCh()
	}
}

// IMECancel убирает набранное без следа: документ, история и выделение как до
// начала ввода.
func (t *RichText) IMECancel() {
	t.mu.Lock()
	if !t.Editable {
		t.mu.Unlock()
		return
	}
	t.layoutLocked()
	t.imeApplyLocked("", 0, false)
	t.contentChangedLocked()
	t.mu.Unlock()
	t.Invalidate()
}
