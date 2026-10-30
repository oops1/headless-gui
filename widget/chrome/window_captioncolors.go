package chrome

import (
	"image/color"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// window_captioncolors.go — цвета кнопок заголовка widget.Window (не
// классического): значки «свернуть/развернуть/закрыть» и подсветка кнопки под
// указателем.
//
// Раньше значки рисовались зашитым серым 180, а подсветка — серой плёнкой
// (80,80,80,100), записанной без смешивания. На тёмном заголовке это читалось,
// а на светлом (светлый цвет акцента, светлая тема) значки сливались с фоном, и
// подсветка выходила грязно-серой. Теперь цвета выводятся из заголовка.

// captionHoverAlpha — доля подсветки кнопки под указателем: 15 % (из 10–20 %,
// как в Windows).
const captionHoverAlpha = 38

// captionHoverPlate — плёнка подсветки кнопки под указателем: затемнение
// светлого заголовка и осветление тёмного на 15 %. Цвет — alpha-premultiplied,
// для FillRectAlpha (простая FillRect записала бы его как есть, без смешивания).
func captionHoverPlate(bg color.RGBA) color.RGBA {
	if base.WCAGLuminance(bg) > 0.4 {
		return color.RGBA{A: captionHoverAlpha} // чёрная плёнка
	}
	return color.RGBA{R: captionHoverAlpha, G: captionHoverAlpha, B: captionHoverAlpha, A: captionHoverAlpha}
}
