package datagrid

import (
	"image"
	"testing"
)

// Таблица сообщала о строке, но не о ячейке: приложение, нарисовавшее в
// ячейке ссылку (номер коммита, путь), не могло поймать щелчок именно по ней
// — разбор координат был целиком приватным.

type ccRow struct {
	Name string
	Hash string
}

// ccGrid — таблица из двух колонок с заданными ширинами и строками.
func ccGrid(rows int) *DataGrid {
	dg := New()
	dg.SetBounds(image.Rect(0, 0, 400, 300))
	dg.HeaderHeight = 24
	dg.RowHeight = 20

	name := NewTextColumn("Имя", "Name")
	name.SetActualWidth(120)
	hash := NewTextColumn("Коммит", "Hash")
	hash.SetActualWidth(100)
	dg.AddColumn(name)
	dg.AddColumn(hash)

	oc := NewObservableCollection()
	for i := 0; i < rows; i++ {
		oc.Add(&ccRow{Name: "строка", Hash: "abc123"})
	}
	dg.SetItemsSource(oc)
	return dg
}

// cellPoint — точка внутри ячейки (row, col).
func cellPoint(dg *DataGrid, row, col, dx, dy int) (int, int) {
	r := dg.CellRect(row, col)
	return r.Min.X + dx, r.Min.Y + dy
}

func TestDataGrid_CellAt(t *testing.T) {
	dg := ccGrid(5)

	x, y := cellPoint(dg, 2, 1, 5, 5)
	row, col, ok := dg.CellAt(x, y)
	if !ok {
		t.Fatalf("точка (%d,%d) внутри ячейки, а CellAt говорит «мимо»", x, y)
	}
	if row != 2 || col != 1 {
		t.Errorf("CellAt = строка %d, колонка %d; ждал 2 и 1", row, col)
	}

	// Заголовок — не данные.
	if _, _, ok := dg.CellAt(10, 5); ok {
		t.Error("щелчок по заголовку выдан за ячейку")
	}
	// Пустое место под последней строкой — тоже.
	if _, _, ok := dg.CellAt(10, 290); ok {
		t.Error("пустое место под строками выдано за ячейку")
	}
	// Правее последней колонки колонок нет.
	if _, _, ok := dg.CellAt(390, 40); ok {
		t.Error("точка правее колонок выдана за ячейку")
	}
}

func TestDataGrid_CellRect(t *testing.T) {
	dg := ccGrid(5)

	r0 := dg.CellRect(0, 0)
	if r0.Dx() != 120 || r0.Dy() != 20 {
		t.Errorf("ячейка %v, ждал 120×20", r0)
	}
	// Вторая колонка стоит вплотную за первой, вторая строка — под первой.
	if got := dg.CellRect(0, 1); got.Min.X != r0.Max.X {
		t.Errorf("вторая колонка начинается на %d, ждал %d", got.Min.X, r0.Max.X)
	}
	if got := dg.CellRect(1, 0); got.Min.Y != r0.Max.Y {
		t.Errorf("вторая строка начинается на %d, ждал %d", got.Min.Y, r0.Max.Y)
	}
	// Несуществующая ячейка — пустой прямоугольник, а не паника.
	if got := dg.CellRect(99, 0); !got.Empty() {
		t.Errorf("несуществующая строка дала %v", got)
	}
	if got := dg.CellRect(0, 9); !got.Empty() {
		t.Errorf("несуществующая колонка дала %v", got)
	}
}

func TestDataGrid_OnCellClicked(t *testing.T) {
	dg := ccGrid(5)
	var got []CellClickedEvent
	dg.OnCellClicked = func(e CellClickedEvent) { got = append(got, e) }

	x, y := cellPoint(dg, 3, 1, 7, 6)
	dg.OnMouseButton(x, y, 0, true)

	if len(got) != 1 {
		t.Fatalf("событий %d, ждал одно", len(got))
	}
	e := got[0]
	if e.RowIndex != 3 || e.ColumnIndex != 1 {
		t.Errorf("щёлкнули по строке %d, колонке %d; ждал 3 и 1", e.RowIndex, e.ColumnIndex)
	}
	if e.Item == nil {
		t.Error("элемент строки не передан")
	}
	if e.Column == nil || e.Column.Header() != "Коммит" {
		t.Errorf("колонка события: %v", e.Column)
	}
	// Координаты — внутри ячейки, от её угла.
	if e.X != 7 || e.Y != 6 {
		t.Errorf("координаты внутри ячейки (%d,%d), ждал (7,6)", e.X, e.Y)
	}
	if e.DoubleClick {
		t.Error("одиночный щелчок помечен двойным")
	}
}

func TestDataGrid_OnCellClicked_DoubleClick(t *testing.T) {
	dg := ccGrid(5)
	var got []CellClickedEvent
	dg.OnCellClicked = func(e CellClickedEvent) { got = append(got, e) }

	x, y := cellPoint(dg, 1, 0, 3, 3)
	dg.OnMouseDoubleClick(x, y)

	if len(got) != 1 {
		t.Fatalf("событий %d, ждал одно", len(got))
	}
	if !got[0].DoubleClick {
		t.Error("двойной щелчок не помечен")
	}
	if got[0].RowIndex != 1 || got[0].ColumnIndex != 0 {
		t.Errorf("щёлкнули по строке %d, колонке %d; ждал 1 и 0",
			got[0].RowIndex, got[0].ColumnIndex)
	}
}

// Щелчок мимо данных события не порождает: заголовок сортирует, пустое место
// ничего не значит.
func TestDataGrid_OnCellClicked_OutsideData(t *testing.T) {
	dg := ccGrid(2)
	n := 0
	dg.OnCellClicked = func(e CellClickedEvent) { n++ }

	dg.OnMouseButton(10, 5, 0, true)   // заголовок
	dg.OnMouseButton(10, 290, 0, true) // пусто под строками
	if n != 0 {
		t.Errorf("событий %d, ждал ни одного", n)
	}
}

// Колбэк зовётся ВНЕ замка: обработчик почти наверняка обратится к таблице.
func TestDataGrid_OnCellClicked_CallbackMayUseGrid(t *testing.T) {
	dg := ccGrid(3)
	var item interface{}
	dg.OnCellClicked = func(e CellClickedEvent) {
		item = dg.ItemAtRow(e.RowIndex) // взял бы замок повторно — взаимоблокировка
	}
	x, y := cellPoint(dg, 2, 0, 4, 4)
	dg.OnMouseButton(x, y, 0, true)

	if item == nil {
		t.Error("обработчик не смог получить элемент строки")
	}
}

func TestDataGrid_ColumnCursor(t *testing.T) {
	dg := ccGrid(3)
	link := NewTextColumn("Ссылка", "Hash")
	link.SetActualWidth(80)
	link.SetCursor(CursorHand)
	dg.AddColumn(link)

	// Над обычной колонкой — стрелка.
	x, y := cellPoint(dg, 1, 0, 5, 5)
	if got := dg.CursorAt(x, y); got != CursorArrow {
		t.Errorf("над обычной колонкой курсор %d, ждал стрелку", got)
	}
	// Над колонкой-ссылкой — рука.
	x, y = cellPoint(dg, 1, 2, 5, 5)
	if got := dg.CursorAt(x, y); got != CursorHand {
		t.Errorf("над колонкой со ссылкой курсор %d, ждал руку (%d)", got, CursorHand)
	}
	// Над пустым местом — стрелка, а не рука последней колонки.
	if got := dg.CursorAt(10, 290); got != CursorArrow {
		t.Errorf("над пустым местом курсор %d, ждал стрелку", got)
	}
}
