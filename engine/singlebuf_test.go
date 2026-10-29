package engine

// singlebuf_test.go — один буфер кадра (ADR-0007, приёмка 1–3): кадры движка
// с одним буфером и с двумя побайтно равны (front, множество тайлов,
// признаки регионов) на сценах размытия, с оверлеями, модальным окном,
// подсказкой, переходами оверлеев, переносами, на прямом пути,
// последовательном пути и с чужими буферами вывода; слои поверх дерева,
// записанные командами, равны прямому пути; память — один буфер кадра, окно
// back заводится под слои после растра и освобождается.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// singlePair — две одинаковые сцены: один буфер кадра и два.
func singlePair(build func() (*Engine, widget.Widget)) cmdPair {
	on, rootOn := build()
	off, rootOff := build()
	off.SetSingleBuffer(false)
	return cmdPair{on: on, off: off, rootOn: rootOn, rootOff: rootOff, closeAll: func() { on.Stop(); off.Stop() }}
}

// directPair — путь команд против прямого пути (оба с одним буфером).
func directPair(build func() (*Engine, widget.Widget)) cmdPair {
	on, rootOn := build()
	off, rootOff := build()
	off.SetCommandPath(false)
	return cmdPair{on: on, off: off, rootOn: rootOn, rootOff: rootOff, closeAll: func() { on.Stop(); off.Stop() }}
}

// sbCompare — renderFrame у пары: front побайтно, множество тайлов, признаки
// регионов по тайлам, переносы.
func sbCompare(t *testing.T, p cmdPair, name string) (output.Frame, output.Frame) {
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
		t.Errorf("%s: front расходится в %d пикселях, первый %v (%v против %v)", name, n, first, a.RGBAAt(first.X, first.Y), b.RGBAAt(first.X, first.Y))
	}
	key := func(tl output.DirtyTile) string { return fmt.Sprintf("%d,%d", tl.X, tl.Y) }
	ta, tb := map[string]bool{}, map[string]bool{}
	for _, tl := range fa.Tiles {
		ta[key(tl)] = true
	}
	for _, tl := range fb.Tiles {
		tb[key(tl)] = true
	}
	if len(ta) != len(tb) || len(ta) != len(fa.Tiles) || len(tb) != len(fb.Tiles) {
		t.Errorf("%s: тайлов %d против %d (уникальных %d/%d)", name, len(fa.Tiles), len(fb.Tiles), len(ta), len(tb))
	} else {
		for k := range ta {
			if !tb[k] {
				t.Errorf("%s: тайл %s есть только у одного буфера", name, k)
				break
			}
		}
	}
	if len(fa.Regions) != len(fb.Regions) {
		t.Errorf("%s: регионов %d против %d", name, len(fa.Regions), len(fb.Regions))
	} else {
		ra := map[string]output.Region{}
		for _, r := range fa.Regions {
			ra[fmt.Sprintf("%d,%d", r.Rect.Min.X, r.Rect.Min.Y)] = r
		}
		for _, r := range fb.Regions {
			if q, ok := ra[fmt.Sprintf("%d,%d", r.Rect.Min.X, r.Rect.Min.Y)]; ok && (q.Kind != r.Kind || q.Color != r.Color) {
				t.Errorf("%s: признак тайла %v: %v/%v против %v/%v", name, r.Rect.Min, q.Kind, q.Color, r.Kind, r.Color)
				break
			}
		}
	}
	if len(fa.Moves) != len(fb.Moves) {
		t.Errorf("%s: переносов %d против %d", name, len(fa.Moves), len(fb.Moves))
	}
	return fa, fb
}

// tileDataMatchesFront — данные тайлов кадра равны front в их местах.
func tileDataMatchesFront(t *testing.T, eng *Engine, fr output.Frame, name string) {
	t.Helper()
	front := eng.canvas.Front()
	for _, tl := range fr.Tiles {
		if tl.Data == nil {
			continue
		}
		for row := 0; row < tl.H; row++ {
			o := front.PixOffset(tl.X, tl.Y+row)
			if !bytes.Equal(tl.Data[row*tl.W*4:(row+1)*tl.W*4], front.Pix[o:o+tl.W*4]) {
				t.Fatalf("%s: данные тайла %d,%d расходятся с front в строке %d", name, tl.X, tl.Y, row)
			}
		}
	}
}

func TestSingleBuffer_IdentityBlurScenes(t *testing.T) {
	testreset.Globals(t)
	for _, k := range []float64{1, 1.25} {
		t.Run(scaleName(k), func(t *testing.T) {
			p := singlePair(func() (*Engine, widget.Widget) { return blurScene(k) })
			defer p.closeAll()
			var la, lb []*widget.Label
			labelsOf(p.rootOn, &la)
			labelsOf(p.rootOff, &lb)
			fa, _ := sbCompare(t, p, "полный кадр")
			tileDataMatchesFront(t, p.on, fa, "полный кадр")
			sbCompare(t, p, "без изменений")
			la[4].SetText("правка вне слоя")
			lb[4].SetText("правка вне слоя")
			fa, _ = sbCompare(t, p, "частичный кадр вне слоя")
			tileDataMatchesFront(t, p.on, fa, "частичный кадр")
			la[7].SetText("под стеклом")
			lb[7].SetText("под стеклом")
			sbCompare(t, p, "частичный кадр под слоем")
			sbCompare(t, p, "кадр без повреждения")
			invalidateW(p.rootOn)
			invalidateW(p.rootOff)
			sbCompare(t, p, "Invalidate корня")
			p.on.Invalidate()
			p.off.Invalidate()
			sbCompare(t, p, "Engine.Invalidate")
			p.on.InvalidateRect(image.Rect(0, 0, 30, 30))
			p.off.InvalidateRect(image.Rect(0, 0, 30, 30))
			p.on.InvalidateRect(image.Rect(600, 440, 640, 480))
			p.off.InvalidateRect(image.Rect(600, 440, 640, 480))
			sbCompare(t, p, "две далёкие области")
			if p.on.Stats().BufferBytes != uint64(len(p.on.canvas.Front().Pix)) {
				t.Errorf("один буфер: байт буферов %d, front %d", p.on.Stats().BufferBytes, len(p.on.canvas.Front().Pix))
			}
		})
	}
}

// Оверлей, модальное окно и подсказка — командами: один буфер против двух
// (частичные кадры) и путь команд против прямого (полные кадры: частичный
// кадр прямого пути под слоем размытия читает за краем повреждения то, что
// легло поверх растра в прошлом кадре, и с полным кадром не совпадает — это
// давнее свойство прямого пути, а не слоёв командами).
func TestSingleBuffer_IdentityTopLayers(t *testing.T) {
	testreset.Globals(t)
	for _, mode := range []string{"один/два буфера", "команды/прямой"} {
		t.Run(mode, func(t *testing.T) {
			var pops []*popper
			var labels [][]*widget.Label
			build := func() (*Engine, widget.Widget) {
				eng := New(640, 480, 20)
				eng.SetRenderOnDemand(mode == "один/два буфера")
				eng.SetTooltipsEnabled(true)
				eng.SetTooltipDelay(time.Hour)
				root, ls := busyScene(640, 480)
				root.AddChild(newGlass(image.Rect(137, 101, 537, 331), 12, color.RGBA{R: 200, G: 200, B: 220, A: 110}, "стекло"))
				pop := &popper{}
				pop.SetBounds(image.Rect(200, 100, 320, 130))
				root.AddChild(pop)
				w := &tipped{}
				w.SetBounds(image.Rect(300, 200, 400, 240))
				root.AddChild(w)
				pops = append(pops, pop)
				labels = append(labels, ls)
				eng.SetRoot(root)
				return eng, root
			}
			var p cmdPair
			if mode == "один/два буфера" {
				p = singlePair(build)
			} else {
				p = directPair(build)
			}
			defer p.closeAll()
			engines := []*Engine{p.on, p.off}
			sbCompare(t, p, "полный кадр")
			for i, pop := range pops {
				pop.open = true
				pop.Invalidate()
				engines[i].InvalidateRect(image.Rect(160, 130, 420, 280))
			}
			sbCompare(t, p, "оверлей открыт (частичный)")
			sbCompare(t, p, "оверлей, без изменений")
			for _, ls := range labels {
				ls[4].SetText("правка под оверлеем")
			}
			sbCompare(t, p, "правка под оверлеем")
			for _, eng := range engines {
				eng.SendMouseMove(350, 220)
				eng.SetTooltipDelay(0)
			}
			for _, ls := range labels {
				ls[0].SetText("правка при подсказке")
			}
			sbCompare(t, p, "подсказка над оверлеем")
			sbCompare(t, p, "подсказка, без изменений")
			var ms []*dimModal
			for _, eng := range engines {
				m := &dimModal{modal: true}
				m.SetBounds(image.Rect(160, 120, 480, 360))
				eng.ShowModal(m)
				ms = append(ms, m)
			}
			sbCompare(t, p, "модальный показан")
			sbCompare(t, p, "модальный, без изменений")
			for _, ls := range labels {
				ls[1].SetText("правка под затемнением")
			}
			sbCompare(t, p, "частичный кадр под модальным")
			for _, eng := range engines {
				eng.SetTooltipDelay(time.Hour)
				eng.SendMouseMove(20, 20)
			}
			sbCompare(t, p, "подсказка исчезла под модальным")
			for i, eng := range engines {
				ms[i].modal = false
				eng.CloseModal(ms[i])
			}
			sbCompare(t, p, "модальный закрыт")
			for i, pop := range pops {
				pop.open = false
				pop.Invalidate()
				engines[i].InvalidateRect(image.Rect(160, 130, 420, 280))
			}
			sbCompare(t, p, "оверлей закрыт")
			sbCompare(t, p, "после закрытия")
		})
	}
}

// Модальное окно поверх неизменного дерева: с записанными слоями кадр без
// изменений пропускает все тайлы (прежде модальное принуждало их каждый кадр).
func TestSingleBuffer_ModalFrameSkipsTiles(t *testing.T) {
	testreset.Globals(t)
	eng := New(640, 480, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(false)
	root, _ := busyScene(640, 480)
	eng.SetRoot(root)
	m := &dimModal{modal: true}
	m.SetBounds(image.Rect(160, 120, 480, 360))
	eng.ShowModal(m)
	eng.renderFrame()
	eng.renderFrame()
	before := eng.Stats()
	fr := eng.renderFrame()
	after := eng.Stats()
	if n := gridTiles(eng); after.TilesSkipped-before.TilesSkipped != uint64(n) {
		t.Errorf("кадр с модальным без изменений пропустил %d тайлов из %d", after.TilesSkipped-before.TilesSkipped, n)
	}
	if len(fr.Tiles) != 0 {
		t.Errorf("кадр с модальным без изменений отдал %d тайлов", len(fr.Tiles))
	}
	if eng.canvas.BackWindow() != (image.Rectangle{}) {
		t.Errorf("окно back заведено без слоёв после растра: %v", eng.canvas.BackWindow())
	}
}

func TestSingleBuffer_IdentityTransitions(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"один/два буфера", "команды/прямой"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 8)
			a := newOvtScene(t, true, m, false)
			b := newOvtScene(t, true, m, false)
			if mode == "один/два буфера" {
				b.eng.SetSingleBuffer(false)
			} else {
				b.eng.SetCommandPath(false)
			}
			// Оверлей мимо присмотра после меню и подсказка над ним.
			for _, s := range []*ovtScene{a, b} {
				panel := &ovtSelf{}
				panel.SetBounds(image.Rect(150, 0, 151, 1))
				panel.rect = image.Rect(100, 80, 300, 200)
				panel.content = color.RGBA{R: 20, G: 200, B: 20, A: 255}
				s.root.AddChild(panel)
				tip := &tipped{}
				tip.SetBounds(image.Rect(60, 180, 120, 220))
				s.root.AddChild(tip)
				s.eng.SetTooltipsEnabled(true)
				s.eng.SetTooltipDelay(time.Hour)
			}
			p := cmdPair{on: a.eng, off: b.eng, rootOn: a.root, rootOff: b.root}
			sbCompare(t, p, "панель и меню закрыты")
			a.setOpen(true)
			b.setOpen(true)
			sbCompare(t, p, "меню открыто: кадр начала перехода")
			t0 := time.Now()
			for ms := 0; ms <= 120; ms += 30 {
				widget.StepAnimationsFor(a.eng, t0.Add(time.Duration(ms)*time.Millisecond))
				widget.StepAnimationsFor(b.eng, t0.Add(time.Duration(ms)*time.Millisecond))
				sbCompare(t, p, fmt.Sprintf("шаг появления %d мс", ms))
			}
			sbCompare(t, p, "меню в покое")
			// Подсказка над меню и правка рядом.
			for _, s := range []*ovtScene{a, b} {
				s.eng.SendMouseMove(90, 200)
				s.eng.SetTooltipDelay(0)
				s.eng.InvalidateRect(image.Rect(0, 0, 20, 20))
			}
			sbCompare(t, p, "подсказка над меню")
			sbCompare(t, p, "подсказка, без изменений")
			for _, s := range []*ovtScene{a, b} {
				s.eng.SetTooltipDelay(time.Hour)
				s.eng.SendMouseMove(5, 5)
			}
			sbCompare(t, p, "подсказка исчезла")
			// Панель сама анимируется: её сдвиг — повреждение.
			for _, s := range []*ovtScene{a, b} {
				s.eng.InvalidateRect(image.Rect(100, 80, 300, 200))
			}
			sbCompare(t, p, "правка под панелью")
			a.setOpen(false)
			b.setOpen(false)
			sbCompare(t, p, "меню закрыто: кадр начала исчезновения")
			t1 := time.Now()
			for ms := 0; ms <= 120; ms += 30 {
				widget.StepAnimationsFor(a.eng, t1.Add(time.Duration(ms)*time.Millisecond))
				widget.StepAnimationsFor(b.eng, t1.Add(time.Duration(ms)*time.Millisecond))
				sbCompare(t, p, fmt.Sprintf("шаг исчезновения %d мс", ms))
			}
			sbCompare(t, p, "после исчезновения")
			sbCompare(t, p, "без изменений")
		})
	}
}

// Переходы и модальное окно над меню: порядок слоёв и смесь шага.
func TestSingleBuffer_IdentityTransitionUnderModal(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	a := newOvtScene(t, true, m, false)
	b := newOvtScene(t, true, m, false)
	b.eng.SetSingleBuffer(false)
	p := cmdPair{on: a.eng, off: b.eng, rootOn: a.root, rootOff: b.root}
	a.setOpen(true)
	b.setOpen(true)
	sbCompare(t, p, "меню открыто")
	t0 := time.Now()
	widget.StepAnimationsFor(a.eng, t0)
	widget.StepAnimationsFor(b.eng, t0)
	widget.StepAnimationsFor(a.eng, t0.Add(30*time.Millisecond))
	widget.StepAnimationsFor(b.eng, t0.Add(30*time.Millisecond))
	sbCompare(t, p, "шаг появления")
	var ms []*dimModal
	for _, eng := range []*Engine{a.eng, b.eng} {
		md := &dimModal{modal: true}
		md.SetBounds(image.Rect(60, 60, 260, 200))
		eng.ShowModal(md)
		ms = append(ms, md)
	}
	sbCompare(t, p, "модальный над меню в переходе")
	widget.StepAnimationsFor(a.eng, t0.Add(60*time.Millisecond))
	widget.StepAnimationsFor(b.eng, t0.Add(60*time.Millisecond))
	sbCompare(t, p, "шаг под модальным")
	widget.StepAnimationsFor(a.eng, t0.Add(200*time.Millisecond))
	widget.StepAnimationsFor(b.eng, t0.Add(200*time.Millisecond))
	sbCompare(t, p, "конец перехода под модальным")
	sbCompare(t, p, "без изменений")
	for i, eng := range []*Engine{a.eng, b.eng} {
		ms[i].modal = false
		eng.CloseModal(ms[i])
	}
	sbCompare(t, p, "модальный закрыт")
}

func TestSingleBuffer_IdentityMovesPartial(t *testing.T) {
	testreset.Globals(t)
	var panes [][]*widget.Panel
	p := singlePair(func() (*Engine, widget.Widget) {
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
	sbCompare(t, p, "полный кадр")
	for step := 0; step < 3; step++ {
		for _, ps := range panes {
			w := ps[0]
			src := w.Bounds()
			dst := src.Add(image.Pt(40, 30))
			w.SetBounds(dst)
			widget.NotifyMove(src, dst)
		}
		fa, _ := sbCompare(t, p, fmt.Sprintf("перенос %d", step))
		tileDataMatchesFront(t, p.on, fa, "перенос")
		sbCompare(t, p, "после переноса")
	}
}

// Прямой путь (SetCommandPath(false)) с одним буфером: окно back размером с
// кадр, сброс по заявленным областям — против прямого пути с двумя буферами.
func TestSingleBuffer_IdentityDirectPath(t *testing.T) {
	testreset.Globals(t)
	p := singlePair(func() (*Engine, widget.Widget) {
		eng, root := blurScene(1)
		eng.SetCommandPath(false)
		return eng, root
	})
	defer p.closeAll()
	var la, lb []*widget.Label
	labelsOf(p.rootOn, &la)
	labelsOf(p.rootOff, &lb)
	sbCompare(t, p, "полный кадр")
	la[4].SetText("правка")
	lb[4].SetText("правка")
	sbCompare(t, p, "частичный кадр")
	p.on.InvalidateRect(image.Rect(0, 0, 32, 32))
	p.off.InvalidateRect(image.Rect(0, 0, 32, 32))
	p.on.InvalidateRect(image.Rect(600, 440, 640, 480))
	p.off.InvalidateRect(image.Rect(600, 440, 640, 480))
	sbCompare(t, p, "две далёкие области")
	sbCompare(t, p, "без повреждения")
	if w := p.on.canvas.BackWindow(); w != p.on.canvas.Frame() {
		t.Errorf("прямой путь: окно back %v, ждали весь кадр %v", w, p.on.canvas.Frame())
	}
}

// Последовательный путь (тайлы выключены) и один процессор (NewOffscreen).
func TestSingleBuffer_IdentitySerial(t *testing.T) {
	testreset.Globals(t)
	for _, offscreen := range []bool{false, true} {
		name := "тайлы выключены"
		if offscreen {
			name = "offscreen"
		}
		t.Run(name, func(t *testing.T) {
			p := singlePair(func() (*Engine, widget.Widget) {
				var eng *Engine
				if offscreen {
					eng = NewOffscreen(640, 480, 20)
					eng.SetRenderOnDemand(true)
				} else {
					eng = New(640, 480, 20)
					eng.SetRenderOnDemand(true)
					eng.SetTiledRaster(false)
				}
				root, _ := busyScene(640, 480)
				root.AddChild(newGlass(image.Rect(137, 101, 537, 331), 12, color.RGBA{R: 200, G: 200, B: 220, A: 110}, "стекло"))
				eng.SetRoot(root)
				return eng, root
			})
			defer p.closeAll()
			var la, lb []*widget.Label
			labelsOf(p.rootOn, &la)
			labelsOf(p.rootOff, &lb)
			sbCompare(t, p, "полный кадр")
			la[2].SetText("вне стекла")
			lb[2].SetText("вне стекла")
			sbCompare(t, p, "частичный кадр вне слоя")
			la[7].SetText("под стеклом")
			lb[7].SetText("под стеклом")
			sbCompare(t, p, "частичный кадр под слоем")
			sbCompare(t, p, "без повреждения")
			p.on.Invalidate()
			p.off.Invalidate()
			sbCompare(t, p, "Engine.Invalidate")
		})
	}
}

// Чужие буферы вывода: один буфер движка ничего не меняет (back — память
// потребителя), буферы потребителя равны.
func TestSingleBuffer_IdentitySurfaces(t *testing.T) {
	testreset.Globals(t)
	p := singlePair(func() (*Engine, widget.Widget) { return blurScene(1) })
	defer p.closeAll()
	const w, h = 640, 480
	bufs := map[*Engine][][]byte{}
	for _, eng := range []*Engine{p.on, p.off} {
		b := [][]byte{make([]byte, w*h*4), make([]byte, w*h*4)}
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
		fa, fb := sbCompare(t, p, name)
		if fa.Surface == 0 && fb.Surface == 0 {
			return
		}
		if fa.Surface == 0 || fb.Surface == 0 {
			t.Fatalf("%s: кадр не в буфере потребителя (%d, %d)", name, fa.Surface, fb.Surface)
		}
		if !bytes.Equal(bufs[p.on][fa.Surface-1], bufs[p.off][fb.Surface-1]) {
			t.Errorf("%s: буферы потребителя расходятся", name)
		}
		p.on.ReleaseSurface(fa.Surface)
		p.off.ReleaseSurface(fb.Surface)
	}
	check("полный кадр")
	la[3].SetText("буфер")
	lb[3].SetText("буфер")
	check("правка лейбла")
	check("без изменений")
	for _, eng := range []*Engine{p.on, p.off} {
		if err := eng.SetSurfaces(nil, 0, FormatRGBA); err != nil {
			t.Fatal(err)
		}
	}
	check("назад к своему буферу")
	la[5].SetText("снова свой")
	lb[5].SetText("снова свой")
	check("правка в своём буфере")
}

func TestSingleBuffer_IdentityDesktop(t *testing.T) {
	testreset.Globals(t)
	a := buildPipelineDesktop()
	b := buildPipelineDesktop()
	defer a.closeAll()
	defer b.closeAll()
	defer a.eng.Stop()
	defer b.eng.Stop()
	b.eng.SetSingleBuffer(false)
	p := cmdPair{on: a.eng, off: b.eng, rootOn: a.eng.root, rootOff: b.eng.root}
	sbCompare(t, p, "полный кадр")
	sbCompare(t, p, "без изменений")
	btn := a.startBtn.Bounds()
	onX, onY := (btn.Min.X+btn.Max.X)/2, (btn.Min.Y+btn.Max.Y)/2
	a.eng.SendMouseMove(onX, onY)
	b.eng.SendMouseMove(onX, onY)
	sbCompare(t, p, "наведение на Пуск")
	a.eng.SendMouseMove(onX, pipeH/2)
	b.eng.SendMouseMove(onX, pipeH/2)
	sbCompare(t, p, "снятие наведения")
	a.eng.Invalidate()
	b.eng.Invalidate()
	sbCompare(t, p, "Invalidate")
}

// Память: после кадров без слоёв после растра у холста движка один
// полноразмерный буфер; меню с переходом заводит окно под его место, модальное
// с переходами — окно размером с кадр; через backWindowIdle кадров без слоёв
// окно освобождается.
func TestSingleBuffer_MemoryOneFrontBuffer(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	s := newOvtScene(t, true, m, false)
	eng := s.eng
	frontBytes := uint64(len(eng.canvas.Front().Pix))
	for i := 0; i < 3; i++ {
		eng.RenderOnce()
	}
	if got := eng.Stats().BufferBytes; got != frontBytes {
		t.Fatalf("без слоёв после растра: байт буферов %d, front %d", got, frontBytes)
	}
	if w := eng.canvas.BackWindow(); !w.Empty() {
		t.Fatalf("окно back без слоёв: %v", w)
	}
	// Меню под присмотром переходов: окно над его местом, не весь кадр.
	ovtFinishOpen(s)
	eng.RenderOnce()
	if w := eng.canvas.BackWindow(); w.Empty() || w == eng.canvas.Frame() {
		t.Fatalf("меню с переходом: окно back %v", w)
	}
	if got := eng.Stats().BufferBytes; got <= frontBytes || got >= 2*frontBytes {
		t.Fatalf("меню с переходом: байт буферов %d при front %d", got, frontBytes)
	}
	// Модальное выше меню — весь кадр.
	md := &dimModal{modal: true}
	md.SetBounds(image.Rect(60, 60, 260, 200))
	eng.ShowModal(md)
	eng.RenderOnce()
	if w := eng.canvas.BackWindow(); w != eng.canvas.Frame() {
		t.Fatalf("модальное над переходом: окно back %v, ждали весь кадр", w)
	}
	if got := eng.Stats().BufferBytes; got != 2*frontBytes {
		t.Fatalf("модальное над переходом: байт буферов %d, ждали %d", got, 2*frontBytes)
	}
	// Закрыли всё (исчезновение меню доигрывается): окно освобождается через
	// backWindowIdle кадров.
	md.modal = false
	eng.CloseModal(md)
	s.setOpen(false)
	eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimationsFor(eng, t0)
	eng.RenderOnce()
	widget.StepAnimationsFor(eng, t0.Add(300*time.Millisecond))
	eng.RenderOnce()
	eng.RenderOnce()
	for i := 0; i < 70; i++ {
		eng.Invalidate()
		eng.RenderOnce()
	}
	if got := eng.Stats().BufferBytes; got != frontBytes {
		t.Fatalf("после закрытия: байт буферов %d, front %d (окно %v)", got, frontBytes, eng.canvas.BackWindow())
	}
}

// Два буфера (SetSingleBuffer(false)): окно back — весь кадр и остаётся.
func TestSingleBuffer_OffKeepsTwoBuffers(t *testing.T) {
	testreset.Globals(t)
	eng := New(320, 240, 20)
	defer eng.Stop()
	eng.SetSingleBuffer(false)
	root, _ := busyScene(320, 240)
	eng.SetRoot(root)
	for i := 0; i < 3; i++ {
		eng.RenderOnce()
	}
	if w := eng.canvas.BackWindow(); w != eng.canvas.Frame() {
		t.Fatalf("два буфера: окно back %v", w)
	}
	if got, want := eng.Stats().BufferBytes, 2*uint64(len(eng.canvas.Front().Pix)); got != want {
		t.Fatalf("два буфера: байт буферов %d, ждали %d", got, want)
	}
	if !bytes.Equal(eng.canvas.Back().Pix, eng.canvas.Front().Pix) {
		t.Fatal("два буфера: back не равен front после кадра")
	}
}

// Снимок рекордера (призрак DockManager): Snapshot при записи оверлея
// отдаёт панель, нарисованную по записанным командам.
func TestSingleBuffer_RecorderSnapshot(t *testing.T) {
	testreset.Globals(t)
	eng := New(320, 240, 20)
	defer eng.Stop()
	root := widget.NewPanel(color.RGBA{R: 10, G: 20, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 320, 240))
	eng.SetRoot(root)
	eng.RenderOnce()
	rec := eng.rec
	rec.BeginFrame(eng.canvas, true, nil)
	widget.DrawTraced(root, rec)
	img := rec.Snapshot(image.Rect(10, 10, 50, 30))
	rec.EndFrame()
	if img == nil || img.Rect.Dx() != 40 || img.Rect.Dy() != 20 {
		t.Fatalf("снимок %v", img)
	}
	if got := img.RGBAAt(5, 5); got != (color.RGBA{R: 10, G: 20, B: 30, A: 255}) {
		t.Fatalf("пиксель снимка %v", got)
	}
}
