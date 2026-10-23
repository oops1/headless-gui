package engine

// blit_pixels.go — копирование части чужого *image.RGBA в back пиксель в
// пиксель (widget.PixelBlitter, v3.39, E7).
//
// DrawImage логически растягивает картинку на HiDPI-масштаб, а кэш
// DrawImageScaled ключует источник по идентичности. Для буфера удалённого
// рабочего стола (3840×2160 = 33 МБ, меняется кусками) нужно обратное: без
// масштабирования, без кэша и только та часть, что попадает под клип. Работа
// ложится на blitImage → blitSrcRect (непрозрачный буфер: копирование строк,
// большие области — параллельно по полосам) или blitOverRect.

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"
)

var _ widget.PixelBlitter = (*Canvas)(nil)

// BlitPixels кладёт часть sr буфера src на холст без масштабирования и без
// копии в кэш. Пиксель src.Rect.Min попадает в физическую позицию
// логической точки (x, y); см. widget.PixelBlitter.
func (c *Canvas) BlitPixels(src *image.RGBA, sr image.Rectangle, x, y int, opaque bool) {
	if src == nil {
		return
	}
	sr = sr.Intersect(src.Rect)
	if sr.Empty() {
		return
	}
	// Физическая позиция src.Rect.Min; остальное — смещение в пикселях.
	origin := image.Pt(c.sx(x)-src.Rect.Min.X, c.sx(y)-src.Rect.Min.Y)
	dst := sr.Add(origin)
	c.markImage(dst)
	vis := c.clampRect(dst).Intersect(c.back.Bounds())
	if c.round.active {
		vis = vis.Intersect(c.round.rect)
	}
	if vis.Empty() {
		return
	}
	c.blitImage(vis, src, vis.Min.Sub(origin), opaque)
}
