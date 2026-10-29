package base

// drawtrace.go — какие виджеты рисуются прямо сейчас, и медленный путь
// отрисовки.
//
// Нужно движку для проверки контракта отрисовки (Engine.SetDrawContractCheck):
// Draw не меняет состояние виджета, а значит, не зовёт Invalidate, SetBounds,
// AddChild. Движок узнаёт о нарушении по уведомлению, пришедшему посреди
// кадра, но само уведомление не говорит, от кого оно. Этот учёт говорит: кто
// сейчас рисуется на этой горутине, от корня вглубь. Он же называет виджет,
// чей Draw запаниковал (политика сбоев, docs/adr/0001).
//
// Учёт по горутинам, а не один на процесс: движков бывает несколько, и
// рисуют они одновременно, каждый на своей горутине.
//
// Медленный путь — один счётчик на процесс (drawSlowPath): его поднимают и
// включённый учёт, и сломанные виджеты (broken.go). Пока он нулевой, обход
// детей стоит одну атомарную загрузку на ребёнка; ненулевой стоит номера
// горутины и замка на каждый виджет, и включать учёт стоит только для
// отладки.

import (
	"sync"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/internal/goid"
)

var (
	// drawSlowPath — сколько причин идти медленным путём: включения учёта
	// плюс сломанные виджеты. Счётчик, а не флаг: движок, выключивший
	// проверку у себя, не должен выключать её у соседа.
	drawSlowPath atomic.Int32

	// drawTraceRefs — сколько потребителей включили учёт рисующихся.
	drawTraceRefs atomic.Int32

	drawTraceMu     sync.Mutex
	drawTraceStacks map[uint64][]Widget
)

// EnableDrawTrace включает учёт рисующихся виджетов и возвращает функцию,
// которая его выключает. Включений может быть несколько, учёт работает, пока
// не выключено последнее; повторный вызов возвращённой функции ничего не
// делает.
func EnableDrawTrace() (disable func()) {
	drawTraceRefs.Add(1)
	drawSlowPath.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			drawTraceRefs.Add(-1)
			drawSlowPath.Add(-1)
		})
	}
}

// drawTraceOn — включён ли учёт.
func drawTraceOn() bool { return drawTraceRefs.Load() > 0 }

// DrawTraceUsers — сколько включений учёта не выключено. Для тестов: учёт
// общий на процесс, и забытое включение замедляет кадры всех движков.
func DrawTraceUsers() int { return int(drawTraceRefs.Load()) }

// DrawTraced рисует w, отмечая его рисующимся на время Draw, а сломанный —
// заменяет заглушкой. Пока учёт выключен и сломанных нет — то же, что
// w.Draw(ctx). Для контейнеров, которые обходят детей своим циклом, а не
// через DrawChildren.
//
// Под рекордером команд (retain.go) это единственная точка входа удержания:
// Draw виджета с верным списком не зовётся вовсе.
//
// Обёртка короткая: при нулевом медленном пути она компилируется в тот же
// вызов, что и прежде. Ветка с учётом вынесена отдельно — функция с defer не
// встраивается, и контекст, переданный через неё, уходил бы в кучу на каждом
// кадре (тест попапов без аллокаций это ловит).
func DrawTraced(w Widget, ctx DrawContext) {
	if r, off, ok := retainerOf(ctx); ok {
		if h, ok := w.(ListHolder); ok {
			if bp, ok := w.(baseProvider); ok {
				drawRetained(w, h, bp.baseRef(), r, off, ctx)
				return
			}
		}
	}
	if drawSlowPath.Load() > 0 {
		drawTracedSlow(w, ctx)
		return
	}
	w.Draw(ctx)
}

func drawTracedSlow(w Widget, ctx DrawContext) {
	if IsBroken(w) {
		drawBrokenPlaceholder(w, ctx)
		return
	}
	if !drawTraceOn() {
		w.Draw(ctx)
		return
	}
	// Снятие со стека — без defer, намеренно: при панике в Draw defer
	// выполнился бы раньше, чем движок перехватит панику, и стек был бы уже
	// пуст, когда движок спросит, кто упал. Паника оставляет стек как есть,
	// движок читает его и сбрасывает (ResetDrawTrace).
	id := pushDrawTrace(w)
	w.Draw(ctx)
	popDrawTrace(id)
}

// DrawOverlayTraced рисует оверлей виджета w (выпадающий список, меню),
// отмечая w рисующимся; у сломанного виджета оверлей не рисуется. Устроена
// как DrawTraced.
func DrawOverlayTraced(w Widget, od OverlayDrawer, ctx DrawContext) {
	if drawSlowPath.Load() > 0 {
		drawOverlayTracedSlow(w, od, ctx)
		return
	}
	od.DrawOverlay(ctx)
}

func drawOverlayTracedSlow(w Widget, od OverlayDrawer, ctx DrawContext) {
	if IsBroken(w) {
		return
	}
	if !drawTraceOn() {
		od.DrawOverlay(ctx)
		return
	}
	id := pushDrawTrace(w)
	od.DrawOverlay(ctx)
	popDrawTrace(id)
}

// DrawTracePath возвращает виджеты, рисующиеся сейчас на вызывающей горутине,
// от внешнего к внутреннему (последний — тот, чей Draw идёт). nil — учёт
// выключен или ничего не рисуется. Срез — копия.
func DrawTracePath() []Widget {
	if !drawTraceOn() {
		return nil
	}
	id := goid.Current()
	drawTraceMu.Lock()
	defer drawTraceMu.Unlock()
	st := drawTraceStacks[id]
	if len(st) == 0 {
		return nil
	}
	return append([]Widget(nil), st...)
}

// ResetDrawTrace забывает стек рисующихся на вызывающей горутине. Движок зовёт
// это после паники в Draw: раскрутка стека прошла мимо popDrawTrace, и без
// сброса следующий кадр начинал бы с чужого хвоста.
func ResetDrawTrace() {
	if !drawTraceOn() {
		return
	}
	id := goid.Current()
	drawTraceMu.Lock()
	delete(drawTraceStacks, id)
	drawTraceMu.Unlock()
}

func pushDrawTrace(w Widget) uint64 {
	id := goid.Current()
	drawTraceMu.Lock()
	if drawTraceStacks == nil {
		drawTraceStacks = make(map[uint64][]Widget)
	}
	drawTraceStacks[id] = append(drawTraceStacks[id], w)
	drawTraceMu.Unlock()
	return id
}

func popDrawTrace(id uint64) {
	drawTraceMu.Lock()
	st := drawTraceStacks[id]
	if n := len(st); n > 1 {
		st[n-1] = nil
		drawTraceStacks[id] = st[:n-1]
	} else {
		// Горутина дорисовала: запись убирается, чтобы карта не росла от
		// горутин, которых больше нет.
		delete(drawTraceStacks, id)
	}
	drawTraceMu.Unlock()
}
