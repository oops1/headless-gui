//go:build linux && !android

package x11

import (
	"image/color"
	"os"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/platform/pointer"
)

// Живая проверка против настоящего X-сервера (Xvfb, WSLg/XWayland через
// DISPLAY); без DISPLAY тесты пропускаются. Указателя пользователя нет, поэтому
// проверяется то, что видно без него: RENDER действительно создаёт курсор
// (первый курсор проверен сервером), а XInput2 принимает подписку и сервер даёт
// или отказывает в захвате указателя — что именно, зависит от сервера.

func x11LiveWindow(t *testing.T) *X11Window {
	t.Helper()
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}
	t.Setenv("HEADLESS_GUI_X11", "1")
	w := &X11Window{}
	if err := w.Create("x11 cursor e2e", 320, 240); err != nil {
		t.Skipf("не удалось поднять окно: %v", err)
	}
	t.Cleanup(w.Close)
	go w.RunEventLoop()
	xlfWait(t, "цикл событий", func() bool { return w.curImg.looping.Load() })
	return w
}

func TestX11CursorImageE2E(t *testing.T) {
	w := x11LiveWindow(t)
	w.SetCursorImage(x11ciImage(48, 48, color.NRGBA{R: 255, G: 40, B: 40, A: 180}, 2, 3))
	xlfWait(t, "курсор построен", func() bool {
		w.cursors.mu.Lock()
		defer w.cursors.mu.Unlock()
		return w.curImg.created == 1
	})
	w.cursors.mu.Lock()
	render, verified, shown := w.curImg.render, w.curImg.verified, w.curImg.shown
	w.cursors.mu.Unlock()
	t.Logf("RENDER: состояние=%d (1 — есть), проверен сервером=%v, курсор=%#x", render, verified, shown)
	if render == x11RenderOK && !verified {
		t.Error("курсор RENDER не прошёл проверку сервера")
	}
	if shown == 0 {
		t.Error("курсор не повешен на окно")
	}
	// Ещё несколько, форма поверх них и снова картинка: сервер не должен
	// отвечать ошибками (они бы закрыли окно или остались в потоке).
	for i := 0; i < 20; i++ {
		w.SetCursorImage(x11ciImage(32, 32, color.NRGBA{R: uint8(i * 10), G: 100, B: 200, A: 255}, 1, 1))
	}
	w.SetCursor(pointer.CursorIBeam)
	w.SetCursorImage(x11ciImage(48, 48, color.NRGBA{R: 255, G: 40, B: 40, A: 180}, 2, 3)) // из кэша
	time.Sleep(300 * time.Millisecond)
	if w.closed.Load() {
		t.Fatal("окно закрыто после курсоров-картинок")
	}
}

func TestX11PointerLockE2E(t *testing.T) {
	w := x11LiveWindow(t)
	time.Sleep(300 * time.Millisecond) // окно появилось и получило фокус
	got := w.SetPointerLock(true)
	t.Logf("SetPointerLock(true)=%v, XInput2 подписан=%v, режим возврата=%v", got, w.plock.selected, w.plock.warp)
	// Окно, не получившее фокус (нет оконного менеджера или он медлит),
	// захватывает указатель позже: захват включится с фокусом.
	if got && !w.plock.blurred.Load() {
		if !w.pointerLockActive() {
			t.Error("захват принят окном в фокусе, но указатель не захвачен")
		}
	}
	if !w.SetPointerLock(false) {
		t.Error("выключение вернуло false")
	}
	if w.pointerLockActive() {
		t.Error("указатель остался захваченным после выключения")
	}
	if w.closed.Load() {
		t.Fatal("окно закрыто")
	}
}

// pointerLockActive — указатель захвачен прямо сейчас (читается с другой
// горутины).
func (w *X11Window) pointerLockActive() bool { return w.plock.activeFlag.Load() }
