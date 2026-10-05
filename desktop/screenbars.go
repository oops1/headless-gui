// screenbars.go — по панели задач на каждый экран.
//
// Рабочий стол с несколькими мониторами держит панель на каждом: у каждой свои
// кнопки, свои всплывающие панели и свой край. ScreenBars читает набор
// мониторов у потребителя (Screens), просит у него оболочку монитора (панель и
// её всплывающие панели) для каждого нового экрана, ставит панели к краям
// своих мониторов (Taskbar.DockTo) и убирает оболочки пропавших экранов.
//
// Всплывающие панели всех мониторов лежат в одном слое (FlyoutManager) и
// открываются по одной на весь рабочий стол: открыл «Пуск» на втором мониторе —
// календарь на первом закрылся, как в системе.
package desktop

import (
	"image"
	"sort"
	"sync"

	"github.com/oops1/headless-gui/v3/widget"
)

// MonitorShell — оболочка одного монитора: панель задач и её всплывающие
// панели (меню «Пуск», календарь, центр уведомлений…).
type MonitorShell struct {
	// Bar — панель задач монитора (уже с элементами). Обязательна.
	Bar *Taskbar
	// Flyouts — всплывающие панели монитора по именам (имя уникально в пределах
	// монитора). ScreenBars привязывает их к краю и монитору панели
	// (Taskbar.BindFlyouts) и кладёт в общий слой.
	Flyouts map[string]FlyoutPanel
}

// ShellFactory создаёт оболочку монитора m. Зовётся один раз на экран, когда
// он появляется. Вернувшая nil фабрика оставляет экран без панели.
type ShellFactory func(m Monitor) *MonitorShell

// ScreenBars — набор панелей задач по одной на экран. Нулевое значение не
// годится: создавай через NewScreenBars.
//
// Это виджет: положи его в корень дерева ПОСЛЕ окон, вместо отдельных панелей и
// FlyoutManager — панели и слой всплывающих панелей он держит сам.
type ScreenBars struct {
	widget.Base

	screens Screens
	factory ShellFactory
	layer   *FlyoutManager

	// Post, если задан, переносит обновление набора экранов в поток интерфейса
	// (например, Engine.Post): подписка Screens зовётся из горутины потребителя,
	// а менять дерево виджетов оттуда нельзя. Без Post обновление идёт на месте.
	Post func(func())

	// OnShellRemoved зовётся, когда экран пропал, до того как оболочка закрыта:
	// потребитель отпускает свои ресурсы (подписки, окна).
	OnShellRemoved func(id string, sh *MonitorShell)

	mu     sync.Mutex
	shells []*screenShell
	unsub  func()
}

type screenShell struct {
	id      string
	shell   *MonitorShell
	unreg   []func()
	flyouts []FlyoutPanel
}

// NewScreenBars создаёт набор и сразу читает мониторы. Подписывается на
// изменения Screens; отписка — в Close.
func NewScreenBars(screens Screens, factory ShellFactory) *ScreenBars {
	s := &ScreenBars{screens: screens, factory: factory, layer: NewFlyoutManager()}
	s.AddChild(s.layer)
	s.Sync()
	if screens != nil {
		s.unsub = screens.Subscribe(func() {
			if s.Post != nil {
				s.Post(s.Sync)
				return
			}
			s.Sync()
		})
	}
	return s
}

// Sync перечитывает мониторы: новым создаёт оболочки, у существующих
// переставляет панель по новой геометрии, у пропавших закрывает оболочки.
// Компоненты существующих экранов не пересоздаются.
func (s *ScreenBars) Sync() {
	var ms []Monitor
	if s.screens != nil {
		ms = s.screens.Monitors()
	}

	s.mu.Lock()
	old := s.shells
	s.mu.Unlock()

	var next []*screenShell
	for _, m := range ms {
		var ss *screenShell
		for _, o := range old {
			if o.id == m.ID {
				ss = o
				break
			}
		}
		if ss == nil {
			ss = s.createShell(m)
			if ss == nil {
				continue
			}
		}
		ss.shell.Bar.DockTo(m)
		next = append(next, ss)
	}

	for _, o := range old {
		keep := false
		for _, n := range next {
			if n == o {
				keep = true
				break
			}
		}
		if !keep {
			s.removeShell(o)
		}
	}

	s.mu.Lock()
	s.shells = next
	s.mu.Unlock()
	s.Invalidate()
}

func (s *ScreenBars) createShell(m Monitor) *screenShell {
	if s.factory == nil {
		return nil
	}
	sh := s.factory(m)
	if sh == nil || sh.Bar == nil {
		return nil
	}
	ss := &screenShell{id: m.ID, shell: sh}
	// Порядок регистрации — по имени: он же порядок отрисовки слоя.
	for _, name := range sortedNames(sh.Flyouts) {
		p := sh.Flyouts[name]
		if p == nil || p.AsFlyout() == nil {
			continue
		}
		ss.flyouts = append(ss.flyouts, p)
		ss.unreg = append(ss.unreg, s.layer.Register(flyoutName(m.ID, name), p))
	}
	sh.Bar.BindFlyouts(ss.flyouts...)
	// Панель в дерево раньше слоя, чтобы слой рисовался над ней.
	s.RemoveChild(s.layer)
	s.AddChild(sh.Bar)
	s.AddChild(s.layer)
	return ss
}

func (s *ScreenBars) removeShell(ss *screenShell) {
	if s.OnShellRemoved != nil {
		s.OnShellRemoved(ss.id, ss.shell)
	}
	for _, un := range ss.unreg {
		un()
	}
	for _, p := range ss.flyouts {
		closePanel(&managedFlyout{panel: p, fl: p.AsFlyout()})
	}
	ss.shell.Bar.UnbindFlyouts(ss.flyouts...)
	s.RemoveChild(ss.shell.Bar)
	ss.shell.Bar.Close()
}

// Bars возвращает панели в порядке мониторов.
func (s *ScreenBars) Bars() []*Taskbar {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Taskbar, len(s.shells))
	for i, ss := range s.shells {
		out[i] = ss.shell.Bar
	}
	return out
}

// Shell возвращает оболочку монитора id (nil, если такого экрана нет).
func (s *ScreenBars) Shell(id string) *MonitorShell {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ss := range s.shells {
		if ss.id == id {
			return ss.shell
		}
	}
	return nil
}

// BarAt возвращает панель того монитора, на котором лежит точка pt (nil, если
// точка вне всех мониторов). Нужна оболочке, чтобы по горячей клавише открыть
// «Пуск» на экране под курсором.
func (s *ScreenBars) BarAt(pt image.Point) *Taskbar {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ss := range s.shells {
		if m, ok := ss.shell.Bar.Monitor(); ok && pt.In(m.Bounds) {
			return ss.shell.Bar
		}
	}
	return nil
}

// Flyouts возвращает общий слой всплывающих панелей всех мониторов. Имена в
// нём — flyoutName(monitorID, name); пользоваться удобнее методами Open, Toggle
// и Flyout.
func (s *ScreenBars) Flyouts() *FlyoutManager { return s.layer }

// Flyout возвращает всплывающую панель name монитора id (nil, если нет).
func (s *ScreenBars) Flyout(id, name string) FlyoutPanel {
	return s.layer.Panel(flyoutName(id, name))
}

// Toggle открывает или закрывает панель name монитора id от якоря anchor; на
// остальных мониторах открытые панели закрываются.
func (s *ScreenBars) Toggle(id, name string, anchor image.Rectangle) bool {
	return s.layer.Toggle(flyoutName(id, name), anchor)
}

// Bounds — объединение границ панелей и открытых всплывающих панелей: движок
// ведёт события к детям только внутри границ родителя.
func (s *ScreenBars) Bounds() image.Rectangle {
	r := s.Base.Bounds()
	for _, c := range s.Base.Children() {
		if b := c.Bounds(); !b.Empty() {
			r = r.Union(b)
		}
	}
	return r
}

// Draw рисует панели и слой всплывающих панелей.
func (s *ScreenBars) Draw(ctx widget.DrawContext) { s.DrawChildren(ctx) }

// Close отписывается от Screens и закрывает все оболочки.
func (s *ScreenBars) Close() {
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
	s.mu.Lock()
	shells := s.shells
	s.shells = nil
	s.mu.Unlock()
	for _, ss := range shells {
		s.removeShell(ss)
	}
}

func flyoutName(monitorID, name string) string { return monitorID + "/" + name }

// sortedNames — ключи карты по возрастанию: порядок, не зависящий от случая.
func sortedNames(m map[string]FlyoutPanel) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
