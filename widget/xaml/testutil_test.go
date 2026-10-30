package xaml

// Помощники XAML-тестов. Неэкспортированного состояния ядра они не трогают —
// только через internal/testhook и открытый API, — поэтому переезжают в
// widget/xaml вместе с тестами (ADR-0012, 8.2b).

import (
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testhook"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// SaveStrings запоминает таблицы переводов и возвращает их после теста.
// Звать перед ClearStrings.
func SaveStrings(t *testing.T) {
	t.Helper()
	t.Cleanup(testhook.SaveStrings())
}

// testRuneW — ширина руны в тестовом измерителе (неравномерная, как у шрифта).
func testRuneW(r rune) int {
	switch {
	case r == ' ':
		return 4
	case r == 'i' || r == 'l':
		return 3
	case r >= 'а' && r <= 'я':
		return 8
	default:
		return 5 + int(r)%4
	}
}

// testMeasure — детерминированный измеритель текста XAML-тестов.
func testMeasure(text string, sizePt float64) int {
	w := 0
	for _, r := range text {
		w += testRuneW(r)
	}
	return int(float64(w) * sizePt / 14)
}

// useTestMeasure ставит testMeasure на время теста.
func useTestMeasure(t testing.TB) {
	t.Helper()
	restore := testhook.SaveTextMeasurer()
	base.SetTextMeasurer(testMeasure)
	t.Cleanup(restore)
}

// LanguageListenerCount — число подписчиков смены языка.
func LanguageListenerCount() int { return testhook.LanguageListeners() }

// CheckBoxAutoWidth — ширина флажка по содержимому, как её считает CheckBox:
// квадрат 16, зазор 6, подпись, поле фокуса 4.
func CheckBoxAutoWidth(cb *base.CheckBox) int {
	const boxSize, textPad, focusPad = 16, 6, 4
	if cb.Text == "" {
		return boxSize
	}
	size := cb.FontSize
	if size <= 0 {
		size = base.DefaultFontSize()
	}
	return boxSize + textPad + base.MeasureTextFor(cb, cb.Text, size) + focusPad
}
