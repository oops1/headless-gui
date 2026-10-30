package dock

import (
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testhook"
	"github.com/oops1/headless-gui/v3/widget/base"
)

// dock_testutil_test.go — помощники тестов дока: копии помощников тестов
// ядра; состояние base (измеритель, экран) — через internal/testhook.

// checkBoxAutoWidth — ширина флажка по содержимому, как её считает CheckBox:
// квадрат 16, зазор 6, подпись, поле фокуса 4.
func checkBoxAutoWidth(cb *base.CheckBox) int {
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

// withScreen задаёт экран 2000×2000 для раскладки попапов на время теста.
func withScreen(t *testing.T) {
	t.Helper()
	w, h := testhook.ScreenBounds()
	base.SetScreenBounds(2000, 2000)
	t.Cleanup(func() { base.SetScreenBounds(w, h) })
}

func press(w interface{ OnMouseButton(base.MouseEvent) bool }, x, y int) bool {
	return w.OnMouseButton(base.MouseEvent{X: x, Y: y, Button: base.MouseLeft, Pressed: true})
}

func release(w interface{ OnMouseButton(base.MouseEvent) bool }, x, y int) bool {
	return w.OnMouseButton(base.MouseEvent{X: x, Y: y, Button: base.MouseLeft, Pressed: false})
}

func click(w interface{ OnMouseButton(base.MouseEvent) bool }, x, y int) {
	press(w, x, y)
	release(w, x, y)
}
