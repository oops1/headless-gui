//go:build windows

package core

import (
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testhook"
)

// Полный экран Win32 на настоящем окне: окно создаётся в этом же потоке, поэтому
// SetWindowPos/ShowWindow выполняют wndProc синхронно и цикл сообщений не нужен.

// Мониторы: основной присутствует, прямоугольники осмысленны, масштаб ≥ 1.
func TestWin32Monitors(t *testing.T) {
	list := Monitors()
	if len(list) == 0 {
		t.Fatal("Monitors() пуст на Windows")
	}
	if !list[0].Primary {
		t.Errorf("основной монитор не первый: %+v", list[0])
	}
	for _, m := range list {
		if m.Bounds.Empty() || m.Work.Empty() || !m.Work.In(m.Bounds) {
			t.Errorf("некорректные границы: %+v", m)
		}
		if m.Scale < 1 {
			t.Errorf("масштаб %v меньше 1: %+v", m.Scale, m)
		}
	}
	// Основной монитор всегда содержит начало координат.
	if m, ok := MonitorAt(0, 0); !ok || !m.Primary {
		t.Errorf("точка (0,0) не на основном мониторе: %+v %v", m, ok)
	}
}

func TestWin32MonitorOf(t *testing.T) {
	w := newWin32TestWindow(t)
	win := &Window{}
	win.native = w
	m, ok := MonitorOf(win)
	if !ok {
		t.Fatal("MonitorOf не нашёл монитор окна")
	}
	pos := testhook.Win32WindowRect(w)
	if pos.Intersect(m.Bounds).Empty() {
		t.Errorf("окно %v не пересекает свой монитор %v", pos, m.Bounds)
	}
	if _, ok := MonitorOf(nil); ok {
		t.Error("MonitorOf(nil) нашёл монитор")
	}
}
