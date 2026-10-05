package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// renderSVGIconAt рисует SVG-иконку в bounds на холсте с масштабом k и
// возвращает физический кадр.
func renderSVGIconAt(t *testing.T, data string, k float64, bounds image.Rectangle) *image.RGBA {
	t.Helper()
	eng := engine.New(40, 40, 30)
	eng.SetScale(k)

	root := widget.NewPanel(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 40, 40))

	ic := widget.NewSVGIcon()
	if err := ic.SetSVG([]byte(data)); err != nil {
		t.Fatal(err)
	}
	ic.SetColor(color.RGBA{A: 255})
	ic.SetBounds(bounds)
	root.AddChild(ic)

	eng.SetRoot(root)
	return eng.RenderOnce()
}

// SVG-значок на HiDPI растеризуется в физическом размере: край фигуры, лежащий
// на целой физической границе, остаётся резким. Раньше значок растеризовался в
// логическом размере и растягивался движком на масштаб — край превращался в
// размытую ступеньку из промежуточных оттенков.
func TestSVGIcon_HiDPIRasterizesInPhysicalSize(t *testing.T) {
	// Левая половина закрашена: в иконке 20×20 край на логической x=10, на
	// масштабе 1.5 (30×30 физических пикселей) — на физической x=15.
	const data = `<svg viewBox="0 0 24 24"><rect x="0" y="0" width="12" height="24" fill="#000"/></svg>`
	img := renderSVGIconAt(t, data, 1.5, image.Rect(10, 10, 30, 30))

	// Иконка занимает физические x∈[15,45), y∈[15,45). Край — на x=30.
	y := 30
	for x := 15; x < 45; x++ {
		c := img.RGBAAt(x, y)
		wantDark := x < 30
		if wantDark && c.R > 8 {
			t.Fatalf("x=%d: ожидался чёрный, получено %v", x, c)
		}
		if !wantDark && c.R < 247 {
			t.Fatalf("x=%d: ожидался белый, получено %v — край размыт (значок растянут, а не растеризован в физическом размере)", x, c)
		}
	}
}

// На масштабе 1 путь тот же, что был: значок рисуется картинкой логического
// размера, результат — резкий край.
func TestSVGIcon_Scale1Unchanged(t *testing.T) {
	const data = `<svg viewBox="0 0 24 24"><rect x="0" y="0" width="12" height="24" fill="#000"/></svg>`
	img := renderSVGIconAt(t, data, 1, image.Rect(10, 10, 30, 30))
	for x := 10; x < 30; x++ {
		c := img.RGBAAt(x, 20)
		if (x < 20) != (c.R < 128) {
			t.Fatalf("x=%d: %v", x, c)
		}
	}
}
