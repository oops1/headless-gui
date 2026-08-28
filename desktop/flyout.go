// flyout.go — общая основа всплывающих панелей рабочего стола: меню «Пуск»,
// календарь, быстрые настройки, центр уведомлений.
//
// Все четыре устроены одинаково: висят над панелью задач, открываются от
// своего значка, закрываются кликом мимо или Esc, а рисуются не в потоке
// виджетов, а ОВЕРЛЕЕМ — поверх всего остального. Оверлей здесь не
// украшение: движок умеет выносить его в отдельное окно ОС
// (engine.SetPopupSink), и тогда панель может выходить за границы окна
// оболочки, как настоящее системное меню.
//
// Общая часть — открытие, закрытие, привязка к значку, подложка по стилю
// темы. Содержимое каждая панель рисует своё.
package desktop

import (
	"image"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Edge — с какой стороны панели задач всплывает окно.
type Edge int

const (
	// EdgeBottom — панель внизу экрана, всплывающее окно раскрывается вверх.
	EdgeBottom Edge = iota
	// EdgeTop — панель вверху (macOS), окно раскрывается вниз.
	EdgeTop
)

// Align — к какому краю значка прижимается всплывающее окно.
type Align int

const (
	// AlignStart — по левому краю значка.
	AlignStart Align = iota
	// AlignCenter — по центру значка.
	AlignCenter
	// AlignEnd — по правому краю значка.
	AlignEnd
)

// Flyout — общая основа всплывающей панели.
//
// Встраивается в конкретную панель, которая обязана задать Content —
// отрисовку своего содержимого — и Size — желаемый размер.
type Flyout struct {
	widget.Base

	tm *theme.Manager

	// Component — имя компонента для стилей темы (например, "startmenu").
	Component string

	// Anchor — прямоугольник значка, от которого всплывает окно
	// (абсолютные логические координаты). Пустой — окно встанет от края
	// экрана.
	Anchor image.Rectangle
	// Edge и Align — как окно стоит относительно значка.
	Edge  Edge
	Align Align
	// Margin — зазор между значком и окном.
	Margin int

	// Screen — границы экрана: окно в них вписывается и не уезжает за край.
	Screen image.Rectangle

	// Content рисует содержимое окна в отведённом прямоугольнике.
	// Подложку рисует сам Flyout — содержимому остаётся своё.
	Content func(ctx widget.DrawContext, r image.Rectangle)
	// Size возвращает желаемый размер окна. Обязателен: без него окно
	// нулевое и не показывается.
	Size func() image.Point

	// OnOpen и OnClose — уведомления для оболочки (например, чтобы
	// подсветить значок, от которого открыто окно).
	OnOpen  func()
	OnClose func()

	open int32
}

// NewFlyout создаёт всплывающую панель компонента component темы tm.
func NewFlyout(tm *theme.Manager, component string) *Flyout {
	return &Flyout{tm: tm, Component: component, Margin: 6}
}

// Theme возвращает менеджер тем панели.
func (f *Flyout) Theme() *theme.Manager { return f.tm }

// IsOpen сообщает, открыта ли панель.
func (f *Flyout) IsOpen() bool { return atomic.LoadInt32(&f.open) == 1 }

// Open открывает панель, привязав её к значку anchor.
//
// Повторное открытие уже открытой панели ничего не меняет, кроме привязки:
// значок мог переехать при перераскладке.
func (f *Flyout) Open(anchor image.Rectangle) {
	was := f.rect() // область прежнего положения — её тоже надо перерисовать
	f.Anchor = anchor
	reopened := atomic.SwapInt32(&f.open, 1) == 1
	f.invalidateOverlay(was)
	if reopened {
		return
	}
	if f.OnOpen != nil {
		f.OnOpen()
	}
}

// Close закрывает панель. Закрытие закрытой — не событие.
func (f *Flyout) Close() {
	// Область считается ДО закрытия: после него rect() пуст, и заявлять
	// освободившееся место было бы уже нечем — на экране осталась бы
	// нестёртая панель.
	was := f.rect()
	if atomic.SwapInt32(&f.open, 0) == 0 {
		return
	}
	f.invalidateOverlay(was)
	if f.OnClose != nil {
		f.OnClose()
	}
}

// Toggle открывает закрытую панель и закрывает открытую — то, что делает
// повторный клик по значку.
func (f *Flyout) Toggle(anchor image.Rectangle) {
	if f.IsOpen() {
		f.Close()
		return
	}
	f.Open(anchor)
}

// Invalidate заявляет движку область ОВЕРЛЕЯ, а не границы виджета.
//
// Границы виджета у всплывающей панели — это значок на панели задач, а
// меняется у неё содержимое окна, нарисованного совсем в другом месте.
// Заяви она себя, движок обрезал бы перерисовку по значку, и панель на
// экране осталась бы прежней.
func (f *Flyout) Invalidate() { f.invalidateOverlay(image.Rectangle{}) }

// invalidateOverlay заявляет текущую область оверлея и, если задана, прежнюю.
func (f *Flyout) invalidateOverlay(also image.Rectangle) {
	f.Base.Invalidate() // значок мог измениться сам по себе
	if r := f.rect(); !r.Empty() && f.IsOpen() {
		widget.InvalidateRect(r)
	}
	if !also.Empty() {
		widget.InvalidateRect(also)
	}
}

// OverlayBounds возвращает прямоугольник окна в абсолютных логических
// координатах (пустой, если закрыто). Реализует widget.OverlayBoundsProvider.
func (f *Flyout) OverlayBounds() image.Rectangle {
	if !f.IsOpen() {
		return image.Rectangle{}
	}
	return f.rect()
}

// HasOverlay реализует widget.OverlayDrawer.
func (f *Flyout) HasOverlay() bool { return f.IsOpen() && !f.rect().Empty() }

// DrawOverlay рисует подложку по стилю темы и отдаёт содержимому остальное.
func (f *Flyout) DrawOverlay(ctx widget.DrawContext) {
	r := f.rect()
	if !f.IsOpen() || r.Empty() {
		return
	}
	s := f.style(theme.StateNormal)
	PaintStyle(ctx, r, s)
	if f.Content == nil {
		return
	}
	// Содержимое клипуется окном: длинный список не должен вылезать за
	// подложку, а рисовать его укороченным — забота самого содержимого.
	prev := ctx.Clip()
	ctx.SetClip(r.Intersect(prev))
	f.Content(ctx, r.Inset(int(s.PadX)))
	ctx.SetClip(prev)
}

// Draw в потоке виджетов не рисует ничего: всё содержимое панели уходит в
// DrawOverlay. Метод нужен, чтобы панель можно было положить в дерево — а в
// дереве она обязана быть, иначе движок не найдёт её оверлей.
func (f *Flyout) Draw(ctx widget.DrawContext) { f.DrawChildren(ctx) }

// OnMouseButton закрывает панель кликом мимо неё.
//
// Клик ВНУТРИ панель не поглощает: его разбирает содержимое — иначе ни одна
// кнопка внутри меню «Пуск» не нажалась бы.
func (f *Flyout) OnMouseButton(e widget.MouseEvent) bool {
	if !f.IsOpen() || !e.Pressed {
		return false
	}
	if image.Pt(e.X, e.Y).In(f.rect()) {
		return false
	}
	f.Close()
	return true
}

// OnKeyEvent закрывает панель по Esc.
func (f *Flyout) OnKeyEvent(e widget.KeyEvent) {
	if f.IsOpen() && e.Pressed && e.Code == widget.KeyEscape {
		f.Close()
	}
}

// rect считает положение окна: желаемый размер, привязка к значку, вписывание
// в экран.
func (f *Flyout) rect() image.Rectangle {
	if f.Size == nil {
		return image.Rectangle{}
	}
	sz := f.Size()
	if sz.X <= 0 || sz.Y <= 0 {
		return image.Rectangle{}
	}

	screen := f.Screen
	if screen.Empty() {
		screen = f.Anchor
	}

	// По вертикали — от значка в сторону от края, к которому прижата панель.
	var y int
	switch f.Edge {
	case EdgeTop:
		y = f.Anchor.Max.Y + f.Margin
	default:
		y = f.Anchor.Min.Y - f.Margin - sz.Y
	}

	// По горизонтали — по выбранному краю значка.
	var x int
	switch f.Align {
	case AlignCenter:
		x = f.Anchor.Min.X + (f.Anchor.Dx()-sz.X)/2
	case AlignEnd:
		x = f.Anchor.Max.X - sz.X
	default:
		x = f.Anchor.Min.X
	}

	r := image.Rect(x, y, x+sz.X, y+sz.Y)
	if screen.Empty() {
		return r
	}
	return fitInto(r, screen)
}

// fitInto сдвигает r внутрь screen, не меняя размера (а если не влезает по
// размеру — обрезает по экрану: лучше показать часть, чем ничего).
func fitInto(r, screen image.Rectangle) image.Rectangle {
	if dx := r.Max.X - screen.Max.X; dx > 0 {
		r = r.Sub(image.Pt(dx, 0))
	}
	if dx := screen.Min.X - r.Min.X; dx > 0 {
		r = r.Add(image.Pt(dx, 0))
	}
	if dy := r.Max.Y - screen.Max.Y; dy > 0 {
		r = r.Sub(image.Pt(0, dy))
	}
	if dy := screen.Min.Y - r.Min.Y; dy > 0 {
		r = r.Add(image.Pt(0, dy))
	}
	return r.Intersect(screen)
}

// style читает стиль панели из темы.
func (f *Flyout) style(st theme.State) *theme.Style {
	if f.tm == nil {
		return &theme.Style{}
	}
	return f.tm.GetStyle(f.Component, "", st)
}

// metric читает метрику темы.
func (f *Flyout) metric(k theme.Key) int {
	if f.tm == nil {
		return 0
	}
	return int(f.tm.GetMetric(k))
}
