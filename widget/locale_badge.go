// locale_badge.go — отрисовка индикатора текущей локали в заголовках окон/диалогов.
package widget

import (
	"image/color"
)

// localeBadgeHeight — высота плашки-индикатора локали.
const localeBadgeHeight = 16

// drawLocaleBadge рисует небольшую плашку с текущей локалью (напр. «EN»),
// выровненную по правому краю rightX, вертикально по центру полосы высотой
// barH, начинающейся с координаты top. fg — базовый цвет (берётся плашкой
// для фона/рамки/текста с разной прозрачностью).
//
// Возвращает ширину занятой области (0, если локаль пустая) — вызывающий код
// может учесть её, чтобы не перекрыть текст заголовка.
func drawLocaleBadge(ctx DrawContext, rightX, top, barH int, fg color.RGBA) int {
	label := Locale()
	if label == "" {
		return 0
	}

	const padX = 6
	tw := ctx.MeasureText(label, DefaultFontSizePt)
	badgeW := tw + padX*2
	badgeH := localeBadgeHeight
	if badgeH > barH-4 && barH > 4 {
		badgeH = barH - 4
	}

	bx := rightX - badgeW
	by := top + (barH-badgeH)/2

	// Фон плашки рисуем через FillRectAlpha — настоящее альфа-смешивание поверх
	// заголовка. Обычные FillRect/FillRoundRect копируют цвет в режиме Src (без
	// блендинга), из-за чего полупрозрачный цвет в нативном окне (где альфа
	// буфера игнорируется) превращается в сплошную заливку. Полупрозрачная
	// тёмная подложка слегка затемняет заголовок, а рамка и текст рисуются
	// НЕПРОЗРАЧНЫМ цветом fg, чтобы метка («EN», «RU») всегда была читаемой.
	ctx.FillRectAlpha(bx, by, badgeW, badgeH, color.RGBA{R: 0, G: 0, B: 0, A: 70})
	ctx.DrawBorder(bx, by, badgeW, badgeH, fg)
	ctx.DrawText(label, bx+padX, by+(badgeH-13)/2, fg)

	return badgeW
}
