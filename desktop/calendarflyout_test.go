package desktop

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// calendarThemeManager собирает минимальную тему для тестов календаря: все
// части компонента "calendar" и три его метрики. Своя, а не testThemeManager
// из tray_test.go — тот набор темы не знает о календаре, а дополнять чужой
// файл нельзя.
func calendarThemeManager(t *testing.T) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	p := theme.NewProfile("CalendarTest")

	// Отступ содержимого панели — читается Flyout.style() и используется и
	// DrawOverlay (движком), и contentRect() (обработкой кликов): оба обязаны
	// сойтись на одном и том же прямоугольнике.
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
	p.SetStyle(ComponentCalendar, calendarPartDay, theme.StateActive, theme.StyleDelta{
		Fill: theme.C(theme.RGB(0, 120, 215)),
		Text: theme.C(theme.RGB(255, 255, 255)),
	})
	p.SetStyle(ComponentCalendar, calendarPartDay, theme.StateFocused, theme.StyleDelta{
		Border: theme.C(theme.RGB(0, 120, 215)),
	})
	p.SetStyle(ComponentCalendar, calendarPartDay, theme.StateDisabled, theme.StyleDelta{
		Text: theme.C(theme.RGB(90, 90, 90)),
	})

	// Ширина кратна 7 — раскладка столбцов сетки без остатка от деления,
	// проще сравнивать координаты в тестах.
	p.SetMetric(KeyCalendarCell, 20)
	p.SetMetric(KeyCalendarWidth, 140)
	p.SetMetric(KeyCalendarHeaderHeight, 24)

	if err := m.RegisterTheme(p); err != nil {
		t.Fatalf("RegisterTheme: %v", err)
	}
	if err := m.SetTheme("CalendarTest"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	return m
}

// newTestCalendar создаёт календарь с большим экраном (чтобы fitInto не
// обрезал панель) и уже открытым.
func newTestCalendar(t *testing.T, at time.Time) (*CalendarFlyout, *FakeClock) {
	t.Helper()
	tm := calendarThemeManager(t)
	clk := NewFakeClock(at)
	c := NewCalendarFlyout(tm, clk)
	c.Screen = image.Rect(0, 0, 2000, 2000)
	c.Open(image.Rect(100, 100, 130, 130))
	return c, clk
}

// ─── Открытие/закрытие ───────────────────────────────────────────────────────

func TestCalendarFlyout_OpenClose(t *testing.T) {
	c, _ := newTestCalendar(t, time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))
	if !c.IsOpen() {
		t.Fatal("после Open панель не открыта")
	}
	c.Close()
	if c.IsOpen() {
		t.Error("после Close панель всё ещё открыта")
	}
}

func TestCalendarFlyout_EscCloses(t *testing.T) {
	c, _ := newTestCalendar(t, time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))
	c.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if c.IsOpen() {
		t.Error("Esc не закрыл панель")
	}
}

func TestCalendarFlyout_ClickOutsideCloses(t *testing.T) {
	c, _ := newTestCalendar(t, time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))
	r := c.rect()
	// Точка заведомо вне прямоугольника панели, но внутри экрана.
	far := image.Pt(r.Max.X+50, r.Max.Y+50)

	consumed := c.OnMouseButton(widget.MouseEvent{X: far.X, Y: far.Y, Button: widget.MouseLeft, Pressed: true})
	if !consumed {
		t.Error("клик мимо панели не поглощён")
	}
	if c.IsOpen() {
		t.Error("клик мимо панели её не закрыл")
	}
}

func TestCalendarFlyout_ClickInsideDoesNotClose(t *testing.T) {
	c, _ := newTestCalendar(t, time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))
	r := c.rect()
	center := r.Min.Add(image.Pt(r.Dx()/2, r.Dy()/2))

	c.OnMouseButton(widget.MouseEvent{X: center.X, Y: center.Y, Button: widget.MouseLeft, Pressed: true})
	if !c.IsOpen() {
		t.Error("клик внутри панели закрыл её — а должен был остаться внутри содержимого")
	}
}

// ─── Сетка месяца ────────────────────────────────────────────────────────────

// TestMonthGrid_KnownMonth — август 2026 года: 31 день, известная граница
// недель. Опорная точка (Weekday() у 1 и 31 августа) берётся из самого
// пакета time — так тест проверяет НАШУ раскладку по неделям, а не
// повторяет вручную вычисление дня недели.
func TestMonthGrid_KnownMonth(t *testing.T) {
	loc := time.UTC
	grid := monthGrid(2026, time.August, loc)

	if len(grid) == 0 {
		t.Fatal("пустая сетка")
	}
	for i, row := range grid {
		if len(row) != 7 {
			t.Fatalf("строка %d содержит %d ячеек, ждали 7", i, len(row))
		}
	}

	// Неделя начинается с понедельника.
	if grid[0][0].date.Weekday() != time.Monday {
		t.Errorf("первая ячейка сетки — %s, ждали понедельник", grid[0][0].date.Weekday())
	}
	last := grid[len(grid)-1][6]
	if last.date.Weekday() != time.Sunday {
		t.Errorf("последняя ячейка сетки — %s, ждали воскресенье", last.date.Weekday())
	}

	first := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	foundFirst := false
	inMonthCount := 0
	for _, row := range grid {
		for _, cell := range row {
			if cell.inMonth {
				inMonthCount++
			}
			if sameDay(cell.date, first) {
				foundFirst = true
				if !cell.inMonth {
					t.Error("1 августа помечено как день чужого месяца")
				}
			}
		}
	}
	if !foundFirst {
		t.Fatal("1 августа не найдено в сетке")
	}
	if inMonthCount != 31 {
		t.Errorf("дней своего месяца в сетке: %d, ждали 31 (в августе 31 день)", inMonthCount)
	}
}

// TestMonthGrid_NeighbourDaysAreDimmed — дни соседних месяцев в сетке есть
// (иначе последняя/первая неделя была бы неполной) и помечены inMonth=false.
func TestMonthGrid_NeighbourDaysAreDimmed(t *testing.T) {
	grid := monthGrid(2026, time.August, time.UTC)
	first := grid[0][0]
	if first.date.Month() == time.August && first.date.Day() == 1 {
		// 1 августа 2026 — суббота: понедельник этой недели уже в июле,
		// так что первая ячейка обязана быть днём июля. Если это не так —
		// проверка неактуальна для другой раскладки месяца, но именно для
		// августа 2026 она обязана выполняться.
		t.Fatal("первая ячейка сетки августа 2026 не должна быть 1 августа")
	}
	if first.inMonth {
		t.Error("день соседнего месяца в начале сетки помечен как inMonth=true")
	}
}

// ─── Листание месяцев ────────────────────────────────────────────────────────

func TestCalendarFlyout_NextMonthCrossesYearBoundary(t *testing.T) {
	c, _ := newTestCalendar(t, time.Date(2026, 12, 15, 10, 0, 0, 0, time.UTC))
	if got := c.ViewMonth(); got.Month() != time.December || got.Year() != 2026 {
		t.Fatalf("начальный месяц = %v, ждали декабрь 2026", got)
	}
	c.NextMonth()
	got := c.ViewMonth()
	if got.Month() != time.January || got.Year() != 2027 {
		t.Errorf("после NextMonth из декабря 2026 = %s %d, ждали январь 2027", got.Month(), got.Year())
	}
}

func TestCalendarFlyout_PrevMonthCrossesYearBoundary(t *testing.T) {
	c, _ := newTestCalendar(t, time.Date(2027, 1, 10, 10, 0, 0, 0, time.UTC))
	c.PrevMonth()
	got := c.ViewMonth()
	if got.Month() != time.December || got.Year() != 2026 {
		t.Errorf("после PrevMonth из января 2027 = %s %d, ждали декабрь 2026", got.Month(), got.Year())
	}
}

func TestCalendarFlyout_ArrowKeysChangeMonth(t *testing.T) {
	c, _ := newTestCalendar(t, time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))

	c.OnKeyEvent(widget.KeyEvent{Code: widget.KeyRight, Pressed: true})
	if got := c.ViewMonth(); got.Month() != time.September {
		t.Errorf("KeyRight не перелистнул вперёд: %s", got.Month())
	}
	c.OnKeyEvent(widget.KeyEvent{Code: widget.KeyLeft, Pressed: true})
	c.OnKeyEvent(widget.KeyEvent{Code: widget.KeyLeft, Pressed: true})
	if got := c.ViewMonth(); got.Month() != time.July {
		t.Errorf("KeyLeft не перелистнул назад: %s", got.Month())
	}
}

func TestCalendarFlyout_ArrowClickChangesMonth(t *testing.T) {
	c, _ := newTestCalendar(t, time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))
	layout := c.computeLayout(c.contentRect())
	next := layout.nextBtn.Min.Add(image.Pt(layout.nextBtn.Dx()/2, layout.nextBtn.Dy()/2))

	c.OnMouseButton(widget.MouseEvent{X: next.X, Y: next.Y, Button: widget.MouseLeft, Pressed: true})
	if got := c.ViewMonth(); got.Month() != time.September {
		t.Errorf("клик по стрелке «вперёд» не перелистнул месяц: %s", got.Month())
	}
}

// ─── Сегодня и выбор ─────────────────────────────────────────────────────────

func TestCalendarFlyout_HighlightsToday(t *testing.T) {
	today := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	c, _ := newTestCalendar(t, today)

	layout := c.computeLayout(c.contentRect())
	var todayRect image.Rectangle
	found := false
	for _, row := range layout.days {
		for _, cell := range row {
			if sameDay(cell.day.date, today) {
				todayRect = cell.rect
				found = true
			}
		}
	}
	if !found {
		t.Fatal("сегодняшний день не найден в раскладке")
	}

	ctx := &recCtx{}
	c.DrawOverlay(ctx)

	activeFill := theme.RGB(0, 120, 215)
	okFill := false
	for _, f := range ctx.fills {
		if f.x == todayRect.Min.X && f.y == todayRect.Min.Y && f.w == todayRect.Dx() && f.h == todayRect.Dy() && f.col == activeFill {
			okFill = true
		}
	}
	if !okFill {
		t.Errorf("не нашли заливку сегодняшней ячейки цветом активного состояния среди %+v", ctx.fills)
	}
}

func TestCalendarFlyout_ClickSelectsDay(t *testing.T) {
	c, _ := newTestCalendar(t, time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))
	layout := c.computeLayout(c.contentRect())

	// Берём любую ячейку своего месяца — не важно, какую именно.
	var target dayCellLayout
	found := false
	for _, row := range layout.days {
		for _, cell := range row {
			if cell.day.inMonth {
				target = cell
				found = true
			}
		}
	}
	if !found {
		t.Fatal("не нашли ни одной ячейки своего месяца")
	}
	pt := target.rect.Min.Add(image.Pt(target.rect.Dx()/2, target.rect.Dy()/2))

	var selectedVia time.Time
	c.OnSelect = func(d time.Time) { selectedVia = d }

	c.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})

	if !sameDay(c.Selected(), target.day.date) {
		t.Errorf("Selected() = %v, ждали %v", c.Selected(), target.day.date)
	}
	if !sameDay(selectedVia, target.day.date) {
		t.Errorf("OnSelect вызван с %v, ждали %v", selectedVia, target.day.date)
	}
}

// ─── Устойчивость без темы ───────────────────────────────────────────────────

func TestCalendarFlyout_NilThemeManagerDoesNotPanic(t *testing.T) {
	clk := NewFakeClock(time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))
	c := NewCalendarFlyout(nil, clk)
	c.Screen = image.Rect(0, 0, 2000, 2000)
	c.Open(image.Rect(100, 100, 130, 130))

	ctx := &recCtx{}
	c.DrawOverlay(ctx)
	c.NextMonth()
	c.OnMouseButton(widget.MouseEvent{X: 0, Y: 0, Button: widget.MouseLeft, Pressed: true})
}
