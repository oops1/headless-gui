package widget

import (
	"image"
	"testing"
)

// TestTabControl_HitTest_NoDrawAfterAddTab проверяет, что попадание по
// вкладке считается верно СРАЗУ после добавления новой вкладки — даже если
// Draw ни разу не вызывался. Раньше ширины вкладок узнавались только внутри
// Draw (кэш tc.tabWidths), а хит-тест читал этот кэш; после того как движок
// научился пропускать Draw для поддеревьев вне изменившейся области
// (SkipSubtree), кэш мог быть пустым/устаревшим, и клик уходил бы мимо.
func TestTabControl_HitTest_NoDrawAfterAddTab(t *testing.T) {
	useTestMeasurer(t)

	tc := NewTabControl(TabItem{Header: "A"})
	tc.SetBounds(image.Rect(0, 0, 400, 300))

	// Вторую вкладку добавляем уже после SetBounds — и Draw НИ РАЗУ не
	// вызывается за весь тест.
	tc.AddTab("Второй заголовок подлиннее", nil)

	// Независимо от layoutTabs считаем ожидаемые границы вручную — тем же
	// измерителем, что зарегистрирован useTestMeasurer, — чтобы не сверять
	// код сам с собой.
	w0 := rawMeasure("A", DefaultFontSizePt) + tc.TabPadH*2
	w1 := rawMeasure("Второй заголовок подлиннее", DefaultFontSizePt) + tc.TabPadH*2
	if w0 <= 0 || w1 <= 0 {
		t.Fatalf("некорректные ширины для теста: w0=%d w1=%d", w0, w1)
	}

	// Клик в середину второй вкладки должен переключить активную на неё.
	x := w0 + w1/2
	if !tc.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: false, X: x, Y: 5}) {
		t.Fatalf("клик по второй вкладке (x=%d) не обработан", x)
	}
	if got := tc.Active(); got != 1 {
		t.Fatalf("активная вкладка = %d, хотим 1 (w0=%d w1=%d, x=%d)", got, w0, w1, x)
	}
}

// TestTabControl_HitTest_NoDrawAfterHeaderChange проверяет, что после
// переименования заголовка (SetTabHeader) хит-тест сразу учитывает НОВУЮ
// ширину, а не то, что было посчитано (или не посчитано вовсе) в последнем
// Draw. Точка клика выбрана так, чтобы под старой шириной первой вкладки
// она принадлежала второй вкладке, а под новой (увеличенной) — первой:
// если бы хит-тест пользовался устаревшим кэшем, тест бы упал.
func TestTabControl_HitTest_NoDrawAfterHeaderChange(t *testing.T) {
	useTestMeasurer(t)

	tc := NewTabControl(
		TabItem{Header: "A"},
		TabItem{Header: "B"},
	)
	tc.SetBounds(image.Rect(0, 0, 400, 300))

	oldW0 := rawMeasure("A", DefaultFontSizePt) + tc.TabPadH*2

	// Draw ни разу не вызывался. Меняем заголовок первой вкладки на
	// значительно более длинный.
	const longHeader = "Значительно более длинный заголовок"
	tc.SetTabHeader(0, longHeader)

	newW0 := rawMeasure(longHeader, DefaultFontSizePt) + tc.TabPadH*2
	if newW0 <= oldW0 {
		t.Fatalf("тестовый заголовок должен быть шире исходного: old=%d new=%d", oldW0, newW0)
	}

	// Точка чуть правее старой границы первой вкладки, но внутри новой —
	// раньше (по кэшу из Draw, которого не было) она бы считалась вне
	// первой вкладки вообще (кэш пуст → fallback-ширина) или досталась
	// второй вкладке.
	x := oldW0 + 2
	if x >= newW0 {
		t.Fatalf("тестовая точка x=%d должна попадать внутрь новой ширины первой вкладки (%d)", x, newW0)
	}

	if !tc.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: false, X: x, Y: 5}) {
		t.Fatalf("клик по первой вкладке (x=%d) не обработан", x)
	}
	if got := tc.Active(); got != 0 {
		t.Fatalf("активная вкладка = %d, хотим 0 (oldW0=%d newW0=%d, x=%d)", got, oldW0, newW0, x)
	}
}

// TestTabControl_HitTest_HoverNoDraw — то же самое, но для наведения
// (OnMouseMove/hoverIdx), которое тоже раньше зависело от кэша Draw.
func TestTabControl_HitTest_HoverNoDraw(t *testing.T) {
	useTestMeasurer(t)

	tc := NewTabControl(TabItem{Header: "A"})
	tc.SetBounds(image.Rect(0, 0, 400, 300))
	tc.AddTab("Ещё одна вкладка", nil)

	w0 := rawMeasure("A", DefaultFontSizePt) + tc.TabPadH*2
	w1 := rawMeasure("Ещё одна вкладка", DefaultFontSizePt) + tc.TabPadH*2

	tc.OnMouseMove(w0+w1/2, 5)
	tc.mu.Lock()
	hover := tc.hoverIdx
	tc.mu.Unlock()
	if hover != 1 {
		t.Fatalf("hoverIdx = %d, хотим 1 (w0=%d w1=%d)", hover, w0, w1)
	}
}
