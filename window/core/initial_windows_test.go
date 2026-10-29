//go:build windows

package core

import (
	"image"
	"runtime"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// Начальное положение и «развёрнуто» на настоящем окне Win32 (initial.go).
// Окно создаётся в этом же потоке, поэтому wndProc отрабатывает синхронно и
// цикл сообщений не нужен.

func newInitialWin32(t *testing.T, p native.InitialPlacement, w, h int) *Win32Window {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	win := &Win32Window{}
	win.noQuit = true
	win.setInitialPlacement(p)
	if err := win.Create("initial-test", w, h); err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	t.Cleanup(func() { procDestroyWindow.Call(uintptr(win.hwnd)) })
	return win
}

func isZoomed(w *Win32Window) bool {
	z, _, _ := procIsZoomed.Call(uintptr(w.hwnd))
	return z != 0
}

func isVisible(w *Win32Window) bool {
	v, _, _ := procIsWindowVisible.Call(uintptr(w.hwnd))
	return v != 0
}

// Положение задано — окно рождается ровно там, а не по центру экрана.
func TestWin32Initial_PositionAtCreate(t *testing.T) {
	w := newInitialWin32(t, native.InitialPlacement{PosSet: true, X: 137, Y: 91, SizeSet: true}, 500, 350)

	if got, want := w.windowRect(), image.Rect(137, 91, 137+500, 91+350); got != want {
		t.Errorf("прямоугольник окна %v, ждал %v", got, want)
	}
	if !isVisible(w) {
		t.Error("окно не показано")
	}
	if isZoomed(w) || w.IsMaximized() {
		t.Error("обычное окно оказалось развёрнутым")
	}
	if cw, ch := w.clientSize(); cw != 500 || ch != 350 {
		t.Errorf("клиентская область %dx%d, ждал 500x350 (окно без рамки)", cw, ch)
	}
}

// Положение не задано — окно по центру экрана, как и раньше.
func TestWin32Initial_DefaultStillCentered(t *testing.T) {
	w := newInitialWin32(t, native.InitialPlacement{}, 500, 350)
	sw, sh := getSystemMetrics(0), getSystemMetrics(1)
	want := image.Rect((sw-500)/2, (sh-350)/2, (sw-500)/2+500, (sh-350)/2+350)
	if got := w.windowRect(); got != want {
		t.Errorf("прямоугольник окна %v, ждал по центру %v", got, want)
	}
}

// Отрицательные координаты (монитор левее и выше основного) доходят как есть.
func TestWin32Initial_NegativePositionOnVirtualDesktop(t *testing.T) {
	mons := platformMonitors()
	var left *native.Monitor
	for i := range mons {
		if mons[i].Bounds.Min.X < 0 {
			left = &mons[i]
		}
	}
	if left == nil {
		t.Skip("нет монитора с отрицательными координатами")
	}
	x, y := left.Bounds.Min.X+50, left.Bounds.Min.Y+50
	w := newInitialWin32(t, native.InitialPlacement{PosSet: true, X: x, Y: y, SizeSet: true}, 400, 300)
	if gx, gy := w.GetPosition(); gx != x || gy != y {
		t.Errorf("положение %d,%d, ждал %d,%d", gx, gy, x, y)
	}
}

// Развёрнуто: окно сразу IsZoomed (сразу после Create, без единого сообщения
// из цикла), IsMaximized отвечает true, клиентская область больше заказанной.
func TestWin32Initial_MaximizedAtCreate(t *testing.T) {
	w := newInitialWin32(t, native.InitialPlacement{Maximized: true}, 500, 350)

	if !isZoomed(w) {
		t.Fatal("IsZoomed ложно: окно создано не развёрнутым")
	}
	if !w.IsMaximized() {
		t.Error("IsMaximized false у развёрнутого окна")
	}
	if !isVisible(w) {
		t.Error("развёрнутое окно не показано")
	}
	cw, ch := w.clientSize()
	if cw <= 500 || ch <= 350 {
		t.Errorf("клиентская область %dx%d не больше заказанной 500x350", cw, ch)
	}
	// Размер, о котором окно знает само, — развёрнутый (WM_SIZE дошёл до Create).
	if sw, sh := w.GetSize(); sw != cw || sh != ch {
		t.Errorf("GetSize %dx%d расходится с клиентской областью %dx%d", sw, sh, cw, ch)
	}
}

// Главное: развёрнутое окно с границами. «Восстановить» возвращает именно их,
// а не центр экрана и не заказанный размер без положения.
func TestWin32Initial_RestoreReturnsInitialBounds(t *testing.T) {
	w := newInitialWin32(t, native.InitialPlacement{PosSet: true, X: 210, Y: 160, SizeSet: true, Maximized: true}, 620, 410)

	if !isZoomed(w) {
		t.Fatal("окно создано не развёрнутым")
	}
	w.Restore()
	if isZoomed(w) || w.IsMaximized() {
		t.Fatal("после Restore окно всё ещё развёрнуто")
	}
	if got, want := w.windowRect(), image.Rect(210, 160, 210+620, 160+410); got != want {
		t.Errorf("после восстановления прямоугольник %v, ждал начальный %v", got, want)
	}
	// И обратно: Maximize после Restore работает как обычно.
	w.Maximize()
	if !isZoomed(w) {
		t.Error("повторное Maximize не развернуло окно")
	}
}

// Развёрнутое окно без положения: «восстановить» вернёт заказанный размер по
// центру, как у обычного окна.
func TestWin32Initial_MaximizedOnlyRestoresToCentered(t *testing.T) {
	w := newInitialWin32(t, native.InitialPlacement{Maximized: true}, 500, 350)
	w.Restore()
	sw, sh := getSystemMetrics(0), getSystemMetrics(1)
	want := image.Rect((sw-500)/2, (sh-350)/2, (sw-500)/2+500, (sh-350)/2+350)
	if got := w.windowRect(); got != want {
		t.Errorf("после восстановления %v, ждал %v", got, want)
	}
}

// Полный экран поверх начального «развёрнуто»: выход из него возвращает
// развёрнутое окно, а «восстановить» после него — начальные границы.
func TestWin32Initial_FullscreenKeepsInitialBounds(t *testing.T) {
	w := newInitialWin32(t, native.InitialPlacement{PosSet: true, X: 180, Y: 120, SizeSet: true, Maximized: true}, 600, 400)

	w.applyFullscreen(true)
	if !w.IsFullscreen() {
		t.Fatal("не вошли в полный экран")
	}
	if !w.IsMaximized() {
		t.Error("IsMaximized потерял начальное «развёрнуто» на время полного экрана")
	}
	w.applyFullscreen(false)
	if !isZoomed(w) {
		t.Fatal("после выхода окно не развёрнуто")
	}
	w.Restore()
	if got, want := w.windowRect(), image.Rect(180, 120, 780, 520); got != want {
		t.Errorf("после выхода и восстановления %v, ждал %v", got, want)
	}
}

// После старта SetPosition/SetSize работают как раньше.
func TestWin32Initial_SetPositionAfterCreateStillWorks(t *testing.T) {
	w := newInitialWin32(t, native.InitialPlacement{PosSet: true, X: 100, Y: 100, SizeSet: true}, 500, 350)
	w.SetPosition(300, 200)
	w.SetSize(450, 300)
	if got, want := w.windowRect(), image.Rect(300, 200, 750, 500); got != want {
		t.Errorf("прямоугольник %v, ждал %v", got, want)
	}
}

// Окно без заказа клиентскую область отдаёт как есть: сверка размера после
// Create ничего не меняет для обычных окон.
func TestWin32Initial_ClientSizeOfPlainWindow(t *testing.T) {
	w := newInitialWin32(t, native.InitialPlacement{}, 640, 480)
	if cw, ch := w.clientSize(); cw != 640 || ch != 480 {
		t.Errorf("клиентская область %dx%d, ждал 640x480", cw, ch)
	}
}

// Свёрнутое окно размера не сообщает: сверка не должна принять нулевой клиент.
func TestWin32Initial_ClientSizeOfMinimizedIsZero(t *testing.T) {
	w := newInitialWin32(t, native.InitialPlacement{}, 640, 480)
	w.Minimize()
	if cw, ch := w.clientSize(); cw != 0 || ch != 0 {
		t.Errorf("у свёрнутого окна клиентская область %dx%d, ждал 0x0", cw, ch)
	}
}
