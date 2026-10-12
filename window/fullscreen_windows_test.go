//go:build windows

package window

import (
	"image"
	"runtime"
	"testing"
)

// Полный экран Win32 на настоящем окне: окно создаётся в этом же потоке, поэтому
// SetWindowPos/ShowWindow выполняют wndProc синхронно и цикл сообщений не нужен.

func newFsWindow(t *testing.T) *Win32Window {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	w := &Win32Window{}
	w.noQuit = true
	if err := w.Create("fs-test", 400, 300); err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	t.Cleanup(func() { procDestroyWindow.Call(uintptr(w.hwnd)) })
	return w
}

func TestWin32Fullscreen_EnterLeave(t *testing.T) {
	w := newFsWindow(t)
	w.SetPosition(120, 90)
	before := w.windowRect()
	styleBefore := w.windowStyle()
	if styleBefore&fsStyleThickFrame == 0 {
		t.Fatalf("окно создано без WS_THICKFRAME: %#x", styleBefore)
	}

	var seen []bool
	w.SetOnFullscreenChanged(func(on bool) { seen = append(seen, on) })

	w.applyFullscreen(true)
	if !w.IsFullscreen() {
		t.Fatal("IsFullscreen false после входа")
	}
	mon, ok := w.monitorOfWindow()
	if !ok {
		t.Fatal("монитор окна не определён")
	}
	// Монитор, а не рабочая область: таскбар закрыт.
	if got := w.windowRect(); got != mon.Bounds {
		t.Errorf("прямоугольник окна %v, ждал rcMonitor %v", got, mon.Bounds)
	}
	if w.windowStyle()&fsStyleThickFrame != 0 {
		t.Error("WS_THICKFRAME не снят на время полного экрана")
	}
	if w.IsMaximized() {
		t.Error("IsMaximized true у полноэкранного окна, не бывшего развёрнутым")
	}

	// Положение и размер, заказанные в полном экране, окно не двигают.
	w.SetSize(500, 400)
	w.SetPosition(5, 5)
	if got := w.windowRect(); got != mon.Bounds {
		t.Errorf("SetSize/SetPosition сдвинули полноэкранное окно: %v", got)
	}

	w.applyFullscreen(false)
	if w.IsFullscreen() {
		t.Fatal("IsFullscreen true после выхода")
	}
	if w.windowStyle() != styleBefore {
		t.Errorf("стиль после выхода %#x, был %#x", w.windowStyle(), styleBefore)
	}
	// Окно вернулось на запомненное место с заказанным в полном экране
	// положением и размером.
	if got, want := w.windowRect(), image.Rect(5, 5, 505, 405); got != want {
		t.Errorf("прямоугольник после выхода %v, ждал %v (было %v)", got, want, before)
	}
	if len(seen) != 2 || !seen[0] || seen[1] {
		t.Errorf("уведомления %v, ждал [true false]", seen)
	}
}

// Повторный вход/выход ничего не ломает и не шлёт лишних уведомлений.
func TestWin32Fullscreen_Idempotent(t *testing.T) {
	w := newFsWindow(t)
	n := 0
	w.SetOnFullscreenChanged(func(bool) { n++ })
	w.applyFullscreen(false) // и так не в полном экране
	w.applyFullscreen(true)
	w.applyFullscreen(true)
	w.applyFullscreen(false)
	w.applyFullscreen(false)
	if n != 2 {
		t.Errorf("уведомлений %d, ждал 2", n)
	}
}

// Развёрнутое окно: на время полного экрана «развёрнуто» сохраняется, после
// выхода окно снова развёрнуто.
func TestWin32Fullscreen_KeepsMaximized(t *testing.T) {
	w := newFsWindow(t)
	w.Maximize()
	if !w.IsMaximized() {
		t.Skip("окно не разворачивается в этой среде")
	}
	w.applyFullscreen(true)
	if !w.IsMaximized() {
		t.Error("IsMaximized потерял прежнее значение «развёрнуто» в полном экране")
	}
	if zoomed, _, _ := procIsZoomed.Call(uintptr(w.hwnd)); zoomed != 0 {
		t.Error("окно осталось zoomed: таскбар не скроется")
	}
	// Maximize/Restore в полном экране не возвращают таскбар.
	w.Restore()
	if got, mon := w.windowRect(), w.monitorRectOfWindow(); got != mon {
		t.Errorf("Restore сдвинул полноэкранное окно: %v", got)
	}
	w.Maximize()
	w.applyFullscreen(false)
	if !w.IsMaximized() {
		t.Error("после выхода окно не развёрнуто")
	}
}

// Смена разрешения или перенос окна: refitFullscreen возвращает окно на монитор.
func TestWin32Fullscreen_RefitAfterMove(t *testing.T) {
	w := newFsWindow(t)
	w.applyFullscreen(true)
	mon := w.monitorRectOfWindow()

	// Имитация чужого перемещения (разрешение изменилось, окно сдвинули).
	w.setWindowRect(image.Rect(mon.Min.X+40, mon.Min.Y+30, mon.Min.X+640, mon.Min.Y+430), swpNozorder|swpNoactivate)
	w.refitFullscreen()
	if got := w.windowRect(); got != w.monitorRectOfWindow() {
		t.Errorf("после refit окно %v, ждал монитор %v", got, w.monitorRectOfWindow())
	}
}

// Мониторы: основной присутствует, прямоугольники осмысленны, масштаб ≥ 1.
func TestWin32Monitors(t *testing.T) {
	list := Monitors()
	if len(list) == 0 {
		t.Fatal("Monitors() пуст на Windows")
	}
	if !list[0].Primary {
		t.Errorf("основной монитор не первый: %+v", list[0])
	}
	for _, m := range list {
		if m.Bounds.Empty() || m.Work.Empty() || !m.Work.In(m.Bounds) {
			t.Errorf("некорректные границы: %+v", m)
		}
		if m.Scale < 1 {
			t.Errorf("масштаб %v меньше 1: %+v", m.Scale, m)
		}
	}
	// Основной монитор всегда содержит начало координат.
	if m, ok := MonitorAt(0, 0); !ok || !m.Primary {
		t.Errorf("точка (0,0) не на основном мониторе: %+v %v", m, ok)
	}
}

func TestWin32MonitorOf(t *testing.T) {
	w := newFsWindow(t)
	win := &Window{}
	win.native = w
	m, ok := MonitorOf(win)
	if !ok {
		t.Fatal("MonitorOf не нашёл монитор окна")
	}
	pos := w.windowRect()
	if pos.Intersect(m.Bounds).Empty() {
		t.Errorf("окно %v не пересекает свой монитор %v", pos, m.Bounds)
	}
	if _, ok := MonitorOf(nil); ok {
		t.Error("MonitorOf(nil) нашёл монитор")
	}
}
