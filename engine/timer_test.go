package engine

import (
	"image"
	"image/color"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// Таймеры движка: After и Every исполняются на горутине цикла кадров и гаснут
// вместе с движком.

// timerEngine — запущенный движок, у которого кадры кто-то читает: иначе
// канал Frames заполнился бы и цикл встал на отправке.
func timerEngine(t *testing.T) *Engine {
	t.Helper()
	e := New(200, 150, 60)
	e.SetRenderOnDemand(true)
	e.Start()
	go func() {
		for range e.Frames() {
		}
	}()
	return e
}

// waitFor ждёт выполнения условия до двух секунд.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// timersLeft — сколько таймеров числится в реестре движка.
func timersLeft(e *Engine) int {
	e.timersMu.Lock()
	defer e.timersMu.Unlock()
	return len(e.timers)
}

func TestTimer_AfterRunsOnEngineGoroutine(t *testing.T) {
	e := timerEngine(t)
	defer e.Stop()

	var onLoop atomic.Bool
	done := make(chan struct{})
	tm := e.After(10*time.Millisecond, func() {
		onLoop.Store(e.loopGID.Load() == curGoroutineID())
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("After не сработал")
	}
	if !onLoop.Load() {
		t.Fatal("fn выполнился не на горутине цикла кадров")
	}
	if !tm.Stopped() {
		t.Error("разовый таймер после выполнения должен считаться остановленным")
	}
	waitFor(t, "реестр пуст", func() bool { return timersLeft(e) == 0 })
}

// Из fn можно трогать дерево виджетов, пока движок рисует кадры: под -race
// это проверяет, что таймер не выполняет работу на своей горутине.
func TestTimer_TouchesWidgetTreeWithoutRace(t *testing.T) {
	e := New(200, 150, 120)
	e.SetRenderOnDemand(true)
	root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 150))
	lbl := widget.NewLabel("0", color.RGBA{R: 240, G: 240, B: 240, A: 255})
	lbl.SetBounds(image.Rect(10, 10, 190, 40))
	root.AddChild(lbl)
	e.SetRoot(root)
	e.Start()
	go func() {
		for range e.Frames() {
		}
	}()
	defer e.Stop()

	var n atomic.Int32
	tm := e.Every(2*time.Millisecond, func() {
		lbl.SetText(string(rune('a' + n.Add(1)%26)))
		e.Invalidate()
	})
	waitFor(t, "10 срабатываний", func() bool { return n.Load() >= 10 })
	tm.Stop()
}

func TestTimer_AfterCancel(t *testing.T) {
	e := timerEngine(t)
	defer e.Stop()

	var calls atomic.Int32
	tm := e.After(30*time.Millisecond, func() { calls.Add(1) })
	tm.Stop()
	tm.Stop() // идемпотентно
	if !tm.Stopped() {
		t.Fatal("после Stop таймер должен быть остановлен")
	}
	time.Sleep(100 * time.Millisecond)
	e.Flush()
	if calls.Load() != 0 {
		t.Fatal("отменённый таймер сработал")
	}
	if n := timersLeft(e); n != 0 {
		t.Fatalf("в реестре осталось %d таймеров", n)
	}
}

// Отмена после срабатывания системного таймера, но до разбора очереди: вызов
// уже стоит в Post, и отмена обязана его погасить. Движок не запущен, поэтому
// очередь стоит, пока её не разберёт Flush.
func TestTimer_CancelAfterQueued(t *testing.T) {
	e := New(100, 100, 60)
	defer e.Stop()

	var calls atomic.Int32
	tm := e.After(0, func() { calls.Add(1) })
	waitFor(t, "вызов встал в очередь", func() bool {
		e.postMu.Lock()
		defer e.postMu.Unlock()
		return len(e.posted) > 0
	})
	tm.Stop()
	e.Flush()
	if calls.Load() != 0 {
		t.Fatal("вызов, отменённый после постановки в очередь, выполнился")
	}
}

func TestTimer_EveryRepeatsAndStops(t *testing.T) {
	e := timerEngine(t)
	defer e.Stop()

	var calls atomic.Int32
	tm := e.Every(5*time.Millisecond, func() { calls.Add(1) })
	waitFor(t, "5 повторов", func() bool { return calls.Load() >= 5 })
	if tm.Stopped() {
		t.Fatal("работающий Every не должен быть остановлен")
	}

	tm.Stop()
	e.Flush() // добираем уже стоявший в очереди вызов
	got := calls.Load()
	time.Sleep(60 * time.Millisecond)
	e.Flush()
	if calls.Load() != got {
		t.Fatalf("после Stop вызовы продолжились: %d -> %d", got, calls.Load())
	}
	if n := timersLeft(e); n != 0 {
		t.Fatalf("в реестре осталось %d таймеров", n)
	}
}

// Every останавливает сам себя из fn: следующий запуск не планируется.
func TestTimer_EveryStopsItself(t *testing.T) {
	e := timerEngine(t)
	defer e.Stop()

	var calls atomic.Int32
	var tm *Timer
	var mu sync.Mutex
	mu.Lock() // tm присваивается после возврата из Every; fn может успеть раньше
	tm = e.Every(2*time.Millisecond, func() {
		mu.Lock()
		defer mu.Unlock()
		if calls.Add(1) == 3 {
			tm.Stop()
		}
	})
	mu.Unlock()

	waitFor(t, "3 вызова", func() bool { return calls.Load() >= 3 })
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 3 {
		t.Fatalf("после Stop из fn вызовы продолжились: %d", calls.Load())
	}
	if !tm.Stopped() {
		t.Fatal("таймер должен быть остановлен")
	}
}

// Следующий запуск планируется после fn: медленный обработчик не копит
// очередь, между концом одного вызова и началом следующего проходит период.
func TestTimer_EveryWaitsForHandler(t *testing.T) {
	e := timerEngine(t)
	defer e.Stop()

	const period = 20 * time.Millisecond
	const work = 60 * time.Millisecond
	var mu sync.Mutex
	var starts, ends []time.Time
	tm := e.Every(period, func() {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		time.Sleep(work)
		mu.Lock()
		ends = append(ends, time.Now())
		mu.Unlock()
	})
	waitFor(t, "4 вызова", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ends) >= 4
	})
	tm.Stop()

	mu.Lock()
	defer mu.Unlock()
	for i := 1; i < len(starts) && i <= len(ends); i++ {
		// Небольшой допуск на точность часов.
		if gap := starts[i].Sub(ends[i-1]); gap < period-2*time.Millisecond {
			t.Errorf("вызов %d начался через %v после конца предыдущего, период %v", i, gap, period)
		}
	}
}

// Остановка движка гасит все его таймеры.
func TestTimer_EngineStopStopsTimers(t *testing.T) {
	e := timerEngine(t)

	var calls atomic.Int32
	every := e.Every(2*time.Millisecond, func() { calls.Add(1) })
	after := e.After(time.Hour, func() { calls.Add(1000) })
	waitFor(t, "первый повтор", func() bool { return calls.Load() >= 1 })

	e.Stop()

	if !every.Stopped() || !after.Stopped() {
		t.Fatal("после Stop движка таймеры должны быть остановлены")
	}
	if n := timersLeft(e); n != 0 {
		t.Fatalf("в реестре осталось %d таймеров", n)
	}
	got := calls.Load()
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != got {
		t.Fatal("таймер сработал после остановки движка")
	}
	every.Stop() // безопасно и после остановки движка
}

// У остановленного движка таймеры не заводятся.
func TestTimer_OnStoppedEngine(t *testing.T) {
	e := timerEngine(t)
	e.Stop()

	var calls atomic.Int32
	a := e.After(0, func() { calls.Add(1) })
	b := e.Every(time.Millisecond, func() { calls.Add(1) })
	if !a.Stopped() || !b.Stopped() {
		t.Fatal("таймер остановленного движка должен быть уже остановлен")
	}
	if n := timersLeft(e); n != 0 {
		t.Fatalf("в реестр попало %d таймеров", n)
	}
	time.Sleep(30 * time.Millisecond)
	e.Flush()
	if calls.Load() != 0 {
		t.Fatal("таймер остановленного движка сработал")
	}
	a.Stop()
	b.Stop()
}

func TestTimer_NilFunc(t *testing.T) {
	e := New(100, 100, 60)
	defer e.Stop()
	if !e.After(0, nil).Stopped() || !e.Every(time.Millisecond, nil).Stopped() {
		t.Fatal("таймер без функции должен быть остановлен")
	}
}

// Stop из многих горутин сразу и параллельно с остановкой движка: гонок нет,
// паники нет.
func TestTimer_ConcurrentStop(t *testing.T) {
	e := timerEngine(t)

	timers := make([]*Timer, 0, 50)
	for i := 0; i < 50; i++ {
		timers = append(timers, e.Every(time.Millisecond, func() {}))
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, tm := range timers {
				tm.Stop()
				_ = tm.Stopped()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.Stop()
	}()
	// Создание таймеров одновременно с остановкой: ни один не должен пережить движок.
	for i := 0; i < 50; i++ {
		e.Every(time.Millisecond, func() {})
	}
	wg.Wait()
	if n := timersLeft(e); n != 0 {
		t.Fatalf("в реестре осталось %d таймеров", n)
	}
}

// Утечек горутин нет: после остановки движка с десятками живых таймеров их
// число возвращается к исходному.
func TestTimer_NoGoroutineLeak(t *testing.T) {
	// Даём завершиться горутинам, оставшимся от предыдущих тестов.
	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()

	for round := 0; round < 3; round++ {
		e := timerEngine(t)
		for i := 0; i < 100; i++ {
			e.Every(time.Millisecond, func() {})
			e.After(time.Duration(i)*time.Millisecond, func() {})
			e.After(time.Hour, func() {})
		}
		time.Sleep(20 * time.Millisecond)
		e.Stop()
	}

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			buf = buf[:runtime.Stack(buf, true)]
			t.Fatalf("утечка горутин: было %d, стало %d\n%s", before, runtime.NumGoroutine(), buf)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Ожидающий таймер не держит горутин: тысяча таймеров на час не прибавляет
// ни одной.
func TestTimer_WaitingTimersHoldNoGoroutines(t *testing.T) {
	e := timerEngine(t)
	defer e.Stop()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	timers := make([]*Timer, 0, 1000)
	for i := 0; i < 1000; i++ {
		timers = append(timers, e.After(time.Hour, func() {}))
	}
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Fatalf("ожидающие таймеры завели горутины: было %d, стало %d", before, after)
	}
	for _, tm := range timers {
		tm.Stop()
	}
	if n := timersLeft(e); n != 0 {
		t.Fatalf("в реестре осталось %d таймеров", n)
	}
}
