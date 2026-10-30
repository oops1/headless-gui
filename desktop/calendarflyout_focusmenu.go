// calendarflyout_focusmenu.go — меню выбора длительности у кнопки «▶ Фокусировка»
// календаря Windows 11: правый щелчок по кнопке (или клавиша меню / Shift+F10,
// когда кнопка в клавиатурном фокусе) открывает список 15/30/45/60/90 минут;
// выбранная длительность уходит в FocusSession.SetDuration.
//
// Меню рисует PopupMenu движка (профиль темы красит его так же, как остальные
// меню оболочки), а сам календарь держит его в дочерних: щелчок по пункту тогда
// не считается щелчком мимо календаря. «− +» остаются главным способом выбора —
// как на настоящей Windows 11, меню лишь быстрый путь к круглым значениям.
package desktop

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v4/widget"
)

// DefaultFocusDurations — длительности меню выбора у кнопки «Фокусировка»,
// пока CalendarFlyout.FocusDurations не задано.
var DefaultFocusDurations = []time.Duration{
	15 * time.Minute, 30 * time.Minute, 45 * time.Minute, 60 * time.Minute, 90 * time.Minute,
}

// focusDurations возвращает длительности меню: свои либо стандартные.
func (c *CalendarFlyout) focusDurations() []time.Duration {
	if len(c.FocusDurations) > 0 {
		return c.FocusDurations
	}
	return DefaultFocusDurations
}

// ensureFocusMenu создаёт меню при первой надобности и кладёт его в дочерние
// календаря (см. описание файла).
func (c *CalendarFlyout) ensureFocusMenu() *widget.PopupMenu {
	c.mu.Lock()
	p := c.focusMenu
	c.mu.Unlock()
	if p != nil {
		return p
	}
	p = widget.NewPopupMenu()
	p.SetHost(c)
	c.mu.Lock()
	if c.focusMenu == nil {
		c.focusMenu = p
	} else {
		p = c.focusMenu
	}
	c.mu.Unlock()
	c.AddChild(p)
	return p
}

// FocusMenuOpen сообщает, открыто ли меню выбора длительности.
func (c *CalendarFlyout) FocusMenuOpen() bool {
	c.mu.Lock()
	p := c.focusMenu
	c.mu.Unlock()
	return p != nil && p.IsOpen()
}

// CloseFocusMenu закрывает меню выбора длительности, если оно открыто.
func (c *CalendarFlyout) CloseFocusMenu() {
	c.mu.Lock()
	p := c.focusMenu
	c.mu.Unlock()
	if p != nil && p.IsOpen() {
		p.Close()
	}
}

// OpenFocusDurationMenu открывает меню выбора длительности над кнопкой
// «Фокусировка». Возвращает false, если открывать нечего: календарь закрыт или
// рисуется не видом Windows 11, сеанса нет, либо он уже идёт (длительность
// идущего сеанса не меняют).
func (c *CalendarFlyout) OpenFocusDurationMenu() bool {
	s := c.FocusSession()
	if s == nil || !c.IsOpen() || !c.win11() {
		return false
	}
	st := s.State()
	if st.Running {
		return false
	}
	l := c.w11Layout(c.rect())
	if l.faction.Empty() {
		return false
	}
	items := make([]widget.MenuItem, 0, len(c.focusDurations()))
	for _, d := range c.focusDurations() {
		d := d
		items = append(items, widget.MenuItem{
			Text:      focusLabel(d),
			Checkable: true,
			Checked:   d == st.Duration,
			OnClick:   func() { s.SetDuration(focusClamp(d)) },
		})
	}
	p := c.ensureFocusMenu()
	p.SetItems(items)
	themeMenu(p, c.Theme())
	// Меню ставят над кнопкой: ниже неё панель задач. PopupMenu умеет только «от
	// точки», поэтому высоту узнаём пробным показом.
	p.Show(l.faction.Min.X, l.faction.Min.Y)
	if r := p.OverlayBounds(); !r.Empty() {
		p.Show(l.faction.Min.X, l.faction.Min.Y-r.Dy()-2)
	}
	return true
}

// focusMenuMouse разбирает правую кнопку: на отпускании над «Фокусировкой»
// открывает меню. Возвращает true, если событие принято.
func (c *CalendarFlyout) focusMenuMouse(e widget.MouseEvent) bool {
	if e.Button != widget.MouseRight || !c.IsOpen() {
		return false
	}
	pt := image.Pt(e.X, e.Y)
	if !pt.In(c.rect()) {
		return false
	}
	l := c.w11Layout(c.rect())
	if l.stopAt(pt) != calStopAction {
		return false
	}
	if !e.Pressed {
		// Как у контекстных меню оболочки, меню открывается на отпускании: правая
		// кнопка, отпущенная над уже открытым меню, закрыла бы его тут же.
		c.OpenFocusDurationMenu()
	}
	return true
}

// focusMenuKey отдаёт клавишу открытому меню (стрелки, Enter, Esc закрывает
// только меню) либо открывает его по клавише меню и Shift+F10 на кнопке
// «Фокусировка». Возвращает true, если клавиша разобрана.
func (c *CalendarFlyout) focusMenuKey(e widget.KeyEvent) bool {
	if c.FocusMenuOpen() {
		if e.Code == widget.KeyEscape {
			c.CloseFocusMenu()
		} else {
			c.mu.Lock()
			p := c.focusMenu
			c.mu.Unlock()
			p.OnKeyEvent(e)
		}
		return true
	}
	c.mu.Lock()
	onAction := c.focusStop == calStopAction
	c.mu.Unlock()
	if onAction && (e.Code == widget.KeyMenu || (e.Code == widget.KeyF10 && e.Mod&widget.ModShift != 0)) {
		return c.OpenFocusDurationMenu()
	}
	return false
}

// DismissOnEscape: Esc сначала закрывает меню выбора длительности и только
// потом — календарь.
func (c *CalendarFlyout) DismissOnEscape() bool {
	if c.FocusMenuOpen() {
		c.CloseFocusMenu()
		return true
	}
	return c.Flyout.DismissOnEscape()
}
