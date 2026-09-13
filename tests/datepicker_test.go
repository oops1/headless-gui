package tests

import (
	"image"
	"testing"
	"testing/fstest"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// GG-67: поле даты с выпадающим календарём. Проверяется как им пользуется
// человек: набор с клавиатуры, календарь с клавиатуры и мышью, границы,
// культура языка и разметка.

func withLanguage(t *testing.T, code string) {
	t.Helper()
	prev := widget.Language()
	widget.SetLanguage(code)
	t.Cleanup(func() { widget.SetLanguage(prev) })
}

func fixedNow() time.Time { return time.Date(2026, time.September, 13, 15, 30, 0, 0, time.Local) }

func dateScene(t *testing.T, p *widget.DatePicker) *engine.Engine {
	t.Helper()
	p.Now = fixedNow
	root := widget.NewPanel(widget.CurrentTheme().WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 600, 400))
	p.SetBounds(image.Rect(20, 20, 220, 52))
	root.AddChild(p)
	eng := engine.New(600, 400, 30)
	eng.SetRoot(root)
	eng.SetFocus(p)
	return eng
}

func ymd(t time.Time) (int, time.Month, int) { return t.Date() }

func wantDate(t *testing.T, p *widget.DatePicker, y int, m time.Month, d int) {
	t.Helper()
	got, ok := p.SelectedDate()
	if !ok {
		t.Fatalf("дата не выбрана, ждал %04d-%02d-%02d (текст %q)", y, m, d, p.Text())
	}
	if gy, gm, gd := ymd(got); gy != y || gm != m || gd != d {
		t.Fatalf("выбрана %v, ждал %04d-%02d-%02d", got, y, m, d)
	}
	if h, mi, s := got.Clock(); h != 0 || mi != 0 || s != 0 {
		t.Fatalf("у даты осталось время суток: %v", got)
	}
}

// Выбранная дата показывается в формате языка и стирается.
func TestDatePicker_FormatByLanguage(t *testing.T) {
	withLanguage(t, "RU")
	p := widget.NewDatePicker()
	if !p.SetSelectedDate(fixedNow()) {
		t.Fatal("дату без границ выбрать не удалось")
	}
	wantDate(t, p, 2026, time.September, 13)
	if got := p.Text(); got != "13.09.2026" {
		t.Fatalf("RU: %q", got)
	}
	if p.FirstDayOfWeek() != time.Monday {
		t.Fatalf("RU: неделя с %v", p.FirstDayOfWeek())
	}

	widget.SetLanguage("EN")
	if got := p.Text(); got != "09/13/2026" {
		t.Fatalf("EN: %q", got)
	}
	if p.FirstDayOfWeek() != time.Sunday {
		t.Fatalf("EN: неделя с %v", p.FirstDayOfWeek())
	}

	var events []bool
	p.OnSelectedDateChanged = func(_ time.Time, ok bool) { events = append(events, ok) }
	p.ClearSelectedDate()
	if _, ok := p.SelectedDate(); ok || p.Text() != "" {
		t.Fatalf("после ClearSelectedDate: %q", p.Text())
	}
	if len(events) != 1 || events[0] {
		t.Fatalf("событие стирания: %v", events)
	}
}

// Набор даты с клавиатуры через движок: цифры, Enter — дата выбрана и событие
// пришло.
func TestDatePicker_TypeDate(t *testing.T) {
	withLanguage(t, "RU")
	p := widget.NewDatePicker()
	eng := dateScene(t, p)

	var got time.Time
	p.OnSelectedDateChanged = func(d time.Time, ok bool) { got = d }
	dvKeys(eng, dvRunes("01.02.2026")...)
	dvKeys(eng, dvKey(widget.KeyEnter, 0))

	wantDate(t, p, 2026, time.February, 1)
	if gy, gm, gd := ymd(got); gy != 2026 || gm != time.February || gd != 1 {
		t.Fatalf("событие принесло %v", got)
	}
}

// Набор терпим к виду: без ведущих нулей, другой разделитель, двузначный год,
// ISO 8601.
func TestDatePicker_TolerantParsing(t *testing.T) {
	withLanguage(t, "RU")
	for _, in := range []string{"1.2.2026", "01/02/2026", "1-2-26", "2026-02-01"} {
		p := widget.NewDatePicker()
		eng := dateScene(t, p)
		dvKeys(eng, dvRunes(in)...)
		dvKeys(eng, dvKey(widget.KeyEnter, 0))
		got, ok := p.SelectedDate()
		if !ok {
			t.Fatalf("%q не разобрано", in)
		}
		if gy, gm, gd := ymd(got); gy != 2026 || gm != time.February || gd != 1 {
			t.Fatalf("%q разобрано как %v", in, got)
		}
	}
}

// Неразобранный набор не меняет дату и помечает поле; уход фокуса возвращает
// выбранную дату, а не оставляет мусор.
func TestDatePicker_InvalidInput(t *testing.T) {
	withLanguage(t, "RU")
	p := widget.NewDatePicker()
	eng := dateScene(t, p)
	p.SetSelectedDate(fixedNow())

	dvKeys(eng, dvRunes("32.13.2026")...)
	dvKeys(eng, dvKey(widget.KeyEnter, 0))
	wantDate(t, p, 2026, time.September, 13)
	if p.Text() != "32.13.2026" {
		t.Fatalf("неразобранный текст пропал до исправления: %q", p.Text())
	}

	p.SetFocused(false)
	if p.Text() != "13.09.2026" {
		t.Fatalf("уход фокуса не вернул выбранную дату: %q", p.Text())
	}
}

// Границы: набор вне границ не принимается, выбранная дата вне новых границ
// стирается, календарь не выходит за границы.
func TestDatePicker_DisplayDateRange(t *testing.T) {
	withLanguage(t, "RU")
	p := widget.NewDatePicker()
	eng := dateScene(t, p)
	start := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, time.September, 30, 0, 0, 0, 0, time.Local)
	p.SetDisplayDateRange(start, end)

	if p.SetSelectedDate(time.Date(2026, time.October, 5, 0, 0, 0, 0, time.Local)) {
		t.Fatal("SetSelectedDate принял дату за границей")
	}
	dvKeys(eng, dvRunes("05.10.2026")...)
	dvKeys(eng, dvKey(widget.KeyEnter, 0))
	if _, ok := p.SelectedDate(); ok {
		t.Fatal("набор за границей выбрал дату")
	}

	p.SetSelectedDate(time.Date(2026, time.September, 20, 0, 0, 0, 0, time.Local))
	p.SetDisplayDateRange(start, time.Date(2026, time.September, 10, 0, 0, 0, 0, time.Local))
	if _, ok := p.SelectedDate(); ok {
		t.Fatal("дата вне новых границ не стёрта")
	}

	// Календарь: PgDn за конец границ месяц не листает.
	p.SetDropDownOpen(true)
	before := p.ViewMonth()
	dvKeys(eng, dvKey(widget.KeyPageDown, 0))
	if !p.ViewMonth().Equal(before) {
		t.Fatalf("календарь перелистнул за границу: %v → %v", before, p.ViewMonth())
	}
}

// Календарь с клавиатуры: Alt+↓ открывает на выбранной дате, стрелки двигают
// курсор, Enter выбирает и закрывает.
func TestDatePicker_CalendarKeyboard(t *testing.T) {
	withLanguage(t, "RU")
	p := widget.NewDatePicker()
	eng := dateScene(t, p)
	p.SetSelectedDate(fixedNow())

	dvKeys(eng, widget.KeyEvent{Code: widget.KeyDown, Mod: widget.ModAlt})
	if !p.IsDropDownOpen() {
		t.Fatal("Alt+↓ не открыл календарь")
	}
	dvKeys(eng, dvKey(widget.KeyRight, 0), dvKey(widget.KeyDown, 0)) // +1 день, +неделя
	dvKeys(eng, dvKey(widget.KeyEnter, 0))
	if p.IsDropDownOpen() {
		t.Fatal("Enter не закрыл календарь")
	}
	wantDate(t, p, 2026, time.September, 21)

	dvKeys(eng, dvKey(widget.KeyF4, 0), dvKey(widget.KeyPageUp, 0))
	if m := p.ViewMonth().Month(); m != time.August {
		t.Fatalf("PgUp показал %v", m)
	}
	dvKeys(eng, dvKey(widget.KeyEscape, 0))
	if p.IsDropDownOpen() {
		t.Fatal("Escape не закрыл календарь")
	}
	wantDate(t, p, 2026, time.September, 21)
}

// Календарь мышью: щелчок по кнопке открывает, щелчок по числу выбирает день
// показанного месяца и закрывает, Dismiss закрывает без выбора.
func TestDatePicker_CalendarMouse(t *testing.T) {
	withLanguage(t, "RU")
	p := widget.NewDatePicker()
	dateScene(t, p)
	field := p.BaseBounds()

	press := func(x, y int) {
		p.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	}
	press(field.Max.X-10, field.Min.Y+field.Dy()/2)
	if !p.IsDropDownOpen() {
		t.Fatal("щелчок по кнопке календаря его не открыл")
	}
	cal := p.OverlayBounds()
	if cal.Empty() || cal.Min.Y < field.Max.Y {
		t.Fatalf("календарь не под полем: %v (поле %v)", cal, field)
	}

	p.Dismiss()
	if p.IsDropDownOpen() {
		t.Fatal("Dismiss не закрыл календарь")
	}

	// Щелчок по середине сетки — это число сентября (без выбора календарь
	// открывается на месяце «сегодня»).
	press(field.Max.X-10, field.Min.Y+field.Dy()/2)
	press(cal.Min.X+cal.Dx()/2, cal.Min.Y+cal.Dy()/2+20)
	if p.IsDropDownOpen() {
		t.Fatal("выбор числа не закрыл календарь")
	}
	got, ok := p.SelectedDate()
	if !ok || got.Month() != time.September || got.Year() != 2026 {
		t.Fatalf("щелчок по сетке выбрал %v (ok=%v)", got, ok)
	}
}

// Разметка: тег, дата, границы, шаблон формата .NET, первый день недели.
func TestDatePicker_XAML(t *testing.T) {
	withLanguage(t, "RU")
	_, reg, err := widget.LoadUIFromXAMLFS([]byte(`<Canvas Width="400" Height="300">
  <DatePicker x:Name="d" SelectedDate="2026-09-13" DisplayDateStart="2026-01-01"
              DateFormat="yyyy/MM/dd" FirstDayOfWeek="Sunday"/>
</Canvas>`), fstest.MapFS{})
	if err != nil {
		t.Fatalf("разметка: %v", err)
	}
	p, ok := reg["d"].(*widget.DatePicker)
	if !ok {
		t.Fatalf("тег DatePicker не собрался: %T", reg["d"])
	}
	wantDate(t, p, 2026, time.September, 13)
	if p.Text() != "2026/09/13" {
		t.Fatalf("DateFormat: %q", p.Text())
	}
	if p.FirstDayOfWeek() != time.Sunday {
		t.Fatalf("FirstDayOfWeek: %v", p.FirstDayOfWeek())
	}
	if start, _ := p.DisplayDateRange(); start.Year() != 2026 || start.Month() != time.January {
		t.Fatalf("DisplayDateStart: %v", start)
	}
}

// Команда разметки получает выбранную дату.
func TestDatePicker_Command(t *testing.T) {
	p := widget.NewDatePicker()
	var got any
	if !p.SetCommand("SelectedDateChangedCommand", &widget.RelayCommand{ExecuteFn: func(v any) { got = v }}) {
		t.Fatal("SetCommand не принял SelectedDateChangedCommand")
	}
	p.SetSelectedDate(fixedNow())
	d, ok := got.(time.Time)
	if !ok || d.Day() != 13 {
		t.Fatalf("команда получила %#v", got)
	}
}
