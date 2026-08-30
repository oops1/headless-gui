package desktop

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Наведение на день календаря.
//
// Отрисовка дня передавала признак наведения ложью ВСЕГДА, а OnMouseMove у
// календаря не было вовсе. Значит стиль StateHover, который профили честно
// задают для части «day», не мог примениться никогда: тема его описывала, а
// увидеть его было нельзя.

var calendarHoverFill = theme.RGB(70, 70, 70)

// calendarHoverTheme — как calendarThemeManager, но с заливкой дня при
// наведении: именно её видимость и проверяется.
func calendarHoverTheme(t *testing.T) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	p := theme.NewProfile("CalendarHoverTest")

	p.SetStyle(ComponentCalendar, "", theme.StateNormal, theme.StyleDelta{PadX: theme.N(4)})
	p.SetStyle(ComponentCalendar, calendarPartHeader, theme.StateNormal, theme.StyleDelta{
		Text: theme.C(theme.RGB(230, 230, 230)),
	})
	p.SetStyle(ComponentCalendar, calendarPartWeekday, theme.StateNormal, theme.StyleDelta{
		Text: theme.C(theme.RGB(160, 160, 160)),
	})
	p.SetStyle(ComponentCalendar, calendarPartDay, theme.StateNormal, theme.StyleDelta{
		Text: theme.C(theme.RGB(220, 220, 220)),
	})
	p.SetStyle(ComponentCalendar, calendarPartDay, theme.StateHover, theme.StyleDelta{
		Fill: theme.C(calendarHoverFill),
	})

	p.SetMetric(KeyCalendarCell, 20)
	p.SetMetric(KeyCalendarWidth, 140)
	p.SetMetric(KeyCalendarHeaderHeight, 24)

	if err := m.RegisterTheme(p); err != nil {
		t.Fatalf("RegisterTheme: %v", err)
	}
	if err := m.SetTheme("CalendarHoverTest"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	return m
}

func newHoverCalendar(t *testing.T) *CalendarFlyout {
	t.Helper()
	c := NewCalendarFlyout(calendarHoverTheme(t), NewFakeClock(
		time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)))
	c.Screen = image.Rect(0, 0, 2000, 2000)
	c.Open(image.Rect(100, 100, 130, 130))
	return c
}

// anyDayCell возвращает ячейку своего месяца — какую именно, не важно.
func anyDayCell(t *testing.T, c *CalendarFlyout) dayCellLayout {
	t.Helper()
	for _, row := range c.computeLayout(c.contentRect()).days {
		for _, cell := range row {
			if cell.day.inMonth {
				return cell
			}
		}
	}
	t.Fatal("не нашли ни одной ячейки своего месяца")
	return dayCellLayout{}
}

// filledWith сообщает, была ли ячейка залита цветом col.
func filledWith(ctx *recCtx, r image.Rectangle, col color.RGBA) bool {
	for _, f := range ctx.fills {
		if f.x == r.Min.X && f.y == r.Min.Y && f.w == r.Dx() && f.h == r.Dy() && f.col == col {
			return true
		}
	}
	return false
}

func TestCalendarFlyout_HoveredDayIsHighlighted(t *testing.T) {
	c := newHoverCalendar(t)
	cell := anyDayCell(t, c)
	mid := cell.rect.Min.Add(image.Pt(cell.rect.Dx()/2, cell.rect.Dy()/2))

	// До наведения заливки нет.
	before := &recCtx{}
	c.DrawOverlay(before)
	if filledWith(before, cell.rect, calendarHoverFill) {
		t.Fatal("ячейка залита цветом наведения до того, как на неё навели")
	}

	c.OnMouseMove(mid.X, mid.Y)

	after := &recCtx{}
	c.DrawOverlay(after)
	if !filledWith(after, cell.rect, calendarHoverFill) {
		t.Errorf("день под курсором не подсвечен: заливки %+v", after.fills)
	}
}

// Курсор ушёл — подсветка снялась. В том числе когда он ушёл под другой
// оверлей и движок прислал CursorNowhere.
func TestCalendarFlyout_HoverClearsWhenCursorLeaves(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y int
	}{
		{"курсор в стороне", 5, 5},
		{"курсор под другим оверлеем", widget.CursorNowhere, widget.CursorNowhere},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newHoverCalendar(t)
			cell := anyDayCell(t, c)
			mid := cell.rect.Min.Add(image.Pt(cell.rect.Dx()/2, cell.rect.Dy()/2))

			c.OnMouseMove(mid.X, mid.Y)
			c.OnMouseMove(tc.x, tc.y)

			ctx := &recCtx{}
			c.DrawOverlay(ctx)
			if filledWith(ctx, cell.rect, calendarHoverFill) {
				t.Error("подсветка дня осталась после ухода курсора")
			}
		})
	}
}

// Закрытая панель наведение не запоминает: открывшись, она не должна
// показывать подсвеченным день, над которым курсора давно нет.
func TestCalendarFlyout_ClosedPanelTakesNoHover(t *testing.T) {
	c := newHoverCalendar(t)
	cell := anyDayCell(t, c)
	mid := cell.rect.Min.Add(image.Pt(cell.rect.Dx()/2, cell.rect.Dy()/2))

	c.Close()
	c.OnMouseMove(mid.X, mid.Y)
	c.Open(image.Rect(100, 100, 130, 130))

	ctx := &recCtx{}
	c.DrawOverlay(ctx)
	if filledWith(ctx, cell.rect, calendarHoverFill) {
		t.Error("закрытая панель запомнила наведение и показала его при открытии")
	}
}

// Сворачивание сетки дней.
//
// В Windows у панели над месяцем есть стрелка: она убирает сетку, оставляя
// строку с датой. Развёрнутый календарь занимает большую часть высоты экрана,
// и когда нужен только список уведомлений, месяц мешает.

func TestCalendarFlyout_CollapseShrinksThePanel(t *testing.T) {
	c := newHoverCalendar(t)

	full := c.Size()
	if full.Y <= 0 {
		t.Fatalf("развёрнутая панель нулевой высоты: %v", full)
	}

	c.SetCollapsed(true)
	small := c.Size()

	if small.Y >= full.Y {
		t.Errorf("свёрнутая панель высотой %d против %d развёрнутой — окно оставит пустоту",
			small.Y, full.Y)
	}
	if small.X != full.X {
		t.Errorf("ширина изменилась при сворачивании: %d вместо %d", small.X, full.X)
	}

	// Сетка дней не рисуется вовсе.
	ctx := &recCtx{}
	c.DrawOverlay(ctx)
	for _, tx := range ctx.texts {
		for _, name := range calendarWeekdayNames {
			if tx.text == name {
				t.Errorf("у свёрнутой панели нарисован день недели %q", tx.text)
			}
		}
	}
}

// Стрелка в заголовке сворачивает и разворачивает.
func TestCalendarFlyout_ChevronTogglesCollapse(t *testing.T) {
	c := newHoverCalendar(t)

	click := func() {
		btn := c.computeLayout(c.contentRect()).collapseBtn
		if btn.Empty() {
			t.Fatal("в заголовке нет стрелки сворачивания")
		}
		c.OnMouseButton(widget.MouseEvent{
			X: btn.Min.X + btn.Dx()/2, Y: btn.Min.Y + btn.Dy()/2,
			Button: widget.MouseLeft, Pressed: true,
		})
	}

	click()
	if !c.Collapsed() {
		t.Fatal("стрелка не свернула календарь")
	}
	click()
	if c.Collapsed() {
		t.Error("повторное нажатие не развернуло календарь")
	}
}

// Состояние переживает закрытие и повторное открытие: свернувший календарь
// один раз обычно хочет его свёрнутым и дальше.
func TestCalendarFlyout_CollapseSurvivesReopen(t *testing.T) {
	c := newHoverCalendar(t)
	c.SetCollapsed(true)

	c.Close()
	c.Open(image.Rect(100, 100, 130, 130))

	if !c.Collapsed() {
		t.Error("после закрытия и открытия календарь развернулся сам")
	}
}

// Свёрнутая панель не листает месяцы: листать нечего, и клик по месту, где
// были стрелки, не должен менять показанный месяц.
func TestCalendarFlyout_CollapsedDoesNotPageMonths(t *testing.T) {
	c := newHoverCalendar(t)
	expandedLayout := c.computeLayout(c.contentRect())
	prev := expandedLayout.prevBtn
	if prev.Empty() {
		t.Fatal("у развёрнутой панели нет стрелки листания")
	}
	before := c.ViewMonth()

	c.SetCollapsed(true)
	c.OnMouseButton(widget.MouseEvent{
		X: prev.Min.X + prev.Dx()/2, Y: prev.Min.Y + prev.Dy()/2,
		Button: widget.MouseLeft, Pressed: true,
	})

	if !c.ViewMonth().Equal(before) {
		t.Errorf("свёрнутая панель перелистнула месяц с %v на %v", before, c.ViewMonth())
	}
}
