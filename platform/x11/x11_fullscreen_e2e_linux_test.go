//go:build linux && !android

package x11

import (
	"os"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/native"
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

	// Колбэк зовёт цикл событий, а читает тест — срез под замком.
	var (
		mu     sync.Mutex
		events []bool
	)
	snapshot := func() []bool {
		mu.Lock()
		defer mu.Unlock()
		return append([]bool(nil), events...)
	}
	w.SetOnFullscreenChanged(func(on bool) {
		mu.Lock()
		events = append(events, on)
		mu.Unlock()
	})
	go w.RunEventLoop()

	if !w.SetFullscreen(true) {
		t.Skip("оконный менеджер не объявил _NET_WM_STATE_FULLSCREEN")
	}
	if !fsWaitFor(func() bool { return w.IsFullscreen() }) {
		t.Skip("оконный менеджер не перевёл окно в полный экран (нет WM или он не умеет)")
	}
	if ev := snapshot(); len(ev) < 1 || !ev[0] {
		t.Errorf("уведомления %v, ждал первым true", ev)
	}

	w.SetFullscreen(false)
	if !fsWaitFor(func() bool { return !w.IsFullscreen() }) {
		t.Fatal("после REMOVE окно осталось в полном экране")
	}
	if !fsWaitFor(func() bool { ev := snapshot(); return len(ev) > 0 && !ev[len(ev)-1] }) {
		t.Errorf("уведомления %v, ждал последним false", snapshot())
	}
}

// Мониторы реального сервера: хотя бы один, основной первым, границы осмысленны.
func TestX11MonitorsE2E(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}
	list := EnumMonitors()
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
	if !native.SortMonitors(list)[0].Primary {
		t.Error("основной монитор не первый")
	}
}
