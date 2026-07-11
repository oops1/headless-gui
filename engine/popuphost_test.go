package engine

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// buildDropdownEngine создаёт движок с открытым Dropdown в заданных bounds.
func buildDropdownEngine(t *testing.T) (*Engine, *widget.Dropdown, widget.Widget) {
	t.Helper()
	e := New(300, 300, 20)
	e.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 300))
	dd := widget.NewDropdown("Alpha", "Beta", "Gamma")
	dd.SetBounds(image.Rect(40, 40, 200, 70))
	root.AddChild(dd)
	e.SetRoot(root)
	return e, dd, root
}

// TestTranslatingContext_PixelMatch: оверлей, отрендеренный в отдельный
// popup-канвас через транслирующий DrawContext, попиксельно совпадает с
// соответствующей областью прежней in-canvas отрисовки (оверлей внутри холста).
func TestTranslatingContext_PixelMatch(t *testing.T) {
	e, dd, root := buildDropdownEngine(t)
	dd.SetOpen(true)

	r := dd.OverlayBounds()
	if r.Empty() {
		t.Fatal("OverlayBounds пуст при открытом Dropdown")
	}
	if r.Max.X > 300 || r.Max.Y > 300 || r.Min.X < 0 || r.Min.Y < 0 {
		t.Fatalf("оверлей выходит за холст, тест некорректен: %v", r)
	}

	c := e.canvas

	// In-canvas отрисовка оверлея поверх очищенного фона.
	c.blitBackground()
	drawOverlays(root, c, false)

	// Offscreen отрисовка через транслирующий контекст.
	oc := c.renderOverlay(dd, r)

	// Сравнение области r основного холста с popup-канвасом (scale==1).
	if oc.Bounds().Dx() != r.Dx() || oc.Bounds().Dy() != r.Dy() {
		t.Fatalf("размер popup-канваса %v != OverlayBounds %v", oc.Bounds(), r)
	}
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			in := c.back.RGBAAt(r.Min.X+x, r.Min.Y+y)
			out := oc.RGBAAt(x, y)
			if in != out {
				t.Fatalf("пиксель (%d,%d): in-canvas=%v popup-canvas=%v", x, y, in, out)
			}
		}
	}
}

// TestPopupSink_OpenClose: sink получает корректный Rect при открытии Dropdown,
// не вызывается при отсутствии изменений и получает пустой slice при закрытии.
func TestPopupSink_OpenClose(t *testing.T) {
	e, dd, _ := buildDropdownEngine(t)

	var mu sync.Mutex
	var last []PopupFrame
	calls := 0
	e.SetPopupSink(func(f []PopupFrame) {
		mu.Lock()
		last = f
		calls++
		mu.Unlock()
	})

	// Открытие.
	dd.SetOpen(true)
	e.renderFrame()

	mu.Lock()
	if len(last) != 1 {
		mu.Unlock()
		t.Fatalf("ожидался 1 popup-кадр при открытии, got %d", len(last))
	}
	want := dd.OverlayBounds()
	if last[0].Rect != want {
		t.Fatalf("Rect кадра %v != OverlayBounds %v", last[0].Rect, want)
	}
	if last[0].Img == nil {
		t.Fatal("Img кадра nil")
	}
	if last[0].ID == 0 {
		t.Fatal("ID кадра нулевой")
	}
	openCalls := calls
	mu.Unlock()

	// Повторный рендер без изменений — sink не должен вызываться.
	e.renderFrame()
	mu.Lock()
	if calls != openCalls {
		mu.Unlock()
		t.Fatalf("sink вызван без изменений: calls %d → %d", openCalls, calls)
	}
	mu.Unlock()

	// Закрытие — sink с пустым slice.
	dd.SetOpen(false)
	e.renderFrame()
	mu.Lock()
	if len(last) != 0 {
		mu.Unlock()
		t.Fatalf("ожидался пустой slice при закрытии, got %d", len(last))
	}
	mu.Unlock()
}

// TestPopupSink_NotDrawnInCanvas: при активном sink оверлей НЕ рисуется в
// основной холст (там, где раньше был список, содержимое не меняется).
func TestPopupSink_NotDrawnInCanvas(t *testing.T) {
	e, dd, root := buildDropdownEngine(t)
	e.SetPopupSink(func(f []PopupFrame) {})

	c := e.canvas
	r := func() image.Rectangle { dd.SetOpen(true); return dd.OverlayBounds() }()

	// Рисуем дерево + оверлеи в hosted-режиме: список НЕ должен попасть в холст.
	c.blitBackground()
	root.Draw(c)
	drawOverlays(root, c, true)
	hostedPix := c.back.RGBAAt(r.Min.X+2, r.Min.Y+2)

	// Тот же кадр без оверлея вовсе (dropdown закрыт).
	dd.SetOpen(false)
	c.blitBackground()
	root.Draw(c)
	drawOverlays(root, c, true)
	closedPix := c.back.RGBAAt(r.Min.X+2, r.Min.Y+2)

	if hostedPix != closedPix {
		t.Fatalf("в hosted-режиме оверлей просочился в холст: %v vs %v", hostedPix, closedPix)
	}
}
