//go:build linux && !android

package x11

import (
	"os"
	"sync"
	"testing"
)

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
