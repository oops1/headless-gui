package core

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

// Начальное положение и «развёрнуто» (initial.go, задание PixGo E18).
//
// Чистая арифметика и порядок, в котором bringUp настраивает бэкенд, — на
// любой платформе; живые окна — в platform/win32/initial_windows_test.go,
// провод X11 и Wayland — в initial_x11_linux_test.go и
// initial_wayland_linux_test.go.

// initFake — бэкенд, записывающий порядок вызовов: что получил до Create и с
// каким размером его создали.
type initFake struct {
	log       []string
	placement native.InitialPlacement
	placed    int
	createW   int
	createH   int
	maximized bool
}

func (f *initFake) SetInitialPlacement(p native.InitialPlacement) {
	f.log = append(f.log, "placement")
	f.placement, f.placed = p, f.placed+1
}

func (f *initFake) Create(title string, w, h int) error {
	f.log = append(f.log, "create")
	f.createW, f.createH = w, h
	// Как настоящий бэкенд: окно, о котором просили «развёрнуто», развёрнуто.
	f.maximized = f.placement.Maximized
	return nil
}
func (f *initFake) RunEventLoop() error                        { return nil }
func (f *initFake) Close()                                     {}
func (f *initFake) SetTitle(string)                            {}
func (f *initFake) SetSize(int, int)                           {}
func (f *initFake) SetResizable(bool)                          {}
func (f *initFake) SetMinSize(int, int)                        {}
func (f *initFake) GetSize() (int, int)                        { return f.createW, f.createH }
func (f *initFake) SetPosition(int, int)                       {}
func (f *initFake) GetPosition() (int, int)                    { return 0, 0 }
func (f *initFake) Minimize()                                  {}
func (f *initFake) Maximize()                                  {}
func (f *initFake) Restore()                                   {}
func (f *initFake) IsMaximized() bool                          { return f.maximized }
func (f *initFake) SetCornerRadius(int)                        {}
func (f *initFake) BlitRGBA(*image.RGBA)                       {}
func (f *initFake) SetOnResize(func(int, int))                 {}
func (f *initFake) SetOnClose(func() bool)                     {}
func (f *initFake) SetOnMouseMove(func(int, int))              {}
func (f *initFake) SetOnMouseButton(func(int, int, int, bool)) {}
func (f *initFake) SetOnKeyDown(func(int))                     {}
func (f *initFake) SetOnKeyUp(func(int))                       {}
func (f *initFake) SetOnChar(func(rune))                       {}

// initBringUp поднимает окно на подставном бэкенде с масштабом scale.
func initBringUp(t *testing.T, scale string, setup func(win *Window)) (*Window, *initFake) {
	t.Helper()
	return initBringUpOn(t, scale, engine.New(800, 600, 30), setup)
}

// syncEngine — движок, выполняющий Post сразу: события окна, поставленные в
// очередь, тест видит без запуска цикла движка.
type syncEngine struct{ *engine.Engine }

func (s syncEngine) Post(fn func()) { fn() }

func initBringUpOn(t *testing.T, scale string, eng EngineAPI, setup func(win *Window)) (*Window, *initFake) {
	t.Helper()
	t.Setenv("HEADLESS_GUI_SCALE", scale)
	f := &initFake{}
	old := newNativeWindow
	newNativeWindow = func() native.NativeWindow { return f }
	t.Cleanup(func() { newNativeWindow = old })

	win := New(eng, "initial")
	if setup != nil {
		setup(win)
	}
	if err := win.bringUp(); err != nil {
		t.Fatalf("bringUp: %v", err)
	}
	return win, f
}

// Положение и размер доходят до бэкенда ДО Create, а Create получает
// заказанный размер, а не размер холста по умолчанию.
func TestInitialBounds_ReachBackendBeforeCreate(t *testing.T) {
	win, f := initBringUp(t, "1", func(win *Window) { win.SetInitialBounds(120, 80, 640, 480) })

	if len(f.log) < 2 || f.log[0] != "placement" || f.log[1] != "create" {
		t.Fatalf("порядок вызовов %v, ждал placement перед create", f.log)
	}
	if want := (native.InitialPlacement{PosSet: true, X: 120, Y: 80, SizeSet: true}); f.placement != want {
		t.Errorf("бэкенд получил %+v, ждал %+v", f.placement, want)
	}
	if f.createW != 640 || f.createH != 480 {
		t.Errorf("Create(%d, %d), ждал 640x480", f.createW, f.createH)
	}
	if w, h := win.eng.CanvasSize(); w != 640 || h != 480 {
		t.Errorf("холст движка %dx%d, ждал 640x480", w, h)
	}
}

// Размер логический: на HiDPI окно получает физический, холст остаётся
// логическим.
func TestInitialBounds_SizeIsLogical(t *testing.T) {
	win, f := initBringUp(t, "2", func(win *Window) { win.SetInitialBounds(10, 20, 640, 480) })

	if f.createW != 1280 || f.createH != 960 {
		t.Errorf("Create(%d, %d), ждал физические 1280x960", f.createW, f.createH)
	}
	if w, h := win.eng.CanvasSize(); w != 640 || h != 480 {
		t.Errorf("холст %dx%d, ждал логические 640x480", w, h)
	}
	// Положение — экранное, масштабом не умножается.
	if f.placement.X != 10 || f.placement.Y != 20 {
		t.Errorf("положение %d,%d, ждал 10,20 без масштаба", f.placement.X, f.placement.Y)
	}
}

// Одно «развёрнуто» без границ: положения нет, размер прежний.
func TestInitialMaximized_Only(t *testing.T) {
	win, f := initBringUp(t, "1", func(win *Window) { win.SetInitialMaximized(true) })

	if want := (native.InitialPlacement{Maximized: true}); f.placement != want {
		t.Errorf("бэкенд получил %+v, ждал %+v", f.placement, want)
	}
	if f.createW != 800 || f.createH != 600 {
		t.Errorf("Create(%d, %d), ждал прежний размер холста 800x600", f.createW, f.createH)
	}
	_ = win
}

// Границы вместе с «развёрнуто»: прямоугольник — то, куда окно вернётся по
// «восстановить»; бэкенд получает оба.
func TestInitialBoundsAndMaximized(t *testing.T) {
	_, f := initBringUp(t, "1", func(win *Window) {
		win.SetInitialBounds(-1500, 40, 700, 500)
		win.SetInitialMaximized(true)
	})
	want := native.InitialPlacement{PosSet: true, X: -1500, Y: 40, SizeSet: true, Maximized: true}
	if f.placement != want {
		t.Errorf("бэкенд получил %+v, ждал %+v", f.placement, want)
	}
	if f.createW != 700 || f.createH != 500 {
		t.Errorf("Create(%d, %d), ждал 700x500", f.createW, f.createH)
	}
}

// Нулевой размер не затирает размер окна: положение применяется, размер нет.
func TestInitialBounds_PositionOnly(t *testing.T) {
	_, f := initBringUp(t, "1", func(win *Window) { win.SetInitialBounds(30, 40, 0, 0) })
	if want := (native.InitialPlacement{PosSet: true, X: 30, Y: 40}); f.placement != want {
		t.Errorf("бэкенд получил %+v, ждал %+v", f.placement, want)
	}
	if f.createW != 800 || f.createH != 600 {
		t.Errorf("Create(%d, %d), ждал 800x600", f.createW, f.createH)
	}
}

// Ничего не заказано — бэкенд о начальном положении не слышит вовсе: окно
// создаётся как раньше.
func TestInitial_NothingRequested(t *testing.T) {
	_, f := initBringUp(t, "1", nil)
	if f.placed != 0 {
		t.Errorf("SetInitialPlacement вызван %d раз без заказа", f.placed)
	}
	if f.createW != 800 || f.createH != 600 {
		t.Errorf("Create(%d, %d), ждал 800x600", f.createW, f.createH)
	}
}

// Значения применяются один раз: после создания окна они забыты.
func TestInitial_AppliedOnce(t *testing.T) {
	win, f := initBringUp(t, "1", func(win *Window) {
		win.SetInitialBounds(1, 2, 640, 480)
		win.SetInitialMaximized(true)
	})
	if win.initial != (initialState{}) {
		t.Errorf("начальные значения не забыты: %+v", win.initial)
	}
	// Вызов после Run ничего не двигает: бэкенд уже создан и о новых
	// значениях не услышит.
	placed, w, h := f.placed, f.createW, f.createH
	win.SetInitialBounds(5, 6, 300, 200)
	win.SetInitialMaximized(false)
	if f.placed != placed || f.createW != w || f.createH != h {
		t.Error("SetInitialBounds после Run тронул работающее окно")
	}
}

// FitScale: дизайн-размер остаётся прежним, окно создаётся заказанного размера.
func TestInitialBounds_FitScaleKeepsDesignSize(t *testing.T) {
	win, f := initBringUp(t, "1", func(win *Window) {
		win.SetContentFit(FitScale)
		win.SetInitialBounds(0, 0, 1000, 700)
	})
	if win.fitBaseW != 800 || win.fitBaseH != 600 {
		t.Errorf("дизайн-размер %dx%d, ждал прежний 800x600", win.fitBaseW, win.fitBaseH)
	}
	if f.createW != 1000 || f.createH != 700 {
		t.Errorf("Create(%d, %d), ждал 1000x700", f.createW, f.createH)
	}
	if w, h := win.eng.CanvasSize(); w != 800 || h != 600 {
		t.Errorf("холст %dx%d изменился, в FitScale он дизайнерский", w, h)
	}
}

// FitScale: окно создано другого размера, чем дизайн, — кадр сразу подгоняется
// (масштаб min(1000/800, 700/600)), а не ждёт ресайза от системы.
func TestInitialBounds_FitScaleFitsFrameAtOnce(t *testing.T) {
	win, _ := initBringUpOn(t, "1", syncEngine{engine.New(800, 600, 30)}, func(win *Window) {
		win.SetContentFit(FitScale)
		win.SetInitialBounds(0, 0, 1000, 700)
	})
	want := 700.0 / 600.0
	if d := win.scale - want; d > 1e-9 || d < -1e-9 {
		t.Errorf("масштаб %v, ждал %v", win.scale, want)
	}
	if b := win.current.Bounds(); b.Dx() != 1000 || b.Dy() != 700 {
		t.Errorf("буфер кадра %v, ждал 1000x700", b)
	}
}

// Виджетное окно с первого кадра знает, что развёрнуто (кнопка «восстановить»,
// пункты системного меню), а его границы — заказанный размер, не разметка.
func TestInitialMaximized_WidgetWindowKnowsState(t *testing.T) {
	eng := engine.New(800, 600, 30)
	ww := widget.NewWindow("корень", 500, 400)
	eng.SetRoot(ww)
	initBringUpOn(t, "1", syncEngine{eng}, func(win *Window) {
		win.SetInitialBounds(10, 10, 640, 480)
		win.SetInitialMaximized(true)
	})
	if !ww.IsMaximized() {
		t.Error("виджетное окно не знает, что развёрнуто")
	}
	if b := ww.Bounds(); b.Dx() != 640 || b.Dy() != 480 {
		t.Errorf("границы корня %v, ждал 640x480 из SetInitialBounds", b)
	}
}

// Без «развёрнуто» виджетное окно остаётся обычным.
func TestInitialMaximized_NotAskedWidgetWindowNormal(t *testing.T) {
	eng := engine.New(800, 600, 30)
	ww := widget.NewWindow("корень", 500, 400)
	eng.SetRoot(ww)
	initBringUpOn(t, "1", syncEngine{eng}, func(win *Window) { win.SetInitialBounds(10, 10, 640, 480) })
	if ww.IsMaximized() {
		t.Error("виджетное окно считает себя развёрнутым")
	}
}

// Размер из SetInitialBounds важнее размера, с которым создан движок.
func TestInitialBounds_BeatsEngineCanvas(t *testing.T) {
	win, _ := initBringUp(t, "1", func(win *Window) { win.SetInitialBounds(0, 0, 1024, 768) })
	if win.w != 1024 || win.h != 768 {
		t.Errorf("размер окна %dx%d, ждал 1024x768", win.w, win.h)
	}
}

// initMonitors подкладывает два монитора разного масштаба и включает выбор
// масштаба по монитору на любой платформе.
func initMonitors(t *testing.T) {
	t.Helper()
	oldList, oldOn := initialMonitorList, initialPerMonitorScale
	initialMonitorList = func() []native.Monitor {
		return []native.Monitor{
			{Bounds: image.Rect(0, 0, 1920, 1080), Scale: 1, Primary: true},
			{Bounds: image.Rect(1920, 0, 5760, 2160), Scale: 2},
		}
	}
	initialPerMonitorScale = true
	t.Cleanup(func() { initialMonitorList, initialPerMonitorScale = oldList, oldOn })
}

// Окно, которое появится на втором мониторе, создаётся в его масштабе:
// размер логический, физический — умноженный на масштаб ЭТОГО монитора.
func TestInitialBounds_ScaleOfTargetMonitor(t *testing.T) {
	initMonitors(t)
	win, f := initBringUp(t, "", func(win *Window) { win.SetInitialBounds(2200, 100, 640, 480) })
	if f.createW != 1280 || f.createH != 960 {
		t.Errorf("Create(%d, %d), ждал 1280x960 (масштаб второго монитора 2)", f.createW, f.createH)
	}
	if w, h := win.eng.CanvasSize(); w != 640 || h != 480 {
		t.Errorf("холст %dx%d, ждал логические 640x480", w, h)
	}
	if win.scale != 2 {
		t.Errorf("масштаб окна %v, ждал 2", win.scale)
	}
}

// То же окно на основном мониторе остаётся в масштабе 1.
func TestInitialBounds_ScaleOfPrimaryMonitor(t *testing.T) {
	initMonitors(t)
	_, f := initBringUp(t, "", func(win *Window) { win.SetInitialBounds(100, 100, 640, 480) })
	if f.createW != 640 || f.createH != 480 {
		t.Errorf("Create(%d, %d), ждал 640x480", f.createW, f.createH)
	}
}

// Явный HEADLESS_GUI_SCALE важнее монитора.
func TestInitialBounds_ExplicitScaleBeatsMonitor(t *testing.T) {
	initMonitors(t)
	_, f := initBringUp(t, "1.5", func(win *Window) { win.SetInitialBounds(2200, 100, 640, 480) })
	if f.createW != 960 || f.createH != 720 {
		t.Errorf("Create(%d, %d), ждал 960x720 (масштаб 1.5 из окружения)", f.createW, f.createH)
	}
}

// Без размера масштаб по монитору не выбирается: окно живёт в масштабе системы.
func TestInitialBounds_NoSizeNoMonitorScale(t *testing.T) {
	initMonitors(t)
	win, _ := initBringUp(t, "", func(win *Window) { win.SetInitialBounds(2200, 100, 0, 0) })
	if win.scale != 1 {
		t.Errorf("масштаб %v, ждал системный 1", win.scale)
	}
}

// ── Чистые функции ───────────────────────────────────────────────────────────

func TestInitialScaleFor(t *testing.T) {
	list := []native.Monitor{
		{Bounds: image.Rect(0, 0, 1920, 1080), Scale: 1, Primary: true},
		{Bounds: image.Rect(1920, 0, 5760, 2160), Scale: 2},
	}
	// Окно целиком на втором мониторе — его масштаб.
	if k := initialScaleFor(list, image.Rect(2000, 100, 3000, 700), 1); k != 2 {
		t.Errorf("масштаб на втором мониторе %v, ждал 2", k)
	}
	// Окно на границе: побеждает монитор с большей долей площади.
	if k := initialScaleFor(list, image.Rect(1500, 100, 2100, 600), 1); k != 1 {
		t.Errorf("на границе, больше на первом: %v, ждал 1", k)
	}
	// Окно вне мониторов (отключённый экран): базовый масштаб.
	if k := initialScaleFor(list, image.Rect(9000, 9000, 9500, 9500), 1.25); k != 1.25 {
		t.Errorf("вне мониторов %v, ждал базовый 1.25", k)
	}
	// Монитор без масштаба не обнуляет интерфейс.
	if k := initialScaleFor([]native.Monitor{{Bounds: image.Rect(0, 0, 100, 100)}}, image.Rect(0, 0, 50, 50), 1.5); k != 1.5 {
		t.Errorf("монитор без масштаба: %v, ждал базовый 1.5", k)
	}
}

func TestInitialPhysicalRect(t *testing.T) {
	if r := initialPhysicalRect(10, 20, 800, 600, 1.25); r != image.Rect(10, 20, 1010, 770) {
		t.Errorf("прямоугольник %v", r)
	}
	if r := initialPhysicalRect(0, 0, 100, 100, 0); r != image.Rect(0, 0, 100, 100) {
		t.Errorf("нулевой масштаб: %v", r)
	}
}

func TestInitialState_Placement(t *testing.T) {
	if _, ok := (initialState{}).placement(); ok {
		t.Error("пустое состояние заказывает положение")
	}
	p, ok := (initialState{boundsSet: true, x: 1, y: 2, w: 3, h: 4}).placement()
	if !ok || p != (native.InitialPlacement{PosSet: true, X: 1, Y: 2, SizeSet: true}) {
		t.Errorf("%+v %v", p, ok)
	}
	// Размер не положителен — не задан.
	p, _ = (initialState{boundsSet: true, x: 1, y: 2, w: 0, h: 4}).placement()
	if p.SizeSet {
		t.Error("нулевая ширина принята за размер")
	}
	// Положение (0, 0) — тоже положение, а не «не задано».
	p, ok = (initialState{boundsSet: true}).placement()
	if !ok || !p.PosSet {
		t.Error("положение (0,0) потеряно")
	}
}
