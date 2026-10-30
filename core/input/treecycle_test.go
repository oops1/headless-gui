package input

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// cyclicWidget — виджет, чей Children() возвращает самого себя.
// Draw специально не рекурсивен: проверяем обходы ввода, а не виджета.
type cyclicWidget struct {
	b        image.Rectangle
	overlay  bool
	dismissN int
}

func (c *cyclicWidget) Draw(widget.DrawContext)        {}
func (c *cyclicWidget) Bounds() image.Rectangle        { return c.b }
func (c *cyclicWidget) SetBounds(r image.Rectangle)    { c.b = r }
func (c *cyclicWidget) Children() []widget.Widget      { return []widget.Widget{c} }
func (c *cyclicWidget) AddChild(widget.Widget)         {}
func (c *cyclicWidget) HasOverlay() bool               { return c.overlay }
func (c *cyclicWidget) DrawOverlay(widget.DrawContext) {}
func (c *cyclicWidget) OverlayBounds() image.Rectangle { return image.Rect(10, 10, 60, 60) }
func (c *cyclicWidget) Dismiss()                       { c.dismissN++ }
func (c *cyclicWidget) OnMouseMove(x, y int)           {}

// Обходы ввода не должны зацикливаться на дереве с циклом в Children().
func TestTreeCycle_InputWalksDoNotHang(t *testing.T) {
	testreset.Globals(t)
	w := &cyclicWidget{b: image.Rect(0, 0, 200, 200), overlay: true}
	d := New(&stubHost{root: w})

	done := make(chan struct{})
	go func() {
		defer close(done)
		if p := hitTestPath(w, 5, 5); len(p) > maxTreeDepth {
			t.Errorf("путь hit-test длиннее предела: %d", len(p))
		}
		hitTest(w, 5, 5)
		findOverlayAt(w, 5, 5)
		findCapturer(w, 5, 5, widget.MouseEvent{})
		DismissOutside(w, nil, 5, 5)
		broadcastMouseMove(w, 0, 0, 5, 5)
		d.SendMouseMove(5, 5)
		d.SendMouseButton(5, 5, widget.MouseLeft, true)
		d.SendMouseButton(5, 5, widget.MouseLeft, false)
		if widgetInTree(w, &cyclicWidget{}) {
			t.Error("чужой виджет найден в циклическом дереве")
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("обход дерева с циклом завис")
	}
}
