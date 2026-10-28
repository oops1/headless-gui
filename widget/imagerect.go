package widget

import (
	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	"image"

	geom "github.com/oops1/headless-gui/v3/core/geom"
)

// Опциональные возможности контекста для изображений: частичная отрисовка с
// выбором фильтра и сброс кэша масштабирования. Оба интерфейса — по образцу
// остальных расширений контекста (RoundClipper, ShadowDrawer): реализует
// engine.Canvas, обёртки пробрасывают, а вызывающий пользуется помощниками
// DrawImageRect и InvalidateImageCache, которые сами выбирают запасной путь.

// ImageFilter — ядро передискретизации DrawImageRect. Тип и константы живут
// в core/geom; здесь алиасы, чтобы код приложений не менялся.
type ImageFilter = geom.ImageFilter

const (
	FilterNearest    = geom.FilterNearest
	FilterBilinear   = geom.FilterBilinear
	FilterCatmullRom = geom.FilterCatmullRom
)

// ImageRectDrawer — см. draw.ImageRectDrawer (core/draw); алиас: тот же тип.
type ImageRectDrawer = hgdraw.ImageRectDrawer

// GlyphCoverage — см. draw.GlyphCoverage (core/draw); алиас: тот же тип.
type GlyphCoverage = hgdraw.GlyphCoverage

// ContextHasGlyph спрашивает контекст про глиф руны в шрифте по умолчанию.
// known=false — контекст ответить не умеет.
func ContextHasGlyph(ctx DrawContext, r rune) (has, known bool) {
	if gc, ok := ctx.(GlyphCoverage); ok {
		return gc.HasGlyph(r), true
	}
	return false, false
}

// ImageCacheInvalidator — см. draw.ImageCacheInvalidator (core/draw); алиас: тот же тип.
type ImageCacheInvalidator = hgdraw.ImageCacheInvalidator

// DrawImageRect рисует часть sr изображения src в прямоугольник dr с
// фильтром f (см. ImageRectDrawer). Контекст без поддержки получает
// запасной путь: вырезанная часть через DrawImageScaled — билинейно и
// без выигрыша от клипа.
func DrawImageRect(ctx DrawContext, src image.Image, sr, dr image.Rectangle, f ImageFilter) {
	if src == nil {
		return
	}
	if d, ok := ctx.(ImageRectDrawer); ok {
		d.DrawImageRect(src, sr, dr, f)
		return
	}
	sr, dr = ClampImageRect(src.Bounds(), sr, dr)
	if sr.Empty() || dr.Empty() {
		return
	}
	ctx.DrawImageScaled(cropImage(src, sr), dr.Min.X, dr.Min.Y, dr.Dx(), dr.Dy())
}

// InvalidateImageCache сбрасывает кэш масштабирования src в контексте (nil —
// весь кэш); контекст без кэша не делает ничего. См. ImageCacheInvalidator.
func InvalidateImageCache(ctx DrawContext, src image.Image) {
	if inv, ok := ctx.(ImageCacheInvalidator); ok {
		inv.InvalidateImageCache(src)
	}
}

// ClampImageRect обрезает sr по границам источника и уменьшает dr
// пропорционально; см. geom.ClampImageRect.
func ClampImageRect(bounds, sr, dr image.Rectangle) (image.Rectangle, image.Rectangle) {
	return geom.ClampImageRect(bounds, sr, dr)
}

// cropImage возвращает src, суженный до r: SubImage, если источник его умеет
// (общие пиксели, без копии), иначе обёртка с урезанными границами.
func cropImage(src image.Image, r image.Rectangle) image.Image {
	if r == src.Bounds() {
		return src
	}
	if s, ok := src.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return s.SubImage(r)
	}
	return &croppedImage{Image: src, r: r}
}

// croppedImage — изображение, видимое через прямоугольник r.
type croppedImage struct {
	image.Image
	r image.Rectangle
}

func (c *croppedImage) Bounds() image.Rectangle { return c.r }
