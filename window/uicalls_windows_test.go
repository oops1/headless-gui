//go:build windows

package window

import "testing"

// TestUICallQueueTake проверяет обычный путь: колбэк кладётся и забирается
// один раз, повторный take даёт nil.
func TestUICallQueueTake(t *testing.T) {
	called := 0
	id := queueUICall(0xABC, func() { called++ })

	fn := takeUICall(id)
	if fn == nil {
		t.Fatal("колбэк не найден по id")
	}
	fn()
	if called != 1 {
		t.Fatalf("колбэк вызван %d раз", called)
	}
	if takeUICall(id) != nil {
		t.Error("колбэк остался в очереди после take")
	}
}

// TestDropUICallsOnDestroy проверяет дренаж очереди при разрушении окна:
// колбэки мёртвого окна выбрасываются, чужие остаются.
func TestDropUICallsOnDestroy(t *testing.T) {
	const dead, alive = uintptr(0x1001), uintptr(0x2002)

	d1 := queueUICall(dead, func() {})
	d2 := queueUICall(dead, func() {})
	a1 := queueUICall(alive, func() {})

	if n := dropUICalls(dead); n != 2 {
		t.Fatalf("дренировано %d колбэков, ожидалось 2", n)
	}
	if takeUICall(d1) != nil || takeUICall(d2) != nil {
		t.Error("колбэк мёртвого окна остался в очереди")
	}
	if takeUICall(a1) == nil {
		t.Error("колбэк живого окна выброшен вместе с чужими")
	}
	if n := dropUICalls(dead); n != 0 {
		t.Errorf("повторный дренаж вернул %d", n)
	}
}

// TestDropUICallsReleasesClosure проверяет, что очередь не удерживает
// замыкание разрушенного окна.
func TestDropUICallsReleasesClosure(t *testing.T) {
	const hwnd = uintptr(0x3003)
	queueUICall(hwnd, func() {})
	dropUICalls(hwnd)

	uiCallMu.Lock()
	defer uiCallMu.Unlock()
	for _, c := range uiCalls {
		if c.hwnd == hwnd {
			t.Fatal("замыкание осталось в uiCalls после дренажа")
		}
	}
}
