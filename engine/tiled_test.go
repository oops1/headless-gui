package engine

// tiled_test.go — тайловый растр (ADR-0005, приёмка 1–2): кадр по тайлам
// побайтно равен кадру одной горутины на сценах с размытой подложкой
// (попадание, промах, слой на нечётном смещении, слой над слоем, слой в
// группе прозрачности, слой у края), Mica, шумом, тенями на стыках,
// картинками на дробном масштабе, поверхностью под слоем, чужим back в
// BGRX; регионы совпадают; паника прямого виджета в воркере; ореол большого
// радиуса не зовёт пул.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"testing"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	hgraster "github.com/oops1/headless-gui/v3/core/raster"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// glass — слой размытой подложки с тинтом и подписью поверх.
type glass struct {
	widget.Base
	radius int
	tint   color.RGBA
	text   string
	group  bool // размытие внутри группы прозрачности
}

func newGlass(r image.Rectangle, radius int, tint color.RGBA, text string) *glass {
	g := &glass{radius: radius, tint: tint, text: text}
	g.SetBounds(r)
	return g
}

func (g *glass) Draw(ctx widget.DrawContext) {
	b := g.Bounds()
	draw := func() {
		if bd, ok := ctx.(widget.BackdropDrawer); ok {
			bd.BlurBehind(b, g.radius, g.tint)
		}
		ctx.DrawTextSize(g.text, b.Min.X+12, b.Min.Y+10, 14, color.RGBA{R: 20, G: 20, B: 30, A: 255})
		ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 6, color.RGBA{R: 255, G: 255, B: 255, A: 120})
	}
	if g.group {
		if od, ok := ctx.(widget.OpacityDrawer); ok {
			od.DrawWithOpacity(b, 0.8, draw)
			return
		}
	}
	draw()
}

// micaPane — слой Mica поверх обоев.
type micaPane struct {
	widget.Base
	radius int
}

func (m *micaPane) Draw(ctx widget.DrawContext) {
	b := m.Bounds()
	if md, ok := ctx.(widget.MicaDrawer); ok {
		md.MicaBehind(b, m.radius, color.RGBA{R: 40, G: 40, B: 60, A: 90})
	}
	ctx.DrawText("mica", b.Min.X+8, b.Min.Y+8, color.RGBA{R: 240, G: 240, B: 240, A: 255})
}

// noisePane — заливка с зерном.
type noisePane struct{ widget.Base }

func (n *noisePane) Draw(ctx widget.DrawContext) {
	b := n.Bounds()
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), color.RGBA{R: 90, G: 100, B: 120, A: 255})
	if nd, ok := ctx.(widget.NoiseDrawer); ok {
		nd.DrawNoise(b, 0.3)
	}
}

// tiledPair — две одинаковые сцены: с тайловым растром и без (путь команд
// одной горутиной).
func tiledPair(build func() (*Engine, widget.Widget)) cmdPair {
	on, rootOn := build()
	off, rootOff := build()
	off.SetTiledRaster(false)
	return cmdPair{on: on, off: off, rootOn: rootOn, rootOff: rootOff, closeAll: func() { on.Stop(); off.Stop() }}
}

// busyScene — неоднородный фон: клетки, подписи, картинка; w×h логических.
func busyScene(w, h int) (*widget.Panel, []*widget.Label) {
	root := widget.NewPanel(color.RGBA{R: 24, G: 40, B: 80, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	const cell = 37
	for y := 0; y < h; y += cell {
		for x := 0; x < w; x += cell {
			shade := uint8(60 + (x/cell*7+y/cell*13)%150)
			p := widget.NewPanel(color.RGBA{R: shade, G: uint8(255 - shade), B: uint8(shade / 2), A: 255})
			p.ShowHeader = false
			p.SetBounds(image.Rect(x, y, x+cell, y+cell))
			root.AddChild(p)
		}
	}
	var labels []*widget.Label
	for i := 0; i < 12; i++ {
		l := widget.NewLabel(fmt.Sprintf("строка %d — Wg", i), color.RGBA{R: 250, G: 250, B: 250, A: 255})
		l.SetBounds(image.Rect(20+(i%3)*200, 30+(i/3)*90, 200+(i%3)*200, 54+(i/3)*90))
		root.AddChild(l)
		labels = append(labels, l)
	}
	return root, labels
}

// blurScene — слои размытия: на нечётном смещении (≥ 3 тайлов), поверх него
// второй, слой в группе прозрачности, слой у края кадра.
func blurScene(k float64) (*Engine, widget.Widget) {
	eng := New(640, 480, 20)
	eng.SetRenderOnDemand(true)
	eng.SetScale(k)
	root, _ := busyScene(640, 480)
	root.AddChild(newGlass(image.Rect(137, 101, 537, 331), 12, color.RGBA{R: 200, G: 200, B: 220, A: 110}, "стекло 12"))
	root.AddChild(newGlass(image.Rect(300, 200, 620, 420), 22, color.RGBA{R: 40, G: 40, B: 40, A: 90}, "стекло 22 над"))
	g := newGlass(image.Rect(30, 350, 260, 470), 10, color.RGBA{R: 255, G: 255, B: 255, A: 80}, "в группе")
	g.group = true
	root.AddChild(g)
	root.AddChild(newGlass(image.Rect(560, 400, 700, 520), 16, color.RGBA{R: 255, G: 240, B: 200, A: 100}, "край"))
	eng.SetRoot(root)
	return eng, root
}

func TestTiled_IdentityBlurScenes(t *testing.T) {
	testreset.Globals(t)
	for _, k := range []float64{1, 1.25, 1.5, 2} {
		t.Run(scaleName(k), func(t *testing.T) {
			p := tiledPair(func() (*Engine, widget.Widget) { return blurScene(k) })
			defer p.closeAll()
			cmdCompare(t, p, "полный кадр (промах)")
			cmdCompare(t, p, "кадр без повреждения (попадание)")
			var la, lb []*widget.Label
			labelsOf(p.rootOn, &la)
			labelsOf(p.rootOff, &lb)
			// Правка под слоем: промах кэша, ореол в частичном кадре.
			la[4].SetText("под стеклом")
			lb[4].SetText("под стеклом")
			cmdCompare(t, p, "правка под слоем")
			cmdCompare(t, p, "следующий кадр (попадание)")
			// Правка рядом со слоем, в кольце запаса: слой не трогается.
			la[0].SetText("рядом")
			lb[0].SetText("рядом")
			cmdCompare(t, p, "правка рядом со слоем")
			// Повреждение внутри слоя мимо виджетов.
			p.on.InvalidateRect(image.Rect(150, 120, 170, 140))
			p.off.InvalidateRect(image.Rect(150, 120, 170, 140))
			cmdCompare(t, p, "повреждение внутри слоя")
			st := p.on.Stats()
			if st.TiledFrames == 0 || st.BlurMisses == 0 || st.BlurHits == 0 {
				t.Errorf("статистика: тайловых кадров %d, промахов %d, попаданий %d", st.TiledFrames, st.BlurMisses, st.BlurHits)
			}
		})
	}
}

func TestTiled_IdentityMicaNoiseShadows(t *testing.T) {
	testreset.Globals(t)
	for _, k := range []float64{1, 1.25, 1.5} {
		t.Run(scaleName(k), func(t *testing.T) {
			build := func() (*Engine, widget.Widget) {
				eng := New(640, 480, 20)
				eng.SetRenderOnDemand(true)
				eng.SetScale(k)
				wall := image.NewRGBA(image.Rect(0, 0, 320, 240))
				for y := 0; y < 240; y++ {
					for x := 0; x < 320; x++ {
						wall.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8((x + y) / 2), A: 255})
					}
				}
				if err := eng.SetBackground(wall); err != nil {
					t.Fatal(err)
				}
				root, _ := busyScene(640, 480)
				m := &micaPane{radius: 40}
				m.SetBounds(image.Rect(93, 71, 430, 300))
				root.AddChild(m)
				n := &noisePane{}
				n.SetBounds(image.Rect(350, 60, 630, 250))
				root.AddChild(n)
				for i, sz := range []struct{ x, y, w, h int }{{40, 300, 300, 150}, {500, 400, 30, 24}, {60, 60, 20, 18}} {
					w := widget.NewWindow(fmt.Sprint("w", i), sz.w, sz.h)
					w.CornerRadius = 8
					w.SetBounds(image.Rect(sz.x, sz.y, sz.x+sz.w, sz.y+sz.h))
					w.SetShadow(testShadow)
					root.AddChild(w)
				}
				eng.SetRoot(root)
				return eng, root
			}
			p := tiledPair(build)
			defer p.closeAll()
			cmdRunSteps(t, p, 17)
			p.on.SetShadowCache(false)
			p.off.SetShadowCache(false)
			cmdCompare(t, p, "тени без кэша шаблонов")
		})
	}
}

func TestTiled_IdentityImagesAndSurfaceUnderGlass(t *testing.T) {
	testreset.Globals(t)
	for _, k := range []float64{1, 1.25} {
		t.Run(scaleName(k), func(t *testing.T) {
			var surfaces []*widget.ExternalSurface
			build := func() (*Engine, widget.Widget) {
				eng := New(640, 480, 20)
				eng.SetRenderOnDemand(true)
				eng.SetScale(k)
				root, _ := busyScene(640, 480)
				big := image.NewRGBA(image.Rect(0, 0, 400, 300))
				for y := 0; y < 300; y++ {
					for x := 0; x < 400; x++ {
						big.SetRGBA(x, y, color.RGBA{R: uint8(x * 255 / 400), G: uint8(y * 255 / 300), B: 128, A: 255})
					}
				}
				iw := widget.NewImageWidget()
				iw.SetBounds(image.Rect(20, 20, 620, 420)) // больше четырёх тайлов — растр на горутине кадра
				iw.SetImage(hgdraw.OpaqueImage(big))
				root.AddChild(iw)
				es := widget.NewExternalSurface()
				es.SetBounds(image.Rect(200, 150, 500, 350))
				buf := image.NewRGBA(image.Rect(0, 0, 300, 200))
				for i := range buf.Pix {
					buf.Pix[i] = uint8(i * 7)
				}
				es.Swap(buf, true, nil)
				root.AddChild(es)
				surfaces = append(surfaces, es)
				root.AddChild(newGlass(image.Rect(150, 100, 560, 400), 18, color.RGBA{R: 220, G: 220, B: 240, A: 100}, "над поверхностью"))
				eng.SetRoot(root)
				return eng, root
			}
			p := tiledPair(build)
			defer p.closeAll()
			cmdCompare(t, p, "полный кадр")
			cmdCompare(t, p, "кадр без повреждения")
			// Поверхность под слоем: ключ непригоден, слой каждый раз ореолом.
			for _, es := range surfaces {
				es.Update(func(buf *image.RGBA) []image.Rectangle {
					for i := range buf.Pix {
						buf.Pix[i] ^= 0x55
					}
					return []image.Rectangle{buf.Rect}
				})
			}
			cmdCompare(t, p, "обновление поверхности под слоем")
			if st := p.on.Stats(); st.BlurHits != 0 {
				t.Errorf("слой над поверхностью не должен кэшироваться: попаданий %d", st.BlurHits)
			}
		})
	}
}

func TestTiled_IdentityBGRXSurfaces(t *testing.T) {
	testreset.Globals(t)
	p := tiledPair(func() (*Engine, widget.Widget) { return blurScene(1) })
	defer p.closeAll()
	for _, eng := range []*Engine{p.on, p.off} {
		bufs := [][]byte{make([]byte, 640*480*4), make([]byte, 640*480*4)}
		if err := eng.SetSurfaces(bufs, 640*4, FormatBGRX); err != nil {
			t.Fatal(err)
		}
	}
	cmdCompare(t, p, "чужой back BGRX: полный кадр")
	var la, lb []*widget.Label
	labelsOf(p.rootOn, &la)
	labelsOf(p.rootOff, &lb)
	la[4].SetText("BGRX")
	lb[4].SetText("BGRX")
	// Буферов два: кадры чередуются, освобождаем оба.
	for i := 1; i <= 2; i++ {
		p.on.ReleaseSurface(i)
		p.off.ReleaseSurface(i)
	}
	cmdCompare(t, p, "чужой back BGRX: частичный кадр под слоем")
}

func TestTiled_RegionsMatch(t *testing.T) {
	testreset.Globals(t)
	p := tiledPair(func() (*Engine, widget.Widget) { return blurScene(1) })
	defer p.closeAll()
	compare := func(name string) {
		t.Helper()
		p.on.Invalidate()
		p.off.Invalidate()
		a, b := p.on.renderFrame(), p.off.renderFrame()
		if len(a.Regions) != len(b.Regions) {
			t.Fatalf("%s: регионов %d и %d", name, len(a.Regions), len(b.Regions))
		}
		for i := range a.Regions {
			if a.Regions[i] != b.Regions[i] {
				t.Errorf("%s: регион %d: %+v против %+v", name, i, a.Regions[i], b.Regions[i])
				return
			}
		}
	}
	compare("полный кадр (промах)")
	compare("полный кадр (попадание)")
}

func TestTiled_SerialFallbackForSmallDamage(t *testing.T) {
	testreset.Globals(t)
	eng, _, labels := cmdGrid(1280, 800, 6, 30)
	defer eng.Stop()
	eng.RenderOnce()
	before := eng.Stats().TiledFrames
	labels[7].SetText("мало тайлов") // ≤ 4 тайлов — прямое воспроизведение
	eng.RenderOnce()
	if eng.Stats().TiledFrames != before {
		t.Error("мелкое повреждение пошло по тайлам")
	}
	eng.Invalidate()
	eng.RenderOnce()
	if eng.Stats().TiledFrames != before+1 {
		t.Error("полный кадр не пошёл по тайлам")
	}
}

func TestTiled_DirectPanicInWorker(t *testing.T) {
	testreset.Globals(t)
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("нужен пул")
	}
	eng := New(640, 480, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	eng.SetFailurePolicy(FailureIsolate)
	root, _ := busyScene(640, 480)
	p := &cmdDirectPanicker{boom: 1}
	p.SetBounds(image.Rect(560, 400, 630, 470)) // последний тайл — не часть 0
	root.AddChild(p)
	eng.SetRoot(root)
	eng.RenderOnce()
	if !widget.IsBroken(p) || eng.Stats().Panics != 1 {
		t.Fatalf("после паники в воркере: сломан=%v, Panics=%d", widget.IsBroken(p), eng.Stats().Panics)
	}
	if widget.DrawTraceUsers() != 0 {
		t.Error("учёт рисующихся остался включённым")
	}
	img := eng.RenderOnce()
	if got := img.RGBAAt(580, 440); got != (color.RGBA{R: 96, G: 96, B: 96, A: 255}) {
		t.Errorf("на месте сломанного прямого виджета %v, ждали заглушку", got)
	}
	// Распространение: наружу выходит исходное значение.
	eng2 := New(640, 480, 20)
	defer eng2.Stop()
	eng2.SetRenderOnDemand(true)
	eng2.SetFailurePolicy(FailurePropagate)
	root2, _ := busyScene(640, 480)
	p2 := &cmdDirectPanicker{boom: 1}
	p2.SetBounds(image.Rect(560, 400, 630, 470))
	root2.AddChild(p2)
	eng2.SetRoot(root2)
	func() {
		defer func() {
			if r := recover(); r != "cmdDirectPanicker: boom" {
				t.Errorf("при распространении наружу вышло %v", r)
			}
		}()
		eng2.RenderOnce()
	}()
}

// bigGlass — слой с физическим радиусом, дающим ореол шире 512: под ним
// непрозрачная картинка во весь кадр — блит из воркера не должен звать пул.
func TestTiled_HugeHaloDoesNotUsePool(t *testing.T) {
	testreset.Globals(t)
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("нужен пул")
	}
	p := tiledPair(func() (*Engine, widget.Widget) {
		eng := New(900, 700, 20)
		eng.SetRenderOnDemand(true)
		root := widget.NewPanel(color.RGBA{A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, 900, 700))
		big := image.NewRGBA(image.Rect(0, 0, 900, 700))
		for i := range big.Pix {
			big.Pix[i] = uint8(i*3 | 0x80)
		}
		for i := 3; i < len(big.Pix); i += 4 {
			big.Pix[i] = 255
		}
		iw := widget.NewImageWidget()
		iw.SetBounds(image.Rect(0, 0, 900, 700))
		iw.SetImage(hgdraw.OpaqueImage(big))
		root.AddChild(iw)
		root.AddChild(newGlass(image.Rect(100, 100, 800, 600), 120, color.RGBA{R: 200, G: 200, B: 220, A: 100}, "огромный радиус"))
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "ореол шире 512")
	cmdCompare(t, p, "попадание")
}

func TestTiled_FontRevRebuildsViews(t *testing.T) {
	testreset.Globals(t)
	p := tiledPair(func() (*Engine, widget.Widget) {
		eng, root := benchTextScene(640, 400)
		eng.SetRenderOnDemand(true)
		return eng, root
	})
	defer p.closeAll()
	cmdCompare(t, p, "до смены DPI")
	p.on.SetDPI(120)
	p.off.SetDPI(120)
	cmdCompare(t, p, "после смены DPI")
	p.on.SetTextSubpixel(true)
	p.off.SetTextSubpixel(true)
	cmdCompare(t, p, "после включения подпикселя")
}

// Переключение тайлового растра на живом движке — тот же кадр.
func TestTiled_ToggleKeepsFrame(t *testing.T) {
	testreset.Globals(t)
	eng, _ := blurScene(1.25)
	defer eng.Stop()
	a := eng.RenderOnce()
	eng.SetTiledRaster(false)
	b := eng.RenderOnce()
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Error("кадр после выключения тайлового растра отличается")
	}
	if eng.TiledRaster() {
		t.Error("TiledRaster после выключения")
	}
	_ = hgraster.FormatRGBA
}
