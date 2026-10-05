// notificationcenter_win11.go — центр уведомлений Windows 11 как третий вариант
// NotificationCenter: размещение над календарём, «Не беспокоить», связь с
// календарём в группу, передача клавиатурного фокуса.
//
// Компонент один. Вид (плоский, Windows 10, Windows 11) выбирает презентер,
// который назначает профиль темы; здесь, как и в notificationcenter_win10.go,
// имени темы нет. Раскладка, рисование и ввод — общие с Windows 10
// (notifyview*.go); отличается то, что стоит вокруг: карточка центра сидит у
// правого края рабочей области над календарём, а не растянута на всю высоту, и
// открывается вместе с календарём одной группой.
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/internal/focusreq"
	"github.com/oops1/headless-gui/v3/theme"
)

func init() {
	RegisterPresenter(theme.NotificationCenterWin11Presenter, ncPresenter{})
}

// win11 сообщает, что центру назначен презентер Windows 11.
func (nc *NotificationCenter) win11() bool { return nc.view.w11() }

// LinkNotificationCenter соединяет центр уведомлений и календарь Windows 11 в
// группу: центр стоит над календарём, панели закрываются вместе, клик по одной
// не закрывает другую, Tab переходит из одной в другую. Возвращает группу —
// OpenAll(anchor) открывает обе от часов или колокольчика, CloseAll закрывает.
//
// Календарь открывается первым: центр встаёт на то место, которое оставил
// календарь, и не прыгает при открытии. Центр без календаря (закрытого или не
// связанного) стоит прямо над панелью задач.
func LinkNotificationCenter(nc *NotificationCenter, cal *CalendarFlyout) *FlyoutGroup {
	if nc == nil || cal == nil {
		return nil
	}
	nc.mu.Lock()
	old := nc.unlinkCal
	nc.below = cal
	nc.mu.Unlock()
	if old != nil {
		old()
	}
	cal.linkAbove(nc)
	unsub := cal.Subscribe(func(bool) { nc.relayout() })
	nc.mu.Lock()
	nc.unlinkCal = unsub
	nc.mu.Unlock()
	return NewFlyoutGroup(cal.Flyout, nc.Flyout)
}

// relayout перерисовывает центр там, где он стоял, и там, где встанет: календарь
// под ним открылся, закрылся или сменил высоту.
func (nc *NotificationCenter) relayout() {
	nc.mu.Lock()
	was := nc.lastRect
	nc.mu.Unlock()
	if !nc.IsOpen() {
		return
	}
	nc.invalidateOverlay(was)
}

// applySlide выбирает, откуда выезжает панель: Windows 11 поднимается вместе с
// календарём снизу на небольшое расстояние, Windows 10 выезжает из-за правого
// края экрана целиком.
func (nc *NotificationCenter) applySlide() {
	if nc.win11() {
		nc.Slide, nc.SlideDistance = SlideBottom, 0
		return
	}
	nc.Slide, nc.SlideDistance = SlideRight, -1
}

// SetDoNotDisturb задаёт модель «Не беспокоить» (nil — без колокольчика в
// заголовке: он рисуется, но ничего не показывает и не переключает). Колокольчик
// показывает только центр Windows 11. Модель общая с кнопкой центра на панели
// задач и с тостом (NotificationToast.SetDoNotDisturb).
func (nc *NotificationCenter) SetDoNotDisturb(d DoNotDisturb) {
	nc.mu.Lock()
	old := nc.unsubDND
	nc.unsubDND = nil
	nc.dnd = d
	nc.mu.Unlock()
	if old != nil {
		old()
	}
	if nc.IsOpen() {
		nc.attach()
	}
	nc.Invalidate()
}

// DoNotDisturb возвращает модель «Не беспокоить».
func (nc *NotificationCenter) DoNotDisturb() DoNotDisturb {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	return nc.dnd
}

func (nc *NotificationCenter) dndEnabled() bool {
	d := nc.DoNotDisturb()
	return d != nil && d.Enabled()
}

func (nc *NotificationCenter) toggleDND() {
	if d := nc.DoNotDisturb(); d != nil {
		d.SetEnabled(!d.Enabled())
	}
}

// onDNDChanged — режим «Не беспокоить» сменился (горутина потребителя):
// перерисовывается только колокольчик.
func (nc *NotificationCenter) onDNDChanged() {
	if !nc.IsOpen() {
		return
	}
	nc.view.mu.Lock()
	r := nc.view.lastLayout.dnd
	nc.view.mu.Unlock()
	if r.Empty() {
		nc.Invalidate()
		return
	}
	nc.view.invalidate(r)
}

// ─── Размещение ──────────────────────────────────────────────────────────────

// w11Area — область, в которой стоят панели Windows 11: рабочая область, если
// она задана, иначе экран, обрезанный по панели задач (по краю значка-привязки).
func w11Area(workArea, screen, anchor image.Rectangle, edge Edge) image.Rectangle {
	if !workArea.Empty() {
		return workArea
	}
	if screen.Empty() {
		return image.Rectangle{}
	}
	a := screen
	if !anchor.Empty() {
		if edge == EdgeTop {
			a.Min.Y = anchor.Max.Y
		} else {
			a.Max.Y = anchor.Min.Y
		}
	}
	return a
}

// w11Frame возвращает, где может стоять центр: правый край (до поля edge), нижняя
// граница (над календарём, а без него — над панелью задач) и верх. Пустая
// область — экран не назван, высоту даёт запасное значение.
func (nc *NotificationCenter) w11Frame(m ncMetrics) (right, top, bottom int, ok bool) {
	area := w11Area(nc.WorkArea, nc.Screen, nc.Anchor, nc.Edge)
	if area.Empty() {
		return 0, 0, 0, false
	}
	right = area.Max.X - m.edge
	top = area.Min.Y + m.edge
	bottom = area.Max.Y - m.edge
	nc.mu.Lock()
	cal := nc.below
	nc.mu.Unlock()
	if cal != nil && cal.IsOpen() {
		bottom = cal.restRect().Min.Y - m.stackGap
	}
	return right, top, bottom, true
}

// w11Size — размер центра Windows 11: по содержимому, но не выше места над
// календарём.
func (nc *NotificationCenter) w11Size() image.Point {
	m := ncReadMetrics(nc.Theme())
	w := m.width
	_, top, bottom, ok := nc.w11Frame(m)
	maxH := ncFallbackHeight
	if ok {
		maxH = bottom - top
		if area := w11Area(nc.WorkArea, nc.Screen, nc.Anchor, nc.Edge); w > area.Dx()-2*m.edge {
			w = area.Dx() - 2*m.edge
		}
	}
	if maxH < 1 {
		maxH = 1
	}
	return nc.view.sizeW11(w, maxH)
}

// w11Place — прямоугольник центра: у правого края, нижней стороной над
// календарём (или панелью задач).
func (nc *NotificationCenter) w11Place(size image.Point) image.Rectangle {
	m := ncReadMetrics(nc.Theme())
	right, top, bottom, ok := nc.w11Frame(m)
	if !ok {
		// Экран не назван: от значка-привязки.
		right = nc.Anchor.Max.X
		bottom = nc.Anchor.Min.Y - m.edge
		top = bottom - size.Y
	}
	if nc.Edge == EdgeTop && ok {
		return image.Rect(right-size.X, top, right, top+size.Y)
	}
	return image.Rect(right-size.X, bottom-size.Y, right, bottom)
}

// ─── Фокус ───────────────────────────────────────────────────────────────────

// handoffFocus передаёт клавиатурный фокус календарю под центром, когда Tab
// вышел за последнюю остановку центра. Истина — календарь фокус принял.
func (nc *NotificationCenter) handoffFocus(delta int) bool {
	nc.mu.Lock()
	cal := nc.below
	nc.mu.Unlock()
	if cal == nil || !cal.IsOpen() || !cal.win11() || delta < 0 {
		return false
	}
	cal.takeFocus(false)
	return true
}

// takeFocus принимает клавиатурный фокус от календаря: на последнюю остановку,
// если фокус пришёл по Shift+Tab, либо на первую.
func (nc *NotificationCenter) takeFocus(last bool) {
	if !nc.IsOpen() || !nc.win11() {
		return
	}
	nc.view.takeFocus(nc.rect(), last)
	nc.fs.noteKey()
	focusreq.Request(nc)
	nc.Invalidate()
}
