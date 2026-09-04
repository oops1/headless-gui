package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// Подсказка на строку таблицы — запрос GG-27.
//
// Base.ToolTip один на весь виджет, а строке нужен свой: состояние файла,
// причина конфликта, полный путь. Обёртке приходилось пересчитывать индекс
// строки на каждое движение мыши и звать SetToolTip.

type tipRow struct{ Name, State string }

func TestDataGridWidget_RowToolTip(t *testing.T) {
	w := widget.NewDataGridWidget()
	w.Grid.RowHeight = 20
	w.Grid.HeaderHeight = 20
	col := datagrid.NewTextColumn("Имя", "Name")
	col.SetActualWidth(120)
	w.Grid.AddColumn(col)

	oc := datagrid.NewObservableCollection()
	oc.Add(&tipRow{Name: "a.go", State: "изменён"})
	oc.Add(&tipRow{Name: "b.go", State: "конфликт"})
	w.Grid.SetItemsSource(oc)
	w.SetBounds(image.Rect(0, 0, 200, 100))

	w.ToolTip = "таблица"
	w.Grid.RowToolTip = func(item interface{}, row int) string {
		if r, ok := item.(*tipRow); ok {
			return r.State
		}
		return ""
	}

	// Курсор над заголовком — строки нет, показывается общая подсказка.
	w.OnMouseMove(10, 5)
	if got := w.GetToolTip(); got != "таблица" {
		t.Errorf("над заголовком подсказка %q, ждали общую", got)
	}

	w.OnMouseMove(10, 20+10) // первая строка
	if got := w.GetToolTip(); got != "изменён" {
		t.Errorf("над первой строкой подсказка %q", got)
	}

	w.OnMouseMove(10, 20+20+10) // вторая строка
	if got := w.GetToolTip(); got != "конфликт" {
		t.Errorf("над второй строкой подсказка %q", got)
	}

	// Пустой ответ приложения означает «своей подсказки нет» — остаётся общая.
	w.Grid.RowToolTip = func(interface{}, int) string { return "" }
	if got := w.GetToolTip(); got != "таблица" {
		t.Errorf("при пустом ответе подсказка %q, ждали общую", got)
	}
}
