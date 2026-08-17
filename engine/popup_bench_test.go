package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// benchPopupEngine — движок с открытым меню и активным popup-хостом.
func benchPopupEngine(b *testing.B) (*Engine, image.Rectangle) {
	b.Helper()
	e := New(400, 400, 20)
	e.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 400))
	dd := widget.NewDropdown("Alpha", "Beta", "Gamma", "Delta", "Epsilon")
	dd.SetBounds(image.Rect(40, 40, 220, 70))
	root.AddChild(dd)
	e.SetRoot(root)
	e.SetPopupSink(func([]PopupFrame) {})
	dd.SetOpen(true)
	e.renderFrame() // первый кадр — попап отдан хосту
	return e, dd.OverlayBounds()
}

// BenchmarkPopupFrame_OutsideDamage: кадры с инвалидацией ВНЕ меню.
func BenchmarkPopupFrame_OutsideDamage(b *testing.B) {
	e, r := benchPopupEngine(b)
	far := image.Rect(300, 340, 340, 360)
	if far.Overlaps(r) {
		b.Fatalf("тестовая область %v пересекает меню %v", far, r)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.InvalidateRect(far)
		e.renderFrame()
	}
}
