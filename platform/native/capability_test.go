package native

import (
	"sync"
	"testing"
)

// Последняя регистрация выигрывает, restore возвращает прежнюю.
func TestCapSlot_RegisterRestore(t *testing.T) {
	s := &capSlot[int]{name: "test"}
	if _, ok := s.get(); ok || s.registered() {
		t.Fatal("пустая ячейка отвечает ok")
	}
	r1 := s.register(1)
	r2 := s.register(2)
	if v, ok := s.get(); !ok || v != 2 {
		t.Fatalf("get = %d, %v; ждал 2 (последняя регистрация)", v, ok)
	}
	r2()
	if v, ok := s.get(); !ok || v != 1 {
		t.Fatalf("после restore второй get = %d, %v; ждал 1", v, ok)
	}
	r1()
	if _, ok := s.get(); ok {
		t.Fatal("после restore первой ячейка не пуста")
	}
}

// Чтение и запись из разных горутин (go test -race).
func TestCapSlot_Concurrent(t *testing.T) {
	s := &capSlot[func() int]{name: "test"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			restore := s.register(func() int { return 1 })
			restore()
		}()
		go func() {
			defer wg.Done()
			if f, ok := s.get(); ok && f != nil {
				_ = f
			}
		}()
	}
	wg.Wait()
}

// Capabilities перечисляет только зарегистрированные ячейки, по алфавиту.
func TestCapabilities_ListsRegistered(t *testing.T) {
	b := newCapSlot[int]("zz-test-b")
	a := newCapSlot[int]("zz-test-a")
	t.Cleanup(func() {
		capMu.Lock()
		capList = capList[:len(capList)-2]
		capMu.Unlock()
	})
	has := func(name string) bool {
		for _, n := range Capabilities() {
			if n == name {
				return true
			}
		}
		return false
	}
	if has("zz-test-a") || has("zz-test-b") {
		t.Fatal("незарегистрированная возможность в списке")
	}
	defer b.register(2)()
	defer a.register(1)()
	got := Capabilities()
	ia, ib := -1, -1
	for i, n := range got {
		switch n {
		case "zz-test-a":
			ia = i
		case "zz-test-b":
			ib = i
		}
	}
	if ia < 0 || ib < 0 || ia > ib {
		t.Fatalf("Capabilities = %v: ждал zz-test-a раньше zz-test-b", got)
	}
}
