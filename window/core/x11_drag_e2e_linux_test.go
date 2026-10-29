//go:build linux && !android

package core

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// Сквозная проверка с НАСТОЯЩИМ X-сервером (если он есть; иначе пропуск): два
// окна одного процесса, у каждого своё соединение. Источник захватывает
// указатель, ведёт приёмника по XDND, тот забирает файлы через XdndSelection.
// Мышью не управляем (нет XTest): движение и отпускание подаются событиями
// прямо в источник, а вот всё, что идёт через сервер, — настоящее: захват,
// ClientMessage'ы, владение выделением, SelectionRequest/Notify, свойства.

func TestX11E2E_DragBetweenWindows(t *testing.T) {
	a, b := &X11Window{}, &X11Window{}
	if err := a.Create("src", 200, 150); err != nil {
		t.Skipf("нет X-сервера: %v", err)
	}
	if err := b.Create("dst", 200, 150); err != nil {
		t.Skipf("второе окно: %v", err)
	}
	a.StartEventPump()
	b.StartEventPump()
	t.Cleanup(func() { b.Close(); a.Close() })

	var mu sync.Mutex
	var log []string
	note := func(s string) { mu.Lock(); log = append(log, s); mu.Unlock() }
	dropped := make(chan []string, 1)
	b.SetOnDragEnter(func(x, y int) { note("enter") })
	b.SetOnDragOver(func(x, y int) bool { note("over"); return true })
	b.SetOnDragLeave(func() { note("leave") })
	b.SetOnFilesDropped(func(p []string, x, y int) { dropped <- p })

	done := make(chan native.DragEffect, 1)
	files := []string{"/tmp/один файл.png", "/tmp/b.jpg"}
	a.drag.find = func(rx, ry int) (uint32, int) { return b.wid, 5 }
	a.btnDown.Store(1)
	if !a.BeginDragFiles(files, native.DragEffectCopy|native.DragEffectMove, func(e native.DragEffect) { done <- e }) {
		t.Fatal("BeginDragFiles отказал")
	}
	deadline := time.Now().Add(3 * time.Second)
	for !a.drag.active.Load() {
		select {
		case e := <-done:
			t.Skipf("сервер не дал захватить указатель (итог %v)", e)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("перетаскивание не началось")
		}
		time.Sleep(5 * time.Millisecond)
	}

	bx, by := b.GetPosition()
	a.handleX11Event(motion(bx+20, by+30, 0))
	waitFor(t, "XdndStatus принят", func() bool {
		a.drag.mu.Lock()
		defer a.drag.mu.Unlock()
		return a.drag.accepted
	})
	a.handleX11Event(buttonEv(5, 1))

	select {
	case e := <-done:
		if e != native.DragEffectCopy {
			t.Errorf("итог %v, ждал copy", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("XdndFinished не пришёл")
	}
	select {
	case p := <-dropped:
		if !reflect.DeepEqual(p, files) {
			t.Errorf("сброшено %q, ждал %q", p, files)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetOnFilesDropped не вызван")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(log) < 2 || log[0] != "enter" || log[1] != "over" {
		t.Errorf("события приёмника: %v, ждал enter, over…", log)
	}
	for _, s := range log {
		if s == "leave" {
			t.Errorf("события %v: после drop leave приходить не должен", log)
		}
	}
	if a.drag.active.Load() {
		t.Error("источник остался активным")
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
