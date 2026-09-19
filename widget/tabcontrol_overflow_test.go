package widget

import (
	"image"
	"testing"
)

// Вкладки, не поместившиеся в полосу, раньше просто уезжали за правый край:
// вкладка есть, а открыть её нечем. Тесты держат новое поведение — лишние
// заголовки уходят в меню под шевроном, активная из полосы не исчезает.

// tabSlotOf отдаёт прямоугольник заголовка вкладки (пустой — не показана).
func tabSlotOf(tc *TabControl, idx int) image.Rectangle {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	rects, _, _ := tc.tabSlots()
	return rects[idx]
}

func wideTabs(n int) []TabItem {
	tabs := make([]TabItem, n)
	for i := range tabs {
		tabs[i] = TabItem{Header: "Заголовок вкладки номер " + string(rune('A'+i))}
	}
	return tabs
}

func TestTabControl_Overflow_HidesExtraTabs(t *testing.T) {
	useTestMeasurer(t)

	tc := NewTabControl(wideTabs(8)...)
	tc.SetBounds(image.Rect(0, 0, 400, 300))

	if tc.OverflowCount() == 0 {
		t.Fatal("восемь широких заголовков влезли в 400px — тест бесполезен")
	}
	// Показанные заголовки не выходят за полосу (место под шеврон учтено).
	limit := 400 - tabChevronW
	shown := 0
	for i := 0; i < tc.TabCount(); i++ {
		r := tabSlotOf(tc, i)
		if r.Empty() {
			continue
		}
		shown++
		if r.Max.X > limit {
			t.Errorf("вкладка %d кончается на %d — залезла под шеврон (limit=%d)", i, r.Max.X, limit)
		}
	}
	if shown == 0 {
		t.Error("в полосе не показано ни одной вкладки")
	}
	if shown+tc.OverflowCount() != tc.TabCount() {
		t.Errorf("показано %d + спрятано %d ≠ всего %d", shown, tc.OverflowCount(), tc.TabCount())
	}
}

func TestTabControl_Overflow_ActiveTabStaysVisible(t *testing.T) {
	useTestMeasurer(t)

	tc := NewTabControl(wideTabs(8)...)
	tc.SetBounds(image.Rect(0, 0, 400, 300))

	last := tc.TabCount() - 1
	tc.SetActive(last)
	if r := tabSlotOf(tc, last); r.Empty() {
		t.Fatal("активная вкладка не показана в полосе — до неё не добраться мышью")
	}
	// Уехавшие влево тоже попадают в меню, а не пропадают совсем.
	if tc.OverflowCount() == 0 {
		t.Error("при активной последней вкладке остальные обязаны уйти в меню")
	}
	if r := tabSlotOf(tc, 0); !r.Empty() {
		t.Error("первая вкладка осталась в полосе — места на все не хватает")
	}
}

func TestTabControl_Overflow_MenuSwitchesTab(t *testing.T) {
	useTestMeasurer(t)

	tc := NewTabControl(wideTabs(8)...)
	tc.SetBounds(image.Rect(0, 0, 400, 300))

	var gotIdx int = -1
	var gotHeader string
	tc.OnTabChange = func(index int, header string) { gotIdx, gotHeader = index, header }

	tc.mu.Lock()
	_, hidden, chevron := tc.tabSlots()
	tc.mu.Unlock()
	if chevron.Empty() {
		t.Fatal("шеврона нет, хотя вкладки не поместились")
	}

	// Щелчок по шеврону открывает меню из спрятанных вкладок.
	if !tc.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: false,
		X: chevron.Min.X + chevron.Dx()/2, Y: chevron.Min.Y + chevron.Dy()/2}) {
		t.Fatal("щелчок по шеврону не обработан")
	}
	if !tc.HasOverlay() {
		t.Fatal("меню переполнения не открылось")
	}

	items := tc.overflowTabItems(hidden)
	if len(items) != len(hidden) {
		t.Fatalf("пунктов меню %d, спрятано вкладок %d", len(items), len(hidden))
	}
	want := hidden[len(hidden)-1]
	items[len(items)-1].OnClick()
	if tc.Active() != want {
		t.Errorf("активная вкладка %d, ждал %d — выбор из меню не сработал", tc.Active(), want)
	}
	if gotIdx != want || gotHeader != tc.TabHeader(want) {
		t.Errorf("OnTabChange получил (%d, %q), ждал (%d, %q)",
			gotIdx, gotHeader, want, tc.TabHeader(want))
	}
	if r := tabSlotOf(tc, want); r.Empty() {
		t.Error("выбранная из меню вкладка в полосе не показана")
	}
	tc.Dismiss()
}

func TestTabControl_Overflow_NoChevronWhenTabsFit(t *testing.T) {
	useTestMeasurer(t)

	tc := NewTabControl(TabItem{Header: "A"}, TabItem{Header: "B"})
	tc.SetBounds(image.Rect(0, 0, 400, 300))

	if tc.OverflowCount() != 0 {
		t.Fatalf("две короткие вкладки не поместились в 400px: спрятано %d", tc.OverflowCount())
	}
	// Раскладка прежняя: заголовки подряд от левого края, без зазора под шеврон.
	w0 := rawMeasure("A", DefaultFontSizePt) + tc.TabPadH*2
	r0, r1 := tabSlotOf(tc, 0), tabSlotOf(tc, 1)
	if r0.Min.X != 0 || r0.Dx() != w0 {
		t.Errorf("первая вкладка %v, ждал ширину %d от нуля", r0, w0)
	}
	if r1.Min.X != r0.Max.X {
		t.Errorf("вторая вкладка начинается на %d, ждал %d", r1.Min.X, r0.Max.X)
	}
}
