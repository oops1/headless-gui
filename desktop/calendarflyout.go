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
	rows := len(c.gridSnapshot())
	pad := int(c.style(theme.StateNormal).PadX)
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
	header, prevBtn, nextBtn, title, weekday image.Rectangle
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
	prevBtn := image.Rect(header.Min.X, header.Min.Y, header.Min.X+btnW, header.Max.Y)
	nextBtn := image.Rect(header.Max.X-btnW, header.Min.Y, header.Max.X, header.Max.Y)
	title := image.Rect(prevBtn.Max.X, header.Min.Y, nextBtn.Min.X, header.Max.Y)

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
	return calendarLayout{header: header, prevBtn: prevBtn, nextBtn: nextBtn, title: title, weekday: weekday, days: days}
}

// ─── Отрисовка ───────────────────────────────────────────────────────────────

func (c *CalendarFlyout) draw(ctx widget.DrawContext, r image.Rectangle) {
	if r.Empty() {
		return
	}
	layout := c.computeLayout(r)

	headerStyle := c.themeStyle(calendarPartHeader, theme.StateNormal)
	DrawTextCentered(ctx, layout.title, c.monthTitle(), headerStyle)
	arrowInk := ink(headerStyle)
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
	for _, row := range layout.days {
		for _, cell := range row {
			st := StateOf(false, false,
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
