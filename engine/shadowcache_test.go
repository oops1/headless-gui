package engine

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

var testShadow = theme.ShadowSpec{Blur: 16, OffsetY: 8, Color: color.RGBA{A: 96}}

// shadowScene — три окна с тенью разных размеров и скруглений, одно — меньше
// шаблона (запасной путь).
func shadowScene(t *testing.T, scale float64) *Engine {
	t.Helper()
	eng := New(640, 480, 30)
	t.Cleanup(eng.Stop)
	if scale != 1 {
		eng.SetScale(scale)
	}
	root := widget.NewPanel(color.RGBA{R: 210, G: 220, B: 230, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 640, 480))
	for i, sz := range []struct{ x, y, w, h, corner int }{
		{40, 40, 300, 200, 8},
		{360, 60, 220, 300, 0},
		{120, 300, 160, 120, 12},
		{500, 400, 30, 24, 4}, // меньше шаблона
	} {
		w := widget.NewWindow(fmt.Sprint("w", i), sz.w, sz.h)
		w.CornerRadius = sz.corner
		w.SetBounds(image.Rect(sz.x, sz.y, sz.x+sz.w, sz.y+sz.h))
		w.SetShadow(testShadow)
		root.AddChild(w)
	}
	eng.SetRoot(root)
	return eng
}

func maxDiff(a, b *image.RGBA) (int, image.Point) {
	worst, at := 0, image.Point{}
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			i := a.PixOffset(x, y)
			for k := 0; k < 4; k++ {
				d := int(a.Pix[i+k]) - int(b.Pix[i+k])
				if d < 0 {
					d = -d
				}
				if d > worst {
					worst, at = d, image.Pt(x, y)
				}
			}
		}
	}
	return worst, at
}

// Тень из шаблона совпадает с прямым размытием побайтно.
func TestShadowCache_MatchesDirectBlur(t *testing.T) {
	testreset.Globals(t)
	for _, scale := range []float64{1, 1.5} {
		eng := shadowScene(t, scale)
		eng.SetTiledRaster(false) // шаблоны считаются на холсте движка, а не в окнах воркеров
		cached := eng.RenderOnce()
		if eng.canvas.ShadowCacheLen() == 0 {
			t.Fatalf("scale %v: шаблонный путь не сработал — тест ничего не сравнил", scale)
		}
		eng.SetShadowCache(false)
		direct := eng.RenderOnce()
		if d, at := maxDiff(cached, direct); d > 1 {
			t.Fatalf("scale %v: расхождение %d/255 в %v", scale, d, at)
		}
		if eng.ShadowCache() {
			t.Fatal("ShadowCache после SetShadowCache(false)")
		}
	}
}

// Частичный кадр через тень даёт то же, что полный: шва на краю
// перерисованной области нет.
func TestShadowCache_PartialFrameHasNoSeam(t *testing.T) {
	testreset.Globals(t)
	eng := shadowScene(t, 1)
	eng.SetRenderOnDemand(true)
	eng.RenderOnce()
	// Область режет нижнюю тень первого окна поперёк.
	eng.InvalidateRect(image.Rect(100, 230, 180, 250))
	partial := eng.RenderOnce()

	fresh := shadowScene(t, 1)
	full := fresh.RenderOnce()
	if d, at := maxDiff(partial, full); d > 1 {
		t.Fatalf("частичный кадр расходится с полным: %d/255 в %v", d, at)
	}
}

// Окно с тенью тащат по рабочему столу: кадр с тенью из шаблона и с прямым
// размытием.
func benchWindowDragShadow(b *testing.B, cache bool) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	eng := sc.eng
	eng.SetRenderOnDemand(true)
	eng.SetShadowCache(cache)
	win := widget.NewWindow("Окно", 720, 440)
	win.CornerRadius = 8
	win.SetShadow(testShadow)
	win.SetBounds(image.Rect(120, 80, 840, 520))
	eng.Root().(*widget.Panel).AddChild(win)
	eng.renderFrame()
	posA, posB := image.Pt(120, 80), image.Pt(360, 260)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos := posA
		if i%2 == 1 {
			pos = posB
		}
		win.SetBounds(image.Rectangle{Min: pos, Max: pos.Add(image.Pt(720, 440))})
		eng.renderFrame()
	}
}

func BenchmarkPipeline_WindowDragShadow(b *testing.B)       { benchWindowDragShadow(b, true) }
func BenchmarkPipeline_WindowDragShadowDirect(b *testing.B) { benchWindowDragShadow(b, false) }
