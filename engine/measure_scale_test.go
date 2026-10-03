package engine

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// MeasureUIText обязан отвечать в ЛОГИЧЕСКИХ пикселях при любом масштабе.
//
// Измеритель регистрировался значением метода холста, а SetScale заменяет
// холст копией: метод оставался привязан к первому холсту, у которого масштаб
// прежний, а шрифты общие и уже перестроены под физический DPI. В итоге на
// масштабе 2 ширина строки приходила вдвое больше, и всё, что считает перенос
// до кадра (RichText, TextBox, диалоги), переносило строки раньше времени.
func TestMeasureUIText_LogicalAfterSetScale(t *testing.T) {
	const text = "Длинный абзац текста для проверки переноса слов"
	e := New(400, 100, 1)
	defer e.Stop()

	base := widget.MeasureUIText(text, 12)
	if base <= 0 {
		t.Fatalf("ширина на масштабе 1: %d", base)
	}
	for _, k := range []float64{1.5, 2} {
		e.SetScale(k)
		got := widget.MeasureUIText(text, 12)
		// Логическая ширина — та же с точностью до округления шрифта в
		// физическом размере; физическая была бы в k раз больше.
		if d := got - base; d < -4 || d > 4 {
			t.Errorf("масштаб %v: ширина %d, на масштабе 1 было %d — похоже на физическую", k, got, base)
		}
	}
}
