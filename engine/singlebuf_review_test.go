package engine

// singlebuf_review_test.go — случаи из ревью 4а.4 (ADR-0007): корректность
// (шаг перехода вне повреждения на прямом пути, оверлей под присмотром,
// рисующий за rest, переходы после освобождения окна, меню в покое с
// переносами, исчезающее меню под модальным с подсказкой, полный кадр со
// слоями после растра, чужие буферы с переходами, смена разрешения посреди
// перехода, два буфера после возврата из чужой памяти, паника при записи
// оверлея, последовательный путь со слоями и стеклом, хостовые попапы,
// след после перехода, снимок рекордера, подсказка на кромке тайла,
// акриловое меню с чтением за окном) и цена/память (гонки слоёв с ореолом и
// Stop посреди кадра, аллокации на кадр со слоями, статичное меню под
// присмотром, окно на последовательном пути, старый холст после
// SetResolution, бенчмарки мелкого частичного кадра и прямого пути).

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/output"
	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// corrPair — две сцены переходов: вторая настроена по mode ("two" — два
// буфера, "direct" — прямой путь, "" — одинаковые).
func corrPair(t *testing.T, m *theme.Manager, mode string, mk func() (*ovtOverlay, widget.Widget)) (a, b *ovtScene, p cmdPair) {
	t.Helper()
	build := func() *ovtScene {
		eng := New(320, 240, 60)
		t.Cleanup(eng.Stop)
		if err := eng.ApplyThemeProfile(m); err != nil {
			t.Fatal(err)
		}
		root := &ovtRoot{bg: ovtBG}
		root.SetBounds(image.Rect(0, 0, 320, 240))
		s := &ovtScene{eng: eng, root: root}
		var o *ovtOverlay
		var ov widget.Widget
		if mk != nil {
			o, ov = mk()
		} else {
			o = &ovtOverlay{}
			ov = o
		}
		o.SetBounds(image.Rect(40, 20, 120, 44))
		o.rect = image.Rect(40, 44, 200, 164)
		s.o, s.ov = o, ov
		root.AddChild(ov)
		eng.SetRoot(root)
		eng.SetOverlayTransitions(true)
		return s
	}
	a, b = build(), build()
	switch mode {
	case "two":
		b.eng.SetSingleBuffer(false)
	case "direct":
		b.eng.SetCommandPath(false)
	case "directtwo":
		b.eng.SetCommandPath(false)
		b.eng.SetSingleBuffer(false)
	}
	a.eng.RenderOnce()
	b.eng.RenderOnce()
	p = cmdPair{on: a.eng, off: b.eng, rootOn: a.root, rootOff: b.root}
	return a, b, p
}

func corrStep(a, b *ovtScene, at time.Time) {
	widget.StepAnimationsFor(a.eng, at)
	widget.StepAnimationsFor(b.eng, at)
}

func tileHas(fr output.Frame, pt image.Point) bool {
	for _, tl := range fr.Tiles {
		if pt.In(image.Rect(tl.X, tl.Y, tl.X+tl.W, tl.Y+tl.H)) {
			return true
		}
	}
	return false
}

// Линза 3/5: прямой путь, шаг перехода вне повреждения в кадре с другим
// повреждением. Смесь шага кладётся в окно back на месте покоя, но тайлы
// места не помечены тронутыми (Touch только по damage) — сброс окна их не
// сравнивает, и шаг не доходит до front. Сравнение с путём команд, где место
// отложенного слоя помечается (Touch(postRect)).
func TestSingleBufferReview_DirectPathStepWithUnrelatedDamage(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"direct", "two", "directtwo"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 0)
			a, b, p := corrPair(t, m, mode, nil)
			a.setOpen(true)
			b.setOpen(true)
			sbCompare(t, p, "меню открыто")
			t0 := time.Now()
			corrStep(a, b, t0)
			sbCompare(t, p, "шаг без повреждения")
			corrStep(a, b, t0.Add(50*time.Millisecond))
			a.eng.InvalidateRect(image.Rect(300, 220, 320, 240))
			b.eng.InvalidateRect(image.Rect(300, 220, 320, 240))
			fa, fb := sbCompare(t, p, "шаг 50% + повреждение в углу")
			if !tileHas(fa, image.Pt(120, 100)) {
				t.Errorf("путь команд: тайл меню не в кадре шага")
			}
			if !tileHas(fb, image.Pt(120, 100)) {
				t.Errorf("%s: тайл меню не в кадре шага", mode)
			}
			corrStep(a, b, t0.Add(200*time.Millisecond))
			sbCompare(t, p, "конец перехода")
		})
	}
}

// farOverlay — меню под присмотром, рисующее ещё и далеко за своими
// OverlayBounds (дальше запаса DefaultOverlayMargin).
type farOverlay struct {
	ovtOverlay
	far image.Rectangle
}

func (o *farOverlay) DrawOverlay(ctx widget.DrawContext) {
	o.ovtOverlay.DrawOverlay(ctx)
	ctx.FillRect(o.far.Min.X, o.far.Min.Y, o.far.Dx(), o.far.Dy(), color.RGBA{R: 250, G: 20, B: 20, A: 255})
}

// Линза 3: оверлей под присмотром рисует за пределами места покоя. Меню
// открывается повреждением, чьё объединение накрывает дальний прямоугольник,
// а тайл под ним хэш-дифф пропускает. Два буфера: дальний прямоугольник
// ложится в back, тайл не тронут — сброс его не сравнивает, front без него
// (в 4а.3 дифф шёл по повреждению и показывал его); прямой путь помечает
// всё повреждение. Один буфер: окно back уже места — дальний прямоугольник
// не рисуется вовсе (ADR-0007 это оговаривает).
func TestSingleBufferReview_ManagedOverlayDrawsBeyondRest(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"two", "single"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 0)
			far := image.Rect(260, 200, 300, 230) // тайл (4,3), вне rest (16,20,224,188)
			a, b, p := corrPair(t, m, "direct", func() (*ovtOverlay, widget.Widget) {
				w := &farOverlay{far: far}
				return &w.ovtOverlay, w
			})
			if mode == "two" {
				a.eng.SetSingleBuffer(false)
				b.eng.SetSingleBuffer(false)
				a.eng.RenderOnce()
				b.eng.RenderOnce()
			}
			open := func(s *ovtScene) {
				s.o.open = true
				s.o.Invalidate()
				// Повреждение: место меню и две далёкие точки, объединение которых
				// накрывает дальний прямоугольник, не трогая его тайл напрямую.
				s.eng.InvalidateRect(image.Rect(16, 20, 224, 188))
				s.eng.InvalidateRect(image.Rect(200, 236, 204, 240)) // тайл (3,3)
				s.eng.InvalidateRect(image.Rect(310, 150, 314, 154)) // тайл (4,2)
			}
			open(a)
			open(b)
			sbCompare(t, p, "меню открыто частичным кадром")
			t0 := time.Now()
			corrStep(a, b, t0)
			corrStep(a, b, t0.Add(200*time.Millisecond))
			sbCompare(t, p, "конец перехода")
			// Позже тайл дальнего прямоугольника трогает другая правка: при двух
			// буферах в back под ним лежит невыданный прямоугольник — он утечёт.
			a.eng.InvalidateRect(image.Rect(300, 196, 304, 200))
			b.eng.InvalidateRect(image.Rect(300, 196, 304, 200))
			a.eng.renderFrame()
			b.eng.renderFrame()
			pt := image.Pt(270, 210)
			ca, cb := a.eng.canvas.Front().RGBAAt(pt.X, pt.Y), b.eng.canvas.Front().RGBAAt(pt.X, pt.Y)
			t.Logf("%s: пиксель дальнего прямоугольника после правки рядом: команды %v, прямой %v", mode, ca, cb)
			if mode == "two" && ca != cb {
				t.Errorf("два буфера: команды %v, прямой %v", ca, cb)
			}
		})
	}
}

// Линза 1: длинная последовательность с переходами — открытие, шаги,
// закрытие, 70 кадров без слоёв (окно back освобождается), повторное
// открытие (окно заводится заново поверх свежего front), подсказка над меню,
// частичные правки под меню.
func TestSingleBufferReview_TransitionsAfterWindowRelease(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"two", "direct"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 6)
			a, b, p := corrPair(t, m, mode, nil)
			for _, s := range []*ovtScene{a, b} {
				tip := &tipped{}
				tip.SetBounds(image.Rect(60, 180, 120, 220))
				s.root.AddChild(tip)
				s.eng.SetTooltipsEnabled(true)
				s.eng.SetTooltipDelay(time.Hour)
			}
			run := func(tag string) {
				a.setOpen(true)
				b.setOpen(true)
				sbCompare(t, p, tag+": открыто")
				t0 := time.Now()
				for ms := 0; ms <= 120; ms += 40 {
					corrStep(a, b, t0.Add(time.Duration(ms)*time.Millisecond))
					sbCompare(t, p, tag+": шаг")
				}
				sbCompare(t, p, tag+": покой")
				// Правка под меню (частичный кадр, место перерисовывается целиком).
				a.eng.InvalidateRect(image.Rect(100, 100, 110, 110))
				b.eng.InvalidateRect(image.Rect(100, 100, 110, 110))
				sbCompare(t, p, tag+": правка под меню")
				a.setOpen(false)
				b.setOpen(false)
				sbCompare(t, p, tag+": закрытие")
				t1 := time.Now()
				for ms := 0; ms <= 120; ms += 40 {
					corrStep(a, b, t1.Add(time.Duration(ms)*time.Millisecond))
					sbCompare(t, p, tag+": шаг исчезновения")
				}
				sbCompare(t, p, tag+": исчезло")
			}
			run("первый раз")
			for i := 0; i < 70; i++ {
				a.eng.InvalidateRect(image.Rect(i, 230, i+1, 231))
				b.eng.InvalidateRect(image.Rect(i, 230, i+1, 231))
				sbCompare(t, p, "простой")
			}
			// Мелкие частичные кадры идут «всем выбранным одним воспроизведением»
			// в окно back над областью растра — окно в деле каждый кадр и не
			// освобождается; освобождение проверяет singlebuf_test.go.
			t.Logf("окно back после простоя: %v", a.eng.canvas.BackWindow())
			run("после освобождения")
			// Подсказка над меню в покое, затем её исчезновение.
			a.setOpen(true)
			b.setOpen(true)
			sbCompare(t, p, "снова открыто")
			t2 := time.Now()
			corrStep(a, b, t2)
			corrStep(a, b, t2.Add(300*time.Millisecond))
			sbCompare(t, p, "в покое")
			for _, s := range []*ovtScene{a, b} {
				s.eng.SendMouseMove(90, 200)
				s.eng.SetTooltipDelay(0)
				s.eng.InvalidateRect(image.Rect(0, 0, 8, 8))
			}
			sbCompare(t, p, "подсказка над меню")
			sbCompare(t, p, "подсказка, без изменений")
			for _, s := range []*ovtScene{a, b} {
				s.eng.SetTooltipDelay(time.Hour)
				s.eng.SendMouseMove(5, 5)
			}
			sbCompare(t, p, "подсказка исчезла")
			sbCompare(t, p, "после исчезновения")
		})
	}
}

// Линза 2: меню под присмотром в покое и перенос панели в другом месте кадра
// (частичный кадр с переносом: все выбранные тайлы принуждаются, окно back
// над местом меню заполняется из уже перенесённого front).
func TestSingleBufferReview_MenuAtRestWithMoves(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"two", "direct"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 0)
			a, b, p := corrPair(t, m, mode, nil)
			var panes []*widget.Panel
			for _, s := range []*ovtScene{a, b} {
				win := widget.NewPanel(color.RGBA{R: 200, G: 200, B: 50, A: 255})
				win.ShowHeader = false
				win.SetBounds(image.Rect(210, 10, 300, 60))
				s.root.AddChild(win)
				panes = append(panes, win)
			}
			a.setOpen(true)
			b.setOpen(true)
			sbCompare(t, p, "открыто")
			t0 := time.Now()
			corrStep(a, b, t0)
			corrStep(a, b, t0.Add(300*time.Millisecond))
			sbCompare(t, p, "в покое")
			for step := 0; step < 3; step++ {
				for _, w := range panes {
					src := w.Bounds()
					dst := src.Add(image.Pt(0, 40))
					w.SetBounds(dst)
					widget.NotifyMove(src, dst)
				}
				fa, _ := sbCompare(t, p, "перенос")
				tileDataMatchesFront(t, a.eng, fa, "перенос")
				sbCompare(t, p, "после переноса")
			}
			// Перенос, задевающий место меню: окно back и front переносятся
			// согласованно.
			for _, w := range panes {
				src := w.Bounds()
				dst := image.Rect(150, 120, 240, 170)
				w.SetBounds(dst)
				widget.NotifyMove(src, dst)
			}
			sbCompare(t, p, "перенос на место меню")
			sbCompare(t, p, "после")
		})
	}
}

// Линза 3: закрывающееся меню (postClosed), модальное окно над ним и
// подсказка над модальным — порядок слоёв после точки переключения.
func TestSingleBufferReview_ClosingUnderModalWithTooltip(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"two", "direct"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 0)
			a, b, p := corrPair(t, m, mode, nil)
			for _, s := range []*ovtScene{a, b} {
				s.eng.SetTooltipsEnabled(true)
				s.eng.SetTooltipDelay(time.Hour)
			}
			a.setOpen(true)
			b.setOpen(true)
			sbCompare(t, p, "открыто")
			t0 := time.Now()
			corrStep(a, b, t0)
			corrStep(a, b, t0.Add(300*time.Millisecond))
			sbCompare(t, p, "в покое")
			a.setOpen(false)
			b.setOpen(false)
			sbCompare(t, p, "закрытие началось")
			t1 := time.Now()
			corrStep(a, b, t1)
			corrStep(a, b, t1.Add(30*time.Millisecond))
			sbCompare(t, p, "шаг исчезновения")
			var ms []*dimModal
			for _, s := range []*ovtScene{a, b} {
				md := &dimModal{modal: true}
				md.SetBounds(image.Rect(60, 60, 260, 200))
				s.eng.ShowModal(md)
				ms = append(ms, md)
			}
			sbCompare(t, p, "модальное над исчезающим")
			for _, s := range []*ovtScene{a, b} {
				s.eng.SendMouseMove(100, 100)
				s.eng.SetTooltipDelay(0)
			}
			ms[0].SetToolTip("над модальным")
			ms[1].SetToolTip("над модальным")
			corrStep(a, b, t1.Add(60*time.Millisecond))
			sbCompare(t, p, "подсказка над модальным, шаг")
			sbCompare(t, p, "без изменений")
			corrStep(a, b, t1.Add(300*time.Millisecond))
			sbCompare(t, p, "исчезновение кончилось")
			for i, s := range []*ovtScene{a, b} {
				s.eng.SetTooltipDelay(time.Hour)
				s.eng.SendMouseMove(5, 5)
				ms[i].modal = false
				s.eng.CloseModal(ms[i])
			}
			sbCompare(t, p, "всё закрыто")
			sbCompare(t, p, "без изменений")
		})
	}
}

// Линза 8: полный кадр (потеря кадра потребителем) при отложенных слоях —
// все тайлы берутся из front после сброса окна.
func TestSingleBufferReview_FullFrameWithPostLayers(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	a, b, p := corrPair(t, m, "two", nil)
	for _, s := range []*ovtScene{a, b} {
		s.eng.SetTooltipsEnabled(true)
		s.eng.SetTooltipDelay(time.Hour)
	}
	a.setOpen(true)
	b.setOpen(true)
	sbCompare(t, p, "открыто")
	t0 := time.Now()
	corrStep(a, b, t0)
	corrStep(a, b, t0.Add(40*time.Millisecond))
	var ms []*dimModal
	for _, s := range []*ovtScene{a, b} {
		md := &dimModal{modal: true}
		md.SetBounds(image.Rect(60, 60, 260, 200))
		md.SetToolTip("полный")
		s.eng.ShowModal(md)
		ms = append(ms, md)
		s.eng.SendMouseMove(100, 100)
		s.eng.SetTooltipDelay(0)
	}
	sbCompare(t, p, "модальное и подсказка над шагом")
	a.eng.fullNext.Store(true)
	b.eng.fullNext.Store(true)
	corrStep(a, b, t0.Add(60*time.Millisecond))
	fa, fb := sbCompare(t, p, "полный кадр")
	if n := gridTiles(a.eng); len(fa.Tiles) != n || len(fb.Tiles) != n {
		t.Errorf("полный кадр: тайлов %d/%d, ждали %d", len(fa.Tiles), len(fb.Tiles), n)
	}
	tileDataMatchesFront(t, a.eng, fa, "полный кадр")
	sbCompare(t, p, "после полного")
}

// Линза 6: чужие буферы вывода и слои после растра (переходы, модальное,
// подсказка): путь команд против прямого.
func TestSingleBufferReview_SurfacesWithTransitions(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	a, b, p := corrPair(t, m, "direct", nil)
	const w, h = 320, 240
	bufs := map[*Engine][][]byte{}
	for _, s := range []*ovtScene{a, b} {
		bb := [][]byte{make([]byte, w*h*4), make([]byte, w*h*4)}
		if err := s.eng.SetSurfaces(bb, w*4, FormatRGBA); err != nil {
			t.Fatal(err)
		}
		bufs[s.eng] = bb
		s.eng.SetTooltipsEnabled(true)
		s.eng.SetTooltipDelay(time.Hour)
	}
	check := func(name string) {
		t.Helper()
		fa, fb := sbCompare(t, p, name)
		if fa.Surface == 0 && fb.Surface == 0 {
			return
		}
		if fa.Surface == 0 || fb.Surface == 0 {
			t.Fatalf("%s: кадр не в буфере потребителя (%d, %d)", name, fa.Surface, fb.Surface)
		}
		if string(bufs[a.eng][fa.Surface-1]) != string(bufs[b.eng][fb.Surface-1]) {
			t.Errorf("%s: буферы потребителя расходятся", name)
		}
		a.eng.ReleaseSurface(fa.Surface)
		b.eng.ReleaseSurface(fb.Surface)
	}
	check("полный")
	a.setOpen(true)
	b.setOpen(true)
	check("открыто")
	t0 := time.Now()
	for ms := 0; ms <= 120; ms += 40 {
		corrStep(a, b, t0.Add(time.Duration(ms)*time.Millisecond))
		check("шаг")
	}
	check("покой")
	a.eng.InvalidateRect(image.Rect(100, 100, 110, 110))
	b.eng.InvalidateRect(image.Rect(100, 100, 110, 110))
	check("правка под меню")
	var ms []*dimModal
	for _, s := range []*ovtScene{a, b} {
		md := &dimModal{modal: true}
		md.SetBounds(image.Rect(60, 60, 260, 200))
		s.eng.ShowModal(md)
		ms = append(ms, md)
	}
	check("модальное")
	check("модальное, без изменений")
	a.eng.InvalidateRect(image.Rect(10, 200, 20, 210))
	b.eng.InvalidateRect(image.Rect(10, 200, 20, 210))
	check("правка под затемнением")
	for i, s := range []*ovtScene{a, b} {
		ms[i].modal = false
		s.eng.CloseModal(ms[i])
	}
	check("модальное закрыто")
	a.setOpen(false)
	b.setOpen(false)
	check("закрытие")
	t1 := time.Now()
	for ms := 0; ms <= 120; ms += 40 {
		corrStep(a, b, t1.Add(time.Duration(ms)*time.Millisecond))
		check("шаг исчезновения")
	}
	check("исчезло")
}

// Линза 1: смена разрешения посреди перехода и посреди показа подсказки —
// новый холст, окно back с нуля.
func TestSingleBufferReview_ResolutionChangeMidTransition(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"two", "direct"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 0)
			a, b, p := corrPair(t, m, mode, nil)
			a.setOpen(true)
			b.setOpen(true)
			sbCompare(t, p, "открыто")
			t0 := time.Now()
			corrStep(a, b, t0)
			corrStep(a, b, t0.Add(40*time.Millisecond))
			sbCompare(t, p, "шаг")
			a.eng.SetResolution(400, 300)
			b.eng.SetResolution(400, 300)
			sbCompare(t, p, "новое разрешение")
			sbCompare(t, p, "без изменений")
			a.eng.InvalidateRect(image.Rect(100, 100, 110, 110))
			b.eng.InvalidateRect(image.Rect(100, 100, 110, 110))
			sbCompare(t, p, "правка под меню")
			a.setOpen(false)
			b.setOpen(false)
			sbCompare(t, p, "закрытие")
			t1 := time.Now()
			corrStep(a, b, t1)
			corrStep(a, b, t1.Add(40*time.Millisecond))
			sbCompare(t, p, "шаг исчезновения")
			a.eng.SetScale(1.25)
			b.eng.SetScale(1.25)
			sbCompare(t, p, "масштаб 1,25")
			sbCompare(t, p, "без изменений")
		})
	}
}

// Линза 1/6: два буфера, меню под присмотром в покое; уход в чужие буферы
// вывода и возврат к своему (UseOwnBack: backClean=false) — SetBackWindow
// под место меню сжимает окно back до места, хотя режим двух буферов обещает
// окно размером с кадр; воркеры копируют тайлы в сжатое окно, тайлы вне его
// теряются. Сравнение с одним буфером.
func TestSingleBufferReview_TwoBuffersAfterSurfaceReturn(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	a, b, p := corrPair(t, m, "two", nil)
	var panes []*widget.Panel
	for _, s := range []*ovtScene{a, b} {
		win := widget.NewPanel(color.RGBA{R: 200, G: 200, B: 50, A: 255})
		win.ShowHeader = false
		win.SetBounds(image.Rect(250, 200, 300, 230))
		s.root.AddChild(win)
		panes = append(panes, win)
	}
	a.setOpen(true)
	b.setOpen(true)
	sbCompare(t, p, "открыто")
	t0 := time.Now()
	corrStep(a, b, t0)
	corrStep(a, b, t0.Add(300*time.Millisecond))
	sbCompare(t, p, "в покое")
	const w, h = 320, 240
	for _, s := range []*ovtScene{a, b} {
		bb := [][]byte{make([]byte, w*h*4), make([]byte, w*h*4)}
		if err := s.eng.SetSurfaces(bb, w*4, FormatRGBA); err != nil {
			t.Fatal(err)
		}
	}
	fa, fb := sbCompare(t, p, "в чужих буферах")
	a.eng.ReleaseSurface(fa.Surface)
	b.eng.ReleaseSurface(fb.Surface)
	for _, s := range []*ovtScene{a, b} {
		if err := s.eng.SetSurfaces(nil, 0, FormatRGBA); err != nil {
			t.Fatal(err)
		}
	}
	sbCompare(t, p, "назад к своему буферу")
	if bw := b.eng.canvas.BackWindow(); bw != b.eng.canvas.Frame() {
		t.Errorf("два буфера: окно back %v, ждали весь кадр %v", bw, b.eng.canvas.Frame())
	}
	for i, w := range panes {
		w.SetBounds(image.Rect(260, 190, 310, 220)) // правка вне сжатого окна
		// Широкое повреждение: кадр идёт по тайлам в пуле (≥ 5 тайлов), и
		// воркеры копируют тайлы в окно back — тайлы вне сжатого окна теряются.
		[]*Engine{a.eng, b.eng}[i].InvalidateRect(image.Rect(200, 0, 320, 240))
	}
	sbCompare(t, p, "правка вне окна")
	sbCompare(t, p, "без изменений")
	if got := b.eng.canvas.Front().RGBAAt(300, 200); got != (color.RGBA{R: 200, G: 200, B: 50, A: 255}) {
		t.Errorf("два буфера: панель вне сжатого окна не дошла до front: %v", got)
	}
}

// Линза 7: паника в DrawOverlay при записи (FailureIsolate): Abort рекордера,
// следующий кадр — обычный. Путь команд против прямого.
type boomPopper struct {
	popper
	boom int
}

func (p *boomPopper) DrawOverlay(ctx widget.DrawContext) {
	if p.boom > 0 {
		p.boom--
		panic("boomPopper: overlay")
	}
	p.popper.DrawOverlay(ctx)
}

func TestSingleBufferReview_OverlayPanicDuringRecord(t *testing.T) {
	testreset.Globals(t)
	var pops []*boomPopper
	p := directPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		eng.SetFailurePolicy(FailureIsolate)
		root, _ := busyScene(640, 480)
		pop := &boomPopper{}
		pop.SetBounds(image.Rect(200, 100, 320, 130))
		root.AddChild(pop)
		pops = append(pops, pop)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	sbCompare(t, p, "полный кадр")
	// Учёт рисующихся включён заранее (общее состояние процесса): виновник
	// назван с первой паники у обоих движков одинаково.
	release := widget.EnableDrawTrace()
	defer release()
	for _, pop := range pops {
		pop.open = true
		pop.boom = 1
		pop.Invalidate()
	}
	p.on.renderFrame()
	p.off.renderFrame()
	if !widget.IsBroken(pops[0]) || !widget.IsBroken(pops[1]) {
		t.Fatalf("виновник не помечен: %v %v", widget.IsBroken(pops[0]), widget.IsBroken(pops[1]))
	}
	sbCompare(t, p, "кадр после паники")
	sbCompare(t, p, "без изменений")
	widget.ClearBroken(pops[0])
	widget.ClearBroken(pops[1])
	p.on.Invalidate()
	p.off.Invalidate()
	sbCompare(t, p, "после ClearBroken")
}

// Линза 2: последовательный путь (тайлы выключены) — «всё выбранное одним
// воспроизведением» в окно back над областью растра и областями чтения слоёв,
// при отложенном меню под присмотром: окно растёт в одном кадре.
func TestSingleBufferReview_SerialWholeWithPostAndBlur(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"two", "direct"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 0)
			a, b, p := corrPair(t, m, mode, nil)
			var labels [][]*widget.Label
			for _, s := range []*ovtScene{a, b} {
				s.eng.SetTiledRaster(false)
				g := newGlass(image.Rect(180, 120, 310, 230), 10, color.RGBA{R: 200, G: 200, B: 220, A: 110}, "стекло")
				s.root.AddChild(g)
				var ls []*widget.Label
				for i := 0; i < 3; i++ {
					l := widget.NewLabel("метка", color.RGBA{R: 250, G: 250, B: 250, A: 255})
					l.SetBounds(image.Rect(200, 130+i*30, 300, 150+i*30))
					s.root.AddChild(l)
					ls = append(ls, l)
				}
				labels = append(labels, ls)
			}
			sbCompare(t, p, "полный")
			a.setOpen(true)
			b.setOpen(true)
			sbCompare(t, p, "открыто")
			t0 := time.Now()
			corrStep(a, b, t0)
			sbCompare(t, p, "шаг")
			for _, ls := range labels {
				ls[0].SetText("под стеклом")
			}
			sbCompare(t, p, "правка под стеклом при шаге")
			corrStep(a, b, t0.Add(50*time.Millisecond))
			for _, ls := range labels {
				ls[1].SetText("ещё правка")
			}
			sbCompare(t, p, "шаг + правка под стеклом")
			corrStep(a, b, t0.Add(300*time.Millisecond))
			sbCompare(t, p, "покой")
			a.eng.InvalidateRect(image.Rect(0, 200, 10, 210))
			b.eng.InvalidateRect(image.Rect(0, 200, 10, 210))
			sbCompare(t, p, "правка вне всего")
			sbCompare(t, p, "без изменений")
		})
	}
}

// Линза 3: хост попапов (SetPopupSink) при переходах: оверлей с границами
// уходит хосту, оверлей без границ рисуется в холсте.
func TestSingleBufferReview_HostedPopupsWithTransitions(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"two", "direct"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 0)
			a, b, p := corrPair(t, m, mode, nil)
			var pops []*popper
			for _, s := range []*ovtScene{a, b} {
				s.eng.SetPopupSink(func([]PopupFrame) {})
				pop := &popper{}
				pop.SetBounds(image.Rect(150, 180, 200, 200))
				s.root.AddChild(pop)
				pops = append(pops, pop)
			}
			sbCompare(t, p, "полный")
			a.setOpen(true)
			b.setOpen(true)
			for _, pop := range pops {
				pop.open = true
				pop.Invalidate()
			}
			sbCompare(t, p, "меню у хоста, плашка в холсте")
			sbCompare(t, p, "без изменений")
			a.eng.InvalidateRect(image.Rect(100, 100, 110, 110))
			b.eng.InvalidateRect(image.Rect(100, 100, 110, 110))
			sbCompare(t, p, "правка")
			a.setOpen(false)
			b.setOpen(false)
			sbCompare(t, p, "меню закрыто")
			for _, pop := range pops {
				pop.open = false
				pop.Invalidate()
			}
			sbCompare(t, p, "плашка закрыта")
			sbCompare(t, p, "без изменений")
		})
	}
}

// Линза 4: после исчезновения меню — кадр без заявленного повреждения и
// кадр с правкой в стороне: следов слоя нет, хэш-дифф снова пропускает.
func TestSingleBufferReview_NoTraceAfterTransitionEnds(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 4)
	a, b, p := corrPair(t, m, "direct", nil)
	a.setOpen(true)
	b.setOpen(true)
	sbCompare(t, p, "открыто")
	t0 := time.Now()
	corrStep(a, b, t0)
	corrStep(a, b, t0.Add(300*time.Millisecond))
	sbCompare(t, p, "покой")
	a.setOpen(false)
	b.setOpen(false)
	sbCompare(t, p, "закрытие")
	t1 := time.Now()
	corrStep(a, b, t1)
	sbCompare(t, p, "шаг")
	corrStep(a, b, t1.Add(50*time.Millisecond))
	sbCompare(t, p, "шаг 50%")
	corrStep(a, b, t1.Add(300*time.Millisecond))
	sbCompare(t, p, "конец")
	// Кадр без заявленного повреждения (RenderOnce в on-demand: дерево
	// перезаписывается, хэш-дифф убирает растр).
	ia, ib := a.eng.RenderOnce(), b.eng.RenderOnce()
	if !sameImage(ia, ib) {
		t.Errorf("кадр без повреждения после исчезновения расходится")
	}
	before := a.eng.Stats()
	fr := a.eng.renderFrame()
	after := a.eng.Stats()
	if len(fr.Tiles) != 0 || after.TilesSkipped-before.TilesSkipped != uint64(gridTiles(a.eng)) {
		t.Errorf("после исчезновения кадр без изменений: тайлов %d, пропущено %d из %d", len(fr.Tiles), after.TilesSkipped-before.TilesSkipped, gridTiles(a.eng))
	}
	a.eng.InvalidateRect(image.Rect(300, 220, 320, 240))
	b.eng.InvalidateRect(image.Rect(300, 220, 320, 240))
	sbCompare(t, p, "правка в стороне")
}

// Снимок рекордера: оверлей, рисующий снимок области кадра (как призрак
// DockManager), на пути команд против прямого.
type snapOverlay struct {
	widget.Base
	open bool
}

func (s *snapOverlay) Draw(ctx widget.DrawContext) {
	b := s.Bounds()
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), color.RGBA{R: 200, G: 30, B: 30, A: 255})
	ctx.DrawText("снимок", b.Min.X+4, b.Min.Y+4, color.RGBA{A: 255})
}
func (s *snapOverlay) HasOverlay() bool { return s.open }
func (s *snapOverlay) DrawOverlay(ctx widget.DrawContext) {
	sn, ok := ctx.(widget.Snapshotter)
	if !ok {
		return
	}
	b := s.Bounds()
	img := sn.Snapshot(image.Rect(b.Min.X-10, b.Min.Y-10, b.Max.X+10, b.Max.Y+10))
	if img == nil {
		return
	}
	ctx.DrawImage(img, b.Min.X+100, b.Min.Y+120)
}

func TestSingleBufferReview_RecorderSnapshotOverlay(t *testing.T) {
	testreset.Globals(t)
	var ws []*snapOverlay
	p := directPair(func() (*Engine, widget.Widget) {
		eng := New(640, 480, 20)
		root, _ := busyScene(640, 480)
		w := &snapOverlay{}
		w.SetBounds(image.Rect(100, 80, 220, 120))
		root.AddChild(w)
		ws = append(ws, w)
		eng.SetRoot(root)
		return eng, root
	})
	defer p.closeAll()
	sbCompare(t, p, "полный")
	for i, w := range ws {
		w.open = true
		w.Invalidate()
		[]*Engine{p.on, p.off}[i].InvalidateRect(image.Rect(180, 180, 360, 260))
	}
	sbCompare(t, p, "снимок в оверлее")
	sbCompare(t, p, "без изменений")
	p.on.InvalidateRect(image.Rect(0, 0, 20, 20))
	p.off.InvalidateRect(image.Rect(0, 0, 20, 20))
	sbCompare(t, p, "правка в стороне")
}

// Плашка подсказки у границы тайла при отложенных слоях: окно back в один
// буфер — по месту плашки; то, что текст рисует за плашкой, в окно не
// попадает.
func TestSingleBufferReview_TooltipAtTileEdgeUnderTransitions(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"direct", "two"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 0)
			a, b, p := corrPair(t, m, mode, nil)
			for _, s := range []*ovtScene{a, b} {
				tip := &tipped{}
				tip.SetBounds(image.Rect(230, 0, 320, 60))
				s.root.AddChild(tip)
				s.eng.SetTooltipsEnabled(true)
				s.eng.SetTooltipDelay(time.Hour)
			}
			a.setOpen(true)
			b.setOpen(true)
			sbCompare(t, p, "открыто")
			t0 := time.Now()
			corrStep(a, b, t0)
			corrStep(a, b, t0.Add(300*time.Millisecond))
			sbCompare(t, p, "покой")
			for _, s := range []*ovtScene{a, b} {
				s.eng.SendMouseMove(240, 21) // плашка y в [41,64): нижняя кромка на границе тайла
				s.eng.SetTooltipDelay(0)
				s.eng.InvalidateRect(image.Rect(0, 230, 8, 240))
			}
			sbCompare(t, p, "подсказка у границы тайла")
			sbCompare(t, p, "без изменений")
			for _, s := range []*ovtScene{a, b} {
				s.eng.SetTooltipDelay(time.Hour)
				s.eng.SendMouseMove(5, 5)
			}
			sbCompare(t, p, "подсказка исчезла")
			a.eng.InvalidateRect(image.Rect(300, 70, 310, 80))
			b.eng.InvalidateRect(image.Rect(300, 70, 310, 80))
			sbCompare(t, p, "правка под бывшей плашкой")
		})
	}
}

// acrylicOverlay — меню под присмотром с размытой подложкой радиусом больше
// запаса места покоя: размытие читает кадр за пределами rest.
type acrylicOverlay struct {
	ovtOverlay
	radius int
}

func (o *acrylicOverlay) DrawOverlay(ctx widget.DrawContext) {
	o.draws++
	if bd, ok := ctx.(widget.BackdropDrawer); ok {
		bd.BlurBehind(o.rect, o.radius, color.RGBA{R: 240, G: 240, B: 250, A: 120})
	}
	ctx.DrawRoundBorder(o.rect.Min.X, o.rect.Min.Y, o.rect.Dx(), o.rect.Dy(), 4, color.RGBA{A: 255})
}

// Линза 3/1: акриловое меню под присмотром переходов с радиусом размытия
// больше OverlayMargin: на пути команд с одним буфером оно рисуется в окно
// back по месту покоя, и BlurBehind читает подложку только в пределах окна —
// у кромок окна размытие считается по обрезанным данным. Сравнение с двумя
// буферами (окно — весь кадр) и с прямым путём.
func TestSingleBufferReview_AcrylicMenuReadsBeyondWindow(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, mode := range []string{"two", "direct"} {
		t.Run(mode, func(t *testing.T) {
			m := ovtManager(t, 100*time.Millisecond, 0)
			a, b, p := corrPair(t, m, mode, func() (*ovtOverlay, widget.Widget) {
				w := &acrylicOverlay{radius: 40}
				return &w.ovtOverlay, w
			})
			for _, s := range []*ovtScene{a, b} {
				// Неоднородная подложка вокруг меню: размытие у кромки заметно.
				for i := 0; i < 6; i++ {
					pn := widget.NewPanel(color.RGBA{R: uint8(40 * i), G: 200, B: uint8(255 - 40*i), A: 255})
					pn.ShowHeader = false
					pn.SetBounds(image.Rect(i*50, 160+(i%2)*40, i*50+50, 240))
					s.root.AddChild(pn)
				}
			}
			sbCompare(t, p, "полный")
			a.setOpen(true)
			b.setOpen(true)
			sbCompare(t, p, "открыто")
			t0 := time.Now()
			corrStep(a, b, t0)
			corrStep(a, b, t0.Add(300*time.Millisecond))
			sbCompare(t, p, "покой")
			a.eng.InvalidateRect(image.Rect(100, 100, 110, 110))
			b.eng.InvalidateRect(image.Rect(100, 100, 110, 110))
			sbCompare(t, p, "правка под меню")
		})
	}
}

// reviewBytesPer — средние байты аллокаций на вызов fn.
func reviewBytesPer(n int, fn func()) float64 {
	_, b := reviewAllocs(n, fn)
	return b
}

// reviewAllocs — средние аллокации и байты на вызов fn при текущем
// GOMAXPROCS (testing.AllocsPerRun ставит GOMAXPROCS=1 и уводит кадр на
// последовательный путь — здесь это не годится).
func reviewAllocs(n int, fn func()) (allocs, bytes float64) {
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	for i := 0; i < n; i++ {
		fn()
	}
	runtime.ReadMemStats(&b)
	return float64(b.Mallocs-a.Mallocs) / float64(n), float64(b.TotalAlloc-a.TotalAlloc) / float64(n)
}

// reviewSceneWithLayers — занятая сцена со стеклом, меню под присмотром
// переходов поверх стекла и виджетом с подсказкой.
func reviewSceneWithLayers(t *testing.T, transitions bool) (*Engine, *widget.Panel, *ovtOverlay, []*widget.Label) {
	t.Helper()
	eng := New(640, 480, 20)
	t.Cleanup(eng.Stop)
	if transitions {
		if err := eng.ApplyThemeProfile(ovtManager(t, 100*time.Millisecond, 8)); err != nil {
			t.Fatal(err)
		}
	}
	root, ls := busyScene(640, 480)
	root.AddChild(newGlass(image.Rect(137, 101, 537, 331), 12, color.RGBA{R: 200, G: 200, B: 220, A: 110}, "стекло"))
	o := &ovtOverlay{}
	o.SetBounds(image.Rect(200, 60, 280, 84))
	o.rect = image.Rect(200, 84, 420, 260) // над стеклом, границы не по сетке тайлов
	root.AddChild(o)
	tip := &tipped{}
	tip.SetBounds(image.Rect(300, 350, 400, 390))
	root.AddChild(tip)
	eng.SetRoot(root)
	eng.SetOverlayTransitions(transitions)
	eng.SetTooltipsEnabled(true)
	eng.SetTooltipDelay(time.Hour)
	return eng, root, o, ls
}

// Линза 1. Воркеры пишут front (SyncTileFrom) и окно back (CopyTo) под
// местом отложенных слоёв, окна с ореолом читают front вне области растра;
// частичные кадры с повреждением на кромках тайлов рядом с местом меню,
// шаги переходов, подсказка над переходом. Запускать с -race при
// GOMAXPROCS 2 и по умолчанию.
func TestSingleBufferReview_RaceLayersHaloPartial(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, procs := range []int{2, runtime.GOMAXPROCS(0)} {
		t.Run(fmt.Sprintf("GOMAXPROCS=%d", procs), func(t *testing.T) {
			prev := runtime.GOMAXPROCS(procs)
			defer runtime.GOMAXPROCS(prev)
			eng, _, o, ls := reviewSceneWithLayers(t, true)
			eng.SetRenderOnDemand(true)
			eng.renderFrame()
			o.open = true
			eng.Invalidate()
			eng.renderFrame() // кадр начала перехода: меню уходит после растра
			t0 := time.Now()
			damages := []image.Rectangle{
				image.Rect(120, 90, 210, 130),  // кромка стекла и меню, через границу тайла 128/192
				image.Rect(400, 250, 470, 300), // правый нижний угол меню и стекло
				image.Rect(60, 300, 140, 340),  // только стекло
				image.Rect(600, 440, 640, 480), // угол кадра, вдали
			}
			for i := 0; i < 40; i++ {
				widget.StepAnimationsFor(eng, t0.Add(time.Duration(i*5)*time.Millisecond))
				ls[i%len(ls)].SetText(fmt.Sprintf("шаг %d", i))
				eng.InvalidateRect(damages[i%len(damages)])
				if i == 10 {
					eng.SendMouseMove(350, 370)
					eng.SetTooltipDelay(0)
				}
				if i == 25 {
					eng.SetTooltipDelay(time.Hour)
					eng.SendMouseMove(5, 5)
				}
				if i == 30 {
					o.open = false
					eng.Invalidate()
				}
				eng.renderFrame()
			}
			// Полный кадр и кадр без повреждения — все тайлы, ореолы без префилла.
			eng.Invalidate()
			eng.renderFrame()
			eng.SetRenderOnDemand(false)
			eng.renderFrame()
		})
	}
}

// Линза 1. Stop во время кадра с другой горутины: tr обнуляется под frameMu.
func TestSingleBufferReview_RaceStopDuringRenderOnce(t *testing.T) {
	testreset.Globals(t)
	eng := New(640, 480, 20)
	root, _ := busyScene(640, 480)
	root.AddChild(newGlass(image.Rect(137, 101, 537, 331), 12, color.RGBA{R: 200, G: 200, B: 220, A: 110}, "стекло"))
	eng.SetRoot(root)
	eng.RenderOnce()
	var done atomic.Bool
	fin := make(chan struct{})
	go func() {
		defer close(fin)
		for i := 0; i < 200 && !done.Load(); i++ {
			eng.Invalidate()
			eng.RenderOnce()
		}
	}()
	time.Sleep(3 * time.Millisecond)
	eng.Stop()
	done.Store(true)
	<-fin
}

// Линза 2. Аллокации на кадр: подсказка командами, статичное меню под
// присмотром переходов (окно back над его местом каждый кадр), модальное над
// переходом (окно размером с кадр каждый кадр), кадр без слоёв.
func TestSingleBufferReview_AllocsPerFrameWithLayers(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	far := image.Rect(560, 420, 620, 470)
	frame := func(eng *Engine) func() {
		return func() {
			eng.InvalidateRect(far)
			eng.renderFrame()
		}
	}
	measure := func(name string, eng *Engine, limit float64) {
		t.Helper()
		for _, procs := range []int{runtime.GOMAXPROCS(0), 1} {
			prev := runtime.GOMAXPROCS(procs)
			for i := 0; i < 5; i++ {
				frame(eng)()
			}
			allocs, bytes := reviewAllocs(50, frame(eng))
			t.Logf("%s, GOMAXPROCS=%d: %.1f аллокаций, %.0f байт на кадр; окно back %v; буферы %d байт (front %d)",
				name, procs, allocs, bytes, eng.canvas.BackWindow(), eng.Stats().BufferBytes, len(eng.canvas.Front().Pix))
			if allocs > limit {
				t.Errorf("%s, GOMAXPROCS=%d: %.1f аллокаций на кадр (порог %v)", name, procs, allocs, limit)
			}
			runtime.GOMAXPROCS(prev)
		}
	}

	// Порог: кадр без слоёв стоит ~11 аллокаций (слэб тайлов, кадр, регионы —
	// столько же было в 4а.3, см. BenchmarkRenderFramePartial); слои поверх
	// дерева не должны добавлять больше нескольких.
	const limit = 16
	t.Run("без слоёв", func(t *testing.T) {
		eng, _, _, _ := reviewSceneWithLayers(t, false)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		measure("без слоёв", eng, limit)
	})
	t.Run("подсказка командами", func(t *testing.T) {
		eng, _, _, _ := reviewSceneWithLayers(t, false)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		eng.SendMouseMove(350, 370)
		eng.SetTooltipDelay(0)
		measure("подсказка", eng, limit)
	})
	t.Run("меню под присмотром, статично", func(t *testing.T) {
		eng, _, o, _ := reviewSceneWithLayers(t, true)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		o.open = true
		eng.Invalidate()
		eng.renderFrame()
		t0 := time.Now()
		widget.StepAnimationsFor(eng, t0)
		widget.StepAnimationsFor(eng, t0.Add(300*time.Millisecond))
		eng.renderFrame()
		measure("меню статично", eng, limit)
	})
	t.Run("подсказка над меню", func(t *testing.T) {
		eng, _, o, _ := reviewSceneWithLayers(t, true)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		o.open = true
		eng.Invalidate()
		eng.renderFrame()
		t0 := time.Now()
		widget.StepAnimationsFor(eng, t0)
		widget.StepAnimationsFor(eng, t0.Add(300*time.Millisecond))
		eng.renderFrame()
		eng.SendMouseMove(350, 370)
		eng.SetTooltipDelay(0)
		measure("подсказка над меню", eng, limit)
	})
	t.Run("модальное над меню", func(t *testing.T) {
		eng, _, o, _ := reviewSceneWithLayers(t, true)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		o.open = true
		eng.Invalidate()
		eng.renderFrame()
		t0 := time.Now()
		widget.StepAnimationsFor(eng, t0)
		widget.StepAnimationsFor(eng, t0.Add(300*time.Millisecond))
		eng.renderFrame()
		m := &dimModal{modal: true}
		m.SetBounds(image.Rect(160, 120, 480, 360))
		eng.ShowModal(m)
		eng.renderFrame()
		measure("модальное над меню", eng, limit)
		if w := eng.canvas.BackWindow(); w != eng.canvas.Frame() {
			t.Errorf("модальное над переходом: окно %v, ждали весь кадр", w)
		}
	})
}

// Линза 3. Статичное меню под присмотром переходов при рендере каждый тик
// (не on-demand): тайлы под его местом лишаются хэша каждый кадр и
// растрятся заново, окно back сбрасывается — меню не стало командой; с
// модальным выше — окно размером с кадр и полный растр каждый кадр.
func TestSingleBufferReview_StaticMenuUnderTransitionsRerastersEveryFrame(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, transitions := range []bool{false, true} {
		for _, modal := range []bool{false, true} {
			t.Run(fmt.Sprintf("переходы=%v/модальное=%v", transitions, modal), func(t *testing.T) {
				eng, _, o, _ := reviewSceneWithLayers(t, transitions)
				eng.SetRenderOnDemand(false)
				eng.renderFrame()
				o.open = true
				eng.renderFrame()
				t0 := time.Now()
				widget.StepAnimationsFor(eng, t0)
				widget.StepAnimationsFor(eng, t0.Add(300*time.Millisecond))
				eng.renderFrame()
				if modal {
					m := &dimModal{modal: true}
					m.SetBounds(image.Rect(160, 120, 480, 360))
					eng.ShowModal(m)
					eng.renderFrame()
				}
				eng.renderFrame()
				before := eng.Stats()
				start := time.Now()
				const n = 50
				for i := 0; i < n; i++ {
					eng.renderFrame()
				}
				el := time.Since(start) / n
				after := eng.Stats()
				grid := gridTiles(eng)
				skipped := float64(after.TilesSkipped-before.TilesSkipped) / n
				t.Logf("без изменений: пропущено %.0f тайлов из %d на кадр, кадр %v, окно back %v, буферы %d, тайлов в выдаче %d",
					skipped, grid, el, eng.canvas.BackWindow(), after.BufferBytes, after.Tiles-before.Tiles)
			})
		}
	}
}

// Линза 2/4. Последовательный путь (тайлы выключены или один процессор):
// при отложенных слоях окно back задаётся дважды за кадр (место слоёв, потом
// область растра) — второе объединяет живое окно и заводит новый буфер;
// с кочующим повреждением окно растёт до кадра и не освобождается, пока меню
// открыто.
func TestSingleBufferReview_SerialPathWindowGrowsToFrame(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, tiled := range []bool{true, false} {
		t.Run(fmt.Sprintf("тайлы=%v", tiled), func(t *testing.T) {
			eng, _, o, _ := reviewSceneWithLayers(t, true)
			eng.SetTiledRaster(tiled)
			eng.SetRenderOnDemand(true)
			eng.renderFrame()
			o.open = true
			eng.Invalidate()
			eng.renderFrame()
			t0 := time.Now()
			widget.StepAnimationsFor(eng, t0)
			widget.StepAnimationsFor(eng, t0.Add(300*time.Millisecond))
			eng.renderFrame()
			eng.InvalidateRect(image.Rect(0, 0, 20, 20))
			eng.renderFrame()
			w1 := eng.canvas.BackWindow()
			b1 := eng.Stats().BufferBytes
			// Повреждение в другом углу: на последовательном пути окно
			// объединяется (новый буфер), на тайловом — нет.
			grow := reviewBytesPer(1, func() {
				eng.InvalidateRect(image.Rect(600, 440, 640, 480))
				eng.renderFrame()
			})
			w2 := eng.canvas.BackWindow()
			b2 := eng.Stats().BufferBytes
			// Дальше повреждение чередуется: окно уже накрывает оба угла.
			steady := reviewBytesPer(20, func() {
				eng.InvalidateRect(image.Rect(0, 0, 20, 20))
				eng.renderFrame()
				eng.InvalidateRect(image.Rect(600, 440, 640, 480))
				eng.renderFrame()
			})
			front := len(eng.canvas.Front().Pix)
			t.Logf("окно после первого повреждения %v (%d байт буферов), после второго %v (%d байт; кадр роста выделил %.0f байт); в стационаре %.0f байт на пару кадров; front %d",
				w1, b1, w2, b2, grow, steady, front)
			if !tiled && w2 != eng.canvas.Frame() {
				t.Logf("последовательный путь: окно не выросло до кадра — %v", w2)
			}
			// Меню открыто 70 кадров подряд: окно используется каждый кадр и
			// не освобождается.
			for i := 0; i < 70; i++ {
				eng.InvalidateRect(image.Rect(0, 0, 20, 20))
				eng.renderFrame()
			}
			t.Logf("через 70 кадров: окно %v, буферы %d байт", eng.canvas.BackWindow(), eng.Stats().BufferBytes)
		})
	}
}

// Линза 4. Смена разрешения: воркеры тайлового растра живут между кадрами с
// окнами, созданными над прежним холстом; окно держит его (viewsOf) — старый
// холст с его front не собирается сборщиком, пока жив движок.
func TestSingleBufferReview_SetResolutionRetainsOldCanvas(t *testing.T) {
	testreset.Globals(t)
	eng := New(1280, 800, 20)
	defer eng.Stop()
	root, _ := busyScene(1280, 800)
	eng.SetRoot(root)
	eng.renderFrame() // воркеры заводятся над этим холстом
	eng.renderFrame()
	old := eng.canvas
	oldBytes := len(old.Front().Pix)
	var freed atomic.Bool
	runtime.SetFinalizer(old, func(*Canvas) { freed.Store(true) })
	old = nil
	eng.SetResolution(800, 600)
	for i := 0; i < 3; i++ {
		eng.Invalidate()
		eng.renderFrame()
	}
	reviewCollect(&freed)
	if !freed.Load() {
		t.Errorf("старый холст (front %d байт) удерживается после SetResolution и трёх кадров на новом", oldBytes)
	}
	// Для сравнения: движок без тайлов (без воркеров) старый холст отпускает.
	eng2 := New(1280, 800, 20)
	defer eng2.Stop()
	eng2.SetTiledRaster(false)
	root2, _ := busyScene(1280, 800)
	eng2.SetRoot(root2)
	eng2.renderFrame()
	eng2.renderFrame()
	var freed2 atomic.Bool
	runtime.SetFinalizer(eng2.canvas, func(*Canvas) { freed2.Store(true) })
	eng2.SetResolution(800, 600)
	for i := 0; i < 3; i++ {
		eng2.Invalidate()
		eng2.renderFrame()
	}
	reviewCollect(&freed2)
	if !freed2.Load() {
		t.Errorf("без тайлового растра старый холст тоже удерживается — причина не в воркерах")
	}
}

// reviewCollect гонит сборщик, пока не сработает финализатор (или 20 циклов).
func reviewCollect(flag *atomic.Bool) {
	for i := 0; i < 20 && !flag.Load(); i++ {
		runtime.GC()
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
}

// Линза 3. Мелкое повреждение (≤ 4 тайлов) при доступном пуле: тайлы идут
// через окно воркера на горутине кадра (RunPart), а не воспроизведением в
// холст движка, как replayTiles 4а.3. Пара в одном запуске: тайлы включены
// (RunPart) и выключены (воспроизведение в окно back).
func BenchmarkSingleBufferReview_SmallPartial_RunPartVsReplay(b *testing.B) {
	for _, tiled := range []bool{true, false} {
		name := "RunPart(тайлы вкл)"
		if !tiled {
			name = "воспроизведение(тайлы выкл)"
		}
		b.Run(name, func(b *testing.B) {
			eng, root := benchTextScene(1280, 800)
			defer eng.Stop()
			eng.SetTiledRaster(tiled)
			eng.SetRenderOnDemand(true)
			var ls []*widget.Label
			labelsOf(root, &ls)
			eng.renderFrame()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ls[77].SetText(fmt.Sprintf("значение %d", i&1))
				eng.renderFrame()
			}
		})
	}
}

// Линза 3. Прямой путь, частичный кадр: окно back размером с кадр задаётся
// каждый кадр, и SetBackWindow на чистом окне трогает все его тайлы — сброс
// идёт по всем тайлам кадра (в пуле), хотя нарисовано повреждение.
func BenchmarkSingleBufferReview_DirectPartial(b *testing.B) {
	eng, _ := benchTextScene(1280, 800)
	defer eng.Stop()
	eng.SetCommandPath(false)
	eng.SetRenderOnDemand(true)
	eng.renderFrame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.InvalidateRect(image.Rect(400, 400, 600, 424))
		eng.renderFrame()
	}
}
