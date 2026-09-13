package widget

// anim.go — ядро анимационного фреймворка (тайминг + прогресс), интегрированное
// с рендером по запросу движка.
//
// Модель: анимация не хранит собственных часов — часы принадлежат движку.
// Раз в кадр движок зовёт StepAnimations(now), продвигая все активные анимации
// к моменту now. tick(t) получает ПРОГРЕСС ПОСЛЕ кривой (t ∈ [0,1]).
//
// Взаимодействие с on-demand циклом:
//   - Animate/AnimateOwned будят спящий цикл через notifyUIChanged (тот же
//     нотификатор, что регистрирует движок как e.Invalidate) — иначе анимация,
//     запущенная из колбэка вне кадра, не начнётся до следующего события;
//   - AnimationsActive() движок опрашивает в animationNeeded — пока есть
//     активные анимации, кадры не пропускаются;
//   - тики сами двигают сеттеры виджетов (SetValue/SetBounds/...), те
//     самоинвалидируются (авто-damage) → перерисовка частичная.
//
// Гарантии StepAnimations (см. требования задачи):
//  1. тики зовутся ВНЕ мьютекса аниматора;
//  2. из тика можно звать Animate/AnimateOwned/Stop (цепочки) без дедлока;
//     анимация, рождённая в тике, НЕ получает шаг в этом же Step и не теряется;
//  3. порядок тиков — порядок регистрации;
//  4. потокобезопасно.

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v3/internal/goid"
)

// Easing определён в widget/easing.go (другой файл пакета):
//   type Easing func(t float64) float64
// Здесь тип НЕ переопределяется — используется как есть.

// Animation — одна активная анимация. Создаётся через Animate/AnimateOwned;
// напрямую не конструируется вызывающим кодом.
type Animation struct {
	duration time.Duration
	curve    Easing
	tick     func(t float64)

	// started/start — ленивый старт: часы фиксируются на ПЕРВОМ Step,
	// а не при регистрации (часы принадлежат движку).
	started bool
	start   time.Time

	// owner/tag — для семантики CSS-transition: новая анимация с тем же
	// (owner, tag) останавливает предыдущую. owner сравнивается по == .
	owner any
	tag   string

	// loop — ключ цикла, который шагает анимацию (BindAnimationLoop); nil —
	// анимация ещё ничья, её заберёт первый шагнувший цикл.
	loop any

	// reversed — текущее направление в цикле AutoReverse (true = фаза 1→0).
	reversed bool

	done    bool // завершилась штатно (был вызван/будет вызван OnDone)
	stopped bool // снята через Stop (без OnDone)

	// Конфигурационные поля. КОНТРАКТ: выставляются СРАЗУ после Animate/
	// AnimateOwned, в той же горутине, ДО передачи управления рендер-циклу.
	// Движок читает их в своей горутине под мьютексом аниматора; запись в
	// живую (уже шагающую) анимацию из другой горутины — гонка. Из тика
	// анимации менять их безопасно (тик исполняется вне мьютекса, но в
	// потоке Step). Для «завершения» из чужой горутины используйте Stop.
	OnDone      func() // вызывается по штатному завершению (не при Stop)
	Loop        bool   // после завершения начинается заново
	AutoReverse bool   // 0→1→0 считается ОДНИМ циклом
}

// animator — реестр активных анимаций процесса.
//
// Реестр общий, а шагает его каждый движок на своей горутине: анимация
// привязана к циклу, на горутине которого её завели, и тики зовутся только
// оттуда. Без привязки тик анимации в диалоге вторичного движка мог
// выполниться на горутине главного — параллельно с обработчиками диалога
// (GG-68).
type animator struct {
	mu     sync.Mutex
	active []*Animation // порядок = порядок регистрации
	// stepping — циклы посреди прохода: защита от вложенного Step из тика.
	// Анимация, заведённая во время прохода, в снимок прохода не попадает и
	// получит шаг на следующем.
	stepping map[any]bool
}

var anim animator

// stepAllKey — ключ прохода StepAnimations: все анимации, чьи бы они ни были.
type stepAllKey struct{}

// animLoops — горутины циклов, шагающих анимации: id горутины → ключ цикла.
// animLoopCount избавляет от поиска, пока циклов нет (тесты без движка).
var (
	animLoops     sync.Map
	animLoopCount atomic.Int32
)

// BindAnimationLoop объявляет текущую горутину циклом анимаций с ключом key и
// возвращает функцию, снимающую объявление. Анимации, заведённые на этой
// горутине — из обработчика, из Post, из тика или OnDone, — шагает только
// StepAnimationsFor(key).
//
// Для движков; приложению звать не нужно.
func BindAnimationLoop(key any) (unbind func()) {
	id := goid.Current()
	animLoops.Store(id, key)
	animLoopCount.Add(1)
	return func() {
		animLoops.Delete(id)
		animLoopCount.Add(-1)
	}
}

// currentAnimLoop возвращает ключ цикла текущей горутины или nil.
func currentAnimLoop() any {
	if animLoopCount.Load() == 0 {
		return nil
	}
	key, _ := animLoops.Load(goid.Current())
	return key
}

// add регистрирует анимацию в реестр за циклом текущей горутины и будит
// движок.
func (am *animator) add(a *Animation) {
	a.loop = currentAnimLoop()
	am.mu.Lock()
	am.active = append(am.active, a)
	am.mu.Unlock()
	// Разбудить спящий on-demand цикл: даже если Animate вызван вне кадра
	// (из колбэка/горутины), ближайший тик отрендерит первый шаг.
	notifyUIChanged()
}

// step зовёт покадровый колбэк, если он есть.
//
// Пустой колбэк — законный случай: анимация без него не рисует ничего и
// служит таймером «позови по завершении» (OnDone). Такие заводит,
// например, предпросмотр окна на панели задач — задержка до появления и
// период обновления миниатюры. Раньше StepAnimations звала tick без
// проверки и на таком таймере падала.
func (a *Animation) step(t float64) {
	if a.tick != nil {
		a.tick(t)
	}
}

// Animate регистрирует анимацию длительностью dur с кривой curve (nil →
// линейная). tick(t) вызывается с ПРОГРЕССОМ ПОСЛЕ кривой (t ∈ [0,1]):
// первый тик — t по фактическому времени первого шага, последний —
// гарантированно ровно 1.0 (после чего OnDone). Будит движок.
// Владельца нет — Loop-анимацию остановит только Stop.
func Animate(dur time.Duration, curve Easing, tick func(t float64)) *Animation {
	a := &Animation{
		duration: dur,
		curve:    curve,
		tick:     tick,
	}
	anim.add(a)
	return a
}

// AnimateOwned — как Animate, но новая анимация с тем же (owner, tag)
// ОСТАНАВЛИВАЕТ предыдущую (семантика CSS-transition: анимации не дерутся).
// owner сравнивается по == (указатель виджета), tag — строка ("bounds", "fade").
// Loop-анимация снимается сама, когда владелец скрыт (!IsVisible).
func AnimateOwned(owner any, tag string, dur time.Duration, curve Easing, tick func(t float64)) *Animation {
	a := &Animation{
		duration: dur,
		curve:    curve,
		tick:     tick,
		owner:    owner,
		tag:      tag,
	}
	// Снимаем предыдущую с тем же (owner,tag) — в том числе родившуюся в
	// текущем Step: она уже в active.
	if owner != nil {
		anim.mu.Lock()
		for _, other := range anim.active {
			if !other.stopped && !other.done && other.owner == owner && other.tag == tag {
				other.stopped = true
			}
		}
		anim.mu.Unlock()
	}
	anim.add(a)
	return a
}

// Stop снимает анимацию без вызова OnDone. Идемпотентно и безопасно из тика.
func (a *Animation) Stop() {
	anim.mu.Lock()
	a.stopped = true
	anim.mu.Unlock()
}

// ownerHidden — владелец известен и скрыт.
func (a *Animation) ownerHidden() bool {
	v, ok := a.owner.(interface{ IsVisible() bool })
	return ok && !v.IsVisible()
}

// Running сообщает, активна ли анимация (не завершена и не снята).
func (a *Animation) Running() bool {
	anim.mu.Lock()
	r := !a.done && !a.stopped
	anim.mu.Unlock()
	return r
}

// progress вычисляет прогресс кривой t ∈ [0,1] на момент now и признак
// завершения фазы. Обрабатывает reverse-фазу AutoReverse.
// Вызывается БЕЗ мьютекса (a обрабатывается монопольно в рамках Step).
// animPhase — снимок полей анимации, нужных для расчёта прогресса.
//
// Отдельный тип, а не чтение из *Animation: фаза тиков идёт без мьютекса, и
// снимок делает невозможным случайное чтение живого поля оттуда.
type animPhase struct {
	start    time.Time
	duration time.Duration
	curve    Easing
	reversed bool
}

func (a animPhase) progress(now time.Time) (t float64, phaseDone bool) {
	// Линейный прогресс по времени фазы.
	var lin float64
	if a.duration <= 0 {
		lin = 1
	} else {
		elapsed := now.Sub(a.start)
		lin = float64(elapsed) / float64(a.duration)
	}
	if lin < 0 {
		lin = 0
	}
	if lin >= 1 {
		lin = 1
		phaseDone = true
	}
	// Направление в AutoReverse-цикле: на обратной фазе прогресс идёт 1→0.
	dir := lin
	if a.reversed {
		dir = 1 - lin
	}
	// Применяем кривую (nil → линейная).
	if a.curve != nil {
		t = a.curve(dir)
	} else {
		t = dir
	}
	return t, phaseDone
}

// StepAnimations продвигает все активные анимации к моменту now и возвращает
// true, если после шага остались активные. Движок вызывает раз в кадр.
//
// Порядок:
//  1. под мьютексом снимаем снапшот active (порядок регистрации) и отмечаем
//     цикл в stepping — новые Animate встают в конец active, в снапшот не
//     попадают и шаг получат на следующем проходе;
//  2. ВНЕ мьютекса зовём тики в порядке снапшота, вычисляя завершения;
//  3. под мьютексом применяем результаты: перезапуск Loop/фазу AutoReverse,
//     удаление завершённых/снятых;
//  4. OnDone-колбэки зовём ВНЕ мьютекса (они могут звать Animate/Stop).
//
// Шагает ВСЕ анимации, к какому бы циклу они ни были привязаны: для тестов и
// приложений без движка. Движок зовёт StepAnimationsFor.
func StepAnimations(now time.Time) bool {
	return stepAnimations(stepAllKey{}, now)
}

// StepAnimationsFor — StepAnimations для одного цикла: продвигает анимации,
// привязанные к key (BindAnimationLoop), и ещё ничьи — их цикл забирает себе.
// Возвращает, остались ли активные у этого цикла. Движок зовёт раз в кадр на
// своей горутине.
func StepAnimationsFor(key any, now time.Time) bool {
	if key == nil {
		return StepAnimations(now)
	}
	return stepAnimations(key, now)
}

func stepAnimations(key any, now time.Time) bool {
	_, all := key.(stepAllKey)
	anim.mu.Lock()
	if anim.stepping[key] || anim.stepping[stepAllKey{}] || (all && len(anim.stepping) > 0) {
		// Вложенный Step (из тика) не поддерживаем — просто сообщаем, есть ли
		// активные. Проход «всех» не пересекается ни с каким другим: он шагает
		// и чужие анимации.
		hasActive := anim.hasActiveLocked(key)
		anim.mu.Unlock()
		return hasActive
	}
	if anim.stepping == nil {
		anim.stepping = map[any]bool{}
	}
	anim.stepping[key] = true
	// Снимок: продвигаем только анимации этого цикла, активные ДО этого Step.
	snapshot := make([]*Animation, 0, len(anim.active))
	for _, a := range anim.active {
		if !all {
			if a.loop == nil {
				a.loop = key // ничья — цикл забирает её себе
			}
			if a.loop != key {
				continue
			}
		}
		snapshot = append(snapshot, a)
	}
	// Лениво стартуем анимации, у которых ещё нет часов — старт = now.
	for _, a := range snapshot {
		if !a.started && !a.stopped {
			a.started = true
			a.start = now
		}
	}
	// Поля, нужные для расчёта прогресса, снимаем ЗДЕСЬ, под мьютексом.
	//
	// Фаза тиков идёт вне мьютекса — так задумано: тик зовёт код виджета,
	// который вправе сам обратиться к анимациям. Но читать оттуда поля живой
	// анимации нельзя: пишутся они под мьютексом (ленивый старт выше,
	// перезапуск цикла ниже), и чтение без него — гонка, которую детектор
	// ловит у потребителя, как только на экране появляются часы. Снимок
	// примитивов стоит дешевле, чем блокировка вокруг чужого кода.
	phase := make([]animPhase, len(snapshot))
	for i, a := range snapshot {
		phase[i] = animPhase{
			start:    a.start,
			duration: a.duration,
			curve:    a.curve,
			reversed: a.reversed,
		}
	}
	anim.mu.Unlock()

	// ── Фаза тиков (вне мьютекса) ────────────────────────────────────────
	// Для каждой анимации вычисляем t и зовём tick. Копим завершившиеся,
	// чтобы после тиков применить Loop/AutoReverse/OnDone.
	type doneItem struct {
		a  *Animation
		cb func()
	}
	var completed []doneItem

	for i, a := range snapshot {
		// Могла быть снята из предыдущего тика этого же прохода.
		anim.mu.Lock()
		skip := a.stopped || a.done
		loop := a.Loop
		anim.mu.Unlock()
		if skip {
			continue
		}
		// Скрытый владелец: вечный цикл некому смотреть — снимаем.
		if loop && a.ownerHidden() {
			anim.mu.Lock()
			a.stopped = true
			anim.mu.Unlock()
			continue
		}

		t, phaseDone := phase[i].progress(now)

		if !phaseDone {
			a.step(t)
			continue
		}

		// Фаза завершилась. Обработка зависит от AutoReverse/Loop.
		if a.AutoReverse && !a.reversed {
			// Прошла прямая фаза 0→1 — по времени t уже равен curve(1) на
			// границе, но семантически цикл ещё не закончен. Начинаем
			// обратную фазу 1→0, сдвигая часы на now.
			a.step(t) // t здесь = curve(1) (или reversed-эквивалент) — граница
			anim.mu.Lock()
			a.reversed = true
			a.start = now
			anim.mu.Unlock()
			continue
		}

		// Цикл завершён (обычная анимация — после прямой фазы; AutoReverse —
		// после обратной). Гарантируем финальный тик ровно с конечным
		// значением.
		final := 0.0
		if a.AutoReverse {
			// Конец обратной фазы: dir=0 → curve(0).
			if a.curve != nil {
				final = a.curve(0)
			} else {
				final = 0
			}
		} else {
			// Конец прямой фазы: dir=1 → curve(1). Требование: последний тик
			// ровно 1.0 для линейной; для кривой — curve(1).
			if a.curve != nil {
				final = a.curve(1)
			} else {
				final = 1
			}
		}
		a.step(final)

		if a.Loop {
			// Перезапуск: сбрасываем фазу и часы на now.
			anim.mu.Lock()
			a.reversed = false
			a.start = now
			anim.mu.Unlock()
			continue
		}

		// Штатное завершение — пометим done, OnDone вызовем после мьютекса.
		anim.mu.Lock()
		a.done = true
		cb := a.OnDone
		anim.mu.Unlock()
		completed = append(completed, doneItem{a: a, cb: cb})
	}

	// ── Фаза очистки (под мьютексом) ─────────────────────────────────────
	anim.mu.Lock()
	// Пересобираем active: выкидываем done/stopped, сохраняем порядок.
	// Заведённые во время прохода уже стоят в конце active.
	kept := anim.active[:0]
	for _, a := range anim.active {
		if a.done || a.stopped {
			continue
		}
		kept = append(kept, a)
	}
	clear(anim.active[len(kept):]) // не держать снятые анимации
	anim.active = kept
	delete(anim.stepping, key)
	hasActive := anim.hasActiveLocked(key)
	anim.mu.Unlock()

	// ── OnDone (вне мьютекса; колбэки могут звать Animate/Stop) ───────────
	for _, d := range completed {
		if d.cb != nil {
			d.cb()
		}
	}
	// OnDone мог зарегистрировать новые анимации. Пересчитываем актуальное
	// наличие активных.
	if !hasActive {
		anim.mu.Lock()
		hasActive = anim.hasActiveLocked(key)
		anim.mu.Unlock()
	}
	return hasActive
}

// hasActiveLocked сообщает, есть ли активные анимации у цикла key (при
// удержанном mu). Ничьи считаются: их заберёт первый шагнувший цикл.
func (am *animator) hasActiveLocked(key any) bool {
	if _, all := key.(stepAllKey); all {
		return len(am.active) > 0
	}
	for _, a := range am.active {
		if !a.done && !a.stopped && (a.loop == nil || a.loop == key) {
			return true
		}
	}
	return false
}

// AnimationsActive сообщает, есть ли зарегистрированные анимации.
//
// Движок этим НЕ решает, готовить ли кадр: анимация сама заявляет, что
// изменила, а StepAnimations проходит до решения о пропуске — см.
// Engine.animationNeeded. Функция осталась для тестов и приложений, ждущих
// завершения анимации.
func AnimationsActive() bool {
	anim.mu.Lock()
	active := len(anim.active) > 0
	anim.mu.Unlock()
	return active
}

// StopAllAnimations снимает все анимации без OnDone (для тестов/шатдауна).
func StopAllAnimations() {
	anim.mu.Lock()
	for _, a := range anim.active {
		a.stopped = true
	}
	clear(anim.active)
	anim.active = anim.active[:0]
	anim.mu.Unlock()
}
