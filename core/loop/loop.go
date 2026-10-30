// Package loop — цикл кадров движка: горутина цикла и её темп (тикер или
// внешний запрос кадра), доставка кадров стоку и в канал, рендер по запросу и
// повреждение кадра, очередь Post и Flush, таймеры After/Every (ADR-0011,
// задача 7.3).
//
// Пакет не знает о сцене: кадр готовит движок (Host.RenderFrame), анимации
// шагает он же (Host.StepAnimations), решение «нужен ли кадр без
// инвалидации» (каретка, подсказка, переходы) — тоже его. Движок
// (engine.Engine) создаёт Loop в конструкторе и отдаёт ему свои публичные
// методы цикла (Post, Flush, After, Every, SetPacing, RequestFrame,
// SetFrameSink, SetMaxFPS, Invalidate, InvalidateRect, SetRenderOnDemand);
// типы Timer, Pacing и FrameSink движок держит алиасами — открытый API не
// меняется.
//
// Замки. Все замки пакета листовые (ADR-0011): damageMu, postMu, timersMu,
// Timer.mu, sinkMu. Под ними не берут другой замок, не зовут Host и не зовут
// код потребителя (функции Post, таймеров, сток кадров) — проверка в
// lockorder_test.go. Движок зовёт Loop под своим frameMu (кадр забирает
// повреждение), но не под листовыми замками.
//
// Горутины. Post, Invalidate*, RequestFrame, таймеры — из любой горутины.
// Цикл (run) — своя горутина; функции очереди Post и таймеров выполняются на
// ней (у незапущенного цикла — на горутине RunPosted/Flush).
package loop

import (
	"image"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v4/internal/goid"
	"github.com/oops1/headless-gui/v4/output"
)

// Host — что цикл берёт у движка. Методы зовутся без удержанных замков
// пакета loop.
type Host interface {
	// RenderFrame готовит кадр (сам берёт замок кадра движка).
	RenderFrame() output.Frame
	// StepAnimations шагает анимации этого движка на момент now.
	StepAnimations(now time.Time)
	// AnimationNeeded — нужен ли кадр без инвалидации: мигающая каретка у
	// виджета с фокусом, дозревающая подсказка, шаг перехода. Вправе
	// заявить повреждение (NoteDamage) под кадр, который цикл и так готовит.
	AnimationNeeded(interval time.Duration) bool
	// FullFramePending — следующий кадр полный (холст пересоздан, кадр
	// потерян потребителем, поверхности сменились).
	FullFramePending() bool
	// GuardCall выполняет fn под границей сбоев движка where (Post, Timer,
	// Animation, FrameSink).
	GuardCall(where string, fn func())
	// MotionScope привязывает профиль движения движка к горутине
	// вызывающего; отвязка — результат (nil — не нужна).
	MotionScope() func()
	// BindLoop делает горутину цикла циклом анимаций этого движка; отвязка
	// — результат.
	BindLoop() func()
	// Scale — HiDPI-масштаб: повреждение хранится в физических пикселях.
	Scale() float64
	// OnInvalidateAll — перед полной инвалидацией: кадр, заставший её,
	// обязан застать и новое поколение записи команд.
	OnInvalidateAll()
}

// Counters — счётчики цикла для статистики движка.
type Counters struct {
	Posted, PostedRun uint64 // поставлено в очередь Post и выполнено
	Skipped           uint64 // тиков цикла без кадра
	Lost              uint64 // кадров, выброшенных из переполненного канала
	DeliverNanos      uint64 // время доставки кадров стоку и в канал
}

type counters struct {
	posted, postedRun, skipped, lost, deliverNs atomic.Uint64
}

// Loop — цикл кадров одного движка.
type Loop struct {
	host Host

	fps    int          // целевой FPS, 1–120
	maxFPS atomic.Int32 // потолок частоты кадров поверх fps; 0 — не задан (maxfps.go)

	started atomic.Bool // Start вызывался (Stop без Start не ждёт цикл)
	stopped atomic.Bool // Stop уже выполнен (идемпотентность)
	quit    chan struct{}
	done    chan struct{}

	// pacing, frameWanted, wake, sink — внешний темп и сток кадров
	// (pacing.go). При PacingExternal кадр готовится только по RequestFrame.
	pacing      atomic.Uint32
	frameWanted atomic.Bool
	wake        chan struct{}
	loopGID     atomic.Uint64 // горутина цикла кадров; 0 — цикл не идёт (см. Flush)
	sinkMu      sync.RWMutex
	sink        FrameSink

	frames chan output.Frame
	// lostFrame — кадр выброшен из переполненного канала, потребитель отстал:
	// следующий кадр полный, как только в канале есть место (движок,
	// fullframe.go).
	lostFrame atomic.Bool

	// ── Рендер по запросу (damage.go) ──────────────────────────────────────
	onDemand atomic.Bool
	invGen   atomic.Uint64

	damageMu  sync.Mutex
	damage    []image.Rectangle // области InvalidateRect с прошлого кадра
	damageAll bool              // Invalidate() — полный diff
	// force — ничейное повреждение кадра (путь команд движка): логические
	// области из InvalidateRect — виджет, чей охват их задевает,
	// перезаписывается. Под damageMu.
	force []image.Rectangle

	// postMu защищает posted: ставят в очередь из любых горутин, разбирает
	// горутина кадра (см. post.go).
	postMu sync.Mutex
	// posted — вызовы, ожидающие выполнения в горутине кадра.
	posted []func()

	// timersMu защищает timers — таймеры After/Every, живые у этого цикла.
	// Stop гасит их все (см. timer.go).
	timersMu sync.Mutex
	timers   map[*Timer]struct{}

	cnt counters
}

// New создаёт цикл движка host с целевой частотой fps (меньше 1 — 20).
// Рендер по запросу включён (режим по умолчанию движка).
func New(host Host, fps int) *Loop {
	if fps < 1 {
		fps = 20
	}
	l := &Loop{
		host:   host,
		fps:    fps,
		frames: make(chan output.Frame, 8),
		// wake будит цикл кадров при внешнем темпе: без него RequestFrame
		// ждал бы ближайшего тика, то есть темп остался бы тикерным.
		wake: make(chan struct{}, 1),
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}
	l.onDemand.Store(true)
	return l
}

// FPS — целевая частота кадров.
func (l *Loop) FPS() int { return l.fps }

// Counters — снимок счётчиков цикла.
func (l *Loop) Counters() Counters {
	return Counters{
		Posted:       l.cnt.posted.Load(),
		PostedRun:    l.cnt.postedRun.Load(),
		Skipped:      l.cnt.skipped.Load(),
		Lost:         l.cnt.lost.Load(),
		DeliverNanos: l.cnt.deliverNs.Load(),
	}
}

// ─── Жизненный цикл ─────────────────────────────────────────────────────────

// MarkStarted отмечает запуск: false — уже запущен или остановлен (повторный
// Start и Start после Stop ничего не делают).
func (l *Loop) MarkStarted() bool {
	return !l.stopped.Load() && l.started.CompareAndSwap(false, true)
}

// Run запускает горутину цикла (после MarkStarted).
func (l *Loop) Run() { go l.run() }

// MarkStopped отмечает остановку: false — уже остановлен (Stop идемпотентен).
func (l *Loop) MarkStopped() bool { return l.stopped.CompareAndSwap(false, true) }

// Halt останавливает цикл и ждёт его завершения: закрывает quit, гасит
// таймеры (stopped уже выставлен, новые не заведутся) и, если цикл
// запускался, ждёт выхода его горутины.
func (l *Loop) Halt() {
	close(l.quit)
	l.stopTimers()
	if l.started.Load() {
		<-l.done
	}
}

// CloseFrames закрывает канал кадров (после Halt).
func (l *Loop) CloseFrames() { close(l.frames) }

// Started — Start вызывался.
func (l *Loop) Started() bool { return l.started.Load() }

// Stopped — Stop вызывался.
func (l *Loop) Stopped() bool { return l.stopped.Load() }

// Running — цикл запущен и не остановлен: функции Post и таймеров выполняет
// он и только он. По флагам Start/Stop, а не по горутине: сразу после Start
// горутина цикла может ещё не записать свой номер.
func (l *Loop) Running() bool { return l.started.Load() && !l.stopped.Load() }

// InLoop — можно выполнять сразу: вызов идёт на горутине цикла или цикл не
// запущен (очередь разбирает вызывающий).
func (l *Loop) InLoop() bool {
	return !l.Running() || l.loopGID.Load() == goid.Current()
}

// OnLoopGoroutine — вызов идёт на горутине работающего цикла.
func (l *Loop) OnLoopGoroutine() bool {
	id := l.loopGID.Load()
	return id != 0 && id == goid.Current()
}

// GoroutineID — номер горутины цикла (0 — цикл не идёт). Для диагностики и
// тестов: логику на номере горутины строить нельзя.
func (l *Loop) GoroutineID() uint64 { return l.loopGID.Load() }

// TimerCount — сколько таймеров After/Every живо (для тестов утечек).
func (l *Loop) TimerCount() int {
	l.timersMu.Lock()
	defer l.timersMu.Unlock()
	return len(l.timers)
}

// Quit — закрывается при Stop: ожидание, которое не должно пережить
// остановку, выбирает и его.
func (l *Loop) Quit() <-chan struct{} { return l.quit }

// ─── Канал кадров ───────────────────────────────────────────────────────────

// Frames — канал готовых кадров (глубина 8); закрывается при Stop.
func (l *Loop) Frames() <-chan output.Frame { return l.frames }

// DrainFrames выбрасывает кадры, ждущие в канале: холст пересоздан, кадр
// прежнего размера не увидит никто.
//
// После CloseFrames (Stop) канал закрыт и приём из него всегда успешен:
// без проверки ok смена холста у остановленного движка крутилась бы здесь
// вечно.
func (l *Loop) DrainFrames() {
	for {
		select {
		case fr, ok := <-l.frames:
			if !ok {
				return
			}
			fr.Release()
			continue
		default:
		}
		return
	}
}

// frameRoom — в канале есть место.
func (l *Loop) frameRoom() bool { return len(l.frames) < cap(l.frames) }

// LostPending — кадр был потерян, и в канале снова есть место: следующий
// кадр должен быть полным.
func (l *Loop) LostPending() bool { return l.lostFrame.Load() && l.frameRoom() }

// TakeLost снимает признак потерянного кадра, если в канале есть место:
// true — следующий кадр полный.
func (l *Loop) TakeLost() bool {
	if l.lostFrame.Load() && l.frameRoom() {
		l.lostFrame.Store(false)
		return true
	}
	return false
}

// ClearLost снимает признак потерянного кадра (кадр и так полный).
func (l *Loop) ClearLost() { l.lostFrame.Store(false) }

// ─── Поколение и пробуждение ────────────────────────────────────────────────

// Gen — поколение инвалидации: растёт при каждом заявленном изменении.
func (l *Loop) Gen() uint64 { return l.invGen.Load() }

// BumpGen объявляет UI изменившимся без области: ближайший тик готовит кадр
// (шаги переходов, поверхности).
func (l *Loop) BumpGen() { l.invGen.Add(1) }

// Wake будит цикл (сигнал не копится: второй, пока первый не забран, не
// нужен).
func (l *Loop) Wake() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// sinceNanos — сколько наносекунд прошло с t; отрицательное (часы перевели)
// считается нулём.
func sinceNanos(t time.Time) uint64 {
	d := time.Since(t)
	if d < 0 {
		return 0
	}
	return uint64(d)
}
