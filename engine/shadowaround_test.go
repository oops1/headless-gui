package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// Тень окна без середины под окном (widget.DrawShadowSpecAround): картинка та
// же, что у тени целиком, у любого окна — и у того, которому вырез не положен.

// shadowWindowScene — одно окно с тенью на полосатом фоне; setup настраивает
// окно до первого кадра.
func shadowWindowScene(t *testing.T, scale float64, setup func(w *widget.Window)) (*Engine, *widget.Window) {
	t.Helper()
	eng := New(480, 360, 30)
	t.Cleanup(eng.Stop)
	if scale != 1 {
		eng.SetScale(scale)
	}
	root := widget.NewPanel(color.RGBA{R: 30, G: 90, B: 160, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 480, 360))
	for i := 0; i < 12; i++ { // пёстрое под окном: стекло и тень от него зависят
		p := widget.NewPanel(color.RGBA{R: uint8(20 * i), G: uint8(255 - 20*i), B: 120, A: 255})
		p.ShowHeader = false
		p.SetBounds(image.Rect(i*40, 0, i*40+20, 360))
		root.AddChild(p)
	}
	w := widget.NewWindow("Окно", 300, 200)
	w.CornerRadius = 8
	w.SetBounds(image.Rect(80, 70, 380, 270))
	w.SetShadow(testShadow)
	if setup != nil {
		setup(w)
	}
	root.AddChild(w)
	eng.SetRoot(root)
	return eng, w
}

// Вырез (путь шаблона) против тени целиком (прямое размытие): тайловый растр
// и последовательный, масштабы, окно с непрозрачным фоном, со стеклом и с
// полупрозрачным фоном.
func TestShadowAround_WindowPictureUnchanged(t *testing.T) {
	testreset.Globals(t)
	cases := []struct {
		name  string
		setup func(w *widget.Window)
	}{
		{"непрозрачный фон", nil},
		{"стекло", func(w *widget.Window) {
			w.SetBackdrop(theme.BackdropSpec{Material: theme.MaterialAcrylic, Radius: 12,
				Tint: color.RGBA{R: 20, G: 20, B: 20, A: 60}})
		}},
		{"полупрозрачный фон", func(w *widget.Window) { w.Background = color.RGBA{R: 40, G: 40, B: 40, A: 120} }},
	}
	for _, tc := range cases {
		for _, scale := range []float64{1, 1.5} {
			for _, tiled := range []bool{true, false} {
				eng, _ := shadowWindowScene(t, scale, tc.setup)
				eng.SetTiledRaster(tiled)
				ring := eng.RenderOnce()
				eng.SetShadowCache(false) // прямой путь рисует тень целиком
				whole := eng.RenderOnce()
				if d, at := maxDiff(ring, whole); d > 1 {
					t.Errorf("%s, scale=%v, tiled=%v: расхождение %d/255 в %v", tc.name, scale, tiled, d, at)
				}
			}
		}
	}
}

// Окно тащат: частичные кадры с вырезом тени равны кадру, нарисованному
// заново, на каждом шаге.
func TestShadowAround_DragMatchesFreshFrame(t *testing.T) {
	testreset.Globals(t)
	eng, w := shadowWindowScene(t, 1, nil)
	eng.SetRenderOnDemand(true)
	eng.RenderOnce()
	for step := 1; step <= 6; step++ {
		d := image.Pt(7*step, 5*step)
		w.SetBounds(image.Rect(80, 70, 380, 270).Add(d))
		got := eng.RenderOnce()

		fresh, fw := shadowWindowScene(t, 1, nil)
		fw.SetBounds(image.Rect(80, 70, 380, 270).Add(d))
		want := fresh.RenderOnce()
		if diff, at := maxDiff(got, want); diff > 1 {
			t.Fatalf("шаг %d: кадр перетаскивания расходится с кадром заново: %d/255 в %v", step, diff, at)
		}
	}
}
