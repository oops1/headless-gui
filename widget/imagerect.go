package widget

import "image"

// Опциональные возможности контекста для изображений: частичная отрисовка с
// выбором фильтра и сброс кэша масштабирования. Оба интерфейса — по образцу
// остальных расширений контекста (RoundClipper, ShadowDrawer): реализует
// engine.Canvas, обёртки пробрасывают, а вызывающий пользуется помощниками
// DrawImageRect и InvalidateImageCache, которые сами выбирают запасной путь.

// ImageFilter — ядро передискретизации DrawImageRect.
type ImageFilter uint8

const (
	// FilterNearest — ближайший пиксель: без сглаживания, быстрее всех. Для
	// увеличения «до пикселей» (просмотр картинки в масштабе 800 %) и пиксель-арта.
	FilterNearest ImageFilter = iota
	// FilterBilinear — билинейное ядро (как у DrawImageScaled). При уменьшении
	// усредняет все накрытые пиксели источника, а не только соседние.
	FilterBilinear
	// FilterCatmullRom — бикубическое ядро Кэтмулла-Рома: чётче билинейного,
	// заметно медленнее; даёт небольшой «звон» у резких границ.
	FilterCatmullRom
)

// String возвращает имя фильтра.
func (f ImageFilter) String() string {
	switch f {
	case FilterNearest:
		return "nearest"
	case FilterBilinear:
		return "bilinear"
	case FilterCatmullRom:
		return "catmullrom"
	}
	return "unknown"
}

// ImageRectDrawer — опциональная возможность контекста рисовать часть
// изображения в прямоугольник с выбором фильтра, масштабируя только ту часть,
// что попадает под текущий клип.
//
// В отличие от DrawImageScaled, которому нужен весь источник и весь целевой
// размер (и кэш ради этого), DrawImageRect работает за время, пропорциональное
// ВИДИМОЙ площади: картинка 12000×8000 на зуме 400 % в окне 1200×800 стоит
// как 1200×800 пикселей, а не как 48000×32000.
//
//   - sr — прямоугольник в координатах источника (пиксели src); часть за
//     src.Bounds() отбрасывается вместе с пропорциональной частью dr;
//   - dr — прямоугольник назначения в ЛОГИЧЕСКИХ координатах контекста (как
//     x, y, w, h у DrawImageScaled); он может быть сколь угодно больше окна
//     и уходить за его края;
//   - f — фильтр.
//
// Реализуется engine.Canvas. Результат не кэшируется. Вызывающему удобнее
// помощник DrawImageRect: контексту без интерфейса он подставит
// DrawImageScaled от вырезанной части (с билинейным ядром и без выигрыша в
// скорости).
type ImageRectDrawer interface {
	DrawImageRect(src image.Image, sr, dr image.Rectangle, f ImageFilter)
}

// ImageCacheInvalidator — опциональная возможность контекста сбросить кэш
// масштабированных изображений (реализуется engine.Canvas). Нужна после
// того, как пиксели картинки переписаны на месте: кэш ключует источник по
// идентичности и содержимого не сравнивает.
type ImageCacheInvalidator interface {
	InvalidateImageCache(src image.Image)
}

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

// ClampImageRect обрезает sr по границам источника bounds и уменьшает dr
// пропорционально, чтобы оставшаяся часть картинки не растянулась. Любой из
// результатов пуст — рисовать нечего.
func ClampImageRect(bounds, sr, dr image.Rectangle) (image.Rectangle, image.Rectangle) {
	if sr.Empty() || dr.Empty() {
		return image.Rectangle{}, image.Rectangle{}
	}
	c := sr.Intersect(bounds)
	if c.Empty() {
		return image.Rectangle{}, image.Rectangle{}
	}
	if c == sr {
		return sr, dr
	}
	sw, sh := int64(sr.Dx()), int64(sr.Dy())
	dw, dh := int64(dr.Dx()), int64(dr.Dy())
	// Доля источника, отрезанная с каждой стороны, переводится в пиксели dr с
	// округлением к ближайшему.
	scaleTo := func(n, d, s int64) int {
		return int((n*d + s/2) / s)
	}
	out := image.Rect(
		dr.Min.X+scaleTo(int64(c.Min.X-sr.Min.X), dw, sw),
		dr.Min.Y+scaleTo(int64(c.Min.Y-sr.Min.Y), dh, sh),
		dr.Max.X-scaleTo(int64(sr.Max.X-c.Max.X), dw, sw),
		dr.Max.Y-scaleTo(int64(sr.Max.Y-c.Max.Y), dh, sh),
	)
	return c, out
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
