package engine

import (
	"image"
	"image/color"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/output"
	"github.com/oops1/headless-gui/v4/widget"
)

// Запрос полного кадра и вызов «есть что рисовать» при внешнем темпе —
// два долга сцены перед потребителем, который рисует кадр в свои буферы
// (пилот сцены WinLine, ENGINE_ISSUES «No way to have the next frame whole»,
// «PacingExternal leaves the shell's own changes undrawn»).

// waitFrames ждёт, пока сток получит больше n кадров.
func waitFrames(sink *recordingSink, n int) bool {
	deadline := time.Now().Add(time.Second)
	for sink.count() <= n && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	return sink.count() > n
}

func (s *recordingSink) last() output.Frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frames[len(s.frames)-1]
}

// Без запущенного цикла: после RequestFullFrame ближайший кадр несёт все
// тайлы, хотя интерфейс не менялся, а кадр за ним — снова разность.
func TestRequestFullFrame_NextFrameIsWhole(t *testing.T) {
	testreset.Globals(t)
	eng, _, root := pacedEngine(t)
	defer eng.Stop()

	eng.RenderFrameNow() // первый кадр — и так полный

	// Малое изменение: кадр — разность.
	chip := widget.NewPanel(color.RGBA{R: 200, A: 255})
	chip.ShowHeader = false
	chip.SetBounds(image.Rect(10, 10, 30, 30))
	root.AddChild(chip)
	if fr := eng.RenderFrameNow(); len(fr.Tiles) == 0 || len(fr.Tiles) >= gridTiles(eng) {
		t.Fatalf("кадр малого изменения несёт %d тайлов из %d — сцена не годится для проверки",
			len(fr.Tiles), gridTiles(eng))
	}

	eng.RequestFullFrame()
	eng.RequestFullFrame() // два запроса — один полный кадр
	fr := eng.RenderFrameNow()
	if got, want := len(fr.Tiles), gridTiles(eng); got != want {
		t.Fatalf("после RequestFullFrame кадр несёт %d тайлов, ждали все %d", got, want)
	}
	for _, tile := range fr.Tiles {
		if len(tile.Data) != tile.W*tile.H*4 {
			t.Fatalf("тайл %d,%d полного кадра без данных", tile.X, tile.Y)
		}
	}
	if fr := eng.RenderFrameNow(); len(fr.Tiles) >= gridTiles(eng) {
		t.Errorf("кадр после полного снова полный (%d тайлов): признак не снят", len(fr.Tiles))
	}
}

// Полный кадр не перезаписывает дерево: Draw виджетов, которые не менялись,
// ради него не зовётся.
func TestRequestFullFrame_DoesNotRedrawTree(t *testing.T) {
	testreset.Globals(t)
	eng, _, root := pacedEngine(t)
	defer eng.Stop()

	probe := &redrawProbe{}
	probe.SetBounds(image.Rect(40, 40, 80, 80))
	root.AddChild(probe)
	eng.RenderFrameNow()
	eng.RenderFrameNow()
	before := probe.draws.Load()

	eng.RequestFullFrame()
	if fr := eng.RenderFrameNow(); len(fr.Tiles) != gridTiles(eng) {
		t.Fatalf("кадр не полный: %d тайлов", len(fr.Tiles))
	}
	if got := probe.draws.Load(); got != before {
		t.Errorf("полный кадр по запросу вызвал Draw неизменившегося виджета (%d → %d)", before, got)
	}
}

// redrawProbe считает свои отрисовки.
type redrawProbe struct {
	widget.Base
	draws atomic.Int32
}

func (d *redrawProbe) Draw(ctx widget.DrawContext) {
	d.draws.Add(1)
	b := d.Bounds()
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), color.RGBA{G: 180, A: 255})
}

// Тикерный темп: полный кадр приходит сам, без изменений в интерфейсе.
func TestRequestFullFrame_TickerDeliversWithoutChanges(t *testing.T) {
	testreset.Globals(t)
	eng, sink, _ := pacedEngine(t)
	eng.Start()
	defer eng.Stop()

	if !waitFrames(sink, 0) {
		t.Fatal("стартовый кадр не пришёл")
	}
	time.Sleep(60 * time.Millisecond) // интерфейс стоит — кадров нет
	n := sink.count()

	eng.RequestFullFrame()
	if !waitFrames(sink, n) {
		t.Fatal("после RequestFullFrame кадр не пришёл: интерфейс не менялся, и тикер кадр пропустил")
	}
	if got := len(sink.last().Tiles); got != gridTiles(eng) {
		t.Errorf("кадр по RequestFullFrame несёт %d тайлов, ждали все %d", got, gridTiles(eng))
	}
}

// Внешний темп: RequestFullFrame сам запрашивает кадр.
func TestRequestFullFrame_ExternalPacingRequestsFrame(t *testing.T) {
	testreset.Globals(t)
	eng, sink, _ := pacedEngine(t)
	eng.SetPacing(PacingExternal)
	eng.Start()
	defer eng.Stop()

	eng.RequestFrame()
	if !waitFrames(sink, 0) {
		t.Fatal("первый кадр не пришёл")
	}
	n := sink.count()

	eng.RequestFullFrame()
	if !waitFrames(sink, n) {
		t.Fatal("RequestFullFrame при внешнем темпе кадра не дал")
	}
	if got := len(sink.last().Tiles); got != gridTiles(eng) {
		t.Errorf("кадр несёт %d тайлов, ждали все %d", got, gridTiles(eng))
	}
}

// Внешний темп: интерфейс изменился сам — потребителю об этом говорят, а
// кадра без его запроса по-прежнему нет.
func TestOnFrameNeeded_ToldAboutInvalidation(t *testing.T) {
	testreset.Globals(t)
	eng, sink, root := pacedEngine(t)
	eng.SetPacing(PacingExternal)
	var calls atomic.Int32
	eng.SetOnFrameNeeded(func() { calls.Add(1) })
	eng.Start()
	defer eng.Stop()

	// Первый кадр: о нём тоже говорят.
	waitCalls := func(more int32) bool {
		deadline := time.Now().Add(time.Second)
		for calls.Load() < more && time.Now().Before(deadline) {
			time.Sleep(2 * time.Millisecond)
		}
		return calls.Load() >= more
	}
	if !waitCalls(1) {
		t.Fatal("о первом кадре не сказали")
	}
	if n := sink.count(); n != 0 {
		t.Fatalf("без RequestFrame подготовлено %d кадров", n)
	}
	// Вызов повторяется, пока кадр не запрошен: пропущенный не теряется.
	seen := calls.Load()
	if !waitCalls(seen + 2) {
		t.Error("вызов не повторился, хотя кадр не запрошен")
	}

	eng.RequestFrame()
	if !waitFrames(sink, 0) {
		t.Fatal("кадр по запросу не пришёл")
	}
	// Всё нарисовано — тишина.
	time.Sleep(40 * time.Millisecond)
	quiet := calls.Load()
	time.Sleep(80 * time.Millisecond)
	if got := calls.Load(); got != quiet {
		t.Errorf("интерфейс не менялся, а вызов пришёл ещё %d раз", got-quiet)
	}

	// Изменение самого интерфейса — снова говорят.
	root.Background = color.RGBA{B: 220, A: 255}
	root.Invalidate()
	if !waitCalls(quiet + 1) {
		t.Fatal("об изменении интерфейса не сказали")
	}
	if n := sink.count(); n != 1 {
		t.Errorf("кадр подготовлен без запроса: %d кадров", n)
	}
}

// Потребитель, отвечающий запросом на каждый вызов, получает кадры всех
// изменений интерфейса — без своего тикера.
func TestOnFrameNeeded_AnsweredWithRequestDrawsChanges(t *testing.T) {
	testreset.Globals(t)
	eng, sink, root := pacedEngine(t)
	eng.SetPacing(PacingExternal)
	eng.SetOnFrameNeeded(eng.RequestFrame)
	eng.Start()
	defer eng.Stop()

	if !waitFrames(sink, 0) {
		t.Fatal("первый кадр не пришёл")
	}
	for i := 0; i < 3; i++ {
		n := sink.count()
		root.Background = color.RGBA{R: uint8(40 + 60*i), A: 255}
		root.Invalidate()
		if !waitFrames(sink, n) {
			t.Fatalf("изменение %d не нарисовано", i)
		}
	}
}

// Тикерный темп: вызов не нужен и не приходит.
func TestOnFrameNeeded_SilentUnderTicker(t *testing.T) {
	testreset.Globals(t)
	eng, sink, root := pacedEngine(t)
	var calls atomic.Int32
	eng.SetOnFrameNeeded(func() { calls.Add(1) })
	eng.Start()
	defer eng.Stop()

	root.Background = color.RGBA{G: 90, A: 255}
	root.Invalidate()
	if !waitFrames(sink, 0) {
		t.Fatal("кадр не пришёл")
	}
	time.Sleep(60 * time.Millisecond)
	if n := calls.Load(); n != 0 {
		t.Errorf("при тикерном темпе вызов пришёл %d раз", n)
	}
}

// Снятый вызов больше не приходит.
func TestOnFrameNeeded_NilRemoves(t *testing.T) {
	testreset.Globals(t)
	eng, _, _ := pacedEngine(t)
	eng.SetPacing(PacingExternal)
	var calls atomic.Int32
	eng.SetOnFrameNeeded(func() { calls.Add(1) })
	eng.SetOnFrameNeeded(nil)
	eng.Start()
	defer eng.Stop()

	time.Sleep(80 * time.Millisecond)
	if n := calls.Load(); n != 0 {
		t.Errorf("снятый вызов пришёл %d раз", n)
	}
}
