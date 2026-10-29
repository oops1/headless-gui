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
package base

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
		MoveChildToBack(&d.Base, w)
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
	w := NavWidthOf(d.navPanel, b.Dx())
	return image.Rect(b.Min.X, b.Min.Y, b.Min.X+w, b.Max.Y)
}

// NavWidthOf — ширина боковой панели, ограниченная половиной окна: панель,
// занявшая всё окно, оставила бы содержимое без места и без объяснений.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func NavWidthOf(w Widget, hostW int) int {
	pw, _ := DesiredOf(w)
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

// MoveChildToBack переставляет ребёнка в начало списка — он рисуется первым,
// а остальные ложатся поверх него.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func MoveChildToBack(b *Base, w Widget) {
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

// DrawRoundClipped выполняет fn с отсечением по скруглённому прямоугольнику.
//
// Скруглённый клип — опциональная возможность контекста (RoundClipper): там,
// где её нет, рисуем как раньше. Прежнее отсечение восстанавливаем полностью —
// SetRoundClip подменяет и прямоугольный клип, а ClearRoundClip снимает только
// скругление, и без возврата всё нарисованное дальше осталось бы обрезанным.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func DrawRoundClipped(ctx DrawContext, r image.Rectangle, radius int, fn func()) {
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
