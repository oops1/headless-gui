//go:build linux && !android

package window

import (
	"os"
	"sync"
	"testing"
)

// Живая проверка против настоящего сервера. Без DISPLAY / WAYLAND_DISPLAY
// тесты пропускаются; что компоновщик или менеджер не умеют (сворачивание,
// xdg_activation_v1), приводит к Skip с пояснением, а не к ошибке.

func TestX11ActivateE2E(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}
	t.Setenv("HEADLESS_GUI_X11", "1")
	w := &X11Window{}
	if err := w.Create("x11 activate e2e", 320, 240); err != nil {
		t.Skipf("не удалось поднять окно: %v", err)
	}
	defer w.Close()

	var (
		mu     sync.Mutex
		events []bool
	)
	w.SetOnMinimizedChanged(func(m bool) {
		mu.Lock()
		events = append(events, m)
		mu.Unlock()
	})
	go w.RunEventLoop()

	if w.IsMinimized() {
		t.Fatal("новое окно свёрнуто")
	}
	// Активация уже показанного окна: запрос уходит без ошибок протокола.
	if !w.Activate() {
		t.Fatal("Activate вернул false на показанном окне")
	}
	w.Minimize()
	if !fsWaitFor(func() bool { return w.IsMinimized() }) {
		t.Skip("оконный менеджер не свернул окно (нет WM или он не умеет)")
	}
	if !w.Activate() {
		t.Fatal("Activate вернул false")
	}
	if !fsWaitFor(func() bool { return !w.IsMinimized() }) {
		t.Skip("менеджер не развернул окно по _NET_ACTIVE_WINDOW")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) < 2 || !events[0] || events[len(events)-1] {
		t.Errorf("уведомления свёрнутости %v, ждал true ... false", events)
	}
}

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
