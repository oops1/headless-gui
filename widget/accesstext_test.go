package widget

import (
	"image"
	"testing"
)

// Содержимое текстового поля уезжало к скринридеру одной строкой —
// «значением» элемента, как у ползунка. Скринридеру этого мало: он читает
// текст кусками, идёт за кареткой и озвучивает выделение.

func TestAccessText_TextBox(t *testing.T) {
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 200, 100))
	tb.SetText("первая строка\nвторая")

	tp, ok := AccessTextOf(tb)
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

	tp, _ := AccessTextOf(ti)
	if got := tp.AccessText(); got != "" {
		t.Errorf("пароль отдан как %q", got)
	}
}

// Скринридер водит человека по тексту и переставляет каретку сам.
func TestAccessText_SetCaret(t *testing.T) {
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 200, 100))
	tb.SetText("первая строка")

	cs, ok := any(tb).(AccessCaretSetter)
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

	setter := any(tb).(AccessTextSetter)
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

// Смещения считаются в РУНАХ: в байтах кириллическая строка дала бы вдвое
// большие числа, и скринридер встал бы не туда.
func TestAccessSubstring(t *testing.T) {
	const s = "первая строка"
	if got := AccessSubstring(s, 0, 6); got != "первая" {
		t.Errorf("отрезок %q", got)
	}
	if got := AccessRuneLen(s); got != 13 {
		t.Errorf("длина %d, ждал 13", got)
	}
	// Границы приходят от скринридера, и он вправе спросить больше, чем есть.
	if got := AccessSubstring(s, 7, 1000); got != "строка" {
		t.Errorf("отрезок за концом: %q", got)
	}
	if got := AccessSubstring(s, -5, 6); got != "первая" {
		t.Errorf("отрицательное начало: %q", got)
	}
	if got := AccessSubstring(s, 10, 3); got != "" {
		t.Errorf("перевёрнутые границы дали %q", got)
	}
}

// Многострочный текст — документ, однострочное поле — поле ввода: по этому
// различию скринридер решает, как читать содержимое.
func TestAccessRole_Document(t *testing.T) {
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 200, 100))
	if got := BuildAccessTree(tb, nil).Role; got != RoleDocument {
		t.Errorf("роль многострочного текста %q", got)
	}

	ti := NewTextInput("")
	ti.SetBounds(image.Rect(0, 0, 200, 30))
	if got := BuildAccessTree(ti, nil).Role; got != RoleTextInput {
		t.Errorf("роль поля ввода %q", got)
	}
}
