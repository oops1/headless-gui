package engine

import (
	"sync"
	"testing"
	"time"
)

// Очередь вызовов в горутину кадра.
//
// Дерево виджетов синхронизации не имеет: менять его из фоновой горутины —
// гонка с обходом в root.Draw. Post даёт обратный путь.

func TestPost_RunsInTheFrameGoroutine(t *testing.T) {
	e := New(200, 150, 60)
	e.SetRenderOnDemand(true)
	e.Start()
	defer e.Stop()

	done := make(chan struct{})
	e.Post(func() { close(done) })

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("отложенный вызов не выполнился")
	}
}

// Порядок постановки сохраняется: приложение планирует шаги, а не мешок
// независимых действий.
func TestPost_KeepsOrder(t *testing.T) {
	e := New(200, 150, 60)
	e.SetRenderOnDemand(true)
	e.Start()
	defer e.Stop()

	const n = 20
	var mu sync.Mutex
	var got []int
	done := make(chan struct{})

	for i := 0; i < n; i++ {
		i := i
		e.Post(func() {
			mu.Lock()
			got = append(got, i)
			last := len(got) == n
			mu.Unlock()
			if last {
				close(done)
			}
		})
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("выполнились не все вызовы")
	}
	mu.Lock()
	defer mu.Unlock()
	for i, v := range got {
		if v != i {
			t.Fatalf("порядок нарушен на позиции %d: %v", i, got)
		}
	}
}

// Постановка ИЗ отложенного вызова попадает в следующий проход, а не
// зацикливает текущий: иначе цикл, планирующий сам себя, никогда не дошёл бы
// до отрисовки.
func TestPost_FromInsidePostDoesNotStarveTheFrame(t *testing.T) {
	e := New(200, 150, 60)
	e.SetRenderOnDemand(true)
	e.Start()
	defer e.Stop()

	var steps int32
	done := make(chan struct{})
	var chain func()
	chain = func() {
		steps++
		if steps < 3 {
			e.Post(chain)
			return
		}
		close(done)
	}
	e.Post(chain)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("цепочка застряла на шаге %d", steps)
	}
}

// Без Start() очередь разбирают RenderOnce и RenderFrameNow: приложение
// попросило кадр, и отложенная работа обязана попасть именно в него.
func TestPost_DrainedByExplicitFrame(t *testing.T) {
	for _, tc := range []struct {
		name string
		draw func(*Engine)
	}{
		{"RenderOnce", func(e *Engine) { e.RenderOnce() }},
		{"RenderFrameNow", func(e *Engine) { e.RenderFrameNow() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := New(200, 150, 60)
			e.SetRenderOnDemand(true)

			ran := false
			e.Post(func() { ran = true })
			if ran {
				t.Fatal("вызов выполнился прямо в Post — это не очередь")
			}
			tc.draw(e)
			if !ran {
				t.Error("явный кадр не разобрал очередь")
			}
		})
	}
}

// Изменение дерева из отложенного вызова попадает в ТОТ ЖЕ кадр: очередь
// разбирается до отрисовки, а не после.
func TestPost_ChangesLandInTheSameFrame(t *testing.T) {
	e := New(200, 150, 60)
	e.SetRenderOnDemand(true)

	seen := false
	e.Post(func() {
		// Заявляем изменение — решение о пропуске кадра принимается по нему.
		e.Invalidate()
		seen = true
	})
	frame := e.RenderFrameNow()

	if !seen {
		t.Fatal("вызов не выполнился")
	}
	if len(frame.Tiles) == 0 {
		t.Error("damage, заявленный из отложенного вызова, не попал в этот кадр")
	}
}

// Post из любой горутины и одновременно с кадрами — под детектором гонок.
func TestPost_IsSafeFromManyGoroutines(t *testing.T) {
	e := New(200, 150, 60)
	e.SetRenderOnDemand(true)
	e.Start()
	defer e.Stop()

	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				e.Post(func() {
					mu.Lock()
					count++
					mu.Unlock()
				})
			}
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := count == 200
		mu.Unlock()
		if done {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Errorf("выполнено %d вызовов из 200", count)
}

// Пустой вызов не ставится: nil в очереди уронил бы горутину кадра.
func TestPost_NilIsIgnored(t *testing.T) {
	e := New(200, 150, 60)
	e.Post(nil)
	e.RenderOnce() // без правки здесь была бы паника
}
