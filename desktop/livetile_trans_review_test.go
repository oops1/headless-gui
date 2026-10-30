package desktop

import (
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/internal/testengine"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// livetile_trans_review_test.go — случаи из ревью 6.2 (переход живой плитки
// через кэш растра): хостовый оверлей строит кэши, наведение и смена акцента
// посреди перехода, повтор состояния и повторное открытие без перехода,
// сжатие набора плиток при прокрутке.

// transReviewTheme — как transTheme (сдвиг), плюс переход цвета при наведении
// длительностью hover.
func transReviewTheme(t *testing.T, dur, hover time.Duration) *theme.Manager {
	t.Helper()
	tm := managerFor(t, theme.ProfileWindows10)
	p := theme.NewProfile("win10-live-review")
	p.Parent = theme.ProfileWindows10
	p.Anims[AnimTileCycle] = theme.AnimSpec{Duration: dur}
	if hover > 0 {
		p.Anims[AnimHover] = theme.AnimSpec{Duration: hover}
	}
	p.SetMetric(KeyTileCycleKind, 0)
	if err := tm.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetTheme("win10-live-review"); err != nil {
		t.Fatal(err)
	}
	return tm
}

func pixAt(img *image.RGBA, x, y int) [4]byte {
	i := img.PixOffset(x, y)
	return [4]byte{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}
}

// Хостовые попапы (настоящее окно всегда ставит SetPopupSink, window.go):
// оверлей меню рисуется через translatingContext арены — он пробрасывает
// OffscreenRenderer, и кэши перехода строятся.
func TestLiveTileTransReview_HostedPopupBuildsCaches(t *testing.T) {
	testreset.Globals(t)
	const dur = 10 * time.Second
	s := newTransScene(t, transTheme(t, dur, 0), 1)
	frames := 0
	s.eng.SetPopupSink(func(fr []engine.PopupFrame) { frames += len(fr) })
	t.Cleanup(func() { s.eng.SetPopupSink(nil) })
	s.src.emit(TileContent{Title: "Первое"})
	s.eng.RenderOnce()
	s.eng.RenderOnce()
	s.src.emit(TileContent{Title: "Второе"})
	s.eng.RenderOnce()
	widget.StepAnimations(time.Now())
	s.eng.RenderOnce()
	if frames == 0 {
		t.Fatal("хост попапов не получил ни одного кадра — меню не вынесено, проверка пуста")
	}
	tr := s.trans()
	if tr == nil {
		t.Fatal("переход не заведён")
	}
	if tr.fromImg == nil || tr.toImg == nil {
		t.Errorf("хостовый попап: переход идёт (k=%.2f), но кэши растра не построены — плитка показана сразу новой, анимация лишь будит кадры", tr.k)
	}
}

// Наведение посреди перехода: пока идёт переход цвета, кэши не хранятся (стиль
// меняется каждый кадр), и после него плитка едет уже с рамкой наведения.
func TestLiveTileTransReview_HoverDuringTransitionFollowsStyleFade(t *testing.T) {
	testreset.Globals(t)
	const dur = 10 * time.Second
	s := newTransScene(t, transReviewTheme(t, dur, 200*time.Millisecond), 1)
	r := s.m.tileRectAbs("t2")
	s.src.emit(TileContent{Title: "Первое"})
	s.eng.RenderOnce()
	s.eng.RenderOnce()
	s.src.emit(TileContent{Title: "Второе"})
	s.eng.RenderOnce() // кэши построены в покое
	t0 := time.Now()
	widget.StepAnimations(t0)

	s.m.v.mu.Lock()
	s.m.v.hover = prefTile + "t2"
	s.m.v.mu.Unlock()
	s.m.Invalidate()
	s.eng.RenderOnce() // кэши перестроены под Hover (переход цвета только начался)

	// Первый шаг заводит часы перехода цвета, второй его заканчивает; плитка
	// на k=0,2. Кадр между ними — ещё посреди перехода цвета.
	widget.StepAnimations(t0.Add(time.Second))
	s.eng.RenderOnce()
	widget.StepAnimations(t0.Add(2 * time.Second))
	mid := crop(s.eng.RenderOnce(), r)
	widget.StepAnimations(t0.Add(2 * dur))
	if s.trans() != nil {
		t.Fatal("переход не закончился")
	}
	rest := crop(s.eng.RenderOnce(), r)

	// Левая рамка наведения (2 px) идёт по всей высоте: в композиции строка 5
	// показывает строку 5+off прежней картинки — тоже рамку.
	if got, want := pixAt(mid, 0, 5), pixAt(rest, 0, 5); got != want {
		t.Errorf("рамка наведения посреди перехода %v, после него %v: кэши заморозили стиль начала перехода цвета", got, want)
	}
	if pixAt(rest, 0, 5) == pixAt(rest, 20, 20) {
		t.Error("в покое рамка наведения не отличается от заливки — проверка ничего не проверяет")
	}
}

// Смена акцента (темы) посреди перехода: кэши сбрасываются, плитка едет уже
// в новом цвете, как остальное меню.
func TestLiveTileTransReview_AccentChangeMidTransitionRebuildsCaches(t *testing.T) {
	testreset.Globals(t)
	const dur = 10 * time.Second
	s := newTransScene(t, transReviewTheme(t, dur, 0), 1)
	r := s.m.tileRectAbs("t2")
	s.src.emit(TileContent{Title: "Первое"})
	s.eng.RenderOnce()
	s.eng.RenderOnce()
	s.src.emit(TileContent{Title: "Второе"})
	old := pixAt(crop(s.eng.RenderOnce(), r), 10, 10)
	t0 := time.Now()
	widget.StepAnimations(t0)

	s.m.Theme().SetAccent(color.RGBA{R: 200, G: 30, B: 30, A: 255})
	s.m.Invalidate()
	widget.StepAnimations(t0.Add(2 * time.Second))
	mid := pixAt(crop(s.eng.RenderOnce(), r), 10, 10)
	widget.StepAnimations(t0.Add(2 * dur))
	if s.trans() != nil {
		t.Fatal("переход не закончился")
	}
	rest := pixAt(crop(s.eng.RenderOnce(), r), 10, 10)
	if rest == old {
		t.Fatal("смена акцента не перекрасила плитку в покое — проверка ничего не проверяет")
	}
	if mid != rest {
		t.Errorf("посреди перехода заливка %v (прежний акцент %v), в покое после смены %v", mid, old, rest)
	}
}

// Источник прислал то же состояние, что уже показано (опрос без перемен):
// перехода нет, плитка не «прокручивается» на месте.
func TestLiveTileTransReview_SameStateEmitNoTransition(t *testing.T) {
	testreset.Globals(t)
	const dur = 400 * time.Millisecond
	s := newTransScene(t, transTheme(t, dur, 0), 1)
	r := s.m.tileRectAbs("t2")
	x := TileContent{Title: "Одно и то же", Subtitle: "без перемен"}
	s.src.emit(x)
	s.eng.RenderOnce()
	rest := crop(s.eng.RenderOnce(), r)
	if s.trans() != nil {
		t.Fatal("первое состояние завело переход")
	}
	s.src.emit(x)
	s.eng.RenderOnce()
	tr := s.trans()
	if tr == nil {
		return
	}
	t.Error("повтор того же состояния завёл переход")
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(dur / 2))
	mid := crop(s.eng.RenderOnce(), r)
	moved := false
	for y := 0; y < r.Dy(); y++ {
		if !rowEqual(mid, y, rest, y) {
			moved = true
			break
		}
	}
	if moved {
		t.Errorf("повтор того же состояния %q: плитка посреди «перехода» отличается от покоя — содержимое зримо прокручивается", x.Title)
	}
}

// Повторное открытие меню: источник синхронно присылает текущее состояние —
// первое состояние новой подписки перехода не заводит.
func TestLiveTileTransReview_ReopenNoTransition(t *testing.T) {
	testreset.Globals(t)
	const dur = 10 * time.Second
	s := newTransScene(t, transTheme(t, dur, 0), 1)
	x := TileContent{Title: "Одно"}
	s.src.emit(x)
	s.eng.RenderOnce()
	s.eng.RenderOnce()
	s.src.mu.Lock()
	s.src.initial = []TileContent{x}
	s.src.mu.Unlock()
	s.m.Close()
	finishAnimations()
	s.eng.RenderOnce()
	s.m.Open(tileAnchor())
	finishAnimations()
	s.eng.RenderOnce()
	if tr := s.trans(); tr != nil {
		t.Errorf("повторное открытие с тем же состоянием завело переход %q → %q", tr.from.Title, tr.to.Title)
	}
}

// transReviewManyScene — сцена newTransScene с восемью лишними группами плиток:
// область плиток прокручивается.
func transReviewManyScene(t *testing.T, tm *theme.Manager) *transScene {
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
	eng.SetRoot(root)
	src := newCountSource()
	groups := liveGroups(LiveTile{Source: src, Period: time.Hour})
	for gi := 0; gi < 8; gi++ {
		g := TileGroup{ID: fmt.Sprintf("x%d", gi), Title: fmt.Sprintf("Группа %d", gi)}
		for ti := 0; ti < 6; ti++ {
			id := fmt.Sprintf("x%d_%d", gi, ti)
			g.Tiles = append(g.Tiles, Tile{ID: TileID(id), App: AppID(id), Size: TileMedium, Content: TileContent{Title: id}})
		}
		groups = append(groups, g)
	}
	m.SetTileGroups(groups)
	m.Open(tileAnchor())
	t.Cleanup(m.Close)
	finishAnimations()
	eng.RenderOnce()
	eng.RenderOnce()
	return &transScene{eng: eng, m: m, src: src}
}

// Плитки сменились на меньший набор, пока область была прокручена:
// tileRectAbs зажимает прокрутку, как отрисовка (clampScroll), и в запущенном
// движке (кадры только по повреждению) новое состояние и шаги перехода
// доходят до экрана.
//
// RenderOnce для этого не годится: он готовит полный кадр и отбирает тайлы
// хэш-диффом, и заявленная область на него не влияет.
func TestLiveTileTransReview_ShrinkWhileScrolledReachesScreen(t *testing.T) {
	testreset.Globals(t)
	for _, tc := range []struct {
		name string
		dur  time.Duration
	}{{"мгновенно", 0}, {"переход", 300 * time.Millisecond}} {
		t.Run(tc.name, func(t *testing.T) {
			testreset.Globals(t)
			t.Cleanup(widget.StopAllAnimations)
			hold, fade := thinBarHold, thinBarFade
			thinBarHold, thinBarFade = 10*time.Millisecond, 10*time.Millisecond
			t.Cleanup(func() { thinBarHold, thinBarFade = hold, fade })

			root := widget.NewPanel(theme.RGB(20, 30, 40))
			root.ShowHeader = false
			root.SetBounds(tileScreen())
			m := NewStartMenu(transTheme(t, tc.dur, 0), NewStaticAppCatalog(tileApps()...))
			m.Screen = tileScreen()
			m.SetBounds(tileAnchor())
			root.AddChild(m)
			eng := testengine.New(t, tileScreenW, tileScreenH, 60)
			eng.SetTooltipsEnabled(false)
			eng.SetRoot(root)
			src := newCountSource()
			many := liveGroups(LiveTile{Source: src, Period: time.Hour})
			for gi := 0; gi < 8; gi++ {
				g := TileGroup{ID: fmt.Sprintf("x%d", gi), Title: fmt.Sprintf("Группа %d", gi)}
				for ti := 0; ti < 6; ti++ {
					id := fmt.Sprintf("x%d_%d", gi, ti)
					g.Tiles = append(g.Tiles, Tile{ID: TileID(id), App: AppID(id), Size: TileMedium, Content: TileContent{Title: id}})
				}
				many = append(many, g)
			}
			eng.Start()
			eng.Post(func() {
				m.SetTileGroups(many)
				m.Open(tileAnchor())
			})
			eng.Flush()
			t.Cleanup(func() { eng.Post(m.Close); eng.Flush() })
			settle := func() {
				// Затишье: кадры перестали идти.
				for i := 0; i < 40; i++ {
					f := eng.Stats().Frames
					time.Sleep(50 * time.Millisecond)
					eng.Flush()
					if eng.Stats().Frames == f {
						return
					}
				}
				t.Fatal("кадры не стихают")
			}
			settle()
			src.emit(TileContent{Title: "Первое"})
			eng.Flush()
			var r0 image.Rectangle
			eng.Post(func() {
				r0 = m.tileRectAbs("t2")
				c := r0.Min.Add(r0.Size().Div(2))
				m.OnMouseWheelPixels(c.X, c.Y, 0, 400)
			})
			eng.Flush()
			settle()
			eng.Post(func() { m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour})) })
			eng.Flush()
			settle()
			var ra image.Rectangle
			eng.Post(func() { ra = m.tileRectAbs("t2") })
			eng.Flush()

			before := eng.Stats().Tiles
			src.emit(TileContent{Title: "Второе"})
			eng.Flush()
			time.Sleep(2*tc.dur + 100*time.Millisecond)
			eng.Flush()
			settle()
			got := eng.Stats().Tiles - before
			t.Logf("место плитки %v, tileRectAbs после сжатия %v, тайлов на новое состояние %d", r0, ra, got)
			if got == 0 {
				t.Errorf("новое состояние (и переход) не отдали ни одного тайла: заявлена область %v, плитка нарисована в %v", ra, r0)
			}
		})
	}
}
