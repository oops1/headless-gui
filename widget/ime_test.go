package widget

import (
	"image"
	"testing"
)

// Китайский, японский и корейский набираются не по букве: человек печатает
// слоги, система показывает кандидатов, и только выбранный вариант становится
// текстом. До поля доходили лишь готовые символы, и набрать иероглиф было
// нельзя вовсе.

func newIMEBox(t *testing.T, text string) *TextBox {
	t.Helper()
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 300, 120))
	tb.SetText(text)
	return tb
}

func TestIME_CompositionShownAndReplaced(t *testing.T) {
	tb := newIMEBox(t, "")

	tb.IMESetComposition("に", 1)
	if got := tb.GetText(); got != "に" {
		t.Errorf("набираемое не показано: %q", got)
	}
	// Следующий слог приходит ЦЕЛИКОМ, а не добавкой: прежнее набранное
	// должно уйти, иначе в поле накопится «ににほ».
	tb.IMESetComposition("にほ", 2)
	if got := tb.GetText(); got != "にほ" {
		t.Errorf("набираемое накопилось: %q", got)
	}
	if got := tb.CaretPosition(); got != 2 {
		t.Errorf("каретка %d, ждал 2 (конец набираемого)", got)
	}
}

func TestIME_CommitBecomesText(t *testing.T) {
	tb := newIMEBox(t, "")
	tb.IMESetComposition("にほん", 3)
	tb.IMECommit("日本")

	if got := tb.GetText(); got != "日本" {
		t.Errorf("после выбора варианта в поле %q", got)
	}
	// Введённое больше не композиция: следующий набор не должен его стереть.
	tb.IMESetComposition("ご", 1)
	if got := tb.GetText(); got != "日本ご" {
		t.Errorf("введённое стёрлось: %q", got)
	}
}

func TestIME_CancelRemovesComposition(t *testing.T) {
	tb := newIMEBox(t, "текст")
	tb.SetCaretPosition(5)

	tb.IMESetComposition("にほ", 2)
	if got := tb.GetText(); got != "текстにほ" {
		t.Errorf("набираемое не на месте каретки: %q", got)
	}
	tb.IMECancel()
	if got := tb.GetText(); got != "текст" {
		t.Errorf("после отмены осталось %q", got)
	}
	if got := tb.CaretPosition(); got != 5 {
		t.Errorf("каретка после отмены %d, ждал 5", got)
	}
}

// Набираемое встаёт на место каретки, а не в конец текста.
func TestIME_CompositionAtCaret(t *testing.T) {
	tb := newIMEBox(t, "абвгд")
	tb.SetCaretPosition(2)

	tb.IMESetComposition("X", 1)
	if got := tb.GetText(); got != "абXвгд" {
		t.Errorf("набираемое встало не туда: %q", got)
	}
	tb.IMECommit("Ж")
	if got := tb.GetText(); got != "абЖвгд" {
		t.Errorf("после выбора варианта %q", got)
	}
}

// Поле только для чтения ввода не принимает.
func TestIME_ReadOnlyIgnores(t *testing.T) {
	tb := newIMEBox(t, "текст")
	tb.ReadOnly = true

	tb.IMESetComposition("に", 1)
	if got := tb.GetText(); got != "текст" {
		t.Errorf("поле только для чтения приняло набор: %q", got)
	}
}

// Курсор внутри набираемого ставится туда, куда просит система: по нему
// человек видит, какой слог правится.
func TestIME_CaretInsideComposition(t *testing.T) {
	tb := newIMEBox(t, "")
	tb.IMESetComposition("にほん", 1)
	if got := tb.CaretPosition(); got != 1 {
		t.Errorf("каретка внутри набираемого %d, ждал 1", got)
	}
}

// То же самое в однострочном поле.
func TestIME_TextInput(t *testing.T) {
	ti := NewTextInput("")
	ti.SetBounds(image.Rect(0, 0, 200, 30))
	ti.SetText("а")

	ti.IMESetComposition("に", 1)
	if got := ti.GetText(); got != "аに" {
		t.Errorf("набираемое: %q", got)
	}
	ti.IMESetComposition("にほ", 2)
	if got := ti.GetText(); got != "аにほ" {
		t.Errorf("набираемое накопилось: %q", got)
	}
	ti.IMECommit("日")
	if got := ti.GetText(); got != "а日" {
		t.Errorf("после выбора варианта: %q", got)
	}
}

// Место каретки нужно системе, чтобы поставить окно кандидатов под набираемым
// словом, а не в углу экрана.
func TestIME_CaretRect(t *testing.T) {
	tb := newIMEBox(t, "первая строка")
	tb.SetBounds(image.Rect(40, 20, 340, 140))
	tb.SetCaretPosition(6)

	r := tb.IMECaretRect()
	if r.Empty() {
		t.Fatal("место каретки неизвестно")
	}
	if r.Min.X <= 40 || r.Min.Y < 20 {
		t.Errorf("прямоугольник каретки %v — вне поля", r)
	}
	if r.Dy() <= 0 {
		t.Errorf("высота каретки %d", r.Dy())
	}
}
