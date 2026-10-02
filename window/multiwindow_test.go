package window

import (
	"image"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// Окно у процесса было одно: Run создавал его и занимал горутину до закрытия.
// Второе окно приходилось делать вторым процессом — так и живёт Блокнот
// WinLine, у которого «окно = процесс».

// stubEngine — движок-заглушка: окну от него нужен кадровый канал, размер и
// корень.
type stubEngine struct {
	frames  chan output.Frame
	stopped atomic.Bool
}

func newStubEngine() *stubEngine { return &stubEngine{frames: make(chan output.Frame)} }

func (e *stubEngine) Frames() <-chan output.Frame                            { return e.frames }
func (e *stubEngine) CanvasSize() (int, int)                                 { return 320, 240 }
func (e *stubEngine) Root() widget.Widget                                    { return nil }
func (e *stubEngine) SendMouseMove(x, y int)                                 {}
func (e *stubEngine) SendMouseButton(x, y int, b widget.MouseButton, p bool) {}
func (e *stubEngine) SendKeyEvent(ev widget.KeyEvent)                        {}
func (e *stubEngine) CursorAt(x, y int) widget.Cursor                        { return widget.CursorArrow }
func (e *stubEngine) Stop()                                                  { e.stopped.Store(true) }

// closableStub — нативное окно, помнящее, что его закрыли.
type closableStub struct {
	NativeWindow
	closed atomic.Bool
}

func (c *closableStub) Close()                  { c.closed.Store(true) }
func (c *closableStub) GetSize() (int, int)     { return 320, 240 }
func (c *closableStub) SetSize(w, h int)        {}
func (c *closableStub) GetPosition() (int, int) { return 0, 0 }
func (c *closableStub) SetPosition(x, y int)    {}

func TestOpenWindow_Rejects(t *testing.T) {
	main := &Window{}

	if err := main.OpenWindow(nil); err == nil {
		t.Error("пустое окно принято")
	}

	child := New(newStubEngine(), "второе")
	if err := main.OpenWindow(child); err == nil {
		t.Error("окно открыто до того, как создано главное")
	} else if !strings.Contains(err.Error(), "главное окно") {
		t.Errorf("невнятная ошибка: %v", err)
	}

	// Главное окно есть, но дочернее уже открыто — повторять нельзя.
	main.native = &plainStub{}
	child.native = &plainStub{}
	if err := main.OpenWindow(child); err == nil {
		t.Error("окно открыто дважды")
	}
}

// Окна, открытые из главного, уходят вместе с ним: иначе их движки и насосы
// остались бы работать, а окна ОС — висеть на экране.
func TestCloseChildWindows(t *testing.T) {
	main := &Window{}
	main.native = &plainStub{}

	eng := newStubEngine()
	child := New(eng, "второе")
	native := &closableStub{}
	child.native = native
	child.current = image.NewRGBA(image.Rect(0, 0, 1, 1))

	main.children.mu.Lock()
	main.children.list = append(main.children.list, child)
	main.children.mu.Unlock()

	main.closeChildWindows()

	if !native.closed.Load() {
		t.Error("окно ОС не закрыто")
	}
	if !eng.stopped.Load() {
		t.Error("движок второго окна не остановлен — горутины остались работать")
	}

	// Повторный вызов ничего не ломает: список уже пуст.
	main.closeChildWindows()
}
