package native

import (
	"image"
	"testing"
)

// MatchMonitor: ключ сильнее имени, имя сильнее границ; чужой монитор не
// находится; результат — запись списка со свежими границами.
func TestMatchMonitor(t *testing.T) {
	a := MonitorWithKey(Monitor{Name: "A", Bounds: image.Rect(0, 0, 1920, 1080)}, 1)
	b := MonitorWithKey(Monitor{Name: "B", Bounds: image.Rect(1920, 0, 3840, 1080)}, 2)
	list := []Monitor{a, b}

	if m, ok := MatchMonitor(list, MonitorWithKey(Monitor{Name: "A"}, 2)); !ok || MonitorKey(m) != 2 {
		t.Errorf("по ключу: %+v %v", m, ok)
	}
	// Ключ устарел (монитор переподключили), имя то же, границы сменились.
	moved := MonitorWithKey(Monitor{Name: "B", Bounds: image.Rect(0, 0, 10, 10)}, 99)
	if m, ok := MatchMonitor(list, moved); !ok || MonitorKey(m) != 2 || m.Bounds != b.Bounds {
		t.Errorf("по имени: %+v %v", m, ok)
	}
	if m, ok := MatchMonitor(list, Monitor{Bounds: a.Bounds}); !ok || MonitorKey(m) != 1 {
		t.Errorf("по границам: %+v %v", m, ok)
	}
	if _, ok := MatchMonitor(list, MonitorWithKey(Monitor{Name: "C", Bounds: image.Rect(5000, 0, 6000, 800)}, 5)); ok {
		t.Error("отключённый монитор нашёлся")
	}
	if _, ok := MatchMonitor(list, Monitor{}); ok {
		t.Error("пустой Monitor нашёлся")
	}
}
