package dock

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
)

func TestDockPane_ContentStaysInside_AfterCanvasMove(t *testing.T) {
	useTestMeasurer(t)

	c := base.NewCanvas()
	pane := NewDockPane("pane1", "Панель", base.NewButton("Внутри"))
	c.AddChildAt(pane, base.CanvasAttached{Left: 10, Top: 10, Right: -1, Bottom: -1}, 300, 200)
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
