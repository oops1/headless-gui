// quickactions.go — модель быстрых действий центра уведомлений.
//
// Плитки «Wi-Fi», «Bluetooth», «Ночной свет», «В самолёте» центр Windows 10
// рисует сам, а вот что они значат и включены ли, знает только потребитель:
// состав и состояние приходят через интерфейс, как список уведомлений.
// Компонент не знает ни про rfkill, ни про BlueZ.
package desktop

import (
	"image"
	"sync"
)

// QuickActionID — идентификатор быстрого действия в модели потребителя.
type QuickActionID string

// QuickAction — одна плитка быстрых действий.
type QuickAction struct {
	ID    QuickActionID
	Title string
	// Icon — значок плитки; IconAt, если задан, главнее и получает сторону в
	// ФИЗИЧЕСКИХ пикселях (значок 16 логических на экране 150 % приходит как
	// 24, а не растягивается из растра 16). Нет ни того ни другого — плитка
	// без значка, с одной подписью.
	Icon   image.Image
	IconAt func(size int) image.Image
	// On — действие включено: плитка заливается акцентом.
	On bool
	// Disabled — действие недоступно на этой системе (нет радиомодуля, нет
	// прав): плитка приглушена и не нажимается. Честное «недоступно» лучше
	// плитки, которая включается и ничего не делает.
	Disabled bool
}

// IconFor возвращает значок для квадрата со стороной size (физические пиксели).
func (a QuickAction) IconFor(size int) image.Image {
	if a.IconAt != nil && size > 0 {
		if img := a.IconAt(size); img != nil {
			return img
		}
	}
	return a.Icon
}

// QuickActionModel — быстрые действия центра уведомлений.
//
// Subscribe возвращает функцию отписки, замыкание зовётся из горутины
// потребителя (см. раздел «Из какой горутины что зовётся» в описании пакета).
// Менять одну плитку — менять её в списке и вызвать подписчиков: центр сам
// сравнит состояние с нарисованным и перерисует только изменившиеся плитки,
// пока набор и порядок плиток прежние.
type QuickActionModel interface {
	List() []QuickAction
	// Toggle — пользователь нажал плитку. Что делать (включить Bluetooth,
	// открыть настройки сети), решает потребитель; новое состояние он
	// сообщает обычным путём — через Subscribe.
	Toggle(QuickActionID)
	Subscribe(func()) func()
}

// QuickActionList — быстрые действия в памяти: готовая модель для потребителя,
// у которого своей нет, и для тестов. Безопасна для вызова из нескольких
// горутин.
type QuickActionList struct {
	mu      sync.Mutex
	list    []QuickAction
	subs    map[int]func()
	nextSub int

	// OnToggle — что делать при нажатии плитки. nil — плитка переключает
	// своё On сама (так ведёт себя заглушка без системы за спиной).
	OnToggle func(QuickActionID)

	// Toggled — журнал нажатий для тестов.
	Toggled []QuickActionID
}

// NewQuickActionList создаёт модель с заданными плитками.
func NewQuickActionList(actions ...QuickAction) *QuickActionList {
	return &QuickActionList{list: append([]QuickAction(nil), actions...), subs: map[int]func(){}}
}

// List возвращает копию списка.
func (q *QuickActionList) List() []QuickAction {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]QuickAction(nil), q.list...)
}

// Replace заменяет весь набор и уведомляет подписчиков.
func (q *QuickActionList) Replace(actions []QuickAction) {
	q.mu.Lock()
	q.list = append([]QuickAction(nil), actions...)
	q.mu.Unlock()
	q.notify()
}

// Set заменяет плитку с тем же ID (нет такой — добавляет в конец) и
// уведомляет подписчиков.
func (q *QuickActionList) Set(a QuickAction) {
	q.mu.Lock()
	found := false
	for i := range q.list {
		if q.list[i].ID == a.ID {
			q.list[i], found = a, true
			break
		}
	}
	if !found {
		q.list = append(q.list, a)
	}
	q.mu.Unlock()
	q.notify()
}

// SetOn включает или выключает плитку id. Неизвестный id и то же состояние —
// не событие.
func (q *QuickActionList) SetOn(id QuickActionID, on bool) {
	q.mu.Lock()
	changed := false
	for i := range q.list {
		if q.list[i].ID == id && q.list[i].On != on {
			q.list[i].On, changed = on, true
			break
		}
	}
	q.mu.Unlock()
	if changed {
		q.notify()
	}
}

// Toggle записывает нажатие в журнал и либо зовёт OnToggle, либо
// переключает плитку сама. Недоступная плитка не нажимается.
func (q *QuickActionList) Toggle(id QuickActionID) {
	q.mu.Lock()
	var cur *QuickAction
	for i := range q.list {
		if q.list[i].ID == id {
			cur = &q.list[i]
			break
		}
	}
	if cur == nil || cur.Disabled {
		q.mu.Unlock()
		return
	}
	q.Toggled = append(q.Toggled, id)
	hook := q.OnToggle
	if hook == nil {
		cur.On = !cur.On
	}
	q.mu.Unlock()
	if hook != nil {
		hook(id)
		return
	}
	q.notify()
}

// Subscribe подписывает на изменения набора и состояния.
func (q *QuickActionList) Subscribe(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	q.mu.Lock()
	q.nextSub++
	id := q.nextSub
	q.subs[id] = fn
	q.mu.Unlock()
	return func() {
		q.mu.Lock()
		delete(q.subs, id)
		q.mu.Unlock()
	}
}

func (q *QuickActionList) notify() {
	q.mu.Lock()
	list := make([]func(), 0, len(q.subs))
	for _, fn := range q.subs {
		list = append(list, fn)
	}
	q.mu.Unlock()
	for _, fn := range list {
		fn()
	}
}
