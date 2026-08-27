package widget

import (
	"image"
	"image/color"
)

// DrawBevel рисует объёмную рамку Windows 2000 по цветам, заданным темой.
//
// Раньше «классический» вид жил ветками `if Classic3D` внутри каждого
// виджета, а цвета брались из глобальной темы. Теперь то же самое рисуется
// по данным: тема отдаёт тройку цветов в Style.Bevel, а компонент только
// передаёт их сюда и не знает, какая тема активна.
//
// sunken=false — выпуклая рамка (кнопка в покое): светлая грань сверху и
// слева, тёмная снизу и справа. sunken=true — утопленная (поле ввода,
// дорожка прогресса): грани меняются местами.
func DrawBevel(ctx DrawContext, r image.Rectangle, light, shadow, dark color.RGBA, sunken bool) {
	if r.Dx() < 2 || r.Dy() < 2 {
		return
	}
	outerTL, outerBR := light, dark
	innerTL, innerBR := light, shadow
	if sunken {
		outerTL, outerBR = shadow, light
		innerTL, innerBR = dark, light
	}
	x, y, w, h := r.Min.X, r.Min.Y, r.Dx(), r.Dy()

	// Внешняя грань.
	ctx.DrawHLine(x, y, w, outerTL)
	ctx.DrawVLine(x, y, h, outerTL)
	ctx.DrawHLine(x, y+h-1, w, outerBR)
	ctx.DrawVLine(x+w-1, y, h, outerBR)

	if w < 4 || h < 4 {
		return
	}
	// Внутренняя грань — от неё рамка и читается объёмной.
	ctx.DrawHLine(x+1, y+1, w-2, innerTL)
	ctx.DrawVLine(x+1, y+1, h-2, innerTL)
	ctx.DrawHLine(x+1, y+h-2, w-2, innerBR)
	ctx.DrawVLine(x+w-2, y+1, h-2, innerBR)
}
