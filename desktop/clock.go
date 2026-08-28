// clock.go — часы панели задач: время и (если хватает места) дата.
//
// Тиканье не будит рендер вхолостую: секундный Animate проверяет, изменилась
// ли ОТОБРАЖАЕМАЯ СТРОКА, и зовёт Invalidate только тогда. Секунда, в
// которую минута не сменилась, кадра не стоит.
package desktop

import (
	"image"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentClock — имя компонента для стилей темы. Части: "" — строка
// времени (весь элемент, если дата не показывается), "date" — строка даты
// под ней.
const ComponentClock = "clock"

// Форматы по умолчанию, если поля TimeFormat/DateFormat не заданы.
const (
	defaultTimeFormat = "15:04"
	defaultDateFormat = "02.01.2006"
)

// ClockItem — часы в трее панели задач.
//
// Показывает время и, если по вертикали хватает места (высота панели у
// современных тем больше, чем у классики), дату под ним. Ширина, отступы,
// шрифт и цвета — из стиля темы (ComponentClock, части "" и "date"); тут нет
// ни одного цвета и ни одного магического размера — оба измерения читаются
// из Style.PadX/PadY, которые уже существуют в theme.Style и ни разу не
// использовались отрисовкой: PadX — горизонтальный отступ блока, PadY части
// "date" — вертикальный зазор между строкой времени и строкой даты.
type ClockItem struct {
	widget.Base

	tm  *theme.Manager
	clk Clock

	// TimeFormat/DateFormat — форматы time.Time.Format. Пустые — берутся
	// значения по умолчанию (defaultTimeFormat/defaultDateFormat).
	TimeFormat string
	DateFormat string

	// OnClick — щелчок по часам. Оболочка вешает на него календарь: именно
	// так он открывается на настоящем рабочем столе.
	OnClick func()

	hovered int32
	pressed int32

	mu       sync.Mutex
	lastTime string
	lastDate string
	anim     *widget.Animation
}

// NewClock создаёт часы, оформляемые темой tm и показывающие время clk.
func NewClock(tm *theme.Manager, clk Clock) *ClockItem {
	return &ClockItem{tm: tm, clk: clk}
}

// OnMouseMove обновляет наведение — часы подсвечиваются, как и значки трея,
// раз по ним можно щёлкнуть.
func (c *ClockItem) OnMouseMove(x, y int) {
	trayHandleMove(&c.hovered, c.Bounds(), x, y, c.Invalidate)
}

// OnMouseButton — щелчок срабатывает на отпускании над часами, как у всех
// кнопок панели задач.
func (c *ClockItem) OnMouseButton(e widget.MouseEvent) bool {
	if c.OnClick == nil {
		return false
	}
	return trayHandleClick(&c.pressed, c.Bounds(), e, c.OnClick, c.Invalidate)
}

// Close останавливает секундный тик. Часы, снятые со сцены и не закрытые,
// продолжали бы будить рендер раз в секунду вечно.
func (c *ClockItem) Close() {
	c.mu.Lock()
	a := c.anim
	c.anim = nil
	c.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

// now возвращает текущее время источника (SystemClock, если clk не задан).
func (c *ClockItem) now() time.Time {
	if c.clk == nil {
		return SystemClock{}.Now()
	}
	return c.clk.Now()
}

func (c *ClockItem) timeFormat() string {
	if c.TimeFormat != "" {
		return c.TimeFormat
	}
	return defaultTimeFormat
}

func (c *ClockItem) dateFormat() string {
	if c.DateFormat != "" {
		return c.DateFormat
	}
	return defaultDateFormat
}

// style возвращает стиль часов из активной темы (пустой стиль, если темы
// нет — как и остальные компоненты пакета).
func (c *ClockItem) style(part string, st theme.State) *theme.Style {
	if c.tm == nil {
		return &theme.Style{}
	}
	return c.tm.GetStyle(ComponentClock, part, st)
}

// fontSize — кегль шрифта строки времени (он же используется для даты:
// разный размер usually задаётся её собственным стилем, если тема захочет
// сделать дату мельче — можно переопределить Style.Font.Size части "date").
func fontSizeOf(s *theme.Style) float64 {
	if s.Font.Size > 0 {
		return s.Font.Size
	}
	return widget.DefaultFontSizePt
}

// lineHeight — высота строки текста тем же способом, каким её считает
// paint.go (DrawTextCentered/DrawTextLeft): size*1.4. Часы обязаны знать эту
// высоту ДО отрисовки, в PreferredSize, где ctx ещё недоступен — поэтому
// повторяют формулу локально, а не переиспользуют приватную функцию соседнего
// файла.
func lineHeight(size float64) int {
	return int(size * 1.4)
}

// KeyClockDate — показывать ли дату под временем.
//
// Признак, а не расчёт по месту: в классических часах Windows 2000 даты нет
// вовсе, сколько бы места ни было — она живёт во всплывающей подсказке. Без
// признака часы решали бы это высотой панели и на высокой панели показывали
// бы дату там, где её быть не должно.
const KeyClockDate theme.Key = "clock.date"

// twoLine решает, показывать ли вторую строку (дату): разрешает ли её тема и
// хватает ли высоты.
func (c *ClockItem) twoLine(availY int, timeSize float64) bool {
	if c.tm != nil && !c.tm.GetFlag(KeyClockDate, true) {
		return false
	}
	dateStyle := c.style("date", theme.StateNormal)
	dateSize := fontSizeOf(dateStyle)
	gap := int(dateStyle.PadY)
	need := lineHeight(timeSize) + gap + lineHeight(dateSize)
	return availY >= need
}

// PreferredSize считает желаемый размер по фактической строке (см.
// widget.MeasureUIText — единственный измеритель текста, работающий вне
// Draw: docs/AI_AGENT_REFERENCE.md, «Text measurement outside Draw»).
func (c *ClockItem) PreferredSize(avail image.Point) image.Point {
	timeStyle := c.style("", theme.StateNormal)
	timeSize := fontSizeOf(timeStyle)
	padX := int(timeStyle.PadX)

	now := c.now()
	timeStr := now.Format(c.timeFormat())
	w := widget.MeasureUIText(timeStr, timeSize)
	h := lineHeight(timeSize)

	if c.twoLine(avail.Y, timeSize) {
		dateStyle := c.style("date", theme.StateNormal)
		dateSize := fontSizeOf(dateStyle)
		dateStr := now.Format(c.dateFormat())
		if dw := widget.MeasureUIText(dateStr, dateSize); dw > w {
			w = dw
		}
		h = lineHeight(timeSize) + int(dateStyle.PadY) + lineHeight(dateSize)
	}

	return image.Point{X: w + padX*2, Y: h}
}

// Draw рисует время и, если помещается, дату под ним. Оба через
// DrawTextCentered (paint.go) — она же централизованно берёт цвет и шрифт
// из стиля и центрирует строку в переданном прямоугольнике, поэтому здесь
// нет ни одного цвета и ни одного размера в пикселях.
func (c *ClockItem) Draw(ctx widget.DrawContext) {
	b := c.Bounds()
	if b.Empty() {
		return
	}

	timeStyle := c.style("", theme.StateNormal)
	timeSize := fontSizeOf(timeStyle)
	now := c.now()
	timeStr := now.Format(c.timeFormat())
	dateStr := now.Format(c.dateFormat())

	c.mu.Lock()
	c.lastTime = timeStr
	c.lastDate = dateStr
	c.mu.Unlock()
	c.ensureTick()

	if c.twoLine(b.Dy(), timeSize) {
		dateStyle := c.style("date", theme.StateNormal)
		half := b.Min.Y + b.Dy()/2
		top := image.Rect(b.Min.X, b.Min.Y, b.Max.X, half)
		bottom := image.Rect(b.Min.X, half, b.Max.X, b.Max.Y)
		DrawTextCentered(ctx, top, timeStr, timeStyle)
		DrawTextCentered(ctx, bottom, dateStr, dateStyle)
		return
	}
	DrawTextCentered(ctx, b, timeStr, timeStyle)
}

// ensureTick запускает секундный Animate при первом Draw (как
// widget/progressbar_glow.go делает для свечения — см. ensureGlowAnim) и не
// трогает уже идущую анимацию.
func (c *ClockItem) ensureTick() {
	c.mu.Lock()
	running := c.anim != nil && c.anim.Running()
	c.mu.Unlock()
	if running {
		return
	}

	a := widget.Animate(time.Second, nil, c.onTick)
	a.Loop = true
	c.mu.Lock()
	c.anim = a
	c.mu.Unlock()
}

// onTick вызывается раз в секунду (широковещательный тик Animate). Он не
// перерисовывает по расписанию — только если фактически изменилась строка,
// которую видит пользователь: это и есть требование не будить рендер
// вхолостую (перерисовка нужна раз в минуту, а не раз в секунду).
func (c *ClockItem) onTick(float64) {
	now := c.now()
	timeStr := now.Format(c.timeFormat())
	dateStr := now.Format(c.dateFormat())

	c.mu.Lock()
	changed := timeStr != c.lastTime || dateStr != c.lastDate
	c.mu.Unlock()

	if changed {
		c.Invalidate()
	}
}
