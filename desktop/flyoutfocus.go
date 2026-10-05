package desktop

import "github.com/oops1/headless-gui/v3/widget"

// FocusRequester — то, у чего панель просит фокус; его реализует
// *engine.Engine (методы RequestFocus и ReturnFocus).
//
// Интерфейс, а не движок, чтобы desktop не зависел от engine и панель
// проверялась на подставном исполнителе.
type FocusRequester interface {
	RequestFocus(w widget.Widget)
	ReturnFocus(w widget.Widget)
}

// FocusOnOpen заставляет панель f брать фокус у движка, когда она открывается,
// и отдавать его прежнему владельцу, когда закрывается. target — виджет,
// который получает фокус (обычно сама панель, а для панели с полем ввода —
// это поле); nil означает саму панель, f.
//
// Зачем: панель просит фокус сама (focusreq), но только пока движок доставляет
// событие. Панель, открытая не событием — клавишей Win, пойманной оболочкой
// вне движка, таймером, сообщением из другой горутины, — фокус не получает, и
// стрелки с буквами уходят прежнему виджету. Через FocusRequester фокус
// берётся и отдаётся с любой горутины (Engine.RequestFocus ставит вызов в
// очередь кадра), а при открытии событием просьба повторяет собственную и
// ничего не меняет.
//
// Возвращает функцию, снимающую подписку.
func FocusOnOpen(f *Flyout, eng FocusRequester, target widget.Widget) (stop func()) {
	if f == nil || eng == nil {
		return func() {}
	}
	if target == nil {
		target = f
	}
	return f.Subscribe(func(open bool) {
		if open {
			eng.RequestFocus(target)
		} else {
			eng.ReturnFocus(target)
		}
	})
}

// FocusOnOpen делает то же для всех панелей менеджера: открытая панель берёт
// фокус, закрытая — отдаёт. Фокус получает сама панель (то, что зарегистрировано
// в менеджере), поэтому панели, которой нужно поле внутри себя, надёжнее
// подписать отдельно функцией FocusOnOpen.
func (m *FlyoutManager) FocusOnOpen(eng FocusRequester) (stop func()) {
	if eng == nil {
		return func() {}
	}
	return m.Subscribe(func(ev FlyoutEvent) {
		if ev.Panel == nil {
			return
		}
		if ev.Kind == FlyoutOpened {
			eng.RequestFocus(ev.Panel)
		} else {
			eng.ReturnFocus(ev.Panel)
		}
	})
}
