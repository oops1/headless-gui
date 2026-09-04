package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// Ctrl+Click и Shift+Click по строкам таблицы — запрос GG-29.
//
// DataGrid умел множественный выбор с самого начала: selectRow(row, shift,
// ctrl) честно делает и toggle, и диапазон. Позвать это было нечем —
// widget.MouseEvent нёс только координаты и кнопку, и в движке на месте
// вызова стояло `false, false` с TODO. Снаружи выделялась ровно одна строка.
//
// Проверяется весь путь: SetModifiers → SendMouseButton → MouseEvent.Mod →
// DataGridWidget → selectRow.

type modRow struct{ Name string }

func modGrid(t *testing.T) (*engine.Engine, *widget.DataGridWidget) {
	t.Helper()
	root := widget.NewPanel(widget.Theme{}.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))

	w := widget.NewDataGridWidget()
	w.Grid.RowHeight = 20
	w.Grid.HeaderHeight = 20
	w.Grid.SelectionMode = datagrid.SelectionExtended
	col := datagrid.NewTextColumn("Имя", "Name")
	col.SetActualWidth(200)
	w.Grid.AddColumn(col)

	oc := datagrid.NewObservableCollection()
	for _, n := range []string{"один", "два", "три", "четыре", "пять"} {
		oc.Add(&modRow{Name: n})
	}
	w.Grid.SetItemsSource(oc)
	w.SetBounds(image.Rect(0, 0, 300, 200))
	root.AddChild(w)

	eng := engine.New(400, 300, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	return eng, w
}

// rowY — середина строки row в координатах холста.
func rowY(w *widget.DataGridWidget, row int) int {
	return w.Bounds().Min.Y + w.Grid.HeaderHeight + row*w.Grid.RowHeight + w.Grid.RowHeight/2
}

func click(eng *engine.Engine, x, y int, mod widget.KeyMod) {
	eng.SetModifiers(mod)
	eng.SendMouseButton(x, y, widget.MouseLeft, true)
	eng.SendMouseButton(x, y, widget.MouseLeft, false)
	eng.SetModifiers(widget.ModNone)
}

func TestDataGrid_CtrlClickAddsRow(t *testing.T) {
	eng, w := modGrid(t)

	click(eng, 40, rowY(w, 0), widget.ModNone)
	if got := len(w.Grid.SelectedItems()); got != 1 {
		t.Fatalf("после обычного щелчка выделено %d строк, ждали одну", got)
	}

	click(eng, 40, rowY(w, 2), widget.ModCtrl)
	got := w.Grid.SelectedItems()
	if len(got) != 2 {
		t.Fatalf("после Ctrl+Click выделено %d строк, ждали две", len(got))
	}

	// Повторный Ctrl+Click по той же строке снимает её — это toggle.
	click(eng, 40, rowY(w, 2), widget.ModCtrl)
	if got := len(w.Grid.SelectedItems()); got != 1 {
		t.Errorf("повторный Ctrl+Click оставил %d строк, ждали одну", got)
	}
}

func TestDataGrid_ShiftClickSelectsRange(t *testing.T) {
	eng, w := modGrid(t)

	click(eng, 40, rowY(w, 1), widget.ModNone)
	click(eng, 40, rowY(w, 3), widget.ModShift)

	if got := len(w.Grid.SelectedItems()); got != 3 {
		t.Errorf("Shift+Click выделил %d строк, ждали три (1..3)", got)
	}
}

// Без модификаторов поведение прежнее: щелчок оставляет одну строку.
func TestDataGrid_PlainClickKeepsOneRow(t *testing.T) {
	eng, w := modGrid(t)

	click(eng, 40, rowY(w, 0), widget.ModNone)
	click(eng, 40, rowY(w, 3), widget.ModNone)

	if got := len(w.Grid.SelectedItems()); got != 1 {
		t.Errorf("два обычных щелчка выделили %d строк, ждали одну", got)
	}
}

// Модификаторы доезжают до виджета в самом событии, а не «где-то в движке»:
// виджет, лежащий под курсором, обязан увидеть их в MouseEvent.Mod.
func TestMouseEvent_CarriesModifiers(t *testing.T) {
	root := widget.NewPanel(widget.Theme{}.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 200))
	spy := &modSpy{}
	spy.SetBounds(image.Rect(0, 0, 200, 200))
	root.AddChild(spy)

	eng := engine.New(200, 200, 30)
	eng.SetRoot(root)
	eng.RenderOnce()

	eng.SetModifiers(widget.ModCtrl | widget.ModShift)
	eng.SendMouseButton(50, 50, widget.MouseLeft, true)

	if spy.mod != widget.ModCtrl|widget.ModShift {
		t.Errorf("виджет получил модификаторы %v, ждали Ctrl|Shift", spy.mod)
	}
	if got := eng.Modifiers(); got != widget.ModCtrl|widget.ModShift {
		t.Errorf("движок помнит модификаторы %v", got)
	}
}

type modSpy struct {
	widget.Base
	mod widget.KeyMod
}

func (s *modSpy) Draw(widget.DrawContext) {}

func (s *modSpy) OnMouseButton(e widget.MouseEvent) bool {
	s.mod = e.Mod
	return true
}
