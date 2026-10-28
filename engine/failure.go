package engine

// failure.go — политика сбоев: что движок делает с паникой в чужом коде.
//
// Решение — docs/adr/0001-failure-policy.md. Коротко: по умолчанию паника в
// Draw, обработчике ввода, функции из Post, таймере, тике анимации или стоке
// кадров ловится, виновник помечается сломанным и рисуется заглушкой, а
// процесс живёт. Под go test политика по умолчанию — прежняя: паника идёт
// дальше, и тесты видят её как раньше.

import (
	"fmt"
	"log"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// FailurePolicy — что движок делает с паникой в коде виджета или потребителя.
type FailurePolicy uint8

const (
	// FailureIsolate — паника ловится на своей границе (кадр, функция из
	// Post, таймер, тик анимации, событие ввода, сток кадров), виджет, чей
	// Draw упал, помечается сломанным и рисуется заглушкой, процесс живёт.
	// Политика по умолчанию вне go test.
	FailureIsolate FailurePolicy = iota
	// FailurePropagate — паника идёт дальше с исходным стеком, как в 3.x.
	// Политика по умолчанию под go test (testing.Testing()).
	FailurePropagate
)

// String — имя политики для журнала.
func (p FailurePolicy) String() string {
	switch p {
	case FailureIsolate:
		return "isolate"
	case FailurePropagate:
		return "propagate"
	}
	return fmt.Sprintf("FailurePolicy(%d)", uint8(p))
}

// PanicInfo — что движок знает о пойманной панике.
type PanicInfo struct {
	// Where — граница: "Draw", "Post", "Timer", "Animation", "FrameSink" или
	// имя метода ввода ("SendMouseButton").
	Where string
	// Widget — виновник, если известен (паника в Draw при включённом учёте
	// рисующихся; при первой панике в Draw учёт включается, и виновника
	// называет повторная). nil — неизвестен.
	Widget widget.Widget
	// Path — путь виновника от корня: "Panel > Window > Label". Пустой, если
	// виджет неизвестен.
	Path string
	// Value — значение паники.
	Value any
	// Stack — стек в момент паники (runtime/debug.Stack).
	Stack []byte
}

// failureState — состояние политики у движка.
type failureState struct {
	policy atomic.Uint32

	mu      sync.Mutex
	onPanic func(PanicInfo)
	// traceRelease — учёт рисующихся, включённый после первой паники в Draw,
	// чтобы следующая назвала виновника. nil — не включён.
	traceRelease func()
	// said — о каких парах «место + виджет» уже написано в журнал: виджет,
	// падающий каждый кадр, не должен писать каждый кадр.
	said map[panicKey]struct{}
}

type panicKey struct {
	where string
	w     widget.Widget
}

// defaultFailurePolicy — политика нового движка: изоляция, а под go test —
// прежнее поведение, чтобы тесты видели паники.
func defaultFailurePolicy() FailurePolicy {
	if testing.Testing() {
		return FailurePropagate
	}
	return FailureIsolate
}

// SetFailurePolicy задаёт политику сбоев. Безопасно из любой горутины;
// действует на паники, пойманные после вызова.
func (e *Engine) SetFailurePolicy(p FailurePolicy) { e.failure.policy.Store(uint32(p)) }

// FailurePolicy возвращает текущую политику сбоев.
func (e *Engine) FailurePolicy() FailurePolicy { return FailurePolicy(e.failure.policy.Load()) }

// SetOnPanic задаёт обработчик пойманных паник (при FailureIsolate). Зовётся
// на той горутине, где паника случилась, уже после восстановления; nil
// возвращает запись в log по умолчанию. Обработчик получает каждую панику,
// журнал по умолчанию — одну на пару «место + виджет».
func (e *Engine) SetOnPanic(fn func(PanicInfo)) {
	e.failure.mu.Lock()
	e.failure.onPanic = fn
	e.failure.mu.Unlock()
}

// guard — общий defer для границ. Вызывать как defer e.guard(where)():
// внешний вызов снимает политику ДО паники, внутренняя функция делает
// recover. При FailurePropagate recover не вызывается, и паника идёт дальше
// с исходным стеком.
func (e *Engine) guard(where string) func() {
	if e.FailurePolicy() == FailurePropagate {
		return func() {}
	}
	return func() {
		if r := recover(); r != nil {
			e.caught(where, r)
		}
	}
}

// guardInput — guard для границы ввода: после паники в обработчике снимается
// захват мыши, забывается потребитель нажатия и отменяются касания — иначе
// упавший обработчик нажатия оставил бы мышь захваченной навсегда. Фокус не
// трогается: он принадлежит программе.
func (e *Engine) guardInput(where string) func() {
	// Профиль движения (motionprofile.go) — и при FailurePropagate.
	unbind := e.motionScope()
	if e.FailurePolicy() == FailurePropagate {
		if unbind != nil {
			return unbind
		}
		return func() {}
	}
	return func() {
		// Снимается последней: отмена касаний и захвата ниже тоже зовёт
		// обработчики виджетов, и профиль им нужен тот же.
		if unbind != nil {
			defer unbind()
		}
		if r := recover(); r != nil {
			e.caught(where, r)
			e.ReleaseCapture()
			e.pressConsumer = nil
			e.CancelTouches()
		}
	}
}

// guardCall выполняет fn под защитой границы where. Для вызовов в цикле:
// defer в цикле не раскрывается компилятором, а здесь он один на вызов.
func (e *Engine) guardCall(where string, fn func()) {
	defer e.guard(where)()
	fn()
}

// caught — паника поймана: счётчик, виновник, уведомление.
func (e *Engine) caught(where string, r any) {
	e.stats.panics.Add(1)
	info := PanicInfo{Where: where, Value: r, Stack: debug.Stack()}
	if dp, ok := r.(*directPanic); ok {
		// Паника прямого виджета в воркере тайлового растра (tileframe.go):
		// виновник и стек сняты там, где она случилась.
		r = dp.value
		info.Value, info.Stack, info.Widget = dp.value, dp.stack, dp.w
		e.endDrawContract()
		widget.MarkBroken(dp.w)
		widget.ResetDrawTrace()
		e.fullNext.Store(true)
		e.Invalidate()
	} else if where == "Draw" {
		info.Widget, info.Path = e.identifyDrawPanic()
	}
	e.failure.mu.Lock()
	fn := e.failure.onPanic
	first := false
	if fn == nil {
		if e.failure.said == nil {
			e.failure.said = make(map[panicKey]struct{})
		}
		k := panicKey{where: where, w: info.Widget}
		if _, ok := e.failure.said[k]; !ok {
			e.failure.said[k] = struct{}{}
			first = true
		}
	}
	e.failure.mu.Unlock()
	if fn != nil {
		fn(info)
		return
	}
	if first {
		log.Printf("engine: паника в %s%s: %v (политика %s; далее для этого места — только счётчик Stats.Panics)\n%s",
			where, describeWidget(info), r, e.FailurePolicy(), info.Stack)
	}
}

// identifyDrawPanic — кто упал в Draw. При включённом учёте рисующихся
// виновник известен: он помечается сломанным, учёт, включённый ради него,
// снимается. При выключенном — учёт включается, и виновника назовёт
// следующая паника. В любом случае стек учёта на этой горутине сбрасывается
// (раскрутка прошла мимо popDrawTrace), а следующий кадр — полный.
func (e *Engine) identifyDrawPanic() (w widget.Widget, path string) {
	e.endDrawContract()
	if p := widget.DrawTracePath(); len(p) > 0 {
		w = p[len(p)-1]
		_, _, path = describeDrawPath(p)
		widget.MarkBroken(w)
	}
	widget.ResetDrawTrace()
	e.failure.mu.Lock()
	switch {
	case w != nil && e.failure.traceRelease != nil:
		e.failure.traceRelease()
		e.failure.traceRelease = nil
	case w == nil && e.failure.traceRelease == nil:
		e.failure.traceRelease = widget.EnableDrawTrace()
	}
	e.failure.mu.Unlock()
	e.fullNext.Store(true)
	e.Invalidate()
	return w, path
}

// releaseFailureTrace снимает учёт, включённый ради поиска виновника: при
// остановке движка он никому больше не нужен.
func (e *Engine) releaseFailureTrace() {
	e.failure.mu.Lock()
	if e.failure.traceRelease != nil {
		e.failure.traceRelease()
		e.failure.traceRelease = nil
	}
	e.failure.mu.Unlock()
}

// describeWidget — « у *x.Y (Panel > Y)» для журнала; пусто без виджета.
func describeWidget(info PanicInfo) string {
	if info.Widget == nil {
		return ""
	}
	return fmt.Sprintf(" у %s (%s)", reflect.TypeOf(info.Widget).String(), info.Path)
}
