package widget

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/internal/calendar"
)

// datepicker.go — поле даты с выпадающим месячным календарём (WPF DatePicker).
//
// Дату можно набрать с клавиатуры или выбрать в календаре. Формат и первый
// день недели — культура языка интерфейса: берутся из таблиц строк
// (date.format, date.firstDay), поэтому приложение, добавившее свой язык,
// добавляет и его культуру теми же RegisterStrings. Задать их явно —
// SetFormat и SetFirstDayOfWeek.
//
// Дата хранится без времени суток (полночь выбранного дня): фильтр «с такого-то
// по такое-то», построенный на выборе, не должен зависеть от часа, в который
// человек щёлкнул по числу.

// DatePicker — поле даты с выпадающим календарём.
type DatePicker struct {
	Base
	mu sync.Mutex

	selected   time.Time // полночь выбранного дня; нулевое — даты нет
	start, end time.Time // DisplayDateStart/End; нулевое — без границы
	view       time.Time // первое число показанного в календаре месяца
	cursor     time.Time // день под клавиатурным курсором календаря

	hover   time.Time // день под мышью; нулевое — мыши на сетке нет
	hoverAt int       // что под мышью в шапке календаря (dpHit*)
	open    bool
	focused bool

	text      []rune // набираемый текст; актуален, пока editing
	editing   bool
	selectAll bool // первый символ заменяет весь текст
	invalid   bool // набранное не разобралось или вне границ

	format   string // раскладка time.Format; пусто — по языку
	firstDay time.Weekday
	firstSet bool

	// FontSize — кегль поля и календаря; 0 — общий размер.
	FontSize float64
	// Placeholder — подсказка в пустом поле; пусто — date.placeholder языка.
	Placeholder string
	// Now — источник «сегодня» (отметка в календаре и месяц пустого поля);
	// nil — time.Now. Задаётся в тестах и эталонных кадрах.
	Now func() time.Time

	pal      dpPalette
	commands map[string]ICommand

	// OnSelectedDateChanged вызывается при смене даты: ok=false — дату стёрли.
	// Зовётся без замков контрола.
	OnSelectedDateChanged func(date time.Time, ok bool)
}

// dpPalette — цвета поля и календаря из темы.
type dpPalette struct {
	bg, border, focus, text, placeholder, sel          color.RGBA
	dropBG, dropBorder, dropText, muted, disabled      color.RGBA
	hoverBG, hoverText, accent, accentText, err, glyph color.RGBA
}

// NewDatePicker создаёт поле даты без выбранной даты.
func NewDatePicker() *DatePicker {
	p := &DatePicker{hoverAt: dpHitNone}
	if t := CurrentTheme(); t != nil {
		p.pal = dpPaletteFrom(t)
	} else {
		p.pal = dpPaletteFrom(Win11LightTheme())
	}
	p.view = calendar.FirstOfMonth(p.now())
	p.cursor = calendar.DateOnly(p.now())
	return p
}

func (p *DatePicker) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// ─── Выбор даты ─────────────────────────────────────────────────────────────

// SelectedDate возвращает выбранную дату; ok=false — даты нет.
func (p *DatePicker) SelectedDate() (date time.Time, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.selected, !p.selected.IsZero()
}

// SetSelectedDate выбирает дату. Время суток отбрасывается. Дата вне границ
// (SetDisplayDateRange) не выбирается — возвращается false.
func (p *DatePicker) SetSelectedDate(date time.Time) bool {
	ok := false
	p.change(func() {
		d := calendar.DateOnly(date)
		if !p.inRangeLocked(d) {
			return
		}
		ok = true
		p.setLocked(d)
	})
	return ok
}

// ClearSelectedDate стирает дату.
func (p *DatePicker) ClearSelectedDate() {
	p.change(func() { p.setLocked(time.Time{}) })
}

// setLocked ставит дату (нулевая — стереть) и приводит поле в соответствие.
func (p *DatePicker) setLocked(d time.Time) {
	p.selected = d
	p.editing, p.invalid = false, false
	// В фокусе первый набранный символ заменяет дату целиком — как сразу после
	// получения фокуса. Иначе набор дописывался бы к только что выбранной дате.
	p.selectAll = p.focused
	if !d.IsZero() {
		p.view = calendar.FirstOfMonth(d)
		p.cursor = d
	}
}

// change выполняет fn под замком и, если дата сменилась, сообщает об этом уже
// без замка: обработчик вправе звать методы поля.
func (p *DatePicker) change(fn func()) {
	p.mu.Lock()
	before := p.selected
	wasOpen := p.open
	fn()
	after := p.selected
	open := p.open
	cb := p.OnSelectedDateChanged
	cmd := p.commands["SelectedDateChangedCommand"]
	p.mu.Unlock()

	if open || wasOpen {
		notifyUIChanged() // календарь лежит вне границ поля
	} else {
		p.Invalidate()
	}
	if before.Equal(after) {
		return
	}
	if cb != nil {
		cb(after, !after.IsZero())
	}
	if cmd != nil && cmd.CanExecute(after) {
		cmd.Execute(after)
	}
}

// SetDisplayDateRange задаёт границы выбора: даты вне [start, end] в календаре
// не выбираются и набором не принимаются. Нулевая граница — без ограничения.
// Выбранная дата вне новых границ стирается.
func (p *DatePicker) SetDisplayDateRange(start, end time.Time) {
	p.change(func() {
		p.start, p.end = time.Time{}, time.Time{}
		if !start.IsZero() {
			p.start = calendar.DateOnly(start)
		}
		if !end.IsZero() {
			p.end = calendar.DateOnly(end)
		}
		if !p.selected.IsZero() && !p.inRangeLocked(p.selected) {
			p.setLocked(time.Time{})
		}
	})
}

// DisplayDateRange возвращает границы выбора (нулевые — без ограничения).
func (p *DatePicker) DisplayDateRange() (start, end time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.start, p.end
}

func (p *DatePicker) inRangeLocked(d time.Time) bool {
	if !p.start.IsZero() && d.Before(p.start) {
		return false
	}
	if !p.end.IsZero() && d.After(p.end) {
		return false
	}
	return true
}

// ─── Культура ───────────────────────────────────────────────────────────────

// SetFormat задаёт формат даты раскладкой time.Format ("02.01.2006"). Пусто —
// формат языка интерфейса (ключ date.format).
func (p *DatePicker) SetFormat(layout string) {
	p.mu.Lock()
	p.format = layout
	p.mu.Unlock()
	p.Invalidate()
}

// Format возвращает действующий формат даты.
func (p *DatePicker) Format() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.layoutLocked()
}

// SetFirstDayOfWeek задаёт первый столбец календаря поверх культуры языка.
func (p *DatePicker) SetFirstDayOfWeek(d time.Weekday) {
	p.mu.Lock()
	p.firstDay, p.firstSet = d, true
	p.mu.Unlock()
	notifyUIChanged()
}

// FirstDayOfWeek возвращает действующий первый день недели.
func (p *DatePicker) FirstDayOfWeek() time.Weekday {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.firstDayLocked()
}

// layoutLocked — формат: явный, иначе из таблицы строк языка. Язык без своей
// культуры получает ISO 8601 — единственный формат, который не перепутает
// день с месяцем ни в одной стране.
func (p *DatePicker) layoutLocked() string {
	if p.format != "" {
		return p.format
	}
	if f := Tr("date.format"); strings.Contains(f, "2006") {
		return f
	}
	return "2006-01-02"
}

func (p *DatePicker) firstDayLocked() time.Weekday {
	if p.firstSet {
		return p.firstDay
	}
	if n, err := strconv.Atoi(Tr("date.firstDay")); err == nil && n >= 0 && n <= 6 {
		return time.Weekday(n)
	}
	return time.Monday
}

// Text — то, что сейчас показывает поле: набираемый текст или выбранная дата в
// формате культуры (пусто, если даты нет).
func (p *DatePicker) Text() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.textLocked()
}

func (p *DatePicker) textLocked() string {
	if p.editing {
		return string(p.text)
	}
	if p.selected.IsZero() {
		return ""
	}
	return p.selected.Format(p.layoutLocked())
}

// parseDateInput разбирает набранную дату. Кроме точного формата принимаются:
// числа без ведущих нулей, двузначный год, любой из разделителей «. / -» и
// ISO 8601 — человек, набирающий дату, не обязан попадать в формат до знака.
func parseDateInput(s, layout string, loc *time.Location) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	sep := rune(0)
	for _, r := range layout {
		if r < '0' || r > '9' {
			sep = r
			break
		}
	}
	norm := s
	if sep != 0 {
		norm = strings.Map(func(r rune) rune {
			if r == '.' || r == '/' || r == '-' {
				return sep
			}
			return r
		}, s)
	}
	short := strings.NewReplacer("02", "2", "01", "1").Replace(layout)
	for _, l := range []string{layout, short,
		strings.Replace(layout, "2006", "06", 1), strings.Replace(short, "2006", "06", 1)} {
		if t, err := time.ParseInLocation(l, norm, loc); err == nil {
			return t, true
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// commitLocked принимает набранный текст: пустой стирает дату, разобранный и
// попадающий в границы — выбирает, иначе поле помечается ошибкой и текст
// остаётся, чтобы его поправить.
func (p *DatePicker) commitLocked() {
	if !p.editing {
		return
	}
	s := strings.TrimSpace(string(p.text))
	if s == "" {
		p.setLocked(time.Time{})
		return
	}
	loc := time.Local
	if !p.selected.IsZero() {
		loc = p.selected.Location()
	}
	d, ok := parseDateInput(s, p.layoutLocked(), loc)
	if !ok || !p.inRangeLocked(calendar.DateOnly(d)) {
		p.invalid = true
		return
	}
	p.setLocked(calendar.DateOnly(d))
}

// revertLocked отменяет набор: поле снова показывает выбранную дату.
func (p *DatePicker) revertLocked() {
	p.editing, p.invalid = false, false
	p.selectAll = p.focused
	p.text = nil
}

// ─── Календарь ──────────────────────────────────────────────────────────────

// IsDropDownOpen сообщает, открыт ли календарь.
func (p *DatePicker) IsDropDownOpen() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.open
}

// SetDropDownOpen открывает или закрывает календарь.
func (p *DatePicker) SetDropDownOpen(v bool) {
	p.change(func() { p.setOpenLocked(v) })
}

func (p *DatePicker) setOpenLocked(v bool) {
	if p.open == v {
		return
	}
	p.open = v
	p.hover, p.hoverAt = time.Time{}, dpHitNone
	if !v {
		return
	}
	// Календарь открывается на выбранной дате, а без неё — на сегодняшней.
	p.cursor = calendar.DateOnly(p.now())
	if !p.selected.IsZero() {
		p.cursor = p.selected
	}
	p.cursor = p.clampLocked(p.cursor)
	p.view = calendar.FirstOfMonth(p.cursor)
}

// ViewMonth — первое число месяца, показанного в календаре.
func (p *DatePicker) ViewMonth() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.view
}

// clampLocked прижимает день к границам выбора.
func (p *DatePicker) clampLocked(d time.Time) time.Time {
	if !p.start.IsZero() && d.Before(p.start) {
		return p.start
	}
	if !p.end.IsZero() && d.After(p.end) {
		return p.end
	}
	return d
}

// canShiftLocked — можно ли перелистнуть месяц: в соседнем месяце должен быть
// хоть один выбираемый день.
func (p *DatePicker) canShiftLocked(delta int) bool {
	next := p.view.AddDate(0, delta, 0)
	if delta < 0 && !p.start.IsZero() {
		return !next.AddDate(0, 1, -1).Before(p.start)
	}
	if delta > 0 && !p.end.IsZero() {
		return !next.After(p.end)
	}
	return true
}

func (p *DatePicker) shiftMonthLocked(delta int) {
	if !p.canShiftLocked(delta) {
		return
	}
	p.view = p.view.AddDate(0, delta, 0)
	p.cursor = p.clampLocked(p.view)
}

// moveCursorLocked двигает клавиатурный курсор на days дней; месяц
// перелистывается следом.
func (p *DatePicker) moveCursorLocked(days int) {
	next := p.clampLocked(p.cursor.AddDate(0, 0, days))
	p.cursor = next
	p.view = calendar.FirstOfMonth(next)
}

// ─── Прочее ─────────────────────────────────────────────────────────────────

// SetCommand привязывает команду разметки: SelectedDateChangedCommand —
// параметр time.Time (нулевое — дату стёрли). Возвращает false для
// неизвестного имени.
func (p *DatePicker) SetCommand(name string, cmd ICommand) bool {
	if name != "SelectedDateChangedCommand" {
		return false
	}
	p.mu.Lock()
	if p.commands == nil {
		p.commands = map[string]ICommand{}
	}
	p.commands[name] = cmd
	p.mu.Unlock()
	return true
}

// ApplyTheme — контракт Themeable.
func (p *DatePicker) ApplyTheme(t *Theme) {
	p.mu.Lock()
	p.pal = dpPaletteFrom(t)
	p.mu.Unlock()
	p.Invalidate()
}

func dpPaletteFrom(t *Theme) dpPalette {
	or := func(c, def color.RGBA) color.RGBA {
		if c.A == 0 {
			return def
		}
		return c
	}
	var p dpPalette
	p.bg = or(t.InputBG, dvRGB(0xFFFFFF))
	p.border = or(t.InputBorder, or(t.Border, dvRGB(0x8A8A8A)))
	p.accent = or(t.Accent, dvRGB(0x0078D7))
	p.accent.A = 255
	p.focus = or(t.InputFocus, p.accent)
	p.text = or(t.InputText, dvRGB(0x1F1F1F))
	p.placeholder = or(t.InputPlaceholder, mixRGBA(p.text, p.bg, 0.5))
	p.sel = or(t.TextSelectionBG, mixRGBA(p.bg, p.accent, 0.3))
	p.dropBG = or(t.DropBG, p.bg)
	p.dropBorder = or(t.DropBorder, p.border)
	p.dropText = or(t.DropText, p.text)
	p.muted = or(t.SecondaryText, mixRGBA(p.dropText, p.dropBG, 0.45))
	p.disabled = or(t.Disabled, mixRGBA(p.dropText, p.dropBG, 0.65))
	p.hoverBG = or(t.MenuHoverBG, or(t.ListItemHover, mixRGBA(p.dropBG, p.accent, 0.15)))
	p.hoverText = or(t.MenuHoverText, p.dropText)
	p.accentText = contrastText(p.accent)
	// Ошибка набора — тем же красным, что текст удалённого в сравнении: тема
	// уже подобрала его читаемым на поле ввода.
	p.err = or(t.DiffDelText, dvRGB(0xCF222E))
	p.glyph = or(t.DropArrow, p.muted)
	return p
}

// AccessInfo — контракт доступности: поле даты как комбинированный список.
func (p *DatePicker) AccessInfo() AccessInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	info := AccessInfo{
		Role:        RoleComboBox,
		Name:        Tr("date.a11y"),
		Value:       p.textLocked(),
		Description: p.ToolTip,
		Bounds:      p.Base.Bounds(),
	}
	if p.focused {
		info.States = append(info.States, StateFocused)
	}
	if !p.IsEnabled() {
		info.States = append(info.States, StateDisabled)
	}
	return info
}

// BaseBounds — прямоугольник поля без календаря (для ShiftWidget).
func (p *DatePicker) BaseBounds() image.Rectangle { return p.Base.Bounds() }

// Bounds с открытым календарём охватывает и его: так щелчок по числу находит
// поле, как щелчок по пункту — раскрытый Dropdown.
func (p *DatePicker) Bounds() image.Rectangle {
	b := p.Base.Bounds()
	p.mu.Lock()
	open := p.open
	p.mu.Unlock()
	if !open {
		return b
	}
	return b.Union(dpCalendarRect(b))
}
