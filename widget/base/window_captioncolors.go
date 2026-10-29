package base

import "image/color"

// window_captioncolors.go — цвета кнопок заголовка widget.Window (не
// классического): значки «свернуть/развернуть/закрыть» и подсветка кнопки под
// указателем.
//
// Раньше значки рисовались зашитым серым 180, а подсветка — серой плёнкой
// (80,80,80,100), записанной без смешивания. На тёмном заголовке это читалось,
// а на светлом (светлый цвет акцента, светлая тема) значки сливались с фоном, и
// подсветка выходила грязно-серой. Теперь цвета выводятся из заголовка.

// captionGlyphLegacy — прежний цвет значков кнопок заголовка. Остаётся там, где
// он по-прежнему читается: кадры окон с тёмным заголовком не меняются.
var captionGlyphLegacy = color.RGBA{R: 180, G: 180, B: 180, A: 255}

// captionGlyphMinContrast — наименьший контраст значка с фоном заголовка:
// порог WCAG 2 для несущих смысл элементов интерфейса (не текста).
const captionGlyphMinContrast = 3.0

// captionHoverAlpha — доля подсветки кнопки под указателем: 15 % (из 10–20 %,
// как в Windows).
const captionHoverAlpha = 38

// captionGlyphColor — цвет значков кнопок заголовка с фоном bg и текстом text
// (для неактивного окна оба уже приглушены titleColors).
//
// Прежний серый, если он читается на фоне (контраст не меньше 3:1); иначе цвет
// текста заголовка — на светлом акценте значки тёмные, как и надпись; и если
// текст почему-то не читается на собственном фоне — чёрный или белый, что
// контрастнее.
func captionGlyphColor(text, bg color.RGBA) color.RGBA {
	bg.A = 255
	if wcagContrast(captionGlyphLegacy, bg) >= captionGlyphMinContrast {
		return captionGlyphLegacy
	}
	text.A = 255
	if wcagContrast(text, bg) >= captionGlyphMinContrast {
		return text
	}
	black, white := color.RGBA{A: 255}, color.RGBA{R: 255, G: 255, B: 255, A: 255}
	if wcagContrast(black, bg) >= wcagContrast(white, bg) {
		return black
	}
	return white
}

// captionHoverPlate — плёнка подсветки кнопки под указателем: затемнение
// светлого заголовка и осветление тёмного на 15 %. Цвет — alpha-premultiplied,
// для FillRectAlpha (простая FillRect записала бы его как есть, без смешивания).
func captionHoverPlate(bg color.RGBA) color.RGBA {
	if wcagLuminance(bg) > 0.4 {
		return color.RGBA{A: captionHoverAlpha} // чёрная плёнка
	}
	return color.RGBA{R: captionHoverAlpha, G: captionHoverAlpha, B: captionHoverAlpha, A: captionHoverAlpha}
}
