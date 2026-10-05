// calendarflyout_win11.go — календарь Windows 11 как вариант CalendarFlyout:
// отдельная скруглённая карточка с датой сверху, месяцем, сеткой дней и
// модулем «Фокусировка» под ней.
//
// Компонент один. Вид выбирает презентер, который назначает профиль темы
// (theme.CalendarWin11Presenter); имени темы здесь нет. Месяц, выбор дня,
// свёрнутость, культура и часы — те же, что у плоского календаря, меняется
// раскладка и то, что вокруг: карточка у правого края рабочей области над
// панелью задач, а центр уведомлений (LinkNotificationCenter) стоит над ней.
//
// «Фокусировка» — интерфейс FocusSession от потребителя: календарь рисует
// длительность, «−», «+», «Начать» и идущий отсчёт; секундный отсчёт ведёт он
// сам (Ticker), пока открыт и идёт сеанс.
package desktop

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/internal/focusreq"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

func init() {
	RegisterPresenter(theme.CalendarWin11Presenter, calendarW11Presenter{})
}

// calendarW11Presenter — презентер календаря Windows 11. Рисует и раскладывает
// сам компонент; презентер лишь называет вид в профиле темы.
type calendarW11Presenter struct{}

func (calendarW11Presenter) Measure(Component, image.Point) image.Point          { return image.Point{} }
func (calendarW11Presenter) Layout(Component, image.Rectangle) []image.Rectangle { return nil }
func (calendarW11Presenter) Draw(widget.DrawContext, Component)                  {}

// Метрики календаря Windows 11 (theme/profiles_win11_notify.go).
const (
	calW11KeyWidth   theme.Key = "calendar.w11.width"
	calW11KeyPad     theme.Key = "calendar.w11.pad"
	calW11KeyHead    theme.Key = "calendar.w11.head"
	calW11KeyMonth   theme.Key = "calendar.w11.month"
	calW11KeyWeekday theme.Key = "calendar.w11.weekday"
	calW11KeyCell    theme.Key = "calendar.w11.cell"
	calW11KeyDay     theme.Key = "calendar.w11.day"
	calW11KeyNav     theme.Key = "calendar.w11.nav"
	calW11KeyFocus   theme.Key = "calendar.w11.focus"
	calW11KeyButton  theme.Key = "calendar.w11.button"
	calW11KeyEdge    theme.Key = "calendar.w11.edge"
)

// Части стиля календаря Windows 11.
const (
	calPartDate        = "date"
	calPartMonth       = "month"
	calPartNav         = "nav"
	calPartDivider     = "divider"
	calPartDim         = "dim"
	calPartFocusTitle  = "focus.title"
	calPartFocusValue  = "focus.value"
	calPartFocusButton = "focus.button"
	calPartFocusAccent = "focus.accent"
	calPartProgress    = "progress"
	calPartProgressBar = "progress.fill"
)

// Пропорции модуля «Фокусировка», не размеры темы.
const (
	// calW11FocusTop — отступ от разделителя до заголовка модуля.
	calW11FocusTop = 12
	// calW11FocusTitleH — высота строки заголовка модуля.
	calW11FocusTitleH = 24
	// calW11FocusGap — зазор между строкой заголовка и строкой кнопок.
	calW11FocusGap = 8
	// calW11ValueW — ширина поля с длительностью между «−» и «+».
	calW11ValueW = 88
	// calW11ProgressH — толщина полосы прогресса идущего сеанса.
	calW11ProgressH = 4
	// calW11ButtonPad — поле по бокам надписи кнопки «Начать».
	calW11ButtonPad = 24
	// calW11GlyphDiv — доля стороны кнопки под фигуру значка («−», «+», шеврон).
	calW11GlyphDiv = 3
)

// calStop — остановка клавиатурного фокуса и цель наведения в календаре.
type calStop int

const (
	calStopNone calStop = iota
	calStopCollapse
	calStopPrev
	calStopNext
	calStopGrid // сетка дней: стрелки двигают день, Enter выбирает
	calStopLess
	calStopMore
	calStopAction // «Начать» или «Остановить»
)

// win11 сообщает, что календарю назначен презентер Windows 11.
func (c *CalendarFlyout) win11() bool {
	tm := c.Theme()
	if tm == nil {
		return false
	}
	t := tm.Active()
	return t != nil && t.PresenterName(ComponentCalendar) == theme.CalendarWin11Presenter
}

// SetFocusSession задаёт сеанс «Фокусировки» (nil — модуля нет). Модуль
// показывает только календарь Windows 11.
func (c *CalendarFlyout) SetFocusSession(s FocusSession) {
	c.relayout(func() {
		c.mu.Lock()
		unsub := c.unsubFocus
		c.unsubFocus = nil
		c.focusSess = s
		c.mu.Unlock()
		if unsub != nil {
			unsub()
		}
		if c.IsOpen() {
			c.attachFocus()
		}
	})
}

// FocusSession возвращает сеанс «Фокусировки».
func (c *CalendarFlyout) FocusSession() FocusSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.focusSess
}

// linkAbove запоминает центр уведомлений, стоящий над календарём: при смене
// высоты календаря центр перерисовывается и перекладывается (см.
// LinkNotificationCenter).
func (c *CalendarFlyout) linkAbove(nc *NotificationCenter) {
	c.mu.Lock()
	c.above = nc
	c.mu.Unlock()
}

// relayout выполняет изменение, от которого календарь меняет размер или
// положение: перерисовывается и прежнее место, и новое, а центр над ним —
// заново. Без прежнего места при сворачивании на экране оставался бы след
// верхней части карточки.
func (c *CalendarFlyout) relayout(change func()) {
	var was image.Rectangle
	if c.visible() {
		was = c.dirtyRect()
	}
	change()
	c.invalidateOverlay(was)
	c.mu.Lock()
	above := c.above
	c.mu.Unlock()
	if above != nil {
		above.relayout()
	}
}

// ─── Метрики и раскладка ─────────────────────────────────────────────────────

type calW11Metrics struct {
	width, pad, head, month, weekday, cell, day, nav, focus, button, edge int
}

func (c *CalendarFlyout) w11Metrics() calW11Metrics {
	get := func(k theme.Key, def int) int {
		if v := c.metric(k); v > 0 {
			return v
		}
		return def
	}
	return calW11Metrics{
		width:   get(calW11KeyWidth, 364),
		pad:     get(calW11KeyPad, 16),
		head:    get(calW11KeyHead, 44),
		month:   get(calW11KeyMonth, 36),
		weekday: get(calW11KeyWeekday, 28),
		cell:    get(calW11KeyCell, 40),
		day:     get(calW11KeyDay, 36),
		nav:     get(calW11KeyNav, 32),
		focus:   get(calW11KeyFocus, 96),
		button:  get(calW11KeyButton, 32),
		edge:    c.metric(calW11KeyEdge),
	}
}

// calStopRect — остановка и её прямоугольник.
type calStopRect struct {
	id   calStop
	rect image.Rectangle
}

// calW11Layout — раскладка карточки в прямоугольнике panel. draw, мышь и
// клавиатура считают её одной функцией, так что нажимается то, что нарисовано.
type calW11Layout struct {
	panel image.Rectangle
	m     calW11Metrics

	head, date                  image.Rectangle
	collapse, prev, next, month image.Rectangle
	weekday                     image.Rectangle
	days                        [][]dayCellLayout
	grid                        image.Rectangle // вся сетка дней (остановка клавиатурного фокуса)

	// Модуль «Фокусировка»; focus пуст, пока сеанс не задан.
	divider, focus, ftitle, fless, fvalue, fmore image.Rectangle
	faction, fremain, fprogress                  image.Rectangle
}

// w11Layout раскладывает карточку сверху вниз: дата, месяц, дни недели, сетка,
// разделитель, модуль «Фокусировка». Свёрнутая карточка — только строка даты.
func (c *CalendarFlyout) w11Layout(panel image.Rectangle) calW11Layout {
	m := c.w11Metrics()
	l := calW11Layout{panel: panel, m: m}
	x0 := panel.Min.X + m.pad
	w := panel.Dx() - 2*m.pad
	if w < 1 {
		w = 1
	}
	y := panel.Min.Y + m.pad/2

	l.head = image.Rect(x0, y, x0+w, y+m.head)
	l.collapse = image.Rect(x0+w-m.nav, y+(m.head-m.nav)/2, x0+w, y+(m.head-m.nav)/2+m.nav)
	l.date = image.Rect(x0, y, l.collapse.Min.X-8, y+m.head)
	y += m.head

	collapsed := c.effCollapsed()
	if !collapsed {
		l.month = image.Rect(x0, y, x0+w, y+m.month)
		l.next = image.Rect(x0+w-m.nav, y+(m.month-m.nav)/2, x0+w, y+(m.month-m.nav)/2+m.nav)
		l.prev = l.next.Sub(image.Pt(m.nav, 0))
		y += m.month

		l.weekday = image.Rect(x0, y, x0+w, y+m.weekday)
		y += m.weekday

		grid := c.gridSnapshot()
		colW := w / 7
		l.days = make([][]dayCellLayout, len(grid))
		for row := range grid {
			cells := make([]dayCellLayout, 7)
			for col := 0; col < 7; col++ {
				cx := x0 + col*colW
				cells[col] = dayCellLayout{
					rect: image.Rect(cx, y, cx+colW, y+m.cell),
					day:  grid[row][col],
				}
			}
			l.days[row] = cells
			y += m.cell
		}
		if len(l.days) > 0 {
			l.grid = image.Rect(x0, l.weekday.Max.Y, x0+w, y)
		}
	}

	if sess := c.FocusSession(); sess != nil {
		l.divider = image.Rect(panel.Min.X+m.pad, y+m.pad/2, panel.Max.X-m.pad, y+m.pad/2+1)
		my := l.divider.Max.Y
		l.focus = image.Rect(panel.Min.X, my, panel.Max.X, my+m.focus)
		ty := my + calW11FocusTop
		l.ftitle = image.Rect(x0, ty, x0+w, ty+calW11FocusTitleH)
		ry := ty + calW11FocusTitleH + calW11FocusGap
		b := m.button
		l.fless = image.Rect(x0, ry, x0+b, ry+b)
		l.fvalue = image.Rect(l.fless.Max.X, ry, l.fless.Max.X+calW11ValueW, ry+b)
		l.fmore = image.Rect(l.fvalue.Max.X, ry, l.fvalue.Max.X+b, ry+b)
		l.fremain = image.Rect(x0, ry, x0+w/2, ry+b)
		l.fprogress = image.Rect(x0, ry+b+calW11FocusGap, x0+w, ry+b+calW11FocusGap+calW11ProgressH)
		// Кнопка справа: «Начать» или «Остановить» — по ширине надписи.
		bw := c.w11Fonts().body.width(focusActionLabel(sess.State())) + 2*calW11ButtonPad
		l.faction = image.Rect(x0+w-bw, ry, x0+w, ry+b)
	}
	return l
}

// w11Size — размер карточки по содержимому: высота зависит от числа недель в
// месяце (4–6), от свёрнутости и от модуля «Фокусировка».
func (c *CalendarFlyout) w11Size() image.Point {
	m := c.w11Metrics()
	return image.Pt(m.width, c.w11Height(m, c.effCollapsed()))
}

// w11Height — высота карточки: свёрнутая (только строка даты) или развёрнутая
// (месяц, дни недели, сетка из 4–6 недель), с модулем «Фокусировка» или без.
func (c *CalendarFlyout) w11Height(m calW11Metrics, collapsed bool) int {
	h := m.pad/2 + m.head
	if !collapsed {
		h += m.month + m.weekday + len(c.gridSnapshot())*m.cell
	}
	if c.FocusSession() != nil {
		h += m.pad/2 + 1 + m.focus
	} else {
		h += m.pad
	}
	return h
}

// effCollapsed — свёрнут ли календарь на самом деле: выбор пользователя
// (Collapsed) либо нехватка места. Над календарём стоит центр уведомлений
// (LinkNotificationCenter), и ему нужен хотя бы заголовок с короткой частью
// списка; на низком экране (ноутбук 125–150 %) развёрнутая сетка оставила бы центру
// полоску, поэтому календарь остаётся свёрнутым, пока места не хватает. Выбор
// пользователя при этом не меняется: вырастет экран — сетка вернётся.
func (c *CalendarFlyout) effCollapsed() bool {
	if c.Collapsed() {
		return true
	}
	c.mu.Lock()
	above := c.above
	c.mu.Unlock()
	if above == nil || !above.win11() {
		return false
	}
	area := w11Area(c.WorkArea, c.Screen, c.Anchor, c.Edge)
	if area.Empty() {
		return false
	}
	m := c.w11Metrics()
	nm := ncReadMetrics(c.Theme())
	need := nm.headerH + nm.listMin + nm.pad
	return area.Dy()-2*m.edge-c.w11Height(m, false)-nm.stackGap < need
}

// w11Place — карточка у правого края рабочей области, нижней стороной над
// панелью задач с зазором edge.
func (c *CalendarFlyout) w11Place(size image.Point) image.Rectangle {
	m := c.w11Metrics()
	area := w11Area(c.WorkArea, c.Screen, c.Anchor, c.Edge)
	if area.Empty() {
		// Экран не назван: от значка-привязки.
		right := c.Anchor.Max.X
		bottom := c.Anchor.Min.Y - m.edge
		return image.Rect(right-size.X, bottom-size.Y, right, bottom)
	}
	right := area.Max.X - m.edge
	if c.Edge == EdgeTop {
		y := area.Min.Y + m.edge
		return image.Rect(right-size.X, y, right, y+size.Y)
	}
	bottom := area.Max.Y - m.edge
	return image.Rect(right-size.X, bottom-size.Y, right, bottom)
}

// place — Flyout.Place: у календаря Windows 11 своё размещение, у плоского —
// обычное, от значка.
func (c *CalendarFlyout) place(anchor, screen image.Rectangle, edge Edge, size image.Point) (image.Rectangle, bool) {
	if !c.win11() {
		return image.Rectangle{}, false
	}
	return c.w11Place(size), true
}

// stops — остановки клавиатурного фокуса в порядке обхода.
func (l *calW11Layout) stops() []calStopRect {
	out := []calStopRect{{calStopCollapse, l.collapse}}
	if !l.prev.Empty() {
		out = append(out, calStopRect{calStopPrev, l.prev}, calStopRect{calStopNext, l.next},
			calStopRect{calStopGrid, l.grid})
	}
	if !l.focus.Empty() {
		out = append(out, calStopRect{calStopLess, l.fless}, calStopRect{calStopMore, l.fmore},
			calStopRect{calStopAction, l.faction})
	}
	return out
}

// stopAt находит кнопку под точкой. Сетка дней — остановка клавиатуры, но не
// кнопка: дни под мышью разбирает отдельный обход по числам.
func (l *calW11Layout) stopAt(pt image.Point) calStop {
	for _, s := range l.stops() {
		if s.id != calStopGrid && pt.In(s.rect) {
			return s.id
		}
	}
	return calStopNone
}

// rectOf возвращает прямоугольник остановки.
func (l *calW11Layout) rectOf(id calStop) image.Rectangle {
	for _, s := range l.stops() {
		if s.id == id {
			return s.rect
		}
	}
	return image.Rectangle{}
}

// ─── Фокусировка ─────────────────────────────────────────────────────────────

// focusLabel — длительность словами: «30 мин», «1 ч 30 мин».
func focusLabel(d time.Duration) string {
	mins := int(d / time.Minute)
	if mins >= 60 {
		h, m := mins/60, mins%60
		if m == 0 {
			return fmt.Sprintf(tr(StrFocusHours), h)
		}
		return fmt.Sprintf(tr(StrFocusHoursMinutes), h, m)
	}
	return fmt.Sprintf(tr(StrFocusMinutes), mins)
}

// focusCountdown — остаток для отсчёта: «24:59», а от часа — «1:05:00». Секунда
// округляется вверх: сеанс на 25 минут начинается с 25:00, а не с 24:59.
func focusCountdown(d time.Duration) string {
	secs := int((d + time.Second - 1) / time.Second)
	if secs < 0 {
		secs = 0
	}
	h, m, s := secs/3600, secs%3600/60, secs%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// focusActionLabel — надпись правой кнопки модуля: «Начать» или «Остановить».
func focusActionLabel(st FocusSessionState) string {
	if st.Running {
		return tr(StrFocusStop)
	}
	return tr(StrFocusStart)
}

// focusAction выполняет «Начать» или «Остановить» и возвращает, что сделано.
func (c *CalendarFlyout) focusAction() {
	s := c.FocusSession()
	if s == nil {
		return
	}
	if s.State().Running {
		s.Stop()
	} else {
		s.Start()
	}
}

// focusStep двигает длительность на шаг; идущему сеансу длительность не
// меняют.
func (c *CalendarFlyout) focusStep(dir int) {
	s := c.FocusSession()
	if s == nil {
		return
	}
	st := s.State()
	if st.Running {
		return
	}
	d := focusClamp(st.Duration + time.Duration(dir)*FocusStep)
	if d != st.Duration {
		s.SetDuration(d)
	}
}

// attachFocus подписывается на сеанс, пока календарь открыт, и запускает
// секундный отсчёт, если сеанс идёт.
func (c *CalendarFlyout) attachFocus() {
	c.mu.Lock()
	s := c.focusSess
	need := s != nil && c.unsubFocus == nil
	c.mu.Unlock()
	if need {
		u := s.Subscribe(c.onFocusChanged)
		c.mu.Lock()
		if c.unsubFocus == nil {
			c.unsubFocus, u = u, nil
		}
		c.mu.Unlock()
		if u != nil {
			u()
		}
	}
	c.syncTicker()
}

// detachFocus снимает подписку и отсчёт: закрытый календарь не будит ни
// сеанс, ни рендер.
func (c *CalendarFlyout) detachFocus() {
	c.mu.Lock()
	u := c.unsubFocus
	c.unsubFocus = nil
	stop := c.stopTick
	c.stopTick = nil
	c.mu.Unlock()
	if u != nil {
		u()
	}
	if stop != nil {
		stop()
	}
}

// onFocusChanged — сеанс изменился (горутина потребителя): перерисовывается
// модуль, а отсчёт включается или выключается вместе с сеансом.
func (c *CalendarFlyout) onFocusChanged() {
	if !c.IsOpen() {
		return
	}
	c.syncTicker()
	c.invalidateFocus()
}

// invalidateFocus заявляет перерисовку модуля «Фокусировка».
func (c *CalendarFlyout) invalidateFocus() {
	if !c.IsOpen() || !c.win11() {
		return
	}
	l := c.w11Layout(c.rect())
	if !l.focus.Empty() {
		widget.InvalidateRect(l.focus)
	}
}

// tickEvery заводит повторяющийся вызов f каждые d: своим Ticker, а без него —
// цепочкой таймеров стандартной библиотеки (следующий взводится после вызова,
// медленный обработчик не копит очередь). Раз в секунду перерисовывается отсчёт
// идущего сеанса; это не анимация, «меньше движения» её не касается.
func (c *CalendarFlyout) tickEvery(d time.Duration, f func()) func() {
	if c.Ticker != nil {
		return c.Ticker(d, f)
	}
	var mu sync.Mutex
	var t *time.Timer
	stopped := false
	var fire func()
	fire = func() {
		mu.Lock()
		if stopped {
			mu.Unlock()
			return
		}
		mu.Unlock()
		f()
		mu.Lock()
		if !stopped {
			t = time.AfterFunc(d, fire)
		}
		mu.Unlock()
	}
	mu.Lock()
	t = time.AfterFunc(d, fire)
	mu.Unlock()
	return func() {
		mu.Lock()
		stopped = true
		if t != nil {
			t.Stop()
		}
		mu.Unlock()
	}
}

// syncTicker включает секундный отсчёт, пока календарь открыт и сеанс идёт, и
// выключает в остальное время.
func (c *CalendarFlyout) syncTicker() {
	s := c.FocusSession()
	want := c.IsOpen() && s != nil && c.win11() && s.State().Running
	c.mu.Lock()
	has := c.stopTick != nil
	var stop func()
	if !want && has {
		stop, c.stopTick = c.stopTick, nil
	}
	c.mu.Unlock()
	if stop != nil {
		stop()
		return
	}
	if want && !has {
		stopNew := c.tickEvery(time.Second, c.invalidateFocus)
		c.mu.Lock()
		if c.stopTick == nil {
			c.stopTick, stopNew = stopNew, nil
		}
		c.mu.Unlock()
		if stopNew != nil {
			stopNew()
		}
	}
}

// onOpenChanged — наблюдатель Flyout: открытие подписывает календарь на сеанс и
// просит фокус клавиатуры, закрытие — снимает подписку и отсчёт.
func (c *CalendarFlyout) onOpenChanged(open bool) {
	if open {
		c.attachFocus()
		if c.win11() {
			focusreq.Request(c)
		}
		return
	}
	c.detachFocus()
	c.mu.Lock()
	c.hot, c.focusStop = calStopNone, calStopNone
	c.mu.Unlock()
	if c.win11() {
		focusreq.Return(c)
	}
}

// ─── Рисование ───────────────────────────────────────────────────────────────

// w11Fonts — шрифты карточки.
type w11Fonts struct{ date, title, body, caption, heading ncText }

func (c *CalendarFlyout) w11Fonts() w11Fonts {
	base := c.themeStyle(calendarPartDay, theme.StateNormal).Font
	tm := c.Theme()
	return w11Fonts{
		date:    ncTextOf(themeFont(tm, ncFontHeading, base, true, 1.5)),
		title:   ncTextOf(themeFont(tm, ncFontTitle, base, true, 1.1)),
		body:    ncTextOf(base),
		caption: ncTextOf(themeFont(tm, ncFontCaption, base, false, 0.9)),
		heading: ncTextOf(themeFont(tm, ncFontHeading, base, true, 1.5)),
	}
}

// stopState — состояние стиля остановки: наведение, нажатие мышью (кнопки
// календаря срабатывают на нажатии, так что «нажата» не бывает) и фокус.
func (c *CalendarFlyout) stopState(id calStop, hot, focus calStop, ring bool) theme.State {
	return StateOf(hot == id, false, false, false, ring && focus == id)
}

func (c *CalendarFlyout) w11Draw(ctx widget.DrawContext, r image.Rectangle) {
	l := c.w11Layout(r)
	f := c.w11Fonts()
	c.mu.Lock()
	hot, focus := c.hot, c.focusStop
	c.mu.Unlock()
	ring := c.fs.FocusVisible()

	// Дата: «понедельник, 5 октября» и стрелка свернуть/развернуть.
	ds := c.themeStyle(calPartDate, theme.StateNormal)
	date := f.date.elide(dateHeader(c.Culture, c.now()), l.date.Dx())
	f.date.draw(ctx, date, l.date.Min.X, l.date.Min.Y+(l.date.Dy()-f.date.lineH())/2, ds.Text)
	c.drawNavButton(ctx, l.collapse, calStopCollapse, hot, focus, ring, func(st *theme.Style) {
		drawGlyphChevron(ctx, l.collapse, !c.effCollapsed(), st.Text, l.collapse.Dx()/calW11GlyphDiv)
	})

	if !c.effCollapsed() {
		ms := c.themeStyle(calPartMonth, theme.StateNormal)
		f.title.draw(ctx, f.title.elide(c.monthTitle(), l.prev.Min.X-l.month.Min.X-8),
			l.month.Min.X, l.month.Min.Y+(l.month.Dy()-f.title.lineH())/2, ms.Text)
		c.drawNavButton(ctx, l.prev, calStopPrev, hot, focus, ring, func(st *theme.Style) {
			drawGlyphChevronH(ctx, l.prev, true, st.Text, l.prev.Dx()/calW11GlyphDiv)
		})
		c.drawNavButton(ctx, l.next, calStopNext, hot, focus, ring, func(st *theme.Style) {
			drawGlyphChevronH(ctx, l.next, false, st.Text, l.next.Dx()/calW11GlyphDiv)
		})

		weekdayStyle := c.themeStyle(calendarPartWeekday, theme.StateNormal)
		colW := l.weekday.Dx() / 7
		for col, name := range c.weekdayNames() {
			cellR := image.Rect(l.weekday.Min.X+col*colW, l.weekday.Min.Y, l.weekday.Min.X+(col+1)*colW, l.weekday.Max.Y)
			DrawTextCentered(ctx, cellR, name, weekdayStyle)
		}

		today := c.now()
		selected := c.Selected()
		hasSelection := !selected.IsZero()
		hovered := c.hoveredDay()
		for _, row := range l.days {
			for _, cell := range row {
				st := StateOf(
					sameDay(cell.day.date, hovered),
					false,
					sameDay(cell.day.date, today),
					!cell.day.inMonth,
					hasSelection && sameDay(cell.day.date, selected),
				)
				// Ключ несёт и «в этом ли месяце»: то же число соседнего месяца при
				// листании меняет вид сразу, а не плавно.
				key := calendarDayKey{cell.day.date.Year(), cell.day.date.YearDay(), cell.day.inMonth}
				dr := c.dayRect(cell.rect, l.m.day)
				dayStyle := c.fade.Style(c.Theme(), key, dr, st, func(st theme.State) *theme.Style {
					return c.themeStyle(calendarPartDay, st)
				})
				PaintStyle(ctx, dr, dayStyle)
				DrawTextCentered(ctx, dr, strconv.Itoa(cell.day.date.Day()), dayStyle)
			}
		}
	}

	if !l.focus.Empty() {
		c.drawFocusModule(ctx, l, f, hot, focus, ring)
	}
	if ring && focus == calStopGrid {
		// Кольцо — на дне под клавиатурным курсором, а не вокруг всей сетки.
		kb := c.cursorDay()
		for _, row := range l.days {
			for _, cell := range row {
				if sameDay(cell.day.date, kb) {
					PaintFocusRing(ctx, c.dayRect(cell.rect, l.m.day), c.Theme(), c.themeStyle(calendarPartDay, theme.StateNormal))
				}
			}
		}
	} else if ring && focus != calStopNone {
		if rr := l.rectOf(focus); !rr.Empty() {
			PaintFocusRing(ctx, rr, c.Theme(), c.themeStyle(calPartFocusButton, theme.StateNormal))
		}
	}
}

// dayRect — круг дня в ячейке: квадрат со стороной side по центру ячейки.
func (c *CalendarFlyout) dayRect(cell image.Rectangle, side int) image.Rectangle {
	if side > cell.Dy() {
		side = cell.Dy()
	}
	if side > cell.Dx() {
		side = cell.Dx()
	}
	x := cell.Min.X + (cell.Dx()-side)/2
	y := cell.Min.Y + (cell.Dy()-side)/2
	return image.Rect(x, y, x+side, y+side)
}

// drawNavButton рисует квадратную кнопку: подложка по состоянию и фигура.
func (c *CalendarFlyout) drawNavButton(ctx widget.DrawContext, r image.Rectangle, id, hot, focus calStop, ring bool, glyph func(*theme.Style)) {
	st := c.themeStyle(calPartNav, c.stopState(id, hot, focus, ring))
	PaintStyle(ctx, r, st)
	glyph(st)
}

// drawFocusModule рисует модуль «Фокусировка»: разделитель, заголовок и либо
// выбор длительности с «Начать», либо отсчёт с полосой прогресса и «Остановить».
func (c *CalendarFlyout) drawFocusModule(ctx widget.DrawContext, l calW11Layout, f w11Fonts, hot, focus calStop, ring bool) {
	sess := c.FocusSession()
	if sess == nil {
		return
	}
	st := sess.State()
	div := c.themeStyle(calPartDivider, theme.StateNormal)
	if div.Fill.A > 0 {
		ctx.FillRectAlpha(l.divider.Min.X, l.divider.Min.Y, l.divider.Dx(), l.divider.Dy(), div.Fill)
	}
	ts := c.themeStyle(calPartFocusTitle, theme.StateNormal)
	f.title.draw(ctx, f.title.elide(tr(StrFocusTitle), l.ftitle.Dx()), l.ftitle.Min.X,
		l.ftitle.Min.Y+(l.ftitle.Dy()-f.title.lineH())/2, ts.Text)

	b := l.m.button
	// Правая кнопка: «Начать» (акцент) или «Остановить».
	label, part := focusActionLabel(st), calPartFocusAccent
	if st.Running {
		part = calPartFocusButton
	}
	action := l.faction
	as := c.themeStyle(part, c.stopState(calStopAction, hot, focus, ring))
	PaintStyle(ctx, action, as)
	f.body.draw(ctx, label, action.Min.X+(action.Dx()-f.body.width(label))/2,
		action.Min.Y+(action.Dy()-f.body.lineH())/2, as.Text)

	if !st.Running {
		less := c.themeStyle(calPartFocusButton, c.stopState(calStopLess, hot, focus, ring))
		more := c.themeStyle(calPartFocusButton, c.stopState(calStopMore, hot, focus, ring))
		PaintStyle(ctx, l.fless, less)
		PaintStyle(ctx, l.fmore, more)
		drawGlyphMinus(ctx, l.fless, less.Text, b/calW11GlyphDiv)
		drawGlyphPlus(ctx, l.fmore, more.Text, b/calW11GlyphDiv)
		vs := c.themeStyle(calPartFocusValue, theme.StateNormal)
		val := focusLabel(st.Duration)
		vw := f.title.width(val)
		f.title.draw(ctx, val, l.fvalue.Min.X+(l.fvalue.Dx()-vw)/2, l.fvalue.Min.Y+(l.fvalue.Dy()-f.title.lineH())/2, vs.Text)
		return
	}

	// Идёт сеанс: остаток крупно слева, под ним полоса прогресса.
	remain := focusRemaining(st, c.now())
	vs := c.themeStyle(calPartFocusValue, theme.StateNormal)
	f.heading.draw(ctx, focusCountdown(remain), l.fremain.Min.X, l.fremain.Min.Y+(l.fremain.Dy()-f.heading.lineH())/2, vs.Text)
	track := c.themeStyle(calPartProgress, theme.StateNormal)
	PaintStyle(ctx, l.fprogress, track)
	frac := 0.0
	if st.Duration > 0 {
		frac = 1 - float64(remain)/float64(st.Duration)
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	if w := int(float64(l.fprogress.Dx())*frac + 0.5); w > 0 {
		bar := image.Rect(l.fprogress.Min.X, l.fprogress.Min.Y, l.fprogress.Min.X+w, l.fprogress.Max.Y)
		PaintStyle(ctx, bar, c.themeStyle(calPartProgressBar, theme.StateNormal))
	}
}

// drawGlyphChevronH рисует горизонтальный шеврон шириной side в центре r:
// острием влево (left) либо вправо.
func drawGlyphChevronH(ctx widget.DrawContext, r image.Rectangle, left bool, col color.RGBA, side int) {
	if side < 3 || col.A == 0 {
		return
	}
	half := side / 2
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	for i := 0; i <= half; i++ {
		x := cx + half/2 - i
		if left {
			x = cx - half/2 + i
		}
		plotPx(ctx, x, cy-i, col)
		plotPx(ctx, x, cy+i, col)
	}
}

// drawGlyphMinus и drawGlyphPlus рисуют «−» и «+» стороной side в центре r.
func drawGlyphMinus(ctx widget.DrawContext, r image.Rectangle, col color.RGBA, side int) {
	if side < 3 || col.A == 0 {
		return
	}
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	ctx.FillRectAlpha(cx-side/2, cy, side, 1, col)
}

func drawGlyphPlus(ctx widget.DrawContext, r image.Rectangle, col color.RGBA, side int) {
	if side < 3 || col.A == 0 {
		return
	}
	drawGlyphMinus(ctx, r, col, side)
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	ctx.FillRectAlpha(cx, cy-side/2, 1, side, col)
}

// ─── Ввод ────────────────────────────────────────────────────────────────────

// w11Mouse — кнопка мыши в календаре Windows 11. Вернёт true, когда событие
// обработано; клик по кнопке или дню срабатывает на нажатии, как у плоского
// календаря.
func (c *CalendarFlyout) w11Mouse(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft || !c.IsOpen() {
		return false
	}
	outer := c.rect()
	pt := image.Pt(e.X, e.Y)
	if !pt.In(outer) {
		// Соседка по группе (центр уведомлений над календарём) — не «мимо».
		if e.Pressed && !c.ownsPoint(pt) {
			c.Close()
			return true
		}
		return false
	}
	if !e.Pressed {
		return true
	}
	if c.fs.NotePointer(e) {
		c.Invalidate()
	}
	focusreq.Request(c)
	l := c.w11Layout(outer)
	if id := l.stopAt(pt); id != calStopNone {
		c.activateStop(id)
		return true
	}
	for _, row := range l.days {
		for _, cell := range row {
			if pt.In(c.dayRect(cell.rect, l.m.day)) {
				c.selectDay(cell.day.date)
				return true
			}
		}
	}
	return true
}

// activateStop выполняет действие остановки.
func (c *CalendarFlyout) activateStop(id calStop) {
	switch id {
	case calStopCollapse:
		c.ToggleCollapsed()
	case calStopPrev:
		c.PrevMonth()
	case calStopNext:
		c.NextMonth()
	case calStopGrid:
		c.selectDay(c.cursorDay())
	case calStopLess:
		c.focusStep(-1)
	case calStopMore:
		c.focusStep(+1)
	case calStopAction:
		c.focusAction()
	}
}

// w11Move подсвечивает день и кнопку под курсором.
func (c *CalendarFlyout) w11Move(x, y int) {
	var day time.Time
	hot := calStopNone
	if c.IsOpen() && !widget.CursorIsNowhere(x, y) {
		pt := image.Pt(x, y)
		l := c.w11Layout(c.rect())
		hot = l.stopAt(pt)
	rows:
		for _, row := range l.days {
			for _, cell := range row {
				if pt.In(c.dayRect(cell.rect, l.m.day)) {
					day = cell.day.date
					break rows
				}
			}
		}
	}
	c.mu.Lock()
	changedDay := !sameDay(c.hovered, day)
	c.hovered = day
	oldHot := c.hot
	c.hot = hot
	c.mu.Unlock()
	if changedDay {
		c.Invalidate()
		return
	}
	if oldHot != hot {
		l := c.w11Layout(c.rect())
		widget.InvalidateRect(l.rectOf(oldHot))
		widget.InvalidateRect(l.rectOf(hot))
	}
}

// w11Key — клавиатура календаря Windows 11: Tab и стрелки ходят по кнопкам,
// Enter и пробел нажимают, Влево/Вправо и PgUp/PgDn листают месяцы. Вернёт true,
// когда клавиша обработана.
func (c *CalendarFlyout) w11Key(e widget.KeyEvent) bool {
	if !c.IsOpen() || !e.Pressed {
		return false
	}
	c.mu.Lock()
	onGrid := c.focusStop == calStopGrid
	c.mu.Unlock()
	if onGrid && e.Mod&(widget.ModCtrl|widget.ModAlt|widget.ModMeta) == 0 {
		// На сетке стрелки двигают день; месяцы листают PgUp/PgDn и кнопки ‹ ›.
		switch e.Code {
		case widget.KeyLeft:
			c.moveCursorDay(-1)
			return true
		case widget.KeyRight:
			c.moveCursorDay(+1)
			return true
		case widget.KeyUp:
			c.moveCursorDay(-7)
			return true
		case widget.KeyDown:
			c.moveCursorDay(+7)
			return true
		}
	}
	switch e.Code {
	case widget.KeyEscape:
		c.DismissOnEscape()
		return true
	case widget.KeyLeft, widget.KeyPageUp:
		if !c.effCollapsed() {
			c.PrevMonth()
		}
		return true
	case widget.KeyRight, widget.KeyPageDown:
		if !c.effCollapsed() {
			c.NextMonth()
		}
		return true
	case widget.KeyTab:
		delta := +1
		if e.Mod&widget.ModShift != 0 {
			delta = -1
		}
		c.moveStop(delta)
		return true
	case widget.KeyEnter, widget.KeySpace:
		if e.Repeat {
			return true
		}
		c.mu.Lock()
		id := c.focusStop
		c.mu.Unlock()
		if id != calStopNone {
			c.activateStop(id)
		}
		return true
	}
	return false
}

// moveStop переносит клавиатурный фокус на соседнюю кнопку. С последней Tab
// уходит в центр уведомлений над календарём (если он открыт), с первой Shift+Tab
// — тоже; без центра обход замыкается.
func (c *CalendarFlyout) moveStop(delta int) {
	if c.fs.noteKey() {
		c.Invalidate()
	}
	l := c.w11Layout(c.rect())
	stops := l.stops()
	if len(stops) == 0 {
		return
	}
	c.mu.Lock()
	cur := -1
	for i, s := range stops {
		if s.id == c.focusStop {
			cur = i
			break
		}
	}
	above := c.above
	c.mu.Unlock()

	next := cur + delta
	switch {
	case cur < 0 && delta > 0:
		next = 0
	case cur < 0:
		next = len(stops) - 1
	case next >= len(stops) || next < 0:
		if above != nil && above.IsOpen() && above.win11() {
			c.mu.Lock()
			old := c.focusStop
			c.focusStop = calStopNone
			c.mu.Unlock()
			widget.InvalidateRect(l.rectOf(old))
			above.takeFocus(delta < 0)
			return
		}
		next = (next + len(stops)) % len(stops)
	}
	c.mu.Lock()
	old := c.focusStop
	c.focusStop = stops[next].id
	if stops[next].id == calStopGrid {
		c.kbDay = time.Time{} // курсор встаёт на выбранный день, сегодня или 1-е число
	}
	c.mu.Unlock()
	widget.InvalidateRect(l.rectOf(old))
	widget.InvalidateRect(stops[next].rect)
}

// cursorDay — день под клавиатурным курсором сетки: выбранный, а без выбора
// сегодняшний, если он на показанном месяце, иначе первое число месяца.
func (c *CalendarFlyout) cursorDay() time.Time {
	c.mu.Lock()
	kb, sel, vm := c.kbDay, c.selected, c.viewMonth
	c.mu.Unlock()
	if !kb.IsZero() {
		return kb
	}
	in := func(d time.Time) bool { return d.Year() == vm.Year() && d.Month() == vm.Month() }
	if !sel.IsZero() && in(sel) {
		return sel
	}
	if now := c.now(); in(now) {
		return now
	}
	return vm
}

// moveCursorDay сдвигает клавиатурный курсор на delta дней; ушёл за показанный
// месяц — календарь листает на тот, где курсор оказался.
func (c *CalendarFlyout) moveCursorDay(delta int) {
	d := c.cursorDay().AddDate(0, 0, delta)
	c.mu.Lock()
	c.kbDay = d
	vm := c.viewMonth
	c.mu.Unlock()
	if d.Year() != vm.Year() || d.Month() != vm.Month() {
		c.relayout(func() {
			c.mu.Lock()
			c.viewMonth = firstOfMonth(d)
			c.mu.Unlock()
		})
		return
	}
	c.Invalidate()
}

// takeFocus принимает клавиатурный фокус от центра уведомлений: на первую или
// последнюю кнопку.
func (c *CalendarFlyout) takeFocus(last bool) {
	if !c.IsOpen() || !c.win11() {
		return
	}
	l := c.w11Layout(c.rect())
	stops := l.stops()
	if len(stops) == 0 {
		return
	}
	id := stops[0].id
	if last {
		id = stops[len(stops)-1].id
	}
	c.mu.Lock()
	c.focusStop = id
	c.mu.Unlock()
	c.fs.noteKey()
	focusreq.Request(c)
	c.Invalidate()
}

// ─── Клавиатурный фокус ──────────────────────────────────────────────────────

// SetFocused реализует widget.Focusable.
func (c *CalendarFlyout) SetFocused(focused bool) {
	if c.fs.Set(focused) {
		c.Invalidate()
	}
}

// IsFocused реализует widget.Focusable.
func (c *CalendarFlyout) IsFocused() bool { return c.fs.IsFocused() }

// FocusVisible — видна ли рамка фокуса (фокус пришёл с клавиатуры).
func (c *CalendarFlyout) FocusVisible() bool { return c.fs.FocusVisible() }

// TabIndex исключает календарь из обхода Tab, пока он закрыт или рисуется
// плоским.
func (c *CalendarFlyout) TabIndex() int {
	if !c.IsOpen() || !c.win11() {
		return -1
	}
	return 0
}

// AcceptsTab реализует widget.TabAcceptor: в открытом календаре Windows 11 Tab
// ходит по его кнопкам, а не уводит фокус из панели (выйти можно по Esc).
func (c *CalendarFlyout) AcceptsTab() bool { return c.IsOpen() && c.win11() }
