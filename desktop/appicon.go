// appicon.go — рисование значка приложения нужного размера.
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"
)

// drawAppIcon рисует значок приложения в логическом квадрате r. Если есть
// источник по размеру (AppInfo.IconAt), он получает сторону в ФИЗИЧЕСКИХ
// пикселях — значок 24 px на экране 150 % приходит как 36 px, а не растягивается
// из растра 24 или уменьшается из 64. Иначе рисуется единственная картинка.
func drawAppIcon(ctx widget.DrawContext, icon image.Image, at func(size int) image.Image, r image.Rectangle) {
	if at != nil {
		p := widget.PhysicalRect(ctx, r)
		side := p.X
		if p.Y < side {
			side = p.Y
		}
		if img := at(side); img != nil {
			ctx.DrawImageScaled(img, r.Min.X, r.Min.Y, r.Dx(), r.Dy())
			return
		}
	}
	if icon != nil {
		ctx.DrawImageScaled(icon, r.Min.X, r.Min.Y, r.Dx(), r.Dy())
	}
}
