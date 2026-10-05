// dnd.go — состояние «Не беспокоить».
//
// Что значит «не беспокоить» — не показывать тосты, глушить звук, молчать
// на время фокусировки — решает потребитель; оболочке нужно знать одно:
// включено ли, и чтобы её об этом уведомляли. Состояние приходит через
// интерфейс, как список уведомлений: центр Windows 11 рисует по нему
// колокольчик в заголовке, тост молчит, а кнопка центра в трее (панель задач)
// рисует перечёркнутый колокольчик. Все они подписаны на ОДНУ модель.
package desktop

import "sync"

// DoNotDisturb — режим «Не беспокоить», который ведёт потребитель.
//
// SetEnabled вызывает оболочка, когда пользователь нажал колокольчик; что
// делать с режимом, решает потребитель, а новое состояние он сообщает обычным
// путём — через Subscribe (замыкание зовётся из любой горутины). Модель,
// которая просто запоминает состояние, — DoNotDisturbState.
type DoNotDisturb interface {
	Enabled() bool
	SetEnabled(bool)
	Subscribe(func()) (unsubscribe func())
}

// DoNotDisturbState — режим «Не беспокоить» в памяти: готовая модель для
// потребителя, у которого своей нет, и для тестов. Безопасна для вызова из
// нескольких горутин.
type DoNotDisturbState struct {
	mu      sync.Mutex
	on      bool
	subs    map[int]func()
	nextSub int

	// OnChange — что сделать потребителю при смене режима (выключить звук,
	// записать в настройки). Зовётся после уведомления подписчиков, вне замка.
	OnChange func(enabled bool)
}

// NewDoNotDisturb создаёт модель в заданном начальном состоянии.
func NewDoNotDisturb(enabled bool) *DoNotDisturbState {
	return &DoNotDisturbState{on: enabled, subs: map[int]func(){}}
}

var _ DoNotDisturb = (*DoNotDisturbState)(nil)

// Enabled сообщает, включён ли режим.
func (d *DoNotDisturbState) Enabled() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.on
}

// SetEnabled включает или выключает режим. То же состояние — не событие.
func (d *DoNotDisturbState) SetEnabled(on bool) {
	d.mu.Lock()
	if d.on == on {
		d.mu.Unlock()
		return
	}
	d.on = on
	hook := d.OnChange
	subs := make([]func(), 0, len(d.subs))
	for _, fn := range d.subs {
		subs = append(subs, fn)
	}
	d.mu.Unlock()
	for _, fn := range subs {
		fn()
	}
	if hook != nil {
		hook(on)
	}
}

// Subscribe подписывает на смену режима и возвращает функцию отписки.
func (d *DoNotDisturbState) Subscribe(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	d.mu.Lock()
	d.nextSub++
	id := d.nextSub
	d.subs[id] = fn
	d.mu.Unlock()
	return func() {
		d.mu.Lock()
		delete(d.subs, id)
		d.mu.Unlock()
	}
}
