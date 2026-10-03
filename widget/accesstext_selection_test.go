package widget

import "testing"

// Скринридер выделяет диапазон через паттерн Text (ITextRangeProvider.Select):
// поля обязаны принять отрезок, усечь его до длины текста и поставить каретку
// на конец. Границы приходят от клиента, который держит диапазон, пока
// пользователь печатает, — за конец текста они выходить вправе.
func TestAccessSetSelection(t *testing.T) {
	ti := NewTextInput("")
	ti.SetText("привет мир")
	tb := NewTextBox("")
	tb.SetText("привет мир")

	for name, w := range map[string]interface {
		AccessSelectionSetter
		AccessTextProvider
	}{"TextInput": ti, "TextBox": tb} {
		if !w.AccessSetSelection(7, 10) {
			t.Fatalf("%s: выделение отвергнуто", name)
		}
		if from, to := w.AccessSelection(); from != 7 || to != 10 {
			t.Errorf("%s: выделение [%d,%d), ждал [7,10)", name, from, to)
		}
		if c := w.AccessCaret(); c != 10 {
			t.Errorf("%s: каретка %d, ждал 10 (конец выделения)", name, c)
		}

		// За концом текста и вперемешку: усекается и упорядочивается.
		w.AccessSetSelection(500, 3)
		if from, to := w.AccessSelection(); from != 3 || to != 10 {
			t.Errorf("%s: выделение с выходом за текст [%d,%d), ждал [3,10)", name, from, to)
		}

		// Пустой отрезок снимает выделение и ставит каретку.
		w.AccessSetSelection(4, 4)
		if from, to := w.AccessSelection(); from != 4 || to != 4 {
			t.Errorf("%s: после снятия [%d,%d), ждал каретку в 4", name, from, to)
		}
		if c := w.AccessCaret(); c != 4 {
			t.Errorf("%s: каретка %d, ждал 4", name, c)
		}
	}
}
