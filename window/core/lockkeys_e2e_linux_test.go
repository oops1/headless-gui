//go:build linux && !android

package core

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/platform/keyboard"
)

// Живая проверка против настоящего сервера. Без DISPLAY / WAYLAND_DISPLAY
// тесты пропускаются; чего сервер не умеет (нет XKB), приводит к Skip с
// пояснением, а не к ошибке. Исходное состояние переключателей возвращается в
// Cleanup, что бы ни случилось.

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
		if w.SetLockKeys(keyboard.CapsLock) {
			t.Error("SetLockKeys ответил true без менеджера")
		}
		return
	}
	if !ok {
		t.Skip("менеджер есть, но окно без фокуса клавиатуры: менять нечем")
	}
	t.Cleanup(func() { w.SetLockKeys(k) })
	want := k ^ keyboard.NumLock
	got := w.SetLockKeys(want)
	now, _ := w.LockKeys()
	t.Logf("SetLockKeys(%v) = %v, после: %v", want, got, now)
	if got && now != want {
		t.Errorf("true, а состояние %v, ждал %v", now, want)
	}
}
