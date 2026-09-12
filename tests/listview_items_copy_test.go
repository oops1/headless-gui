package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-59: ListView.SetItems забирал срез вызывающего себе.
//
// Следствий было два, и оба ловятся здесь. Первое: элементы читаются из потока
// отрисовки, поэтому запись в переданный массив после вызова — гонка за
// строкой. Второе: страж изменений сравнивал массив сам с собой, и виджет,
// которому отдали тот же срез с поправленным элементом, не перерисовывался.

// TestListView_SetItemsCopiesSlice — правка исходного массива после вызова не
// меняет то, что показывает виджет.
func TestListView_SetItemsCopiesSlice(t *testing.T) {
	lv := widget.NewListView()
	lv.SetBounds(image.Rect(0, 0, 200, 100))

	lines := []string{"Запись объектов: 12 из 35"}
	lv.SetItems(lines)
	lines[0] = "Запись объектов: 13 из 35"

	if got := lv.Items()[0]; got != "Запись объектов: 12 из 35" {
		t.Errorf("виджет отдал %q — срез вызывающего не скопирован", got)
	}
}

// TestListView_SetItemsSameSliceInvalidates — тот же срез с изменённым
// элементом перерисовку запрашивает.
func TestListView_SetItemsSameSliceInvalidates(t *testing.T) {
	lv := widget.NewListView()
	lv.SetBounds(image.Rect(0, 0, 200, 100))

	lines := []string{"a"}
	lv.SetItems(lines)

	n := 0
	h := widget.RegisterUINotifier(nil, func(image.Rectangle) { n++ })
	defer widget.UnregisterUINotifier(h)

	lines[0] = "b"
	lv.SetItems(lines)

	if n == 0 {
		t.Error("перерисовка не запрошена: страж изменений сравнил массив сам с собой")
	}
	if got := lv.Items()[0]; got != "b" {
		t.Errorf("виджет показывает %q, а не новый текст", got)
	}
}

// TestListView_AddItemKeepsCallerSlice — AddItem дописывает в СВОЙ массив, а не
// в массив вызывающего (append в чужой срез с запасом писал бы в его память).
func TestListView_AddItemKeepsCallerSlice(t *testing.T) {
	lv := widget.NewListView()
	lv.SetBounds(image.Rect(0, 0, 200, 100))

	lines := make([]string, 1, 4) // запас: append писал бы в этот же массив
	lines[0] = "первая"
	lv.SetItems(lines)
	lv.AddItem("вторая")

	if got := lines[:cap(lines)][1]; got != "" {
		t.Errorf("AddItem дописал в массив вызывающего: %q", got)
	}
	if items := lv.Items(); len(items) != 2 || items[1] != "вторая" {
		t.Errorf("AddItem не добавил элемент в список виджета: %v", items)
	}
}
