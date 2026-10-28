package engine

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Переходы оверлеев (overlaytrans.go).

var (
	ovtBG   = color.RGBA{R: 30, G: 60, B: 90, A: 255}
	ovtMenu = color.RGBA{R: 230, G: 210, B: 40, A: 255}
)

// ovtOverlay — «меню»: владелец-кнопка с оверлеем под ней. Считает вызовы
// DrawOverlay; открытие и закрытие заявляют весь кадр, как PopupMenu.
type ovtOverlay struct {
	widget.Base
	open    bool
	rect    image.Rectangle
	draws   int
	self    bool // AnimatesOverlay
	content color.RGBA
}

func (o *ovtOverlay) Draw(ctx widget.DrawContext) {
	b := o.Bounds()
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), color.RGBA{R: 200, A: 255})
}
func (o *ovtOverlay) HasOverlay() bool { return o.open }
func (o *ovtOverlay) OverlayBounds() image.Rectangle {
	if !o.open {
		return image.Rectangle{}
	}
	return o.rect
}
func (o *ovtOverlay) DrawOverlay(ctx widget.DrawContext) {
	o.draws++
	c := o.content
	if c.A == 0 {
		c = ovtMenu
	}
	r := o.rect
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), c)
}

type ovtSelf struct{ ovtOverlay }

func (o *ovtSelf) AnimatesOverlay() bool { return true }

// ovtRoot — корень, считающий свои Draw.
type ovtRoot struct {
	widget.Base
	bg    color.RGBA
	draws int
}

func (r *ovtRoot) Draw(ctx widget.DrawContext) {
	r.draws++
	b := r.Bounds()
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), r.bg)
	for _, c := range r.Children() {
		widget.DrawTraced(c, ctx)
	}
}

type ovtScene struct {
	eng  *Engine
	root *ovtRoot
	ov   widget.Widget
	o    *ovtOverlay
}

// ovtManager — тема с переходом меню dur (линейно) и сдвигом offset.
func ovtManager(t *testing.T, dur time.Duration, offset float64) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	p := theme.NewProfile("ovt")
	p.Anims[theme.AnimMenuOpen] = theme.AnimSpec{Duration: dur, Curve: "linear"}
	if offset > 0 {
		p.SetMetric(theme.KeyMenuOpenOffset, offset)
	}
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("ovt"); err != nil {
		t.Fatal(err)
	}
	return m
}

func newOvtScene(t *testing.T, on bool, m *theme.Manager, self bool) *ovtScene {
	t.Helper()
	eng := New(320, 240, 60)
	t.Cleanup(eng.Stop)
	if m != nil {
		if err := eng.ApplyThemeProfile(m); err != nil {
			t.Fatal(err)
		}
	}
	root := &ovtRoot{bg: ovtBG}
	root.SetBounds(image.Rect(0, 0, 320, 240))
	s := &ovtScene{eng: eng, root: root}
	var o *ovtOverlay
	if self {
		so := &ovtSelf{}
		o = &so.ovtOverlay
		s.ov = so
	} else {
		o = &ovtOverlay{}
		s.ov = o
	}
	o.SetBounds(image.Rect(40, 20, 120, 44))
	o.rect = image.Rect(40, 44, 200, 164)
	s.o = o
	root.AddChild(s.ov)
	eng.SetRoot(root)
	eng.SetOverlayTransitions(on)
	eng.RenderOnce()
	return s
}

func (s *ovtScene) setOpen(open bool) {
	s.o.open = open
	s.eng.Invalidate()
}

func sameImage(a, b *image.RGBA) bool {
	if a.Rect != b.Rect {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

// ovtRefs — кадры без перехода: меню закрыто и открыто.
func ovtRefs(t *testing.T, m *theme.Manager) (closed, open *image.RGBA) {
	t.Helper()
	ref := newOvtScene(t, false, m, false)
	closed = ref.eng.RenderOnce()
	ref.setOpen(true)
	open = ref.eng.RenderOnce()
	return closed, open
}

// Выключенные переходы (по умолчанию) ничего не меняют: меню появляется в
// первом же кадре.
func TestOverlayTrans_OffByDefault(t *testing.T) {
	testreset.Globals(t)
	m := ovtManager(t, 100*time.Millisecond, 0)
	_, open := ovtRefs(t, m)
	s := newOvtScene(t, false, m, false)
	if s.eng.OverlayTransitions() {
		t.Fatal("переходы включены по умолчанию")
	}
	s.setOpen(true)
	if !sameImage(s.eng.RenderOnce(), open) {
		t.Fatal("без переходов меню появилось не в первом кадре")
	}
}

// Проявление: первый кадр — как без меню, середина — смесь, конец — ровно
// кадр покоя. Шаги не зовут ни Draw дерева, ни DrawOverlay.
func TestOverlayTrans_FadeOpenClose(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	closed, open := ovtRefs(t, m)
	s := newOvtScene(t, true, m, false)

	s.setOpen(true)
	if got := s.eng.RenderOnce(); !sameImage(got, closed) {
		t.Fatal("первый кадр перехода — не кадр без меню")
	}
	draws, rootDraws := s.o.draws, s.root.draws
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	s.eng.RenderOnce()
	widget.StepAnimationsFor(s.eng, t0.Add(50*time.Millisecond))
	mid := s.eng.RenderOnce()
	c := mid.RGBAAt(120, 100)
	if !ovtNear(c.R, (int(ovtBG.R)+int(ovtMenu.R))/2) || !ovtNear(c.B, (int(ovtBG.B)+int(ovtMenu.B))/2) {
		t.Fatalf("середина проявления %v, ждали смесь %v и %v", c, ovtBG, ovtMenu)
	}
	if s.o.draws != draws || s.root.draws != rootDraws {
		t.Fatalf("шаги перерисовывали: DrawOverlay %d→%d, корень %d→%d", draws, s.o.draws, rootDraws, s.root.draws)
	}
	widget.StepAnimationsFor(s.eng, t0.Add(100*time.Millisecond))
	if got := s.eng.RenderOnce(); !sameImage(got, open) {
		t.Fatal("последний кадр проявления не равен кадру покоя")
	}
	// Конец рисуется обычным путём — ровно одна отрисовка: так у тайлов
	// меню настоящие признаки содержимого, а не «картинка» смешения.
	if s.o.draws != draws+1 {
		t.Fatalf("конец проявления: DrawOverlay %d→%d, ждали одну отрисовку", draws, s.o.draws)
	}

	// Исчезновение: меню закрыто, но кадр ещё показывает его.
	s.setOpen(false)
	if got := s.eng.RenderOnce(); !sameImage(got, open) {
		t.Fatal("кадр закрытия убрал меню сразу")
	}
	t1 := time.Now()
	widget.StepAnimationsFor(s.eng, t1)
	s.eng.RenderOnce()
	widget.StepAnimationsFor(s.eng, t1.Add(50*time.Millisecond))
	if c := s.eng.RenderOnce().RGBAAt(120, 100); !ovtNear(c.R, (int(ovtBG.R)+int(ovtMenu.R))/2) {
		t.Fatalf("середина исчезновения %v", c)
	}
	widget.StepAnimationsFor(s.eng, t1.Add(100*time.Millisecond))
	if got := s.eng.RenderOnce(); !sameImage(got, closed) {
		t.Fatal("последний кадр исчезновения не равен кадру без меню")
	}
	if n := len(s.eng.ovt.list); n != 0 {
		t.Fatalf("после исчезновения под присмотром %d оверлеев", n)
	}
}

func ovtNear(got uint8, want int) bool { d := int(got) - want; return d >= -2 && d <= 2 }

// Число кадров перехода по профилям движения: Full — на каждом шаге,
// Remote — три (1/3, 2/3, 1), Reduced и Off — ни одного, меню сразу.
func TestOverlayTrans_FramesByProfile(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	_, open := ovtRefs(t, m)
	for _, tc := range []struct {
		p    MotionProfile
		want int // кадров, изменивших картинку после кадра открытия
	}{
		// Full: шаги 20…100 мс (первый тик — доля 0, кадр без изменений).
		{MotionFull, 5}, {MotionRemote, 3}, {MotionReduced, 0}, {MotionOff, 0},
	} {
		s := newOvtScene(t, true, m, false)
		s.eng.SetMotionProfile(tc.p)
		s.setOpen(true)
		first := s.eng.RenderOnce()
		if tc.want == 0 {
			if !sameImage(first, open) {
				t.Errorf("%v: меню появилось не сразу", tc.p)
			}
			s.eng.SetMotionProfile(MotionFull)
			continue
		}
		changes := 0
		t0 := time.Now()
		for ms := 0; ms <= 120; ms += 20 {
			widget.StepAnimationsFor(s.eng, t0.Add(time.Duration(ms)*time.Millisecond))
			fr := s.eng.RenderFrameNow()
			if len(fr.Tiles) > 0 {
				changes++
			}
		}
		if changes != tc.want {
			t.Errorf("%v: кадров с изменениями %d, ждали %d", tc.p, changes, tc.want)
		}
		if !sameImage(s.eng.RenderOnce(), open) {
			t.Errorf("%v: переход не закончился кадром покоя", tc.p)
		}
		s.eng.SetMotionProfile(MotionFull)
	}
}

// Сдвиг: меню под владельцем выезжает сверху вниз; посреди пути его верх
// выше места покоя на половину сдвига.
func TestOverlayTrans_Slide(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 20)
	_, open := ovtRefs(t, m)
	s := newOvtScene(t, true, m, false)
	s.setOpen(true)
	s.eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	s.eng.RenderOnce()
	widget.StepAnimationsFor(s.eng, t0.Add(50*time.Millisecond))
	mid := s.eng.RenderOnce()
	// Верх меню в покое — y=44; на середине — y=34 (сдвиг 20·0.5).
	if c := mid.RGBAAt(120, 38); c == ovtBG {
		t.Errorf("над местом покоя нет меню посреди выезда: %v", c)
	}
	if c := mid.RGBAAt(120, 160); c != ovtBG && !ovtNear(c.R, int(ovtBG.R)) {
		t.Errorf("у низа места покоя меню ещё не должно быть: %v", c)
	}
	widget.StepAnimationsFor(s.eng, t0.Add(100*time.Millisecond))
	if !sameImage(s.eng.RenderOnce(), open) {
		t.Fatal("выезд не закончился кадром покоя")
	}
}

// Дерево под меню меняется посреди перехода: снимки обновляются, конец — кадр
// покоя с новым деревом.
func TestOverlayTrans_TreeChangesUnder(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 10)
	s := newOvtScene(t, true, m, false)
	s.setOpen(true)
	s.eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	widget.StepAnimationsFor(s.eng, t0.Add(40*time.Millisecond))
	s.eng.RenderOnce()
	s.root.bg = color.RGBA{R: 10, G: 120, B: 10, A: 255}
	s.eng.InvalidateRect(image.Rect(150, 150, 170, 170)) // задевает место меню краем
	s.eng.RenderOnce()
	widget.StepAnimationsFor(s.eng, t0.Add(100*time.Millisecond))
	got := s.eng.RenderOnce()

	ref := newOvtScene(t, false, m, false)
	ref.root.bg = s.root.bg
	ref.setOpen(true)
	ref.eng.InvalidateRect(image.Rect(150, 150, 170, 170))
	want := ref.eng.RenderOnce()
	// Сравниваем место меню с запасом: вне его кадры различаются (у ref
	// фон перерисован целиком).
	r := s.eng.ovt.list[0].rest
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if got.RGBAAt(x, y) != want.RGBAAt(x, y) {
				t.Fatalf("(%d,%d): %v, ждали %v — снимок «под» устарел", x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
			}
		}
	}
}

// Повторное открытие посреди исчезновения продолжает с текущей доли.
func TestOverlayTrans_ReopenContinues(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	s := newOvtScene(t, true, m, false)
	s.setOpen(true)
	s.eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	widget.StepAnimationsFor(s.eng, t0.Add(100*time.Millisecond))
	s.eng.RenderOnce()
	s.setOpen(false)
	s.eng.RenderOnce()
	t1 := time.Now()
	widget.StepAnimationsFor(s.eng, t1)
	widget.StepAnimationsFor(s.eng, t1.Add(50*time.Millisecond))
	s.eng.RenderOnce()
	s.setOpen(true)
	if c := s.eng.RenderOnce().RGBAAt(120, 100); !ovtNear(c.R, (int(ovtBG.R)+int(ovtMenu.R))/2) {
		t.Fatalf("переоткрытие начало не с текущей доли: %v", c)
	}
}

// Оверлей, анимирующий себя сам, переходом движка не накрывается.
func TestOverlayTrans_SelfAnimatedOptOut(t *testing.T) {
	testreset.Globals(t)
	m := ovtManager(t, 100*time.Millisecond, 0)
	s := newOvtScene(t, true, m, true)
	s.setOpen(true)
	if c := s.eng.RenderOnce().RGBAAt(120, 100); c != ovtMenu {
		t.Fatalf("самоанимирующийся оверлей получил переход движка: %v", c)
	}
}

// Хостовые попапы переходов не получают (окна ОС без альфы).
func TestOverlayTrans_HostedSkipped(t *testing.T) {
	testreset.Globals(t)
	m := ovtManager(t, 100*time.Millisecond, 0)
	s := newOvtScene(t, true, m, false)
	var got []PopupFrame
	s.eng.SetPopupSink(func(f []PopupFrame) { got = f })
	defer s.eng.SetPopupSink(nil)
	s.setOpen(true)
	s.eng.RenderOnce()
	if len(got) != 1 || len(s.eng.ovt.list) != 0 {
		t.Fatalf("hosted: кадров попапов %d, под присмотром %d", len(got), len(s.eng.ovt.list))
	}
}

// Выключение посреди перехода: следующий кадр — меню целиком.
func TestOverlayTrans_DisableMidway(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 10)
	_, open := ovtRefs(t, m)
	s := newOvtScene(t, true, m, false)
	s.setOpen(true)
	s.eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	widget.StepAnimationsFor(s.eng, t0.Add(30*time.Millisecond))
	s.eng.RenderOnce()
	s.eng.SetOverlayTransitions(false)
	if !sameImage(s.eng.RenderOnce(), open) || len(s.eng.ovt.list) != 0 {
		t.Fatal("после выключения переход не снят")
	}
}

// Переход корневого слоя под модальным окном шагает перерисовкой слоёв:
// смешение без неё легло бы поверх затемнения.
func TestOverlayTrans_UnderModalRedraws(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	s := newOvtScene(t, true, m, false)
	s.setOpen(true)
	s.eng.RenderOnce()
	dlg := widget.NewDialog("x", 100, 60)
	s.eng.ShowModal(dlg)
	s.eng.RenderOnce()
	if len(s.eng.ovt.list) != 1 || s.eng.ovt.list[0].direct.Load() {
		t.Fatal("переход под модальным окном помечен шагом без перерисовки")
	}
	s.eng.CloseModal(dlg)
}

// Шаг перехода: смешение снимков и сравнение области — без отрисовки.
func BenchmarkOverlayTrans_Step(b *testing.B) {
	testreset.Globals(b)
	eng, tr := ovtBenchScene(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.setVal(0.25 + 0.5*float64(i&1))
		tr.ticked.Store(true)
		eng.RenderFrameNow()
	}
}

// Тот же кадр, если бы шаг заявлял область повреждением: фон, дерево и
// оверлей перерисовываются.
func BenchmarkOverlayTrans_RedrawStep(b *testing.B) {
	testreset.Globals(b)
	eng, tr := ovtBenchScene(b)
	tr.direct.Store(false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.setVal(0.25 + 0.5*float64(i&1))
		tr.ticked.Store(true)
		eng.InvalidateRect(tr.restL)
		eng.RenderFrameNow()
	}
}

func ovtBenchScene(b *testing.B) (*Engine, *overlayTrans) {
	m := theme.NewManager()
	p := theme.NewProfile("ovt")
	p.Anims[theme.AnimMenuOpen] = theme.AnimSpec{Duration: time.Hour, Curve: "linear"}
	p.SetMetric(theme.KeyMenuOpenOffset, 8)
	_ = m.RegisterTheme(p)
	_ = m.SetTheme("ovt")
	eng := New(1280, 720, 60)
	b.Cleanup(eng.Stop)
	// Как у потребителя с буферами SetSurfaces: кадр — места изменений, без
	// копии байтов тайлов; иначе бенчмарк мерил бы копию, а не шаг.
	eng.SetFrameTileData(false)
	_ = eng.ApplyThemeProfile(m)
	root := &ovtRoot{bg: ovtBG}
	root.SetBounds(image.Rect(0, 0, 1280, 720))
	o := &ovtOverlay{}
	o.SetBounds(image.Rect(100, 100, 200, 124))
	o.rect = image.Rect(100, 124, 380, 524)
	root.AddChild(o)
	eng.SetRoot(root)
	eng.SetOverlayTransitions(true)
	eng.RenderFrameNow()
	o.open = true
	eng.Invalidate()
	eng.RenderFrameNow()
	widget.StopAllAnimations() // доля ведётся вручную
	tr := eng.ovt.list[0]
	eng.RenderFrameNow()
	if !tr.direct.Load() {
		b.Fatal("сцена бенчмарка: шаг не прямой")
	}
	return eng, tr
}

// Настоящее меню в теме Windows 11 (акрил, мягкая тень, кривая с выбросом,
// сдвиг 8 точек): переход кончается кадром, равным кадру без переходов, и
// закрытие — кадром без меню.
func TestOverlayTrans_PopupMenuWin11(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	newMgr := func() *theme.Manager {
		m := theme.NewManager()
		if err := theme.RegisterBuiltinProfiles(m); err != nil {
			t.Fatal(err)
		}
		if err := m.SetTheme(theme.ProfileWindows11); err != nil {
			t.Fatal(err)
		}
		return m
	}
	scene := func(on bool) (*Engine, *widget.PopupMenu) {
		e := New(480, 360, 60)
		t.Cleanup(e.Stop)
		if err := e.ApplyThemeProfile(newMgr()); err != nil {
			t.Fatal(err)
		}
		root := widget.NewPanel(color.RGBA{R: 70, G: 110, B: 160, A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, 480, 360))
		lbl := widget.NewLabel("Фон под меню — текст и цвета", color.RGBA{R: 250, G: 250, B: 250, A: 255})
		lbl.SetBounds(image.Rect(40, 70, 440, 100))
		root.AddChild(lbl)
		m := widget.NewPopupMenu()
		m.SetItems([]widget.MenuItem{{Text: "Открыть"}, {Text: "Переименовать"}, {Text: "Удалить"}})
		root.AddChild(m)
		e.SetRoot(root)
		e.SetOverlayTransitions(on)
		e.RenderOnce()
		return e, m
	}
	ref, rm := scene(false)
	closed := ref.RenderOnce()
	rm.Show(60, 60)
	open := ref.RenderOnce()

	e, m := scene(true)
	m.Show(60, 60)
	if !sameImage(e.RenderOnce(), closed) {
		t.Fatal("первый кадр перехода — не кадр без меню")
	}
	t0 := time.Now()
	var mid *image.RGBA
	for ms := 0; ms <= 220; ms += 16 {
		widget.StepAnimationsFor(e, t0.Add(time.Duration(ms)*time.Millisecond))
		img := e.RenderOnce()
		if ms == 96 {
			mid = img
		}
	}
	if sameImage(mid, closed) || sameImage(mid, open) {
		t.Fatal("посреди перехода кадр равен одному из концов")
	}
	if !sameImage(e.RenderOnce(), open) {
		t.Fatal("переход не кончился кадром покоя Windows 11")
	}
	m.Close()
	e.RenderOnce()
	t1 := time.Now()
	for ms := 0; ms <= 220; ms += 16 {
		widget.StepAnimationsFor(e, t1.Add(time.Duration(ms)*time.Millisecond))
		e.RenderOnce()
	}
	if !sameImage(e.RenderOnce(), closed) {
		t.Fatal("исчезновение не кончилось кадром без меню")
	}
}

// ovtFinishOpen проводит открытие до конца.
func ovtFinishOpen(s *ovtScene) {
	s.setOpen(true)
	s.eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	widget.StepAnimationsFor(s.eng, t0.Add(200*time.Millisecond))
}

// Шаг, чья область лежит внутри объединения двух далёких повреждений, тоже
// сравнивается: иначе потребитель остался бы с недосмешанным кадром.
func TestOverlayTrans_StepInsideDamageUnionIsDiffed(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	s := newOvtScene(t, true, m, false)
	s.setOpen(true)
	s.eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	s.eng.RenderOnce() // шаги дальше — без перерисовки
	// Шаг посреди пути (тик повреждения не заявляет) и два далёких
	// повреждения, чьё объединение накрывает меню.
	widget.StepAnimationsFor(s.eng, t0.Add(50*time.Millisecond))
	s.eng.InvalidateRect(image.Rect(0, 0, 4, 4))
	s.eng.InvalidateRect(image.Rect(316, 236, 320, 240))
	s.eng.RenderFrameNow()
	if f, b := s.eng.canvas.front.RGBAAt(120, 100), s.eng.canvas.back.RGBAAt(120, 100); f != b {
		t.Fatalf("шаг смешан в буфер, но не отдан: передний %v, задний %v", f, b)
	}
}

// Исчезающее меню остаётся на своём месте в порядке рисования: под
// оверлеем, который рисуется после него, и не прожигает в нём дыру.
func TestOverlayTrans_ClosingStaysUnderLaterOverlay(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	s := newOvtScene(t, true, m, false)
	// Панель, анимирующая себя сама, рисуется после меню и накрывает его.
	panel := &ovtSelf{}
	panel.SetBounds(image.Rect(150, 0, 151, 1))
	panel.rect = image.Rect(100, 80, 300, 200)
	panel.content = color.RGBA{R: 20, G: 200, B: 20, A: 255}
	panel.open = true
	s.root.AddChild(panel)
	ovtFinishOpen(s)
	s.eng.RenderOnce()
	s.setOpen(false)
	s.eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	s.eng.InvalidateRect(image.Rect(150, 120, 160, 130)) // наведение на панель над меню
	s.eng.RenderOnce()
	for ms := 20; ms <= 120; ms += 20 {
		widget.StepAnimationsFor(s.eng, t0.Add(time.Duration(ms)*time.Millisecond))
		img := s.eng.RenderOnce()
		if c := img.RGBAAt(150, 150); c != panel.content {
			t.Fatalf("на %d мс панель над исчезающим меню прожжена: %v", ms, c)
		}
		// Вне панели меню ещё видно — исчезновение идёт, а не оборвано.
		if c := img.RGBAAt(60, 100); ms == 40 && (c == ovtBG || c == ovtMenu) {
			t.Fatalf("на %d мс вне панели не смесь, а %v: исчезновения нет", ms, c)
		}
	}
}

// Конец перехода — прямая отрисовка: признаки тайлов меню те же, что без
// переходов.
func TestOverlayTrans_FinalFrameMarks(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	ref := newOvtScene(t, false, m, false)
	ref.setOpen(true)
	ref.eng.RenderOnce()
	s := newOvtScene(t, true, m, false)
	ovtFinishOpen(s)
	s.eng.RenderOnce()
	// Тайл (1,1) — 64…128 по обеим осям — целиком внутри меню 40…200 × 44…164.
	i := 1*s.eng.canvas.tilesX + 1
	if got, want := s.eng.canvas.marks[i].kind, ref.eng.canvas.marks[i].kind; got != want {
		t.Fatalf("признак тайла меню %v, без переходов %v", got, want)
	}
}

// Открытие посреди исчезновения в другом месте: прежнее место не хранит
// недосмешанного призрака.
func TestOverlayTrans_ReopenElsewhereClearsOld(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	s := newOvtScene(t, true, m, false)
	ovtFinishOpen(s)
	s.eng.RenderOnce()
	s.setOpen(false)
	s.eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	widget.StepAnimationsFor(s.eng, t0.Add(50*time.Millisecond))
	s.eng.RenderOnce()
	// Владелец открывает меню далеко и заявляет только новое место (прежнее
	// с запасом — до 224×188 — не задето).
	s.o.rect = image.Rect(274, 214, 310, 230)
	s.o.open = true
	s.eng.InvalidateRect(image.Rect(250, 190, 320, 240))
	// Кадры — как у цикла: только когда что-то заявлено. Кадр без
	// повреждения рисуется полным путём и починил бы что угодно.
	gen := s.eng.invGen.Load()
	s.eng.RenderFrameNow()
	t1 := time.Now()
	for ms := 0; ms <= 200; ms += 20 {
		widget.StepAnimationsFor(s.eng, t1.Add(time.Duration(ms)*time.Millisecond))
		if g := s.eng.invGen.Load(); g != gen {
			gen = g
			s.eng.RenderFrameNow()
		}
	}
	if c := s.eng.canvas.front.RGBAAt(120, 100); c != ovtBG {
		t.Fatalf("на прежнем месте меню остался призрак: %v", c)
	}
}

// Включение переходов при открытом меню не прячет и не проигрывает его заново.
func TestOverlayTrans_EnableWhileOpen(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	s := newOvtScene(t, false, m, false)
	s.setOpen(true)
	s.eng.RenderOnce()
	s.eng.SetOverlayTransitions(true)
	s.eng.InvalidateRect(image.Rect(110, 90, 130, 110))
	if c := s.eng.RenderOnce().RGBAAt(120, 100); c != ovtMenu {
		t.Fatalf("меню пропало после включения переходов: %v", c)
	}
	s.eng.Invalidate()
	if c := s.eng.RenderOnce().RGBAAt(120, 100); c != ovtMenu {
		t.Fatalf("меню проявляется заново после включения переходов: %v", c)
	}
}

// anchored — владелец, чьи границы уже включают оверлей (как у PopupMenu),
// с точкой привязки отдельно.
type anchored struct {
	widget.Base
	at image.Point
}

func (a *anchored) OverlayAnchor() image.Point { return a.at }
func (a *anchored) Draw(widget.DrawContext)    {}

// Направление сдвига — по точке привязки: меню, вписанное выше неё, выезжает
// снизу, хотя границы владельца начинаются выше меню.
func TestOverlayTrans_SlideDirByAnchor(t *testing.T) {
	testreset.Globals(t)
	a := &anchored{at: image.Pt(10, 200)}
	a.SetBounds(image.Rect(10, 100, 100, 200)) // включает меню 100…190
	if d := ovtSlideDir(a, image.Rect(10, 100, 100, 190), 8); d.Y <= 0 {
		t.Fatalf("меню выше точки привязки: сдвиг %v, ждали снизу (положительный)", d)
	}
	a.at = image.Pt(10, 100)
	if d := ovtSlideDir(a, image.Rect(10, 100, 100, 190), 8); d.Y >= 0 {
		t.Fatalf("меню ниже точки привязки: сдвиг %v, ждали сверху (отрицательный)", d)
	}
	m := widget.NewPopupMenu()
	m.SetItems([]widget.MenuItem{{Text: "a"}})
	m.Show(30, 70)
	if got := m.OverlayAnchor(); got != image.Pt(30, 70) {
		t.Fatalf("PopupMenu.OverlayAnchor = %v, ждали точку Show", got)
	}
	m.Close()
}

// Смена формата буфера посреди перехода снимает переходы: снимки в прежнем
// порядке каналов.
func TestOverlayTrans_FormatChangeDrops(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 0)
	s := newOvtScene(t, true, m, false)
	s.setOpen(true)
	s.eng.RenderOnce()
	if len(s.eng.ovt.list) != 1 {
		t.Fatal("меню не под присмотром")
	}
	bufs := [][]byte{make([]byte, 320*240*4), make([]byte, 320*240*4)}
	if err := s.eng.SetSurfaces(bufs, 320*4, FormatBGRX); err != nil {
		t.Fatal(err)
	}
	s.eng.RenderFrameNow()
	for _, tr := range s.eng.ovt.list {
		if tr.state != ovtStatic {
			t.Fatalf("после смены формата переход остался: фаза %v", tr.state)
		}
	}
}

// Область перехода, добавленная к повреждению, сама расширяется на
// размытую подложку, которую задевает: иначе подложка перерисовалась бы
// куском поверх прошлого кадра (backdropdamage.go).
func TestOverlayTrans_PrepareExpandsBackdrops(t *testing.T) {
	testreset.Globals(t)
	e := New(320, 240, 60)
	defer e.Stop()
	c := e.canvas
	c.backdrops = []image.Rectangle{image.Rect(0, 200, 320, 240)} // «панель задач»
	tr := &overlayTrans{state: ovtStatic, layer: nil, rest: image.Rect(40, 60, 200, 210)}
	e.ovt.list = []*overlayTrans{tr}
	e.ovt.canvas, e.ovt.format = c, c.format
	rects, _ := e.ovtPrepare(c, []image.Rectangle{image.Rect(50, 70, 60, 80)}, false, []widget.Widget{nil}, true)
	if !coveredByOne(rects, tr.rest) {
		t.Fatalf("область перехода не стала повреждением: %v", rects)
	}
	if !coveredByOne(rects, image.Rect(0, 200, 320, 240)) {
		t.Fatalf("подложка, задетая областью перехода, не расширена целиком: %v", rects)
	}
}

// Исчезновение идёт по theme.AnimMenuClose, если профиль его объявил, иначе
// по AnimMenuOpen.
func TestOverlayTrans_CloseUsesMenuClose(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, tc := range []struct {
		name   string
		close  time.Duration
		closed bool // к 40 мс исчезновение закончено
	}{{"menu.close 40 мс", 40 * time.Millisecond, true}, {"без menu.close", 0, false}} {
		m := ovtManagerClose(t, 100*time.Millisecond, tc.close)
		closed, _ := ovtRefs(t, m)
		s := newOvtScene(t, true, m, false)
		ovtFinishOpen(s)
		s.eng.RenderOnce()
		s.setOpen(false)
		s.eng.RenderOnce()
		t0 := time.Now()
		widget.StepAnimationsFor(s.eng, t0)
		widget.StepAnimationsFor(s.eng, t0.Add(40*time.Millisecond))
		if got := sameImage(s.eng.RenderOnce(), closed); got != tc.closed {
			t.Errorf("%s: к 40 мс меню исчезло=%v, ждали %v", tc.name, got, tc.closed)
		}
	}
}

// ovtManagerClose — ovtManager с отдельным исчезновением (0 — не объявлено).
func ovtManagerClose(t *testing.T, open, close time.Duration) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	p := theme.NewProfile("ovt-close")
	p.Anims[theme.AnimMenuOpen] = theme.AnimSpec{Duration: open, Curve: "linear"}
	if close > 0 {
		p.Anims[theme.AnimMenuClose] = theme.AnimSpec{Duration: close, Curve: "linear"}
	}
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("ovt-close"); err != nil {
		t.Fatal(err)
	}
	return m
}
