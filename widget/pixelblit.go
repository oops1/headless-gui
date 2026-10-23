package widget

import (
	"image"
	"math"
)

// PixelBlitter — опциональная возможность контекста положить часть чужого
// *image.RGBA на холст КАК ЕСТЬ: пиксель в пиксель, без масштабирования и
// без кэша (v3.39, E7). Реализует engine.Canvas; обёртки пробрасывают.
//
// В отличие от DrawImage (логический размер = размер картинки в пикселях, при
// HiDPI растягивается на масштаб) здесь единица измерения буфера — ФИЗИЧЕСКИЙ
// пиксель холста. Сессия удалённого рабочего стола идёт в физическом
// разрешении, и растягивать её на 125–200 % значит мылить картинку.
//
//   - src — буфер приложения; sr — часть буфера (в координатах src, то есть
//     относительно src.Rect), которую надо положить; часть за src.Rect
//     отбрасывается;
//   - (x, y) — ЛОГИЧЕСКАЯ точка, в физическую позицию которой попадает пиксель
//     src.Rect.Min (округление по краям, как у остальных операций холста,
//     поэтому буфер стыкуется с соседними виджетами без щелей);
//   - opaque — все пиксели буфера имеют альфу 255 (см. OpaqueImage): тогда
//     область перезаписывается композицией Src, параллельно по полосам;
//     иначе — Over.
//
// Рисование подчиняется текущему клипу (в том числе скруглённому) и тратит
// время пропорционально ВИДИМОЙ части sr. Буфер не копируется и не
// запоминается: после возврата вызывающий снова им владеет.
type PixelBlitter interface {
	BlitPixels(src *image.RGBA, sr image.Rectangle, x, y int, opaque bool)
}

// BlitPixels кладёт часть sr буфера src на холст пиксель в пиксель (см.
// PixelBlitter). Контекст без поддержки получает запасной путь: вырезанная
// часть уходит через DrawImage (масштаб 1) или DrawImageScaled (HiDPI, с
// растяжением) — картинка верна, но копирование идёт прежним путём.
func BlitPixels(ctx DrawContext, src *image.RGBA, sr image.Rectangle, x, y int, opaque bool) {
	if src == nil {
		return
	}
	sr = sr.Intersect(src.Rect)
	if sr.Empty() {
		return
	}
	if b, ok := ctx.(PixelBlitter); ok {
		b.BlitPixels(src, sr, x, y, opaque)
		return
	}
	var img image.Image = src.SubImage(sr)
	if opaque {
		img = OpaqueImage(img)
	}
	k := ContextScale(ctx)
	dx := x + int(math.Round(float64(sr.Min.X-src.Rect.Min.X)/k))
	dy := y + int(math.Round(float64(sr.Min.Y-src.Rect.Min.Y)/k))
	if k == 1 {
		ctx.DrawImage(img, dx, dy)
		return
	}
	w := max(1, int(math.Round(float64(sr.Dx())/k)))
	h := max(1, int(math.Round(float64(sr.Dy())/k)))
	ctx.DrawImageScaled(img, dx, dy, w, h)
}
