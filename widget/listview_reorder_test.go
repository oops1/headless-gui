package widget

import (
	"image"
	"strings"
	"testing"
)

// GG-85: строки списка переставляются перетаскиванием, а программное
// выделение шлёт OnSelect. Раньше ни того, ни другого не было: окно
// «Настроить панель инструментов» обходилось кнопками «Вверх» и «Вниз».

type listFixture struct {
	lv       *ListView
	reorders [][2]int
	selects  []int
}

// Список 200×120, строки по 28 точек: a, b, c, d.
func newListFixture(t *testing.T) *listFixture {
	t.Helper()
	f := &listFixture{lv: NewListView("a", "b", "c", "d")}
	f.lv.Reorderable = true
	f.lv.OnReorder = func(from, to int) { f.reorders = append(f.reorders, [2]int{from, to}) }
	f.lv.OnSelect = func(idx int, _ string) { f.selects = append(f.selects, idx) }
	f.lv.SetBounds(image.Rect(0, 0, 200, 120))
	return f
}

func (f *listFixture) items() string { return strings.Join(f.lv.Items(), ",") }

// rowY — центр строки i.
func rowY(lv *ListView, i int) int { return i*lv.ItemHeight + lv.ItemHeight/2 }

// Строку тащат вниз: порядок меняется, событие приходит с новым индексом,
// выделение едет за строкой.
func TestListReorder_DragDown(t *testing.T) {
	f := newListFixture(t)

	press(f.lv, 20, rowY(f.lv, 0))
	f.lv.OnMouseMove(20, rowY(f.lv, 1))
	f.lv.OnMouseMove(20, 80) // ниже середины третьей строки — вставка после неё
	release(f.lv, 20, 80)

	if got := f.items(); got != "b,c,a,d" {
		t.Fatalf("порядок %q, ждал b,c,a,d", got)
	}
	if len(f.reorders) != 1 || f.reorders[0] != [2]int{0, 2} {
		t.Fatalf("события перестановки: %v, ждал одно {0 2}", f.reorders)
	}
	if got := f.lv.Selected(); got != 2 {
		t.Fatalf("выделение %d, ждал 2 — оно едет за строкой", got)
	}
}

// Строку тащат вверх.
func TestListReorder_DragUp(t *testing.T) {
	f := newListFixture(t)

	press(f.lv, 20, rowY(f.lv, 3))
	f.lv.OnMouseMove(20, 40)
	f.lv.OnMouseMove(20, 10) // выше середины первой строки — в самое начало
	release(f.lv, 20, 10)

	if got := f.items(); got != "d,a,b,c" {
		t.Fatalf("порядок %q, ждал d,a,b,c", got)
	}
	if len(f.reorders) != 1 || f.reorders[0] != [2]int{3, 0} {
		t.Fatalf("события перестановки: %v, ждал одно {3 0}", f.reorders)
	}
}

// Щелчок без движения остаётся щелчком: порядок прежний, выбор сообщён.
func TestListReorder_ClickIsNotDrag(t *testing.T) {
	f := newListFixture(t)

	press(f.lv, 20, rowY(f.lv, 1))
	f.lv.OnMouseMove(20, rowY(f.lv, 1)+2) // меньше порога
	release(f.lv, 20, rowY(f.lv, 1)+2)

	if got := f.items(); got != "a,b,c,d" {
		t.Fatalf("порядок после щелчка %q", got)
	}
	if len(f.reorders) != 0 {
		t.Fatalf("щелчок дал перестановку: %v", f.reorders)
	}
	if len(f.selects) != 1 || f.selects[0] != 1 {
		t.Fatalf("выбор: %v, ждал один — строку 1", f.selects)
	}
}

// Без Reorderable перетаскивание не работает.
func TestListReorder_OffByDefault(t *testing.T) {
	f := newListFixture(t)
	f.lv.Reorderable = false

	press(f.lv, 20, rowY(f.lv, 0))
	f.lv.OnMouseMove(20, 80)
	release(f.lv, 20, 80)

	if got := f.items(); got != "a,b,c,d" {
		t.Fatalf("порядок изменился без Reorderable: %q", got)
	}
	if len(f.reorders) != 0 {
		t.Fatalf("событие без Reorderable: %v", f.reorders)
	}
}

// Программное выделение шлёт OnSelect; тихое — нет.
func TestListReorder_SetSelectedNotifies(t *testing.T) {
	f := newListFixture(t)

	f.lv.SetSelected(2)
	if len(f.selects) != 1 || f.selects[0] != 2 {
		t.Fatalf("SetSelected: события %v, ждал одно со строкой 2", f.selects)
	}
	f.lv.SetSelected(2) // то же самое — молча
	if len(f.selects) != 1 {
		t.Fatalf("повтор того же выделения прислал событие: %v", f.selects)
	}
	f.lv.SetSelectedQuiet(0)
	if len(f.selects) != 1 {
		t.Fatalf("SetSelectedQuiet прислал событие: %v", f.selects)
	}
	if got := f.lv.Selected(); got != 0 {
		t.Fatalf("тихое выделение не сработало: %d", got)
	}
}

// Разметка: <ListView Reorderable="True">, а SelectedIndex не шлёт событие.
func TestListReorder_XAML(t *testing.T) {
	_, reg, err := LoadUIFromXAML([]byte(`<Window Title="T" Width="300" Height="200">
  <ListView Name="lv" Left="0" Top="0" Width="200" Height="120" Reorderable="True" SelectedIndex="1">
    <Item Content="a"/>
    <Item Content="b"/>
  </ListView>
</Window>`))
	if err != nil {
		t.Fatal(err)
	}
	lv := reg["lv"].(*ListView)
	if !lv.Reorderable {
		t.Fatal("Reorderable из разметки не применился")
	}
	if got := lv.Selected(); got != 1 {
		t.Fatalf("SelectedIndex из разметки: %d", got)
	}
}
