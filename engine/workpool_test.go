package engine

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// countTask считает выполненные части и может упасть на заданной.
type countTask struct {
	done    []atomic.Int32
	panicAt int
}

func (t *countTask) RunPart(i int) {
	t.done[i].Add(1)
	if i == t.panicAt {
		panic("часть упала")
	}
}

// Каждая часть выполняется ровно один раз.
func TestWorkPool_EveryPartOnce(t *testing.T) {
	testreset.Globals(t)
	for _, parts := range []int{1, 2, 7, 64} {
		task := &countTask{done: make([]atomic.Int32, parts), panicAt: -1}
		var b poolBatch
		for round := 0; round < 3; round++ {
			poolRun(task, &b, parts)
		}
		for i := range task.done {
			if got := task.done[i].Load(); got != 3 {
				t.Fatalf("parts=%d: часть %d выполнена %d раз за 3 прохода", parts, i, got)
			}
		}
	}
}

// Паника в части воркера доходит до вызывающего после завершения всех
// частей, и пул после неё работает.
func TestWorkPool_PanicReachesCaller(t *testing.T) {
	testreset.Globals(t)
	const parts = 8
	for _, at := range []int{0, parts - 1} {
		task := &countTask{done: make([]atomic.Int32, parts), panicAt: at}
		var b poolBatch
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("паника в части %d не дошла до вызывающего", at)
				}
			}()
			poolRun(task, &b, parts)
		}()
		for i := range task.done {
			if task.done[i].Load() != 1 {
				t.Fatalf("паника в части %d: часть %d не выполнена до возврата", at, i)
			}
		}
	}
	task := &countTask{done: make([]atomic.Int32, parts), panicAt: -1}
	var b poolBatch
	poolRun(task, &b, parts)
	for i := range task.done {
		if task.done[i].Load() != 1 {
			t.Fatalf("после паники пул не выполнил часть %d", i)
		}
	}
}

// Движки рисуют одновременно, пул общий; движок останавливают посреди
// чужой работы — пул живёт. Смысл теста — под -race.
func TestWorkPool_ConcurrentEnginesAndStop(t *testing.T) {
	testreset.Globals(t)
	var wg sync.WaitGroup
	for e := 0; e < 4; e++ {
		wg.Add(1)
		go func(seed byte) {
			defer wg.Done()
			eng := New(1280, 720, 20)
			c := eng.canvas
			c.BlitBackground()
			c.DiffAndSync()
			for i := 0; i < 20; i++ {
				fillNoise(c, seed+byte(i))
				c.DiffAndSync()
			}
			eng.Stop()
		}(byte(e * 17))
	}
	wg.Wait()

	// После всех остановок пул работает: новый движок диффит параллельно.
	eng := New(1920, 1080, 20)
	defer eng.Stop()
	c := eng.canvas
	c.BlitBackground()
	c.DiffAndSync()
	fillNoise(c, 1)
	if len(c.DiffAndSync()) == 0 {
		t.Fatal("после остановки движков дифф не нашёл изменений")
	}
}
