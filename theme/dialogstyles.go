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

// addDialogStyles дописывает профилю p затемнение диалога, если он его не
// объявил: чёрное, полупрозрачное — стол остаётся виден сквозь него.
//
// Заголовок диалога профиль не объявляет: при разрешении темы он повторяет
// заголовок активного окна (resolve.go, partFallbacks) — синий с белым текстом
// у Windows 2000, цвета окна у остальных; разновидность получает свой без
// лишнего токена. Объявленное профилем (SetStyle) не перезаписывается.
// Разновидность профиля (Parent задан) наследует затемнение родителя.
func addDialogStyles(p *Profile) {
	if p.Styles == nil {
		return
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
