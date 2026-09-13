package engine

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-68: одна горутина для отложенной работы и ввода.
//
// Под нативным окном ввод приходил из насоса событий ОС, а Post разбирал цикл
// кадров — обработчик кнопки и функция из Post шли на разных горутинах, и
// детектор гонок у потребителя это ловил. Здесь — то, что для этого должен
// гарантировать сам движок: очередь разбирает одна горутина, будится она сразу,
// кадр идёт следом, а подождать очередь можно без кадра.

// Post будит тикерный цикл сразу: при темпе раз в секунду работа не ждёт тика.
func TestPost_WakesTickerLoop(t *testing.T) {
	e := New(200, 150, 1)
	e.SetRenderOnDemand(true)
	e.Start()
	defer e.Stop()

	// Первый тик через секунду — даём циклу встать и ставим работу.
	time.Sleep(50 * time.Millisecond)
	done := make(chan struct{})
	start := time.Now()
	e.Post(func() { close(done) })
	select {
	case <-done:
		if d := time.Since(start); d > 400*time.Millisecond {
			t.Fatalf("работа ждала %v — цикл не разбужен, дождался тика", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("отложенный вызов не выполнился")
	}
}

// Кадр следует за разобранной работой, а не ждёт следующего тика: через
// очередь приходит ввод окна, и нажатие должно отзываться сразу.
func TestPost_FrameFollowsPostedChange(t *testing.T) {
	e := New(200, 150, 2) // тик раз в полсекунды
	e.SetRenderOnDemand(true)
	// Изменение должно быть ВИДИМЫМ: кадр отдаёт только изменившиеся тайлы, и
	// голый Invalidate без смены пикселей кадра не даёт.
	root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 150))
	lbl := widget.NewLabel("до", color.RGBA{R: 240, G: 240, B: 240, A: 255})
	lbl.SetBounds(image.Rect(10, 10, 190, 40))
	root.AddChild(lbl)
	e.SetRoot(root)
	e.Start()
	defer e.Stop()

	// Первый кадр (Start инвалидирует) — и пауза дольше темпа, чтобы кадру по
	// пробуждению не мешало ограничение частоты.
	select {
	case <-e.Frames():
	case <-time.After(2 * time.Second):
		t.Fatal("первого кадра нет")
	}
	time.Sleep(700 * time.Millisecond)

	start := time.Now()
	e.Post(func() { lbl.SetText("после") })
	select {
	case <-e.Frames():
		if d := time.Since(start); d > 300*time.Millisecond {
			t.Fatalf("кадр пришёл через %v — ждал тика, а не пришёл за работой", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("кадр за изменением из Post не пришёл")
	}
}

// С запущенным циклом RenderOnce очередь не разбирает: работа выполняется на
// горутине цикла, а не на той, что попросила кадр.
func TestPost_RenderOnceLeavesQueueToLoop(t *testing.T) {
	e := New(200, 150, 60)
	e.SetRenderOnDemand(true)
	e.Start()
	defer e.Stop()

	caller := curGoroutineID()
	var ranOn atomic.Uint64
	for i := 0; i < 20; i++ {
		e.Post(func() { ranOn.Store(curGoroutineID()) })
		e.RenderOnce()
		if got := ranOn.Load(); got == caller {
			t.Fatal("RenderOnce выполнил отложенную работу на своей горутине при запущенном цикле")
		}
	}
	e.Flush()
	if ranOn.Load() == caller || ranOn.Load() == 0 {
		t.Fatalf("работа выполнилась не на горутине цикла: %d (вызывающий %d)", ranOn.Load(), caller)
	}
}

// Ввод и Post, поставленные из разных горутин (насос ОС и фоновая задача),
// выполняются на ОДНОЙ горутине — горутине цикла.
func TestPost_InputAndPostShareOneGoroutine(t *testing.T) {
	e := New(200, 150, 60)
	e.SetRenderOnDemand(true)
	e.Start()
	defer e.Stop()

	var mu sync.Mutex
	gids := map[uint64]int{}
	record := func() {
		mu.Lock()
		gids[curGoroutineID()]++
		mu.Unlock()
	}

	var wg sync.WaitGroup
	for g := 0; g < 2; g++ { // «насос ОС» и «фоновая задача»
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				if g == 0 {
					e.Post(func() { e.SendMouseMove(10+i%50, 10); record() })
				} else {
					e.Post(record)
				}
			}
		}(g)
	}
	wg.Wait()
	e.Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(gids) != 1 {
		t.Fatalf("работа выполнялась на %d горутинах: %v", len(gids), gids)
	}
	for gid, n := range gids {
		if n != 200 {
			t.Fatalf("выполнено %d из 200", n)
		}
		if loop := e.loopGID.Load(); gid != loop {
			t.Fatalf("работа шла на горутине %d, а цикл — %d", gid, loop)
		}
	}
}

// Flush без запущенного цикла разбирает очередь на месте — и цепочку, которая
// ставит себя заново.
func TestFlush_NotStarted(t *testing.T) {
	e := New(200, 150, 60)
	steps := 0
	var chain func()
	chain = func() {
		steps++
		if steps < 3 {
			e.Post(chain)
		}
	}
	e.Post(chain)
	e.Flush()
	if steps != 3 {
		t.Fatalf("Flush выполнил %d шагов цепочки из 3", steps)
	}
}

// Flush с запущенным циклом ждёт, пока цикл выполнит поставленное.
func TestFlush_WaitsForLoop(t *testing.T) {
	e := New(200, 150, 60)
	e.Start()
	defer e.Stop()

	var done atomic.Bool
	e.Post(func() {
		time.Sleep(50 * time.Millisecond)
		done.Store(true)
	})
	e.Flush()
	if !done.Load() {
		t.Fatal("Flush вернулся раньше, чем выполнилась поставленная работа")
	}
}

// Flush из горутины движка не виснет: ждать собственную очередь изнутри нельзя.
func TestFlush_FromEngineGoroutineReturns(t *testing.T) {
	e := New(200, 150, 60)
	e.Start()
	defer e.Stop()

	done := make(chan struct{})
	e.Post(func() {
		e.Flush()
		close(done)
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Flush из горутины движка повис")
	}
}

// После Stop Flush не ждёт остановленный цикл.
func TestFlush_AfterStop(t *testing.T) {
	e := New(200, 150, 60)
	e.Start()
	e.Stop()

	ran := false
	e.Post(func() { ran = true })
	done := make(chan struct{})
	go func() {
		e.Flush()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Flush после Stop повис")
	}
	if !ran {
		t.Fatal("после Stop Flush должен разобрать очередь сам")
	}
}
