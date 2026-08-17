package engine

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// cyclicWidget — виджет, чей Children() возвращает самого себя.
// Draw специально не рекурсивен: проверяем обходы движка, а не виджета.
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

// Обходы движка не должны зацикливаться на дереве с циклом в Children().
func TestTreeCycle_DoesNotHang(t *testing.T) {
	w := &cyclicWidget{b: image.Rect(0, 0, 200, 200), overlay: true}

	e := New(200, 200, 20)
	e.SetTooltipsEnabled(false)
	e.SetRoot(w)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if p := hitTestPath(w, 5, 5); len(p) > maxTreeDepth {
			t.Errorf("путь hit-test длиннее предела: %d", len(p))
		}
		hitTest(w, 5, 5)
		findOverlayAt(w, 5, 5)
		findCapturer(w, 5, 5, widget.MouseEvent{})
		dismissOutside(w, nil)
		broadcastMouseMove(w, 0, 0, 5, 5)
		injectCaptureManager(w, e)
		drawOverlays(w, e.canvas, false)
		e.renderFrame()
		e.SendMouseMove(5, 5)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("обход дерева с циклом завис")
	}
}

// Хостируемые попапы в циклическом дереве собираются не глубже предела.
func TestTreeCycle_CollectPopupsBounded(t *testing.T) {
	w := &cyclicWidget{b: image.Rect(0, 0, 200, 200), overlay: true}
	items := collectPopups(nil, w, 0)
	if len(items) != maxTreeDepth {
		t.Fatalf("собрано %d оверлеев, ожидалось %d", len(items), maxTreeDepth)
	}
}
