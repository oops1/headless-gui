package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// focusRingButtonFrame рисует одну кнопку 120×36 в позиции (20,20) и отдаёт
// кадр. accent — акцентная кнопка (OK диалога), ring — общая рамка фокуса.
func focusRingButtonFrame(t *testing.T, accent, ring, focused bool) *image.RGBA {
	t.Helper()
	eng := New(200, 100, 20)
	t.Cleanup(eng.Stop)
	root := widget.NewPanel(color.RGBA{R: 40, G: 40, B: 40, A: 255})
	root.SetBounds(image.Rect(0, 0, 200, 100))
	var btn *widget.Button
	if accent {
		btn = widget.NewWin10AccentButton("OK")
	} else {
		btn = widget.NewButton("OK")
	}
	btn.FocusRing = ring
	btn.SetBounds(image.Rect(20, 20, 140, 56))
	root.AddChild(btn)
	eng.SetRoot(root)
	if focused {
		eng.SetFocus(btn)
	}
	return eng.RenderOnce()
}

// На акцентной кнопке в фокусе прежняя рамка цвета акцента сливалась с
// заливкой — фокус не был виден. Общая рамка видна: у края кнопки появляются
// пиксели, которых нет без фокуса и которые отличаются от заливки.
func TestFocusRing_VisibleOnAccentButton(t *testing.T) {
	testreset.Globals(t)
	idle := focusRingButtonFrame(t, true, true, false)
	ring := focusRingButtonFrame(t, true, true, true)

	// Кнопка 20..140 × 20..56; контур фокуса — на втором-третьем пикселях от
	// края (наружный в две точки, внутренний в одну), заливка — внутри.
	probe := []image.Point{{60, 21}, {60, 22}, {60, 54}, {60, 53}, {21, 38}, {22, 38}, {138, 38}, {139, 38}}
	diff := func(a, b *image.RGBA) int {
		n := 0
		for _, p := range probe {
			if a.RGBAAt(p.X, p.Y) != b.RGBAAt(p.X, p.Y) {
				n++
			}
		}
		return n
	}
	if n := diff(ring, idle); n < len(probe)/2 {
		t.Errorf("общая рамка заметна лишь в %d из %d проверенных точек", n, len(probe))
	}
	idlePlain := focusRingButtonFrame(t, true, false, false)
	// Без фокуса рамка не рисуется: кадр кнопки с FocusRing в покое совпадает с
	// кадром обычной кнопки в покое.
	if n := diff(idle, idlePlain); n != 0 {
		t.Errorf("кнопка с FocusRing в покое отличается от обычной в %d точках", n)
	}
	// Рамка двухцветная: на левом краю наружный контур и внутренний различаются.
	if ring.RGBAAt(21, 38) == ring.RGBAAt(22, 38) && ring.RGBAAt(22, 38) == ring.RGBAAt(23, 38) {
		t.Error("контур фокуса однотонный: ожидались наружный и контрастный внутренний")
	}
}
