package desktop

import (
	"image"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/internal/testengine"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Живые плитки (ADR-0010, задача 6.1).

// countSource — источник плитки, который считает вызовы и помнит всех
// когда-либо подписанных (и отписанных) — чтобы позвать устаревшую подписку.
type countSource struct {
	mu         sync.Mutex
	subscribes int
	unsubs     int
	active     map[int]func(TileContent)
	all        []func(TileContent)
	next       int
	// initial — что присылать синхронно из Subscribe.
	initial []TileContent
}

func newCountSource(initial ...TileContent) *countSource {
	return &countSource{active: map[int]func(TileContent){}, initial: initial}
}

func (s *countSource) Subscribe(fn func(TileContent)) func() {
	s.mu.Lock()
	s.subscribes++
	s.next++
	id := s.next
	s.active[id] = fn
	s.all = append(s.all, fn)
	initial := append([]TileContent(nil), s.initial...)
	s.mu.Unlock()
	for _, c := range initial {
		fn(c)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.unsubs++
			delete(s.active, id)
			s.mu.Unlock()
		})
	}
}

// emit рассылает состояние действующим подписчикам.
func (s *countSource) emit(c TileContent) {
	s.mu.Lock()
	list := make([]func(TileContent), 0, len(s.active))
	for _, f := range s.active {
		list = append(list, f)
	}
	s.mu.Unlock()
	for _, f := range list {
		f(c)
	}
}

// emitStale зовёт все когда-либо выданные подписки, в том числе отписанные —
// так источник ведёт себя в гонке с отпиской.
func (s *countSource) emitStale(c TileContent) {
	s.mu.Lock()
	list := append([]func(TileContent){}, s.all...)
	s.mu.Unlock()
	for _, f := range list {
		f(c)
	}
}

func (s *countSource) counts() (subscribes, unsubs, active int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subscribes, s.unsubs, len(s.active)
}

// liveGroups — плитки tileGroups, у которых t2 живая.
func liveGroups(lt LiveTile) []TileGroup {
	g := tileGroups()
	g[0].Tiles[2].Live = lt
	return g
}

func shownOf(m *StartMenu, id TileID) TileContent {
	for _, g := range m.TileGroups() {
		for _, t := range g.Tiles {
			if t.ID == id {
				return m.liveContent(t)
			}
		}
	}
	return TileContent{}
}

// ─── Приёмка: закрытое меню — ни кадров, ни вызовов ─────────────────────────

// liveSettle ждёт затишья движка (барьер Flush, три стабильных замера) и
// отдаёт счётчик кадров.
func liveSettle(e *engine.Engine) uint64 {
	prev := ^uint64(0)
	stable := 0
	for i := 0; i < 80 && stable < 3; i++ {
		time.Sleep(25 * time.Millisecond)
		e.Flush()
		e.Flush()
		cur := e.Stats().Frames
		if cur == prev {
			stable++
		} else {
			stable = 0
		}
		prev = cur
	}
	return prev
}

func TestLiveTile_ClosedMenuNoFramesNoSourceCalls(t *testing.T) {
	testreset.Globals(t)
	t.Cleanup(widget.StopAllAnimations)

	root := widget.NewPanel(theme.RGB(20, 30, 40))
	root.ShowHeader = false
	root.SetBounds(tileScreen())
	m := NewStartMenu(managerFor(t, theme.ProfileWindows10), NewStaticAppCatalog(tileApps()...))
	m.Screen = tileScreen()
	m.SetBounds(tileAnchor())
	root.AddChild(m)

	eng := testengine.New(t, tileScreenW, tileScreenH, 60)
	eng.SetTooltipsEnabled(false)
	eng.SetRoot(root)
	// Свои постановки меню — отдельным счётчиком: Flush в liveSettle тоже
	// идёт через Post движка.
	var posts atomic.Int64
	m.Post = func(fn func()) { posts.Add(1); eng.Post(fn) }

	src := newCountSource()
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: 30 * time.Millisecond}))
	eng.Start()
	t.Cleanup(m.Close)

	// Меню ни разу не открывали: источник не вызван, его события кадров не дают.
	f0 := liveSettle(eng)
	if n := posts.Load(); n != 0 {
		t.Errorf("закрытое меню ставило работу в Post: %d", n)
	}
	src.emit(TileContent{Title: "до открытия"})
	if sub, _, _ := src.counts(); sub != 0 {
		t.Fatalf("закрытое меню подписалось на источник: Subscribe %d", sub)
	}
	if f := liveSettle(eng); f != f0 {
		t.Errorf("закрытое меню: кадров %d → %d", f0, f)
	}

	// Открыли: подписка одна, состояние доходит до плитки через Post и даёт кадр.
	eng.Post(func() { m.Open(tileAnchor()) })
	f1 := liveSettle(eng)
	if sub, _, act := src.counts(); sub != 1 || act != 1 {
		t.Fatalf("открытое меню: Subscribe %d, подписок %d, ждали 1 и 1", sub, act)
	}
	src.emit(TileContent{Title: "раз"})
	src.emit(TileContent{Title: "два"}) // очередь из двух — пошёл перебор по таймеру
	if f := liveSettle(eng); f == f1 {
		t.Error("состояние живой плитки не дало кадра")
	}
	if posts.Load() == 0 {
		t.Error("обновление плитки шло не через Post")
	}
	var shown TileContent
	eng.Post(func() { shown = shownOf(m, "t2") })
	eng.Flush()
	if shown.Title != "раз" && shown.Title != "два" {
		t.Errorf("плитка показывает %q, ждали состояние источника", shown.Title)
	}

	// Закрыли: отписка, таймер перебора остановлен.
	eng.Post(m.Close)
	f2 := liveSettle(eng)
	sub2, un2, act2 := src.counts()
	if act2 != 0 || un2 != 1 {
		t.Fatalf("после закрытия подписок %d, отписок %d, ждали 0 и 1", act2, un2)
	}
	p2 := posts.Load()
	src.emit(TileContent{Title: "после закрытия"})
	// Источник в гонке с отпиской зовёт устаревшую подписку — меню её глушит.
	src.emitStale(TileContent{Title: "устаревшее"})
	time.Sleep(150 * time.Millisecond) // несколько периодов перебора
	if f := liveSettle(eng); f != f2 {
		t.Errorf("закрытое меню: кадров %d → %d", f2, f)
	}
	if got := posts.Load(); got != p2 {
		t.Errorf("закрытое меню ставило работу в Post: %d → %d", p2, got)
	}
	if sub, _, _ := src.counts(); sub != sub2 {
		t.Errorf("закрытое меню звало Subscribe: %d → %d", sub2, sub)
	}
}

// ─── Очередь и перебор ──────────────────────────────────────────────────────

func TestLiveTile_QueueCyclesAndCaps(t *testing.T) {
	testreset.Globals(t)
	src := newCountSource()
	m, _ := tiledMenu(t)
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Queue: 3, Period: time.Hour}))
	if sub, _, act := src.counts(); sub != 1 || act != 1 {
		t.Fatalf("SetTileGroups на открытом меню: Subscribe %d, подписок %d", sub, act)
	}
	if got := shownOf(m, "t2").Title; got != "t2" {
		t.Errorf("до первого состояния плитка показывает %q, ждали Content", got)
	}
	for _, s := range []string{"a", "b", "c", "d"} {
		src.emit(TileContent{Title: s})
	}
	if got := shownOf(m, "t2").Title; got != "d" {
		t.Errorf("новое состояние не показано сразу: %q", got)
	}
	// Шаги перебора — напрямую (период час): очередь b, c, d, «a» вытеснено.
	ls := &m.v.live
	step := func() string {
		ls.mu.Lock()
		st := ls.tiles["t2"]
		sub, tick := st.sub, st.tick
		ls.mu.Unlock()
		m.liveAdvance("t2", st, sub, tick)
		return shownOf(m, "t2").Title
	}
	var seq []string
	for i := 0; i < 4; i++ {
		seq = append(seq, step())
	}
	if want := []string{"b", "c", "d", "b"}; !equalStrings(seq, want) {
		t.Errorf("перебор %v, ждали %v", seq, want)
	}
	// Модель потребителя состояний не видит.
	for _, g := range m.TileGroups() {
		for _, tl := range g.Tiles {
			if tl.ID == "t2" && tl.Content.Title != "t2" {
				t.Errorf("состояние источника попало в модель: %q", tl.Content.Title)
			}
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Таймер перебора шагает сам, пока меню открыто.
func TestLiveTile_TimerCycles(t *testing.T) {
	testreset.Globals(t)
	src := newCountSource()
	m, _ := tiledMenu(t)
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: 10 * time.Millisecond}))
	src.emit(TileContent{Title: "a"})
	src.emit(TileContent{Title: "b"})
	deadline := time.Now().Add(3 * time.Second)
	for shownOf(m, "t2").Title != "a" && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if got := shownOf(m, "t2").Title; got != "a" {
		t.Errorf("таймер не перебрал очередь: плитка показывает %q", got)
	}
}

// Обновление заявляет только прямоугольник плитки, без полной перерисовки.
func TestLiveTile_UpdateInvalidatesOnlyTile(t *testing.T) {
	testreset.Globals(t)
	src := newCountSource()
	m, _ := tiledMenu(t)
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour}))
	want := m.tileRectAbs("t2")
	if want.Empty() {
		t.Fatal("плитка t2 не на экране")
	}
	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)
	src.emit(TileContent{Title: "новое"})
	if fulls != 0 {
		t.Errorf("полных перерисовок %d, ждали 0", fulls)
	}
	if len(rects) != 1 || rects[0] != want {
		t.Errorf("заявленные области %v, ждали одну: плитку %v", rects, want)
	}
}

// ─── Жизненный цикл подписки ────────────────────────────────────────────────

// Повторное открытие: показано прежнее, пока источник не прислал своё; первое
// состояние новой подписки начинает очередь заново — текущее, присланное при
// подписке, не копится дублем.
func TestLiveTile_ReopenKeepsShownAndRestartsQueue(t *testing.T) {
	testreset.Globals(t)
	src := newCountSource()
	m, _ := tiledMenu(t)
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour}))
	src.emit(TileContent{Title: "a"})
	src.emit(TileContent{Title: "b"})
	m.Close()
	if got := shownOf(m, "t2").Title; got != "b" {
		t.Errorf("закрытое меню показывает %q, ждали последнее показанное", got)
	}
	m.Open(tileAnchor())
	if got := shownOf(m, "t2").Title; got != "b" {
		t.Errorf("после открытия до состояния плитка показывает %q, ждали прежнее", got)
	}
	src.emit(TileContent{Title: "c"})
	ls := &m.v.live
	ls.mu.Lock()
	n := len(ls.tiles["t2"].queue)
	ls.mu.Unlock()
	if n != 1 {
		t.Errorf("очередь после переподписки из %d состояний, ждали 1", n)
	}
	if sub, un, act := src.counts(); sub != 2 || un != 1 || act != 1 {
		t.Errorf("Subscribe %d, отписок %d, подписок %d; ждали 2, 1, 1", sub, un, act)
	}
}

// Источник, присылающий текущее синхронно из Subscribe, показан сразу.
func TestLiveTile_SyncInitialState(t *testing.T) {
	testreset.Globals(t)
	src := newCountSource(TileContent{Title: "сразу"})
	m, _ := tiledMenu(t)
	m.SetTileGroups(liveGroups(LiveTile{Source: src}))
	if got := shownOf(m, "t2").Title; got != "сразу" {
		t.Errorf("плитка показывает %q, ждали состояние из Subscribe", got)
	}
}

// Смена плиток на открытом меню: снятая живая плитка отписана, новая подписана.
func TestLiveTile_SetTileGroupsResubscribes(t *testing.T) {
	testreset.Globals(t)
	a, b := newCountSource(), newCountSource()
	m, _ := tiledMenu(t)
	m.SetTileGroups(liveGroups(LiveTile{Source: a}))
	g := tileGroups()
	g[1].Tiles[0].Live = LiveTile{Source: b}
	m.SetTileGroups(g)
	if _, un, act := a.counts(); un != 1 || act != 0 {
		t.Errorf("снятая живая плитка: отписок %d, подписок %d", un, act)
	}
	if sub, _, act := b.counts(); sub != 1 || act != 1 {
		t.Errorf("новая живая плитка: Subscribe %d, подписок %d", sub, act)
	}
	m.Close()
	if _, _, act := b.counts(); act != 0 {
		t.Errorf("после закрытия подписок %d", act)
	}
}

// У меню без плиток (Windows 11) источник не зовётся; смена темы на открытом
// меню снимает и возвращает подписку.
func TestLiveTile_OnlyTiledPresenterSubscribes(t *testing.T) {
	testreset.Globals(t)
	t.Cleanup(widget.StopAllAnimations)
	src := newCountSource()
	m := NewStartMenu(managerFor(t, theme.ProfileWindows11), NewStaticAppCatalog(tileApps()...))
	m.Screen = tileScreen()
	m.SetTileGroups(liveGroups(LiveTile{Source: src}))
	m.Open(tileAnchor())
	t.Cleanup(m.Close)
	if sub, _, _ := src.counts(); sub != 0 {
		t.Fatalf("меню Windows 11 подписалось на источник плитки: %d", sub)
	}
	if err := m.Theme().SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if sub, _, act := src.counts(); sub != 1 || act != 1 {
		t.Errorf("после перехода на плитки: Subscribe %d, подписок %d", sub, act)
	}
	if err := m.Theme().SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	if _, _, act := src.counts(); act != 0 {
		t.Errorf("после ухода с плиток подписок %d", act)
	}
}
