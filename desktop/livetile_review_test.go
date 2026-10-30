package desktop

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testengine"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// livetile_review_test.go — случаи из ревью 6.1 (живые плитки): смена
// источника у плитки с тем же id, круг тем, состояние в Post при
// переподписке, подписка из наблюдателя темы, Close изнутри Subscribe, гонка
// открытия, закрытия и источника, шаги таймера в потоке движка дерева.

// Смена источника у плитки с тем же id на открытом меню: состояния прежнего
// источника сброшены, перебор остановлен.
func TestLiveTileReview_SourceReplacedDropsForeignState(t *testing.T) {
	testreset.Globals(t)
	a, b := newCountSource(), newCountSource()
	m, _ := tiledMenu(t)
	m.SetTileGroups(liveGroups(LiveTile{Source: a, Period: time.Hour}))
	a.emit(TileContent{Title: "a1"})
	a.emit(TileContent{Title: "a2"})
	m.SetTileGroups(liveGroups(LiveTile{Source: b, Period: time.Hour}))
	if _, un, act := a.counts(); un != 1 || act != 0 {
		t.Errorf("прежний источник: отписок %d, подписок %d", un, act)
	}
	ls := &m.v.live
	ls.mu.Lock()
	st := ls.tiles["t2"]
	n, timer := len(st.queue), st.timer != nil
	ls.mu.Unlock()
	if n != 0 || timer {
		t.Errorf("после смены источника очередь %d, таймер %v; ждали пусто", n, timer)
	}
	if got := shownOf(m, "t2").Title; got != "t2" {
		t.Errorf("плитка с новым источником показывает состояние прежнего: %q", got)
	}
}

// Смена темы туда и обратно (плитки → без плиток → плитки) сохраняет
// показанное, а подписку на время вида без плиток снимает.
func TestLiveTileReview_ThemeRoundTripKeepsShown(t *testing.T) {
	testreset.Globals(t)
	src := newCountSource()
	m, _ := tiledMenu(t)
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour}))
	src.emit(TileContent{Title: "x"})
	if err := m.Theme().SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	if _, _, act := src.counts(); act != 0 {
		t.Errorf("вид без плиток: подписок %d", act)
	}
	if err := m.Theme().SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if got := shownOf(m, "t2").Title; got != "x" {
		t.Errorf("после круга тем показано %q, ждали прежнее", got)
	}
	if sub, _, act := src.counts(); sub != 2 || act != 1 {
		t.Errorf("после круга тем Subscribe %d, подписок %d; ждали 2 и 1", sub, act)
	}
}

// Состояние, уже поставленное в Post, доходит до плитки, даже если до его
// выполнения сменили плитки (тот же источник — подписка сохраняется).
func TestLiveTileReview_PostedStateSurvivesSetTileGroups(t *testing.T) {
	testreset.Globals(t)
	src := newCountSource()
	m, _ := tiledMenu(t)
	var mu sync.Mutex
	var posted []func()
	m.Post = func(fn func()) { mu.Lock(); posted = append(posted, fn); mu.Unlock() }
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour}))
	src.emit(TileContent{Title: "x"})
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour}))
	mu.Lock()
	list := posted
	posted = nil
	mu.Unlock()
	for _, fn := range list {
		fn()
	}
	if got := shownOf(m, "t2").Title; got != "x" || len(list) != 1 {
		t.Errorf("показано %q (постов %d), ждали x и 1", got, len(list))
	}
	if sub, un, _ := src.counts(); sub != 1 || un != 0 {
		t.Errorf("тот же источник переподписан: Subscribe %d, отписок %d", sub, un)
	}
}

// Источник присылает текущее синхронно из Subscribe, Post нет, а подписка идёт
// из наблюдателя темы: liveApply → tileRectAbs → метрики темы из обработчика
// темы. Проверка на взаимную блокировку.
func TestLiveTileReview_SyncInitialFromThemeHandlerNoDeadlock(t *testing.T) {
	testreset.Globals(t)
	src := newCountSource(TileContent{Title: "сразу"})
	m, _ := tiledMenu(t)
	m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: time.Hour}))
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := m.Theme().SetTheme(theme.ProfileWindows11); err != nil {
			t.Error(err)
		}
		if err := m.Theme().SetTheme(theme.ProfileWindows10); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("взаимная блокировка: смена темы с синхронным состоянием не завершилась")
	}
	if got := shownOf(m, "t2").Title; got != "сразу" {
		t.Errorf("показано %q", got)
	}
}

// Источник зовёт fn синхронно из Subscribe и тут же закрывает меню (Close из
// Subscribe) — подписка не должна остаться у закрытого меню.
func TestLiveTileReview_CloseInsideSubscribe(t *testing.T) {
	testreset.Globals(t)
	m, _ := tiledMenu(t)
	var unsubbed atomic.Int32
	var subs atomic.Int32
	src := TileSourceFunc(func(fn func(TileContent)) func() {
		subs.Add(1)
		fn(TileContent{Title: "s"})
		m.Close()
		return func() { unsubbed.Add(1) }
	})
	m.SetTileGroups(liveGroups(LiveTile{Source: src}))
	if m.IsOpen() {
		t.Fatal("меню открыто")
	}
	if s, u := subs.Load(), unsubbed.Load(); s != u {
		t.Errorf("подписок %d, отписок %d — подписка осталась у закрытого меню", s, u)
	}
}

// Гонка: Close/Open с одной горутины, SetTileGroups с другой, источник шлёт с
// третьей, таймер перебора короткий. Инварианты: после финального закрытия
// подписок нет, новых постановок в Post нет, таймеры молчат.
func TestLiveTileReview_StressOpenCloseEmitRace(t *testing.T) {
	testreset.Globals(t)
	src := newCountSource(TileContent{Title: "init"})
	m, _ := tiledMenu(t)
	var posts atomic.Int64
	var postedAfterClose atomic.Int64
	var closedFinal atomic.Bool
	queue := make(chan func(), 4096)
	m.Post = func(fn func()) {
		posts.Add(1)
		if closedFinal.Load() {
			postedAfterClose.Add(1)
		}
		select {
		case queue <- fn:
		default:
		}
	}
	drain := func() {
		for {
			select {
			case fn := <-queue:
				fn()
			default:
				return
			}
		}
	}
	lt := LiveTile{Source: src, Period: time.Millisecond, Queue: 3}
	m.SetTileGroups(liveGroups(lt))

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	// «Поток интерфейса»: Open/Close/SetTileGroups/смена темы — последовательно,
	// как того требует Flyout (поля раскладки пишутся из одного потока).
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			drain()
			switch i % 7 {
			case 0, 4:
				m.Close()
			case 1, 5:
				m.Open(tileAnchor())
			case 2:
				m.SetTileGroups(tileGroups()) // живых нет
			case 3:
				m.SetTileGroups(liveGroups(lt))
			case 6:
				_ = m.Theme().SetTheme(theme.ProfileWindows11)
				_ = m.Theme().SetTheme(theme.ProfileWindows10)
			}
			time.Sleep(200 * time.Microsecond)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			src.emit(TileContent{Title: "e"})
			time.Sleep(100 * time.Microsecond)
		}
	}()
	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()

	drain()
	m.Close()
	drain()
	closedFinal.Store(true)
	time.Sleep(50 * time.Millisecond)
	p := posts.Load()
	src.emit(TileContent{Title: "late"})
	src.emitStale(TileContent{Title: "stale"})
	time.Sleep(50 * time.Millisecond)
	if sub, un, act := src.counts(); act != 0 {
		t.Errorf("после закрытия подписок %d (Subscribe %d, отписок %d)", act, sub, un)
	}
	if got := posts.Load(); got != p {
		t.Errorf("закрытое меню ставило работу в Post: %d → %d", p, got)
	}
	if n := postedAfterClose.Load(); n != 0 {
		t.Errorf("Post после финального закрытия: %d", n)
	}
	ls := &m.v.live
	ls.mu.Lock()
	for id, st := range ls.tiles {
		if st.timer != nil || st.unsub != nil {
			t.Errorf("плитка %s после закрытия: таймер %v, отписка %v", id, st.timer != nil, st.unsub != nil)
		}
	}
	ls.mu.Unlock()
}

// Без StartMenu.Post шаги таймера перебора идут в поток движка, которому
// принадлежит дерево меню (widget.PostFor), а не с горутины таймера: под
// -race открытие и закрытие в потоке движка не гоняются с перебором.
func TestLiveTileReview_TimerStepsRunOnOwnerEngine(t *testing.T) {
	testreset.Globals(t)
	t.Cleanup(widget.StopAllAnimations)
	root := widget.NewPanel(theme.RGB(20, 30, 40))
	root.ShowHeader = false
	root.SetBounds(tileScreen())
	m := NewStartMenu(managerFor(t, theme.ProfileWindows10), NewStaticAppCatalog(tileApps()...))
	m.Screen = tileScreen()
	m.SetBounds(tileAnchor())
	root.AddChild(m)
	if widget.PostFor(m, func() {}) {
		t.Fatal("PostFor без движка: ждали false")
	}
	eng := testengine.New(t, tileScreenW, tileScreenH, 60)
	eng.SetTooltipsEnabled(false)
	eng.SetRoot(root)
	eng.Start()
	t.Cleanup(func() { eng.Post(m.Close); eng.Flush() })

	src := newCountSource(TileContent{Title: "a"}, TileContent{Title: "b"})
	eng.Post(func() { m.SetTileGroups(liveGroups(LiveTile{Source: src, Period: 200 * time.Microsecond})) })
	for i := 0; i < 100; i++ {
		eng.Post(m.Close)
		eng.Post(func() { m.Open(tileAnchor()) })
		time.Sleep(200 * time.Microsecond)
	}
	eng.Flush()
	seen := map[string]bool{}
	deadline := time.Now().Add(3 * time.Second)
	for len(seen) < 2 && time.Now().Before(deadline) {
		var title string
		eng.Post(func() { title = shownOf(m, "t2").Title })
		eng.Flush()
		seen[title] = true
		time.Sleep(time.Millisecond)
	}
	if !seen["a"] || !seen["b"] {
		t.Errorf("перебор в потоке движка не шёл: показано %v", seen)
	}
}
