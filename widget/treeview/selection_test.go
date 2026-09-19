package treeview

import (
	"image"
	"testing"
)

// GG-86: дерево умеет выбирать набор узлов, а не только один.
//
// Команда «выбрать устаревшие локальные ветки, чтобы удалить их разом» упиралась
// в единственное выделение: пометить удавалось лишь один узел.

type selFixture struct {
	tv     *TreeView
	items  []*TreeViewItem
	events [][]string
}

// Дерево из четырёх корневых узлов: a, b, c, d. Строки по 22 точки.
func newSelFixture(t *testing.T, mode SelectionMode) *selFixture {
	t.Helper()
	f := &selFixture{tv: New()}
	f.tv.SelectionMode = mode
	for _, name := range []string{"a", "b", "c", "d"} {
		it := NewItem(name)
		f.items = append(f.items, it)
		f.tv.AddRoot(it)
	}
	f.tv.SetBounds(image.Rect(0, 0, 200, 200))
	f.tv.OnSelectionChanged = func(e SelectionChangedEvent) {
		var names []string
		for _, it := range e.Items {
			names = append(names, it.Text)
		}
		f.events = append(f.events, names)
	}
	return f
}

// names — выбранные узлы по именам.
func (f *selFixture) names() []string {
	var out []string
	for _, it := range f.tv.SelectedItems() {
		out = append(out, it.Text)
	}
	return out
}

func (f *selFixture) click(row int, shift, ctrl bool) {
	y := row*f.tv.itemH() + f.tv.itemH()/2
	f.tv.OnMouseButtonMod(100, y, 0, 1, shift, ctrl)
	f.tv.OnMouseButtonMod(100, y, 0, 0, shift, ctrl)
}

func eq(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Ctrl добавляет и снимает узлы по одному.
func TestSelection_CtrlToggles(t *testing.T) {
	f := newSelFixture(t, SelectionExtended)

	f.click(0, false, false)
	f.click(2, false, true)
	if got := f.names(); !eq(got, "a", "c") {
		t.Fatalf("после Ctrl+клика выбраны %v, ждал [a c]", got)
	}
	if !f.items[0].IsSelected || !f.items[2].IsSelected {
		t.Fatal("отметки узлов не расставлены — подсветка рисуется по ним")
	}

	f.click(0, false, true) // снимаем первый
	if got := f.names(); !eq(got, "c") {
		t.Fatalf("после снятия выбраны %v, ждал [c]", got)
	}
	if f.items[0].IsSelected {
		t.Fatal("снятый узел остался подсвеченным")
	}
	if got := f.tv.SelectedItem(); got != f.items[0] {
		t.Fatalf("курсор выбора %v, ждал узел, по которому щёлкнули", got)
	}
}

// Shift берёт диапазон от якоря, в обе стороны.
func TestSelection_ShiftRange(t *testing.T) {
	f := newSelFixture(t, SelectionExtended)

	f.click(1, false, false)
	f.click(3, true, false)
	if got := f.names(); !eq(got, "b", "c", "d") {
		t.Fatalf("диапазон вниз: %v, ждал [b c d]", got)
	}

	f.click(0, true, false) // тот же якорь, диапазон вверх
	if got := f.names(); !eq(got, "a", "b") {
		t.Fatalf("диапазон вверх: %v, ждал [a b]", got)
	}
}

// Щелчок без модификаторов схлопывает набор до одного узла, а по уже
// выбранному — набор сохраняет (с него начинают перетаскивание или зовут
// контекстное меню на всю группу).
func TestSelection_PlainClick(t *testing.T) {
	f := newSelFixture(t, SelectionExtended)
	f.tv.SetSelectedItems([]*TreeViewItem{f.items[0], f.items[1], f.items[2]})

	f.click(1, false, false) // по выбранному
	if got := f.names(); !eq(got, "a", "b", "c") {
		t.Fatalf("щелчок по выбранному сломал набор: %v", got)
	}
	if f.tv.SelectedItem() != f.items[1] {
		t.Fatal("курсор выбора не встал на узел под курсором")
	}

	f.click(3, false, false) // по невыбранному
	if got := f.names(); !eq(got, "d") {
		t.Fatalf("щелчок по невыбранному: %v, ждал [d]", got)
	}
}

// Shift+↓ расширяет набор с клавиатуры, ↓ без Shift — схлопывает.
func TestSelection_KeyboardExtend(t *testing.T) {
	f := newSelFixture(t, SelectionExtended)
	f.click(0, false, false)

	f.tv.OnKeyEvent(keyDown, 0, true, true, false)
	f.tv.OnKeyEvent(keyDown, 0, true, true, false)
	if got := f.names(); !eq(got, "a", "b", "c") {
		t.Fatalf("Shift+↓ дал %v, ждал [a b c]", got)
	}

	f.tv.OnKeyEvent(keyDown, 0, true, false, false)
	if got := f.names(); !eq(got, "d") {
		t.Fatalf("↓ без Shift дал %v, ждал [d]", got)
	}
}

// Одиночный режим остаётся прежним: модификаторы ничего не меняют.
func TestSelection_SingleModeUnchanged(t *testing.T) {
	f := newSelFixture(t, SelectionSingle)

	f.click(0, false, false)
	f.click(2, false, true)
	if got := f.names(); !eq(got, "c") {
		t.Fatalf("в одиночном режиме Ctrl выбрал %v, ждал [c]", got)
	}
	f.click(3, true, false)
	if got := f.names(); !eq(got, "d") {
		t.Fatalf("в одиночном режиме Shift выбрал %v, ждал [d]", got)
	}
	if got := f.tv.SelectedItem(); got != f.items[3] {
		t.Fatalf("SelectedItem = %v", got)
	}
}

// Программная смена набора шлёт событие; SetSelectedItem схлопывает набор.
func TestSelection_ProgrammaticEvents(t *testing.T) {
	f := newSelFixture(t, SelectionExtended)

	f.tv.SetSelectedItems([]*TreeViewItem{f.items[0], f.items[2]})
	if got := f.names(); !eq(got, "a", "c") {
		t.Fatalf("SetSelectedItems дал %v", got)
	}
	if len(f.events) != 1 || !eq(f.events[0], "a", "c") {
		t.Fatalf("события набора: %v, ждал одно [a c]", f.events)
	}
	if !f.tv.IsItemSelected(f.items[2]) || f.tv.IsItemSelected(f.items[1]) {
		t.Fatal("IsItemSelected отвечает неверно")
	}

	f.tv.SetSelectedItem(f.items[3])
	if got := f.names(); !eq(got, "d") {
		t.Fatalf("SetSelectedItem не схлопнул набор: %v", got)
	}
	if f.items[0].IsSelected || f.items[2].IsSelected {
		t.Fatal("прежние отметки остались")
	}

	// Тот же набор второй раз события не даёт.
	n := len(f.events)
	f.tv.SetSelectedItem(f.items[3])
	if len(f.events) != n {
		t.Fatalf("повтор того же выбора прислал событие: %v", f.events)
	}
}
