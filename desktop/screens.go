// screens.go — модель экранов: прямоугольники мониторов и их рабочие области.
//
// Компоненты оболочки не знают, как устроены мониторы в системе (Win32,
// RandR, Wayland): потребитель описывает их интерфейсом Screens, как и
// остальные источники данных пакета. Панель задач, ставшая на монитор
// (Taskbar.DockTo), и её всплывающие панели (Flyout.SetMonitor) работают в
// координатах этого монитора, в общей системе логических координат рабочего
// стола — у второго монитора начало не в нуле.
package desktop

import (
	"image"
	"sync"
)

// Monitor — один экран.
type Monitor struct {
	// ID — устойчивый идентификатор: по нему панель находит свой монитор после
	// того, как список изменился. Пустой ID допустим для единственного экрана.
	ID string
	// Name — название для людей (подписи, журнал).
	Name string
	// Bounds — весь экран в логических координатах рабочего стола.
	Bounds image.Rectangle
	// WorkArea — экран без панелей и других зарезервированных полос. Пусто —
	// как Bounds. Панель задач, ставшая на монитор, сама вычитает свою полосу
	// (Taskbar.WorkArea); поле нужно потребителю, у которого есть и другие
	// зарезервированные полосы.
	WorkArea image.Rectangle
	// Primary — основной монитор.
	Primary bool
}

// Work возвращает рабочую область: WorkArea, а если она не задана — Bounds.
func (m Monitor) Work() image.Rectangle {
	if !m.WorkArea.Empty() {
		return m.WorkArea
	}
	return m.Bounds
}

// Screens — модель экранов от потребителя.
//
// Monitors возвращает текущий набор в стабильном порядке (основной монитор
// обычно первый, но порядок решает потребитель). Subscribe сообщает об
// изменении набора или геометрии (подключили монитор, сменили разрешение,
// переставили экраны) и возвращает отписку; замыкание зовётся из горутины
// потребителя, см. «Из какой горутины что зовётся» в contract.go.
type Screens interface {
	Monitors() []Monitor
	Subscribe(func()) func()
}

// FakeScreens — набор экранов, который меняет тест или демонстрация.
type FakeScreens struct {
	mu      sync.Mutex
	list    []Monitor
	subs    map[int]func()
	nextSub int
}

// NewFakeScreens создаёт модель с заданными мониторами.
func NewFakeScreens(ms ...Monitor) *FakeScreens {
	return &FakeScreens{list: append([]Monitor(nil), ms...), subs: map[int]func(){}}
}

// Monitors возвращает копию списка.
func (s *FakeScreens) Monitors() []Monitor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Monitor(nil), s.list...)
}

// SetMonitors заменяет список и уведомляет подписчиков.
func (s *FakeScreens) SetMonitors(ms ...Monitor) {
	s.mu.Lock()
	s.list = append([]Monitor(nil), ms...)
	s.mu.Unlock()
	s.notify()
}

// Subscribe подписывает на изменения набора мониторов.
func (s *FakeScreens) Subscribe(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	s.mu.Lock()
	s.nextSub++
	id := s.nextSub
	s.subs[id] = fn
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
	}
}

func (s *FakeScreens) notify() {
	s.mu.Lock()
	list := make([]func(), 0, len(s.subs))
	for _, fn := range s.subs {
		list = append(list, fn)
	}
	s.mu.Unlock()
	for _, fn := range list {
		fn()
	}
}

// monitorByID ищет монитор с идентификатором id.
func monitorByID(ms []Monitor, id string) (Monitor, bool) {
	for _, m := range ms {
		if m.ID == id {
			return m, true
		}
	}
	return Monitor{}, false
}
