package window

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// GG-68, пример из замечания: счётчик растёт и из OnClick кнопки, и из функций
// Post фоновой горутины. Под -race это была гонка: обработчик выполнялся в
// насосе событий ОС, функция из Post — в цикле кадров.
func TestInput_HandlerAndPostShareGoroutine(t *testing.T) {
	eng := engine.New(200, 100, 60)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 100))
	btn := widget.NewButton("x")
	btn.SetBounds(image.Rect(10, 10, 110, 40))
	root.AddChild(btn)
	eng.SetRoot(root)

	clicks := 0
	btn.OnClick = func() { clicks++ }

	fn := &inputNative{}
	s := &surface{eng: eng, native: fn, scale: 1}
	s.setupInput()
	eng.Start()
	defer eng.Stop()

	const n = 40
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // фоновая горутина приложения
		defer wg.Done()
		for i := 0; i < n; i++ {
			eng.Post(func() { clicks++ })
		}
	}()
	for i := 0; i < n; i++ { // насос событий ОС
		fn.onMove(20, 20)
		fn.onButton(20, 20, 0, true)
		fn.onButton(20, 20, 0, false)
	}
	wg.Wait()
	eng.Flush()

	got := 0
	eng.Post(func() { got = clicks })
	eng.Flush()
	if got != 2*n {
		t.Fatalf("щелчков %d, ждал %d (%d кнопкой и %d из Post)", got, 2*n, n, n)
	}
}
