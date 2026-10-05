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
	"sync"
	"sync/atomic"
	"time"

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

// SlideFrom — откуда выезжает панель при появлении (и куда уезжает при
// закрытии).
type SlideFrom int

const (
	// SlideAuto — от края привязки: панель над нижней панелью задач выезжает
	// снизу, под верхней строкой меню — сверху.
	SlideAuto SlideFrom = iota
	// SlideBottom — снизу («Пуск» над панелью задач).
	SlideBottom
	// SlideTop — сверху.
	SlideTop
	// SlideLeft — слева.
	SlideLeft
	// SlideRight — справа (центр уведомлений).
	SlideRight
	// SlideNone — без сдвига: панель только проявляется.
	SlideNone
)

// KeyFlyoutSlideDistance — метрика темы: на сколько точек панель сдвинута в
// момент начала появления. Не заявлена — flyoutSlideDefault.
const KeyFlyoutSlideDistance theme.Key = "flyout.slide.distance"

// flyoutSlideDefault — сдвиг в начале появления, если ни панель, ни тема своего
// не назвали.
const flyoutSlideDefault = 48

// Flyout — общая основа всплывающей панели.
//
// Встраивается в конкретную панель, которая обязана задать Content —
// отрисовку своего содержимого — и Size — желаемый размер.
//
// Открытие и закрытие идут по анимации темы (по умолчанию AnimMenuOpen):
// панель выезжает от края привязки и проявляется, закрываясь — уезжает и
// гаснет. Нулевая длительность в теме — мгновенно. Пока идёт анимация,
// перерисовывается только область панели, в которой она движется.
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

	// afterClose — уборка встроенной панели при закрытии: таймеры,
	// накопленное состояние. Своё поле, а не OnClose, потому что OnClose
	// принадлежит ОБОЛОЧКЕ, и занять его значило бы отнять.
	//
	// Нужно потому, что закрывают панель со стороны: гашение движком
	// (DismissAt), клик мимо, Esc — всё это зовёт Flyout.Close, а не
	// переопределённый Close встроенной панели. Встраивание в Go
	// виртуальных вызовов не даёт.
	afterClose func()

	// group — набор панелей, считающих прямоугольники друг друга своими
	// (см. FlyoutGroup). nil — панель сама по себе.
	group *FlyoutGroup

	// OnOpen и OnClose — уведомления для оболочки (например, чтобы
	// подсветить значок, от которого открыто окно).
	OnOpen  func()
	OnClose func()

	// Slide — откуда выезжает панель; SlideAuto — от края привязки.
	Slide SlideFrom
	// SlideDistance — на сколько точек панель сдвинута в начале появления.
	// 0 — взять метрику темы KeyFlyoutSlideDistance; отрицательное — до
	// самого края экрана (боковая панель выезжает из-за края целиком).
	SlideDistance int
	// AnimKey — имя анимации темы, управляющей появлением. Пусто — AnimMenuOpen.
	AnimKey theme.Key

	open int32

	// presence — «присутствие» панели: 0 — нет, 1 — на месте. Идёт за open с
	// анимацией; меньше единицы — панель выезжает или уезжает (биты float64:
	// читается из раскладки на каждый кадр без замков).
	presence atomic.Uint64
	amu      sync.Mutex
	anim     *widget.Animation

	// watchers — наблюдатели за открытием и закрытием (см. Subscribe); у них
	// свой замок: наблюдатель вправе открывать и закрывать другие панели.
	wmu      sync.Mutex
	watchers map[int]func(open bool)
	nextW    int

	// anchorDismissAt — когда панель в последний раз закрыло нажатие мимо
	// неё, пришедшееся на её собственный якорь (unix-наносекунды; 0 — не
	// закрывало). См. Toggle.
	anchorDismissAt atomic.Int64
}

// anchorDismissWindow — сколько после такого закрытия Toggle помнит, что
// нажатие на якорь уже его закрыло. Нажатие и отпускание одного клика
// разделяют считанные десятки миллисекунд; с запасом на медленную мышь.
const anchorDismissWindow = 600 * time.Millisecond

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
	// Область прежнего положения — её тоже надо перерисовать. Но только если
	// панель была показана: у закрытой прежнего положения нет, а расчёт по
	// старой (чаще пустой) привязке дал бы прямоугольник в углу экрана, и
	// открытие панели перерисовывало бы то, что с ней не связано.
	var was image.Rectangle
	if f.visible() {
		was = f.dirtyRect()
	}
	f.Anchor = anchor
	reopened := atomic.SwapInt32(&f.open, 1) == 1
	if !reopened {
		f.anchorDismissAt.Store(0)
		f.animateTo(1)
	}
	f.invalidateOverlay(was)
	if reopened {
		return
	}
	// Наблюдатели раньше оболочки: менеджер панелей закрывает остальные до
	// того, как оболочка зажжёт кнопку этой.
	f.notify(true)
	if f.OnOpen != nil {
		f.OnOpen()
	}
}

// Close закрывает панель. Закрытие закрытой — не событие.
func (f *Flyout) Close() {
	// Область считается ДО закрытия: после него rect() пуст, и заявлять
	// освободившееся место было бы уже нечем — на экране осталась бы
	// нестёртая панель.
	was := f.dirtyRect()
	if atomic.SwapInt32(&f.open, 0) == 0 {
		return
	}
	f.animateTo(0)
	f.invalidateOverlay(was)
	// Сначала встроенная панель прибирает за собой, потом узнаёт оболочка.
	// Порядок важен: обработчик оболочки вправе тут же открыть что-то ещё, а
	// панель к этому моменту обязана быть в покое.
	if f.afterClose != nil {
		f.afterClose()
	}
	f.notify(false)
	if f.OnClose != nil {
		f.OnClose()
	}
}

// Toggle открывает закрытую панель и закрывает открытую — то, что делает
// повторный клик по значку.
//
// Если панель только что закрыло нажатие на её же якорь (клик мимо панели,
// пришедшийся на кнопку, которая её открывала), Toggle её заново не открывает:
// движок гасит панель на нажатии, а кнопка срабатывает на отпускании, и без
// этой оговорки клик по кнопке открытой панели закрывал бы её и тут же
// открывал снова.
func (f *Flyout) Toggle(anchor image.Rectangle) {
	if f.IsOpen() {
		f.Close()
		return
	}
	if f.consumeAnchorDismiss() {
		return
	}
	f.Open(anchor)
}

// consumeAnchorDismiss сообщает, закрывало ли панель только что нажатие на её
// якорь, и забывает об этом: одно закрытие гасит один Toggle.
func (f *Flyout) consumeAnchorDismiss() bool {
	t := f.anchorDismissAt.Swap(0)
	return t != 0 && time.Since(time.Unix(0, t)) < anchorDismissWindow
}

// DismissedByAnchor сообщает, закрыло ли панель только что (в пределах
// anchorDismissWindow) нажатие на её якорь. Кнопка, не использующая Toggle,
// по этому признаку решает, открывать ли панель на отпускании.
func (f *Flyout) DismissedByAnchor() bool {
	t := f.anchorDismissAt.Load()
	return t != 0 && time.Since(time.Unix(0, t)) < anchorDismissWindow
}

// Subscribe подписывает h на открытие (open=true) и закрытие (open=false)
// панели и возвращает функцию отписки. h зовётся в горутине, открывшей или
// закрывшей панель, до OnOpen и OnClose оболочки. В отличие от них, подписчиков
// может быть сколько угодно — на это рассчитан FlyoutManager.
func (f *Flyout) Subscribe(h func(open bool)) (unsubscribe func()) {
	if h == nil {
		return func() {}
	}
	f.wmu.Lock()
	if f.watchers == nil {
		f.watchers = map[int]func(bool){}
	}
	f.nextW++
	id := f.nextW
	f.watchers[id] = h
	f.wmu.Unlock()
	return func() {
		f.wmu.Lock()
		delete(f.watchers, id)
		f.wmu.Unlock()
	}
}

// notify сообщает наблюдателям об открытии или закрытии.
func (f *Flyout) notify(open bool) {
	f.wmu.Lock()
	if len(f.watchers) == 0 {
		f.wmu.Unlock()
		return
	}
	hs := make([]func(bool), 0, len(f.watchers))
	for _, h := range f.watchers {
		hs = append(hs, h)
	}
	f.wmu.Unlock()
	for _, h := range hs {
		h(open)
	}
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
	if r := f.dirtyRect(); !r.Empty() && f.visible() {
		widget.InvalidateRect(r)
	}
	if !also.Empty() {
		widget.InvalidateRect(also)
	}
}

// Bounds расширяет обычные границы виджета до прямоугольника открытой панели.
//
// Без этого движок панель попросту не видит: и поиск оверлея под курсором, и
// путь попадания идут по Bounds, а у панели в потоке виджетов геометрии нет —
// всё содержимое живёт в оверлее. Меню «Пуск» и быстрые настройки объявляли
// это каждое у себя, а календарь и центр уведомлений не объявляли вовсе — и
// не получали от движка ни кликов, ни движения мыши. Оболочке приходилось
// разносить события по панелям вручную.
func (f *Flyout) Bounds() image.Rectangle {
	base := f.Base.Bounds()
	if !f.IsOpen() {
		return base
	}
	return base.Union(f.restRect())
}

// Group возвращает группу панели (nil — панель сама по себе).
func (f *Flyout) Group() *FlyoutGroup { return f.group }

// SetGroup включает панель в группу: панели одной группы считают
// прямоугольники друг друга своими и закрываются вместе.
func (f *Flyout) SetGroup(g *FlyoutGroup) {
	if f.group == g {
		return
	}
	if f.group != nil {
		f.group.remove(f)
	}
	f.group = g
	if g != nil {
		g.add(f)
	}
}

// DismissAt реализует widget.DismissableAt: панель закрывается, когда клик
// пришёлся мимо неё — но НЕ когда он попал в соседку по группе.
func (f *Flyout) DismissAt(x, y int) {
	if !f.IsOpen() {
		return
	}
	pt := image.Pt(x, y)
	if f.ownsPoint(pt) {
		return
	}
	if !f.Anchor.Empty() && pt.In(f.Anchor) {
		f.anchorDismissAt.Store(time.Now().UnixNano())
	}
	f.Close()
}

// DismissOnEscape реализует widget.EscapeDismisser: Esc закрывает открытую
// панель вместе с группой, в которую она входит, где бы ни был клавиатурный
// фокус. Возвращает true, если что-то закрыла.
func (f *Flyout) DismissOnEscape() bool {
	if !f.IsOpen() {
		return false
	}
	if f.group != nil {
		f.group.CloseAll()
		return true
	}
	f.Close()
	return true
}

// ownsPoint — принадлежит ли точка этой панели или её соседке по группе.
func (f *Flyout) ownsPoint(pt image.Point) bool {
	if pt.In(f.rect()) {
		return true
	}
	return f.group.covers(pt)
}

// OverlayBounds возвращает прямоугольник окна в абсолютных логических
// координатах (пустой, если закрыто). Реализует widget.OverlayBoundsProvider.
//
// Это место панели в покое: пока она выезжает или уезжает, окно-носитель
// остаётся прежним, а сдвигается рисунок внутри него. Закрываемая панель
// остаётся в оверлее до конца анимации.
func (f *Flyout) OverlayBounds() image.Rectangle {
	if !f.visible() {
		return image.Rectangle{}
	}
	return f.restRect()
}

// HasOverlay реализует widget.OverlayDrawer.
func (f *Flyout) HasOverlay() bool { return f.visible() && !f.restRect().Empty() }

// DrawOverlay рисует подложку по стилю темы и отдаёт содержимому остальное.
//
// В покое — как всегда. Пока панель появляется или исчезает, она рисуется со
// сдвигом от края привязки (его несёт сам прямоугольник r, а значит и всё
// содержимое, считающее раскладку от него), обрезанная областью движения, и
// как слой с прозрачностью — если контекст это умеет.
func (f *Flyout) DrawOverlay(ctx widget.DrawContext) {
	r := f.rect()
	if !f.visible() || r.Empty() {
		return
	}
	p := f.Presence()
	if p >= 1 {
		f.paint(ctx, r)
		return
	}

	region := f.dirtyRect()
	prev := ctx.Clip()
	ctx.SetClip(region.Intersect(prev))
	if od, ok := ctx.(widget.OpacityDrawer); ok {
		od.DrawWithOpacity(region, p, func() { f.paint(ctx, r) })
	} else {
		f.paint(ctx, r)
	}
	ctx.SetClip(prev)
}

// paint рисует подложку и содержимое панели в прямоугольнике r.
func (f *Flyout) paint(ctx widget.DrawContext, r image.Rectangle) {
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
// Событие НЕ поглощается ни в каком случае. Клик внутри разбирает содержимое
// (иначе ни одна кнопка внутри меню «Пуск» не нажалась бы), а поглощённый
// клик мимо гасил соседнюю панель: центр уведомлений и календарь стоят один
// над другим, клик по числу приходится вне центра — тот закрывался и съедал
// событие, и календарь не видел ни чисел, ни стрелок месяца.
//
// Клик в соседку по группе панель не закрывает: на то она и группа.
func (f *Flyout) OnMouseButton(e widget.MouseEvent) bool {
	if !f.IsOpen() || !e.Pressed {
		return false
	}
	if f.ownsPoint(image.Pt(e.X, e.Y)) {
		return false
	}
	f.Close()
	return false
}

// OnKeyEvent закрывает панель по Esc.
func (f *Flyout) OnKeyEvent(e widget.KeyEvent) {
	if f.IsOpen() && e.Pressed && e.Code == widget.KeyEscape {
		f.Close()
	}
}

// rect — где панель сейчас: место в покое, сдвинутое на текущий шаг появления
// или исчезновения. Раскладка и попадание мыши считаются отсюда, так что
// нажимается то, что видно.
func (f *Flyout) rect() image.Rectangle {
	r := f.restRect()
	if off := f.slideOffset(); off != (image.Point{}) {
		r = r.Add(off)
	}
	return r
}

// restRect считает положение окна в покое: желаемый размер, привязка к
// значку, вписывание в экран.
func (f *Flyout) restRect() image.Rectangle {
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
