package engine

// tiled_review_test.go — случаи из ревью 4а.2: прямой виджет под прокруткой,
// смена окружения холста при тёплом кэше, слой над слоем с правкой вне
// области чтения верхнего, первый кадр слоя частичным повреждением,
// картинка в группе прозрачности под слоем, два одинаковых слоя.

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

func TestTiled_DirectWidgetInScrollView(t *testing.T) {
	testreset.Globals(t)
	p := tiledPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, _ := busyScene(640, 480)
		sv := widget.NewScrollView()
		sv.SetBounds(image.Rect(40, 40, 600, 440))
		sv.ContentHeight = 1200
		es := widget.NewExternalSurface()
		es.SetBounds(image.Rect(60, 200, 360, 400))
		buf := image.NewRGBA(image.Rect(0, 0, 300, 200))
		for i := range buf.Pix {
			buf.Pix[i] = uint8(i*5 | 0x40)
		}
		es.Swap(buf, true, nil)
		sv.AddChild(es)
		root.AddChild(sv)
		sv.SetScrollY(120)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "поверхность в прокрутке")
}

func TestTiled_CacheResetsOnEnvironmentChange(t *testing.T) {
	testreset.Globals(t)
	wall := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for i := range wall.Pix {
		wall.Pix[i] = uint8(i*11 | 0x20)
	}
	for i := 3; i < len(wall.Pix); i += 4 {
		wall.Pix[i] = 255
	}
	steps := []struct {
		name string
		act  func(eng *Engine)
	}{
		{"чужой back BGRX после тёплого кэша", func(eng *Engine) {
			bufs := [][]byte{make([]byte, 640*480*4)}
			if err := eng.SetSurfaces(bufs, 640*4, FormatBGRX); err != nil {
				t.Fatal(err)
			}
		}},
		{"масштаб 1,25", func(eng *Engine) { eng.SetScale(1.25) }},
		{"новый фон под слоем", func(eng *Engine) {
			if err := eng.SetBackground(wall); err != nil {
				t.Fatal(err)
			}
		}},
		{"DPI 120", func(eng *Engine) { eng.SetDPI(120) }},
		{"без фона", func(eng *Engine) { eng.ClearBackground() }},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			p := tiledPair(func() (*Engine, widget.Widget) { return blurScene(1) })
			defer p.closeAll()
			cmdCompare(t, p, "полный кадр")
			cmdCompare(t, p, "попадание")
			st.act(p.on)
			st.act(p.off)
			cmdCompare(t, p, "после смены")
			cmdCompare(t, p, "ещё кадр")
		})
	}
}

func TestTiled_LayerOverLayerChangeOutsideUpperSrc(t *testing.T) {
	testreset.Globals(t)
	var boxes []*widget.Panel
	p := tiledPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, _ := busyScene(640, 480)
		box := widget.NewPanel(color.RGBA{R: 250, G: 30, B: 30, A: 255})
		box.ShowHeader = false
		box.SetBounds(image.Rect(240, 200, 282, 216)) // под нижним слоем, вне области чтения верхнего
		root.AddChild(box)
		boxes = append(boxes, box)
		root.AddChild(newGlass(image.Rect(100, 100, 500, 400), 24, color.RGBA{R: 200, G: 200, B: 220, A: 90}, "нижний"))
		root.AddChild(newGlass(image.Rect(300, 150, 480, 380), 10, color.RGBA{R: 40, G: 40, B: 60, A: 80}, "верхний"))
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "полный кадр")
	cmdCompare(t, p, "попадание")
	for _, b := range boxes {
		b.Background = color.RGBA{R: 30, G: 30, B: 250, A: 255}
		b.Invalidate()
	}
	cmdCompare(t, p, "смена цвета под нижним слоем")
	cmdCompare(t, p, "следующий кадр")
	p.on.Invalidate()
	p.off.Invalidate()
	cmdCompare(t, p, "полный кадр после смены")
}

// appearingGlass — слой, который появляется по флагу и заявляет только часть
// своей области: первый кадр слоя — частичный, повреждение уже слоя.
type appearingGlass struct {
	glass
	on bool
}

func (g *appearingGlass) Draw(ctx widget.DrawContext) {
	if g.on {
		g.glass.Draw(ctx)
	}
}

func TestTiled_PartialFirstFrameOfLayerIsNotCached(t *testing.T) {
	testreset.Globals(t)
	var gs []*appearingGlass
	p := tiledPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, _ := busyScene(640, 480)
		g := &appearingGlass{glass: *newGlass(image.Rect(10, 10, 390, 290), 14, color.RGBA{R: 220, G: 220, B: 240, A: 100}, "появился")}
		g.SetBounds(image.Rect(10, 10, 390, 290))
		root.AddChild(g)
		gs = append(gs, g)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "без слоя")
	for _, g := range gs {
		g.on = true
		g.MarkDirty()
		g.InvalidateArea(image.Rect(60, 60, 140, 140)) // повреждение меньше слоя
	}
	cmdCompare(t, p, "первый частичный кадр слоя")
	p.on.Invalidate()
	p.off.Invalidate()
	cmdCompare(t, p, "полный кадр — растр частичного не должен попасть")
	cmdCompare(t, p, "попадание")
}

// groupedImage — картинка в группе прозрачности.
type groupedImage struct {
	widget.Base
	img *image.RGBA
}

func (g *groupedImage) Draw(ctx widget.DrawContext) {
	b := g.Bounds()
	if od, ok := ctx.(widget.OpacityDrawer); ok {
		od.DrawWithOpacity(b, 0.8, func() { ctx.DrawImage(g.img, b.Min.X, b.Min.Y) })
		return
	}
	ctx.DrawImage(g.img, b.Min.X, b.Min.Y)
}

func TestTiled_ImageInOpacityGroupUnderGlass(t *testing.T) {
	testreset.Globals(t)
	var imgs []*groupedImage
	p := tiledPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, _ := busyScene(640, 480)
		img := image.NewRGBA(image.Rect(0, 0, 200, 120))
		for i := range img.Pix {
			img.Pix[i] = uint8(i*3 | 0x80)
		}
		gi := &groupedImage{img: img}
		gi.SetBounds(image.Rect(150, 150, 350, 270))
		root.AddChild(gi)
		imgs = append(imgs, gi)
		root.AddChild(newGlass(image.Rect(100, 100, 500, 400), 16, color.RGBA{R: 220, G: 220, B: 240, A: 100}, "над группой"))
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "полный кадр")
	cmdCompare(t, p, "попадание")
	for i, gi := range imgs {
		for j := range gi.img.Pix {
			gi.img.Pix[j] ^= 0x33
		}
		eng := []*Engine{p.on, p.off}[i]
		eng.InvalidateImageCache(gi.img)
		gi.Invalidate()
	}
	cmdCompare(t, p, "картинка переписана на месте")
	cmdCompare(t, p, "следующий кадр")
}

func TestTiled_TwoIdenticalLayersKeepSeparateEntries(t *testing.T) {
	testreset.Globals(t)
	eng := New(640, 480, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root, _ := busyScene(640, 480)
	box1 := widget.NewPanel(color.RGBA{R: 50, G: 50, B: 60, A: 255})
	box1.ShowHeader = false
	box1.SetBounds(image.Rect(0, 0, 320, 480))
	box2 := widget.NewPanel(color.RGBA{R: 50, G: 50, B: 60, A: 255})
	box2.ShowHeader = false
	box2.SetBounds(image.Rect(320, 0, 640, 480))
	same := image.Rect(20, 20, 300, 200)
	box1.AddChild(newGlass(same, 12, color.RGBA{R: 200, G: 200, B: 220, A: 100}, "один"))
	box2.AddChild(newGlass(same.Add(image.Pt(320, 0)), 12, color.RGBA{R: 200, G: 200, B: 220, A: 100}, "один"))
	root.AddChild(box1)
	root.AddChild(box2)
	eng.SetRoot(root)
	eng.RenderOnce()
	before := eng.Stats()
	for i := 0; i < 4; i++ {
		eng.Invalidate()
		eng.RenderOnce()
	}
	after := eng.Stats()
	if hits := after.BlurHits - before.BlurHits; hits != 8 {
		t.Errorf("два слоя за 4 кадра: попаданий %d, ждали 8 (промахов %d)", hits, after.BlurMisses-before.BlurMisses)
	}
}
