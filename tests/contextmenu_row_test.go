package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
	"github.com/oops1/headless-gui/v3/widget/treeview"
)

// Контекстное меню на строку и на узел — запрос GG-13 (и половина GG-11).
//
// SetContextMenu вешает на виджет ОДНО готовое меню. Действия над файлом,
// коммитом и веткой зависят от того, по какой строке щёлкнули, и приложение
// разбирало правую кнопку само, повторяя формулу «какая строка под этим Y».

type ctxRow struct{ Name string }

func ctxGridScene(t *testing.T) (*engine.Engine, *widget.DataGridWidget) {
	t.Helper()
	root := widget.NewPanel(widget.Theme{}.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))

	w := widget.NewDataGridWidget()
	w.Grid.RowHeight = 20
	w.Grid.HeaderHeight = 20
	col := datagrid.NewTextColumn("Имя", "Name")
	col.SetActualWidth(200)
	w.Grid.AddColumn(col)
	oc := datagrid.NewObservableCollection()
	for _, n := range []string{"a.go", "b.go", "c.go"} {
		oc.Add(&ctxRow{Name: n})
	}
	w.Grid.SetItemsSource(oc)
	w.SetBounds(image.Rect(0, 0, 300, 200))
	root.AddChild(w)

	eng := engine.New(400, 300, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	return eng, w
}

func rightClick(eng *engine.Engine, x, y int) {
	eng.SendMouseButton(x, y, widget.MouseRight, true)
	eng.SendMouseButton(x, y, widget.MouseRight, false)
}

// Колбэк получает ИМЕННО ту строку, по которой щёлкнули.
func TestRowContextMenu_GetsTheClickedRow(t *testing.T) {
	eng, w := ctxGridScene(t)

	var gotRow int = -99
	var gotName string
	w.RowContextMenu = func(item interface{}, row int) []widget.MenuItem {
		gotRow = row
		if r, ok := item.(*ctxRow); ok {
			gotName = r.Name
		}
		return []widget.MenuItem{{Text: "Открыть"}}
	}

	rightClick(eng, 40, 20+20+10) // вторая строка
	if gotRow != 1 || gotName != "b.go" {
		t.Errorf("колбэку досталась строка %d (%q), ждали 1 (b.go)", gotRow, gotName)
	}
}

// Щелчок мимо строк даёт row == -1 и nil-элемент: приложение решает само.
func TestRowContextMenu_MissIsReported(t *testing.T) {
	eng, w := ctxGridScene(t)

	gotRow := -99
	var gotItem interface{} = "не тронуто"
	w.RowContextMenu = func(item interface{}, row int) []widget.MenuItem {
		gotRow, gotItem = row, item
		return nil
	}

	rightClick(eng, 40, 5) // заголовок
	if gotRow != -1 || gotItem != nil {
		t.Errorf("щелчок по заголовку дал строку %d, элемент %v", gotRow, gotItem)
	}
}

// Пустой список означает «меню здесь нет»: пустая рамка на экране — это не
// «меню без пунктов», а ошибка, которую пользователь не исправит.
func TestRowContextMenu_EmptyMeansNoMenu(t *testing.T) {
	eng, w := ctxGridScene(t)
	w.RowContextMenu = func(interface{}, int) []widget.MenuItem { return nil }

	before := snapshotRGBA(eng.RenderOnce())
	rightClick(eng, 40, 30)
	eng.Invalidate()
	after := snapshotRGBA(eng.RenderOnce())

	for i := range before.Pix {
		if before.Pix[i] != after.Pix[i] {
			t.Fatal("пустой список пунктов всё-таки нарисовал меню")
		}
	}
}

// Выделение правым щелчком НЕ меняется: движок не вправе менять выбор
// пользователя за его спиной.
func TestRowContextMenu_DoesNotChangeSelection(t *testing.T) {
	eng, w := ctxGridScene(t)
	w.RowContextMenu = func(interface{}, int) []widget.MenuItem {
		return []widget.MenuItem{{Text: "Открыть"}}
	}

	w.Grid.SetSelectedIndex(0)
	rightClick(eng, 40, 20+2*20+10) // третья строка

	if got := w.Grid.SelectedItem(); got == nil || got.(*ctxRow).Name != "a.go" {
		t.Errorf("правый щелчок сменил выделение на %v", got)
	}
}

// Меню действительно показывается: на экране появляются пиксели поверх таблицы.
func TestRowContextMenu_IsShown(t *testing.T) {
	eng, w := ctxGridScene(t)
	w.RowContextMenu = func(item interface{}, row int) []widget.MenuItem {
		return []widget.MenuItem{{Text: "Открыть"}, {Separator: true}, {Text: "Удалить"}}
	}

	before := snapshotRGBA(eng.RenderOnce())
	rightClick(eng, 40, 30)
	eng.Invalidate()
	after := snapshotRGBA(eng.RenderOnce())

	diff := 0
	for i := range before.Pix {
		if before.Pix[i] != after.Pix[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Error("контекстное меню строки не появилось на экране")
	}
}

// Дерево: колбэк получает узел под курсором.
func TestNodeContextMenu_GetsTheClickedNode(t *testing.T) {
	root := widget.NewPanel(widget.Theme{}.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 200))

	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 200))
	names := []string{"main", "develop", "release"}
	for _, n := range names {
		tw.Tree.AddRoot(treeview.NewItem(n))
	}
	root.AddChild(tw)

	eng := engine.New(300, 200, 30)
	eng.SetRoot(root)
	eng.RenderOnce()

	var got string
	miss := false
	tw.NodeContextMenu = func(it *treeview.TreeViewItem) []widget.MenuItem {
		if it == nil {
			miss = true
			return nil
		}
		got = it.DisplayText()
		return []widget.MenuItem{{Text: "Переключиться"}}
	}

	ih := 22 // defaultItemHeight
	rightClick(eng, 40, ih+ih/2) // вторая строка
	if got != "develop" {
		t.Errorf("колбэку достался узел %q, ждали develop", got)
	}

	rightClick(eng, 40, ih*len(names)+ih/2) // ниже последней строки
	if !miss {
		t.Error("щелчок мимо узлов не дошёл до колбэка с nil")
	}
}

// Щелчок по пункту меню выполняет действие и не доходит до строки под меню.
func TestRowContextMenu_ClickRunsTheItem(t *testing.T) {
	eng, w := ctxGridScene(t)

	fired := ""
	w.RowContextMenu = func(item interface{}, row int) []widget.MenuItem {
		name := ""
		if r, ok := item.(*ctxRow); ok {
			name = r.Name
		}
		return []widget.MenuItem{
			{Text: "Открыть", OnClick: func() { fired = "открыть " + name }},
			{Text: "Удалить", OnClick: func() { fired = "удалить " + name }},
		}
	}

	w.Grid.SetSelectedIndex(0)
	rightClick(eng, 40, 20+20+10) // вторая строка
	eng.RenderOnce()

	// Первый пункт лежит сразу под точкой открытия.
	eng.SendMouseMove(50, 20+20+10+8)
	eng.SendMouseButton(50, 20+20+10+8, widget.MouseLeft, true)
	eng.SendMouseButton(50, 20+20+10+8, widget.MouseLeft, false)

	if fired != "открыть b.go" {
		t.Errorf("сработало %q, ждали «открыть b.go»", fired)
	}
	// Щелчок по пункту не должен был выделить строку под меню.
	if got := w.Grid.SelectedItem(); got == nil || got.(*ctxRow).Name != "a.go" {
		t.Errorf("щелчок по пункту меню сменил выделение на %v", got)
	}
}
