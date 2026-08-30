// calendarflyout.go — календарь, всплывающий по клику на часы панели задач.
//
// Панель сама не выбирает месяц, который видит пользователь: раскладка
// (сетка недель, положение стрелок, размер ячейки) читается из темы, а
// текущая дата — из Clock, который передаёт вызывающий код. Часы
// подставляют FakeClock в тестах и SystemClock в реальной оболочке — сам
// компонент разницы не видит и `time.Now()` не вызывает нигде, кроме
// зашитого фолбэка на случай отсутствующих часов (тот же приём, что и в
// clock.go: см. ClockItem.now()).
//
// Открытие/закрытие, клик мимо и Esc уже умеет Flyout (flyout.go) — здесь
// только содержимое: сетка дней и листание месяцев.
package desktop

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentCalendar — имя компонента для стилей темы.
const ComponentCalendar = "calendar"

// Ключи метрик темы, которыми управляется раскладка календаря.
const (
	// KeyCalendarCell — сторона (высота) одной ячейки: и строки дней недели,
	// и строки чисел месяца — они выровнены в одну сетку по столбцам.
	KeyCalendarCell theme.Key = "calendar.cell"
	// KeyCalendarWidth — ширина содержимого панели (7 столбцов сетки).
	KeyCalendarWidth theme.Key = "calendar.width"
	// KeyCalendarHeaderHeight — высота строки заголовка (месяц, год, стрелки
	// листания).
	KeyCalendarHeaderHeight theme.Key = "calendar.header.height"
)

// Части компонента "calendar" для GetStyle: не отдельные подкомпоненты, а
// разные роли внутри одной панели, каждая читает свой стиль вплоть до
// состояния (сегодняшний день и выбранный отличаются не частью, а
// состоянием — см. draw).
const (
	calendarPartHeader  = "header"
	calendarPartWeekday = "weekday"
	calendarPartDay     = "day"
)

// calendarArrowInsetDiv — доля стороны кнопки листания, на которую стрелка
// поджимается от края квадрата. Не пиксельный размер (магические размеры
// запрещены), а пропорция фигуры — тот же приём, что и в tray.go
// (batteryNubDiv, acStripeDiv и соседи).
const calendarArrowInsetDiv = 4

// Русские названия месяцев (именительный падеж) и сокращения дней недели.
// Компонент рабочего стола локализован на русский так же, как остальные
// строки пакета (см. fakes.go: "Сеть", ошибки StaticAppCatalog).
var calendarMonthNames = [...]string{
	"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
	"Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь",
}

var calendarWeekdayNames = [...]string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}

// CalendarFlyout — панель-календарь, всплывающая по клику на часы.
type CalendarFlyout struct {
	*Flyout

	clk Clock

	mu        sync.Mutex
	viewMonth time.Time // всегда 1-е число месяца, который сейчас показан
	selected  time.Time // нулевое значение — выбора нет
	// collapsed — сетка дней свёрнута, видна только строка с датой.
	//
	// Живёт между открытиями намеренно: свернувший календарь один раз обычно
	// хочет его свёрнутым и дальше.
	collapsed bool
	// hovered — день под курсором; нулевое значение — курсора на сетке нет.
	//
	// Хранится датой, а не индексом ячейки: месяц можно перелистнуть колесом
	// или стрелкой, не двигая мышь, и индекс после этого указывал бы на
	// другое число.
	hovered time.Time

	// OnSelect вызывается при выборе дня кликом (необязателен).
	OnSelect func(time.Time)
}

// NewCalendarFlyout создаёт календарь, оформляемый темой tm и читающий
// текущую дату из clk.
func NewCalendarFlyout(tm *theme.Manager, clk Clock) *CalendarFlyout {
	c := &CalendarFlyout{
		Flyout: NewFlyout(tm, ComponentCalendar),
		clk:    clk,
	}
	c.viewMonth = firstOfMonth(c.now())
	c.Content = c.draw
	c.Size = c.size
	return c
}

// now возвращает текущее время источника (SystemClock, если clk не задан —
// как и ClockItem.now(), это фолбэк на случай отсутствующих часов, а не
// обычный путь выполнения).
func (c *CalendarFlyout) now() time.Time {
	if c.clk == nil {
		return SystemClock{}.Now()
	}
	return c.clk.Now()
}

func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// Collapsed сообщает, свёрнута ли сетка дней.
func (c *CalendarFlyout) Collapsed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.collapsed
}

// SetCollapsed сворачивает или разворачивает сетку дней. Свёрнутая панель
// показывает только строку с датой и занимает ровно её высоту.
func (c *CalendarFlyout) SetCollapsed(v bool) {
	c.mu.Lock()
	changed := c.collapsed != v
	c.collapsed = v
	if v {
		// Свёрнутая сетка курсора не видит — иначе развёрнутая панель
		// показала бы подсвеченным день, над которым курсора давно нет.
		c.hovered = time.Time{}
	}
	c.mu.Unlock()
	if changed {
		c.Invalidate()
	}
}

// ToggleCollapsed переключает свёрнутость.
func (c *CalendarFlyout) ToggleCollapsed() { c.SetCollapsed(!c.Collapsed()) }

// PrevMonth и NextMonth листают показанный месяц. AddDate сам переносит год
// на границе декабря/января — специального кода для этого случая не нужно.
func (c *CalendarFlyout) PrevMonth() { c.shiftMonth(-1) }
func (c *CalendarFlyout) NextMonth() { c.shiftMonth(1) }

func (c *CalendarFlyout) shiftMonth(delta int) {
	c.mu.Lock()
	c.viewMonth = c.viewMonth.AddDate(0, delta, 0)
	c.mu.Unlock()
	c.Invalidate()
}

// ViewMonth возвращает месяц, который сейчас показан (всегда 1-е число).
func (c *CalendarFlyout) ViewMonth() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.viewMonth
}

// Selected возвращает выбранный день (нулевое время — выбора нет).
func (c *CalendarFlyout) Selected() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.selected
}

// selectDay запоминает выбранный день и уведомляет подписчика.
func (c *CalendarFlyout) selectDay(t time.Time) {
	c.mu.Lock()
	c.selected = t
	c.mu.Unlock()
	c.Invalidate()
	if c.OnSelect != nil {
		c.OnSelect(t)
	}
}

// themeStyle читает стиль части компонента "calendar" (переживает tm==nil,
// как и остальные компоненты пакета — см. trayStyle в tray.go).
func (c *CalendarFlyout) themeStyle(part string, st theme.State) *theme.Style {
	tm := c.Theme()
	if tm == nil {
		return &theme.Style{}
	}
	return tm.GetStyle(c.Component, part, st)
}

// monthTitle — строка заголовка вида «Август 2026».
func (c *CalendarFlyout) monthTitle() string {
	vm := c.ViewMonth()
	return fmt.Sprintf("%s %d", calendarMonthNames[vm.Month()-1], vm.Year())
}

// gridSnapshot строит сетку показанного месяца.
func (c *CalendarFlyout) gridSnapshot() [][]dayCell {
	vm := c.ViewMonth()
	return monthGrid(vm.Year(), vm.Month(), vm.Location())
}

// contentRect — прямоугольник содержимого: то же, что получает draw через
// Content (Flyout.DrawOverlay передаёт r.Inset(PadX)), но посчитанное
// заново для обработки кликов, у которых своего вызова draw нет.
func (c *CalendarFlyout) contentRect() image.Rectangle {
	r := c.rect()
	if r.Empty() {
		return image.Rectangle{}
	}
	pad := int(c.style(theme.StateNormal).PadX)
	return r.Inset(pad)
}

// size — Flyout.Size: желаемый размер панели. Высота зависит от числа
// недель в показанном месяце (4–6), поэтому при листании панель может
// слегка менять высоту — так же ведёт себя системный календарь Windows.
func (c *CalendarFlyout) size() image.Point {
	width := c.metric(KeyCalendarWidth)
	headerH := c.metric(KeyCalendarHeaderHeight)
	cell := c.metric(KeyCalendarCell)
	pad := int(c.style(theme.StateNormal).PadX)
	if c.Collapsed() {
		// Свёрнутая панель — это строка с датой и ничего больше. Без этого
		// всплывающее окно осталось бы прежней высоты и показывало пустоту.
		return image.Point{X: width + 2*pad, Y: headerH + 2*pad}
	}
	rows := len(c.gridSnapshot())
	return image.Point{
		X: width + 2*pad,
		Y: headerH + cell + rows*cell + 2*pad,
	}
}

// ─── Раскладка ───────────────────────────────────────────────────────────────

// dayCell — один день сетки: дата и признак принадлежности показанному
// месяцу (false — день соседнего месяца, дорисованный для полноты недели).
type dayCell struct {
	date    time.Time
	inMonth bool
}

// dayCellLayout — ячейка сетки вместе с её прямоугольником на экране.
// draw и OnMouseButton считают раскладку одной и той же функцией
// (computeLayout), поэтому дата под курсором клика гарантированно совпадает
// с датой, нарисованной в этом же прямоугольнике.
type dayCellLayout struct {
	rect image.Rectangle
	day  dayCell
}

type calendarLayout struct {
	header, prevBtn, nextBtn, collapseBtn, title, weekday image.Rectangle
	days                                     [][]dayCellLayout
}

// computeLayout раскладывает содержимое календаря внутри content: строку
// заголовка со стрелками, строку названий дней недели и сетку чисел.
func (c *CalendarFlyout) computeLayout(content image.Rectangle) calendarLayout {
	headerH := c.metric(KeyCalendarHeaderHeight)
	cell := c.metric(KeyCalendarCell)

	header := image.Rect(content.Min.X, content.Min.Y, content.Max.X, content.Min.Y+headerH)
	btnW := headerH
	if half := content.Dx() / 2; btnW > half {
		// Вырожденная тема с огромной высотой заголовка не должна свести
		// область названия месяца к отрицательной ширине.
		btnW = half
	}
	// Стрелка сворачивания стоит у правого края заголовка — там же, где её
	// держит Windows. Стрелки листания месяца при свёрнутой сетке не рисуются
	// и кликов не принимают: листать нечего.
	collapseBtn := image.Rect(header.Max.X-btnW, header.Min.Y, header.Max.X, header.Max.Y)
	var prevBtn, nextBtn, title image.Rectangle
	if c.Collapsed() {
		title = image.Rect(header.Min.X, header.Min.Y, collapseBtn.Min.X, header.Max.Y)
	} else {
		prevBtn = image.Rect(header.Min.X, header.Min.Y, header.Min.X+btnW, header.Max.Y)
		nextBtn = image.Rect(collapseBtn.Min.X-btnW, header.Min.Y, collapseBtn.Min.X, header.Max.Y)
		title = image.Rect(prevBtn.Max.X, header.Min.Y, nextBtn.Min.X, header.Max.Y)
	}

	if c.Collapsed() {
		return calendarLayout{header: header, collapseBtn: collapseBtn, title: title}
	}

	weekday := image.Rect(content.Min.X, header.Max.Y, content.Max.X, header.Max.Y+cell)

	colW := 0
	if content.Dx() > 0 {
		colW = content.Dx() / 7
	}
	grid := c.gridSnapshot()
	days := make([][]dayCellLayout, len(grid))
	for row := range grid {
		y0 := weekday.Max.Y + row*cell
		cells := make([]dayCellLayout, 7)
		for col := 0; col < 7; col++ {
			x0 := content.Min.X + col*colW
			cells[col] = dayCellLayout{
				rect: image.Rect(x0, y0, x0+colW, y0+cell),
				day:  grid[row][col],
			}
		}
		days[row] = cells
	}
	return calendarLayout{
		header: header, prevBtn: prevBtn, nextBtn: nextBtn, collapseBtn: collapseBtn,
		title: title, weekday: weekday, days: days,
	}
}

// ─── Отрисовка ───────────────────────────────────────────────────────────────

func (c *CalendarFlyout) draw(ctx widget.DrawContext, r image.Rectangle) {
	if r.Empty() {
		return
	}
	layout := c.computeLayout(r)

	headerStyle := c.themeStyle(calendarPartHeader, theme.StateNormal)
	arrowInk := ink(headerStyle)

	if c.Collapsed() {
		// Свёрнутая панель — строка с датой и стрелка, которая её развернёт.
		DrawTextCentered(ctx, layout.title, c.dateTitle(), headerStyle)
		// Уголок вниз — «развернуть». Фигура та же, что у трея (systemtray.go):
		// одна на пакет, чтобы уголки на панели не разошлись видом.
		drawChevron(ctx, layout.collapseBtn, false, arrowInk)
		return
	}

	DrawTextCentered(ctx, layout.title, c.monthTitle(), headerStyle)
	drawChevron(ctx, layout.collapseBtn, true, arrowInk) // вверх — «свернуть»
	drawArrow(ctx, layout.prevBtn, true, arrowInk)
	drawArrow(ctx, layout.nextBtn, false, arrowInk)

	weekdayStyle := c.themeStyle(calendarPartWeekday, theme.StateNormal)
	colW := 0
	if r.Dx() > 0 {
		colW = r.Dx() / 7
	}
	for col, name := range calendarWeekdayNames {
		cellR := image.Rect(r.Min.X+col*colW, layout.weekday.Min.Y, r.Min.X+(col+1)*colW, layout.weekday.Max.Y)
		DrawTextCentered(ctx, cellR, name, weekdayStyle)
	}

	today := c.now()
	selected := c.Selected()
	hasSelection := !selected.IsZero()
	hovered := c.hoveredDay()
	for _, row := range layout.days {
		for _, cell := range row {
			st := StateOf(
				sameDay(cell.day.date, hovered),
				false,
				sameDay(cell.day.date, today),
				!cell.day.inMonth,
				hasSelection && sameDay(cell.day.date, selected),
			)
			dayStyle := c.themeStyle(calendarPartDay, st)
			PaintStyle(ctx, cell.rect, dayStyle)
			DrawTextCentered(ctx, cell.rect, strconv.Itoa(cell.day.date.Day()), dayStyle)
		}
	}
}

// drawArrow рисует треугольник-стрелку внутри r цветом col: фигура, а не
// иконка — как значки трея в tray.go, пока в наборе темы нет настоящих
// глифов. pointLeft=true — вершина смотрит влево (лист назад), false —
// вправо (лист вперёд).
func drawArrow(ctx widget.DrawContext, r image.Rectangle, pointLeft bool, col color.RGBA) {
	insetX := r.Dx() / calendarArrowInsetDiv
	insetY := r.Dy() / calendarArrowInsetDiv
	rr := image.Rect(r.Min.X+insetX, r.Min.Y+insetY, r.Max.X-insetX, r.Max.Y-insetY)
	w, h := rr.Dx(), rr.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	mid := h / 2
	for y := 0; y < h; y++ {
		dist := y - mid
		if dist < 0 {
			dist = -dist
		}
		width := w * (mid - dist) / (mid + 1)
		if width <= 0 {
			continue
		}
		x := rr.Min.X
		if pointLeft {
			x = rr.Max.X - width
		}
		ctx.FillRect(x, rr.Min.Y+y, width, 1, col)
	}
}

// dateTitle — строка с датой для свёрнутой панели: «27 августа 2026».
func dateTitleFor(t time.Time) string {
	return strconv.Itoa(t.Day()) + " " + calendarMonthGenitive[int(t.Month())-1] +
		" " + strconv.Itoa(t.Year())
}

func (c *CalendarFlyout) dateTitle() string { return dateTitleFor(c.now()) }

// calendarMonthGenitive — месяцы в родительном падеже: «27 августа», а не
// «27 август». Отдельный список, потому что заголовок месяца («Август 2026»)
// требует именительного, и одним набором не обойтись.
var calendarMonthGenitive = [...]string{
	"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

// ─── Ввод ────────────────────────────────────────────────────────────────────

// OnMouseButton закрывает панель кликом мимо (как Flyout), а внутри неё
// обрабатывает клики по стрелкам листания и по числам дней.
//
// У календаря нет собственных дочерних виджетов — всё содержимое рисует
// draw вручную, — поэтому, в отличие от базового Flyout (там клик внутри
// не поглощается: его разбирают дочерние виджеты меню), здесь клик внутри
// панели поглощается целиком: иначе он "проваливался" бы сквозь панель к
// тому, что расположено под ней.
func (c *CalendarFlyout) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft || !c.IsOpen() {
		return false
	}
	outer := c.rect()
	pt := image.Pt(e.X, e.Y)
	if !pt.In(outer) {
		if e.Pressed {
			c.Close()
			return true
		}
		return false
	}
	if !e.Pressed {
		return true
	}

	layout := c.computeLayout(c.contentRect())
	if pt.In(layout.collapseBtn) {
		c.ToggleCollapsed()
		return true
	}
	if pt.In(layout.prevBtn) {
		c.PrevMonth()
		return true
	}
	if pt.In(layout.nextBtn) {
		c.NextMonth()
		return true
	}
	for _, row := range layout.days {
		for _, cell := range row {
			if pt.In(cell.rect) {
				c.selectDay(cell.day.date)
				return true
			}
		}
	}
	return true
}

// hoveredDay возвращает день под курсором (нулевое время — курсора нет).
func (c *CalendarFlyout) hoveredDay() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hovered
}

// OnMouseMove подсвечивает день под курсором.
//
// Без этого метода состояния StateHover и StateActive, которые профили честно
// задают для части «day», не могли примениться никогда: отрисовка передавала
// признак наведения ложью всегда, и заданная темой заливка при наведении была
// не видна.
func (c *CalendarFlyout) OnMouseMove(x, y int) {
	var day time.Time
	if c.IsOpen() && !widget.CursorIsNowhere(x, y) {
		pt := image.Pt(x, y)
		layout := c.computeLayout(c.contentRect())
	rows:
		for _, row := range layout.days {
			for _, cell := range row {
				if pt.In(cell.rect) {
					day = cell.day.date
					break rows
				}
			}
		}
	}

	c.mu.Lock()
	changed := !sameDay(c.hovered, day)
	c.hovered = day
	c.mu.Unlock()
	if changed {
		c.Invalidate()
	}
}

// OnKeyEvent листает месяцы стрелками влево/вправо, а закрытие по Esc
// оставляет базовому Flyout.
func (c *CalendarFlyout) OnKeyEvent(e widget.KeyEvent) {
	if c.IsOpen() && e.Pressed {
		switch e.Code {
		case widget.KeyLeft:
			c.PrevMonth()
			return
		case widget.KeyRight:
			c.NextMonth()
			return
		}
	}
	c.Flyout.OnKeyEvent(e)
}

// ─── Сетка месяца ────────────────────────────────────────────────────────────

// weekdayMondayIndex переводит воскресный отсчёт time.Weekday (Sunday=0) в
// европейский, где неделя начинается с понедельника (Monday=0).
func weekdayMondayIndex(t time.Time) int {
	return (int(t.Weekday()) + 6) % 7
}

// monthGrid строит полные недели месяца month года year: с понедельника
// недели, содержащей 1-е число, по воскресенье недели, содержащей последний
// день месяца. Дни соседних месяцев тоже попадают в сетку (inMonth=false) —
// иначе последняя неделя короткого месяца обрывалась бы неполным рядом
// вместо ровных семи столбцов.
//
// Неделя начинается с понедельника — европейский календарь выбран
// осознанно (это выбор, а не случайность): пакет ориентирован в первую
// очередь на профили Windows для российского и европейского рынка.
func monthGrid(year int, month time.Month, loc *time.Location) [][]dayCell {
	first := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	start := first.AddDate(0, 0, -weekdayMondayIndex(first))

	next := first.AddDate(0, 1, 0)
	last := next.AddDate(0, 0, -1)
	end := last.AddDate(0, 0, 6-weekdayMondayIndex(last))

	var rows [][]dayCell
	for d := start; !d.After(end); {
		row := make([]dayCell, 7)
		for col := 0; col < 7; col++ {
			row[col] = dayCell{date: d, inMonth: d.Month() == month}
			d = d.AddDate(0, 0, 1)
		}
		rows = append(rows, row)
	}
	return rows
}

// sameDay сравнивает дату (без времени суток).
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
