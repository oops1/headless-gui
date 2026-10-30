package loop

import (
	"image"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/output"
)

// stubHost — движок для тестов цикла без движка: журнал вызовов, масштаб,
// граница сбоев выполняет fn как есть.
type stubHost struct {
	mu      sync.Mutex
	scale   float64
	calls   []string
	frames  atomic.Int32
	guarded []string
	lastGen uint64
	loop    *Loop
}

var _ Host = (*stubHost)(nil)

func (h *stubHost) note(s string) {
	h.mu.Lock()
	h.calls = append(h.calls, s)
	h.mu.Unlock()
}

func (h *stubHost) RenderFrame() output.Frame {
	h.frames.Add(1)
	return output.Frame{}
}
func (h *stubHost) StepAnimations(time.Time)           {}
func (h *stubHost) AnimationNeeded(time.Duration) bool { return false }
func (h *stubHost) FullFramePending() bool             { return false }
func (h *stubHost) MotionScope() func()                { return nil }
func (h *stubHost) BindLoop() func()                   { return func() {} }
func (h *stubHost) Scale() float64 {
	if h.scale == 0 {
		return 1
	}
	return h.scale
}
func (h *stubHost) GuardCall(where string, fn func()) {
	h.mu.Lock()
	h.guarded = append(h.guarded, where)
	h.mu.Unlock()
	fn()
}

// OnInvalidateAll запоминает поколение на момент вызова: оно должно быть
// прежним — хук идёт ДО полного повреждения и нового поколения.
func (h *stubHost) OnInvalidateAll() {
	h.mu.Lock()
	if h.loop != nil {
		h.lastGen = h.loop.Gen()
	}
	h.mu.Unlock()
	h.note("invalidate-all")
}

func newStub(t *testing.T, scale float64) (*Loop, *stubHost) {
	t.Helper()
	h := &stubHost{scale: scale}
	l := New(h, 60)
	h.loop = l
	return l, h
}

// Полная инвалидация: хук движка (поколение записи команд) — первым, потом
// полное повреждение и новое поколение.
func TestLoop_InvalidateCallsHostBeforeDamage(t *testing.T) {
	testreset.Globals(t)
	l, h := newStub(t, 1)
	before := l.Gen()
	l.Invalidate()
	if len(h.calls) != 1 || h.calls[0] != "invalidate-all" {
		t.Fatalf("вызовы хоста %v, ждали один invalidate-all", h.calls)
	}
	if h.lastGen != before {
		t.Errorf("хук увидел поколение %d, ждали прежнее %d", h.lastGen, before)
	}
	if l.Gen() == before {
		t.Error("поколение не выросло")
	}
	if _, all := l.ConsumeDamage(); !all {
		t.Error("полного повреждения нет")
	}
}

// Повреждение хранится в физических пикселях (масштаб хоста), ничейное — в
// логических; TakeForce отдаёт его в буфер вызывающего и очищает.
func TestLoop_DamageScaledForceLogical(t *testing.T) {
	testreset.Globals(t)
	l, _ := newStub(t, 2)
	l.ConsumeDamage()
	r := image.Rect(10, 10, 20, 30)
	l.InvalidateRect(r)
	rects, all := l.ConsumeDamage()
	if all || len(rects) != 1 || rects[0] != image.Rect(20, 20, 40, 60) {
		t.Fatalf("повреждение %v all=%v, ждали физическое (20,20)-(40,60)", rects, all)
	}
	buf := make([]image.Rectangle, 0, 4)
	force := l.TakeForce(buf)
	if len(force) != 1 || force[0] != r {
		t.Fatalf("ничейное %v, ждали логическое %v", force, r)
	}
	if again := l.TakeForce(force); len(again) != 0 {
		t.Errorf("ничейное не очистилось: %v", again)
	}
	// DamageRect — без ничейного.
	l.DamageRect(r)
	if f := l.TakeForce(nil); len(f) != 0 {
		t.Errorf("DamageRect заявил ничейное повреждение: %v", f)
	}
}

// У незапущенного цикла очередь Post разбирает Flush на горутине
// вызывающего, каждая функция — под границей сбоев хоста; функция, ставящая
// себя заново, тоже доходит.
func TestLoop_PostFlushNotStarted(t *testing.T) {
	testreset.Globals(t)
	l, h := newStub(t, 1)
	var n int
	var again func()
	again = func() {
		n++
		if n < 3 {
			l.Post(again)
		}
	}
	l.Post(again)
	if !l.InLoop() {
		t.Fatal("незапущенный цикл: InLoop должно быть true")
	}
	l.Flush()
	if n != 3 {
		t.Fatalf("выполнено %d, ждали 3", n)
	}
	if c := l.Counters(); c.Posted != 3 || c.PostedRun != 3 {
		t.Errorf("счётчики Post %+v, ждали 3 и 3", c)
	}
	if len(h.guarded) != 3 || h.guarded[0] != "Post" {
		t.Errorf("граница сбоев: %v", h.guarded)
	}
}

// Запущенный цикл готовит кадры и выполняет Post на своей горутине; Stop
// (MarkStopped + Halt) гасит таймеры и ждёт выхода.
func TestLoop_RunPostAndHalt(t *testing.T) {
	testreset.Globals(t)
	l, h := newStub(t, 1)
	if !l.MarkStarted() {
		t.Fatal("MarkStarted вернул false")
	}
	l.Invalidate()
	l.Run()
	var gid atomic.Uint64
	done := make(chan struct{})
	l.Post(func() {
		gid.Store(l.GoroutineID())
		close(done)
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Post не выполнился на запущенном цикле")
	}
	if gid.Load() == 0 {
		t.Error("функция Post шла не на горутине цикла")
	}
	tm := l.Every(time.Hour, func() {})
	deadline := time.Now().Add(3 * time.Second)
	for h.frames.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if h.frames.Load() == 0 {
		t.Error("цикл не приготовил ни одного кадра после Invalidate")
	}
	if !l.MarkStopped() {
		t.Fatal("MarkStopped вернул false")
	}
	l.Halt()
	l.CloseFrames()
	if !tm.Stopped() || l.TimerCount() != 0 {
		t.Errorf("таймер после Halt: stopped=%v, живых %d", tm.Stopped(), l.TimerCount())
	}
	if l.Running() || l.MarkStopped() || l.MarkStarted() {
		t.Error("после остановки: Running, повторный Stop или Start не no-op")
	}
}
