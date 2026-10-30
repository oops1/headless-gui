package desktop

import (
	"image/color"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testengine"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// Щелчок по плитке «Пуска» через движок запускает приложение, а не переносит
// плитку (замечание WinLine на v3.37.0). Меню — открытый оверлей, и под ним
// движок рассылал каждое движение в два приёма: всему дереву «курсора нет»,
// включая само меню, затем меню — настоящую точку. «Курсора нет» меню
// принимало за точку в 2^30 от нажатия и начинало перенос, так что рука,
// дрогнувшая на месте, переставляла плитку вместо запуска.
func TestTiles_ClickThroughEngineWithJitterLaunches(t *testing.T) {
	testreset.Globals(t)
	m, cat := tiledMenu(t)
	root := widget.NewPanel(color.RGBA{R: 0, G: 60, B: 120, A: 255})
	root.ShowHeader = false
	root.SetBounds(tileScreen())
	root.AddChild(m)
	eng := testengine.New(t, tileScreenW, tileScreenH, 30)
	eng.SetRoot(root)

	before := m.TileGroups()
	x, y := pointIn(m.tileRectAbs("t1"))
	eng.SendMouseMove(x, y)
	eng.SendMouseButton(x, y, widget.MouseLeft, true)
	eng.SendMouseMove(x, y)   // движение на месте
	eng.SendMouseMove(x+1, y) // и дрожание на пиксель
	eng.SendMouseButton(x+1, y, widget.MouseLeft, false)

	if len(cat.Launched) != 1 || cat.Launched[0] != "t1" {
		t.Errorf("запущено %v, ждали t1", cat.Launched)
	}
	if after := m.TileGroups(); !reflect.DeepEqual(before, after) {
		t.Errorf("плитки переставлены щелчком:\nбыло  %v\nстало %v", before, after)
	}
}

// Сам «Пуск» тоже не верит «курсора нет»: даже если такое движение придёт
// между нажатием и отпусканием, перенос не начинается.
func TestTiles_NowhereMoveDoesNotStartDrag(t *testing.T) {
	testreset.Globals(t)
	m, cat := tiledMenu(t)
	before := m.TileGroups()
	x, y := pointIn(m.tileRectAbs("t1"))
	m.OnMouseMove(x, y)
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	m.OnMouseMove(widget.CursorNowhere, widget.CursorNowhere)
	m.OnMouseMove(x, y)
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
	if len(cat.Launched) != 1 || cat.Launched[0] != "t1" {
		t.Errorf("запущено %v, ждали t1", cat.Launched)
	}
	if after := m.TileGroups(); !reflect.DeepEqual(before, after) {
		t.Errorf("плитки переставлены: %v", after)
	}
}
