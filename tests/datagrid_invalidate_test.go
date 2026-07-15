package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

type dgRow struct {
	Name string
}

// buildGrid создаёт DataGridWidget с n строками, полностью помещающимися в
// bounds (без скроллбара), чтобы выбор строки не вызывал прокрутку.
func buildGrid(t *testing.T, n int) (*widget.DataGridWidget, int, int) {
	t.Helper()
	dg := widget.NewDataGridWidget()
	oc := datagrid.NewObservableCollection()
	for i := 0; i < n; i++ {
		oc.Add(dgRow{Name: "row"})
	}
	dg.Grid.SetItemsSource(oc)
	dg.Grid.AddColumn(datagrid.NewTextColumn("Name", "Name"))

	rowH := dg.Grid.RowHeight       // 28
	headerH := dg.Grid.HeaderHeight // 30
	h := headerH + rowH*n           // всё видно, без скроллбара
	dg.SetBounds(image.Rect(0, 0, 300, h))
	return dg, rowH, headerH
}

// clickRow эмулирует нажатие ЛКМ в центре строки row (в области данных).
func clickRow(dg *widget.DataGridWidget, row, rowH, headerH int) {
	y := headerH + row*rowH + rowH/2
	dg.OnMouseButton(widget.MouseEvent{X: 50, Y: y, Button: widget.MouseLeft, Pressed: true})
}

// TestDataGridInvalidate_SelectionRowLevel проверяет, что смена выделения
// строки инвалидирует НЕ БОЛЬШЕ высоты двух строк (прежде и ныне выделенной),
// а не весь виджет — даже когда строки далеко друг от друга.
func TestDataGridInvalidate_SelectionRowLevel(t *testing.T) {
	dg, rowH, headerH := buildGrid(t, 20)

	// Устанавливаем начальное выделение (строка 2) ДО подмены нотификатора.
	clickRow(dg, 2, rowH, headerH)

	var rects []image.Rectangle
	widget.SetUIRectChangeNotifier(func(r image.Rectangle) {
		rects = append(rects, r)
	})
	defer widget.SetUIRectChangeNotifier(nil)

	// Выбираем далёкую строку 15: damage = только строки 2 и 15.
	clickRow(dg, 15, rowH, headerH)

	if len(rects) == 0 {
		t.Fatal("выбор строки не вызвал ни одной инвалидации")
	}

	maxDy, totalArea := 0, 0
	for _, r := range rects {
		if r.Dy() > maxDy {
			maxDy = r.Dy()
		}
		totalArea += r.Dx() * r.Dy()
	}

	// Ни один прямоугольник не выше двух строк (не span 2..15 = 14 строк).
	if maxDy > 2*rowH {
		t.Errorf("самый высокий damage-прямоугольник %dpx > 2 строк (%dpx): инвалидация грубая", maxDy, 2*rowH)
	}
	// Суммарная площадь — не больше двух полос строк (+ небольшой запас).
	w := dg.Bounds().Dx()
	if limit := 2*rowH*w + w; totalArea > limit {
		t.Errorf("суммарная площадь damage %d > лимита %d (≈2 строки)", totalArea, limit)
	}
}

// TestDataGridInvalidate_HoverRowLevel проверяет, что смена hover-строки
// инвалидирует не больше двух строк (прежней и новой), а не весь виджет.
func TestDataGridInvalidate_HoverRowLevel(t *testing.T) {
	dg, rowH, headerH := buildGrid(t, 20)

	// Наводим на строку 3, затем сбрасываем накопитель.
	dg.OnMouseMove(50, headerH+3*rowH+rowH/2)

	var rects []image.Rectangle
	widget.SetUIRectChangeNotifier(func(r image.Rectangle) {
		rects = append(rects, r)
	})
	defer widget.SetUIRectChangeNotifier(nil)

	// Наводим на далёкую строку 12.
	dg.OnMouseMove(50, headerH+12*rowH+rowH/2)

	if len(rects) == 0 {
		t.Fatal("смена hover не вызвала инвалидации")
	}
	maxDy := 0
	for _, r := range rects {
		if r.Dy() > maxDy {
			maxDy = r.Dy()
		}
	}
	if maxDy > 2*rowH {
		t.Errorf("hover: damage-прямоугольник %dpx > 2 строк (%dpx)", maxDy, 2*rowH)
	}
}
