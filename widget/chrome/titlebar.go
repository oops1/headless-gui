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
package chrome

import (
	"image"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// ─── Кнопка сворачивания ────────────────────────────────────────────────────

// ─── Диалог: отступ содержимого ─────────────────────────────────────────────

// ─── Диалог: начинка полосы заголовка ───────────────────────────────────────

// ─── Диалог: кнопка сворачивания ────────────────────────────────────────────

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
func (w *Window) SetTitleBarContent(c base.Widget) {
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
func (w *Window) TitleBarContent() base.Widget { return w.titleBarContent }

// TitleBarContentBounds — свободная часть полосы заголовка окна.
func (w *Window) TitleBarContentBounds() image.Rectangle {
	tb := w.titleBarRect()
	if tb.Empty() || w.macTitleBar() {
		return image.Rectangle{}
	}
	r := image.Rect(w.titleContentLeft(), tb.Min.Y+base.TitleBarPadV,
		w.titleContentRight(), tb.Max.Y-base.TitleBarPadV)
	if r.Dx() < base.TitleBarMinW || r.Dy() < 1 {
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
	return !w.style().Classic3D && w.resolvedTitleStyle() == base.WindowTitleMac
}

// titleContentLeft — левая граница свободной части полосы: после кнопки
// сворачивания и после подписи.
func (w *Window) titleContentLeft() int {
	tb := w.titleBarRect()
	x := w.titleTextLeft(tb)
	if w.Title != "" && !w.titleTabsActive() {
		x += w.titleTextW() + base.TitleBarGap
	}
	// Начинка не заезжает на колонку боковой панели (см. Dialog).
	if nb := w.NavPanelBounds(); !nb.Empty() && nb.Max.X+base.TitleBarGap > x {
		x = nb.Max.X + base.TitleBarGap
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
		right -= base.LocaleBadgeReserve
	}
	// Свободная полоска между начинкой и кнопками — за неё тащат окно.
	return right - base.TitleBarHandleW
}

// titleTextW — ширина подписи заголовка окна тем шрифтом, которым её рисуют.
func (w *Window) titleTextW() int {
	if w.Title == "" {
		return 0
	}
	if w.style().Classic3D {
		return base.MeasureTextFontFor(w, w.Title, base.DefaultFontSize(), base.BuiltinFontBold)
	}
	return base.MeasureTextFor(w, w.Title, 10)
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
		nb := &base.TitleNavButton{}
		nb.Toggle = func() { w.SetNavCollapsed(!nb.Collapsed) }
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
	w.navBtn.Icon, w.navBtn.IconAlt = expanded, collapsed
	w.navBtn.Invalidate()
}

// SetNavCollapsed задаёт состояние кнопки сворачивания и уведомляет
// приложение (OnNavToggle).
func (w *Window) SetNavCollapsed(v bool) {
	if w.navBtn == nil || w.navBtn.Collapsed == v {
		return
	}
	w.navBtn.Collapsed = v
	w.navBtn.Invalidate()
	if c, ok := w.navPanel.(interface{ SetCollapsed(bool) }); ok {
		c.SetCollapsed(v)
	}
	if w.OnNavToggle != nil {
		w.OnNavToggle(v)
	}
}

// IsNavCollapsed сообщает состояние кнопки сворачивания.
func (w *Window) IsNavCollapsed() bool { return w.navBtn != nil && w.navBtn.Collapsed }

// layoutTitleBar расставляет кнопку сворачивания и начинку полосы заголовка.
func (w *Window) layoutTitleBar() {
	tb := w.titleBarRect()
	w.layoutIcon(tb)
	if w.navBtn != nil {
		// В mac-раскладке своей начинки в полосе нет — и кнопке сворачивания
		// там тоже не место: слева стоят кнопки окна.
		if tb.Empty() || w.macTitleBar() {
			w.navBtn.SetVisible(false)
			w.navBtn.SetBounds(image.Rectangle{})
		} else {
			sz := base.NavBtnSize
			if h := tb.Dy() - 4; sz > h {
				sz = h
			}
			left := tb.Min.X + 6
			if ir := w.IconBounds(); !ir.Empty() {
				left = ir.Max.X + base.TitleBarGap/2 // за значком окна, а не поверх него
			}
			top := tb.Min.Y + (tb.Dy()-sz)/2
			w.navBtn.SetVisible(true)
			w.navBtn.SetBounds(image.Rect(left, top, left+sz, top+sz))
		}
	}
	if w.titleBarContent != nil {
		r := w.TitleBarContentBounds()
		base.SetWidgetVisible(w.titleBarContent, !r.Empty())
		w.titleBarContent.SetBounds(r)
	}
}

// titleBarChildHit — точка над начинкой полосы или кнопкой сворачивания.
//
// Такая точка не начинает перетаскивание окна: виджет приложения в полосе
// обязан получать свои клики, иначе поиск в заголовке нельзя ни выделить, ни
// прокрутить — окно уезжало бы вслед за курсором.
func (w *Window) titleBarChildHit(pt image.Point) bool {
	if w.navBtn != nil && base.IsWidgetVisible(w.navBtn) && pt.In(w.navBtn.Bounds()) {
		return true
	}
	if w.titleBarContent != nil && base.IsWidgetVisible(w.titleBarContent) &&
		pt.In(w.titleBarContent.Bounds()) {
		return true
	}
	if w.iconClaims(pt) {
		return true
	}
	// Явные способы приложения (titlebar_hit.go): виджет, назвавшийся
	// владельцем нажатия, и решение по точке.
	return w.titleBarOwnedByApp(pt)
}

// isTitleBarWidget сообщает, принадлежит ли ребёнок полосе заголовка.
//
// Такие дети не растягиваются на клиентскую область (SetBounds): их геометрию
// задаёт полоса, а не содержимое окна.
func (w *Window) isTitleBarWidget(c base.Widget) bool {
	if c == nil {
		return false
	}
	return (w.navBtn != nil && base.Widget(w.navBtn) == c) || w.titleBarContent == c ||
		w.navPanel == c || w.isIconWidget(c)
}
