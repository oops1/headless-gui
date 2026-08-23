// Тест TextBox (ENGINE_ISSUES tts-studio #8):
// публичные SetCaretPosition/InsertAtCaret.
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestTextBox_SetCaretPosition_InsertAtCaret — issue #8.
func TestTextBox_SetCaretPosition_InsertAtCaret(t *testing.T) {
	tb := widget.NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 200, 30))
	tb.SetText("абвгд")

	if got := tb.CaretPosition(); got != 5 {
		t.Fatalf("каретка после SetText = %d, ждали 5", got)
	}

	tb.SetCaretPosition(2)
	if got := tb.CaretPosition(); got != 2 {
		t.Fatalf("SetCaretPosition(2): каретка = %d", got)
	}

	tb.InsertAtCaret("XY")
	if got := tb.GetText(); got != "абXYвгд" {
		t.Errorf("текст = %q, ждали %q", got, "абXYвгд")
	}
	if got := tb.CaretPosition(); got != 4 {
		t.Errorf("каретка после вставки = %d, ждали 4", got)
	}

	// Пара кавычек с кареткой между ними — сценарий палитры символов.
	tb.InsertAtCaret("«»")
	tb.SetCaretPosition(tb.CaretPosition() - 1)
	tb.InsertAtCaret("в кавычках")
	if got := tb.GetText(); got != "абXY«в кавычках»вгд" {
		t.Errorf("текст = %q", got)
	}

	// Клампинг за границы.
	tb.SetCaretPosition(9999)
	if got, n := tb.CaretPosition(), len([]rune(tb.GetText())); got != n {
		t.Errorf("каретка = %d, ждали %d (конец)", got, n)
	}
	tb.SetCaretPosition(-5)
	if got := tb.CaretPosition(); got != 0 {
		t.Errorf("каретка = %d, ждали 0", got)
	}
}
