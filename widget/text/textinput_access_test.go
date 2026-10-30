package text

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

func TestAccessText_TextBox(t *testing.T) {
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 200, 100))
	tb.SetText("первая строка\nвторая")

	tp, ok := base.AccessTextOf(tb)
	if !ok {
		t.Fatal("многострочное поле не отдаёт текстовую семантику")
	}
	if got := tp.AccessText(); got != "первая строка\nвторая" {
		t.Errorf("текст %q", got)
	}

	tb.SetCaretPosition(7)
	if got := tp.AccessCaret(); got != 7 {
		t.Errorf("каретка %d, ждал 7", got)
	}
	// Без выделения границы совпадают с кареткой: «ничего не выделено».
	if from, to := tp.AccessSelection(); from != to {
		t.Errorf("выделение %d..%d без выделения", from, to)
	}
	if tp.AccessReadOnly() {
		t.Error("обычное поле объявлено доступным только для чтения")
	}
	tb.ReadOnly = true
	if !tp.AccessReadOnly() {
		t.Error("поле только для чтения объявлено редактируемым")
	}
}

// Пароль скринридеру не читают: роль уже сказала, что это пароль.
func TestAccessText_PasswordHidden(t *testing.T) {
	ti := NewTextInput("")
	ti.SetBounds(image.Rect(0, 0, 200, 30))
	ti.SetText("секрет")
	ti.SetPasswordMode(true)

	tp, _ := base.AccessTextOf(ti)
	if got := tp.AccessText(); got != "" {
		t.Errorf("пароль отдан как %q", got)
	}
}

// Скринридер водит человека по тексту и переставляет каретку сам.
func TestAccessText_SetCaret(t *testing.T) {
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 200, 100))
	tb.SetText("первая строка")

	cs, ok := any(tb).(base.AccessCaretSetter)
	if !ok {
		t.Fatal("каретку переставить нечем")
	}
	cs.AccessSetCaret(6)
	if got := tb.CaretPosition(); got != 6 {
		t.Errorf("каретка %d, ждал 6", got)
	}
	// Позиция за концом текста усекается, а не ломает поле.
	cs.AccessSetCaret(1000)
	if got := tb.CaretPosition(); got != len([]rune("первая строка")) {
		t.Errorf("каретка за концом: %d", got)
	}
}

func TestAccessText_SetText(t *testing.T) {
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 200, 100))
	tb.SetText("старое")

	setter := any(tb).(base.AccessTextSetter)
	if !setter.AccessSetText("новое") || tb.GetText() != "новое" {
		t.Errorf("текст не заменён: %q", tb.GetText())
	}

	tb.ReadOnly = true
	if setter.AccessSetText("ещё") {
		t.Error("поле только для чтения приняло правку")
	}
	if tb.GetText() != "новое" {
		t.Errorf("текст изменился вопреки запрету: %q", tb.GetText())
	}
}

// Многострочный текст — документ, однострочное поле — поле ввода: по этому
// различию скринридер решает, как читать содержимое.
func TestAccessRole_Document(t *testing.T) {
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 200, 100))
	if got := base.BuildAccessTree(tb, nil).Role; got != base.RoleDocument {
		t.Errorf("роль многострочного текста %q", got)
	}

	ti := NewTextInput("")
	ti.SetBounds(image.Rect(0, 0, 200, 30))
	if got := base.BuildAccessTree(ti, nil).Role; got != base.RoleTextInput {
		t.Errorf("роль поля ввода %q", got)
	}
}
