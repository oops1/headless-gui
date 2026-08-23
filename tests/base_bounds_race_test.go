// Тест гонки Base.bounds (ENGINE_ISSUES winline, v3.13.2): Invalidate из
// рабочей горутины одновременно с SetBounds из layout-прохода.
// Ловится только под -race; без детектора просто прогоняется.
package tests

import (
	"image"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func TestBase_BoundsRace_InvalidateVsSetBounds(t *testing.T) {
	lbl := widget.NewWin10Label("pty")
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() { // рабочая горутина: перерисовка виджета (как терминал из pty)
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				lbl.Invalidate()
				_ = lbl.Bounds()
			}
		}
	}()

	for i := 0; i < 5000; i++ { // layout-проход: двигаем виджет
		lbl.SetBounds(image.Rect(i%50, 0, i%50+120, 24))
	}
	close(stop)
	wg.Wait()
}
