package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// moveRecorder — виджет, запоминающий все движения; overlay — открыт ли у
// него оверлей поверх всего.
type moveRecorder struct {
	widget.Base
	overlay bool
	moves   []image.Point
}

func (r *moveRecorder) OnMouseMove(x, y int)           { r.moves = append(r.moves, image.Pt(x, y)) }
func (r *moveRecorder) HasOverlay() bool               { return r.overlay }
func (r *moveRecorder) DrawOverlay(widget.DrawContext) {}
func (r *moveRecorder) OverlayBounds() image.Rectangle { return r.Bounds() }
func (r *moveRecorder) Draw(widget.DrawContext)        {}

// Под открытым оверлеем остальное дерево получает «курсора нет» (снять
// наведение), а сам владелец оверлея — только настоящую точку. Раньше и ему
// сперва приходило «курсора нет», и меню «Пуск» принимало это за скачок мыши
// и начинало перенос плитки (замечание WinLine на v3.37.0).
func TestOverlayMove_OwnerGetsOnlyTheRealPoint(t *testing.T) {
	testreset.Globals(t)
	e := New(400, 300, 30)
	defer e.Stop()
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))
	under := &moveRecorder{}
	under.SetBounds(image.Rect(0, 0, 400, 300))
	menu := &moveRecorder{overlay: true}
	menu.SetBounds(image.Rect(50, 50, 250, 250))
	root.AddChild(under)
	root.AddChild(menu)
	e.SetRoot(root)

	e.SendMouseMove(100, 100)
	e.SendMouseMove(101, 100)
	for _, p := range menu.moves {
		if widget.CursorIsNowhere(p.X, p.Y) {
			t.Fatalf("владелец оверлея получил «курсора нет»: %v", menu.moves)
		}
	}
	if len(menu.moves) == 0 || menu.moves[len(menu.moves)-1] != image.Pt(101, 100) {
		t.Fatalf("владелец оверлея не получил настоящую точку: %v", menu.moves)
	}
	if len(under.moves) == 0 || !widget.CursorIsNowhere(under.moves[len(under.moves)-1].X, under.moves[len(under.moves)-1].Y) {
		t.Fatalf("виджет под оверлеем не получил «курсора нет»: %v", under.moves)
	}
}
