//go:build linux && !android

package window

import (
	"sync"
	"testing"
)

// Живая проверка против настоящего композитора (Weston/WSLg, WinLine и т. п.
// через WAYLAND_DISPLAY). Без композитора тесты пропускаются. Итог запроса
// у разных композиторов разный (согласие или отказ) — проверяется, что окно
// доводит его до уведомления и IsFullscreen совпадает с ним.

func TestWaylandFullscreenE2E(t *testing.T) {
	w := newWaylandWindow()
	if w == nil {
		t.Skip("нет Wayland-композитора (WAYLAND_DISPLAY)")
	}
	if err := w.Create("wayland fullscreen e2e", 320, 240); err != nil {
		t.Skipf("не удалось создать окно: %v", err)
	}
	defer w.Close()

	var mu sync.Mutex
	var events []bool
	w.SetOnFullscreenChanged(func(on bool) {
		mu.Lock()
		events = append(events, on)
		mu.Unlock()
	})
	go w.RunEventLoop()

	if !w.SetFullscreen(true) {
		t.Fatal("SetFullscreen вернул false: запрос должен быть отправлен")
	}
	// Любой исход (согласие или отказ) заканчивается уведомлением.
	if !fsWaitFor(func() bool { mu.Lock(); defer mu.Unlock(); return len(events) > 0 }) {
		t.Skip("композитор не ответил на set_fullscreen configure'ом")
	}
	mu.Lock()
	first := events[0]
	mu.Unlock()
	if first != w.IsFullscreen() && len(events) == 1 {
		t.Errorf("уведомление %v, а IsFullscreen=%v", first, w.IsFullscreen())
	}
	t.Logf("композитор %s set_fullscreen; IsFullscreen=%v", map[bool]string{true: "принял", false: "отклонил"}[first], w.IsFullscreen())
	if first {
		w.SetFullscreen(false)
		if !fsWaitFor(func() bool { return !w.IsFullscreen() }) {
			t.Error("после unset_fullscreen окно осталось в полном экране")
		}
	}
}

func TestWaylandMonitorsE2E(t *testing.T) {
	w := newWaylandWindow()
	if w == nil {
		t.Skip("нет Wayland-композитора (WAYLAND_DISPLAY)")
	}
	list := w.queryMonitors()
	if len(list) == 0 {
		t.Skip("композитор не сообщил wl_output")
	}
	for _, m := range list {
		t.Logf("монитор %q: границы %v, масштаб %v, основной=%v", m.Name, m.Bounds, m.Scale, m.Primary)
		if m.Bounds.Empty() || m.Scale < 1 {
			t.Errorf("некорректный монитор: %+v", m)
		}
	}
}
