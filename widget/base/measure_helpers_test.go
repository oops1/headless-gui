package base

// measure_helpers_test.go — детерминированный измеритель текста для тестов
// ядра (у тестов widget/text — своя копия в testutil_test.go).

import (
	"sync/atomic"
	"testing"
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

// useTestMeasurer ставит детерминированный измеритель на время теста.
func useTestMeasurer(t testing.TB) {
	t.Helper()
	prev := uiTextMeasurer.Load()
	measurersMu.Lock()
	prevBase := baseMeasurer
	measurersMu.Unlock()
	SetTextMeasurer(func(text string, sizePt float64) int {
		w := 0
		for _, r := range text {
			w += runeW(r)
		}
		return int(float64(w) * sizePt / 14)
	})
	t.Cleanup(func() {
		// SetTextMeasurer запоминает измеритель в baseMeasurer и публикует его в
		// uiTextMeasurer. Вернуть надо оба: иначе тестовый измеритель остаётся
		// действующим для всех следующих тестов, рассчитанных на эвристику ширины
		// (меню, обёртки, флажки) — зависимость от порядка запуска.
		measurersMu.Lock()
		baseMeasurer = prevBase
		measurersMu.Unlock()
		if prev != nil {
			uiTextMeasurer.Store(prev)
		} else {
			// В atomic.Value nil не положить; до первой публикации он пуст, и
			// MeasureUIText считает эвристикой — вернём его в то же состояние.
			uiTextMeasurer = atomic.Value{}
		}
		BumpTextMetricsRev()
	})
}

// rawMeasure — зарегистрированный измеритель без мемоизации.
func rawMeasure(text string, sizePt float64) int {
	return uiTextMeasurer.Load().(textMeasurer)(text, sizePt)
}
