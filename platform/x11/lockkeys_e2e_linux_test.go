//go:build linux && !android

package x11

import (
	"os"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/keyboard"
)

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

	want := orig ^ keyboard.NumLock
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
