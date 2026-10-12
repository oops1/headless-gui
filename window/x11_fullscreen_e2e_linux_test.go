//go:build linux && !android

package window

import (
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Живая проверка против настоящего X-сервера (Xvfb, WSLg/XWayland через
// DISPLAY). Без DISPLAY или без оконного менеджера, умеющего полный экран,
// тесты пропускаются: проверяется то, что оконный менеджер действительно
// отвечает на ClientMessage свойством _NET_WM_STATE, а мы это свойство читаем.

func TestX11FullscreenE2E(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}
	t.Setenv("HEADLESS_GUI_X11", "1")
	w := &X11Window{}
	if err := w.Create("x11 fullscreen e2e", 320, 240); err != nil {
		t.Skipf("не удалось поднять окно: %v", err)
	}
	defer w.Close()

	var events []bool
	var calls atomic.Int32
	w.SetOnFullscreenChanged(func(on bool) {
		events = append(events, on)
		calls.Add(1)
	})
	go w.RunEventLoop()

	if !w.SetFullscreen(true) {
		t.Skip("оконный менеджер не объявил _NET_WM_STATE_FULLSCREEN")
	}
	if !fsWaitFor(func() bool { return w.IsFullscreen() }) {
		t.Skip("оконный менеджер не перевёл окно в полный экран (нет WM или он не умеет)")
	}
	if calls.Load() < 1 || !events[0] {
		t.Errorf("уведомления %v, ждал первым true", events)
	}

	w.SetFullscreen(false)
	if !fsWaitFor(func() bool { return !w.IsFullscreen() }) {
		t.Fatal("после REMOVE окно осталось в полном экране")
	}
	if last := events[len(events)-1]; last {
		t.Errorf("последнее уведомление %v, ждал false", last)
	}
}

func fsWaitFor(cond func() bool) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// Мониторы реального сервера: хотя бы один, основной первым, границы осмысленны.
func TestX11MonitorsE2E(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}
	list := x11PlatformMonitors()
	if len(list) == 0 {
		t.Skip("X-сервер недоступен")
	}
	for _, m := range list {
		t.Logf("монитор %q: границы %v, рабочая область %v, масштаб %v, основной=%v",
			m.Name, m.Bounds, m.Work, m.Scale, m.Primary)
		if m.Bounds.Empty() || m.Work.Empty() || !m.Work.In(m.Bounds) || m.Scale <= 0 {
			t.Errorf("некорректный монитор: %+v", m)
		}
	}
	if !sortMonitors(list)[0].Primary {
		t.Error("основной монитор не первый")
	}
}
