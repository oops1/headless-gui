package desktop

import "sync"

// FakeStartSource — источник списка приложений меню «Пуск» с плитками, которым
// распоряжается тест или демонстрация: недавние, алфавитные группы, папки.
// Рассылает уведомления подписчикам при замене данных, как настоящий.
type FakeStartSource struct {
	mu      sync.Mutex
	recent  []StartEntry
	groups  []StartLetterGroup
	subs    map[int]func()
	nextSub int
}

var _ StartMenuSource = (*FakeStartSource)(nil)

// NewFakeStartSource создаёт источник с заданными данными.
func NewFakeStartSource(recent []StartEntry, groups []StartLetterGroup) *FakeStartSource {
	return &FakeStartSource{recent: recent, groups: groups, subs: map[int]func(){}}
}

// Recent возвращает раздел «Недавно добавленные».
func (s *FakeStartSource) Recent() []StartEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]StartEntry(nil), s.recent...)
}

// Groups возвращает алфавитные группы.
func (s *FakeStartSource) Groups() []StartLetterGroup {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]StartLetterGroup(nil), s.groups...)
}

// Set заменяет данные и уведомляет подписчиков.
func (s *FakeStartSource) Set(recent []StartEntry, groups []StartLetterGroup) {
	s.mu.Lock()
	s.recent = append([]StartEntry(nil), recent...)
	s.groups = append([]StartLetterGroup(nil), groups...)
	list := make([]func(), 0, len(s.subs))
	for _, f := range s.subs {
		list = append(list, f)
	}
	s.mu.Unlock()
	for _, f := range list {
		f()
	}
}

// Subscribe подписывает на смену данных.
func (s *FakeStartSource) Subscribe(fn func()) func() {
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
