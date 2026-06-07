// locale_badge.go — отрисовка индикатора текущей локали в заголовках окон/диалогов.
package widget

import (
	"image"
	"image/color"
)

// localeBadgeHeight — высота плашки-индикатора локали.
const localeBadgeHeight = 16

// drawLocaleBadge рисует небольшую плашку с текущей локалью (напр. «EN»),
// выровненную по правому краю rightX, вертикально по центру полосы высотой
// barH, начинающейся с координаты top. fg — базовый цвет рамки/текста.
//
// Возвращает прямоугольник плашки (пустой, если локаль пустая) — вызывающий код
// использует его для hit-теста клика (открытие контекстного меню выбора локали).
func drawLocaleBadge(ctx DrawContext, rightX, top, barH int, fg color.RGBA) image.Rectangle {
	label := Locale()
	if label == "" {
		return image.Rectangle{}
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

	return image.Rect(bx, by, bx+badgeW, by+badgeH)
}

// drawLocaleMenu рисует выпадающий список выбора локали под плашкой badge.
// highlight — индекс подсвеченного пункта (-1 = нет). cur — текущая локаль.
// Возвращает прямоугольники пунктов (для hit-теста).
func drawLocaleMenu(ctx DrawContext, badge image.Rectangle, items []string, cur string) []image.Rectangle {
	if badge.Empty() || len(items) == 0 {
		return nil
	}
	const itemH = 22
	// Ширина меню — не уже плашки и достаточной для самого длинного пункта.
	w := badge.Dx()
	for _, it := range items {
		iw := ctx.MeasureText(it, DefaultFontSizePt) + 24
		if iw > w {
			w = iw
		}
	}
	x := badge.Min.X
	y := badge.Max.Y + 1
	h := itemH * len(items)

	bg := color.RGBA{R: 45, G: 45, B: 48, A: 255}
	border := color.RGBA{R: 120, G: 120, B: 130, A: 255}
	sel := color.RGBA{R: 0, G: 120, B: 215, A: 255}
	fg := color.RGBA{R: 235, G: 235, B: 235, A: 255}

	ctx.FillRect(x, y, w, h, bg)
	ctx.DrawBorder(x, y, w, h, border)

	rects := make([]image.Rectangle, len(items))
	for i, it := range items {
		iy := y + i*itemH
		rects[i] = image.Rect(x, iy, x+w, iy+itemH)
		if normLocale(it) == normLocale(cur) {
			ctx.FillRect(x+1, iy+1, w-2, itemH-2, sel)
		}
		// Маркер-галочка для текущей локали слева, текст со сдвигом.
		ctx.DrawText(it, x+8, iy+(itemH-13)/2, fg)
	}
	return rects
}

