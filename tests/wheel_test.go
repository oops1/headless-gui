// Package tests — прокрутка колесом мыши в скроллируемых виджетах
// (ListView, TreeView, DataGrid).
//
// Проверяется: (а) колесо вниз сдвигает прокрутку, вверх возвращает;
// (б) кламп на границах; (в) когда контент помещается — событие НЕ
// поглощается (OnMouseButton == false), чтобы всплыть к родительскому
// ScrollView; (г) сквозной путь через движок (SendMouseButton).
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	dgrid "github.com/oops1/headless-gui/v3/widget/datagrid"
	"github.com/oops1/headless-gui/v3/widget/treeview"
)

// wheelDown/wheelUp — события колеса в точке (x, y).
func wheelDown(x, y int) widget.MouseEvent {
	return widget.MouseEvent{X: x, Y: y, Button: widget.MouseWheelDown, Pressed: true}
}

func wheelUp(x, y int) widget.MouseEvent {
	return widget.MouseEvent{X: x, Y: y, Button: widget.MouseWheelUp, Pressed: true}
}

// ─── ListView ───────────────────────────────────────────────────────────────

// newScrollableListView создаёт список, содержимое которого заведомо больше
// видимой области (20 строк по 28px в окне 100px).
func newScrollableListView() *widget.ListView {
	items := make([]string, 20)
	for i := range items {
		items[i] = "item"
	}
	lv := widget.NewListView(items...)
	lv.SetBounds(image.Rect(0, 0, 200, 100))
	return lv
}

func TestWheel_ListView_ScrollsDownAndUp(t *testing.T) {
	lv := newScrollableListView()

	if got := lv.ScrollY(); got != 0 {
		t.Fatalf("начальный scrollY = %d, want 0", got)
	}

	// Вниз — прокрутка сдвигается, событие поглощено.
	if !lv.OnMouseButton(wheelDown(50, 50)) {
		t.Fatal("колесо вниз должно поглощаться, когда есть что прокручивать")
	}
	afterDown := lv.ScrollY()
	if afterDown != 3*lv.ItemHeight {
		t.Fatalf("после колеса вниз scrollY = %d, want %d", afterDown, 3*lv.ItemHeight)
	}

	// Вверх — возвращается к началу.
	if !lv.OnMouseButton(wheelUp(50, 50)) {
		t.Fatal("колесо вверх должно поглощаться, пока не у верхней границы")
	}
	if got := lv.ScrollY(); got != 0 {
		t.Fatalf("после колеса вверх scrollY = %d, want 0", got)
	}
}

func TestWheel_ListView_ClampAtBounds(t *testing.T) {
	lv := newScrollableListView()

	// У верхней границы колесо вверх ничего не двигает → не поглощается.
	if lv.OnMouseButton(wheelUp(50, 50)) {
		t.Fatal("колесо вверх у верхней границы не должно поглощаться (нечего прокручивать)")
	}
	if got := lv.ScrollY(); got != 0 {
		t.Fatalf("scrollY = %d, want 0 (кламп сверху)", got)
	}

	// Прокручиваем до самого низа.
	for i := 0; i < 50; i++ {
		lv.OnMouseButton(wheelDown(50, 50))
	}
	maxS := lv.ScrollY()
	// Дальнейшее колесо вниз клампится и не поглощается.
	if lv.OnMouseButton(wheelDown(50, 50)) {
		t.Fatal("колесо вниз у нижней границы не должно поглощаться")
	}
	if got := lv.ScrollY(); got != maxS {
		t.Fatalf("scrollY = %d, want %d (кламп снизу)", got, maxS)
	}
}

func TestWheel_ListView_ContentFits_NotConsumed(t *testing.T) {
	lv := widget.NewListView("a", "b") // 2 строки — помещаются в 100px
	lv.SetBounds(image.Rect(0, 0, 200, 100))

	if lv.OnMouseButton(wheelDown(50, 50)) {
		t.Fatal("когда контент помещается, колесо НЕ должно поглощаться (нужно всплытие к ScrollView)")
	}
	if lv.OnMouseButton(wheelUp(50, 50)) {
		t.Fatal("когда контент помещается, колесо вверх тоже НЕ должно поглощаться")
	}
}

func TestWheel_ListView_ThroughEngine(t *testing.T) {
	eng := newTestEngine()
	lv := newScrollableListView()
	eng.SetRoot(lv)

	eng.SendMouseButton(50, 50, widget.MouseWheelDown, true)
	if got := lv.ScrollY(); got <= 0 {
		t.Fatalf("сквозь движок: scrollY = %d, want > 0", got)
	}
}

// ─── TreeView ───────────────────────────────────────────────────────────────

// newScrollableTreeView создаёт дерево из 30 корневых узлов по 20px в окне 100px.
func newScrollableTreeView() *widget.TreeViewWidget {
	w := widget.NewTreeViewWidget()
	w.Tree.ItemHeight = 20
	for i := 0; i < 30; i++ {
		w.AddRoot(treeview.NewItem("node"))
	}
	w.SetBounds(image.Rect(0, 0, 200, 100))
	return w
}

func TestWheel_TreeView_ScrollsDownAndUp(t *testing.T) {
	w := newScrollableTreeView()

	if !w.OnMouseButton(wheelDown(50, 50)) {
		t.Fatal("колесо вниз должно поглощаться, когда есть что прокручивать")
	}
	afterDown := w.Tree.ScrollY()
	if afterDown != 3*w.Tree.ItemHeight {
		t.Fatalf("после колеса вниз scrollY = %d, want %d", afterDown, 3*w.Tree.ItemHeight)
	}

	if !w.OnMouseButton(wheelUp(50, 50)) {
		t.Fatal("колесо вверх должно поглощаться, пока не у верхней границы")
	}
	if got := w.Tree.ScrollY(); got != 0 {
		t.Fatalf("после колеса вверх scrollY = %d, want 0", got)
	}
}

func TestWheel_TreeView_ClampAtBounds(t *testing.T) {
	w := newScrollableTreeView()

	if w.OnMouseButton(wheelUp(50, 50)) {
		t.Fatal("колесо вверх у верхней границы не должно поглощаться")
	}

	for i := 0; i < 50; i++ {
		w.OnMouseButton(wheelDown(50, 50))
	}
	maxS := w.Tree.ScrollY()
	if w.OnMouseButton(wheelDown(50, 50)) {
		t.Fatal("колесо вниз у нижней границы не должно поглощаться")
	}
	if got := w.Tree.ScrollY(); got != maxS {
		t.Fatalf("scrollY = %d, want %d (кламп снизу)", got, maxS)
	}
}

func TestWheel_TreeView_ContentFits_NotConsumed(t *testing.T) {
	w := widget.NewTreeViewWidget()
	w.Tree.ItemHeight = 20
	w.AddRoot(treeview.NewItem("a"))
	w.AddRoot(treeview.NewItem("b"))
	w.SetBounds(image.Rect(0, 0, 200, 100)) // 2 строки помещаются

	if w.OnMouseButton(wheelDown(50, 50)) {
		t.Fatal("когда контент помещается, колесо НЕ должно поглощаться")
	}
}

func TestWheel_TreeView_ThroughEngine(t *testing.T) {
	eng := newTestEngine()
	w := newScrollableTreeView()
	eng.SetRoot(w)

	eng.SendMouseButton(50, 50, widget.MouseWheelDown, true)
	if got := w.Tree.ScrollY(); got <= 0 {
		t.Fatalf("сквозь движок: scrollY = %d, want > 0", got)
	}
}

// ─── DataGrid ───────────────────────────────────────────────────────────────

type wheelRow struct{ Name string }

// newScrollableDataGrid создаёт таблицу из 30 строк по 20px, заголовок 20px,
// в окне 120px (видимая область данных = 100px).
func newScrollableDataGrid(rows int) *widget.DataGridWidget {
	w := widget.NewDataGridWidget()
	w.Grid.RowHeight = 20
	w.Grid.HeaderHeight = 20
	col := dgrid.NewTextColumn("Name", "Name")
	col.SetActualWidth(150)
	w.Grid.AddColumn(col)

	src := dgrid.NewObservableCollection()
	for i := 0; i < rows; i++ {
		src.Add(&wheelRow{Name: "row"})
	}
	w.Grid.SetItemsSource(src)
	w.SetBounds(image.Rect(0, 0, 200, 120))
	return w
}

func TestWheel_DataGrid_ScrollsDownAndUp(t *testing.T) {
	w := newScrollableDataGrid(30)

	if !w.OnMouseButton(wheelDown(50, 50)) {
		t.Fatal("колесо вниз должно поглощаться, когда есть что прокручивать")
	}
	afterDown := w.Grid.ScrollY()
	if afterDown != 3*w.Grid.RowHeight {
		t.Fatalf("после колеса вниз scrollY = %d, want %d", afterDown, 3*w.Grid.RowHeight)
	}

	if !w.OnMouseButton(wheelUp(50, 50)) {
		t.Fatal("колесо вверх должно поглощаться, пока не у верхней границы")
	}
	if got := w.Grid.ScrollY(); got != 0 {
		t.Fatalf("после колеса вверх scrollY = %d, want 0", got)
	}
}

func TestWheel_DataGrid_ClampAtBounds(t *testing.T) {
	w := newScrollableDataGrid(30)

	if w.OnMouseButton(wheelUp(50, 50)) {
		t.Fatal("колесо вверх у верхней границы не должно поглощаться")
	}

	for i := 0; i < 50; i++ {
		w.OnMouseButton(wheelDown(50, 50))
	}
	maxS := w.Grid.ScrollY()
	if w.OnMouseButton(wheelDown(50, 50)) {
		t.Fatal("колесо вниз у нижней границы не должно поглощаться")
	}
	if got := w.Grid.ScrollY(); got != maxS {
		t.Fatalf("scrollY = %d, want %d (кламп снизу)", got, maxS)
	}
}

func TestWheel_DataGrid_ContentFits_NotConsumed(t *testing.T) {
	w := newScrollableDataGrid(2) // 2 строки помещаются

	if w.OnMouseButton(wheelDown(50, 50)) {
		t.Fatal("когда контент помещается, колесо НЕ должно поглощаться")
	}
}

func TestWheel_DataGrid_ThroughEngine(t *testing.T) {
	eng := newTestEngine()
	w := newScrollableDataGrid(30)
	eng.SetRoot(w)

	eng.SendMouseButton(50, 50, widget.MouseWheelDown, true)
	if got := w.Grid.ScrollY(); got <= 0 {
		t.Fatalf("сквозь движок: scrollY = %d, want > 0", got)
	}
}
