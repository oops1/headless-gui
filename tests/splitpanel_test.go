package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// makeSplit собирает SplitPanel с двумя цветными панелями-детьми.
func makeSplit(orient widget.Orientation, r image.Rectangle) (*widget.SplitPanel, *widget.Panel, *widget.Panel) {
	sp := widget.NewSplitPanel(orient)
	a := widget.NewPanel(tcPanel)
	b := widget.NewPanel(tcPanel)
	sp.AddChild(a)
	sp.AddChild(b)
	sp.SetBounds(r)
	return sp, a, b
}

// ─── Раскладка по доле ──────────────────────────────────────────────────────

func TestSplitPanel_LayoutByFraction(t *testing.T) {
	// Horizontal: панели слева/справа, вертикальная полоса.
	sp, a, b := makeSplit(widget.OrientationHorizontal, image.Rect(0, 0, 200, 100))
	sp.SetPosition(0.5)

	// avail = 200 - 6 = 194; fw = round(0.5*194) = 97.
	if got := a.Bounds(); got != image.Rect(0, 0, 97, 100) {
		t.Errorf("First bounds = %v, want (0,0,97,100)", got)
	}
	if got := b.Bounds(); got != image.Rect(103, 0, 200, 100) {
		t.Errorf("Second bounds = %v, want (103,0,200,100)", got)
	}

	// Vertical: панели сверху/снизу, горизонтальная полоса.
	spv, av, bv := makeSplit(widget.OrientationVertical, image.Rect(0, 0, 100, 200))
	spv.SetPosition(0.5)
	if got := av.Bounds(); got != image.Rect(0, 0, 100, 97) {
		t.Errorf("First(V) bounds = %v, want (0,0,100,97)", got)
	}
	if got := bv.Bounds(); got != image.Rect(0, 103, 100, 200) {
		t.Errorf("Second(V) bounds = %v, want (0,103,100,200)", got)
	}
}

// ─── Перетаскивание мышью через движок ──────────────────────────────────────

func TestSplitPanel_DragMovesBoundary(t *testing.T) {
	sp, a, b := makeSplit(widget.OrientationHorizontal, image.Rect(0, 0, 200, 100))
	sp.SetPosition(0.5)

	eng := engine.New(200, 100, 20)
	eng.SetRoot(sp)

	// Полоса при fw=97, ss=6 → центр x=100, y=50.
	eng.SendMouseButton(100, 50, widget.MouseLeft, true)
	eng.SendMouseMove(140, 50)
	eng.SendMouseButton(140, 50, widget.MouseLeft, false)

	// fw = 140 - 0 - 3(half) = 137.
	if got := a.Bounds().Dx(); got != 137 {
		t.Errorf("First width after drag = %d, want 137", got)
	}
	if got := b.Bounds(); got != image.Rect(143, 0, 200, 100) {
		t.Errorf("Second bounds after drag = %v, want (143,0,200,100)", got)
	}
	// Доля должна обновиться: 137/194 ≈ 0.706.
	if p := sp.Position; p < 0.70 || p > 0.71 {
		t.Errorf("Position after drag = %.4f, want ≈0.706", p)
	}
}

// ─── Клэмп минимальных размеров ─────────────────────────────────────────────

func TestSplitPanel_MinClampOnDrag(t *testing.T) {
	sp, a, _ := makeSplit(widget.OrientationHorizontal, image.Rect(0, 0, 200, 100))
	sp.MinFirst = 50
	sp.MinSecond = 50
	sp.SetPosition(0.5)

	eng := engine.New(200, 100, 20)
	eng.SetRoot(sp)

	// Тянем полосу далеко влево (за минимум First).
	eng.SendMouseButton(100, 50, widget.MouseLeft, true)
	eng.SendMouseMove(5, 50)
	eng.SendMouseButton(5, 50, widget.MouseLeft, false)

	if got := a.Bounds().Dx(); got != 50 {
		t.Errorf("First width clamped = %d, want 50 (MinFirst)", got)
	}

	// Тянем далеко вправо (за минимум Second): avail=194, fw<=194-50=144.
	eng.SendMouseButton(sp.Bounds().Min.X+50+3, 50, widget.MouseLeft, true)
	eng.SendMouseMove(300, 50)
	eng.SendMouseButton(300, 50, widget.MouseLeft, false)
	if got := a.Bounds().Dx(); got != 144 {
		t.Errorf("First width clamped = %d, want 144 (avail-MinSecond)", got)
	}
}

// ─── Двойной клик: коллапс / восстановление (уровень виджета) ────────────────

func TestSplitPanel_DoubleClickCollapseRestore(t *testing.T) {
	sp, a, b := makeSplit(widget.OrientationHorizontal, image.Rect(0, 0, 200, 100))
	sp.SetPosition(0.5)
	wantFirst := a.Bounds().Dx() // 97

	dbl := func() {
		e := widget.MouseEvent{X: 100, Y: 50, Button: widget.MouseLeft, Pressed: true}
		sp.OnMouseButton(e)
		e.Pressed = false
		sp.OnMouseButton(e)
		e.Pressed = true
		sp.OnMouseButton(e)
		e.Pressed = false
		sp.OnMouseButton(e)
	}

	// Первый двойной клик — коллапс First.
	dbl()
	if !sp.IsCollapsed() {
		t.Fatal("после двойного клика First должен быть свёрнут")
	}
	if got := a.Bounds().Dx(); got != 0 {
		t.Errorf("First width collapsed = %d, want 0", got)
	}
	if got := b.Bounds(); got != image.Rect(6, 0, 200, 100) {
		t.Errorf("Second при коллапсе = %v, want (6,0,200,100)", got)
	}

	// Второй двойной клик — восстановление прежней позиции.
	dbl()
	if sp.IsCollapsed() {
		t.Fatal("после повторного двойного клика First должен восстановиться")
	}
	if got := a.Bounds().Dx(); got != wantFirst {
		t.Errorf("First width restored = %d, want %d", got, wantFirst)
	}
}

// TestSplitPanel_DoubleClickViaEngine проверяет коллапс сквозь диспетчер
// движка (WantsCapture → capture → OnMouseButton) end-to-end.
func TestSplitPanel_DoubleClickViaEngine(t *testing.T) {
	sp, a, _ := makeSplit(widget.OrientationHorizontal, image.Rect(0, 0, 200, 100))
	sp.SetPosition(0.5)

	eng := engine.New(200, 100, 20)
	eng.SetRoot(sp)

	// Двойной клик по полосе (центр x=100).
	eng.SendMouseButton(100, 50, widget.MouseLeft, true)
	eng.SendMouseButton(100, 50, widget.MouseLeft, false)
	eng.SendMouseButton(100, 50, widget.MouseLeft, true)
	eng.SendMouseButton(100, 50, widget.MouseLeft, false)

	if !sp.IsCollapsed() || a.Bounds().Dx() != 0 {
		t.Fatalf("движок не свернул First: collapsed=%v w=%d", sp.IsCollapsed(), a.Bounds().Dx())
	}

	// Полоса теперь при x∈[0,6]; двойной клик по ней восстанавливает.
	eng.SendMouseButton(3, 50, widget.MouseLeft, true)
	eng.SendMouseButton(3, 50, widget.MouseLeft, false)
	eng.SendMouseButton(3, 50, widget.MouseLeft, true)
	eng.SendMouseButton(3, 50, widget.MouseLeft, false)

	if sp.IsCollapsed() || a.Bounds().Dx() != 97 {
		t.Fatalf("движок не восстановил First: collapsed=%v w=%d", sp.IsCollapsed(), a.Bounds().Dx())
	}
}

// ─── Вложенный сплит (SplitPanel в SplitPanel) ──────────────────────────────

func TestSplitPanel_Nested(t *testing.T) {
	outer := widget.NewSplitPanel(widget.OrientationHorizontal)
	inner := widget.NewSplitPanel(widget.OrientationVertical)
	inner.AddChild(widget.NewPanel(tcPanel)) // top
	inner.AddChild(widget.NewPanel(tcPanel)) // bottom
	right := widget.NewPanel(tcPanel)

	outer.AddChild(inner) // First
	outer.AddChild(right) // Second
	outer.SetPosition(0.5)
	outer.SetBounds(image.Rect(0, 0, 400, 200))

	// outer: avail=394, fw=197 → inner region = (0,0,197,200).
	if got := inner.Bounds(); got != image.Rect(0, 0, 197, 200) {
		t.Fatalf("inner bounds = %v, want (0,0,197,200)", got)
	}
	// inner (Vertical): avail=200-6=194, fw=97 → верх=(0,0,197,97), низ=(0,103,197,200).
	top := inner.First().Bounds()
	bottom := inner.Second().Bounds()
	if top != image.Rect(0, 0, 197, 97) {
		t.Errorf("inner top = %v, want (0,0,197,97)", top)
	}
	if bottom != image.Rect(0, 103, 197, 200) {
		t.Errorf("inner bottom = %v, want (0,103,197,200)", bottom)
	}
}

// ─── Ресайз контейнера сохраняет долю ───────────────────────────────────────

func TestSplitPanel_ResizePreservesFraction(t *testing.T) {
	sp, a, _ := makeSplit(widget.OrientationHorizontal, image.Rect(0, 0, 200, 100))
	sp.SetPosition(0.25)

	// avail=194 → fw=round(0.25*194)=round(48.5)=49.
	if got := a.Bounds().Dx(); got != 49 {
		t.Fatalf("First width @200 = %d, want 49", got)
	}

	// Ресайз контейнера вдвое — доля сохраняется.
	sp.SetBounds(image.Rect(0, 0, 400, 100))
	// avail=394 → fw=round(0.25*394)=round(98.5)=99.
	if got := a.Bounds().Dx(); got != 99 {
		t.Errorf("First width @400 = %d, want 99 (доля 0.25 сохранена)", got)
	}
	if p := sp.Position; p != 0.25 {
		t.Errorf("Position после ресайза = %v, want 0.25", p)
	}
}
