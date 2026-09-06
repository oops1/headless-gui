// navhost.go — боковая панель во всю высоту окна, включая полосу заголовка.
//
// Обычный ребёнок диалога живёт под полосой заголовка: полосу рисует движок во
// всю ширину, и левый верхний угол оставался движковым — панель навигации
// начиналась под чужой полосой, а её заливка обрывалась, не доходя до верхнего
// края окна.
//
// Здесь панель поднимается под самый верх: она рисуется ПЕРЕД подписью
// заголовка, а подпись движок докладывает поверх — так цвет панели доходит до
// края окна, а заголовок остаётся на месте, как в вертикальных вкладках
// браузера.
package widget

import "image"

// ─── Диалог ─────────────────────────────────────────────────────────────────

// SetNavPanel назначает боковую панель навигации.
//
// Панель занимает левую колонку ВО ВСЮ ВЫСОТУ диалога, включая полосу
// заголовка, а область содержимого (ContentBounds) отступает вправо от неё.
// Ширину панель называет сама (DesiredSize), поэтому свёрнутая полоска с
// иконками отдаёт освободившееся место содержимому без единой строки в
// приложении.
//
//	nav := widget.NewNavPanel()
//	nav.AddItem(iconGeneral, "Общие")
//	dlg.SetNavPanel(nav)
//	dlg.SetNavButton(true) // «≡» в полосе сворачивает эту же панель
//
// Панель — не обязательно NavPanel: годится любой виджет. Ширину у него
// спрашивают через DesiredSize, а сворачивание — через SetCollapsed(bool),
// если он это умеет.
func (d *Dialog) SetNavPanel(w Widget) {
	if d.navPanel == w {
		return
	}
	if d.navPanel != nil {
		d.RemoveChild(d.navPanel)
	}
	d.navPanel = w
	if w != nil {
		d.AddChild(w)
		// Панель рисуется ПЕРВОЙ: поверх неё идут и подпись заголовка, и «≡»,
		// и виджет приложения в полосе. Добавленная последней, она закрыла бы
		// их собой.
		moveChildToBack(&d.Base, w)
		if np, ok := w.(*NavPanel); ok {
			np.setOnLayout(func() {
				d.SetBounds(d.bounds)
				d.Invalidate()
			})
		}
	}
	d.SetBounds(d.bounds)
	d.Invalidate()
}

// NavPanel возвращает боковую панель навигации (nil — не задана).
func (d *Dialog) NavPanel() Widget { return d.navPanel }

// NavPanelBounds — колонка боковой панели: от верхнего края диалога до
// нижнего.
func (d *Dialog) NavPanelBounds() image.Rectangle {
	if d.navPanel == nil {
		return image.Rectangle{}
	}
	b := d.bounds
	w := navWidthOf(d.navPanel, b.Dx())
	return image.Rect(b.Min.X, b.Min.Y, b.Min.X+w, b.Max.Y)
}

// navWidthOf — ширина боковой панели, ограниченная половиной окна: панель,
// занявшая всё окно, оставила бы содержимое без места и без объяснений.
func navWidthOf(w Widget, hostW int) int {
	pw, _ := desiredOf(w)
	if pw <= 0 {
		pw = navExpandedW
	}
	if max := hostW / 2; max > 0 && pw > max {
		pw = max
	}
	return pw
}

// layoutNavPanel ставит панель в её колонку и сообщает ей, сколько сверху
// занято подписью заголовка.
func (d *Dialog) layoutNavPanel() {
	if d.navPanel == nil {
		return
	}
	if ti, ok := d.navPanel.(interface{ SetTopInset(int) }); ok {
		ti.SetTopInset(d.titleH())
	}
	d.navPanel.SetBounds(d.NavPanelBounds())
}

// moveChildToBack переставляет ребёнка в начало списка — он рисуется первым,
// а остальные ложатся поверх него.
func moveChildToBack(b *Base, w Widget) {
	for i, c := range b.children {
		if c != w {
			continue
		}
		copy(b.children[1:i+1], b.children[:i])
		b.children[0] = w
		return
	}
}

// ─── Окно ───────────────────────────────────────────────────────────────────

// SetNavPanel назначает окну боковую панель навигации во всю высоту, включая
// полосу заголовка. Клиентская область (ContentBounds) отступает вправо от
// неё, и остальные дети окна раскладываются уже по ней.
func (w *Window) SetNavPanel(c Widget) {
	if w.navPanel == c {
		return
	}
	if w.navPanel != nil {
		w.RemoveChild(w.navPanel)
	}
	w.navPanel = c
	if c != nil {
		w.AddChild(c)
		moveChildToBack(&w.Base, c)
		if np, ok := c.(*NavPanel); ok {
			np.setOnLayout(func() {
				w.SetBounds(w.Bounds())
				w.Invalidate()
			})
		}
	}
	w.SetBounds(w.Bounds())
	w.Invalidate()
}

// NavPanel возвращает боковую панель окна (nil — не задана).
func (w *Window) NavPanel() Widget { return w.navPanel }

// NavPanelBounds — колонка боковой панели окна: от верхнего края до нижнего,
// внутри рамки.
func (w *Window) NavPanelBounds() image.Rectangle {
	if w.navPanel == nil {
		return image.Rectangle{}
	}
	b := w.Bounds()
	fw := w.frameW()
	pw := navWidthOf(w.navPanel, b.Dx())
	top := b.Min.Y + fw
	if w.resolvedTitleStyle() == WindowTitleMac {
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
func (w *Window) drawTitleCaptionOverNav(ctx DrawContext) {
	if w.Title == "" || w.titleTabsActive() {
		return
	}
	if w.resolvedTitleStyle() == WindowTitleMac {
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
	if w.navBtn != nil && !w.navBtn.bounds.Empty() {
		textX = w.navBtn.bounds.Max.X + titleBarGap
	}
	right := w.titleContentRight()
	if w.titleBarContent != nil {
		right = w.titleContentLeft() - titleBarGap
	}
	title := w.Title
	if maxW := right - textX; maxW <= 0 {
		return
	} else {
		title = ellipsizeText(ctx, title, maxW, DefaultFontSizePt)
	}
	drawTitleText(ctx, title, textX, tb.Min.Y+(w.effTitleH()-13)/2, tc)
}

// drawRoundClipped выполняет fn с отсечением по скруглённому прямоугольнику.
//
// Скруглённый клип — опциональная возможность контекста (RoundClipper): там,
// где её нет, рисуем как раньше. Прежнее отсечение восстанавливаем полностью —
// SetRoundClip подменяет и прямоугольный клип, а ClearRoundClip снимает только
// скругление, и без возврата всё нарисованное дальше осталось бы обрезанным.
func drawRoundClipped(ctx DrawContext, r image.Rectangle, radius int, fn func()) {
	rc, ok := ctx.(RoundClipper)
	if !ok || radius <= 0 {
		fn()
		return
	}
	prev := ctx.Clip()
	rc.SetRoundClip(r, radius)
	fn()
	rc.ClearRoundClip()
	ctx.SetClip(prev)
}
