package engine

// hashdiff_test.go — хэш-дифф тайлов (ADR-0006, приёмка 1–2): кадры движка
// с хэш-диффом и без побайтно равны при кадрах без изменений, Invalidate
// контейнера с точечной правкой, правках под слоем размытия, картинке,
// переписанной на месте, оверлее, модальном, подсказке, поверхности,
// чужих буферах вывода, последовательном пути; кадр без изменений не рисует
// ни одного тайла, правка лейбла под Invalidate корня — только его тайлы.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// hashPair — две одинаковые сцены: с хэш-диффом и без (растр и дифф всех
// выбранных тайлов, как в 4а.2).
func hashPair(build func() (*Engine, widget.Widget)) cmdPair {
	on, rootOn := build()
	off, rootOff := build()
	off.SetHashDiff(false)
	return cmdPair{on: on, off: off, rootOn: rootOn, rootOff: rootOff, closeAll: func() { on.Stop(); off.Stop() }}
}

// invalidateW — Invalidate виджета за интерфейсом.
func invalidateW(w widget.Widget) { w.(interface{ Invalidate() }).Invalidate() }

// gridTiles — число тайлов холста движка.
func gridTiles(eng *Engine) int {
	cols, rows := eng.canvas.TileGrid()
	return cols * rows
}

func TestHashDiff_NoChangeFrameSkipsEveryTile(t *testing.T) {
	testreset.Globals(t)
	for _, tiled := range []bool{true, false} {
		name := "тайлы"
		if !tiled {
			name = "последовательно"
		}
		t.Run(name, func(t *testing.T) {
			eng := New(640, 480, 20)
			defer eng.Stop()
			eng.SetTiledRaster(tiled)
			// Два слоя друг над другом (ключ верхнего включает ключ нижнего);
			// слой в группе прозрачности рисовался бы всегда.
			root, _ := busyScene(640, 480)
			root.AddChild(newGlass(image.Rect(137, 101, 537, 331), 12, color.RGBA{R: 200, G: 200, B: 220, A: 110}, "стекло 12"))
			root.AddChild(newGlass(image.Rect(300, 200, 620, 420), 22, color.RGBA{R: 40, G: 40, B: 40, A: 90}, "стекло 22 над"))
			eng.SetRoot(root)
			first := eng.renderFrame()
			if len(first.Tiles) == 0 {
				t.Fatal("первый кадр без тайлов")
			}
			before := eng.Stats()
			fr := eng.renderFrame()
			after := eng.Stats()
			if n := gridTiles(eng); after.TilesSkipped-before.TilesSkipped != uint64(n) {
				t.Errorf("кадр без изменений пропустил %d тайлов из %d", after.TilesSkipped-before.TilesSkipped, n)
			}
			if len(fr.Tiles) != 0 {
				t.Errorf("кадр без изменений отдал %d тайлов", len(fr.Tiles))
			}
			if after.Recorded == before.Recorded {
				t.Errorf("дерево кадра без повреждения обязано перезаписываться (Recorded не вырос)")
			}
		})
	}
}

func TestHashDiff_IdentityScenes(t *testing.T) {
	testreset.Globals(t)
	for _, k := range []float64{1, 1.25} {
		t.Run(scaleName(k), func(t *testing.T) {
			p := hashPair(func() (*Engine, widget.Widget) {
				eng, root := blurScene(k)
				eng.SetRenderOnDemand(false)
				return eng, root
			})
			defer p.closeAll()
			var la, lb []*widget.Label
			labelsOf(p.rootOn, &la)
			labelsOf(p.rootOff, &lb)
			cmdCompare(t, p, "полный кадр")
			cmdCompare(t, p, "без изменений")
			cmdCompare(t, p, "ещё без изменений")
			la[4].SetText("правка вне слоя")
			lb[4].SetText("правка вне слоя")
			cmdCompare(t, p, "правка лейбла")
			la[7].SetText("под стеклом")
			lb[7].SetText("под стеклом")
			cmdCompare(t, p, "правка под слоем")
			cmdCompare(t, p, "после правки под слоем")
			invalidateW(p.rootOn)
			invalidateW(p.rootOff)
			cmdCompare(t, p, "Invalidate корня без правок")
			p.on.Invalidate()
			p.off.Invalidate()
			cmdCompare(t, p, "Engine.Invalidate")
			cmdCompare(t, p, "и снова без изменений")
		})
	}
}

// Частичные кадры (on-demand) под слоями, затем кадры без повреждения: слой,
// читавший за краем повреждения, не закрепляется пропуском.
func TestHashDiff_IdentityPartialThenFull(t *testing.T) {
	testreset.Globals(t)
	for _, tiled := range []bool{true, false} {
		name := "тайлы"
		if !tiled {
			name = "последовательно"
		}
		t.Run(name, func(t *testing.T) {
			p := hashPair(func() (*Engine, widget.Widget) {
				eng, root := blurScene(1)
				eng.SetTiledRaster(tiled)
				return eng, root
			})
			defer p.closeAll()
			var la, lb []*widget.Label
			labelsOf(p.rootOn, &la)
			labelsOf(p.rootOff, &lb)
			cmdCompare(t, p, "полный кадр")
			la[7].SetText("под стеклом")
			lb[7].SetText("под стеклом")
			cmdCompare(t, p, "частичный кадр под слоем")
			la[1].SetText("вне стекла")
			lb[1].SetText("вне стекла")
			cmdCompare(t, p, "частичный кадр вне слоя")
			cmdCompare(t, p, "кадр без повреждения")
			cmdCompare(t, p, "ещё один")
			p.on.Invalidate()
			p.off.Invalidate()
			cmdCompare(t, p, "Engine.Invalidate")
			la[8].SetText("снова под стеклом")
			lb[8].SetText("снова под стеклом")
			cmdCompare(t, p, "частичный после полного")
			cmdCompare(t, p, "без повреждения после частичного")
		})
	}
}

func TestHashDiff_RootInvalidateRendersOnlyChangedTiles(t *testing.T) {
	testreset.Globals(t)
	var labels [][]*widget.Label
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, ls := busyScene(640, 480)
		eng.SetRoot(root)
		labels = append(labels, ls)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "полный кадр")
	before := p.on.Stats()
	for i, ls := range labels {
		ls[5].SetText("изменено")
		invalidateW([]widget.Widget{p.rootOn, p.rootOff}[i])
	}
	fr := p.on.renderFrame()
	p.off.renderFrame()
	after := p.on.Stats()
	if after.TilesSkipped == before.TilesSkipped {
		t.Errorf("Invalidate корня с правкой одного лейбла: ни один тайл не пропущен")
	}
	lb := p.on.canvas.PhysRect(labels[0][5].Bounds())
	for _, tl := range fr.Tiles {
		if !image.Rect(tl.X, tl.Y, tl.X+tl.W, tl.Y+tl.H).Overlaps(lb) {
			t.Errorf("тайл %d,%d вне лейбла %v попал в выдачу", tl.X, tl.Y, lb)
		}
	}
	cmdCompare(t, p, "после правки")
}

// imagePane рисует картинку как есть (DrawImage).
type imagePane struct {
	widget.Base
	img *image.RGBA
}

func (p *imagePane) Draw(ctx widget.DrawContext) {
	b := p.Bounds()
	ctx.DrawImage(p.img, b.Min.X, b.Min.Y)
}

func TestHashDiff_ImageRewrittenInPlaceNeedsInvalidateImageCache(t *testing.T) {
	testreset.Globals(t)
	var panes []*imagePane
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(false)
		root, _ := busyScene(640, 480)
		img := image.NewRGBA(image.Rect(0, 0, 200, 120))
		for i := range img.Pix {
			img.Pix[i] = uint8(i*3 | 0x80)
		}
		pane := &imagePane{img: img}
		pane.SetBounds(image.Rect(100, 100, 300, 220))
		root.AddChild(pane)
		panes = append(panes, pane)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "полный кадр")
	cmdCompare(t, p, "без изменений")
	engines := []*Engine{p.on, p.off}
	for i, pane := range panes {
		for j := range pane.img.Pix {
			pane.img.Pix[j] ^= 0x33
		}
		engines[i].InvalidateImageCache(pane.img)
	}
	cmdCompare(t, p, "картинка переписана на месте + InvalidateImageCache")
	cmdCompare(t, p, "после")
	// Без поколения правка невидима — это контракт; проверяем, что с ним
	// картинка в кадре новая, а не прошлая.
	a := p.on.RenderOnce()
	for i, pane := range panes {
		for j := range pane.img.Pix {
			pane.img.Pix[j] ^= 0x0F
		}
		engines[i].InvalidateImageCache(pane.img)
	}
	b := p.on.RenderOnce()
	if bytes.Equal(a.Pix, b.Pix) {
		t.Errorf("после InvalidateImageCache кадр обязан измениться")
	}
	cmdCompare(t, p, "и ещё раз")
}

// fader — группа прозрачности с детьми внутри: ребёнок перезаписывается под
// удержанным родителем, хэш группы обязан это видеть.
type fader struct {
	widget.Base
	alpha float64
}

func (f *fader) Draw(ctx widget.DrawContext) {
	b := f.Bounds()
	draw := func() {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), color.RGBA{R: 60, G: 60, B: 70, A: 255})
		for _, ch := range f.Children() {
			widget.DrawTraced(ch, ctx)
		}
	}
	if od, ok := ctx.(widget.OpacityDrawer); ok {
		od.DrawWithOpacity(b, f.alpha, draw)
		return
	}
	draw()
}

func TestHashDiff_ChildInsideOpacityGroup(t *testing.T) {
	testreset.Globals(t)
	var labels []*widget.Label
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(false)
		root, _ := busyScene(640, 480)
		f := &fader{alpha: 0.7}
		f.SetBounds(image.Rect(100, 100, 400, 300))
		l := widget.NewLabel("внутри группы", color.RGBA{R: 255, G: 255, B: 255, A: 255})
		l.SetBounds(image.Rect(120, 120, 380, 150))
		f.AddChild(l)
		root.AddChild(f)
		labels = append(labels, l)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "полный кадр")
	cmdCompare(t, p, "без изменений")
	for _, l := range labels {
		l.SetText("ребёнок изменился")
	}
	cmdCompare(t, p, "правка ребёнка в группе")
	cmdCompare(t, p, "после правки")
	a := p.on.RenderOnce()
	for _, l := range labels {
		l.SetText("и ещё раз")
	}
	b := p.on.RenderOnce()
	if bytes.Equal(a.Pix, b.Pix) {
		t.Errorf("правка ребёнка внутри группы прозрачности не попала в кадр")
	}
}

// popper — виджет с оверлеем в холсте: полупрозрачная плашка поверх соседей
// (двойное смешение было бы видно).
type popper struct {
	widget.Base
	open bool
}

func (p *popper) Draw(ctx widget.DrawContext) {
	b := p.Bounds()
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), color.RGBA{R: 90, G: 90, B: 200, A: 255})
}

func (p *popper) HasOverlay() bool { return p.open }

func (p *popper) DrawOverlay(ctx widget.DrawContext) {
	b := p.Bounds()
	ctx.FillRectAlpha(b.Min.X-40, b.Max.Y, 260, 150, color.RGBA{R: 255, G: 255, B: 255, A: 90})
	ctx.DrawText("оверлей", b.Min.X, b.Max.Y+10, color.RGBA{A: 255})
}

func TestHashDiff_OverlayOpenAndClose(t *testing.T) {
	testreset.Globals(t)
	var pops []*popper
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(false)
		root, _ := busyScene(640, 480)
		pop := &popper{}
		pop.SetBounds(image.Rect(200, 100, 320, 130))
		root.AddChild(pop)
		pops = append(pops, pop)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "полный кадр")
	cmdCompare(t, p, "без изменений")
	for _, pop := range pops {
		pop.open = true
		pop.Invalidate()
	}
	cmdCompare(t, p, "оверлей открыт")
	cmdCompare(t, p, "оверлей открыт, без изменений")
	cmdCompare(t, p, "и ещё кадр")
	for _, pop := range pops {
		pop.open = false
		pop.Invalidate()
	}
	cmdCompare(t, p, "оверлей закрыт")
	cmdCompare(t, p, "после закрытия")
	before := p.on.Stats()
	p.on.RenderOnce()
	if after := p.on.Stats(); after.TilesSkipped == before.TilesSkipped {
		t.Errorf("через кадр после закрытия оверлея пропуск обязан вернуться")
	}
}

// dimModal — модальный виджет с затемнением.
type dimModal struct {
	widget.Base
	modal bool
}

func (m *dimModal) IsModal() bool        { return m.modal }
func (m *dimModal) DimColor() color.RGBA { return color.RGBA{A: 120} }
func (m *dimModal) Draw(ctx widget.DrawContext) {
	b := m.Bounds()
	ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 8, color.RGBA{R: 240, G: 240, B: 245, A: 255})
	ctx.DrawText("диалог", b.Min.X+20, b.Min.Y+20, color.RGBA{A: 255})
}

func TestHashDiff_ModalOpenAndClose(t *testing.T) {
	testreset.Globals(t)
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(false)
		root, _ := busyScene(640, 480)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "полный кадр")
	cmdCompare(t, p, "без изменений")
	var ms []*dimModal
	for _, eng := range []*Engine{p.on, p.off} {
		m := &dimModal{modal: true}
		m.SetBounds(image.Rect(160, 120, 480, 360))
		eng.ShowModal(m)
		ms = append(ms, m)
	}
	cmdCompare(t, p, "модальный показан")
	cmdCompare(t, p, "модальный, без изменений")
	for i, eng := range []*Engine{p.on, p.off} {
		ms[i].modal = false
		eng.CloseModal(ms[i])
	}
	cmdCompare(t, p, "модальный закрыт")
	cmdCompare(t, p, "после закрытия")
}

// tipped — виджет с подсказкой.
type tipped struct {
	widget.Base
}

func (w *tipped) Draw(ctx widget.DrawContext) {
	b := w.Bounds()
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), color.RGBA{R: 200, G: 120, B: 60, A: 255})
}

func (w *tipped) ToolTipAt(x, y int) string { return "подсказка для хэша" }

func TestHashDiff_TooltipAppearsAndDisappears(t *testing.T) {
	testreset.Globals(t)
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(false)
		eng.SetTooltipsEnabled(true)
		eng.SetTooltipDelay(time.Hour)
		root, _ := busyScene(640, 480)
		w := &tipped{}
		w.SetBounds(image.Rect(300, 200, 400, 240))
		root.AddChild(w)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "полный кадр")
	for _, eng := range []*Engine{p.on, p.off} {
		eng.SendMouseMove(350, 220)
	}
	cmdCompare(t, p, "курсор над виджетом, подсказка ещё не созрела")
	for _, eng := range []*Engine{p.on, p.off} {
		eng.SetTooltipDelay(0)
	}
	cmdCompare(t, p, "подсказка показана")
	cmdCompare(t, p, "подсказка показана, без изменений")
	for _, eng := range []*Engine{p.on, p.off} {
		eng.SetTooltipDelay(time.Hour)
		eng.SendMouseMove(20, 20)
	}
	cmdCompare(t, p, "подсказка исчезла")
	cmdCompare(t, p, "после исчезновения")
}

func TestHashDiff_ExternalSurfaceUpdate(t *testing.T) {
	testreset.Globals(t)
	var surfaces []*widget.ExternalSurface
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, _ := busyScene(640, 480)
		es := widget.NewExternalSurface()
		es.SetBounds(image.Rect(200, 150, 500, 350))
		buf := image.NewRGBA(image.Rect(0, 0, 300, 200))
		for i := range buf.Pix {
			buf.Pix[i] = uint8(i * 7)
		}
		es.Swap(buf, true, nil)
		root.AddChild(es)
		surfaces = append(surfaces, es)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "полный кадр")
	cmdCompare(t, p, "без изменений")
	for _, es := range surfaces {
		es.Update(func(buf *image.RGBA) []image.Rectangle {
			for i := range buf.Pix {
				buf.Pix[i] ^= 0x55
			}
			return []image.Rectangle{image.Rect(10, 10, 120, 90)}
		})
	}
	cmdCompare(t, p, "обновление поверхности на месте")
	cmdCompare(t, p, "после обновления")
}

func TestHashDiff_SurfacesGetSkippedTiles(t *testing.T) {
	testreset.Globals(t)
	p := hashPair(func() (*Engine, widget.Widget) {
		eng, root := blurScene(1)
		return eng, root
	})
	defer p.closeAll()
	const w, h = 640, 480
	bufs := map[*Engine][][]byte{}
	for _, eng := range []*Engine{p.on, p.off} {
		b := [][]byte{make([]byte, w*h*4), make([]byte, w*h*4), make([]byte, w*h*4)}
		if err := eng.SetSurfaces(b, w*4, FormatRGBA); err != nil {
			t.Fatal(err)
		}
		bufs[eng] = b
	}
	var la, lb []*widget.Label
	labelsOf(p.rootOn, &la)
	labelsOf(p.rootOff, &lb)
	check := func(name string) {
		t.Helper()
		fa, fb := p.on.renderFrame(), p.off.renderFrame()
		if fa.Surface == 0 && fb.Surface == 0 {
			return // кадр без тайлов буфер не занимает
		}
		if fa.Surface == 0 || fb.Surface == 0 {
			t.Fatalf("%s: кадр не в буфере потребителя (%d, %d)", name, fa.Surface, fb.Surface)
		}
		a, b := bufs[p.on][fa.Surface-1], bufs[p.off][fb.Surface-1]
		if !bytes.Equal(a, b) {
			n := 0
			for i := range a {
				if a[i] != b[i] {
					n++
				}
			}
			t.Errorf("%s: буферы потребителя расходятся в %d байтах", name, n)
		}
		p.on.ReleaseSurface(fa.Surface)
		p.off.ReleaseSurface(fb.Surface)
	}
	check("полный кадр")
	la[3].SetText("буфер")
	lb[3].SetText("буфер")
	invalidateW(p.rootOn)
	invalidateW(p.rootOff)
	check("Invalidate корня с правкой")
	la[8].SetText("ещё")
	lb[8].SetText("ещё")
	check("правка лейбла")
	check("без изменений")
	check("и ещё без изменений")
}

func TestHashDiff_DirectWidgetTilesAlwaysRender(t *testing.T) {
	testreset.Globals(t)
	eng := New(640, 480, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(false)
	root, _ := busyScene(640, 480)
	es := widget.NewExternalSurface()
	es.SetBounds(image.Rect(0, 0, 64, 64))
	buf := image.NewRGBA(image.Rect(0, 0, 64, 64))
	es.Swap(buf, true, nil)
	root.AddChild(es)
	eng.SetRoot(root)
	eng.renderFrame()
	before := eng.Stats()
	eng.renderFrame()
	after := eng.Stats()
	if n := gridTiles(eng); after.TilesSkipped-before.TilesSkipped >= uint64(n) {
		t.Errorf("тайл прямого виджета пропущен: пропущено %d из %d", after.TilesSkipped-before.TilesSkipped, n)
	}
}

// Запись кэша размытия, вымытая по TTL за кадры без обращений, восстанавливается
// при первой правке над слоем (растр слоя собирается принудительно), и дальше
// правки над слоем идут попаданием, а не ореолом.
func TestHashDiff_BlurCacheRestoredAfterTTL(t *testing.T) {
	testreset.Globals(t)
	eng := New(640, 480, 20)
	defer eng.Stop()
	root, _ := busyScene(640, 480)
	root.AddChild(newGlass(image.Rect(60, 60, 500, 300), 12, color.RGBA{R: 200, G: 200, B: 220, A: 110}, "стекло"))
	above := widget.NewLabel("над стеклом", color.RGBA{R: 255, G: 255, B: 255, A: 255})
	above.SetBounds(image.Rect(100, 100, 300, 124))
	root.AddChild(above)
	eng.SetRoot(root)
	eng.renderFrame()
	above.SetText("тёплый кэш")
	eng.renderFrame()
	for i := 0; i < blurCacheTTL+2; i++ {
		eng.renderFrame() // слой без обращений к кэшу
	}
	above.SetText("после простоя")
	eng.renderFrame() // промах: запись вымыта, растр собирается заново
	before := eng.Stats()
	for i := 0; i < 3; i++ {
		above.SetText(fmt.Sprintf("правка %d", i))
		eng.renderFrame()
	}
	after := eng.Stats()
	if hits := after.BlurHits - before.BlurHits; hits != 3 {
		t.Errorf("правки над стеклом после восстановления: попаданий %d, ждали 3 (промахов %d)", hits, after.BlurMisses-before.BlurMisses)
	}
}

func TestHashDiff_EngineInvalidateRendersEverything(t *testing.T) {
	testreset.Globals(t)
	eng, _ := blurScene(1)
	defer eng.Stop()
	eng.renderFrame()
	eng.renderFrame()
	before := eng.Stats()
	eng.Invalidate()
	eng.renderFrame()
	after := eng.Stats()
	if after.TilesSkipped != before.TilesSkipped {
		t.Errorf("Engine.Invalidate: пропущено %d тайлов, ждали 0", after.TilesSkipped-before.TilesSkipped)
	}
}

func TestHashDiff_CmdSumMatchesHash(t *testing.T) {
	testreset.Globals(t)
	eng, rootW := blurScene(1)
	defer eng.Stop()
	eng.RenderOnce()
	root := rootW.(hgdraw.ListHolder).CmdList()
	if root == nil {
		t.Fatal("нет списка кадра")
	}
	n := 0
	hgdraw.VisitOverlapping(root, image.Rect(0, 0, 640, 480), func(c *hgdraw.Cmd) bool {
		if c.Op == hgdraw.OpDirect {
			return true
		}
		n++
		if got, want := c.Sum(), c.Hash(hgdraw.HashSeed); got != want {
			t.Fatalf("Sum команды %v = %x, Hash = %x", c.Op, got, want)
		}
		return true
	})
	if n == 0 {
		t.Fatal("ни одной команды")
	}
}
