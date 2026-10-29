// livetile.go — живые плитки меню «Пуск» (ADR-0010).
//
// Источник (TileSource) присылает состояния плитки; меню держит их очередь и
// перебирает по таймеру. Подписка живёт, только пока меню с плитками открыто:
// закрытое меню источник не вызывает и кадров не будит.
package desktop

import (
	"image"
	"math"
	"reflect"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
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

	// trans — идущий переход между состояниями (6.2); nil — покой.
	trans *tileTrans
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
			// Тема или плитки сменились: кэши перехода нарисованы прежним
			// стилем — перерисуются в ближайшем кадре.
			if st.trans != nil {
				st.trans.fromImg, st.trans.toImg = nil, nil
			}
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
// позвать вне замка. Идущий переход обрывается: плитка показывает новое.
func (st *liveState) drop() func() {
	st.stopTimer()
	st.stopTrans()
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
	first := st.fresh
	if st.fresh {
		st.queue, st.idx, st.fresh = nil, 0, false
	}
	st.queue = append(st.queue, c)
	if n := st.cfg.queueLen(); len(st.queue) > n {
		st.queue = append([]TileContent(nil), st.queue[len(st.queue)-n:]...)
	}
	st.idx = len(st.queue) - 1
	prev, had := st.shown, st.has
	st.shown, st.has = c, true
	st.stopTimer()
	if len(st.queue) > 1 {
		m.scheduleLocked(id, st)
	}
	ls.mu.Unlock()
	// Первое состояние новой подписки (повторное открытие меню) и повтор того
	// же содержимого — без перехода: плитка не должна «прокручиваться» на
	// месте.
	if had && !first && !sameTileContent(prev, c) {
		m.beginTransition(id, st, sub, prev, c)
	} else if had {
		m.cancelTransition(id, st)
	}
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
	prev := st.shown
	st.shown, st.has = st.queue[st.idx], true
	next := st.shown
	m.scheduleLocked(id, st)
	ls.mu.Unlock()
	if !sameTileContent(prev, next) {
		m.beginTransition(id, st, sub, prev, next)
	}
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

// ─── Переход между состояниями (6.2) ────────────────────────────────────────

// AnimTileCycle — переход живой плитки между состояниями: длительность и
// кривая. Не объявлено темой — по AnimFlyoutOpen. Декоративная анимация: при
// «меньше движения» и профилях MotionReduced/MotionOff — мгновенно; под
// MotionRemote — тоже мгновенно (переход по сети не стоит кадров).
const AnimTileCycle theme.Key = "tile.cycle"

// KeyTileCycleKind — вид перехода живой плитки: 0 — сдвиг (новое въезжает
// снизу и выталкивает прежнее вверх, по умолчанию), 1 — переворот вокруг
// горизонтальной оси.
const KeyTileCycleKind theme.Key = "tile.cycle.kind"

// tileTrans — переход плитки: два кэша растра (прежнее и новое содержимое,
// нарисованные вне кадра на прозрачном фоне) и доля пройденного. Кадр
// перехода только компонует кэши: содержимое плитки (текст, значок) заново
// не рисуется, остальное меню не трогается — заявлен прямоугольник плитки.
type tileTrans struct {
	from, to TileContent
	flip     bool
	k        float64
	anim     *widget.Animation

	// Кэши растра и для чего они нарисованы: логический размер плитки и
	// состояние (наведение меняет подложку).
	fromImg, toImg *image.RGBA
	size           image.Point
	state          theme.State
}

// stopTrans обрывает переход (под ls.mu).
func (st *liveState) stopTrans() {
	if st.trans != nil {
		if a := st.trans.anim; a != nil {
			a.Stop()
		}
		st.trans = nil
	}
}

// beginTransition заводит переход плитки id от prev к next. Мгновенно —
// длительность темы нулевая (в том числе «меньше движения», MotionOff),
// профиль движения MotionRemote или вид меню без плиток: тогда идущий переход
// обрывается и плитка сразу показывает next.
func (m *StartMenu) beginTransition(id TileID, st *liveState, sub uint64, prev, next TileContent) {
	ls := &m.v.live
	dur, curve := animationFirst(m.tm, AnimTileCycle, theme.AnimFlyoutOpen)
	if dur <= 0 || widget.CurrentMotionProfile() == theme.MotionRemote || !m.tiled() {
		m.cancelTransition(id, st)
		return
	}
	tr := &tileTrans{from: prev, to: next, flip: m.metricInt(KeyTileCycleKind) == 1}
	ls.mu.Lock()
	if !ls.live(id, st, sub) {
		ls.mu.Unlock()
		return
	}
	st.stopTrans()
	st.trans = tr
	ls.mu.Unlock()

	a := widget.AnimateOwned(m, "livetile:"+string(id), dur, curve, func(k float64) {
		ls.mu.Lock()
		if st.trans != tr {
			ls.mu.Unlock()
			return
		}
		tr.k = k
		if k >= 1 {
			st.trans = nil
		}
		ls.mu.Unlock()
		m.invalidateTile(id)
	})
	ls.mu.Lock()
	if st.trans == tr {
		tr.anim = a
	} else {
		a.Stop()
	}
	ls.mu.Unlock()
}

// cancelTransition обрывает идущий переход плитки: она сразу показывает
// новое.
func (m *StartMenu) cancelTransition(id TileID, st *liveState) {
	ls := &m.v.live
	ls.mu.Lock()
	if ls.tiles[id] == st {
		st.stopTrans()
	}
	ls.mu.Unlock()
}

// sameTileContent — одинаково ли выглядят состояния: повтор источника
// (опрос без перемен) перехода не заводит. Значки сравниваются по
// идентичности (картинка с несравнимым типом — разные), IconAt — по коду
// функции.
func sameTileContent(a, b TileContent) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	if a.Title != b.Title || a.Subtitle != b.Subtitle || a.Badge != b.Badge || a.Background != b.Background {
		return false
	}
	if a.Icon != b.Icon {
		return false
	}
	if (a.IconAt == nil) != (b.IconAt == nil) {
		return false
	}
	if a.IconAt != nil && reflect.ValueOf(a.IconAt).Pointer() != reflect.ValueOf(b.IconAt).Pointer() {
		return false
	}
	return true
}

// drawTileTransition рисует плитку t посреди перехода композицией двух кэшей
// и сообщает, нарисовал ли. Нет перехода или контекст не умеет рисовать вне
// кадра (OffscreenRenderer) — false: плитка рисуется обычным путём, сразу с
// новым содержимым.
func (m *StartMenu) drawTileTransition(ctx widget.DrawContext, rect image.Rectangle, t Tile, st theme.State, key string) bool {
	if t.Live.Source == nil {
		return false
	}
	ls := &m.v.live
	ls.mu.Lock()
	s := ls.tiles[t.ID]
	var tr *tileTrans
	if s != nil {
		tr = s.trans
	}
	if tr == nil {
		ls.mu.Unlock()
		return false
	}
	k, flip := tr.k, tr.flip
	from, to := tr.from, tr.to
	fi, ti := tr.fromImg, tr.toImg
	fresh := fi != nil && ti != nil && tr.size == rect.Size() && tr.state == st
	ls.mu.Unlock()

	if !fresh {
		or, ok := ctx.(widget.OffscreenRenderer)
		if !ok {
			return false
		}
		paint := func(c TileContent) *image.RGBA {
			tc := t
			tc.Content = c
			return or.RenderOffscreen(rect, func(oc widget.DrawContext) { m.paintTile(oc, rect, tc, st, key) })
		}
		fi, ti = paint(from), paint(to)
		if fi == nil || ti == nil {
			return false
		}
		ls.mu.Lock()
		// Пока идёт переход цвета наведения, стиль меняется каждый кадр:
		// картинки годятся только на этот кадр.
		if s.trans == tr && !m.fade.animating(key) {
			tr.fromImg, tr.toImg, tr.size, tr.state = fi, ti, rect.Size(), st
		}
		ls.mu.Unlock()
	}

	prevClip := ctx.Clip()
	ctx.SetClip(rect.Intersect(prevClip))
	defer ctx.SetClip(prevClip)
	h := rect.Dy()
	if !flip {
		off := int(math.Round(k * float64(h)))
		if off < h {
			widget.DrawImageRect(ctx, fi, fi.Bounds(), rect.Sub(image.Pt(0, off)), widget.FilterNearest)
		}
		if off > 0 {
			widget.DrawImageRect(ctx, ti, ti.Bounds(), rect.Add(image.Pt(0, h-off)), widget.FilterNearest)
		}
		return true
	}
	img, scale := fi, 1-2*k
	if k >= 0.5 {
		img, scale = ti, 2*k-1
	}
	if hh := int(math.Round(scale * float64(h))); hh > 0 {
		y := rect.Min.Y + (h-hh)/2
		widget.DrawImageRect(ctx, img, img.Bounds(), image.Rect(rect.Min.X, y, rect.Max.X, y+hh), widget.FilterBilinear)
	}
	return true
}
