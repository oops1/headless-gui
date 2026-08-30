// flyoutgroup.go — панели, которые ведут себя как одна.
//
// Windows показывает уведомления НАД календарём: две панели, стоящие одна над
// другой и открытые вместе. Для каждой из них площадь соседки — «мимо», и без
// общего понятия группы клик по числу календаря считался бы для центра
// уведомлений поводом закрыться.
//
// Группа снимает это: панели одной группы считают прямоугольники друг друга
// своими и закрываются вместе.
package desktop

import (
	"image"
	"sync"
)

// FlyoutGroup — набор всплывающих панелей, ведущих себя как одна.
//
// Пустая группа осмысленна: панель можно включить в неё до того, как в группе
// появится вторая. Нулевой указатель тоже — методы на нём безопасны, поэтому
// панель без группы не требует проверок на месте вызова.
type FlyoutGroup struct {
	mu      sync.Mutex
	members []*Flyout
}

// NewFlyoutGroup собирает группу из перечисленных панелей.
func NewFlyoutGroup(members ...*Flyout) *FlyoutGroup {
	g := &FlyoutGroup{}
	for _, m := range members {
		m.SetGroup(g)
	}
	return g
}

func (g *FlyoutGroup) add(f *Flyout) {
	if g == nil || f == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, m := range g.members {
		if m == f {
			return
		}
	}
	g.members = append(g.members, f)
}

func (g *FlyoutGroup) remove(f *Flyout) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, m := range g.members {
		if m == f {
			g.members = append(g.members[:i], g.members[i+1:]...)
			return
		}
	}
}

// covers сообщает, лежит ли точка в открытой панели группы.
func (g *FlyoutGroup) covers(pt image.Point) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	members := make([]*Flyout, len(g.members))
	copy(members, g.members)
	g.mu.Unlock()

	// Снимок списка, а потом опрос ВНЕ замка: rect() читает тему и метрики, и
	// держать на это время замок группы незачем.
	for _, m := range members {
		if m.IsOpen() && pt.In(m.rect()) {
			return true
		}
	}
	return false
}

// CloseAll закрывает все панели группы. Оболочка зовёт это, когда группу
// нужно убрать целиком — например, по Esc или при смене рабочего стола.
func (g *FlyoutGroup) CloseAll() {
	if g == nil {
		return
	}
	g.mu.Lock()
	members := make([]*Flyout, len(g.members))
	copy(members, g.members)
	g.mu.Unlock()

	for _, m := range members {
		m.Close()
	}
}

// Open открывает панели группы вместе, каждую от своего значка.
//
// Принимает не список, а функцию привязки: у панелей группы значок обычно
// один и тот же (часы), но выравнивание и отступ у каждой свои, и задавать их
// извне неудобно.
func (g *FlyoutGroup) OpenAll(anchor image.Rectangle) {
	if g == nil {
		return
	}
	g.mu.Lock()
	members := make([]*Flyout, len(g.members))
	copy(members, g.members)
	g.mu.Unlock()

	for _, m := range members {
		m.Open(anchor)
	}
}
