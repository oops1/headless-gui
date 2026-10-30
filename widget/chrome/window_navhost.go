package chrome

// window_navhost.go — панель навигации окна (NavPanel) и подпись заголовка
// над ней.

import (
	"image"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// SetNavPanel назначает окну боковую панель навигации во всю высоту, включая
// полосу заголовка. Клиентская область (ContentBounds) отступает вправо от
// неё, и остальные дети окна раскладываются уже по ней.
func (w *Window) SetNavPanel(c base.Widget) {
	if w.navPanel == c {
		return
	}
	if w.navPanel != nil {
		w.RemoveChild(w.navPanel)
	}
	w.navPanel = c
	if c != nil {
		w.AddChild(c)
		base.MoveChildToBack(&w.Base, c)
		if np, ok := c.(*base.NavPanel); ok {
			base.SetNavPanelOnLayout(np, func() {
				w.SetBounds(w.Bounds())
				w.Invalidate()
			})
		}
	}
	w.SetBounds(w.Bounds())
	w.Invalidate()
}

// NavPanel возвращает боковую панель окна (nil — не задана).
func (w *Window) NavPanel() base.Widget { return w.navPanel }

// NavPanelBounds — колонка боковой панели окна: от верхнего края до нижнего,
// внутри рамки.
func (w *Window) NavPanelBounds() image.Rectangle {
	if w.navPanel == nil {
		return image.Rectangle{}
	}
	b := w.Bounds()
	fw := w.frameW()
	pw := base.NavWidthOf(w.navPanel, b.Dx())
	top := b.Min.Y + fw
	if w.macTitleBar() {
		// В mac-стиле «светофор» стоит в левом верхнем углу — колонка,
		// поднятая под полосу, накрыла бы кнопки окна.
		top = b.Min.Y + w.titleH()
	}
	return image.Rect(b.Min.X+fw, top, b.Min.X+fw+pw, b.Max.Y-fw)
}

// layoutNavPanel ставит панель окна в её колонку.
func (w *Window) layoutNavPanel() {
	if w.navPanel == nil {
		return
	}
	if ti, ok := w.navPanel.(interface{ SetTopInset(int) }); ok {
		ti.SetTopInset(w.titleH())
	}
	w.navPanel.SetBounds(w.NavPanelBounds())
}

// drawTitleCaptionOverNav возвращает подпись заголовка поверх боковой панели.
//
// Панель поднята под верхний край окна и рисуется первой из детей — иначе её
// цвет обрывался бы под полосой. Подпись после этого приходится нарисовать
// ещё раз: менять порядок отрисовки полосы и детей ради одного случая дороже,
// чем повторить строку текста.
func (w *Window) drawTitleCaptionOverNav(ctx base.DrawContext) {
	if w.Title == "" || w.titleTabsActive() {
		return
	}
	if w.macTitleBar() {
		// В mac-стиле кнопки окна стоят слева, и панель под полосу не
		// поднимается (см. NavPanelBounds) — подпись никто не закрывал.
		return
	}
	tb := w.titleBarRect()
	if tb.Empty() {
		return
	}
	_, _, tc := w.titleColors()
	textX := tb.Min.X + 12
	if w.navBtn != nil && !w.navBtn.Base.Bounds().Empty() {
		textX = w.navBtn.Base.Bounds().Max.X + base.TitleBarGap
	}
	right := w.titleContentRight()
	if w.titleBarContent != nil {
		right = w.titleContentLeft() - base.TitleBarGap
	}
	title := w.Title
	if maxW := right - textX; maxW <= 0 {
		return
	} else {
		title = base.EllipsizeText(ctx, title, maxW, base.DefaultFontSize())
	}
	base.DrawTitleText(ctx, title, textX, tb.Min.Y+(w.effTitleH()-13)/2, tc)
}
