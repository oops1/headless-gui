package raster

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
	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	"image"
	"image/color"
	stdraw "image/draw"
)

var _ hgdraw.PixelBlitter = (*Canvas)(nil)

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
	origin := image.Pt(c.PhysX(x)-src.Rect.Min.X, c.PhysX(y)-src.Rect.Min.Y)
	dst := sr.Add(origin)
	c.MarkImage(dst)
	vis := c.clampRect(dst).Intersect(c.back.Bounds())
	if c.round.active {
		vis = vis.Intersect(c.round.rect)
	}
	if vis.Empty() {
		return
	}
	c.blitImage(vis, src, vis.Min.Sub(origin), opaque)
}

var _ hgdraw.PixelBlitterBGRA = (*Canvas)(nil)

// bgraImage — буфер, чьи байты лежат в порядке B,G,R,A (ExternalSurface.
// SetBGRA). blitSrcBand копирует его строки на холст FormatBGRX как есть; At
// отдаёт настоящий цвет — для путей, которые читают пиксели по одному.
type bgraImage struct{ *image.RGBA }

func (b bgraImage) At(x, y int) color.Color {
	c := b.RGBA.RGBAAt(x, y)
	c.R, c.B = c.B, c.R
	return c
}

// RGBA64At — тоже с перестановкой: image/draw читает через него, минуя At, и
// иначе взял бы метод встроенного *image.RGBA.
func (b bgraImage) RGBA64At(x, y int) color.RGBA64 {
	c := b.RGBA.RGBA64At(x, y)
	c.R, c.B = c.B, c.R
	return c
}

// BlitPixelsBGRA — BlitPixels для буфера в порядке B,G,R,A (см. bgraImage).
//
// Непрозрачный буфер на холсте FormatBGRX — копирование строк, без
// перестановки на пиксель: ради этого и вызов. Буфер с альфой там же
// смешивается Over напрямую — смешивание не различает R и B, лишь бы порядок
// был один у обоих; под скруглённым клипом и на холсте RGBA — общим путём.
func (c *Canvas) BlitPixelsBGRA(src *image.RGBA, sr image.Rectangle, x, y int, opaque bool) {
	if src == nil {
		return
	}
	sr = sr.Intersect(src.Rect)
	if sr.Empty() {
		return
	}
	origin := image.Pt(c.PhysX(x)-src.Rect.Min.X, c.PhysX(y)-src.Rect.Min.Y)
	dst := sr.Add(origin)
	c.MarkImage(dst)
	vis := c.clampRect(dst).Intersect(c.back.Bounds())
	if c.round.active {
		vis = vis.Intersect(c.round.rect)
	}
	if vis.Empty() {
		return
	}
	if !opaque && c.format == FormatBGRX && !c.round.active {
		stdraw.Draw(c.back, vis, src, vis.Min.Sub(origin), stdraw.Over)
		return
	}
	c.blitImage(vis, bgraImage{src}, vis.Min.Sub(origin), opaque)
}
