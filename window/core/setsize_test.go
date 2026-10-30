package core

import (
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/platform/native"
)

// Window.SetSize (setsize.go): общий слой над бэкендом. Платформенные
// особенности — в platform/x11/x11_setsize_linux_test.go,
// platform/wayland/wayland_setsize_apply_linux_test.go и setsize_windows_test.go.

// szFake — подставной бэкенд: записывает SetSize и отвечает фактическим
// размером. echo — как Win32 (WM_SIZE сразу после MoveWindow): бэкенд тут же
// сообщает новый размер через onResize; без него — как X11, где ответ
// менеджера придёт позже.
type szFake struct {
	initFake
	calls    [][2]int
	cw, ch   int
	echo     bool
	onResize func(w, h int)
}

func (f *szFake) Create(title string, w, h int) error {
	f.initFake.Create(title, w, h)
	f.cw, f.ch = w, h
	return nil
}

func (f *szFake) GetSize() (int, int) { return f.cw, f.ch }

func (f *szFake) SetSize(w, h int) {
	f.calls = append(f.calls, [2]int{w, h})
	if f.echo {
		f.cw, f.ch = w, h
		if f.onResize != nil {
			f.onResize(w, h)
		}
	}
}

func (f *szFake) SetOnResize(fn func(int, int)) { f.onResize = fn }

// szBringUp поднимает окно на szFake с масштабом scale. Движок выполняет Post
// сразу, поэтому события окна видны без цикла движка.
func szBringUp(t *testing.T, scale string, f *szFake, setup func(win *Window)) *Window {
	t.Helper()
	t.Setenv("HEADLESS_GUI_SCALE", scale)
	old := newNativeWindow
	newNativeWindow = func() native.NativeWindow { return f }
	t.Cleanup(func() { newNativeWindow = old })

	win := New(syncEngine{engine.New(800, 600, 30)}, "setsize")
	if setup != nil {
		setup(win)
	}
	if err := win.bringUp(); err != nil {
		t.Fatalf("bringUp: %v", err)
	}
	return win
}

// Размер логический: бэкенду уходят физические пиксели (логический × масштаб),
// холст движка получает логический — через тот же onResize, что и при ресайзе
// мышью, и ровно один раз.
func TestWindowSetSize_LogicalToPhysical(t *testing.T) {
	f := &szFake{echo: true}
	win := szBringUp(t, "2", f, nil)
	resizes := 0
	inner := f.onResize
	f.onResize = func(w, h int) { resizes++; inner(w, h) }

	win.SetSize(520, 370)

	if len(f.calls) != 1 || f.calls[0] != [2]int{1040, 740} {
		t.Fatalf("бэкенд получил %v, ждал один SetSize(1040, 740)", f.calls)
	}
	if resizes != 1 {
		t.Errorf("onResize вызван %d раз, ждал 1", resizes)
	}
	if w, h := win.eng.CanvasSize(); w != 520 || h != 370 {
		t.Errorf("холст %dx%d, ждал логические 520x370", w, h)
	}
	if pw, ph := win.physicalSize(); pw != 1040 || ph != 740 {
		t.Errorf("физический размер кадра %dx%d, ждал 1040x740", pw, ph)
	}

	// И обратно.
	win.SetSize(1400, 860)
	if len(f.calls) != 2 || f.calls[1] != [2]int{2800, 1720} {
		t.Fatalf("бэкенд получил %v, ждал вторым SetSize(2800, 1720)", f.calls)
	}
	if w, h := win.eng.CanvasSize(); w != 1400 || h != 860 {
		t.Errorf("холст %dx%d, ждал 1400x860", w, h)
	}
}

// Размер уже такой — бэкенд не трогается вовсе (ни запроса, ни лишнего
// onResize).
func TestWindowSetSize_SameSizeIsNoOp(t *testing.T) {
	f := &szFake{echo: true}
	win := szBringUp(t, "1", f, nil)

	win.SetSize(800, 600) // размер холста по умолчанию, окно создано таким же
	if len(f.calls) != 0 {
		t.Fatalf("SetSize того же размера дошёл до бэкенда: %v", f.calls)
	}

	win.SetSize(520, 370)
	win.SetSize(520, 370)
	if len(f.calls) != 1 {
		t.Errorf("повторный SetSize дошёл до бэкенда: %v", f.calls)
	}
}

// Нулевой и отрицательный размер игнорируются.
func TestWindowSetSize_NonPositiveIgnored(t *testing.T) {
	f := &szFake{echo: true}
	win := szBringUp(t, "1", f, nil)
	for _, sz := range [][2]int{{0, 300}, {300, 0}, {-1, -1}, {0, 0}} {
		win.SetSize(sz[0], sz[1])
	}
	if len(f.calls) != 0 {
		t.Errorf("недопустимый размер дошёл до бэкенда: %v", f.calls)
	}
}

// Бэкенд, отвечающий позже (X11): SetSize сам холст не трогает, его меняет
// onResize бэкенда, когда тот сообщит фактический размер.
func TestWindowSetSize_CanvasFollowsBackendResize(t *testing.T) {
	f := &szFake{} // без echo
	win := szBringUp(t, "1", f, nil)

	win.SetSize(520, 370)
	if len(f.calls) != 1 || f.calls[0] != [2]int{520, 370} {
		t.Fatalf("бэкенд получил %v", f.calls)
	}
	if w, h := win.eng.CanvasSize(); w != 800 || h != 600 {
		t.Fatalf("холст %dx%d изменился до ответа бэкенда", w, h)
	}
	// Менеджер поправил размер (минимум): движок получает фактический.
	f.cw, f.ch = 600, 400
	f.onResize(600, 400)
	if w, h := win.eng.CanvasSize(); w != 600 || h != 400 {
		t.Errorf("холст %dx%d, ждал фактический 600x400", w, h)
	}
}

// До Run размер становится начальным: Create получает его, холст — логический.
func TestWindowSetSize_BeforeRunIsInitialSize(t *testing.T) {
	f := &szFake{}
	win := szBringUp(t, "2", f, func(win *Window) {
		win.SetSize(640, 480)
		win.SetSize(520, 370) // последний вызов важнее
	})
	if f.createW != 1040 || f.createH != 740 {
		t.Errorf("Create(%d, %d), ждал физические 1040x740", f.createW, f.createH)
	}
	if w, h := win.eng.CanvasSize(); w != 520 || h != 370 {
		t.Errorf("холст %dx%d, ждал 520x370", w, h)
	}
	if len(f.calls) != 0 {
		t.Errorf("SetSize до Run дошёл до бэкенда: %v", f.calls)
	}
	// Положение не задано — бэкенд не получает «положение 0,0».
	if f.placed != 0 {
		t.Errorf("бэкенд получил начальное положение: %+v", f.placement)
	}
}

// SetSize до Run не перекрывает SetInitialBounds, заданный позже: последний
// вызов важнее.
func TestWindowSetSize_InitialBoundsAfterWins(t *testing.T) {
	f := &szFake{}
	szBringUp(t, "1", f, func(win *Window) {
		win.SetSize(640, 480)
		win.SetInitialBounds(10, 20, 700, 500)
	})
	if f.createW != 700 || f.createH != 500 {
		t.Errorf("Create(%d, %d), ждал 700x500", f.createW, f.createH)
	}
}

// В режиме FitScale логического размера окна нет: размер идёт в пикселях окна
// без умножения на масштаб.
func TestWindowSetSize_FitScaleIsWindowPixels(t *testing.T) {
	f := &szFake{}
	win := szBringUp(t, "1", f, func(win *Window) { win.SetContentFit(FitScale) })
	win.scale = 2 // подгонка поставила бы масштаб, не равный 1

	win.SetSize(500, 300)
	if len(f.calls) != 1 || f.calls[0] != [2]int{500, 300} {
		t.Errorf("бэкенд получил %v, ждал SetSize(500, 300) без масштаба", f.calls)
	}
}
