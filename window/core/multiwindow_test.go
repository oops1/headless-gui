package core

import (
	"image"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/platform/native"
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
	native.NativeWindow
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
	nw := &closableStub{}
	child.native = nw
	child.current = image.NewRGBA(image.Rect(0, 0, 1, 1))

	main.children.mu.Lock()
	main.children.list = append(main.children.list, child)
	main.children.mu.Unlock()

	main.closeChildWindows()

	if !nw.closed.Load() {
		t.Error("окно ОС не закрыто")
	}
	if !eng.stopped.Load() {
		t.Error("движок второго окна не остановлен — горутины остались работать")
	}

	// Повторный вызов ничего не ломает: список уже пуст.
	main.closeChildWindows()
}

// secondaryFake — подставной бэкенд, помнящий пометку «вторичное окно» и её
// место относительно создания окна ОС.
type secondaryFake struct {
	initFake
}

func (f *secondaryFake) markSecondary() { f.log = append(f.log, "secondary") }

// Окно из OpenWindow помечается вторичным до создания окна ОС: на Win32 иначе
// его закрытие (WM_DESTROY → PostQuitMessage) завершило бы цикл сообщений
// главного окна — и всё приложение (отчёт RDP-клиента на v3.43.1). Главное
// окно пометки не получает.
func TestOpenWindow_ChildIsSecondary(t *testing.T) {
	var fakes []*secondaryFake
	old := newNativeWindow
	newNativeWindow = func() native.NativeWindow {
		f := &secondaryFake{}
		fakes = append(fakes, f)
		return f
	}
	t.Cleanup(func() { newNativeWindow = old })

	main := New(newStubEngine(), "главное")
	if err := main.bringUp(); err != nil {
		t.Fatalf("bringUp главного: %v", err)
	}
	child := New(newStubEngine(), "второе")
	if err := main.OpenWindow(child); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	if len(fakes) != 2 {
		t.Fatalf("бэкендов %d, ждал 2", len(fakes))
	}
	for _, e := range fakes[0].log {
		if e == "secondary" {
			t.Error("главное окно помечено вторичным — его закрытие не завершило бы приложение")
		}
	}
	log := fakes[1].log
	if len(log) == 0 || log[0] != "secondary" {
		t.Errorf("окно из OpenWindow: порядок %v, ждал пометку раньше create", log)
	}
}
