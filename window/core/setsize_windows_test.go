//go:build windows

package core

import (
	"runtime"
	"testing"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/internal/testhook"
)

// Window.SetSize на настоящем окне Win32: MoveWindow → WM_SIZE → onResize →
// холст движка. Окно создаётся в этом же потоке, а вызовы окну ОС
// (InvokeOnUIThread) исполняет pumpWin32.

// szWin32Window поднимает window.Window на настоящем Win32-окне. count —
// сколько раз бэкенд сообщил движку о ресайзе.
func szWin32Window(t *testing.T, scale string, cw, ch int) (*Window, *Win32Window, *int) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	t.Setenv("HEADLESS_GUI_SCALE", scale)

	win := New(syncEngine{engine.New(cw, ch, 30)}, "setsize-live")
	if err := win.bringUp(); err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	nw, ok := win.native.(*Win32Window)
	if !ok {
		t.Skipf("бэкенд %T, ждал *Win32Window", win.native)
	}
	testhook.Win32SetNoQuit(nw)
	t.Cleanup(func() { procTestDestroyWindow.Call(nw.HWND()) })
	// Опрос раскладки (localePoll) живёт, пока окно не закрыто: без этого он
	// пережил бы тест и менял глобальную локаль в соседних.
	t.Cleanup(func() { win.closeRequested.Store(true) })
	pumpWin32()

	count := new(int)
	testhook.Win32AfterResize(nw, func(w, h int) { *count++ })
	return win, nw, count
}

// 800×600 → SetSize(520,370) → onResize один раз, клиентская область 520×370,
// холст 520×370; затем обратно.
func TestWin32WindowSetSize_RoundTrip(t *testing.T) {
	win, nw, count := szWin32Window(t, "1", 800, 600)
	if cw, ch := nw.GetSize(); cw != 800 || ch != 600 {
		t.Fatalf("исходный размер %dx%d, ждал 800x600", cw, ch)
	}

	win.SetSize(520, 370)
	pumpWin32()
	if cw, ch := nw.GetSize(); cw != 520 || ch != 370 {
		t.Errorf("GetSize = %dx%d, ждал 520x370", cw, ch)
	}
	if *count != 1 {
		t.Errorf("onResize вызван %d раз, ждал 1", *count)
	}
	if w, h := win.eng.CanvasSize(); w != 520 || h != 370 {
		t.Errorf("холст %dx%d, ждал 520x370", w, h)
	}

	// Тот же размер — ничего не происходит.
	win.SetSize(520, 370)
	pumpWin32()
	if *count != 1 {
		t.Errorf("SetSize того же размера дал onResize: %d", *count)
	}

	win.SetSize(800, 600)
	pumpWin32()
	if cw, ch := nw.GetSize(); cw != 800 || ch != 600 {
		t.Errorf("GetSize = %dx%d, ждал 800x600", cw, ch)
	}
	if *count != 2 {
		t.Errorf("onResize вызван %d раз, ждал 2", *count)
	}
	if w, h := win.eng.CanvasSize(); w != 800 || h != 600 {
		t.Errorf("холст %dx%d, ждал 800x600", w, h)
	}
}

// Масштаб интерфейса 2: размер логический, окно — физическое (×2), холст —
// логический.
func TestWin32WindowSetSize_LogicalAtScale(t *testing.T) {
	win, nw, count := szWin32Window(t, "2", 400, 300)
	if cw, ch := nw.GetSize(); cw != 800 || ch != 600 {
		t.Skipf("окно масштаба 2 вышло %dx%d (экран мал?), ждал 800x600", cw, ch)
	}

	win.SetSize(300, 260)
	pumpWin32()
	if cw, ch := nw.GetSize(); cw != 600 || ch != 520 {
		t.Errorf("GetSize = %dx%d, ждал физические 600x520", cw, ch)
	}
	if *count != 1 {
		t.Errorf("onResize вызван %d раз, ждал 1", *count)
	}
	if w, h := win.eng.CanvasSize(); w != 300 || h != 260 {
		t.Errorf("холст %dx%d, ждал логические 300x260", w, h)
	}
}

// Развёрнутому окну заказанный размер достаётся при восстановлении: пока оно
// развёрнуто, размер клиентской области не меняется.
func TestWin32WindowSetSize_WhileMaximizedAppliesOnRestore(t *testing.T) {
	win, nw, count := szWin32Window(t, "1", 800, 600)
	nw.Maximize()
	pumpWin32()
	if !nw.IsMaximized() {
		t.Skip("окно не развернулось")
	}
	mw, mh := nw.GetSize()
	before := *count

	win.SetSize(520, 370)
	pumpWin32()
	if !nw.IsMaximized() {
		t.Error("SetSize снял «развёрнуто»")
	}
	if cw, ch := nw.GetSize(); cw != mw || ch != mh {
		t.Errorf("развёрнутое окно изменило размер: %dx%d, было %dx%d", cw, ch, mw, mh)
	}

	nw.Restore()
	pumpWin32()
	if nw.IsMaximized() {
		t.Fatal("окно осталось развёрнутым")
	}
	if cw, ch := nw.GetSize(); cw != 520 || ch != 370 {
		t.Errorf("после восстановления %dx%d, ждал заказанный 520x370", cw, ch)
	}
	if w, h := win.eng.CanvasSize(); w != 520 || h != 370 {
		t.Errorf("холст %dx%d, ждал 520x370", w, h)
	}
	if *count <= before {
		t.Error("движок не получил ресайз при восстановлении")
	}
}
