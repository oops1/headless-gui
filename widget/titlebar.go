// titlebar.go — своя начинка штатной полосы заголовка и отступ содержимого.
//
// SetChromeless (dialog_chrome.go) решал задачу «или-или»: либо полоса
// движка с подписью и ✕, либо ничего — шапку целиком рисует приложение. На
// практике нужна середина: полоса остаётся движковой (подпись, ✕, свернуть/
// развернуть у окна), но в её свободной части стоит поиск приложения, а слева
// от подписи — кнопка, сворачивающая боковую навигацию.
//
// Вторая половина — отступ содержимого. У диалога он был константой dlgPad,
// и вокруг боковой панели оставалась белая рамка в полтора десятка точек:
// угол формально отдан приложению, а занять его нельзя (запрос GG-52).
package widget

import (
	"image"
	"image/color"
)

const (
	navBtnSize   = 24 // сторона кнопки сворачивания в полосе заголовка
	titleBarGap  = 8  // зазор между элементами полосы
	titleBarPadV = 4  // отступ начинки от верхнего и нижнего края полосы
	titleBarMinW = 48 // уже этого начинку не показываем — рисовать нечего
	// titleBarHandleW — свободное место между начинкой приложения и кнопками
	// окна. Полоса заголовка — это ещё и место, за которое окно тащат: поле
	// поиска во всю ширину не оставляло бы за него ни одной точки.
	titleBarHandleW = 72
	// localeBadgeReserve — место, отводимое плашке локали в полосе заголовка.
	// Её ширина зависит от подписи локали и известна только на отрисовке, а
	// раскладка начинки идёт в SetBounds — берём заведомо достаточное.
	localeBadgeReserve = 44
	dlgVPad            = 12 // вертикальный отступ содержимого диалога
)

// ─── Кнопка сворачивания ────────────────────────────────────────────────────

// titleNavBtn — кнопка в левой части полосы заголовка, сворачивающая боковую
// область приложения.
//
// Само сворачивание делает приложение: движок не знает, ЧТО у него слева —
// панель навигации, дерево или разделённый SplitPanel. Здесь только кнопка,
// её состояние и уведомление; иначе пришлось бы выдумывать за приложение,
// какой виджет считать «боковой областью» и что делать с остальными.
type titleNavBtn struct {
	Base
	icon      image.Image // иконка в развёрнутом состоянии (nil — встроенная «≡»)
	iconAlt   image.Image // иконка в свёрнутом состоянии (nil — та же)
	collapsed bool
	hover     bool
	// armed — кнопка «взведена» нажатием: срабатывание на отпускании и только
	// если курсор всё ещё над ней (семантика кнопок заголовка, как у ✕).
	armed  bool
	fg     color.RGBA // цвет встроенной иконки; хост ставит его перед отрисовкой
	capMgr CaptureManager
	toggle func()
}

func (nb *titleNavBtn) currentIcon() image.Image {
	if nb.collapsed && nb.iconAlt != nil {
		return nb.iconAlt
	}
	return nb.icon
}

func (nb *titleNavBtn) Draw(ctx DrawContext) {
	b := nb.bounds
	if b.Empty() {
		return
	}
	if nb.hover {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 4, win10.BtnHoverBG)
	}
	if ic := nb.currentIcon(); ic != nil {
		sz := b.Dx() - 8
		if h := b.Dy() - 8; h < sz {
			sz = h
		}
		if sz < 1 {
			return
		}
		ctx.DrawImageScaled(ic, b.Min.X+(b.Dx()-sz)/2, b.Min.Y+(b.Dy()-sz)/2, sz, sz)
		return
	}
	// Встроенная «≡»: три полосы. Рисуется цветом подписи заголовка — иконка
	// живёт в полосе и обязана следовать её теме, а не палитре кнопок.
	const lineW, lineH, gap = 14, 2, 4
	x := b.Min.X + (b.Dx()-lineW)/2
	y := b.Min.Y + (b.Dy()-(lineH*3+gap*2))/2
	for i := 0; i < 3; i++ {
		ctx.FillRect(x, y+i*(lineH+gap), lineW, lineH, nb.fg)
	}
}

func (nb *titleNavBtn) OnMouseMove(x, y int) {
	h := image.Pt(x, y).In(nb.bounds)
	if h != nb.hover {
		nb.hover = h
		nb.Invalidate()
	}
}

// SetCaptureManager инжектится движком: захват нужен, чтобы отпускание пришло
// кнопке, даже если курсор ушёл с неё.
func (nb *titleNavBtn) SetCaptureManager(cm CaptureManager) { nb.capMgr = cm }

func (nb *titleNavBtn) WantsCapture(e MouseEvent) bool {
	return e.Button == MouseLeft && e.Pressed && image.Pt(e.X, e.Y).In(nb.bounds)
}

func (nb *titleNavBtn) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	over := image.Pt(e.X, e.Y).In(nb.bounds)
	if e.Pressed {
		if !over {
			return false
		}
		nb.armed = true
		return true
	}
	if !nb.armed {
		return false
	}
	nb.armed = false
	if nb.capMgr != nil {
		nb.capMgr.ReleaseCapture()
	}
	if over && nb.toggle != nil {
		nb.toggle()
	}
	return true
}

// ─── Диалог: отступ содержимого ─────────────────────────────────────────────

// SetContentPadding задаёт отступ содержимого от краёв диалога.
//
// До сих пор это была константа: 14 точек по горизонтали и 12 по вертикали, и
// боковая панель, доходящая до края окна, была недостижима — вокруг неё
// оставалась полоса фона диалога, закрасить которую нечем (фон один на всё
// окно).
//
// Отрицательное значение возвращает умолчание: у обычного диалога прежние
// 14/12 (кнопки и подписи MessageBox рассчитаны на них), у диалога без
// штатной полосы (SetChromeless) — ноль, потому что там приложение рисует всё
// само и отступ движка ему только мешает.
func (d *Dialog) SetContentPadding(px int) {
	if px < 0 {
		if !d.padSet {
			return
		}
		d.padSet, d.pad = false, 0
	} else {
		if d.padSet && d.pad == px {
			return
		}
		d.padSet, d.pad = true, px
	}
	// Область содержимого изменилась — перекладываем.
	d.SetBounds(d.bounds)
	d.Invalidate()
}

// ContentPadding возвращает действующий отступ содержимого по горизонтали и
// вертикали.
func (d *Dialog) ContentPadding() (h, v int) { return d.contentPadding() }

func (d *Dialog) contentPadding() (h, v int) {
	if d.padSet {
		return d.pad, d.pad
	}
	if d.chromeless {
		return 0, 0
	}
	return dlgPad, dlgVPad
}

// ─── Диалог: начинка полосы заголовка ───────────────────────────────────────

// SetTitleBarContent помещает виджет приложения в свободную часть полосы
// заголовка — между подписью и кнопкой ✕.
//
//	dlg.SetTitleBarContent(searchBox) // поиск в полосе заголовка
//	dlg.SetTitleBarContent(nil)       // убрать
//
// Раскладывает его движок: полоса принадлежит ему, её высота зависит от темы,
// а справа в ней стоят его же органы управления. Приложение, которому нужна
// полоса целиком, берёт SetChromeless и рисует её само.
//
// Нажатие на этот виджет не тащит окно: он ребёнок диалога, а дети в штатной
// полосе забирают клик себе (см. titleDragHit).
func (d *Dialog) SetTitleBarContent(w Widget) {
	if d.titleBarContent == w {
		return
	}
	if d.titleBarContent != nil {
		d.RemoveChild(d.titleBarContent)
	}
	d.titleBarContent = w
	if w != nil {
		d.AddChild(w)
	}
	d.layoutTitleBar()
	d.Invalidate()
}

// TitleBarContent возвращает виджет, стоящий в полосе заголовка (nil — нет).
func (d *Dialog) TitleBarContent() Widget { return d.titleBarContent }

// TitleBarContentBounds — свободная часть полосы заголовка: справа от подписи
// (и кнопки сворачивания, если она есть) и слева от ✕.
//
// Пустой прямоугольник означает, что места нет: полосы нет вовсе
// (SetChromeless) или она занята подписью и кнопками целиком.
func (d *Dialog) TitleBarContentBounds() image.Rectangle {
	th := d.titleH()
	if th == 0 {
		return image.Rectangle{}
	}
	b := d.bounds
	left := d.titleTextX() + d.titleTextW()
	if d.Title != "" {
		left += titleBarGap
	}
	// Колонка боковой панели поднята под верх окна — начинка полосы начинается
	// ЗА ней, а не поверх: иначе поле поиска заезжало бы на панель, у которой
	// свой цвет и свои пункты.
	if nb := d.NavPanelBounds(); !nb.Empty() && nb.Max.X+titleBarGap > left {
		left = nb.Max.X + titleBarGap
	}
	// Правее начинки остаётся свободная полоска — за неё окно тащат.
	right := d.sysButtonsLeft() - titleBarHandleW
	if d.ShowLocaleIndicator {
		right -= localeBadgeReserve
	}
	r := image.Rect(left, b.Min.Y+titleBarPadV, right, b.Min.Y+th-titleBarPadV)
	if r.Dx() < titleBarMinW || r.Dy() < 1 {
		return image.Rectangle{}
	}
	return r
}

// titleTextX — левый край подписи заголовка в абсолютных координатах.
// Одна точка правды для отрисовки и для раскладки начинки: кнопка
// сворачивания сдвигает подпись вправо, и посчитать это в двух местах
// по-разному значило бы наложить поиск на заголовок.
func (d *Dialog) titleTextX() int {
	if d.navBtn != nil && !d.navBtn.bounds.Empty() {
		return d.navBtn.bounds.Max.X + titleBarGap
	}
	if currentStyle().Classic3D {
		return d.bounds.Min.X + 10
	}
	return d.bounds.Min.X + dlgPad
}

// titleTextW — ширина подписи заголовка тем шрифтом, которым её рисуют.
func (d *Dialog) titleTextW() int {
	if d.Title == "" {
		return 0
	}
	if currentStyle().Classic3D {
		return MeasureUITextFont(d.Title, DefaultFontSizePt, BuiltinFontBold)
	}
	return MeasureUITextFont(d.Title, 11, BuiltinFontBold)
}

// ─── Диалог: кнопка сворачивания ────────────────────────────────────────────

// SetNavButton показывает или убирает кнопку сворачивания в левой части
// полосы заголовка.
//
// Само сворачивание — за приложением: движку неизвестно, что у него слева.
// Кнопка лишь переключает состояние и зовёт OnNavToggle:
//
//	dlg.SetNavButton(true)
//	dlg.SetNavIcons(iconMenu, nil) // своя иконка (nil — встроенная «≡»)
//	dlg.OnNavToggle = func(collapsed bool) {
//	    nav.SetVisible(!collapsed)
//	    layout()
//	}
//
// У диалога без штатной полосы (SetChromeless) кнопки нет: её негде рисовать,
// и шапку там приложение рисует само — вместе со своей кнопкой.
func (d *Dialog) SetNavButton(v bool) {
	if v == (d.navBtn != nil) {
		return
	}
	if !v {
		d.RemoveChild(d.navBtn)
		d.navBtn = nil
	} else {
		nb := &titleNavBtn{fg: d.TitleColor}
		nb.toggle = func() { d.SetNavCollapsed(!nb.collapsed) }
		d.navBtn = nb
		d.AddChild(nb)
	}
	d.layoutTitleBar()
	d.Invalidate()
}

// HasNavButton сообщает, показана ли кнопка сворачивания.
func (d *Dialog) HasNavButton() bool { return d.navBtn != nil }

// SetNavIcons задаёт иконки кнопки сворачивания для развёрнутого и свёрнутого
// состояния. nil во втором аргументе — рисовать ту же иконку в обоих.
func (d *Dialog) SetNavIcons(expanded, collapsed image.Image) {
	if d.navBtn == nil {
		d.SetNavButton(true)
	}
	d.navBtn.icon, d.navBtn.iconAlt = expanded, collapsed
	d.navBtn.Invalidate()
}

// SetNavCollapsed задаёт состояние кнопки сворачивания и уведомляет
// приложение (OnNavToggle) — как если бы по кнопке щёлкнули.
func (d *Dialog) SetNavCollapsed(v bool) {
	if d.navBtn == nil || d.navBtn.collapsed == v {
		return
	}
	d.navBtn.collapsed = v
	d.navBtn.Invalidate()
	// Кнопка в полосе — орган управления именно боковой панелью, когда она
	// задана: не сворачивать её здесь значило бы просить приложение повторить
	// то же самое в OnNavToggle.
	if c, ok := d.navPanel.(interface{ SetCollapsed(bool) }); ok {
		c.SetCollapsed(v)
	}
	if d.OnNavToggle != nil {
		d.OnNavToggle(v)
	}
}

// IsNavCollapsed сообщает состояние кнопки сворачивания.
func (d *Dialog) IsNavCollapsed() bool { return d.navBtn != nil && d.navBtn.collapsed }

// layoutTitleBar расставляет кнопку сворачивания и начинку полосы.
// Зовётся из SetBounds: ширина полосы меняется вместе с диалогом.
func (d *Dialog) layoutTitleBar() {
	b := d.bounds
	th := d.titleH()
	if d.navBtn != nil {
		if th == 0 {
			d.navBtn.SetBounds(image.Rectangle{})
			d.navBtn.SetVisible(false)
		} else {
			sz := navBtnSize
			if sz > th-4 {
				sz = th - 4
			}
			top := b.Min.Y + (th-sz)/2
			d.navBtn.SetVisible(true)
			d.navBtn.SetBounds(image.Rect(b.Min.X+6, top, b.Min.X+6+sz, top+sz))
		}
	}
	if d.titleBarContent != nil {
		r := d.TitleBarContentBounds()
		setWidgetVisible(d.titleBarContent, !r.Empty())
		d.titleBarContent.SetBounds(r)
	}
}

// setWidgetVisible прячет виджет, если он это умеет.
//
// Начинку полосы даёт приложение, и это произвольный Widget: интерфейс не
// обязывает уметь SetVisible. Пустых границ мало — виджет, рисующий себя без
// оглядки на них, оставил бы след поверх заголовка.
func setWidgetVisible(w Widget, v bool) {
	if s, ok := w.(interface{ SetVisible(bool) }); ok {
		s.SetVisible(v)
	}
}

// ─── Окно: начинка полосы заголовка и кнопка сворачивания ───────────────────

// SetTitleBarContent помещает виджет приложения в свободную часть полосы
// заголовка окна — между подписью и кнопками управления.
//
//	win.SetTitleBarContent(searchBox)
//
// Этот виджет НЕ растягивается на клиентскую область, в отличие от остальных
// детей окна: он живёт в полосе, и её геометрию считает движок.
//
// В mac-стиле подпись стоит по центру полосы, и начинка заняла бы её место —
// поэтому там подпись не рисуется, пока начинка задана (так же ведут себя
// вкладки заголовка).
func (w *Window) SetTitleBarContent(c Widget) {
	if w.titleBarContent == c {
		return
	}
	if w.titleBarContent != nil {
		w.RemoveChild(w.titleBarContent)
	}
	w.titleBarContent = c
	if c != nil {
		w.AddChild(c)
	}
	w.layoutTitleBar()
	w.Invalidate()
}

// TitleBarContent возвращает виджет, стоящий в полосе заголовка (nil — нет).
func (w *Window) TitleBarContent() Widget { return w.titleBarContent }

// TitleBarContentBounds — свободная часть полосы заголовка окна.
func (w *Window) TitleBarContentBounds() image.Rectangle {
	tb := w.titleBarRect()
	if tb.Empty() || w.macTitleBar() {
		return image.Rectangle{}
	}
	r := image.Rect(w.titleContentLeft(), tb.Min.Y+titleBarPadV,
		w.titleContentRight(), tb.Max.Y-titleBarPadV)
	if r.Dx() < titleBarMinW || r.Dy() < 1 {
		return image.Rectangle{}
	}
	return r
}

// macTitleBar сообщает, что полоса заголовка нарисована в mac-раскладке.
//
// В ней своя начинка не поддерживается: кнопки окна стоят СЛЕВА, подпись
// центрирована, и виджет приложения пришлось бы втискивать между ними —
// раскладка, которой в macOS нет ни у одного окна. Полоса остаётся такой,
// какой её ждут на этой системе.
func (w *Window) macTitleBar() bool {
	return !w.style().Classic3D && w.resolvedTitleStyle() == WindowTitleMac
}

// titleContentLeft — левая граница свободной части полосы: после кнопки
// сворачивания и после подписи.
func (w *Window) titleContentLeft() int {
	tb := w.titleBarRect()
	x := tb.Min.X + 12
	if w.navBtn != nil && !w.navBtn.bounds.Empty() {
		x = w.navBtn.bounds.Max.X + titleBarGap
	}
	if w.Title != "" && !w.titleTabsActive() {
		x += w.titleTextW() + titleBarGap
	}
	// Начинка не заезжает на колонку боковой панели (см. Dialog).
	if nb := w.NavPanelBounds(); !nb.Empty() && nb.Max.X+titleBarGap > x {
		x = nb.Max.X + titleBarGap
	}
	return x
}

// titleContentRight — правая граница свободной части: левый край плашки
// локали либо блока кнопок управления.
func (w *Window) titleContentRight() int {
	b := w.Bounds()
	right := b.Max.X - 8
	if w.style().Classic3D {
		if closeR, _, minR := w.classicTitleBtnRects(); !minR.Empty() {
			right = minR.Min.X - 4
		} else if !closeR.Empty() {
			right = closeR.Min.X - 4
		}
	} else if nc := w.btnCount(); nc > 0 {
		right = b.Max.X - w.btnWidth()*nc
	}
	if w.ShowLocaleIndicator {
		right -= localeBadgeReserve
	}
	// Свободная полоска между начинкой и кнопками — за неё тащат окно.
	return right - titleBarHandleW
}

// titleTextW — ширина подписи заголовка окна тем шрифтом, которым её рисуют.
func (w *Window) titleTextW() int {
	if w.Title == "" {
		return 0
	}
	if w.style().Classic3D {
		return MeasureUITextFont(w.Title, DefaultFontSizePt, BuiltinFontBold)
	}
	return MeasureUIText(w.Title, 10)
}

// SetNavButton показывает или убирает кнопку сворачивания в левой части
// полосы заголовка окна. Сворачивание делает приложение по OnNavToggle.
func (w *Window) SetNavButton(v bool) {
	if v == (w.navBtn != nil) {
		return
	}
	if !v {
		w.RemoveChild(w.navBtn)
		w.navBtn = nil
	} else {
		nb := &titleNavBtn{}
		nb.toggle = func() { w.SetNavCollapsed(!nb.collapsed) }
		w.navBtn = nb
		w.AddChild(nb)
	}
	w.layoutTitleBar()
	w.Invalidate()
}

// HasNavButton сообщает, показана ли кнопка сворачивания.
func (w *Window) HasNavButton() bool { return w.navBtn != nil }

// SetNavIcons задаёт иконки кнопки сворачивания для развёрнутого и свёрнутого
// состояния. nil во втором аргументе — одна иконка на оба.
func (w *Window) SetNavIcons(expanded, collapsed image.Image) {
	if w.navBtn == nil {
		w.SetNavButton(true)
	}
	w.navBtn.icon, w.navBtn.iconAlt = expanded, collapsed
	w.navBtn.Invalidate()
}

// SetNavCollapsed задаёт состояние кнопки сворачивания и уведомляет
// приложение (OnNavToggle).
func (w *Window) SetNavCollapsed(v bool) {
	if w.navBtn == nil || w.navBtn.collapsed == v {
		return
	}
	w.navBtn.collapsed = v
	w.navBtn.Invalidate()
	if c, ok := w.navPanel.(interface{ SetCollapsed(bool) }); ok {
		c.SetCollapsed(v)
	}
	if w.OnNavToggle != nil {
		w.OnNavToggle(v)
	}
}

// IsNavCollapsed сообщает состояние кнопки сворачивания.
func (w *Window) IsNavCollapsed() bool { return w.navBtn != nil && w.navBtn.collapsed }

// layoutTitleBar расставляет кнопку сворачивания и начинку полосы заголовка.
func (w *Window) layoutTitleBar() {
	tb := w.titleBarRect()
	if w.navBtn != nil {
		// В mac-раскладке своей начинки в полосе нет — и кнопке сворачивания
		// там тоже не место: слева стоят кнопки окна.
		if tb.Empty() || w.macTitleBar() {
			w.navBtn.SetVisible(false)
			w.navBtn.SetBounds(image.Rectangle{})
		} else {
			sz := navBtnSize
			if h := tb.Dy() - 4; sz > h {
				sz = h
			}
			left := tb.Min.X + 6
			top := tb.Min.Y + (tb.Dy()-sz)/2
			w.navBtn.SetVisible(true)
			w.navBtn.SetBounds(image.Rect(left, top, left+sz, top+sz))
		}
	}
	if w.titleBarContent != nil {
		r := w.TitleBarContentBounds()
		setWidgetVisible(w.titleBarContent, !r.Empty())
		w.titleBarContent.SetBounds(r)
	}
}

// titleBarChildHit — точка над начинкой полосы или кнопкой сворачивания.
//
// Такая точка не начинает перетаскивание окна: виджет приложения в полосе
// обязан получать свои клики, иначе поиск в заголовке нельзя ни выделить, ни
// прокрутить — окно уезжало бы вслед за курсором.
func (w *Window) titleBarChildHit(pt image.Point) bool {
	if w.navBtn != nil && IsWidgetVisible(w.navBtn) && pt.In(w.navBtn.Bounds()) {
		return true
	}
	if w.titleBarContent != nil && IsWidgetVisible(w.titleBarContent) &&
		pt.In(w.titleBarContent.Bounds()) {
		return true
	}
	return false
}

// isTitleBarWidget сообщает, принадлежит ли ребёнок полосе заголовка.
//
// Такие дети не растягиваются на клиентскую область (SetBounds): их геометрию
// задаёт полоса, а не содержимое окна.
func (w *Window) isTitleBarWidget(c Widget) bool {
	if c == nil {
		return false
	}
	return (w.navBtn != nil && Widget(w.navBtn) == c) || w.titleBarContent == c ||
		w.navPanel == c
}
