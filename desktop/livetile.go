// livetile.go — живые плитки меню «Пуск» (ADR-0010).
//
// Источник (TileSource) присылает состояния плитки; меню держит их очередь и
// перебирает по таймеру. Подписка живёт, только пока меню с плитками открыто:
// закрытое меню источник не вызывает и кадров не будит.
package desktop

import (
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// TileSource — источник живого содержимого плитки: часы, погода, почта,
// календарь. Реализует потребитель.
//
// Subscribe подписывает fn на состояния плитки и возвращает отписку. fn можно
// звать из любой горутины, в том числе синхронно из Subscribe (обычно источник
// сразу присылает текущее состояние); меню переносит обновление в поток
// интерфейса через StartMenu.Post. Отписка может прийти из любой горутины;
// после неё источник может быть подписан снова (меню открыли повторно).
type TileSource interface {
	Subscribe(fn func(TileContent)) (unsubscribe func())
}

// TileSourceFunc — TileSource из функции подписки.
type TileSourceFunc func(fn func(TileContent)) (unsubscribe func())

// Subscribe вызывает f.
func (f TileSourceFunc) Subscribe(fn func(TileContent)) func() { return f(fn) }

// LiveTile — живое содержимое плитки (Tile.Live). Значение, а не объект с
// состоянием: очередь и таймер держит меню, которое показывает плитку, —
// у меню на каждом мониторе свои.
type LiveTile struct {
	// Source — источник состояний; nil — плитка не живая.
	Source TileSource
	// Queue — сколько последних состояний плитка перебирает; 0 — 5.
	Queue int
	// Period — пауза между состояниями очереди; 0 — DefaultTileCycle.
	Period time.Duration
}

// DefaultTileCycle — пауза между состояниями живой плитки по умолчанию.
const DefaultTileCycle = 6 * time.Second

// defaultTileQueue — длина очереди состояний по умолчанию (как у Windows 10).
const defaultTileQueue = 5

func (l LiveTile) queueLen() int {
	if l.Queue > 0 {
		return l.Queue
	}
	return defaultTileQueue
}

func (l LiveTile) period() time.Duration {
	if l.Period > 0 {
		return l.Period
	}
	return DefaultTileCycle
}

// liveSet — состояние живых плиток одного меню.
type liveSet struct {
	mu sync.Mutex
	// on — меню открыто и подписки действуют.
	on    bool
	tiles map[TileID]*liveState
}

// liveState — одна живая плитка. Подписку и шаги таймера узнают по указателю
// на состояние и его номерам (sub, tick): состояние от снятой подписки и шаг
// остановленного таймера отбрасываются, не доходя до Post.
type liveState struct {
	cfg LiveTile
	// sub растёт при каждой подписке и отписке плитки.
	sub        uint64
	subscribed bool
	unsub      func()

	queue []TileContent
	idx   int
	shown TileContent
	has   bool
	// fresh — подписка новая: первое состояние начинает очередь заново.
	fresh bool

	timer *time.Timer
	tick  uint64
}

// liveContent — что показывать на плитке t: последнее состояние источника,
// а до него — содержимое по умолчанию.
func (m *StartMenu) liveContent(t Tile) TileContent {
	if t.Live.Source == nil {
		return t.Content
	}
	ls := &m.v.live
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if st := ls.tiles[t.ID]; st != nil && st.has {
		return st.shown
	}
	return t.Content
}

// sameSource — тот же ли источник. Источник с несравнимым динамическим типом
// (TileSourceFunc — функция) считается новым: сравнение паникует.
func sameSource(a, b TileSource) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}

// liveAttach приводит подписки живых плиток открытого меню к текущим плиткам и
// виду:
//   - плитка с тем же источником сохраняет подписку, очередь и показанное —
//     смена плиток или темы не теряет состояний, уже поставленных в Post;
//   - плитка, у которой источник сменился или которой больше нет, отписана, а
//     состояния прежнего источника сброшены;
//   - у меню без плиток (Windows 11, плоский список) подписок нет, но показанное
//     сохраняется до возвращения плиток.
func (m *StartMenu) liveAttach() {
	want := map[TileID]LiveTile{}
	m.v.mu.Lock()
	for _, g := range m.v.groups {
		for _, t := range g.Tiles {
			if t.Live.Source != nil {
				want[t.ID] = t.Live
			}
		}
	}
	m.v.mu.Unlock()
	tiled := m.tiled()

	type pending struct {
		id  TileID
		st  *liveState
		sub uint64
		src TileSource
	}
	var subs []pending
	var unsubs []func()

	ls := &m.v.live
	ls.mu.Lock()
	// Открытость — под замком: Close сбрасывает флаг до liveDetach, и смена
	// плиток с другой горутины, проверившая IsOpen раньше, не оставит
	// подписку закрытому меню.
	if !m.IsOpen() {
		ls.mu.Unlock()
		return
	}
	ls.on = true
	old := ls.tiles
	ls.tiles = make(map[TileID]*liveState, len(want))
	for id, st := range old {
		cfg, ok := want[id]
		if ok && sameSource(st.cfg.Source, cfg.Source) {
			if st.cfg.Period != cfg.Period {
				st.stopTimer()
			}
			st.cfg = cfg
			if n := cfg.queueLen(); len(st.queue) > n {
				st.queue = append([]TileContent(nil), st.queue[len(st.queue)-n:]...)
				st.idx = len(st.queue) - 1
				st.shown = st.queue[st.idx]
			}
			ls.tiles[id] = st
			continue
		}
		if u := st.drop(); u != nil {
			unsubs = append(unsubs, u)
		}
	}
	for id, cfg := range want {
		if ls.tiles[id] == nil {
			ls.tiles[id] = &liveState{cfg: cfg}
		}
	}
	for id, st := range ls.tiles {
		if !tiled {
			if u := st.drop(); u != nil {
				unsubs = append(unsubs, u)
			}
			st.stopTimer()
			continue
		}
		if !st.subscribed {
			st.sub++
			st.subscribed, st.fresh = true, true
			subs = append(subs, pending{id: id, st: st, sub: st.sub, src: st.cfg.Source})
		}
		if st.timer == nil && len(st.queue) > 1 {
			m.scheduleLocked(id, st)
		}
	}
	ls.mu.Unlock()
	for _, u := range unsubs {
		u()
	}

	// Подписка вне замка: источник вправе позвать fn синхронно.
	for _, p := range subs {
		p := p
		u := p.src.Subscribe(func(c TileContent) {
			m.liveDeliver(p.id, p.st, p.sub, 0, func() { m.liveApply(p.id, p.st, p.sub, c) })
		})
		if u == nil {
			continue
		}
		ls.mu.Lock()
		keep := ls.live(p.id, p.st, p.sub)
		if keep {
			p.st.unsub = u
		}
		ls.mu.Unlock()
		if !keep {
			// Меню закрыли (или плитку сняли), пока шла подписка.
			u()
		}
	}
}

// drop снимает подписку плитки (под ls.mu) и отдаёт отписку, которую надо
// позвать вне замка.
func (st *liveState) drop() func() {
	st.stopTimer()
	if !st.subscribed {
		return nil
	}
	st.sub++
	st.subscribed = false
	u := st.unsub
	st.unsub = nil
	return u
}

// live — действует ли подписка sub плитки id (под ls.mu).
func (ls *liveSet) live(id TileID, st *liveState, sub uint64) bool {
	return ls.on && ls.tiles[id] == st && st.subscribed && st.sub == sub
}

// liveDetach снимает подписки и таймеры закрытого меню. Показанное остаётся:
// при следующем открытии плитка начнёт с него.
func (m *StartMenu) liveDetach() {
	ls := &m.v.live
	ls.mu.Lock()
	ls.on = false
	var unsubs []func()
	for _, st := range ls.tiles {
		if u := st.drop(); u != nil {
			unsubs = append(unsubs, u)
		}
	}
	ls.mu.Unlock()
	for _, u := range unsubs {
		u()
	}
}

// liveDeliver переносит fn в поток интерфейса, если подписка (и, при tick ≠ 0,
// шаг таймера) ещё действует; иначе ничего не делает — закрытое меню не будит
// движок. Поток интерфейса — StartMenu.Post, без него — движок дерева меню
// (widget.PostFor); меню вне дерева обновляется на месте.
func (m *StartMenu) liveDeliver(id TileID, st *liveState, sub, tick uint64, fn func()) {
	ls := &m.v.live
	ls.mu.Lock()
	ok := ls.live(id, st, sub) && (tick == 0 || st.tick == tick)
	ls.mu.Unlock()
	if !ok {
		return
	}
	if post := m.Post; post != nil {
		post(fn)
		return
	}
	if widget.PostFor(m, fn) {
		return
	}
	fn()
}

// liveApply ставит новое состояние в очередь плитки и показывает его.
func (m *StartMenu) liveApply(id TileID, st *liveState, sub uint64, c TileContent) {
	ls := &m.v.live
	ls.mu.Lock()
	if !ls.live(id, st, sub) {
		ls.mu.Unlock()
		return
	}
	if st.fresh {
		st.queue, st.idx, st.fresh = nil, 0, false
	}
	st.queue = append(st.queue, c)
	if n := st.cfg.queueLen(); len(st.queue) > n {
		st.queue = append([]TileContent(nil), st.queue[len(st.queue)-n:]...)
	}
	st.idx = len(st.queue) - 1
	st.shown, st.has = c, true
	st.stopTimer()
	if len(st.queue) > 1 {
		m.scheduleLocked(id, st)
	}
	ls.mu.Unlock()
	m.invalidateTile(id)
}

// liveAdvance показывает следующее состояние очереди (шаг таймера tick).
func (m *StartMenu) liveAdvance(id TileID, st *liveState, sub, tick uint64) {
	ls := &m.v.live
	ls.mu.Lock()
	if !ls.live(id, st, sub) || st.tick != tick || len(st.queue) < 2 {
		ls.mu.Unlock()
		return
	}
	st.idx = (st.idx + 1) % len(st.queue)
	st.shown, st.has = st.queue[st.idx], true
	m.scheduleLocked(id, st)
	ls.mu.Unlock()
	m.invalidateTile(id)
}

// scheduleLocked заводит таймер следующего шага перебора (под ls.mu).
func (m *StartMenu) scheduleLocked(id TileID, st *liveState) {
	st.stopTimer()
	st.tick++
	tick, sub := st.tick, st.sub
	st.timer = time.AfterFunc(st.cfg.period(), func() {
		m.liveDeliver(id, st, sub, tick, func() { m.liveAdvance(id, st, sub, tick) })
	})
}

func (st *liveState) stopTimer() {
	if st.timer != nil {
		st.timer.Stop()
		st.timer = nil
	}
}

// invalidateTile заявляет изменившейся только плитку id (если она видна).
func (m *StartMenu) invalidateTile(id TileID) {
	if r := m.tileRectAbs(id); !r.Empty() {
		m.InvalidateArea(r)
	}
}
