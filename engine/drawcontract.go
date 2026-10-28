package engine

// drawcontract.go — проверка контракта отрисовки.
//
// Контракт (GUIDE, «Контракт отрисовки»): Draw только рисует. Он не меняет
// состояние виджета, а значит, не зовёт Invalidate, InvalidateRect, SetBounds,
// AddChild, RemoveChild и не заводит анимаций. Пропуск поддеревьев
// (SetSubtreeCulling) на этом стоит: виджет, который не задет изменением, не
// рисуется вовсе, и всё, что он делал бы в Draw, не происходит.
//
// Нарушение на экране выглядит как дыры и устаревшие куски внутри окон, и по
// картинке не понять, кто виноват. Проверка называет виновника: уведомление
// об изменении, пришедшее от виджетов посреди кадра на горутине, которая этот
// кадр рисует, — нарушение, а кто его сделал, говорит учёт рисующихся
// виджетов (widget.EnableDrawTrace).
//
// Уведомления от других горутин (фоновая загрузка, поток окна) нарушением не
// считаются: так и должно быть, движок соберёт их в следующий кадр. Свои
// инвалидации движок делает не через приёмники уведомлений и проверку не
// задевает.

import (
	"fmt"
	"image"
	"log"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/widget"
)

// drawContract — состояние проверки у движка.
type drawContract struct {
	on atomic.Bool
	// drawGID — горутина, рисующая кадр прямо сейчас; 0 — кадр не рисуется.
	drawGID atomic.Uint64

	mu sync.Mutex
	// release выключает учёт рисующихся виджетов, включённый проверкой.
	release func()
	// seen — о каких виджетах уже сказано: каждый называется один раз за
	// жизнь движка, иначе журнал утонул бы в одной и той же строке на кадр.
	seen map[any]struct{}
	// logf — куда писать; тесты подменяют.
	logf func(format string, args ...any)
}

// SetDrawContractCheck включает проверку контракта отрисовки: о каждом
// виджете, который из своего Draw зовёт Invalidate, InvalidateRect,
// SetBounds, AddChild, RemoveChild или заводит анимацию, движок один раз
// пишет в журнал (log) его тип и путь от корня.
//
// Для отладки: включённая проверка замедляет кадр (учёт рисующихся виджетов
// берёт замок на каждый виджет). Выключенная стоит одной атомарной проверки
// на уведомление и на ребёнка контейнера.
//
// Типичное применение — найти, почему пропуск поддеревьев даёт дыры:
//
//	eng.SetSubtreeCulling(true)
//	eng.SetDrawContractCheck(true)
//	// поработать с интерфейсом и прочитать журнал
func (e *Engine) SetDrawContractCheck(on bool) {
	c := &e.contract
	c.mu.Lock()
	defer c.mu.Unlock()
	if on == c.on.Load() {
		return
	}
	if on {
		c.release = widget.EnableDrawTrace()
		c.on.Store(true)
		return
	}
	c.on.Store(false)
	if c.release != nil {
		c.release()
		c.release = nil
	}
}

// DrawContractCheck сообщает, включена ли проверка контракта отрисовки.
func (e *Engine) DrawContractCheck() bool { return e.contract.on.Load() }

// beginDrawContract отмечает начало отрисовки кадра на текущей горутине.
func (e *Engine) beginDrawContract() {
	if e.contract.on.Load() {
		e.contract.drawGID.Store(curGoroutineID())
	}
}

// endDrawContract отмечает конец отрисовки. Безусловно: проверку могли
// выключить посреди кадра.
func (e *Engine) endDrawContract() { e.contract.drawGID.Store(0) }

// Приёмники уведомлений виджетов (RegisterUINotifierWake): проверка, затем
// обычная работа.

func (e *Engine) onWidgetInvalidate() {
	e.checkDrawContract("Invalidate", image.Rectangle{})
	e.Invalidate()
}

func (e *Engine) onWidgetInvalidateRect(r image.Rectangle) {
	e.checkDrawContract("Invalidate/SetBounds/AddChild", r)
	e.InvalidateRect(r)
}

// onWidgetInvalidateRectOwned — точечная инвалидация от виджета, который уже
// пометил себя устаревшим (widget/retain.go): без ничейного повреждения.
func (e *Engine) onWidgetInvalidateRectOwned(r image.Rectangle) {
	e.checkDrawContract("Invalidate/SetBounds/AddChild", r)
	e.damageRect(r)
}

// onWidgetAnimation — приёмник «заведена анимация». Старт анимации из Draw
// нарушением НЕ считается: правило 2 контракта запрещает анимацию счётчиком
// внутри Draw, а не запуск widget.Animate оттуда. Анимация, заведённая в
// ответ на изменение, которое и вызвало этот Draw (переход цвета при смене
// наведения), или лениво при первом показе (тик часов), тикает независимо от
// отрисовки и сама заявляет свою область; виджет, который не рисуют, в
// анимации не нуждается. Решение этапа 0 работ к 4.0.
func (e *Engine) onWidgetAnimation() {}

// checkDrawContract записывает нарушение, если уведомление пришло из Draw.
func (e *Engine) checkDrawContract(what string, r image.Rectangle) {
	c := &e.contract
	if !c.on.Load() {
		return
	}
	gid := c.drawGID.Load()
	if gid == 0 || gid != curGoroutineID() {
		return
	}
	path := widget.DrawTracePath()
	key, who, trail := describeDrawPath(path)
	c.mu.Lock()
	if c.seen == nil {
		c.seen = make(map[any]struct{})
	}
	if _, ok := c.seen[key]; ok {
		c.mu.Unlock()
		return
	}
	c.seen[key] = struct{}{}
	logf := c.logf
	c.mu.Unlock()
	if logf == nil {
		logf = log.Printf
	}
	area := ""
	if !r.Empty() {
		area = fmt.Sprintf(", область %v", r)
	}
	logf("engine: контракт Draw нарушен: %s из Draw у %s (%s%s)", what, who, trail, area)
}

// describeDrawPath — ключ виджета для «уже сказано», его тип и путь от корня
// вида «Panel > Window > Label». Ключ — адрес виджета, а у виджета-значения
// (не указателя) — его путь: значение нельзя класть ключом в карту, если в нём
// есть срезы.
func describeDrawPath(path []widget.Widget) (key any, who, trail string) {
	if len(path) == 0 {
		return "?", "неизвестного виджета", "вне отслеживаемого обхода"
	}
	names := make([]string, len(path))
	for i, w := range path {
		names[i] = widgetTypeName(w)
	}
	trail = strings.Join(names, " > ")
	last := path[len(path)-1]
	who = reflect.TypeOf(last).String()
	if v := reflect.ValueOf(last); v.Kind() == reflect.Pointer {
		return v.Pointer(), who, trail
	}
	return trail, who, trail
}

// widgetTypeName — короткое имя типа: «Label», а не «*widget.Label».
func widgetTypeName(w widget.Widget) string {
	t := reflect.TypeOf(w)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if n := t.Name(); n != "" {
		return n
	}
	return t.String()
}
