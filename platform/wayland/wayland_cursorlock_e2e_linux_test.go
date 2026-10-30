//go:build linux && !android

package wayland

import (
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/platform/pointer"
)

// Живая проверка против настоящего композитора (Weston/WSLg и т. п. через
// WAYLAND_DISPLAY); без него тесты пропускаются. Указателя в окне нет, поэтому
// проверяется то, что видно без него: композитор принимает запросы и не
// закрывает соединение ошибкой протокола, а ответ на SetPointerLock совпадает с
// набором объявленных им расширений.

func TestWaylandCursorImageE2E(t *testing.T) {
	w := NewWindow()
	if w == nil {
		t.Skip("нет Wayland-композитора (WAYLAND_DISPLAY)")
	}
	if err := w.Create("wayland cursor image e2e", 320, 240); err != nil {
		t.Skipf("не удалось создать окно: %v", err)
	}
	defer w.Close()
	go w.RunEventLoop()

	// Указателя в окне нет, enter не было: подставляем serial, чтобы запросы
	// (пул, буфер, поверхность, set_cursor) дошли до композитора. Он вправе
	// молча отклонить устаревший serial, но обязан разобрать сами запросы.
	w.ptrEnterSerial.Store(1)
	for i := 0; i < 3; i++ {
		w.SetCursorImage(wlciImage(48, 48, color.NRGBA{R: uint8(60 * i), G: 100, B: 200, A: 220}, 3, 3))
	}
	w.SetCursor(pointer.CursorIBeam)
	w.SetCursorImage(wlciImage(48, 48, color.NRGBA{R: 1, A: 255}, 0, 0))
	// Любая ошибка протокола закрывает окно (цикл событий ставит closed).
	time.Sleep(300 * time.Millisecond)
	if w.closed.Load() {
		t.Fatal("композитор закрыл соединение после курсоров-картинок (ошибка протокола)")
	}
}

func TestWaylandPointerLockE2E(t *testing.T) {
	w := NewWindow()
	if w == nil {
		t.Skip("нет Wayland-композитора (WAYLAND_DISPLAY)")
	}
	if err := w.Create("wayland pointer lock e2e", 320, 240); err != nil {
		t.Skipf("не удалось создать окно: %v", err)
	}
	defer w.Close()
	go w.RunEventLoop()

	have := w.plock.gConstraints != 0 && w.plock.gRelative != 0 && w.pointerID != 0
	t.Logf("композитор объявил: pointer_constraints=%d relative_pointer=%d wl_pointer=%d",
		w.plock.gConstraints, w.plock.gRelative, w.pointerID)
	got := w.SetPointerLock(true)
	if got != have {
		t.Fatalf("SetPointerLock(true)=%v при расширениях=%v", got, have)
	}
	if got {
		time.Sleep(200 * time.Millisecond)
		if w.closed.Load() {
			t.Fatal("композитор закрыл соединение после lock_pointer (ошибка протокола)")
		}
		t.Logf("захват принят; locked=%v (без указателя в окне компоновщик его не даст)", w.plock.active.Load())
	}
	if !w.SetPointerLock(false) {
		t.Error("выключение вернуло false")
	}
	time.Sleep(100 * time.Millisecond)
	if w.closed.Load() {
		t.Fatal("композитор закрыл соединение после destroy (ошибка протокола)")
	}
}
