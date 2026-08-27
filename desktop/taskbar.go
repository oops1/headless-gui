package desktop

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Slot — область панели задач, в которую становится элемент.
type Slot int

const (
	// SlotStart — начало панели: кнопка «Пуск», поиск.
	SlotStart Slot = iota
	// SlotApps — середина: запущенные и закреплённые приложения. Именно эта
	// область растягивается, когда места не хватает.
	SlotApps
	// SlotTray — конец панели: значки состояния, часы, уведомления.
	SlotTray
)

// Item — элемент панели задач.
//
// Это обычный виджет, который умеет сказать, сколько места ему нужно.
// Размер запрашивается при каждой раскладке, а не хранится: у часов он
// зависит от длины строки, у кнопок окон — от того, сколько окон открыто.
type Item interface {
	widget.Widget
	// PreferredSize возвращает желаемый размер при доступном месте avail.
	// Панель вправе дать меньше — элемент обязан пережить это (см.
	// требование к деградации в комментарии к Taskbar).
	PreferredSize(avail image.Point) image.Point
}

// Taskbar — панель задач: полоса во всю ширину экрана с тремя областями.
//
// Элементы добавляются в области списком, а не полями структуры: новый
// компонент не должен требовать правки панели. Порядок внутри области —
// порядок добавления.
//
// Раскладка. Начало и конец получают столько, сколько просят; середина —
// остаток. Если середине не хватает, сжимается она: элемент обязан
// деградировать предсказуемо (кнопки окон сжимаются до значков, затем
// уходят в прокрутку), а не вылезать за панель и не исчезать целиком.
//
// Внешний вид — из темы: высота панели, отступы, цвет и подложка берутся
// токенами. В отрисовке панели нет ни одного числа и ни одного цвета.
type Taskbar struct {
	widget.Base

	tm *theme.Manager

	slots [3][]Item

	// unsubTheme снимает подписку на смену темы. Панель перерисовывается
	// сама: она живёт вне обхода дерева, когда её показывают оболочкой.
	unsubTheme func()
}

// Ключи токенов, которыми тема управляет панелью.
const (
	KeyTaskbarHeight theme.Key = "taskbar.height"
	KeyTaskbarPadX   theme.Key = "taskbar.pad.x"
	KeyTaskbarGap    theme.Key = "taskbar.gap"

	// ComponentTaskbar — имя компонента для стилей темы.
	ComponentTaskbar = "taskbar"
)

// NewTaskbar создаёт панель задач, оформляемую темами из tm.
func NewTaskbar(tm *theme.Manager) *Taskbar {
	t := &Taskbar{tm: tm}
	if tm != nil {
		t.unsubTheme = tm.Subscribe(theme.ObserverFunc(func(*theme.Theme) {
			t.relayout()
			t.Invalidate()
		}))
	}
	return t
}

// Close снимает подписку на тему. Панель, которую сняли со сцены и не
// закрыли, остаётся в списке наблюдателей менеджера — это утечка.
func (t *Taskbar) Close() {
	if t.unsubTheme != nil {
		t.unsubTheme()
		t.unsubTheme = nil
	}
}

// AddItem ставит элемент в область панели.
func (t *Taskbar) AddItem(slot Slot, it Item) {
	if it == nil || slot < SlotStart || slot > SlotTray {
		return
	}
	t.slots[slot] = append(t.slots[slot], it)
	t.AddChild(it)
	t.relayout()
	t.Invalidate()
}

// Items возвращает элементы области в порядке добавления.
func (t *Taskbar) Items(slot Slot) []Item {
	if slot < SlotStart || slot > SlotTray {
		return nil
	}
	return append([]Item(nil), t.slots[slot]...)
}

// Height возвращает высоту панели из темы (0 — тема не задала, решает
// вызывающий: панель не выдумывает размеров сама).
func (t *Taskbar) Height() int {
	if t.tm == nil {
		return 0
	}
	return int(t.tm.GetMetric(KeyTaskbarHeight))
}

// ReservedArea возвращает область экрана, занятую панелью.
//
// Нужна потребителю: развёрнутое окно не должно перекрывать панель, а
// узнать её геометрию иначе неоткуда.
func (t *Taskbar) ReservedArea() image.Rectangle { return t.Bounds() }

// SetBounds задаёт положение панели и перекладывает элементы.
func (t *Taskbar) SetBounds(r image.Rectangle) {
	t.Base.SetBounds(r)
	t.relayout()
}

// relayout расставляет элементы по областям.
func (t *Taskbar) relayout() {
	b := t.Bounds()
	if b.Empty() {
		return
	}
	padX := t.metric(KeyTaskbarPadX)
	gap := t.metric(KeyTaskbarGap)
	inner := image.Rect(b.Min.X+padX, b.Min.Y, b.Max.X-padX, b.Max.Y)
	if inner.Empty() {
		return
	}
	avail := image.Pt(inner.Dx(), inner.Dy())

	// Начало — слева, конец — справа, каждый берёт по запросу.
	x := inner.Min.X
	for _, it := range t.slots[SlotStart] {
		sz := t.sizeOf(it, avail)
		place(it, image.Rect(x, inner.Min.Y, x+sz.X, inner.Min.Y+sz.Y), inner)
		x += sz.X + gap
	}
	startEnd := x

	right := inner.Max.X
	for i := len(t.slots[SlotTray]) - 1; i >= 0; i-- {
		it := t.slots[SlotTray][i]
		sz := t.sizeOf(it, avail)
		place(it, image.Rect(right-sz.X, inner.Min.Y, right, inner.Min.Y+sz.Y), inner)
		right -= sz.X + gap
	}
	trayStart := right

	// Середина получает остаток. Просят больше — сжимаем пропорционально:
	// это и есть предсказуемая деградация, о которой договорились с
	// элементами.
	midAvail := trayStart - startEnd
	if midAvail < 0 {
		midAvail = 0
	}
	apps := t.slots[SlotApps]
	if len(apps) == 0 {
		return
	}
	want := make([]int, len(apps))
	total := 0
	for i, it := range apps {
		sz := t.sizeOf(it, image.Pt(midAvail, avail.Y))
		want[i] = sz.X
		total += sz.X
	}
	total += gap * (len(apps) - 1)

	scale := 1.0
	if total > midAvail && total > 0 {
		scale = float64(midAvail-gap*(len(apps)-1)) / float64(total-gap*(len(apps)-1))
		if scale < 0 {
			scale = 0
		}
	}

	mx := startEnd
	for i, it := range apps {
		w := int(float64(want[i]) * scale)
		place(it, image.Rect(mx, inner.Min.Y, mx+w, inner.Max.Y), inner)
		mx += w + gap
	}
}

// place ставит элемент в прямоугольник, вписывая его в границы панели.
func place(it Item, r, inner image.Rectangle) {
	r = r.Intersect(inner)
	if r.Empty() {
		// Место кончилось: элемент прячется, а не рисуется поверх соседа.
		it.SetBounds(image.Rectangle{})
		return
	}
	it.SetBounds(r)
}

// sizeOf спрашивает у элемента желаемый размер, подставляя высоту панели,
// если элемент её не ограничивает.
func (t *Taskbar) sizeOf(it Item, avail image.Point) image.Point {
	sz := it.PreferredSize(avail)
	if sz.X <= 0 {
		sz.X = 0
	}
	if sz.Y <= 0 || sz.Y > avail.Y {
		sz.Y = avail.Y
	}
	return sz
}

// metric читает метрику темы как целое (0, если темы нет).
func (t *Taskbar) metric(k theme.Key) int {
	if t.tm == nil {
		return 0
	}
	return int(t.tm.GetMetric(k))
}

// style возвращает стиль панели из активной темы.
func (t *Taskbar) style(part string, st theme.State) *theme.Style {
	if t.tm == nil {
		return &theme.Style{}
	}
	return t.tm.GetStyle(ComponentTaskbar, part, st)
}

// Draw рисует подложку панели и её элементы.
//
// Ни одного цвета и ни одного размера здесь нет: заливка, скругление,
// подложка и тень приходят стилем из темы. Профиль Windows 11 объявит
// размытую подложку, Windows 2000 — плоскую заливку с фаской, и панель
// об этом не узнает.
func (t *Taskbar) Draw(ctx widget.DrawContext) {
	b := t.Bounds()
	if b.Empty() {
		return
	}
	s := t.style("", theme.StateNormal)

	// Стекло: если тема просит размытую подложку, а контекст это умеет.
	if s.Backdrop.Mode == theme.BackdropBlur {
		if bd, ok := ctx.(widget.BackdropDrawer); ok {
			bd.BlurBehind(b, int(s.Backdrop.Radius), s.Backdrop.Tint)
		} else if s.Backdrop.Tint.A > 0 {
			// Контекст без размытия — остаётся подкраска. Так тема,
			// написанная для стекла, выглядит хотя бы полупрозрачной.
			fillAlpha(ctx, b, s.Backdrop.Tint)
		}
	}

	corner := int(s.Corner)
	if s.Fill.A > 0 {
		if corner > 0 {
			ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), corner, s.Fill)
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), s.Fill)
		}
	}
	if s.Border.A > 0 && s.BorderWidth > 0 {
		if corner > 0 {
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), corner, s.Border)
		} else {
			ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), s.Border)
		}
	}

	t.DrawChildren(ctx)
}

// fillAlpha заливает область с честным смешиванием, если контекст умеет.
func fillAlpha(ctx widget.DrawContext, r image.Rectangle, col color.RGBA) {
	if da, ok := ctx.(widget.DrawContextAlpha); ok {
		da.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), col)
		return
	}
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), col)
}
