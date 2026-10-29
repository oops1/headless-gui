//go:build linux && !android

package core

import (
	"image"
	"testing"
	"time"
)

// Живая проверка против настоящего композитора (Weston/WSLg, WinLine и т. п.
// через WAYLAND_DISPLAY); без него тест пропускается. Кадры попапа идут
// чаще, чем композитор отпускает буферы (шаги перехода), — последний кадр
// должен лечь, а соединение не должно закрыться ошибкой протокола (буферы в
// пуле со смещением, attach при свободном буфере).

func TestWaylandPopupFramesE2E(t *testing.T) {
	w := newWaylandWindow()
	if w == nil {
		t.Skip("нет Wayland-композитора (WAYLAND_DISPLAY)")
	}
	if err := w.Create("wayland popup e2e", 320, 240); err != nil {
		t.Skipf("не удалось создать окно: %v", err)
	}
	defer w.Close()
	go w.RunEventLoop()
	w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 320, 240)))

	if err := w.OpenChildPopup(1, 20, 20, 120, 80); err != nil {
		t.Skipf("попап не открыт: %v", err)
	}
	w.popups.mu.Lock()
	p := w.popups.byID[1]
	w.popups.mu.Unlock()
	configured := func() bool { w.popups.mu.Lock(); defer w.popups.mu.Unlock(); return p.configured }
	if !fsWaitFor(configured) {
		t.Skip("композитор не прислал configure попапу")
	}

	// Двадцать кадров подряд, как шаги перехода: альфа кадра i — 10+i.
	deferred := 0
	const n = 20
	for i := 0; i < n; i++ {
		w.BlitChildPopup(1, popupImg(120, 80, uint8(10+i)), nil)
		w.popups.mu.Lock()
		if p.pending != nil {
			deferred++
		}
		w.popups.mu.Unlock()
	}
	idle := func() bool { w.popups.mu.Lock(); defer w.popups.mu.Unlock(); return p.pending == nil }
	if !fsWaitFor(idle) {
		t.Fatal("придержанный кадр так и не положен: release не пришёл")
	}
	if w.closed.Load() {
		t.Fatal("композитор закрыл соединение (ошибка протокола)")
	}
	p.drawMu.Lock()
	frame := p.stride * p.h
	last := []byte{p.data[3], p.data[frame+3]}
	p.drawMu.Unlock()
	if last[0] != 10+n-1 && last[1] != 10+n-1 {
		t.Errorf("альфа в буферах %v: последний кадр (%d) не положен", last, 10+n-1)
	}
	t.Logf("кадров %d, придержано на ходу %d, альфа в буферах %v", n, deferred, last)

	w.CloseChildPopup(1)
	time.Sleep(200 * time.Millisecond)
	if w.closed.Load() {
		t.Fatal("композитор закрыл соединение после закрытия попапа")
	}
}
