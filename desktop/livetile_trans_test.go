package desktop

import (
	"image"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/internal/testengine"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Переход живой плитки через кэш растра (ADR-0010, задача 6.2).

// transTheme — Windows 10 с переходом плитки длительностью dur (линейно) и
// видом kind (0 — сдвиг, 1 — переворот).
func transTheme(t *testing.T, dur time.Duration, kind int) *theme.Manager {
	t.Helper()
	tm := managerFor(t, theme.ProfileWindows10)
	p := theme.NewProfile("win10-live")
	p.Parent = theme.ProfileWindows10
	p.Anims[AnimTileCycle] = theme.AnimSpec{Duration: dur}
	p.SetMetric(KeyTileCycleKind, float64(kind))
	if err := tm.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetTheme("win10-live"); err != nil {
		t.Fatal(err)
	}
	return tm
}

// transScene — меню в дереве движка (не запущенного: кадры — RenderOnce,
// анимации — StepAnimations), открытое, с живой плиткой t2.
type transScene struct {
	eng *engine.Engine
	m   *StartMenu
	src *countSource
}

func newTransScene(t *testing.T, tm *theme.Manager, scale float64) *transScene {
	t.Helper()
	t.Cleanup(widget.StopAllAnimations)
	root := widget.NewPanel(theme.RGB(20, 30, 40))
	root.ShowHeader = false
	root.SetBounds(tileScreen())
	m := NewStartMenu(tm, NewStaticAppCatalog(tileApps()...))
	m.Screen = tileScreen()
	m.SetBounds(tileAnchor())
	root.AddChild(m)
	eng := testengine.New(t, tileScreenW, tileScreenH, 60)
	eng.SetTooltipsEnabled(false)
	if scale != 1 {
		eng.SetScale(scale)
	}
	eng.SetRoot(root)
	src := newCountSource()
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour}))
	m.Open(tileAnchor())
	t.Cleanup(m.Close)
	finishAnimations()
	eng.RenderOnce()
	eng.RenderOnce()
	return &transScene{eng: eng, m: m, src: src}
}

// crop — копия области r кадра (физические пиксели при масштабе 1).
func crop(img *image.RGBA, r image.Rectangle) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		s := img.PixOffset(r.Min.X, r.Min.Y+y)
		copy(out.Pix[y*out.Stride:(y+1)*out.Stride], img.Pix[s:s+r.Dx()*4])
	}
	return out
}

func rowEqual(a *image.RGBA, ya int, b *image.RGBA, yb int) bool {
	ra := a.Pix[ya*a.Stride : ya*a.Stride+a.Rect.Dx()*4]
	rb := b.Pix[yb*b.Stride : yb*b.Stride+b.Rect.Dx()*4]
	for i := range ra {
		if ra[i] != rb[i] {
			return false
		}
	}
	return true
}

func (s *transScene) trans() *tileTrans {
	ls := &s.m.v.live
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if st := ls.tiles["t2"]; st != nil {
		return st.trans
	}
	return nil
}

// Сдвиг: в начале перехода композиция кэшей совпадает с кадром покоя прежнего
// состояния побайтно; посреди — прежнее уехало вверх на долю высоты, новое
// въехало снизу; в конце — кадр нового состояния.
func TestLiveTileTrans_SlideComposesTwoCaches(t *testing.T) {
	testreset.Globals(t)
	const dur = 400 * time.Millisecond
	s := newTransScene(t, transTheme(t, dur, 0), 1)
	r := s.m.tileRectAbs("t2")
	if r.Empty() {
		t.Fatal("плитка t2 не на экране")
	}

	s.src.emit(TileContent{Title: "Первое", Subtitle: "утро"})
	s.eng.RenderOnce()
	restA := crop(s.eng.RenderOnce(), r)

	s.src.emit(TileContent{Title: "Второе", Subtitle: "вечер"})
	start := crop(s.eng.RenderOnce(), r)
	tr := s.trans()
	if tr == nil {
		t.Fatal("переход не начался")
	}
	if tr.fromImg == nil || tr.toImg == nil {
		t.Fatal("кэши растра не построены: контекст кадра не дал OffscreenRenderer")
	}
	for y := 0; y < r.Dy(); y++ {
		if !rowEqual(start, y, restA, y) {
			t.Fatalf("начало перехода: строка %d отличается от кадра покоя прежнего состояния", y)
		}
	}

	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(dur / 4))
	mid := crop(s.eng.RenderOnce(), r)
	off := r.Dy() / 4
	widget.StepAnimations(t0.Add(2 * dur))
	if s.trans() != nil {
		t.Fatal("переход не закончился")
	}
	restB := crop(s.eng.RenderOnce(), r)

	for y := 0; y < r.Dy()-off; y++ {
		if !rowEqual(mid, y, restA, y+off) {
			t.Fatalf("середина сдвига: строка %d — не строка %d прежнего", y, y+off)
		}
	}
	for y := r.Dy() - off; y < r.Dy(); y++ {
		if !rowEqual(mid, y, restB, y-(r.Dy()-off)) {
			t.Fatalf("середина сдвига: строка %d — не строка %d нового", y, y-(r.Dy()-off))
		}
	}
	if rowEqual(restA, r.Dy()-8, restB, r.Dy()-8) {
		t.Error("кадры двух состояний не различаются — проверка ничего не проверяет")
	}
}

// Переворот: в первой половине — прежнее, сжатое по высоте к середине, во
// второй — новое; над сжатой картинкой видна подложка меню, а не плитка.
func TestLiveTileTrans_FlipScalesAroundMiddle(t *testing.T) {
	testreset.Globals(t)
	const dur = 400 * time.Millisecond
	s := newTransScene(t, transTheme(t, dur, 1), 1)
	r := s.m.tileRectAbs("t2")
	s.src.emit(TileContent{Title: "Первое"})
	s.eng.RenderOnce()
	restA := crop(s.eng.RenderOnce(), r)
	s.src.emit(TileContent{Title: "Второе"})
	s.eng.RenderOnce()
	if tr := s.trans(); tr == nil || !tr.flip {
		t.Fatal("переворот не начался")
	}
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(dur / 4)) // k = 0,25: прежнее в половину высоты
	q := crop(s.eng.RenderOnce(), r)
	if rowEqual(q, 2, restA, 2) {
		t.Error("k=0,25: верх плитки не освободился — картинка не сжата")
	}
	if !rowEqual(q, r.Dy()/2, restA, r.Dy()/2) && rowEqual(q, 2, q, r.Dy()/2) {
		t.Error("k=0,25: середина плитки пуста — картинки нет")
	}
	widget.StepAnimations(t0.Add(2 * dur))
	if s.trans() != nil {
		t.Error("переворот не закончился")
	}
}

// Под MotionOff и MotionRemote смена мгновенная: перехода нет, кэшей нет. Под
// MotionFull тот же сценарий переход заводит (проверка самой проверки).
func TestLiveTileTrans_InstantUnderOffAndRemote(t *testing.T) {
	testreset.Globals(t)
	for _, tc := range []struct {
		name    string
		profile engine.MotionProfile
		want    bool
	}{
		{"Full", theme.MotionFull, true},
		{"Off", theme.MotionOff, false},
		{"Remote", theme.MotionRemote, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testreset.Globals(t)
			t.Cleanup(widget.StopAllAnimations)
			root := widget.NewPanel(theme.RGB(20, 30, 40))
			root.ShowHeader = false
			root.SetBounds(tileScreen())
			m := NewStartMenu(transTheme(t, 10*time.Second, 0), NewStaticAppCatalog(tileApps()...))
			m.Screen = tileScreen()
			m.SetBounds(tileAnchor())
			root.AddChild(m)
			eng := testengine.New(t, tileScreenW, tileScreenH, 60)
			eng.SetTooltipsEnabled(false)
			eng.SetMotionProfile(tc.profile)
			eng.SetRoot(root)
			src := newCountSource()
			eng.Start()
			eng.Post(func() {
				m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour}))
				m.Open(tileAnchor())
			})
			eng.Flush()
			t.Cleanup(func() { eng.Post(m.Close); eng.Flush() })
			src.emit(TileContent{Title: "a"})
			src.emit(TileContent{Title: "b"})
			eng.Flush()
			var got bool
			var shown string
			eng.Post(func() {
				ls := &m.v.live
				ls.mu.Lock()
				got = ls.tiles["t2"] != nil && ls.tiles["t2"].trans != nil
				ls.mu.Unlock()
				shown = shownOf(m, "t2").Title
			})
			eng.Flush()
			if got != tc.want {
				t.Errorf("переход идёт = %v, ждали %v", got, tc.want)
			}
			if shown != "b" {
				t.Errorf("показано %q, ждали новое состояние", shown)
			}
		})
	}
}

// diffBounds — охват пикселей, которыми различаются кадры a и b.
func diffBounds(a, b *image.RGBA) image.Rectangle {
	var out image.Rectangle
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			i := a.PixOffset(x, y)
			if a.Pix[i] != b.Pix[i] || a.Pix[i+1] != b.Pix[i+1] || a.Pix[i+2] != b.Pix[i+2] || a.Pix[i+3] != b.Pix[i+3] {
				out = out.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return out
}

func cloneRGBA(src *image.RGBA) *image.RGBA {
	out := image.NewRGBA(src.Rect)
	copy(out.Pix, src.Pix)
	return out
}

// sinkRec — сток кадров запущенного движка: тайлы каждого кадра.
type sinkRec struct {
	mu     sync.Mutex
	frames [][]image.Rectangle
}

func (r *sinkRec) Present(f output.Frame) {
	rects := make([]image.Rectangle, 0, len(f.Tiles))
	for _, t := range f.Tiles {
		rects = append(rects, image.Rect(t.X, t.Y, t.X+t.W, t.Y+t.H))
	}
	r.mu.Lock()
	r.frames = append(r.frames, rects)
	r.mu.Unlock()
}

func (r *sinkRec) take() [][]image.Rectangle {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.frames
	r.frames = nil
	return out
}

// Приёмка 6.2: обновление плитки 150×150 (средняя плитка Windows 10 при
// масштабе 150 %) у запущенного движка — кадры только по заявленному
// повреждению — отдаёт не больше 9 тайлов на кадр, и все они касаются плитки:
// остальное меню не перерисовывается. Так и для каждого кадра перехода, и для
// мгновенной смены. Счёт — по стоку кадров (SetFrameSink).
func TestLiveTileTrans_UpdateCostsAtMostNineTiles(t *testing.T) {
	testreset.Globals(t)
	for _, tc := range []struct {
		name string
		dur  time.Duration
	}{{"переход", 300 * time.Millisecond}, {"мгновенно", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			testreset.Globals(t)
			t.Cleanup(widget.StopAllAnimations)
			root := widget.NewPanel(theme.RGB(20, 30, 40))
			root.ShowHeader = false
			root.SetBounds(tileScreen())
			m := NewStartMenu(transTheme(t, tc.dur, 0), NewStaticAppCatalog(tileApps()...))
			m.Screen = tileScreen()
			m.SetBounds(tileAnchor())
			root.AddChild(m)
			eng := testengine.New(t, tileScreenW, tileScreenH, 60)
			eng.SetTooltipsEnabled(false)
			eng.SetScale(1.5)
			eng.SetRoot(root)
			rec := &sinkRec{}
			eng.SetFrameSink(rec)
			src := newCountSource()
			eng.Start()
			eng.Post(func() {
				m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour}))
				m.Open(tileAnchor())
			})
			t.Cleanup(func() { eng.Post(m.Close); eng.Flush() })
			liveSettle(eng)
			src.emit(TileContent{Title: "a"})
			liveSettle(eng)

			var r image.Rectangle
			eng.Post(func() { r = m.tileRectAbs("t2") })
			eng.Flush()
			if r.Size().Mul(3).Div(2) != image.Pt(150, 150) {
				t.Fatalf("плитка t2 %v логических — не 150×150 физических", r.Size())
			}
			phys := image.Rect(r.Min.X*3/2, r.Min.Y*3/2, r.Max.X*3/2, r.Max.Y*3/2)
			rec.take()

			src.emit(TileContent{Title: "b", Subtitle: "новое"})
			liveSettle(eng)
			frames := rec.take()
			total := 0
			var counts []int
			for i, f := range frames {
				counts = append(counts, len(f))
				total += len(f)
				if len(f) > 9 {
					t.Errorf("кадр %d: %d тайлов, ждали не больше 9", i, len(f))
				}
				for _, tr := range f {
					if !tr.Overlaps(phys) {
						t.Errorf("кадр %d: тайл %v не касается плитки %v", i, tr, phys)
					}
				}
			}
			if total == 0 {
				t.Error("смена состояния не отдала ни одного тайла — до экрана не дошла")
			}
			var shown string
			eng.Post(func() { shown = shownOf(m, "t2").Title })
			eng.Flush()
			if shown != "b" {
				t.Errorf("показано %q, ждали b", shown)
			}
			t.Logf("плитка %v физически; кадров %d, тайлов по кадрам %v", phys, len(frames), counts)
		})
	}
}

// BenchmarkLiveTileTransitionFrame — кадр перехода плитки 150×150: композиция
// двух кэшей, заявлен только прямоугольник плитки. tiles/op — тайлов кадра на
// шаг (приёмка 6.2: не больше 9).
func BenchmarkLiveTileTransitionFrame(b *testing.B) { benchTransition(b, 400*time.Millisecond) }

// BenchmarkLiveTileInstantUpdate — мгновенная смена состояния плитки 150×150
// (Off/Remote или тема без перехода).
func BenchmarkLiveTileInstantUpdate(b *testing.B) { benchTransition(b, 0) }

func benchTransition(b *testing.B, dur time.Duration) {
	testreset.Globals(b)
	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		b.Fatal(err)
	}
	p := theme.NewProfile("win10-live-bench")
	p.Parent = theme.ProfileWindows10
	p.Anims[AnimTileCycle] = theme.AnimSpec{Duration: dur}
	if err := tm.RegisterTheme(p); err != nil {
		b.Fatal(err)
	}
	if err := tm.SetTheme("win10-live-bench"); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(widget.StopAllAnimations)
	root := widget.NewPanel(theme.RGB(20, 30, 40))
	root.ShowHeader = false
	root.SetBounds(tileScreen())
	m := NewStartMenu(tm, NewStaticAppCatalog(tileApps()...))
	m.Screen = tileScreen()
	m.SetBounds(tileAnchor())
	root.AddChild(m)
	eng := testengine.New(b, tileScreenW, tileScreenH, 60)
	eng.SetTooltipsEnabled(false)
	eng.SetScale(1.5)
	eng.SetRoot(root)
	src := newCountSource()
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour}))
	m.Open(tileAnchor())
	b.Cleanup(m.Close)
	finishAnimations()
	src.emit(TileContent{Title: "a"})
	eng.RenderOnce()
	eng.RenderOnce()

	titles := []string{"Понедельник", "Вторник"}
	if dur > 0 {
		src.emit(TileContent{Title: titles[1], Subtitle: "новое"})
		eng.RenderOnce() // кэши построены
	}
	ls := &m.v.live
	before := eng.Stats().Tiles
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if dur > 0 {
			ls.mu.Lock()
			if tr := ls.tiles["t2"].trans; tr != nil {
				tr.k = float64(i%20+1) / 21
			}
			ls.mu.Unlock()
			m.invalidateTile("t2")
		} else {
			src.emit(TileContent{Title: titles[i%2]})
		}
		eng.RenderOnce()
	}
	b.StopTimer()
	b.ReportMetric(float64(eng.Stats().Tiles-before)/float64(b.N), "tiles/op")
}
