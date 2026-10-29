package engine

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/core/input"
	"github.com/oops1/headless-gui/v3/internal/testreset"
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
	testreset.Globals(t)
	w := &cyclicWidget{b: image.Rect(0, 0, 200, 200), overlay: true}

	e := New(200, 200, 20)
	// Без Stop приёмник переносов движка с циклическим деревом оставался
	// подписан на весь процесс, и перетаскивание окна в следующем тесте
	// обходило это дерево.
	defer e.Stop()
	e.SetTooltipsEnabled(false)
	e.SetRoot(w)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Обходы ввода (hit-тест, захватчик, оверлей, рассылка движения) —
		// core/input/treecycle_test.go.
		input.DismissOutside(w, nil, 5, 5)
		injectCaptureManager(w, e)
		drawOverlays(w, e.canvas, false)
		e.renderFrame()
		e.SendMouseMove(5, 5)
		// Объявление о переносе чужого виджета: поиск его в дереве.
		if widgetInTree(w, &cyclicWidget{}) {
			t.Error("чужой виджет найден в циклическом дереве")
		}
		e.noteMove(widget.MoveNotice{Widget: &cyclicWidget{}, Rect: image.Rect(0, 0, 10, 10)})
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("обход дерева с циклом завис")
	}
}

// Хостируемые попапы в циклическом дереве собираются не глубже предела.
func TestTreeCycle_CollectPopupsBounded(t *testing.T) {
	testreset.Globals(t)
	w := &cyclicWidget{b: image.Rect(0, 0, 200, 200), overlay: true}
	items := collectPopups(nil, w, 0)
	if len(items) != maxTreeDepth {
		t.Fatalf("собрано %d оверлеев, ожидалось %d", len(items), maxTreeDepth)
	}
}
