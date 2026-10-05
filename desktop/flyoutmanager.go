// flyoutmanager.go — менеджер всплывающих панелей.
//
// Панели рабочего стола — «Пуск», календарь, центр уведомлений, быстрые
// настройки, область скрытых значков трея — устроены одинаково (Flyout), но
// каждая жила сама по себе. Оболочке приходилось вручную закрывать остальные
// перед открытием одной, разносить Esc и клики мимо и следить, чтобы панель
// лежала в дереве в нужном месте. Менеджер берёт это на себя.
//
// Он делает пять вещей:
//
//   - регистрация: панель получает имя; потребитель вправе зарегистрировать
//     и СВОЮ — любой тип, встраивающий *Flyout;
//   - «открыл одну — остальные закрылись»: исключение — панели одной группы
//     (FlyoutGroup, календарь вместе с уведомлениями): они открываются вместе;
//   - единый слой: менеджер — один виджет дерева, в который сложены все панели.
//     Положенный в корень после панели задач, он рисует их оверлеи над панелью
//     задач; подсказки движок рисует после всех оверлеев, поэтому они всегда
//     сверху, а контекстные меню лежат выше, если их виджеты стоят в дереве
//     после менеджера;
//   - закрытие по клику вне и по Esc — движок зовёт их у панелей (DismissAt и
//     widget.EscapeDismisser), менеджеру достаточно, что панели в его слое;
//   - события Opened и Closed: оболочка подсвечивает кнопку «Пуск», пока
//     открыто меню, и гасит, когда оно закрыто чем угодно — кликом мимо, Esc,
//     запуском приложения, открытием другой панели.
package desktop

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/widget"
)

// FlyoutPanel — то, что менеджер принимает на регистрацию: виджет, внутри
// которого лежит Flyout. Таковы *Flyout и любой тип, встраивающий *Flyout
// (StartMenu, NotificationCenter, панель потребителя).
//
// В дерево менеджер кладёт сам виджет, а не его Flyout: обработчики мыши и
// клавиш, а с ними и содержимое панели, принадлежат объемлющему типу.
type FlyoutPanel interface {
	widget.Widget
	// AsFlyout возвращает встроенную основу панели.
	AsFlyout() *Flyout
}

// AsFlyout возвращает саму панель: так *Flyout, а вместе с ним и любой тип,
// встраивающий *Flyout, удовлетворяет FlyoutPanel.
func (f *Flyout) AsFlyout() *Flyout { return f }

// FlyoutEventKind — что произошло с панелью.
type FlyoutEventKind int

const (
	// FlyoutOpened — панель открылась.
	FlyoutOpened FlyoutEventKind = iota
	// FlyoutClosed — панель закрылась (чем бы ни было закрыта).
	FlyoutClosed
)

// FlyoutEvent — событие менеджера панелей.
type FlyoutEvent struct {
	// Name — имя панели при регистрации.
	Name string
	Kind FlyoutEventKind
	// Panel — виджет панели, как его зарегистрировали.
	Panel FlyoutPanel
}

// managedFlyout — зарегистрированная панель.
type managedFlyout struct {
	name    string
	panel   FlyoutPanel
	fl      *Flyout
	unwatch func()
}

// FlyoutManager — слой всплывающих панелей рабочего стола. Нулевое значение не
// годится: создавай через NewFlyoutManager.
//
// Менеджер — виджет: положи его в корень дерева ПОСЛЕ панели задач и НЕ клади
// зарегистрированные панели в дерево отдельно — менеджер уже держит их детьми.
type FlyoutManager struct {
	widget.Base

	mu      sync.Mutex
	entries []*managedFlyout
	subs    map[int]func(FlyoutEvent)
	nextSub int
}

// NewFlyoutManager создаёт пустой менеджер.
func NewFlyoutManager() *FlyoutManager {
	return &FlyoutManager{subs: map[int]func(FlyoutEvent){}}
}

// Register добавляет панель под именем name и возвращает функцию, снимающую
// её с учёта. Панель с тем же именем заменяется: прежняя снимается.
//
// Открытые к моменту регистрации панели остаются открытыми; закрывать
// остальные менеджер начнёт со следующего открытия.
func (m *FlyoutManager) Register(name string, p FlyoutPanel) (unregister func()) {
	if p == nil || p.AsFlyout() == nil {
		return func() {}
	}
	m.Unregister(name)

	e := &managedFlyout{name: name, panel: p, fl: p.AsFlyout()}
	e.unwatch = e.fl.Subscribe(func(open bool) { m.onChanged(e, open) })

	m.mu.Lock()
	m.entries = append(m.entries, e)
	m.mu.Unlock()
	m.AddChild(p)

	return func() { m.unregisterEntry(e) }
}

// Unregister снимает панель name с учёта. Панель не закрывается и остаётся
// жить у владельца, но из слоя менеджера уходит.
func (m *FlyoutManager) Unregister(name string) {
	if e := m.find(name); e != nil {
		m.unregisterEntry(e)
	}
}

func (m *FlyoutManager) unregisterEntry(e *managedFlyout) {
	m.mu.Lock()
	idx := -1
	for i, x := range m.entries {
		if x == e {
			idx = i
			break
		}
	}
	if idx < 0 {
		m.mu.Unlock()
		return
	}
	m.entries = append(m.entries[:idx], m.entries[idx+1:]...)
	m.mu.Unlock()
	e.unwatch()
	m.RemoveChild(e.panel)
	m.Invalidate()
}

func (m *FlyoutManager) find(name string) *managedFlyout {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.name == name {
			return e
		}
	}
	return nil
}

func (m *FlyoutManager) snapshot() []*managedFlyout {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*managedFlyout(nil), m.entries...)
}

// Panel возвращает зарегистрированную панель по имени (nil, если такой нет).
func (m *FlyoutManager) Panel(name string) FlyoutPanel {
	if e := m.find(name); e != nil {
		return e.panel
	}
	return nil
}

// Names возвращает имена зарегистрированных панелей в порядке регистрации.
func (m *FlyoutManager) Names() []string {
	es := m.snapshot()
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.name
	}
	return out
}

// IsOpen сообщает, открыта ли панель name.
func (m *FlyoutManager) IsOpen(name string) bool {
	e := m.find(name)
	return e != nil && e.fl.IsOpen()
}

// Opened возвращает имена открытых панелей. Обычно одна; больше — только у
// панелей одной группы.
func (m *FlyoutManager) Opened() []string {
	var out []string
	for _, e := range m.snapshot() {
		if e.fl.IsOpen() {
			out = append(out, e.name)
		}
	}
	return out
}

// Open открывает панель name от якоря anchor; остальные открытые панели (кроме
// входящих в её группу) закрываются. Возвращает false, если такой нет.
func (m *FlyoutManager) Open(name string, anchor image.Rectangle) bool {
	e := m.find(name)
	if e == nil {
		return false
	}
	// Открывает объемлющий тип, если он переопределил Open (StartMenu сбрасывает
	// выделение): через интерфейс, а не через встроенный Flyout.
	if o, ok := e.panel.(interface{ Open(image.Rectangle) }); ok {
		o.Open(anchor)
	} else {
		e.fl.Open(anchor)
	}
	return true
}

// Close закрывает панель name. Возвращает false, если такой нет.
func (m *FlyoutManager) Close(name string) bool {
	e := m.find(name)
	if e == nil {
		return false
	}
	closePanel(e)
	return true
}

// Toggle открывает закрытую панель и закрывает открытую — то, что делает
// повторный клик по её кнопке. Возвращает, открыта ли панель после вызова.
//
// Клик, которым панель только что закрыло нажатие мимо неё (и пришёлся на её
// же кнопку), панель заново не открывает — см. Flyout.Toggle.
func (m *FlyoutManager) Toggle(name string, anchor image.Rectangle) bool {
	e := m.find(name)
	if e == nil {
		return false
	}
	if e.fl.IsOpen() {
		closePanel(e)
		return false
	}
	if e.fl.consumeAnchorDismiss() {
		return false
	}
	m.Open(name, anchor)
	return e.fl.IsOpen()
}

// CloseAll закрывает все открытые панели.
func (m *FlyoutManager) CloseAll() {
	for _, e := range m.snapshot() {
		closePanel(e)
	}
}

// closePanel закрывает панель так, как это делает её тип: у NotificationCenter
// и QuickSettings Close освобождает подписки.
func closePanel(e *managedFlyout) {
	if c, ok := e.panel.(interface{ Close() }); ok {
		c.Close()
		return
	}
	e.fl.Close()
}

// Subscribe подписывает h на события панелей и возвращает функцию отписки.
// h вызывается в той горутине, что открыла или закрыла панель, вне замков
// менеджера.
func (m *FlyoutManager) Subscribe(h func(FlyoutEvent)) (unsubscribe func()) {
	if h == nil {
		return func() {}
	}
	m.mu.Lock()
	m.nextSub++
	id := m.nextSub
	m.subs[id] = h
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.subs, id)
		m.mu.Unlock()
	}
}

// onChanged — панель открылась или закрылась (чем бы ни было вызвано).
func (m *FlyoutManager) onChanged(e *managedFlyout, open bool) {
	if open {
		// Остальные закрываются ДО того, как оболочка узнает об открытии этой
		// (OnOpen панели идёт после наблюдателей): кнопки гаснут раньше, чем
		// зажигается новая.
		for _, other := range m.snapshot() {
			if other == e || !other.fl.IsOpen() || sameFlyoutGroup(e.fl, other.fl) {
				continue
			}
			closePanel(other)
		}
	}

	kind := FlyoutClosed
	if open {
		kind = FlyoutOpened
	}
	m.mu.Lock()
	hs := make([]func(FlyoutEvent), 0, len(m.subs))
	for _, h := range m.subs {
		hs = append(hs, h)
	}
	m.mu.Unlock()
	ev := FlyoutEvent{Name: e.name, Kind: kind, Panel: e.panel}
	for _, h := range hs {
		h(ev)
	}
}

// sameFlyoutGroup — панели входят в одну группу (и потому открываются вместе).
func sameFlyoutGroup(a, b *Flyout) bool {
	return a.group != nil && a.group == b.group
}

// Group собирает зарегистрированные панели с указанными именами в группу
// (см. FlyoutGroup): они считают площади друг друга своими и открываются
// вместе. Незнакомые имена пропускаются.
func (m *FlyoutManager) Group(names ...string) *FlyoutGroup {
	g := &FlyoutGroup{}
	for _, n := range names {
		if e := m.find(n); e != nil {
			e.fl.SetGroup(g)
		}
	}
	return g
}

// Bounds — объединение границ открытых панелей. Движок ведёт события к детям
// только внутри границ родителя, а у самого менеджера геометрии нет: вся она
// в панелях.
func (m *FlyoutManager) Bounds() image.Rectangle {
	r := m.Base.Bounds()
	for _, e := range m.snapshot() {
		if b := e.panel.Bounds(); !b.Empty() {
			r = r.Union(b)
		}
	}
	return r
}

// Draw рисует детей: у самих панелей в потоке виджетов рисовать нечего,
// содержимое уходит в оверлеи.
func (m *FlyoutManager) Draw(ctx widget.DrawContext) { m.DrawChildren(ctx) }
