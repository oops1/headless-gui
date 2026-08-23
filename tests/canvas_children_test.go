// Тесты Canvas: синхронность childInfos при удалении детей
// и смена желаемого размера (ENGINE_ISSUES из WinLine).
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestCanvas_RemoveChild_KeepsChildInfosAligned — удаление среднего
// ребёнка не должно смещать attached-свойства остальных.
func TestCanvas_RemoveChild_KeepsChildInfosAligned(t *testing.T) {
	c := widget.NewCanvas()
	c.SetBounds(image.Rect(0, 0, 400, 300))

	b1 := widget.NewButton("1")
	b2 := widget.NewButton("2")
	b3 := widget.NewButton("3")
	c.AddChildAt(b1, widget.CanvasAttached{Left: 10, Top: 10, Right: -1, Bottom: -1}, 50, 20)
	c.AddChildAt(b2, widget.CanvasAttached{Left: 100, Top: 100, Right: -1, Bottom: -1}, 60, 25)
	c.AddChildAt(b3, widget.CanvasAttached{Left: 200, Top: 200, Right: -1, Bottom: -1}, 70, 30)

	if !c.RemoveChild(b2) {
		t.Fatal("RemoveChild(b2) вернул false")
	}
	if got := len(c.Children()); got != 2 {
		t.Fatalf("детей после удаления: %d, ждали 2", got)
	}

	// Полный layout: третий ребёнок должен остаться на СВОИХ координатах.
	c.SetBounds(image.Rect(0, 0, 401, 300))

	want := image.Rect(200, 200, 270, 230)
	if b3.Bounds() != want {
		t.Errorf("b3.Bounds() = %v, ждали %v (childInfos рассинхронизирован)", b3.Bounds(), want)
	}
	if props := c.GetChildCanvasProps(1); props.Left != 200 || props.Top != 200 {
		t.Errorf("props[1] = %+v, ждали Left=200 Top=200", props)
	}
}

// TestCanvas_RemoveChild_Missing — удаление чужого виджета ничего не ломает.
func TestCanvas_RemoveChild_Missing(t *testing.T) {
	c := widget.NewCanvas()
	c.SetBounds(image.Rect(0, 0, 100, 100))
	b1 := widget.NewButton("1")
	c.AddChildAt(b1, widget.CanvasAttached{Left: 5, Top: 5, Right: -1, Bottom: -1}, 40, 20)

	if c.RemoveChild(widget.NewButton("чужой")) {
		t.Fatal("RemoveChild чужого виджета вернул true")
	}
	if props := c.GetChildCanvasProps(0); props.Left != 5 {
		t.Errorf("props[0] пострадали: %+v", props)
	}
}

// TestCanvas_ClearChildren_ResetsInfos — после очистки добавление
// начинается с чистого childInfos.
func TestCanvas_ClearChildren_ResetsInfos(t *testing.T) {
	c := widget.NewCanvas()
	c.SetBounds(image.Rect(0, 0, 400, 300))
	c.AddChildAt(widget.NewButton("1"), widget.CanvasAttached{Left: 10, Top: 10, Right: -1, Bottom: -1}, 50, 20)
	c.AddChildAt(widget.NewButton("2"), widget.CanvasAttached{Left: 100, Top: 100, Right: -1, Bottom: -1}, 60, 25)

	c.ClearChildren()
	if got := len(c.Children()); got != 0 {
		t.Fatalf("детей после ClearChildren: %d", got)
	}

	b := widget.NewButton("новый")
	c.AddChildAt(b, widget.CanvasAttached{Left: 30, Top: 40, Right: -1, Bottom: -1}, 80, 35)
	want := image.Rect(30, 40, 110, 75)
	if b.Bounds() != want {
		t.Errorf("b.Bounds() = %v, ждали %v", b.Bounds(), want)
	}
	if props := c.GetChildCanvasProps(0); props.Left != 30 || props.Top != 40 {
		t.Errorf("props[0] = %+v, ждали Left=30 Top=40", props)
	}
}

// TestCanvas_SetChildDesiredSize — смена желаемого размера без
// удаления/повторного добавления ребёнка.
func TestCanvas_SetChildDesiredSize(t *testing.T) {
	c := widget.NewCanvas()
	c.SetBounds(image.Rect(0, 0, 400, 300))
	b := widget.NewButton("1")
	c.AddChildAt(b, widget.CanvasAttached{Left: 10, Top: 20, Right: -1, Bottom: -1}, 50, 25)

	c.SetChildDesiredSize(0, 120, 60)
	want := image.Rect(10, 20, 130, 80)
	if b.Bounds() != want {
		t.Errorf("после SetChildDesiredSize: %v, ждали %v", b.Bounds(), want)
	}

	// Размер переживает полный layout-проход.
	c.SetBounds(image.Rect(0, 0, 500, 300))
	if b.Bounds() != want {
		t.Errorf("после resize Canvas: %v, ждали %v", b.Bounds(), want)
	}

	// Некорректный индекс — no-op.
	c.SetChildDesiredSize(5, 1, 1)
	c.SetChildDesiredSize(-1, 1, 1)
	if b.Bounds() != want {
		t.Errorf("no-op индексы изменили bounds: %v", b.Bounds())
	}
}
