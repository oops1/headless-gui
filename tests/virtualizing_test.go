package tests

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func makeItems(n int) []interface{} {
	out := make([]interface{}, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf("Item %d", i)
	}
	return out
}

func newVIC() (*widget.VirtualizingItemsControl, *int) {
	built := 0
	v := widget.NewVirtualizingItemsControl()
	v.ItemHeight = 28
	v.Buffer = 2
	cnt := &built
	v.SetItemBuilder(func(item interface{}, index int) widget.Widget {
		built++
		return widget.NewLabel(item.(string), color.RGBA{255, 255, 255, 255})
	})
	return v, cnt
}

func TestVirtualizing_OnlyVisibleMaterialized(t *testing.T) {
	v, _ := newVIC()
	v.SetItems(makeItems(10000))
	// Высота 280 → 10 видимых строк + буфер 2*2 = максимум ~14.
	v.SetBounds(image.Rect(0, 0, 200, 280))

	n := len(v.Children())
	if n == 0 {
		t.Fatal("no children materialized")
	}
	if n > 16 {
		t.Fatalf("too many widgets materialized: %d (virtualization broken)", n)
	}
	if v.ItemCount() != 10000 {
		t.Fatalf("ItemCount = %d, want 10000", v.ItemCount())
	}
}

func TestVirtualizing_ScrollChangesWindow(t *testing.T) {
	v, _ := newVIC()
	v.SetItems(makeItems(10000))
	v.SetBounds(image.Rect(0, 0, 200, 280))

	first := v.Children()[0].(*widget.Label).Text()
	if first != "Item 0" {
		t.Fatalf("top item = %q, want Item 0", first)
	}

	// Прокручиваем на 100 строк вниз.
	v.ScrollBy(100 * 28)
	topY := 100 * 28
	if v.ScrollY() != topY {
		t.Fatalf("scrollY = %d, want %d", v.ScrollY(), topY)
	}
	// Верхний видимый элемент должен быть около индекса 100 (минус буфер).
	top := v.Children()[0].(*widget.Label).Text()
	if top != "Item 98" {
		t.Fatalf("after scroll top = %q, want Item 98 (100 - buffer 2)", top)
	}
}

func TestVirtualizing_ScrollClampsAtEnd(t *testing.T) {
	v, _ := newVIC()
	v.SetItems(makeItems(50))
	v.SetBounds(image.Rect(0, 0, 200, 280))
	v.ScrollBy(1 << 20) // далеко за конец
	maxS := 50*28 - 280
	if v.ScrollY() != maxS {
		t.Fatalf("scrollY = %d, want clamp %d", v.ScrollY(), maxS)
	}
}

func TestVirtualizing_BindCollectionView(t *testing.T) {
	oc := sampleOC() // из collectionview_test.go (4 person)
	cv := widget.NewCollectionView(oc)
	v := widget.NewVirtualizingItemsControl()
	v.ItemHeight = 28
	v.SetItemBuilder(func(item interface{}, index int) widget.Widget {
		return widget.NewLabel(item.(*person).Name, color.RGBA{255, 255, 255, 255})
	})
	v.SetBounds(image.Rect(0, 0, 200, 280))
	v.BindCollectionView(cv)
	if v.ItemCount() != 4 {
		t.Fatalf("initial count = %d, want 4", v.ItemCount())
	}
	// Фильтр → view-changed → авто-обновление контейнера.
	cv.SetFilter(func(it interface{}) bool { return it.(*person).Age >= 18 })
	if v.ItemCount() != 2 {
		t.Fatalf("after filter count = %d, want 2", v.ItemCount())
	}
}

// TestVirtualizing_RowsNotDoubleShiftedInsideCanvas воспроизводит баг «первый
// показ рисует строки со сдвигом»: Canvas.layoutChild после child.SetBounds
// доводил потомков контейнеров БЕЗ собственной раскладки через
// shiftDescendants. VirtualizingItemsControl свою раскладку имеет
// (SetBounds → updateVisible позиционирует строки заново), но не был
// зарегистрирован в HasOwnLayout — и материализованные строки получали
// смещение Canvas ВТОРОЙ раз.
//
// Сцена повторяет вкладку «3.2.5» витрины: контент-канва вкладки смещена
// вправо на ширину сайдбара (220px) и вниз под шапку, список лежит внутри неё.
func TestVirtualizing_RowsNotDoubleShiftedInsideCanvas(t *testing.T) {
	const (
		itemH   = 26
		listW   = 470
		listH   = 300
		leftOff = 556 // Canvas.Left списка внутри канвы вкладки
		topOff  = 112 // Canvas.Top  списка внутри канвы вкладки
		canvasX = 220 // ширина сайдбара — смещение канвы вкладки
		canvasY = 80
	)

	v := widget.NewVirtualizingItemsControl()
	v.ItemHeight = itemH
	v.Buffer = 0
	// Строка шаблона — контейнер (как StackPanel в DataTemplate витрины):
	// важно проверить и её собственные bounds, и координаты её детей.
	v.SetItemBuilder(func(item interface{}, index int) widget.Widget {
		row := widget.NewStackPanel(widget.OrientationHorizontal)
		row.AddChild(widget.NewLabel(item.(string), color.RGBA{255, 255, 255, 255}))
		return row
	})
	v.SetItems(makeItems(1000))

	cv := widget.NewCanvas()
	cv.AddChildAt(v, widget.CanvasAttached{Left: leftOff, Top: topOff, Right: -1, Bottom: -1},
		listW, listH)
	// Первая раскладка канвы — тот самый «первый показ вкладки».
	cv.SetBounds(image.Rect(canvasX, canvasY, canvasX+1060, canvasY+770))

	wantList := image.Rect(canvasX+leftOff, canvasY+topOff,
		canvasX+leftOff+listW, canvasY+topOff+listH)
	if got := v.Bounds(); got != wantList {
		t.Fatalf("bounds списка = %v, want %v", got, wantList)
	}

	rows := v.Children()
	if len(rows) == 0 {
		t.Fatal("строки не материализованы")
	}
	for i, row := range rows {
		rb := row.Bounds()
		if rb.Min.X != wantList.Min.X {
			t.Fatalf("строка %d: Min.X = %d, want %d (сдвиг родителя применён дважды)",
				i, rb.Min.X, wantList.Min.X)
		}
		wantY := wantList.Min.Y + i*itemH
		if rb.Min.Y != wantY {
			t.Fatalf("строка %d: Min.Y = %d, want %d", i, rb.Min.Y, wantY)
		}
		for j, ch := range row.Children() {
			cb := ch.Bounds()
			if cb.Min.X < wantList.Min.X || cb.Min.X >= wantList.Max.X {
				t.Fatalf("строка %d, потомок %d: Min.X = %d вне списка %v",
					i, j, cb.Min.X, wantList)
			}
		}
	}

	// Повторная раскладка (скролл/hover в витрине «чинили» картинку) не должна
	// ничего менять — позиции уже верные и стабильны.
	before := make([]image.Rectangle, len(rows))
	for i, row := range rows {
		before[i] = row.Bounds()
	}
	cv.SetBounds(image.Rect(canvasX, canvasY, canvasX+1060, canvasY+770))
	for i, row := range v.Children() {
		if i < len(before) && row.Bounds() != before[i] {
			t.Fatalf("строка %d уехала при повторной раскладке: %v → %v",
				i, before[i], row.Bounds())
		}
	}
}
