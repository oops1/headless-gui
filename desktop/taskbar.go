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

	// StyleComponent — имя компонента для стилей темы. Пустое — "taskbar".
	// Второй полосе рабочего стола оболочка ставит ComponentDockbar.
	StyleComponent string

	// startEdge и trayEdge — границы секций, посчитанные раскладкой: между
	// ними классическая тема рисует разделители. Хранятся, а не считаются
	// заново при отрисовке, чтобы разделитель не разъехался с элементами.
	startEdge int
	trayEdge  int
}

// Ключи токенов, которыми тема управляет панелью.
const (
	KeyTaskbarHeight theme.Key = "taskbar.height"
	KeyTaskbarPadX   theme.Key = "taskbar.pad.x"
	KeyTaskbarGap    theme.Key = "taskbar.gap"
	// KeyTaskbarCentered — ставить ли группу «пуск + приложения» по центру
	// панели (Windows 11) вместо прижатия влево (всё остальное).
	KeyTaskbarCentered theme.Key = "taskbar.centered"
	// KeyTaskbarTop — панель прижата к ВЕРХНЕМУ краю экрана.
	// Так устроена строка меню macOS; всплывающие панели при этом
	// раскрываются вниз, а не вверх.
	KeyTaskbarTop theme.Key = "taskbar.top"
	// KeyDockHeight — высота отдельной нижней полосы (док macOS). Ноль —
	// отдельной полосы нет, всё живёт в одной панели.
	KeyDockHeight theme.Key = "dock.height"
	// KeyTaskbarSeparators — рисовать ли разделители между секциями панели.
	// Примета классической панели: между кнопкой «Пуск», кнопками окон и
	// треем стоят вертикальные хваталки с фаской.
	KeyTaskbarSeparators theme.Key = "taskbar.separators"

	// ComponentTaskbar — имя компонента для стилей темы.
	ComponentTaskbar = "taskbar"
	// ComponentDockbar — вторая полоса рабочего стола (док macOS): своё
	// скругление, свои поля, своё стекло.
	ComponentDockbar = "dockbar"
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

// Edge сообщает, к какому краю экрана прижата панель по мнению активной темы.
//
// Нужно не самой панели (она рисуется в тех границах, что ей дали), а
// оболочке и всплывающим панелям: от края зависит, куда им раскрываться.
func (t *Taskbar) Edge() Edge {
	if t.flag(KeyTaskbarTop) {
		return EdgeTop
	}
	return EdgeBottom
}

// DockHeight — высота отдельной нижней полосы, если тема её просит (0 — не
// просит). Отдельная полоса — это док macOS: строка меню и док не одно и то
// же и не могут быть одной панелью.
func (t *Taskbar) DockHeight() int { return t.metric(KeyDockHeight) }

// SetItems заменяет содержимое слота целиком.
//
// Нужно оболочке при смене темы: macOS раскладывает те же компоненты по двум
// полосам, Windows — по одной. Компоненты при этом остаются теми же
// объектами — переезжает только их принадлежность панели.
func (t *Taskbar) SetItems(slot Slot, items ...Item) {
	if slot < 0 || int(slot) >= len(t.slots) {
		return
	}
	for _, old := range t.slots[slot] {
		t.RemoveChild(old)
	}
	t.slots[slot] = nil
	for _, it := range items {
		t.AddItem(slot, it)
	}
	t.relayout()
}

// component — имя компонента для стилей темы.
//
// По умолчанию панель задач, но вторая полоса рабочего стола (док macOS)
// оформляется иначе: у неё своё скругление, свои поля и своё стекло. Одним
// именем это не выразить — строка меню осталась бы со скруглением дока.
func (t *Taskbar) component() string {
	if t.StyleComponent != "" {
		return t.StyleComponent
	}
	return ComponentTaskbar
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

	// Трей справа считается первым: он ни при каком раскладе не двигается, и
	// его левый край задаёт границу, дальше которой остальным нельзя.
	right := inner.Max.X
	for i := len(t.slots[SlotTray]) - 1; i >= 0; i-- {
		it := t.slots[SlotTray][i]
		sz := t.sizeOf(it, avail)
		place(it, image.Rect(right-sz.X, inner.Min.Y, right, inner.Min.Y+sz.Y), inner)
		right -= sz.X + gap
	}
	trayStart := right
	t.trayEdge = trayStart

	start, apps := t.slots[SlotStart], t.slots[SlotApps]

	// Желаемые ширины. Приложениям показываем не всю панель, а то, что
	// осталось после трея и кнопки пуска: иначе они запросят лишнее и
	// потребуют сжатия там, где его можно было избежать.
	startW := make([]int, len(start))
	startTotal := 0
	for i, it := range start {
		startW[i] = t.sizeOf(it, avail).X
		startTotal += startW[i] + gap
	}
	appsAvail := trayStart - inner.Min.X - startTotal
	if appsAvail < 0 {
		appsAvail = 0
	}
	appsW := make([]int, len(apps))
	appsTotal := 0
	for i, it := range apps {
		appsW[i] = t.sizeOf(it, image.Pt(appsAvail, avail.Y)).X
		appsTotal += appsW[i]
	}
	if len(apps) > 0 {
		appsTotal += gap * (len(apps) - 1)
	}

	// Группа «пуск + приложения» либо прижата влево, либо стоит по центру
	// ПАНЕЛИ — так это устроено в Windows 11. По центру именно панели, а не
	// оставшегося места: иначе группа съезжала бы влево от того, что справа
	// висит трей, и центр переставал быть центром.
	x := inner.Min.X
	if t.flag(KeyTaskbarCentered) {
		group := startTotal + appsTotal
		x = inner.Min.X + (inner.Dx()-group)/2
		// Но не поверх трея и не левее края.
		if x+group > trayStart {
			x = trayStart - group
		}
		if x < inner.Min.X {
			x = inner.Min.X
		}
	}

	for i, it := range start {
		place(it, image.Rect(x, inner.Min.Y, x+startW[i], inner.Min.Y+t.sizeOf(it, avail).Y), inner)
		x += startW[i] + gap
	}
	t.startEdge = x
	if len(apps) == 0 {
		return
	}

	// Приложениям — остаток до трея. Просят больше — сжимаем пропорционально:
	// это и есть предсказуемая деградация, о которой договорились с
	// элементами.
	midAvail := trayStart - x
	if midAvail < 0 {
		midAvail = 0
	}
	scale := 1.0
	if appsTotal > midAvail && appsTotal > 0 {
		gaps := gap * (len(apps) - 1)
		if appsTotal > gaps {
			scale = float64(midAvail-gaps) / float64(appsTotal-gaps)
		}
		if scale < 0 {
			scale = 0
		}
	}
	for i, it := range apps {
		w := int(float64(appsW[i]) * scale)
		place(it, image.Rect(x, inner.Min.Y, x+w, inner.Max.Y), inner)
		x += w + gap
	}
}

// place ставит элемент в прямоугольник, вписывая его в границы панели.
//
// Элемент, которому не нужна вся высота панели (значок трея), центрируется
// по вертикали, а не липнет к верхнему краю: панель — это ряд, и всё в ней
// стоит на одной средней линии.
func place(it Item, r, inner image.Rectangle) {
	if h := r.Dy(); h > 0 && h < inner.Dy() {
		top := inner.Min.Y + (inner.Dy()-h)/2
		r = image.Rect(r.Min.X, top, r.Max.X, top+h)
	}
	r = r.Intersect(inner)
	if r.Empty() {
		// Место кончилось: элемент прячется, а не рисуется поверх соседа.
		it.SetBounds(image.Rectangle{})
		return
	}
	it.SetBounds(r)
}

// flag читает признак темы (false, если темы нет).
func (t *Taskbar) flag(k theme.Key) bool {
	if t.tm == nil {
		return false
	}
	return t.tm.GetFlag(k, false)
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
	return t.tm.GetStyle(t.component(), part, st)
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
	// Подложка целиком отдана PaintStyle — тени, стекло, заливка, рамка и
	// фаска там уже разобраны по порядку. Своя копия этой логики здесь
	// когда-то была и разошлась с общей: заливка ложилась ПОВЕРХ размытия, и
	// стеклянная панель macOS выглядела матовой белой плашкой.
	PaintStyle(ctx, b, t.style("", theme.StateNormal))

	t.drawSeparators(ctx)
	t.DrawChildren(ctx)
}

// drawSeparators рисует вертикальные разделители между секциями панели.
//
// Примета классической панели: между кнопкой «Пуск», кнопками окон и треем
// стоят хваталки — светлая и тёмная линии рядом, как фаска. Современные темы
// признак не выставляют, и разделителей у них нет.
func (t *Taskbar) drawSeparators(ctx widget.DrawContext) {
	if !t.flag(KeyTaskbarSeparators) {
		return
	}
	b := t.Bounds()
	if b.Empty() {
		return
	}
	s := t.style("", theme.StateNormal)
	light, shadow := separatorColors(s)
	if light.A == 0 && shadow.A == 0 {
		return
	}
	gap := t.metric(KeyTaskbarGap)
	inset := b.Dy() / 6

	for _, x := range []int{t.startEdge, t.trayEdge} {
		// Разделитель стоит в зазоре между секциями, а не поверх элемента.
		cx := x - gap/2
		if cx <= b.Min.X+1 || cx >= b.Max.X-1 {
			continue
		}
		ctx.FillRect(cx, b.Min.Y+inset, 1, b.Dy()-2*inset, shadow)
		ctx.FillRect(cx+1, b.Min.Y+inset, 1, b.Dy()-2*inset, light)
	}
}

// separatorColors берёт грани фаски стиля панели, а если тема фасок не
// объявляла — обходится рамкой: разделитель обязан быть виден и в плоской
// теме, если та его попросила.
func separatorColors(s *theme.Style) (light, shadow color.RGBA) {
	if s.Bevel != nil {
		return s.Bevel.Light, s.Bevel.Shadow
	}
	return color.RGBA{}, s.Border
}

// fillAlpha заливает область с честным смешиванием, если контекст умеет.
func fillAlpha(ctx widget.DrawContext, r image.Rectangle, col color.RGBA) {
	if da, ok := ctx.(widget.DrawContextAlpha); ok {
		da.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), col)
		return
	}
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), col)
}
