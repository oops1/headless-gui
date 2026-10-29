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

// ─── Живые плитки ───────────────────────────────────────────────────────────

// FakeTileSource — источник живой плитки (TileSource), которым распоряжается
// тест или демонстрация: часы, почта, погода. Set меняет состояние и
// рассылает его подписчикам; подписка получает текущее состояние сразу, как
// настоящий источник при открытии меню.
//
// Каждое состояние несёт номер версии, и подписчик принимает только более
// новые: параллельные Set и Subscribe не оставляют его на устаревшем
// состоянии, даже если доставки пришли в обратном порядке.
type FakeTileSource struct {
	mu      sync.Mutex
	cur     TileContent
	has     bool
	ver     uint64 // номер текущего состояния: растёт при каждом Set
	subs    map[int]*tileSub
	nextSub int
}

var _ TileSource = (*FakeTileSource)(nil)

// NewFakeTileSource создаёт источник. initial — первое состояние; без него
// подписчик ничего не получит, пока Set не вызван.
func NewFakeTileSource(initial ...TileContent) *FakeTileSource {
	s := &FakeTileSource{subs: map[int]*tileSub{}}
	if len(initial) > 0 {
		s.cur, s.has, s.ver = initial[0], true, 1
	}
	return s
}

// Current возвращает текущее состояние; false, если его ещё не было.
func (s *FakeTileSource) Current() (TileContent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur, s.has
}

// Set меняет состояние и рассылает его подписчикам.
func (s *FakeTileSource) Set(c TileContent) {
	s.mu.Lock()
	s.ver++
	ver := s.ver
	s.cur, s.has = c, true
	list := make([]*tileSub, 0, len(s.subs))
	for _, sub := range s.subs {
		list = append(list, sub)
	}
	s.mu.Unlock()
	// Доставка вне замка источника: подписчик вправе отписаться или снова
	// вызвать Set.
	for _, sub := range list {
		sub.deliver(ver, c)
	}
}

// Subscribe подписывает fn на состояния. Если состояние уже есть, fn получает
// его синхронно. Возвращает отписку (повторный вызов безопасен); после неё
// fn больше не вызывается.
func (s *FakeTileSource) Subscribe(fn func(TileContent)) func() {
	if fn == nil {
		return func() {}
	}
	sub := &tileSub{fn: fn}
	s.mu.Lock()
	s.nextSub++
	id := s.nextSub
	s.subs[id] = sub
	cur, has, ver := s.cur, s.has, s.ver
	s.mu.Unlock()
	if has {
		sub.deliver(ver, cur)
	}
	return func() {
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
		sub.close()
	}
}

// tileSub — подписчик FakeTileSource. Доставляет состояния по одному и только
// более новые, чем уже доставленное. Вызов fn идёт без замка подписчика, так
// что fn вправе вызвать Set: вложенная доставка ставится в очередь и уходит
// тем же циклом после возврата из fn, без взаимной блокировки.
type tileSub struct {
	fn func(TileContent)

	mu      sync.Mutex
	last    uint64 // номер последнего доставленного состояния
	pending bool
	pver    uint64 // номер отложенного состояния
	pval    TileContent
	running bool // цикл доставки уже идёт в какой-то горутине
	closed  bool
}

// deliver принимает состояние ver. Если цикл доставки идёт, состояние ставится
// в очередь и уходит им; иначе этот вызов доставляет всё по порядку.
func (s *tileSub) deliver(ver uint64, c TileContent) {
	s.mu.Lock()
	if s.closed || ver <= s.last || (s.pending && ver <= s.pver) {
		s.mu.Unlock()
		return
	}
	s.pending, s.pver, s.pval = true, ver, c
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	for s.pending && !s.closed {
		v, val := s.pver, s.pval
		s.pending = false
		s.last = v
		s.mu.Unlock()
		s.fn(val)
		s.mu.Lock()
	}
	s.running = false
	s.mu.Unlock()
}

// close снимает подписчика: очередь сбрасывается, новые доставки отбрасываются.
func (s *tileSub) close() {
	s.mu.Lock()
	s.closed = true
	s.pending = false
	s.mu.Unlock()
}
