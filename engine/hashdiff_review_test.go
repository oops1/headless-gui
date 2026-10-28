package engine

// hashdiff_review_test.go — случаи из ревью корректности 4а.3 (ADR-0006):
// ребёнок в группе прозрачности и прокрутка в on-demand, картинка в обёртке
// OpaqueImage с InvalidateImageCache внутренней, виджет и лейбл, рисующие за
// границами, после частичного кадра, подсказка и модальное при частичных
// кадрах, оверлей в холсте при частичных кадрах и с чужими буферами вывода,
// переносы, паника в Draw, смена окружения, признаки регионов на
// последовательном пути, тени/Mica/шум по тайлам. Сравниваются front, back,
// множество тайлов, признаки регионов и переносы с движком без хэш-диффа.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// reviewCompareFrames — renderFrame у пары и сравнение front-буферов, тайлов и регионов.
func reviewCompareFrames(t *testing.T, p cmdPair, name string) {
	t.Helper()
	fa, fb := p.on.renderFrame(), p.off.renderFrame()
	a, b := p.on.canvas.Front(), p.off.canvas.Front()
	if !bytes.Equal(a.Pix, b.Pix) {
		n, first := 0, image.Point{-1, -1}
		for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
			for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
				if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
					if n == 0 {
						first = image.Pt(x, y)
					}
					n++
				}
			}
		}
		t.Errorf("%s: front расходится в %d пикселях, первый %v (hash %v, без %v)", name, n, first, a.RGBAAt(first.X, first.Y), b.RGBAAt(first.X, first.Y))
	}
	ba, bb := p.on.canvas.BackImage(), p.off.canvas.BackImage()
	if !bytes.Equal(ba.Pix, bb.Pix) {
		t.Errorf("%s: back расходится", name)
	}
	if len(fa.Tiles) != len(fb.Tiles) {
		t.Errorf("%s: тайлов %d против %d", name, len(fa.Tiles), len(fb.Tiles))
	}
}

// Ребёнок внутри группы прозрачности под удержанным родителем (on-demand).
func TestHashDiffReview_OpacityGroupChildOnDemand(t *testing.T) {
	testreset.Globals(t)
	for _, tiled := range []bool{true, false} {
		name := "тайлы"
		if !tiled {
			name = "последовательно"
		}
		t.Run(name, func(t *testing.T) {
			var labels []*widget.Label
			p := hashPair(func() (*Engine, widget.Widget) {
				eng := New(640, 480, 20)
				eng.SetRenderOnDemand(true)
				eng.SetTiledRaster(tiled)
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
			reviewCompareFrames(t, p, "полный кадр")
			for _, l := range labels {
				l.SetText("ребёнок изменился")
			}
			reviewCompareFrames(t, p, "правка ребёнка в группе (частичный кадр)")
			reviewCompareFrames(t, p, "кадр без повреждения")
			a := p.on.RenderOnce()
			for _, l := range labels {
				l.SetText("и ещё раз")
			}
			b := p.on.RenderOnce()
			if bytes.Equal(a.Pix, b.Pix) {
				t.Errorf("правка ребёнка внутри группы прозрачности не попала в кадр")
			}
		})
	}
}

// Картинка в обёртке OpaqueImage, переписанная на месте + InvalidateImageCache(внутренняя).
func TestHashDiffReview_OpaqueWrapperInvalidateImageCache(t *testing.T) {
	testreset.Globals(t)
	for _, k := range []float64{1, 1.25} {
		t.Run(scaleName(k), func(t *testing.T) {
			var imgs []*image.RGBA
			p := hashPair(func() (*Engine, widget.Widget) {
				eng := New(640, 480, 20)
				eng.SetRenderOnDemand(false)
				eng.SetScale(k)
				root, _ := busyScene(640, 480)
				img := image.NewRGBA(image.Rect(0, 0, 200, 120))
				for i := range img.Pix {
					img.Pix[i] = uint8(i*3 | 0x80)
				}
				for i := 3; i < len(img.Pix); i += 4 {
					img.Pix[i] = 255
				}
				iw := widget.NewImageWidget()
				iw.Stretch = widget.ImageStretchNone
				iw.SetBounds(image.Rect(100, 100, 300, 220))
				iw.SetImage(hgdraw.OpaqueImage(img))
				root.AddChild(iw)
				imgs = append(imgs, img)
				eng.SetRoot(root)
				return eng, root
			})
			defer p.closeAll()
			cmdCompare(t, p, "полный кадр")
			cmdCompare(t, p, "без изменений")
			engines := []*Engine{p.on, p.off}
			for i, img := range imgs {
				for j := 0; j < len(img.Pix); j += 4 {
					img.Pix[j] ^= 0x7f
				}
				engines[i].InvalidateImageCache(img) // внутренняя картинка, как у кэша масштабирования
			}
			cmdCompare(t, p, "переписана на месте + InvalidateImageCache(внутренняя)")
			cmdCompare(t, p, "после")
		})
	}
}

// overhang — виджет, рисующий за своими границами (тень/ореол), Invalidate — по границам.
type overhang struct {
	widget.Base
	col color.RGBA
}

func (w *overhang) Draw(ctx widget.DrawContext) {
	b := w.Bounds()
	ctx.FillRect(b.Min.X-30, b.Min.Y-30, b.Dx()+60, b.Dy()+60, w.col)
}

// Виджет рисует за границами; частичный кадр по Invalidate, затем кадр без повреждения.
func TestHashDiffReview_OverhangAfterPartialThenNoDamage(t *testing.T) {
	testreset.Globals(t)
	var ws []*overhang
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, _ := busyScene(640, 480)
		w := &overhang{col: color.RGBA{R: 200, G: 30, B: 30, A: 255}}
		w.SetBounds(image.Rect(200, 200, 300, 260))
		root.AddChild(w)
		ws = append(ws, w)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	reviewCompareFrames(t, p, "полный кадр")
	for _, w := range ws {
		w.col = color.RGBA{R: 30, G: 200, B: 30, A: 255}
		w.Invalidate()
	}
	reviewCompareFrames(t, p, "частичный кадр по Invalidate")
	reviewCompareFrames(t, p, "кадр без повреждения")
	reviewCompareFrames(t, p, "ещё кадр без повреждения")
}

// Лейбл с текстом шире границ (глифы за краем).
func TestHashDiffReview_LabelOverflowAfterPartialThenNoDamage(t *testing.T) {
	testreset.Globals(t)
	var ls []*widget.Label
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, _ := busyScene(640, 480)
		l := widget.NewLabel("короткий", color.RGBA{R: 255, G: 255, B: 255, A: 255})
		l.SetBounds(image.Rect(200, 200, 260, 220))
		root.AddChild(l)
		ls = append(ls, l)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	reviewCompareFrames(t, p, "полный кадр")
	for _, l := range ls {
		l.SetText("очень длинный текст, вылезающий далеко за границы лейбла")
	}
	reviewCompareFrames(t, p, "частичный кадр по SetText")
	reviewCompareFrames(t, p, "кадр без повреждения")
}

// Прокрутка: смена ScrollY и правка ребёнка внутри (on-demand).
func TestHashDiffReview_ScrollViewOnDemand(t *testing.T) {
	testreset.Globals(t)
	for _, tiled := range []bool{true, false} {
		name := "тайлы"
		if !tiled {
			name = "последовательно"
		}
		t.Run(name, func(t *testing.T) {
			var svs []*widget.ScrollView
			var labels [][]*widget.Label
			p := hashPair(func() (*Engine, widget.Widget) {
				eng := New(640, 480, 20)
				eng.SetRenderOnDemand(true)
				eng.SetTiledRaster(tiled)
				root, _ := busyScene(640, 480)
				sv := widget.NewScrollView()
				sv.SetBounds(image.Rect(100, 100, 500, 400))
				sv.ContentHeight = 1200
				var ls []*widget.Label
				for i := 0; i < 24; i++ {
					l := widget.NewLabel("строка прокрутки", color.RGBA{R: 255, G: 255, B: 255, A: 255})
					l.SetBounds(image.Rect(110, 110+i*48, 480, 110+i*48+24))
					sv.AddChild(l)
					ls = append(ls, l)
				}
				root.AddChild(sv)
				svs = append(svs, sv)
				labels = append(labels, ls)
				eng.SetRoot(root)
				return eng, root
			})
			defer p.closeAll()
			reviewCompareFrames(t, p, "полный кадр")
			for _, sv := range svs {
				sv.SetScrollY(70)
			}
			reviewCompareFrames(t, p, "прокрутка")
			reviewCompareFrames(t, p, "без изменений")
			for _, ls := range labels {
				ls[3].SetText("изменено в прокрутке")
			}
			reviewCompareFrames(t, p, "правка внутри прокрутки")
			for _, sv := range svs {
				sv.SetScrollY(0)
			}
			reviewCompareFrames(t, p, "прокрутка назад")
			reviewCompareFrames(t, p, "без изменений 2")
		})
	}
}

// Подсказка и модальное при частичных кадрах (on-demand).
func TestHashDiffReview_TooltipAndModalOnDemand(t *testing.T) {
	testreset.Globals(t)
	var labels [][]*widget.Label
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		eng.SetTooltipsEnabled(true)
		eng.SetTooltipDelay(time.Hour)
		root, ls := busyScene(640, 480)
		w := &tipped{}
		w.SetBounds(image.Rect(300, 200, 400, 240))
		root.AddChild(w)
		labels = append(labels, ls)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	engines := []*Engine{p.on, p.off}
	reviewCompareFrames(t, p, "полный кадр")
	for _, eng := range engines {
		eng.SendMouseMove(350, 220)
		eng.SetTooltipDelay(0)
	}
	// Кадр без повреждения — подсказка появляется.
	for _, ls := range labels {
		ls[0].SetText("правка при подсказке")
	}
	reviewCompareFrames(t, p, "подсказка + частичный кадр")
	for _, ls := range labels {
		ls[4].SetText("ещё правка")
	}
	reviewCompareFrames(t, p, "подсказка показана, правка далеко")
	for _, ls := range labels {
		ls[6].SetText("правка под плашкой") // лейбл 6: x 20..200, y 210..234
	}
	reviewCompareFrames(t, p, "правка рядом")
	for _, eng := range engines {
		eng.SetTooltipDelay(time.Hour)
		eng.SendMouseMove(20, 20)
	}
	reviewCompareFrames(t, p, "подсказка исчезла")
	reviewCompareFrames(t, p, "после исчезновения")
	// Модальное.
	var ms []*dimModal
	for _, eng := range engines {
		m := &dimModal{modal: true}
		m.SetBounds(image.Rect(160, 120, 480, 360))
		eng.ShowModal(m)
		ms = append(ms, m)
	}
	reviewCompareFrames(t, p, "модальный показан")
	for _, ls := range labels {
		ls[1].SetText("правка под затемнением")
	}
	reviewCompareFrames(t, p, "частичный кадр под модальным")
	reviewCompareFrames(t, p, "без изменений под модальным")
	for i, eng := range engines {
		ms[i].modal = false
		eng.CloseModal(ms[i])
	}
	reviewCompareFrames(t, p, "модальный закрыт")
	reviewCompareFrames(t, p, "после закрытия")
}

// Оверлей в холсте при частичных кадрах (on-demand): открыт, правка рядом, закрыт.
func TestHashDiffReview_OverlayOnDemandPartial(t *testing.T) {
	testreset.Globals(t)
	var pops []*popper
	var labels [][]*widget.Label
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, ls := busyScene(640, 480)
		pop := &popper{}
		pop.SetBounds(image.Rect(200, 100, 320, 130))
		root.AddChild(pop)
		pops = append(pops, pop)
		labels = append(labels, ls)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	reviewCompareFrames(t, p, "полный кадр")
	for i, pop := range pops {
		pop.open = true
		pop.Invalidate()
		[]*Engine{p.on, p.off}[i].InvalidateRect(image.Rect(160, 130, 420, 280))
	}
	reviewCompareFrames(t, p, "оверлей открыт (частичный)")
	for _, ls := range labels {
		ls[4].SetText("правка под оверлеем")
	}
	reviewCompareFrames(t, p, "правка под оверлеем (частичный)")
	reviewCompareFrames(t, p, "без изменений")
	for i, pop := range pops {
		pop.open = false
		pop.Invalidate()
		[]*Engine{p.on, p.off}[i].InvalidateRect(image.Rect(160, 130, 420, 280))
	}
	reviewCompareFrames(t, p, "оверлей закрыт (частичный)")
	reviewCompareFrames(t, p, "после закрытия")
}

// reviewCompareFull — пиксели front/back, множество тайлов и признаки регионов по тайлам.
func reviewCompareFull(t *testing.T, p cmdPair, name string) (output.Frame, output.Frame) {
	t.Helper()
	fa, fb := p.on.renderFrame(), p.off.renderFrame()
	a, b := p.on.canvas.Front(), p.off.canvas.Front()
	if !bytes.Equal(a.Pix, b.Pix) {
		n, first := 0, image.Point{-1, -1}
		for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
			for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
				if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
					if n == 0 {
						first = image.Pt(x, y)
					}
					n++
				}
			}
		}
		t.Errorf("%s: front расходится в %d пикселях, первый %v (hash %v, без %v)", name, n, first, a.RGBAAt(first.X, first.Y), b.RGBAAt(first.X, first.Y))
	}
	if !bytes.Equal(p.on.canvas.BackImage().Pix, p.off.canvas.BackImage().Pix) {
		t.Errorf("%s: back расходится", name)
	}
	key := func(tl output.DirtyTile) string { return fmt.Sprintf("%d,%d", tl.X, tl.Y) }
	ta, tb := map[string]bool{}, map[string]bool{}
	for _, tl := range fa.Tiles {
		ta[key(tl)] = true
	}
	for _, tl := range fb.Tiles {
		tb[key(tl)] = true
	}
	for k := range ta {
		if !tb[k] {
			t.Errorf("%s: тайл %s есть у hash, нет у прямого", name, k)
		}
	}
	for k := range tb {
		if !ta[k] {
			t.Errorf("%s: тайл %s есть у прямого, нет у hash", name, k)
		}
	}
	ra, rb := map[string]output.Region{}, map[string]output.Region{}
	for _, r := range fa.Regions {
		ra[fmt.Sprintf("%d,%d", r.Rect.Min.X, r.Rect.Min.Y)] = r
	}
	for _, r := range fb.Regions {
		rb[fmt.Sprintf("%d,%d", r.Rect.Min.X, r.Rect.Min.Y)] = r
	}
	for k, r := range ra {
		if q, ok := rb[k]; ok && (q.Kind != r.Kind || q.Color != r.Color) {
			t.Errorf("%s: регион %s: hash %v/%v, прямой %v/%v", name, k, r.Kind, r.Color, q.Kind, q.Color)
		}
	}
	if len(fa.Moves) != len(fb.Moves) {
		t.Errorf("%s: переносов %d против %d", name, len(fa.Moves), len(fb.Moves))
	}
	return fa, fb
}

// Регионы при оверлее на последовательном пути (признаки тайлов ставятся до клипа).
func TestHashDiffReview_RegionsWithOverlaySerial(t *testing.T) {
	testreset.Globals(t)
	for _, tiled := range []bool{false, true} {
		name := "последовательно"
		if tiled {
			name = "тайлы"
		}
		t.Run(name, func(t *testing.T) {
			var pops []*popper
			var labels [][]*widget.Label
			p := hashPair(func() (*Engine, widget.Widget) {
				eng := New(640, 480, 20)
				eng.SetRenderOnDemand(true)
				eng.SetTiledRaster(tiled)
				root, ls := busyScene(640, 480)
				pop := &popper{}
				pop.SetBounds(image.Rect(200, 100, 320, 130))
				root.AddChild(pop)
				pops = append(pops, pop)
				labels = append(labels, ls)
				eng.SetRoot(root)
				return eng, root
			})
			defer p.closeAll()
			engines := []*Engine{p.on, p.off}
			reviewCompareFull(t, p, "полный кадр")
			// Несколько частичных кадров с правками лейблов — признаки соседей текут.
			for step := 0; step < 4; step++ {
				for _, ls := range labels {
					ls[step].SetText(fmt.Sprintf("правка %d", step))
				}
				reviewCompareFull(t, p, fmt.Sprintf("правка лейбла %d", step))
			}
			for i, pop := range pops {
				pop.open = true
				pop.Invalidate()
				engines[i].InvalidateRect(image.Rect(160, 130, 420, 280))
			}
			reviewCompareFull(t, p, "оверлей открыт (частичный)")
			for _, ls := range labels {
				ls[5].SetText("правка при оверлее")
			}
			reviewCompareFull(t, p, "правка при открытом оверлее")
			// Кадр без повреждения при открытом оверлее: все выбраны, дифф по всему.
			reviewCompareFull(t, p, "кадр без повреждения при оверлее")
			reviewCompareFull(t, p, "ещё один")
		})
	}
}

// Чужие буферы вывода + оверлей + частичные кадры.
func TestHashDiffReview_SurfacesWithOverlay(t *testing.T) {
	testreset.Globals(t)
	var pops []*popper
	var labels [][]*widget.Label
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, ls := busyScene(640, 480)
		pop := &popper{}
		pop.SetBounds(image.Rect(200, 100, 320, 130))
		root.AddChild(pop)
		pops = append(pops, pop)
		labels = append(labels, ls)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	const w, h = 640, 480
	bufs := map[*Engine][][]byte{}
	engines := []*Engine{p.on, p.off}
	for _, eng := range engines {
		b := [][]byte{make([]byte, w*h*4), make([]byte, w*h*4)}
		if err := eng.SetSurfaces(b, w*4, FormatRGBA); err != nil {
			t.Fatal(err)
		}
		bufs[eng] = b
	}
	check := func(name string) {
		t.Helper()
		fa, fb := p.on.renderFrame(), p.off.renderFrame()
		if fa.Surface == 0 && fb.Surface == 0 {
			return
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
	for _, ls := range labels {
		ls[2].SetText("буфер 1")
	}
	check("правка 1")
	for _, ls := range labels {
		ls[7].SetText("буфер 2")
	}
	check("правка 2")
	for i, pop := range pops {
		pop.open = true
		pop.Invalidate()
		engines[i].InvalidateRect(image.Rect(160, 130, 420, 280))
	}
	check("оверлей открыт")
	for _, ls := range labels {
		ls[0].SetText("правка при оверлее")
	}
	check("правка при оверлее")
	check("без изменений при оверлее")
	for i, pop := range pops {
		pop.open = false
		pop.Invalidate()
		engines[i].InvalidateRect(image.Rect(160, 130, 420, 280))
	}
	check("оверлей закрыт")
	check("после закрытия")
	for _, ls := range labels {
		ls[9].SetText("буфер 3")
	}
	check("правка 3")
	check("без изменений")
}

// Переносы (NotifyMove) при частичном кадре.
func TestHashDiffReview_MovesPartial(t *testing.T) {
	testreset.Globals(t)
	var panes [][]*widget.Panel
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		root, _ := busyScene(640, 480)
		win := widget.NewPanel(color.RGBA{R: 200, G: 200, B: 50, A: 255})
		win.ShowHeader = false
		win.SetBounds(image.Rect(100, 100, 300, 250))
		root.AddChild(win)
		panes = append(panes, []*widget.Panel{win})
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	reviewCompareFull(t, p, "полный кадр")
	reviewCompareFull(t, p, "без изменений")
	for _, ps := range panes {
		w := ps[0]
		src := w.Bounds()
		dst := src.Add(image.Pt(40, 30))
		w.SetBounds(dst)
		widget.NotifyMove(src, dst)
	}
	fa, fb := reviewCompareFull(t, p, "перенос")
	if len(fa.Moves) == 0 || len(fb.Moves) == 0 {
		t.Logf("переносов: hash %d, прямой %d", len(fa.Moves), len(fb.Moves))
	}
	reviewCompareFull(t, p, "после переноса")
	reviewCompareFull(t, p, "без изменений 2")
}

// Паника в Draw записываемого виджета (изоляция) и дальше.
func TestHashDiffReview_PanicInDrawThenFrames(t *testing.T) {
	testreset.Globals(t)
	// Прогрев: первая паника в процессе включает учёт рисующихся, виновник
	// неизвестен — иначе два движка пары повели бы себя по-разному.
	warm := New(200, 200, 20)
	defer warm.Stop() // живёт до конца теста: учёт включён им и остаётся
	warm.SetFailurePolicy(FailureIsolate)
	{
		root := widget.NewPanel(color.RGBA{A: 255})
		root.SetBounds(image.Rect(0, 0, 200, 200))
		pk := &cmdPanicker{boom: 1}
		pk.SetBounds(image.Rect(10, 10, 100, 100))
		root.AddChild(pk)
		warm.SetRoot(root)
		warm.RenderOnce()
	}
	var pks []*cmdPanicker
	var labels [][]*widget.Label
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		eng.SetFailurePolicy(FailureIsolate)
		root, ls := busyScene(640, 480)
		pk := &cmdPanicker{}
		pk.SetBounds(image.Rect(400, 300, 500, 380))
		root.AddChild(pk)
		pks = append(pks, pk)
		labels = append(labels, ls)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	reviewCompareFull(t, p, "полный кадр")
	for _, ls := range labels {
		ls[1].SetText("до паники")
	}
	reviewCompareFull(t, p, "частичный")
	for _, pk := range pks {
		pk.boom = 1
		pk.Invalidate()
	}
	p.on.renderFrame()
	p.off.renderFrame()
	reviewCompareFull(t, p, "кадр после паники")
	for _, ls := range labels {
		ls[2].SetText("после паники")
	}
	reviewCompareFull(t, p, "частичный после паники")
	reviewCompareFull(t, p, "без изменений")
	for _, pk := range pks {
		widget.ClearBroken(pk)
	}
	reviewCompareFull(t, p, "после ClearBroken")
	reviewCompareFull(t, p, "без изменений 2")
}

// Смена размера с тем же числом тайлов, DPI, подпиксель, фон.
func TestHashDiffReview_EnvChanges(t *testing.T) {
	testreset.Globals(t)
	p := hashPair(func() (*Engine, widget.Widget) {
		eng, root := blurScene(1)
		return eng, root
	})
	defer p.closeAll()
	engines := []*Engine{p.on, p.off}
	reviewCompareFull(t, p, "полный кадр")
	reviewCompareFull(t, p, "без изменений")
	for _, eng := range engines {
		eng.SetResolution(600, 470) // 10×8 тайлов, как и 640×480
	}
	reviewCompareFull(t, p, "после SetResolution")
	reviewCompareFull(t, p, "без изменений 2")
	for _, eng := range engines {
		eng.SetDPI(120)
	}
	reviewCompareFull(t, p, "после SetDPI")
	for _, eng := range engines {
		eng.SetTextSubpixel(true)
	}
	reviewCompareFull(t, p, "после подпикселя")
	wall := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for i := range wall.Pix {
		wall.Pix[i] = uint8(i * 5)
	}
	for _, eng := range engines {
		if err := eng.SetBackground(wall); err != nil {
			t.Fatal(err)
		}
	}
	reviewCompareFull(t, p, "после SetBackground")
	reviewCompareFull(t, p, "без изменений 3")
	for _, eng := range engines {
		eng.SetPixelFormat(FormatBGRX)
	}
	reviewCompareFull(t, p, "после BGRX")
	reviewCompareFull(t, p, "без изменений 4")
}

// imgPopper — оверлей, рисующий картинку (признак Image) поверх соседей.
type imgPopper struct {
	widget.Base
	open bool
	img  *image.RGBA
}

func (p *imgPopper) Draw(ctx widget.DrawContext) {
	b := p.Bounds()
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), color.RGBA{R: 90, G: 90, B: 200, A: 255})
}
func (p *imgPopper) HasOverlay() bool { return p.open }
func (p *imgPopper) DrawOverlay(ctx widget.DrawContext) {
	b := p.Bounds()
	ctx.DrawImage(p.img, b.Min.X, b.Max.Y+10)
}

// Утечка признаков тайлов на последовательном пути: корневая заливка
// при растре одного тайла помечает все тайлы сплошными; пропущенный тайл с
// текстом под оверлеем-картинкой уезжает с признаком Image вместо Mixed.
func TestHashDiffReview_RegionLeakSerial(t *testing.T) {
	testreset.Globals(t)
	var pops []*imgPopper
	var labels [][]*widget.Label
	p := hashPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetRenderOnDemand(true)
		eng.SetTiledRaster(false)
		root := widget.NewPanel(color.RGBA{R: 24, G: 40, B: 80, A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, 640, 480))
		l1 := widget.NewLabel("A", color.RGBA{R: 255, G: 255, B: 255, A: 255})
		l1.SetBounds(image.Rect(10, 10, 50, 30)) // тайл (0,0)
		root.AddChild(l1)
		l2 := widget.NewLabel("B текст", color.RGBA{R: 255, G: 255, B: 255, A: 255})
		l2.SetBounds(image.Rect(330, 210, 380, 230)) // тайл (5,3)
		root.AddChild(l2)
		img := image.NewRGBA(image.Rect(0, 0, 30, 20))
		for i := range img.Pix {
			img.Pix[i] = 200
		}
		pop := &imgPopper{img: img}
		pop.SetBounds(image.Rect(330, 150, 400, 170))
		root.AddChild(pop)
		pops = append(pops, pop)
		labels = append(labels, []*widget.Label{l1, l2})
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	engines := []*Engine{p.on, p.off}
	reviewCompareFull(t, p, "полный кадр")
	for _, ls := range labels {
		ls[0].SetText("A2") // частичный кадр: тайл (0,0) по одному, корневая заливка метит все
	}
	reviewCompareFull(t, p, "правка A")
	for i, pop := range pops {
		pop.open = true
		pop.Invalidate()
		engines[i].damageRect(image.Rect(320, 170, 420, 240)) // владельческое повреждение, без принуждения
	}
	fa, fb := reviewCompareFull(t, p, "оверлей-картинка над текстом")
	t.Logf("регионы hash=%v", fa.Regions)
	t.Logf("регионы прямой=%v", fb.Regions)
}

// Последовательный путь по тайлам (replayTiles): тени, Mica, шум на
// стыках тайлов при частичных кадрах, масштабы 1/1,25/1,5.
func TestHashDiffReview_SerialTilesShadowsMicaNoise(t *testing.T) {
	testreset.Globals(t)
	for _, k := range []float64{1, 1.25, 1.5} {
		t.Run(scaleName(k), func(t *testing.T) {
			build := func() (*Engine, widget.Widget) {
				eng := New(640, 480, 20)
				eng.SetRenderOnDemand(true)
				eng.SetTiledRaster(false)
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
			p := hashPair(build)
			defer p.closeAll()
			cmdRunSteps(t, p, 23)
			p.on.SetShadowCache(false)
			p.off.SetShadowCache(false)
			cmdRunSteps(t, p, 29)
		})
	}
}
