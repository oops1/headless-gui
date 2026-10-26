//go:build linux && !android

package window

import (
	"os"
	"testing"
	"time"
)

// Живая проверка против настоящего сервера. Без DISPLAY / WAYLAND_DISPLAY
// тесты пропускаются; чего сервер не умеет (нет XKB), приводит к Skip с
// пояснением, а не к ошибке. Исходное состояние переключателей возвращается в
// Cleanup, что бы ни случилось.

func TestX11LockKeysE2E(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}
	t.Setenv("HEADLESS_GUI_X11", "1")
	w := &X11Window{}
	if err := w.Create("x11 lockkeys e2e", 320, 240); err != nil {
		t.Skipf("не удалось поднять окно: %v", err)
	}
	defer w.Close()
	go w.RunEventLoop()
	lkWaitUntil(t, "запуск цикла событий", func() bool { return w.lk.looping.Load() })

	orig, ok := w.LockKeys()
	if !ok {
		t.Fatal("LockKeys не ответил на настоящем сервере")
	}
	t.Logf("переключатели сейчас: %v; XKB=%v индикаторы=%v модификаторы=%v",
		orig, w.lk.m.xkb, w.lk.m.indicator, w.lk.m.mods)
	if !w.lk.m.xkb {
		t.Skip("сервер без XKB: менять состояние нечем")
	}
	t.Cleanup(func() { w.SetLockKeys(orig) })

	want := orig ^ NumLock
	if !w.SetLockKeys(want) {
		// XWayland под чужим компоновщиком (WSLg) переключателями не всегда
		// распоряжается сам: пока фокус у компоновщика, он возвращает своё
		// состояние. Сервер не применил смену — это его право, а не ошибка
		// движка (запрос и разбор проверены на фальшивом сервере).
		if got, _ := w.LockKeys(); got == orig {
			t.Skipf("сервер не применил SetLockKeys(%v): переключатели держит компоновщик", want)
		}
		t.Fatalf("SetLockKeys(%v) отказал", want)
	}
	if !lkEventually(func() bool { got, _ := w.LockKeys(); return got == want }) {
		got, _ := w.LockKeys()
		t.Fatalf("после SetLockKeys(%v) сервер показывает %v", want, got)
	}
	// Остальные клавиши не тронуты, и возврат работает.
	if !w.SetLockKeys(orig) {
		t.Fatalf("возврат к %v отказал", orig)
	}
	if got, _ := w.LockKeys(); got != orig {
		t.Errorf("после возврата %v, ждал %v", got, orig)
	}
}

func lkEventually(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func TestWaylandLockKeysE2E(t *testing.T) {
	w := newWaylandWindow()
	if w == nil {
		t.Skip("нет Wayland-композитора (WAYLAND_DISPLAY)")
	}
	if err := w.Create("wayland lockkeys e2e", 320, 240); err != nil {
		t.Skipf("не удалось создать окно: %v", err)
	}
	defer w.Close()
	go w.RunEventLoop()

	// Фокус клавиатуры окно может и не получить (нет пользователя за
	// компоновщиком): тогда «неизвестно» — правильный ответ.
	time.Sleep(300 * time.Millisecond)
	k, ok := w.LockKeys()
	t.Logf("LockKeys на настоящем композиторе: %v, известно=%v", k, ok)
	if !ok && k != 0 {
		t.Errorf("неизвестное состояние с ненулевым набором: %v", k)
	}
	if w.LockKeysSettled() != ok {
		t.Errorf("LockKeysSettled=%v при известно=%v", w.LockKeysSettled(), ok)
	}

	// SetLockKeys через виртуальную клавиатуру — только если композитор её
	// объявил и окно в фокусе (иначе false без запросов).
	name := w.gVirtualKeyboard.Load()
	t.Logf("zwp_virtual_keyboard_manager_v1 объявлен: %v", name != 0)
	if name == 0 {
		if w.SetLockKeys(CapsLock) {
			t.Error("SetLockKeys ответил true без менеджера")
		}
		return
	}
	if !ok {
		t.Skip("менеджер есть, но окно без фокуса клавиатуры: менять нечем")
	}
	t.Cleanup(func() { w.SetLockKeys(k) })
	want := k ^ NumLock
	got := w.SetLockKeys(want)
	now, _ := w.LockKeys()
	t.Logf("SetLockKeys(%v) = %v, после: %v", want, got, now)
	if got && now != want {
		t.Errorf("true, а состояние %v, ждал %v", now, want)
	}
}
