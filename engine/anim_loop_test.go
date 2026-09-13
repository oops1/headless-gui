package engine

import (
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-68: анимация, заведённая на горутине движка, шагает только этот движок.
// Раньше каждый движок шагал общий реестр целиком, и тик анимации одного
// движка выполнялся на горутине другого.
func TestAnimations_StepOnOwnEngine(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	start := func(fps int) *Engine {
		e := New(64, 64, fps)
		root := widget.NewPanel(color.RGBA{A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, 64, 64))
		e.SetRoot(root)
		e.Start()
		return e
	}
	own := start(30)
	defer own.Stop()
	other := start(240) // шагает чаще: чужую анимацию схватил бы первым
	defer other.Stop()

	var ownGID uint64
	own.Post(func() { ownGID = curGoroutineID() })
	own.Flush()

	var mu sync.Mutex
	tickGIDs := map[uint64]int{}
	done := make(chan struct{})
	own.Post(func() {
		a := widget.Animate(150*time.Millisecond, nil, func(float64) {
			mu.Lock()
			tickGIDs[curGoroutineID()]++
			mu.Unlock()
		})
		a.OnDone = func() { close(done) }
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("анимация не завершилась")
	}
	mu.Lock()
	defer mu.Unlock()
	for gid, n := range tickGIDs {
		if gid != ownGID {
			t.Fatalf("%d тиков на чужой горутине %d (горутина движка %d): %v", n, gid, ownGID, tickGIDs)
		}
	}
	if len(tickGIDs) == 0 {
		t.Fatal("тиков не было")
	}
}
