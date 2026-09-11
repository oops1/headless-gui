package window

import (
	"image"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// Смена заголовка и право остановить закрытие — пункт 3 замечаний difftool.
//
// Заголовок задавался один раз в window.New, а SetOnClose нативного окна всегда
// отвечал «можно»: показать «*» при несохранённых правках было нечем, и правки
// молча терялись вместе с окном.

// fakeCloseNative — окно ОС, запоминающее заголовок, закрытие и колбэки.
type fakeCloseNative struct {
	NativeWindow
	mu      sync.Mutex
	title   string
	closed  int
	onClose func() bool
}

func (f *fakeCloseNative) SetTitle(s string) {
	f.mu.Lock()
	f.title = s
	f.mu.Unlock()
}
func (f *fakeCloseNative) Close()                        { f.closed++ }
func (f *fakeCloseNative) SetOnClose(fn func() bool)     { f.onClose = fn }
func (f *fakeCloseNative) SetOnResize(func(w, h int))    {}
func (f *fakeCloseNative) GetSize() (int, int)           { return 100, 100 }
func (f *fakeCloseNative) SetPosition(int, int)          {}
func (f *fakeCloseNative) GetPosition() (int, int)       { return 0, 0 }
func (f *fakeCloseNative) BlitRGBA(*image.RGBA)          {}
func (f *fakeCloseNative) Minimize()                     {}
func (f *fakeCloseNative) SetCornerRadius(int)           {}
func (f *fakeCloseNative) SetResizable(bool)             {}
func (f *fakeCloseNative) SetMinSize(int, int)           {}
func (f *fakeCloseNative) IsMaximized() bool             { return false }
func (f *fakeCloseNative) Maximize()                     {}
func (f *fakeCloseNative) Restore()                      {}
func (f *fakeCloseNative) Create(string, int, int) error { return nil }

// postEngine — движок, выполняющий Post сразу и считающий вызовы: проверяем,
// что вопрос уходит движку, а не задаётся в насосе событий ОС.
type postEngine struct {
	root  widget.Widget
	posts int
}

func (e *postEngine) Frames() <-chan output.Frame                        { return nil }
func (e *postEngine) CanvasSize() (int, int)                             { return 100, 100 }
func (e *postEngine) Root() widget.Widget                                { return e.root }
func (e *postEngine) SendMouseMove(int, int)                             {}
func (e *postEngine) SendMouseButton(int, int, widget.MouseButton, bool) {}
func (e *postEngine) SendKeyEvent(widget.KeyEvent)                       {}
func (e *postEngine) CursorAt(int, int) widget.Cursor                    { return 0 }
func (e *postEngine) Post(fn func())                                     { e.posts++; fn() }

func closeTestWindow(t *testing.T) (*Window, *fakeCloseNative, *postEngine, *widget.Window) {
	t.Helper()
	root := widget.NewWindow("Old", 100, 100)
	eng := &postEngine{root: root}
	win := New(eng, "Old")
	fn := &fakeCloseNative{}
	win.native = fn
	win.setupResizeClose()
	return win, fn, eng, root
}

// Заголовок меняется и в полосе движка, и у окна ОС.
func TestSetTitle_UpdatesBarAndOS(t *testing.T) {
	win, fn, _, root := closeTestWindow(t)

	win.SetTitle("difftool — main.go *")

	if root.Title != "difftool — main.go *" {
		t.Errorf("полоса движка: %q", root.Title)
	}
	fn.mu.Lock()
	got := fn.title
	fn.mu.Unlock()
	if got != "difftool — main.go *" {
		t.Errorf("окно ОС: %q", got)
	}
	if win.Title() != "difftool — main.go *" {
		t.Errorf("Title(): %q", win.Title())
	}
}

// До Run заголовок запоминается: окна ОС ещё нет.
func TestSetTitle_BeforeRunIsRemembered(t *testing.T) {
	win := New(&postEngine{}, "Old")
	win.SetTitle("New")
	if win.title != "New" {
		t.Errorf("заголовок до Run не запомнен: %q", win.title)
	}
}

// Без вопроса закрытие средствами ОС идёт, как и раньше.
func TestCloseRequest_NoHookClosesAsBefore(t *testing.T) {
	win, fn, eng, _ := closeTestWindow(t)

	if !fn.onClose() {
		t.Error("без хука окно ОС не отпущено")
	}
	if !win.closeRequested.Load() {
		t.Error("без хука закрытие не запрошено")
	}
	if eng.posts != 0 {
		t.Error("без хука вопрос зачем-то ушёл движку")
	}
}

// Отказ останавливает закрытие средствами ОС, а спрашивают на горутине
// движка — не в насосе событий ОС.
func TestCloseRequest_VetoFromOS(t *testing.T) {
	win, fn, eng, _ := closeTestWindow(t)
	asked := 0
	win.SetOnCloseRequest(func() bool { asked++; return false })

	if fn.onClose() {
		t.Error("окно ОС отпущено, хотя хук ещё не ответил")
	}
	if eng.posts != 1 || asked != 1 {
		t.Errorf("вопрос: через Post %d раз, задан %d раз", eng.posts, asked)
	}
	if win.closeRequested.Load() || fn.closed != 0 {
		t.Error("окно закрыто вопреки отказу")
	}
}

// Согласие закрывает окно.
func TestCloseRequest_AllowFromOS(t *testing.T) {
	win, fn, _, _ := closeTestWindow(t)
	win.SetOnCloseRequest(func() bool { return true })

	fn.onClose()

	if !win.closeRequested.Load() || fn.closed != 1 {
		t.Errorf("согласие не закрыло окно: запрошено=%v, Close=%d",
			win.closeRequested.Load(), fn.closed)
	}
}

// Кнопка × в полосе движка идёт тем же путём.
func TestCloseRequest_VetoFromTitleButton(t *testing.T) {
	win, fn, _, root := closeTestWindow(t)
	win.setupWidgetWindow()
	win.SetOnCloseRequest(func() bool { return false })

	if root.OnClose == nil {
		t.Fatal("кнопке × не назначено действие")
	}
	root.OnClose()

	if win.closeRequested.Load() || fn.closed != 0 {
		t.Error("кнопка × закрыла окно вопреки отказу")
	}
}

// Close — решение приложения, хук не спрашивается.
func TestCloseRequest_CloseIgnoresHook(t *testing.T) {
	win, fn, _, _ := closeTestWindow(t)
	asked := 0
	win.SetOnCloseRequest(func() bool { asked++; return false })

	win.Close()

	if asked != 0 {
		t.Error("Close спросил хук")
	}
	if fn.closed != 1 {
		t.Error("Close не закрыл окно")
	}
}
