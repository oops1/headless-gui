package widget

import (
	"image"
	"testing"
)

// ToolBar и DockPane раскладывают своё содержимое сами (SetBounds → layout),
// поэтому родитель не должен сдвигать их потомков ещё раз: сдвиг
// задваивался, и кнопки уезжали из панели на дельту переезда контейнера —
// в витрине они вылезали поверх соседнего заголовка.

func TestToolBar_ChildrenStayInside_AfterCanvasMove(t *testing.T) {
	useTestMeasurer(t)

	c := NewCanvas()
	tb := NewToolBar()
	tb.AddChild(NewButton("Раз"))
	tb.AddChild(NewButton("Два"))
	c.AddChildAt(tb, CanvasAttached{Left: 10, Top: 10, Right: -1, Bottom: -1}, 300, 40)
	c.SetBounds(image.Rect(0, 0, 500, 400))

	// Контейнер переезжает — так бывает при смене вкладки или доке панели.
	c.SetBounds(image.Rect(200, 100, 700, 500))

	for i, child := range tb.Children() {
		if !child.Bounds().In(tb.Bounds()) {
			t.Errorf("элемент %d панели %v вылез за её границы %v",
				i, child.Bounds(), tb.Bounds())
		}
	}
}

func TestDockPane_ContentStaysInside_AfterCanvasMove(t *testing.T) {
	useTestMeasurer(t)

	c := NewCanvas()
	pane := NewDockPane("pane1", "Панель", NewButton("Внутри"))
	c.AddChildAt(pane, CanvasAttached{Left: 10, Top: 10, Right: -1, Bottom: -1}, 300, 200)
	c.SetBounds(image.Rect(0, 0, 500, 400))
	c.SetBounds(image.Rect(200, 100, 700, 500))

	content := pane.Content()
	if content == nil {
		t.Fatal("у панели нет содержимого")
	}
	if !content.Bounds().In(pane.Bounds()) {
		t.Errorf("содержимое %v вылезло за границы панели %v",
			content.Bounds(), pane.Bounds())
	}
}
