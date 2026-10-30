package engine

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// Цикл кадров действительно рисует реже: движок на 100 кадров/с с отключённым
// режимом «по требованию» (кадр каждый тик) под потолком 10 даёт примерно
// десять кадров в секунду, а не сто.
func TestEngine_SetMaxFPS_SlowsLoop(t *testing.T) {
	testreset.Globals(t)
	e := New(64, 64, 100)
	e.SetRenderOnDemand(false)
	e.Start()
	defer e.Stop()

	// Кадры из канала разбираем, чтобы доставка не упиралась в переполнение.
	go func() {
		for range e.Frames() {
		}
	}()

	measure := func(d time.Duration) uint64 {
		before := e.RenderCount()
		time.Sleep(d)
		return e.RenderCount() - before
	}
	// Прогрев: цикл запущен, первый кадр нарисован.
	time.Sleep(100 * time.Millisecond)
	fast := measure(500 * time.Millisecond)

	e.SetMaxFPS(10)
	time.Sleep(150 * time.Millisecond) // подхват нового периода на тике
	slow := measure(1000 * time.Millisecond)

	if fast < 20 {
		t.Skipf("машина не тянет 100 кадров/с (получено %d за 0.5 с) — сравнивать нечего", fast)
	}
	if slow > 20 || slow < 4 {
		t.Fatalf("при потолке 10 кадров/с за секунду отрисовано %d кадров (без потолка за полсекунды было %d)", slow, fast)
	}
}
