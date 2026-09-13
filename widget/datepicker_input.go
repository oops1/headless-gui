package widget

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/internal/calendar"
)

// datepicker_input.go — клавиатура, мышь, фокус и закрытие календаря.

// SetFocused — контракт Focusable. Уход фокуса принимает набранное; если оно
// не разобралось, поле возвращается к выбранной дате: оставлять в поле текст,
// который ничего не значит, хуже, чем отменить набор.
func (p *DatePicker) SetFocused(v bool) {
	p.change(func() {
		if p.focused == v {
			return
		}
		p.focused = v
		if v {
			p.selectAll = true
			return
		}
		p.commitLocked()
		if p.invalid {
			p.revertLocked()
		}
		p.selectAll = false
		p.setOpenLocked(false)
	})
}

// IsFocused — контракт Focusable.
func (p *DatePicker) IsFocused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.focused
}

// Dismiss — контракт Dismissable: щелчок мимо закрывает календарь.
func (p *DatePicker) Dismiss() {
	p.mu.Lock()
	open := p.open
	p.mu.Unlock()
	if open {
		p.SetDropDownOpen(false)
	}
}

// OnKeyEvent — клавиатура поля и открытого календаря.
func (p *DatePicker) OnKeyEvent(e KeyEvent) {
	if !e.Pressed || !p.IsEnabled() {
		return
	}
	alt := e.Mod&ModAlt != 0
	ctrl := e.Mod&ModCtrl != 0
	p.change(func() {
		if p.open {
			p.calendarKeyLocked(e, alt)
			return
		}
		switch {
		case e.Code == KeyF4, alt && e.Code == KeyDown:
			p.commitLocked()
			p.setOpenLocked(true)
		case e.Code == KeyEnter:
			p.commitLocked()
		case e.Code == KeyEscape:
			p.revertLocked()
		case e.Code == KeyBackspace:
			p.beginEditLocked()
			if p.selectAll {
				p.text, p.selectAll = nil, false
			} else if n := len(p.text); n > 0 {
				p.text = p.text[:n-1]
			}
			p.invalid = false
		case e.Code == KeyDelete:
			p.beginEditLocked()
			p.text, p.selectAll, p.invalid = nil, false, false
		case ctrl && e.Code == KeyA:
			p.selectAll = true
		case e.Rune >= '0' && e.Rune <= '9', e.Rune == '.', e.Rune == '/', e.Rune == '-':
			p.beginEditLocked()
			if p.selectAll {
				p.text, p.selectAll = nil, false
			}
			if len(p.text) < 16 {
				p.text = append(p.text, e.Rune)
			}
			p.invalid = false
		}
	})
}

// beginEditLocked переводит поле в набор: текст начинается с того, что было
// видно, — как в обычном поле ввода.
func (p *DatePicker) beginEditLocked() {
	if p.editing {
		return
	}
	p.text = []rune(p.textLocked())
	p.editing = true
}

// calendarKeyLocked — клавиши открытого календаря: стрелки по дням и неделям,
// PgUp/PgDn по месяцам, Home/End — начало и конец месяца, Enter выбирает.
func (p *DatePicker) calendarKeyLocked(e KeyEvent, alt bool) {
	switch e.Code {
	case KeyEscape, KeyF4:
		p.setOpenLocked(false)
	case KeyUp:
		if alt {
			p.setOpenLocked(false)
			return
		}
		p.moveCursorLocked(-7)
	case KeyDown:
		p.moveCursorLocked(7)
	case KeyLeft:
		p.moveCursorLocked(-1)
	case KeyRight:
		p.moveCursorLocked(1)
	case KeyPageUp:
		p.shiftMonthLocked(-1)
	case KeyPageDown:
		p.shiftMonthLocked(1)
	case KeyHome:
		p.cursor = p.clampLocked(p.view)
	case KeyEnd:
		p.cursor = p.clampLocked(p.view.AddDate(0, 1, -1))
	case KeyEnter, KeySpace:
		if p.inRangeLocked(p.cursor) {
			p.setLocked(p.cursor)
			p.setOpenLocked(false)
		}
	}
}

// OnMouseButton — щелчки по полю, кнопке календаря и числам.
func (p *DatePicker) OnMouseButton(e MouseEvent) bool {
	if !p.IsEnabled() || e.Button != MouseLeft || !e.Pressed {
		return false
	}
	handled := false
	p.change(func() {
		field := p.Base.Bounds()
		pt := image.Pt(e.X, e.Y)
		if p.open {
			handled = true
			if pt.In(field) {
				p.setOpenLocked(false)
				return
			}
			if !pt.In(dpCalendarRect(field)) {
				p.setOpenLocked(false)
				return
			}
			switch hit, d := p.hitLocked(e.X, e.Y); hit {
			case dpHitPrev:
				p.shiftMonthLocked(-1)
			case dpHitNext:
				p.shiftMonthLocked(1)
			case dpHitDay:
				if p.inRangeLocked(d) {
					p.setLocked(calendar.DateOnly(d))
					p.setOpenLocked(false)
				}
			}
			return
		}
		if !pt.In(field) {
			return
		}
		handled = true
		if pt.In(dpButtonRect(field)) {
			p.commitLocked()
			p.setOpenLocked(true)
		}
	})
	return handled
}

// OnMouseMove — подсветка числа и стрелок под мышью.
func (p *DatePicker) OnMouseMove(x, y int) {
	p.mu.Lock()
	if !p.open {
		p.mu.Unlock()
		return
	}
	hit, d := p.hitLocked(x, y)
	hover := time.Time{}
	if hit == dpHitDay && p.inRangeLocked(d) {
		hover = d
	}
	changed := hit != p.hoverAt || !hover.Equal(p.hover)
	p.hoverAt, p.hover = hit, hover
	p.mu.Unlock()
	if changed {
		notifyUIChanged()
	}
}

// OnMouseWheelPixels — колесо над открытым календарём листает месяцы.
func (p *DatePicker) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	handled := false
	p.change(func() {
		if !p.open || !image.Pt(x, y).In(dpCalendarRect(p.Base.Bounds())) || dy == 0 {
			return
		}
		handled = true
		if dy > 0 {
			p.shiftMonthLocked(-1)
		} else {
			p.shiftMonthLocked(1)
		}
	})
	return handled
}

// Cursor — текстовый курсор над полем, стрелка над кнопкой и календарём.
func (p *DatePicker) Cursor(x, y int) Cursor {
	field := p.Base.Bounds()
	pt := image.Pt(x, y)
	if pt.In(field) && !pt.In(dpButtonRect(field)) {
		return CursorIBeam
	}
	return CursorArrow
}
