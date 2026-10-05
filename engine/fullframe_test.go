package engine

import (
	"bytes"
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// columns — виджет, раскладка которого зависит от ширины (как у
// калькулятора): узкое окно — четыре колонки, широкое — шесть, в три ряда.
// Каждое движение мыши перекрашивает клетку под ней (цвет — от числа
// заходов), и только её тайлы уходят этим кадром. Потеряй потребитель кадр
// последнего захода в клетку — и клетка останется у него старой, пока не
// придёт полный кадр.
type columns struct {
	widget.Base
	mu     sync.Mutex
	visits map[int]int
}

const columnRows = 3

func (c *columns) count() int {
	if c.Bounds().Dx() > 1000 {
		return 6
	}
	return 4
}

func (c *columns) Draw(ctx widget.DrawContext) {
	b := c.Bounds()
	n := c.count()
	cw, ch := b.Dx()/n, b.Dy()/columnRows
	c.mu.Lock()
	defer c.mu.Unlock()
	for r := 0; r < columnRows; r++ {
		for i := 0; i < n; i++ {
			col := color.RGBA{uint8(30 * i), uint8(b.Dx() % 251), uint8(60 * r), 255}
			col.B = uint8(37 * c.visits[r*n+i])
			ctx.FillRect(b.Min.X+i*cw, b.Min.Y+r*ch, cw, ch, col)
		}
	}
}

func (c *columns) OnMouseMove(x, y int) {
	b := c.Bounds()
	if !image.Pt(x, y).In(b) {
		return
	}
	n := c.count()
	idx := min((y-b.Min.Y)/(b.Dy()/columnRows), columnRows-1)*n + min((x-b.Min.X)/(b.Dx()/n), n-1)
	c.mu.Lock()
	if c.visits == nil {
		c.visits = map[int]int{}
	}
	c.visits[idx]++
	c.mu.Unlock()
	c.Invalidate()
}

// slowConsumer ведёт себя как окно: копит тайлы в своём буфере, кадр чужого
// размера пропускает, а при смене размера заводит новый буфер — и отстаёт от
// движка на delay за кадр.
type slowConsumer struct {
	delay time.Duration
	mu    sync.Mutex
	cur   *image.RGBA
}

func (s *slowConsumer) run(frames <-chan output.Frame) {
	for f := range frames {
		time.Sleep(s.delay)
		s.mu.Lock()
		if f.Width != s.cur.Bounds().Dx() || f.Height != s.cur.Bounds().Dy() {
			s.mu.Unlock()
			continue
		}
		for _, t := range f.Tiles {
			for row := 0; row < t.H; row++ {
				off := s.cur.PixOffset(t.X, t.Y+row)
				copy(s.cur.Pix[off:off+t.W*4], t.Data[row*t.W*4:(row+1)*t.W*4])
			}
		}
		s.mu.Unlock()
	}
}

// resize — то же, что window.resizeTo: на горутине движка новый холст и
// новый пустой буфер окна.
func (s *slowConsumer) resize(e *Engine, change func()) {
	e.Post(func() {
		change()
		w, h := e.PhysicalSize()
		s.mu.Lock()
		s.cur = image.NewRGBA(image.Rect(0, 0, w, h))
		s.mu.Unlock()
	})
}

// settledEqual ждёт, пока потребитель догонит движок, и сравнивает его
// буфер с тем, что движок считает доставленным (front).
func settledEqual(t *testing.T, e *Engine, s *slowConsumer) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		e.frameMu.Lock()
		e.mu.RLock()
		front := e.canvas.front
		e.mu.RUnlock()
		s.mu.Lock()
		same := front.Rect == s.cur.Rect && bytes.Equal(front.Pix, s.cur.Pix)
		diff := 0
		if !same && front.Rect == s.cur.Rect {
			for i := range front.Pix {
				if front.Pix[i] != s.cur.Pix[i] {
					diff++
				}
			}
		}
		rect := s.cur.Rect
		s.mu.Unlock()
		e.frameMu.Unlock()
		if same {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("буфер потребителя %v не совпал с кадром движка %v: различается байт %d", rect, front.Rect, diff)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func newSlowRig(t *testing.T, delay time.Duration) (*Engine, *slowConsumer) {
	e := New(851, 570, 30)
	root := &columns{}
	e.SetRoot(root)
	s := &slowConsumer{delay: delay, cur: image.NewRGBA(image.Rect(0, 0, 851, 570))}
	e.Start()
	go s.run(e.Frames())
	t.Cleanup(e.Stop)
	return e, s
}

// hoverAround водит мышью по клеткам в темпе, превышающем темп потребителя:
// одно движение — один кадр.
func hoverAround(e *Engine, n int, w, h int) {
	for i := 0; i < n; i++ {
		e.SendMouseMove(10+(i*97)%(w-20), 10+(i*53)%(h-20))
		time.Sleep(35 * time.Millisecond)
	}
}

// Потребитель отстаёт на 100 мс за кадр — канал переполняется, кадры
// выбрасываются. Смены размера 851 → 1920 → 851 с наведением во время и
// после. Раньше в буфере оставалась смесь двух раскладок: выброшенный кадр
// движок считал доставленным и слал разности поверх картинки, которой у
// потребителя не было, а кадры прежнего размера ложились в новый буфер.
func TestSlowConsumer_ResizeKeepsPictureWhole(t *testing.T) {
	if testing.Short() {
		t.Skip("долгий тест")
	}
	e, s := newSlowRig(t, 100*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	s.resize(e, func() { e.SetResolution(1920, 1080) })
	time.Sleep(100 * time.Millisecond)
	hoverAround(e, 30, 1920, 1080)
	s.resize(e, func() { e.SetResolution(851, 570) })
	hoverAround(e, 20, 851, 570)
	e.SendMouseMove(5, 5)
	settledEqual(t, e, s)
}

// То же при смене масштаба (FitScale и перенос окна на монитор с другим DPI
// меняют масштаб, а не логический размер).
func TestSlowConsumer_ScaleKeepsPictureWhole(t *testing.T) {
	if testing.Short() {
		t.Skip("долгий тест")
	}
	e, s := newSlowRig(t, 100*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	s.resize(e, func() { e.SetScale(2) })
	hoverAround(e, 20, 851, 570)
	s.resize(e, func() { e.SetScale(1) })
	hoverAround(e, 20, 851, 570)
	e.SendMouseMove(5, 5)
	settledEqual(t, e, s)
}

// Потеря без смены размера: интерфейс меняется чаще, чем потребитель читает,
// а потом замирает. Полный кадр приходит и в статичный интерфейс — сам, без
// новых изменений.
func TestSlowConsumer_LostFrameHealsWhenIdle(t *testing.T) {
	if testing.Short() {
		t.Skip("долгий тест")
	}
	e, s := newSlowRig(t, 100*time.Millisecond)
	hoverAround(e, 40, 851, 570)
	settledEqual(t, e, s)
}

// Потребитель, который канал не читает вовсе, полных кадров не получает:
// места в канале у него не бывает, и движок не рисует их впустую.
func TestUnreadChannel_NoFullFrames(t *testing.T) {
	e := New(320, 200, 60)
	e.SetRoot(&columns{})
	var mu sync.Mutex
	var sizes []int
	e.SetFrameSink(sinkFunc(func(f output.Frame) {
		mu.Lock()
		sizes = append(sizes, len(f.Tiles))
		mu.Unlock()
	}))
	e.Start()
	defer e.Stop()
	time.Sleep(200 * time.Millisecond)
	hoverAround(e, 30, 320, 200)
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	all := ((320 + output.TileSize - 1) / output.TileSize) * ((200 + output.TileSize - 1) / output.TileSize)
	full := 0
	for _, n := range sizes[1:] { // первый кадр полный всегда
		if n == all {
			full++
		}
	}
	// Новая клетка — это двенадцатая часть холста; полных кадров после
	// первого быть не должно.
	if full > 0 {
		t.Errorf("полных кадров при нечитаемом канале: %d из %d", full, len(sizes)-1)
	}
}

type sinkFunc func(output.Frame)

func (f sinkFunc) Present(fr output.Frame) { f(fr) }

// Кадр несёт размер холста, под который снят.
func TestFrameCarriesCanvasSize(t *testing.T) {
	e := New(300, 200, 30)
	e.SetRoot(&columns{})
	e.mu.RLock()
	w, h := e.canvas.W, e.canvas.H
	e.mu.RUnlock()
	f := e.renderFrame()
	if f.Width != w || f.Height != h {
		t.Errorf("размер кадра %dx%d, холст %dx%d", f.Width, f.Height, w, h)
	}
}
