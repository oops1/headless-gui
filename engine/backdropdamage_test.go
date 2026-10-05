package engine

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// glassBar — полоса со стеклом (как панель задач Windows 11) и квадратиком,
// цвет которого меняется — как подсветка кнопки под мышью.
type glassBar struct {
	widget.Base
	lit bool
}

func (g *glassBar) Draw(ctx widget.DrawContext) {
	b := g.Bounds()
	if bd, ok := ctx.(widget.BackdropDrawer); ok {
		bd.BlurBehind(b, 12, color.RGBA{20, 20, 30, 80})
	}
	c := color.RGBA{200, 200, 200, 255}
	if g.lit {
		c = color.RGBA{255, 120, 0, 255}
	}
	ctx.FillRect(b.Min.X+100, b.Min.Y+10, 20, 20, c)
}

func stripes(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 7), uint8(y * 5), uint8((x + y) * 3), 255})
		}
	}
	return img
}

// Частичная перерисовка внутри размытой полосы даёт те же пиксели, что полный
// кадр. Раньше размытие считалось по куску поверх прошлого кадра — пятно
// светлее остальной полосы там, где сменилась подсветка.
func TestBackdrop_PartialRedrawMatchesFull(t *testing.T) {
	render := func(lit bool, partial bool) []byte {
		e := New(400, 120, 30)
		e.SetRenderOnDemand(true)
		if err := e.SetBackground(stripes(400, 120)); err != nil {
			t.Fatal(err)
		}
		bar := &glassBar{}
		bar.SetBounds(image.Rect(0, 60, 400, 120))
		root := widget.NewCanvas()
		root.SetBounds(image.Rect(0, 0, 400, 120))
		root.AddChild(bar)
		e.SetRoot(root)
		e.RenderOnce()
		if partial {
			bar.lit = lit
			e.InvalidateRect(image.Rect(80, 62, 140, 100))
		} else {
			bar.lit = lit
			e.Invalidate()
		}
		return bytes.Clone(e.RenderOnce().Pix)
	}
	full := render(true, false)
	part := render(true, true)
	if !bytes.Equal(full, part) {
		diff := 0
		for i := range full {
			if full[i] != part[i] {
				diff++
			}
		}
		t.Fatalf("частичная перерисовка расходится с полной: %d байт", diff)
	}
}
