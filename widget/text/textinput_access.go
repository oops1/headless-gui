package text

import "github.com/oops1/headless-gui/v4/widget/base"

// textinput_access.go — текст TextInput и TextBox для вспомогательных
// технологий (AccessTextProvider и сеттеры из accesstext.go): методы читают
// внутренности полей, поэтому живут с типами.

// AccessText — содержимое поля. У поля пароля скринридеру отдаётся пустая
// строка: роль уже сказала, что это пароль, а читать его вслух нельзя.
func (t *TextInput) AccessText() string {
	if t.IsPasswordMode() {
		return ""
	}
	return t.GetText()
}

func (t *TextInput) AccessCaret() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.caretPos
}

func (t *TextInput) AccessSelection() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.selActive() {
		return t.caretPos, t.caretPos
	}
	return t.normSel()
}

func (t *TextInput) AccessReadOnly() bool { return !t.IsEnabled() }

func (t *TextInput) AccessSetCaret(pos int) bool {
	t.mu.Lock()
	if pos < 0 {
		pos = 0
	}
	if pos > len(t.runes) {
		pos = len(t.runes)
	}
	t.caretPos = pos
	t.selStart, t.selEnd = -1, -1
	t.mu.Unlock()
	t.Invalidate()
	return true
}

func (t *TextInput) AccessSetSelection(from, to int) bool {
	t.mu.Lock()
	n := len(t.runes)
	from, to = base.AccessClampPair(from, to, n)
	t.caretPos = to
	if from == to {
		t.selStart, t.selEnd = -1, -1
	} else {
		t.selStart, t.selEnd = from, to
	}
	t.mu.Unlock()
	t.Invalidate()
	return true
}

func (t *TextInput) AccessSetText(s string) bool {
	if !t.IsEnabled() {
		return false
	}
	t.SetText(s)
	return true
}

func (t *TextBox) AccessText() string { return t.GetText() }

func (t *TextBox) AccessCaret() int { return t.CaretPosition() }

func (t *TextBox) AccessSelection() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.selActive() {
		return t.caret, t.caret
	}
	return t.normSel()
}

func (t *TextBox) AccessReadOnly() bool { return t.ReadOnly || !t.IsEnabled() }

func (t *TextBox) AccessSetCaret(pos int) bool {
	t.SetCaretPosition(pos)
	return true
}

func (t *TextBox) AccessSetSelection(from, to int) bool {
	t.mu.Lock()
	from, to = base.AccessClampPair(from, to, len(t.runes))
	t.caret = to
	t.selAnchor = from
	if from == to {
		t.selAnchor = -1
	}
	t.desiredX = -1
	t.ensureLayout()
	t.ensureCaretVisible()
	t.mu.Unlock()
	t.Invalidate()
	return true
}

func (t *TextBox) AccessSetText(s string) bool {
	if t.AccessReadOnly() {
		return false
	}
	t.SetText(s)
	return true
}
