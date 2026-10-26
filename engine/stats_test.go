package engine

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

func newStatsEngine(t *testing.T) *Engine {
	t.Helper()
	eng := New(200, 120, 30)
	t.Cleanup(eng.Stop)
	root := widget.NewPanel(color.RGBA{R: 40, G: 80, B: 120, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 120))
	eng.SetRoot(root)
	return eng
}

// Кадр считается, его тайлы и время отрисовки попадают в счётчики.
func TestStats_FrameCounted(t *testing.T) {
	testreset.Globals(t)
	eng := newStatsEngine(t)
	before := eng.Stats()
	eng.RenderOnce()
	s := eng.Stats()
	if got := s.Frames - before.Frames; got != 1 {
		t.Fatalf("Frames: прибавилось %d, ждали 1", got)
	}
	if s.Tiles == 0 {
		t.Error("Tiles = 0 после первого кадра: весь холст новый")
	}
}

// Время кадра накапливается. Часы на Windows грубые (stats.go), поэтому
// одиночный кадр может записаться нулём: рисуем полные кадры большого холста,
// пока оба счётчика не сдвинутся.
func TestStats_TimeAccumulates(t *testing.T) {
	testreset.Globals(t)
	sc := buildPipelineDesktopApps()
	defer sc.closeAll()
	eng := sc.eng
	deadline := time.Now().Add(5 * time.Second)
	for {
		eng.Invalidate()
		eng.renderFrame()
		s := eng.Stats()
		if s.DrawNanos > 0 && s.DiffNanos > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("за 5 с полных кадров время не учтено: draw %d, diff %d", s.DrawNanos, s.DiffNanos)
		}
	}
}

// slowSink — сток, который заведомо занимает время: его доставка видна и на
// грубых часах.
type slowSink struct{}

func (slowSink) Present(output.Frame) { time.Sleep(20 * time.Millisecond) }

// Время доставки включает работу синхронного стока.
func TestStats_DeliverTime(t *testing.T) {
	testreset.Globals(t)
	eng := newStatsEngine(t)
	eng.SetFrameSink(slowSink{})
	eng.deliver(output.Frame{})
	if got := eng.Stats().DeliverNanos; got < uint64(10*time.Millisecond) {
		t.Fatalf("DeliverNanos = %v, сток спал 20 мс", time.Duration(got))
	}
}

// Кадр, выброшенный из переполненного канала, учитывается как потерянный.
func TestStats_FrameLost(t *testing.T) {
	testreset.Globals(t)
	eng := newStatsEngine(t)
	n := cap(eng.frames)
	for i := 0; i < n; i++ {
		eng.deliver(output.Frame{})
	}
	if got := eng.Stats().FramesLost; got != 0 {
		t.Fatalf("FramesLost = %d, пока канал не полон", got)
	}
	eng.deliver(output.Frame{})
	s := eng.Stats()
	if s.FramesLost != 1 {
		t.Fatalf("FramesLost = %d после переполнения, ждали 1", s.FramesLost)
	}
}

// Очередь Post: поставлено, выполнено, ждёт.
func TestStats_PostQueue(t *testing.T) {
	testreset.Globals(t)
	eng := newStatsEngine(t)
	for i := 0; i < 3; i++ {
		eng.Post(func() {})
	}
	s := eng.Stats()
	if s.Posted != 3 || s.PostedRun != 0 || s.PostQueue != 3 {
		t.Fatalf("до кадра: posted %d, run %d, queue %d", s.Posted, s.PostedRun, s.PostQueue)
	}
	eng.RenderOnce() // без Start очередь разбирает RenderOnce
	s = eng.Stats()
	if s.Posted != 3 || s.PostedRun != 3 || s.PostQueue != 0 {
		t.Fatalf("после кадра: posted %d, run %d, queue %d", s.Posted, s.PostedRun, s.PostQueue)
	}
	eng.Post(nil) // nil в очередь не ставится
	if got := eng.Stats().Posted; got != 3 {
		t.Fatalf("Post(nil) посчитан: posted %d", got)
	}
}

// Запущенный движок без изменений пропускает тики и считает это.
func TestStats_SkippedTicks(t *testing.T) {
	testreset.Globals(t)
	eng := newStatsEngine(t)
	eng.SetRenderOnDemand(true)
	eng.Start()
	deadline := time.Now().Add(3 * time.Second)
	for eng.Stats().FramesSkipped == 0 {
		if time.Now().After(deadline) {
			t.Fatal("FramesSkipped = 0 за 3 с простоя")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
