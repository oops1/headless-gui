package widget

import (
	"image"
	"image/color"
)

// focusring.go — общая рамка клавиатурного фокуса.
//
// Диалоги движка обводили кнопку в фокусе своей рамкой в один пиксель цветом
// акцента. На акцентной кнопке (OK, «Да») этот цвет совпадал с заливкой, и
// фокус не был виден вовсе; на обычной он терялся рядом с её собственной
// рамкой. Рамка нужна двухцветная — наружный контур цвета текста элемента и
// внутренний контрастный, — тогда она видна на любом фоне. Так рисует фокус и
// панель задач (desktop.PaintFocusRing); вид объявляет профиль темы токенами
// theme.KeyFocusRing и theme.KeyFocusRingWidth.

// defaultFocusRingWidth — толщина наружного контура, пока тема её не задала.
// Две точки, как у фокуса в WinUI: одна терялась бы рядом с рамкой кнопки.
const defaultFocusRingWidth = 2

// DrawFocusRing рисует общую рамку фокуса по границам r: наружный контур
// толщиной в FocusRingWidth темы (по умолчанию 2) цветом FocusRing темы или,
// если он не задан, цветом base — обычно цветом текста элемента — и внутренний
// контур в один пиксель цвета, контрастного наружному.
//
// corner — радиус скругления; на каждом шаге внутрь он уменьшается на
// единицу, чтобы контуры шли параллельно. Контур строится от внешнего края
// внутрь и не выходит за r, поэтому клип соседа его не обрежет. Прозрачный base
// заменяется белым.
func DrawFocusRing(ctx DrawContext, r image.Rectangle, corner int, base color.RGBA) {
	if r.Empty() {
		return
	}
	st := currentStyle()
	outer := base
	if st.FocusRing.A > 0 {
		outer = st.FocusRing
	}
	outer = focusOpaque(outer)
	w := defaultFocusRingWidth
	if st.FocusRingWidth > 0 {
		w = st.FocusRingWidth
	}
	rr := r
	for i := 0; i < w; i++ {
		strokeFocusRect(ctx, rr, corner, outer)
		rr = rr.Inset(1)
		if corner > 0 {
			corner--
		}
	}
	strokeFocusRect(ctx, rr, corner, focusContrastInk(outer))
}

// strokeFocusRect рисует однопиксельный контур, скруглённый при corner > 0.
func strokeFocusRect(ctx DrawContext, r image.Rectangle, corner int, col color.RGBA) {
	if r.Empty() {
		return
	}
	if corner > 0 {
		ctx.DrawRoundBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), corner, col)
		return
	}
	ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), col)
}

// focusOpaque делает цвет непрозрачным, сохраняя оттенок (цвета темы хранятся
// с предумноженной альфой); полностью прозрачный цвет заменяется белым.
func focusOpaque(c color.RGBA) color.RGBA {
	switch c.A {
	case 255:
		return c
	case 0:
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	un := func(v uint8) uint8 {
		x := int(v) * 255 / int(c.A)
		if x > 255 {
			x = 255
		}
		return uint8(x)
	}
	return color.RGBA{R: un(c.R), G: un(c.G), B: un(c.B), A: 255}
}

// focusContrastInk — чёрный или белый, смотря что виднее на фоне c.
func focusContrastInk(c color.RGBA) color.RGBA {
	lum := (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000
	if lum > 128 {
		return color.RGBA{A: 255}
	}
	return color.RGBA{R: 255, G: 255, B: 255, A: 255}
}
