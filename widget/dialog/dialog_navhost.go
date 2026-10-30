package dialog

// dialog_navhost.go — панель навигации диалога (NavPanel).

import (
	"image"

	"github.com/oops1/headless-gui/v4/widget/base"
)

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
func (d *Dialog) SetNavPanel(w base.Widget) {
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
		base.MoveChildToBack(&d.Base, w)
		if np, ok := w.(*base.NavPanel); ok {
			base.SetNavPanelOnLayout(np, func() {
				d.SetBounds(d.Base.Bounds())
				d.Invalidate()
			})
		}
	}
	d.SetBounds(d.Base.Bounds())
	d.Invalidate()
}

// NavPanel возвращает боковую панель навигации (nil — не задана).
func (d *Dialog) NavPanel() base.Widget { return d.navPanel }

// NavPanelBounds — колонка боковой панели: от верхнего края диалога до
// нижнего.
func (d *Dialog) NavPanelBounds() image.Rectangle {
	if d.navPanel == nil {
		return image.Rectangle{}
	}
	b := d.Base.Bounds()
	w := base.NavWidthOf(d.navPanel, b.Dx())
	return image.Rect(b.Min.X, b.Min.Y, b.Min.X+w, b.Max.Y)
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
