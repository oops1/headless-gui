package engine

import (
	"bytes"
	"image"
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

// fillNoise заполняет back холста детерминированным шумом с полосами, чтобы
// часть тайлов изменилась, а часть — нет.
func fillNoise(c *Canvas, seed byte) {
	pix := c.back.Pix
	for i := range pix {
		if (i/(c.back.Stride*64))%3 == 0 {
			pix[i] = byte(i) ^ seed
		}
	}
}

// Дифф через пул совпадает с последовательным по тайлам и данным.
func TestWorkPool_DiffMatchesSequential(t *testing.T) {
	testreset.Globals(t)
	eng := New(1920, 1080, 20)
	defer eng.Stop()
	c := eng.canvas
	c.blitBackground()
	c.diffAndSync()
	fillNoise(c, 0x5a)

	// Последовательный ответ — на копии front, чтобы второй дифф видел то же.
	front := append([]byte(nil), c.front.Pix...)
	want := c.diffTileRows(0, 0, c.tilesX-1, c.tilesY-1)
	wantData := make([][]byte, len(want))
	for i, tl := range want {
		wantData[i] = append([]byte(nil), tl.Data...)
	}
	copy(c.front.Pix, front)

	got := c.diffTiles(0, 0, c.tilesX-1, c.tilesY-1)
	if len(got) != len(want) {
		t.Fatalf("тайлов %d, последовательно %d", len(got), len(want))
	}
	for i := range got {
		g, w := got[i], want[i]
		if g.X != w.X || g.Y != w.Y || g.W != w.W || g.H != w.H || !bytes.Equal(g.Data, wantData[i]) {
			t.Fatalf("тайл %d расходится: (%d,%d) против (%d,%d)", i, g.X, g.Y, w.X, w.Y)
		}
	}
	if len(got) == 0 {
		t.Fatal("тест ничего не проверил: изменившихся тайлов нет")
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
			c.blitBackground()
			c.diffAndSync()
			for i := 0; i < 20; i++ {
				fillNoise(c, seed+byte(i))
				c.diffAndSync()
			}
			eng.Stop()
		}(byte(e * 17))
	}
	wg.Wait()

	// После всех остановок пул работает: новый движок диффит параллельно.
	eng := New(1920, 1080, 20)
	defer eng.Stop()
	c := eng.canvas
	c.blitBackground()
	c.diffAndSync()
	fillNoise(c, 1)
	if len(c.diffAndSync()) == 0 {
		t.Fatal("после остановки движков дифф не нашёл изменений")
	}
}

// Большой непрозрачный блит через пул кладёт те же пиксели, что и
// последовательный.
func TestWorkPool_OpaqueBlitMatchesSequential(t *testing.T) {
	testreset.Globals(t)
	eng := New(1920, 1080, 20)
	defer eng.Stop()
	c := eng.canvas
	src := image.NewRGBA(image.Rect(0, 0, 1600, 900))
	for i := range src.Pix {
		src.Pix[i] = byte(i * 7)
	}
	for i := 3; i < len(src.Pix); i += 4 {
		src.Pix[i] = 255
	}
	r := image.Rect(100, 80, 1700, 980)

	c.blitSrcBand(r, src, image.Point{})
	want := append([]byte(nil), c.back.Pix...)
	for i := range c.back.Pix {
		c.back.Pix[i] = 0
	}
	c.blitSrcRect(r, src, image.Point{})
	if !bytes.Equal(c.back.Pix, want) {
		t.Fatal("блит через пул расходится с последовательным")
	}
	if c.blitWork.src != nil {
		t.Fatal("задание блита держит картинку приложения после работы")
	}
}
