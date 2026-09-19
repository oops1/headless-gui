package window

import (
	"fmt"
	"image"
	"image/color"
	"reflect"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// GG-68: ввод окна выполняется на горутине движка, через его очередь.
//
// Окно звало SendMouseMove и SendKeyEvent прямо из насоса событий ОС, и
// обработчики виджетов шли параллельно с функциями из Post, анимациями и
// отрисовкой — без общего замка.

// queueEngine — движок, копящий Post до явного run и записывающий, что до
// него дошло.
type queueEngine struct {
	mu    sync.Mutex
	root  widget.Widget
	queue []func()
	log   []string
}

func (e *queueEngine) add(s string) {
	e.mu.Lock()
	e.log = append(e.log, s)
	e.mu.Unlock()
}

func (e *queueEngine) Post(fn func()) {
	e.mu.Lock()
	e.queue = append(e.queue, fn)
	e.mu.Unlock()
}

// run выполняет накопленную очередь, как цикл кадров, и возвращает её длину.
func (e *queueEngine) run() int {
	e.mu.Lock()
	q := e.queue
	e.queue = nil
	e.mu.Unlock()
	for _, fn := range q {
		fn()
	}
	return len(q)
}

func (e *queueEngine) taken() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.log
	e.log = nil
	return out
}

func (e *queueEngine) Frames() <-chan output.Frame { return nil }
func (e *queueEngine) CanvasSize() (int, int)      { return 100, 100 }
func (e *queueEngine) Root() widget.Widget         { return e.root }
func (e *queueEngine) SendMouseMove(x, y int)      { e.add(fmt.Sprintf("move %d,%d", x, y)) }
func (e *queueEngine) SendMouseButton(x, y int, b widget.MouseButton, p bool) {
	e.add(fmt.Sprintf("button %d,%d %d %v", x, y, b, p))
}
func (e *queueEngine) SendKeyEvent(k widget.KeyEvent) {
	e.add(fmt.Sprintf("key %d %q mod=%d %v", k.Code, k.Rune, k.Mod, k.Pressed))
}
func (e *queueEngine) SetModifiers(m widget.KeyMod) { e.add(fmt.Sprintf("mods %d", m)) }
func (e *queueEngine) SetResolution(w, h int)       { e.add(fmt.Sprintf("res %d,%d", w, h)) }
func (e *queueEngine) CursorAt(x, y int) widget.Cursor {
	if x >= 50 {
		return widget.Cursor(2)
	}
	return 0
}

// inputNative — окно ОС, отдающее колбэки тесту и копящее вызовы потока окна.
type inputNative struct {
	NativeWindow
	onMove   func(x, y int)
	onButton func(x, y, b int, p bool)
	onDown   func(vk int)
	onUp     func(vk int)
	onChar   func(r rune)
	onResize func(w, h int)
	invoked  []func()
	cursors  []int
}

func (f *inputNative) SetOnMouseMove(fn func(x, y int))              { f.onMove = fn }
func (f *inputNative) SetOnMouseButton(fn func(x, y, b int, p bool)) { f.onButton = fn }
func (f *inputNative) SetOnKeyDown(fn func(vk int))                  { f.onDown = fn }
func (f *inputNative) SetOnKeyUp(fn func(vk int))                    { f.onUp = fn }
func (f *inputNative) SetOnChar(fn func(r rune))                     { f.onChar = fn }
func (f *inputNative) SetOnResize(fn func(w, h int))                 { f.onResize = fn }
func (f *inputNative) SetOnClose(func() bool)                        {}
func (f *inputNative) SetCursor(c int)                               { f.cursors = append(f.cursors, c) }
func (f *inputNative) InvokeOnUIThread(fn func())                    { f.invoked = append(f.invoked, fn) }

func inputSurface(t *testing.T) (*surface, *inputNative, *queueEngine) {
	t.Helper()
	eng := &queueEngine{}
	fn := &inputNative{}
	s := &surface{eng: eng, native: fn, scale: 1}
	s.setupInput()
	return s, fn, eng
}

func wantLog(t *testing.T, eng *queueEngine, want ...string) {
	t.Helper()
	if got := eng.taken(); !reflect.DeepEqual(got, want) {
		t.Fatalf("до движка дошло:\n  %q\nждал:\n  %q", got, want)
	}
}

// Событие с насоса ОС не выполняется на месте — только из очереди движка.
func TestInput_RunsFromEngineQueue(t *testing.T) {
	_, fn, eng := inputSurface(t)

	fn.onButton(10, 20, 0, true)
	fn.onDown(VK_ENTER)
	fn.onChar('ж')
	wantLog(t, eng) // насос ничего не выполнил сам

	if n := eng.run(); n != 3 {
		t.Fatalf("в очереди %d событий, ждал 3", n)
	}
	wantLog(t, eng,
		fmt.Sprintf("button 10,20 %d true", widget.MouseLeft),
		"mods 0",
		fmt.Sprintf("key %d %q mod=0 true", widget.KeyEnter, rune(0)),
		fmt.Sprintf("key %d %q mod=0 true", widget.KeyUnknown, 'ж'),
	)
}

// Пока движение стоит в очереди, следующие только обновляют его координаты.
func TestInput_MouseMovesCoalesce(t *testing.T) {
	_, fn, eng := inputSurface(t)

	fn.onMove(1, 1)
	fn.onMove(2, 2)
	fn.onMove(3, 3)
	if n := eng.run(); n != 1 {
		t.Fatalf("в очереди %d движений, ждал одно", n)
	}
	wantLog(t, eng, "move 3,3")

	// Выполненное движение больше не дополняется: следующее встаёт заново.
	fn.onMove(4, 4)
	if n := eng.run(); n != 1 {
		t.Fatalf("после выполнения в очереди %d движений, ждал одно", n)
	}
	wantLog(t, eng, "move 4,4")
}

// Склейка не переставляет события: движение после нажатия не попадает в
// движение, стоящее до нажатия.
func TestInput_MoveClickMoveKeepsOrder(t *testing.T) {
	_, fn, eng := inputSurface(t)

	fn.onMove(1, 1)
	fn.onMove(2, 2)
	fn.onButton(2, 2, 0, true)
	fn.onMove(7, 7)
	fn.onMove(8, 8)
	fn.onButton(8, 8, 0, false)

	if n := eng.run(); n != 4 {
		t.Fatalf("в очереди %d событий, ждал 4", n)
	}
	wantLog(t, eng,
		"move 2,2",
		fmt.Sprintf("button 2,2 %d true", widget.MouseLeft),
		"move 8,8",
		fmt.Sprintf("button 8,8 %d false", widget.MouseLeft),
	)
}

// Модификаторы снимаются в момент события: к выполнению Ctrl уже отпущен, а
// символ, набранный с ним, всё равно едет с Ctrl.
func TestInput_ModifiersCapturedAtEvent(t *testing.T) {
	_, fn, eng := inputSurface(t)

	fn.onDown(VK_CONTROL)
	fn.onDown(VK_A)
	fn.onUp(VK_A)
	fn.onUp(VK_CONTROL)
	eng.run()

	ctrl := widget.ModCtrl
	wantLog(t, eng,
		fmt.Sprintf("mods %d", ctrl),
		fmt.Sprintf("mods %d", ctrl),
		fmt.Sprintf("key %d %q mod=%d true", widget.KeyA, rune(0), ctrl),
		fmt.Sprintf("mods %d", ctrl),
		fmt.Sprintf("key %d %q mod=%d false", widget.KeyA, rune(0), ctrl),
		"mods 0",
	)
}

// Форма курсора отдаётся окну ОС на его потоке и только при смене.
func TestInput_CursorOnUIThreadOnChange(t *testing.T) {
	_, fn, eng := inputSurface(t)

	fn.onMove(60, 10) // над «рукой»
	eng.run()
	if len(fn.cursors) != 0 {
		t.Fatal("курсор задан с горутины движка, а не на потоке окна")
	}
	if len(fn.invoked) != 1 {
		t.Fatalf("потоку окна передано %d вызовов, ждал 1", len(fn.invoked))
	}
	fn.invoked[0]()
	fn.invoked = nil
	if !reflect.DeepEqual(fn.cursors, []int{2}) {
		t.Fatalf("курсоры: %v", fn.cursors)
	}

	fn.onMove(70, 10) // та же форма
	eng.run()
	if len(fn.invoked) != 0 {
		t.Fatal("курсор той же формы отправлен повторно")
	}

	fn.onMove(10, 10) // стрелка
	eng.run()
	if len(fn.invoked) != 1 {
		t.Fatal("смена формы курсора не отправлена")
	}
}

// Движок без Post (своя реализация EngineAPI) получает события сразу — как
// до очереди.
func TestInput_EngineWithoutPostIsDirect(t *testing.T) {
	eng := &queueEngine{}
	legacy := struct{ EngineAPI }{eng}
	fn := &inputNative{}
	s := &surface{eng: legacy, native: fn, scale: 1}
	s.setupInput()

	fn.onMove(5, 6)
	fn.onButton(5, 6, 1, true)
	wantLog(t, eng, "move 5,6", fmt.Sprintf("button 5,6 %d true", widget.MouseRight))
}

// Ресайз меняет холст и границы корня на горутине движка.
func TestInput_ResizeOnEngine(t *testing.T) {
	root := widget.NewPanel(color.RGBA{A: 255})
	eng := &queueEngine{root: root}
	win := New(eng, "t")
	fn := &inputNative{}
	win.native = fn
	win.setupResizeClose()

	fn.onResize(300, 200)
	wantLog(t, eng)
	if root.Bounds().Dx() == 300 {
		t.Fatal("границы корня сменились в насосе ОС")
	}

	eng.run()
	wantLog(t, eng, "res 300,200")
	if b := root.Bounds(); b != image.Rect(0, 0, 300, 200) {
		t.Fatalf("границы корня после ресайза: %v", b)
	}
}

// События попапа идут той же очередью, что и события носителя.
func TestInput_PopupSharesCarrierQueue(t *testing.T) {
	s, fn, eng := inputSurface(t)
	h := newPopupHost(fn, fn, eng, 1, &s.in)
	pop := &inputNative{}
	hp := &hostedPopup{rect: image.Rect(40, 30, 90, 80)}
	hp.setOrigin(hp.rect, 1)
	h.windows[7] = hp
	h.setupPopupInput(pop, hp)

	fn.onMove(1, 1)
	pop.onMove(2, 3) // дополняет движение носителя: в физических координатах носителя
	pop.onButton(2, 3, 0, true)
	fn.onMove(9, 9)

	if n := eng.run(); n != 3 {
		t.Fatalf("в очереди %d событий, ждал 3", n)
	}
	wantLog(t, eng,
		"move 42,33",
		fmt.Sprintf("button 42,33 %d true", widget.MouseLeft),
		"move 9,9",
	)
}
