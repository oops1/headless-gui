// focus_items.go — клавиатурное поведение встроенных элементов панели задач:
// Focusable, TabIndex, клавиши и рамка фокуса. Само состояние живёт в
// встроенном FocusState (focus.go); здесь — то, что у каждого элемента своё:
// что значит «нажать» и какую область обводит рамка.
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Все элементы панели — widget.Focusable и принимают навигатор.
var (
	_ widget.Focusable = (*StartButton)(nil)
	_ widget.Focusable = (*ApplicationArea)(nil)
	_ widget.Focusable = (*RunningApplications)(nil)
	_ widget.Focusable = (*NetworkItem)(nil)
	_ widget.Focusable = (*VolumeItem)(nil)
	_ widget.Focusable = (*PowerItem)(nil)
	_ widget.Focusable = (*TrayLabel)(nil)
	_ widget.Focusable = (*ClockItem)(nil)

	_ FocusNavigable = (*StartButton)(nil)
	_ FocusNavigable = (*ApplicationArea)(nil)
	_ FocusNavigable = (*RunningApplications)(nil)
	_ FocusNavigable = (*SystemTray)(nil)
)

// styleNormal — стиль компонента в покое: по нему выбираются цвет и
// скругление рамки фокуса.
func styleNormal(tm *theme.Manager, component string) *theme.Style {
	return styleOf(tm, component, "", theme.StateNormal)
}

// click вызывает обработчик клика, если он есть.
func click(fn func()) func() {
	return func() {
		if fn != nil {
			fn()
		}
	}
}

// ─── «Пуск» ──────────────────────────────────────────────────────────────────

// SetFocused реализует widget.Focusable.
func (s *StartButton) SetFocused(v bool) {
	if s.FocusState.Set(v) {
		s.Invalidate()
	}
}

// TabIndex исключает кнопку из обхода, пока у неё нет места на панели.
func (s *StartButton) TabIndex() int { return focusTabIndex(s) }

// OnKeyEvent: Enter и Space нажимают кнопку, стрелки переносят фокус к
// соседям области.
func (s *StartButton) OnKeyEvent(e widget.KeyEvent) {
	s.HandleKey(s, e, click(s.OnClick), s.Invalidate)
}

// FocusRing реализует FocusRinger.
func (s *StartButton) FocusRing() (image.Rectangle, *theme.Style) {
	return s.Bounds(), styleNormal(s.tm, ComponentStartButton)
}

// ─── Области приложений ──────────────────────────────────────────────────────

// SetFocused реализует widget.Focusable.
func (a *ApplicationArea) SetFocused(v bool) {
	if a.FocusState.Set(v) {
		a.Invalidate()
	}
}

// TabIndex исключает область из обхода, пока в ней нет ячеек.
func (a *ApplicationArea) TabIndex() int {
	if a.cellCount() == 0 {
		return -1
	}
	return focusTabIndex(a)
}

// cellCount — сколько ячеек имеют прямоугольник на панели (до них можно
// дойти стрелками).
func (a *ApplicationArea) cellCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.rects)
}

// OnKeyEvent: стрелки, Home и End выбирают ячейку, Enter и Space нажимают её
// так же, как щелчок мышью.
func (a *ApplicationArea) OnKeyEvent(e widget.KeyEvent) {
	// Открытое меню команд забирает клавиши себе (стрелки, Enter, Esc).
	if a.menu.routeKey(e) {
		return
	}
	// Клавиша меню и Shift+F10 открывают меню команд выбранной ячейки — то же,
	// что правый щелчок мышью.
	if e.Pressed && !e.Repeat && (e.Code == widget.KeyMenu ||
		(e.Code == widget.KeyF10 && e.Mod&widget.ModShift != 0)) {
		a.ShowCommands(a.FocusState.Cell(a.cellCount()))
		return
	}
	a.HandleCellKey(e, a.cellCount(), a.activateCell, a.Invalidate)
}

// activateCell делает с ячейкой i то же, что щелчок по ней: запускает
// закреплённое незапущенное, сворачивает активное окно, активирует остальные,
// а у стопки окон переключает окна по кругу.
func (a *ApplicationArea) activateCell(i int) { a.activate(i, false) }

// FocusRing обводит выбранную ячейку, а не всю область.
func (a *ApplicationArea) FocusRing() (image.Rectangle, *theme.Style) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	c := a.FocusState.Cell(len(a.rects))
	if c < 0 {
		return image.Rectangle{}, nil
	}
	return a.rects[c], styleNormal(a.tm, ComponentTaskButton)
}

// ButtonFocus возвращает индекс ячейки с клавиатурным фокусом (-1 — фокуса
// нет). Презентерам и оболочке: например, чтобы показать предпросмотр окна у
// ячейки, до которой дошли стрелками.
func (a *ApplicationArea) ButtonFocus() int {
	if !a.FocusVisible() {
		return -1
	}
	return a.FocusState.Cell(a.cellCount())
}

// SetFocused реализует widget.Focusable.
func (r *RunningApplications) SetFocused(v bool) {
	if r.FocusState.Set(v) {
		r.Invalidate()
	}
}

// TabIndex исключает область из обхода, пока в ней нет кнопок.
func (r *RunningApplications) TabIndex() int {
	if r.buttonCount() == 0 {
		return -1
	}
	return focusTabIndex(r)
}

func (r *RunningApplications) buttonCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.btns)
}

// OnKeyEvent: стрелки, Home и End выбирают кнопку, Enter и Space нажимают её:
// активное окно сворачивается, остальные активируются.
func (r *RunningApplications) OnKeyEvent(e widget.KeyEvent) {
	r.HandleCellKey(e, r.buttonCount(), r.activateButton, r.Invalidate)
}

func (r *RunningApplications) activateButton(i int) {
	r.mu.RLock()
	var info WindowInfo
	ok := i >= 0 && i < len(r.btns)
	if ok {
		info = r.btns[i].info
	}
	r.mu.RUnlock()
	if !ok || r.wm == nil {
		return
	}
	if info.Active {
		r.wm.Minimize(info.ID)
	} else {
		r.wm.Activate(info.ID)
	}
}

// FocusRing обводит выбранную кнопку.
func (r *RunningApplications) FocusRing() (image.Rectangle, *theme.Style) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c := r.FocusState.Cell(len(r.btns))
	if c < 0 {
		return image.Rectangle{}, nil
	}
	return r.btns[c].rect, styleNormal(r.tm, ComponentTaskButton)
}

// ─── Значки трея, надпись и часы ─────────────────────────────────────────────

// SetFocused реализует widget.Focusable.
func (n *NetworkItem) SetFocused(v bool) {
	if n.FocusState.Set(v) {
		n.Invalidate()
	}
}

// TabIndex исключает значок из обхода, пока у него нет места на панели.
func (n *NetworkItem) TabIndex() int { return focusTabIndex(n) }

// OnKeyEvent: Enter и Space — как щелчок.
func (n *NetworkItem) OnKeyEvent(e widget.KeyEvent) {
	n.HandleKey(n, e, click(n.OnClick), n.Invalidate)
}

// FocusRing реализует FocusRinger.
func (n *NetworkItem) FocusRing() (image.Rectangle, *theme.Style) {
	return n.Bounds(), styleNormal(n.tm, ComponentNetwork)
}

// SetFocused реализует widget.Focusable.
func (v *VolumeItem) SetFocused(focused bool) {
	if v.FocusState.Set(focused) {
		v.Invalidate()
	}
}

// TabIndex исключает значок из обхода, пока у него нет места на панели.
func (v *VolumeItem) TabIndex() int { return focusTabIndex(v) }

// OnKeyEvent: Enter и Space — как щелчок.
func (v *VolumeItem) OnKeyEvent(e widget.KeyEvent) {
	v.HandleKey(v, e, click(v.OnClick), v.Invalidate)
}

// FocusRing реализует FocusRinger.
func (v *VolumeItem) FocusRing() (image.Rectangle, *theme.Style) {
	return v.Bounds(), styleNormal(v.tm, ComponentVolume)
}

// SetFocused реализует widget.Focusable.
func (p *PowerItem) SetFocused(v bool) {
	if p.FocusState.Set(v) {
		p.Invalidate()
	}
}

// TabIndex исключает значок из обхода, пока у него нет места на панели
// (настольная машина без батареи места не получает).
func (p *PowerItem) TabIndex() int { return focusTabIndex(p) }

// OnKeyEvent: Enter и Space — как щелчок.
func (p *PowerItem) OnKeyEvent(e widget.KeyEvent) {
	p.HandleKey(p, e, click(p.OnClick), p.Invalidate)
}

// FocusRing реализует FocusRinger.
func (p *PowerItem) FocusRing() (image.Rectangle, *theme.Style) {
	return p.Bounds(), styleNormal(p.tm, ComponentPower)
}

// SetFocused реализует widget.Focusable.
func (l *TrayLabel) SetFocused(v bool) {
	if l.FocusState.Set(v) {
		l.Invalidate()
	}
}

// TabIndex: надпись без обработчика клика — не кнопка, и в обход не входит.
func (l *TrayLabel) TabIndex() int {
	if l.OnClick == nil {
		return -1
	}
	return focusTabIndex(l)
}

// OnKeyEvent: Enter и Space — как щелчок.
func (l *TrayLabel) OnKeyEvent(e widget.KeyEvent) {
	l.HandleKey(l, e, click(l.OnClick), l.Invalidate)
}

// FocusRing реализует FocusRinger.
func (l *TrayLabel) FocusRing() (image.Rectangle, *theme.Style) {
	return l.Bounds(), styleNormal(l.tm, ComponentTrayLabel)
}

// SetFocused реализует widget.Focusable.
func (c *ClockItem) SetFocused(v bool) {
	if c.FocusState.Set(v) {
		c.Invalidate()
	}
}

// TabIndex: часы без обработчика клика (календаря) — не кнопка, и в обход не
// входят.
func (c *ClockItem) TabIndex() int {
	if c.OnClick == nil {
		return -1
	}
	return focusTabIndex(c)
}

// OnKeyEvent: Enter и Space открывают календарь, как щелчок по часам.
func (c *ClockItem) OnKeyEvent(e widget.KeyEvent) {
	c.HandleKey(c, e, click(c.OnClick), c.Invalidate)
}

// FocusRing реализует FocusRinger.
func (c *ClockItem) FocusRing() (image.Rectangle, *theme.Style) {
	return c.Bounds(), styleNormal(c.tm, ComponentClock)
}
