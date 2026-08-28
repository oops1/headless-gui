package engine

import (
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// Внешний темп и сток кадров.
//
// Темп задавал внутренний тикер, а переполнение канала кадров теряло кадр
// молча. Локальному выводу нужен темп по вертикальной синхронизации, а
// оболочке удалённого стола — возможность готовить кадр в своей горутине,
// там же, где она меняет сцену.

// recordingSink — сток, запоминающий полученные кадры.
type recordingSink struct {
	mu     sync.Mutex
	frames []output.Frame
}

func (s *recordingSink) Present(f output.Frame) {
	s.mu.Lock()
	s.frames = append(s.frames, f)
	s.mu.Unlock()
}

func (s *recordingSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.frames)
}

// pacedEngine — запущенный движок с простой сценой и стоком.
func pacedEngine(t *testing.T) (*Engine, *recordingSink, *widget.Panel) {
	t.Helper()

	eng := New(200, 120, 60)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 120))
	eng.SetRoot(root)

	sink := &recordingSink{}
	eng.SetFrameSink(sink)
	return eng, sink, root
}

// Без запроса кадры не готовятся вовсе — главное обещание внешнего темпа.
func TestPacing_ExternalPreparesNothingWithoutRequest(t *testing.T) {
	eng, sink, root := pacedEngine(t)
	eng.SetPacing(PacingExternal)
	eng.Start()
	defer eng.Stop()

	// Сцена меняется, время идёт — кадров быть не должно.
	root.Background = color.RGBA{R: 200, A: 255}
	root.Invalidate()
	time.Sleep(120 * time.Millisecond)

	if n := sink.count(); n != 0 {
		t.Errorf("без RequestFrame подготовлено %d кадров", n)
	}
}

// Запрос даёт кадр, и он приходит в сток.
func TestPacing_RequestFrameDelivers(t *testing.T) {
	eng, sink, root := pacedEngine(t)
	eng.SetPacing(PacingExternal)
	eng.Start()
	defer eng.Stop()

	root.Background = color.RGBA{G: 200, A: 255}
	root.Invalidate()
	eng.RequestFrame()

	deadline := time.Now().Add(time.Second)
	for sink.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sink.count() == 0 {
		t.Fatal("после RequestFrame кадр не пришёл")
	}
}

// Тикерный темп работает как раньше: кадры идут сами.
func TestPacing_TickerStillDrivesFrames(t *testing.T) {
	eng, sink, root := pacedEngine(t)
	eng.Start()
	defer eng.Stop()

	root.Background = color.RGBA{B: 200, A: 255}
	root.Invalidate()

	deadline := time.Now().Add(time.Second)
	for sink.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sink.count() == 0 {
		t.Error("при тикерном темпе кадр не подготовился сам")
	}
}

// Сток получает то же, что и канал: он альтернатива, а не замена.
func TestPacing_SinkAndChannelAgree(t *testing.T) {
	eng, sink, root := pacedEngine(t)
	eng.Start()
	defer eng.Stop()

	root.Background = color.RGBA{R: 90, G: 140, B: 200, A: 255}
	root.Invalidate()

	select {
	case frame := <-eng.Frames():
		if len(frame.Tiles) == 0 {
			t.Fatal("кадр из канала пуст")
		}
		if sink.count() == 0 {
			t.Error("канал кадр получил, сток — нет")
		}
	case <-time.After(time.Second):
		t.Fatal("кадр не пришёл в канал")
	}
}
