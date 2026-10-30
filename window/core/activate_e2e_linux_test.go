//go:build linux && !android

package core

import (
	"testing"
)

// Живая проверка против настоящего сервера. Без DISPLAY / WAYLAND_DISPLAY
// тесты пропускаются; что компоновщик или менеджер не умеют (сворачивание,
// xdg_activation_v1), приводит к Skip с пояснением, а не к ошибке.

func TestWaylandActivateE2E(t *testing.T) {
	w := newWaylandWindow()
	if w == nil {
		t.Skip("нет Wayland-композитора (WAYLAND_DISPLAY)")
	}
	if err := w.Create("wayland activate e2e", 320, 240); err != nil {
		t.Skipf("не удалось создать окно: %v", err)
	}
	defer w.Close()
	go w.RunEventLoop()

	if w.gActivation == 0 {
		if w.Activate() || w.ActivateWithToken("tok") {
			t.Error("композитор без xdg_activation_v1, а активация ответила true")
		}
		t.Skip("композитор не объявил xdg_activation_v1")
	}
	// Запрос токена уходит; композитор вправе ответить done и принять activate
	// либо промолчать — протокол ошибок за «плохой» токен не даёт.
	if !w.Activate() {
		t.Fatal("Activate вернул false при наличии xdg_activation_v1")
	}
	if !w.ActivateWithToken("headless-gui-e2e-token") {
		t.Fatal("ActivateWithToken вернул false при наличии xdg_activation_v1")
	}
	if w.IsMinimized() {
		t.Error("IsMinimized на Wayland должен быть false")
	}
}
