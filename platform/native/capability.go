package native

// capability.go — реестр возможностей окна (ADR-0013 §3).
//
// Возможность (доступность, трей, перетаскивание, службы оболочки) живёт в
// своём пакете platform/* и регистрируется здесь из init; ядро окна
// (window/core) её не импортирует, а ищет в реестре во время работы и без неё
// отвечает «не поддерживается». Фасад window подключает все возможности
// пустыми импортами, window/minimal — ни одной.
//
// Ячейка возможности одна на процесс: последняя регистрация выигрывает,
// restore возвращает прежнюю (тестам — через t.Cleanup). Читать реестр можно
// только во время работы, не из init: порядок init пакета возможности и
// пакетов, которые её не импортируют, не определён.

import (
	"sort"
	"sync"
	"sync/atomic"
)

// capSlot — ячейка возможности. Неэкспортированный обобщённый тип:
// экспортированный обобщённый internal/facadegen не пропускает и с
// //facadegen:skip. Чтение — без замка, из любого потока окна.
type capSlot[T any] struct {
	name string
	p    atomic.Pointer[T]
}

// newCapSlot заводит ячейку возможности с именем name (для Capabilities).
func newCapSlot[T any](name string) *capSlot[T] {
	s := &capSlot[T]{name: name}
	capMu.Lock()
	capList = append(capList, s)
	capMu.Unlock()
	return s
}

// register кладёт v в ячейку; restore возвращает то, что было до вызова.
func (s *capSlot[T]) register(v T) (restore func()) {
	old := s.p.Swap(&v)
	return func() { s.p.Store(old) }
}

// get — зарегистрированное значение; ok=false — возможность не подключена.
func (s *capSlot[T]) get() (v T, ok bool) {
	if p := s.p.Load(); p != nil {
		return *p, true
	}
	return v, false
}

func (s *capSlot[T]) capName() string  { return s.name }
func (s *capSlot[T]) registered() bool { return s.p.Load() != nil }

var (
	capMu   sync.Mutex
	capList []interface {
		capName() string
		registered() bool
	}
)

// Capabilities — имена подключённых возможностей окна, по алфавиту:
// диагностика и тесты (window подключает все, window/minimal — ни одной).
//
//facadegen:skip
func Capabilities() []string {
	capMu.Lock()
	defer capMu.Unlock()
	var names []string
	for _, s := range capList {
		if s.registered() {
			names = append(names, s.capName())
		}
	}
	sort.Strings(names)
	return names
}
