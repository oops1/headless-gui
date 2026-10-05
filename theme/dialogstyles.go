package theme

import "image/color"

// Стили модального диалога встроенных профилей.
//
// Диалог (widget.Dialog) читает из темы три цвета: тело ("dialog"), полосу
// заголовка ("dialog", "titlebar") и затемнение всего остального ("dialog",
// "scrim"). Профиль, который их не объявил, получал стиль по умолчанию — плоский
// токен "surface": затемнение выходило серым и непрозрачным (стол за диалогом
// пропадал), а полоса заголовка — серой под белым текстом окна (подпись
// терялась). Здесь у встроенных профилей они объявлены.
const (
	dialogScrimClassic = 90  // альфа затемнения у классики (как в пресете Win2000)
	dialogScrimModern  = 110 // у остальных профилей
)

// addDialogStyles дописывает профилю p стили диалога, которых он не объявил:
//
//   - заголовок диалога — как заголовок активного окна профиля (синий с белым
//     текстом у Windows 2000, цвета окна у остальных): диалог и есть окно, и
//     подпись должна читаться так же;
//   - затемнение — чёрное, полупрозрачное: стол остаётся виден сквозь него.
//
// Объявленное профилем (SetStyle) не перезаписывается. Разновидность профиля
// (Parent задан) наследует затемнение родителя, а заголовок получает свой, если
// у неё есть собственный заголовок активного окна: иначе синяя разновидность
// классики унаследовала бы тёмно-синий заголовок диалога от родителя.
func addDialogStyles(p *Profile) {
	if p.Styles == nil {
		return
	}
	if _, ok := p.Styles[StyleKey{Component: "dialog", Part: "titlebar", State: StateNormal}]; !ok {
		if t, ok := p.Styles[StyleKey{Component: "window", Part: "titlebar", State: StateFocused}]; ok {
			p.SetStyle("dialog", "titlebar", StateNormal, StyleDelta{
				Fill: t.Fill, Text: t.Text, FillFrom: t.FillFrom, TextFrom: t.TextFrom,
			})
		}
	}
	if p.Parent != "" {
		return
	}
	if _, ok := p.Styles[StyleKey{Component: "dialog", Part: "scrim", State: StateNormal}]; !ok {
		a := uint8(dialogScrimModern)
		if p.Flags["style.classic3d"] {
			a = dialogScrimClassic
		}
		p.SetStyle("dialog", "scrim", StateNormal, StyleDelta{Fill: C(color.RGBA{A: a})})
	}
}
