package text

// Помощники тестов пакета. Состояние base (измеритель текста) — через
// internal/testhook: неэкспортированное ядро тесты text не видят.

import (
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testhook"
	"github.com/oops1/headless-gui/v3/widget/base"
)

// runeW — ширина руны в тестовом измерителе (неравномерная, как у шрифта).
func runeW(r rune) int {
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

// testMeasure — детерминированный измеритель тестов.
func testMeasure(text string, sizePt float64) int {
	w := 0
	for _, r := range text {
		w += runeW(r)
	}
	return int(float64(w) * sizePt / 14)
}

// useTestMeasurer ставит детерминированный измеритель на время теста.
func useTestMeasurer(t testing.TB) {
	t.Helper()
	restore := testhook.SaveTextMeasurer()
	base.SetTextMeasurer(testMeasure)
	t.Cleanup(restore)
}

// rawMeasure — тестовый измеритель без мемоизации (тесты ставят его
// useTestMeasurer).
func rawMeasure(text string, sizePt float64) int { return testMeasure(text, sizePt) }

// glyphCtx — контекст, который знает про глифы шрифта по умолчанию.
type glyphCtx struct {
	base.DrawContext
	has map[rune]bool
}

func (c glyphCtx) HasGlyph(r rune) bool { return c.has[r] }
