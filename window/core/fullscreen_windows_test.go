//go:build windows

package core

import (
	"image"
	"runtime"
	"testing"
	"unsafe"

	"github.com/oops1/headless-gui/v3/platform/native"
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
	if styleBefore&native.FullscreenThickFrame == 0 {
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
	if w.windowStyle()&native.FullscreenThickFrame != 0 {
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

// Полный экран на выбранном мониторе: окно встаёт на rcMonitor именно этого
// монитора (с несколькими мониторами — на каждом по очереди, включая перенос
// без выхода из режима), а исчезнувший монитор отвергается.
func TestWin32Fullscreen_OnMonitor(t *testing.T) {
	w := newFsWindow(t)
	list := Monitors()
	if len(list) == 0 {
		t.Skip("мониторов нет")
	}
	var seen []bool
	w.SetOnFullscreenChanged(func(on bool) { seen = append(seen, on) })

	for _, m := range list {
		cur, ok := native.MatchMonitor(platformMonitors(), m)
		if !ok {
			t.Fatalf("монитор %+v не найден в текущем списке", m)
		}
		w.applyFullscreenOn(cur.Bounds)
		if !w.IsFullscreen() {
			t.Fatalf("на мониторе %q нет полного экрана", m.Name)
		}
		if got := w.windowRect(); got != m.Bounds {
			t.Errorf("монитор %q: окно %v, ждал rcMonitor %v", m.Name, got, m.Bounds)
		}
		if got, ok := w.monitorOfWindow(); !ok || native.MonitorKey(got) != native.MonitorKey(m) {
			t.Errorf("монитор %q: окно сообщает монитор %+v", m.Name, got)
		}
	}
	// Перенос между мониторами не выходит из режима: одно уведомление «вошёл».
	if len(seen) != 1 || !seen[0] {
		t.Errorf("уведомления %v, ждал единственное [true]", seen)
	}
	// Окно встаёт именно на переданный прямоугольник, а не на монитор, где оно
	// лежит сейчас (на одном мониторе это единственное, что отличает выбор от
	// умолчания).
	fake := image.Rect(list[0].Bounds.Min.X, list[0].Bounds.Min.Y, list[0].Bounds.Min.X+800, list[0].Bounds.Min.Y+600)
	w.applyFullscreenOn(fake)
	if got := w.windowRect(); got != fake {
		t.Errorf("перенос на %v: окно %v", fake, got)
	}
	w.applyFullscreen(false)
	if len(seen) != 2 || seen[1] {
		t.Errorf("после выхода уведомления %v", seen)
	}
	// Вход сразу на заданный прямоугольник. Без applyCorners: регион окна
	// (SetWindowRgn) шлёт WM_WINDOWPOSCHANGED, и refit вернул бы «монитор»
	// ненастоящего размера на настоящий, а настоящий монитор уже совпадает.
	w.enterFullscreen(fake)
	if !w.IsFullscreen() || w.windowRect() != fake {
		t.Errorf("вход на %v: полный экран=%v, окно %v", fake, w.IsFullscreen(), w.windowRect())
	}
	w.applyFullscreen(false)

	// Вход прямо на нужный монитор (SetFullscreenOn → поток окна).
	if w.SetFullscreenOn(native.MonitorWithKey(native.Monitor{Name: "нет такого", Bounds: image.Rect(90000, 0, 91000, 800)}, 0xDEAD)) {
		t.Error("несуществующий монитор принят")
	}
	if w.SetFullscreenOn(native.Monitor{}) {
		t.Error("нулевой Monitor принят")
	}
}

var procPeekMessageW = user32.NewProc("PeekMessageW")

// fsPumpMessages разбирает очередь сообщений потока окна (тест — поток окна).
func fsPumpMessages() {
	var m msg
	for {
		if r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1); r == 0 {
			return
		}
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// SetFullscreenOn целиком: монитор из Monitors() → поток окна → rcMonitor.
func TestWin32Fullscreen_SetFullscreenOnPumped(t *testing.T) {
	w := newFsWindow(t)
	for _, m := range Monitors() {
		if !w.SetFullscreenOn(m) {
			t.Fatalf("SetFullscreenOn(%q) отказал", m.Name)
		}
		fsPumpMessages()
		if !w.IsFullscreen() || w.windowRect() != m.Bounds {
			t.Errorf("монитор %q: полный экран=%v, окно %v, ждал %v",
				m.Name, w.IsFullscreen(), w.windowRect(), m.Bounds)
		}
	}
	w.SetFullscreen(false)
	fsPumpMessages()
	if w.IsFullscreen() {
		t.Error("SetFullscreen(false) не вывел окно из полного экрана")
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
