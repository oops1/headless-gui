package window

import (
	"image"
	"image/color"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// recycleFakeNative — окно ОС, считающее блиты.
type recycleFakeNative struct {
	NativeWindow
	blits atomic.Int32
}

func (f *recycleFakeNative) BlitRGBA(*image.RGBA) { f.blits.Add(1) }

// Окно — единственный потребитель кадров: оно включает переработку буферов
// кадров движка и отдаёт их назад после наложения на свой буфер. Картинка
// в окне при этом верная.
func TestFramePump_EnablesRecyclingAndShowsFrames(t *testing.T) {
	eng := engine.New(96, 64, 60)
	defer eng.Stop()
	panel := widget.NewPanel(color.RGBA{R: 200, G: 30, B: 40, A: 255})
	panel.ShowHeader = false
	panel.SetBounds(image.Rect(0, 0, 96, 64))
	eng.SetRoot(panel)
	eng.Start()

	win := New(eng, "recycle")
	nat := &recycleFakeNative{}
	win.native = nat
	win.current = image.NewRGBA(image.Rect(0, 0, 96, 64))
	go win.framePump()

	deadline := time.Now().Add(5 * time.Second)
	for nat.blits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if nat.blits.Load() == 0 {
		t.Fatal("кадр не дошёл до блита")
	}
	if !eng.FrameRecycling() {
		t.Error("окно не включило переработку буферов кадров")
	}
	win.mu.Lock()
	o := (32*96 + 48) * 4
	got := win.current.Pix[o : o+4]
	ok := got[0] == 200 && got[1] == 30 && got[2] == 40 && got[3] == 255
	win.mu.Unlock()
	if !ok {
		t.Errorf("пиксель окна = %v, ожидался цвет панели", got)
	}
}
