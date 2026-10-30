package draw

import (
	"image"
	"image/color"
)

// Признак «изображение непрозрачно» для быстрого пути холста (v3.38).
//
// DrawImage/DrawImageScaled/DrawImageRect кладут пиксели в холст композицией
// Over: на каждый пиксель читается его альфа и смешивается с тем, что уже
// лежит в холсте. Просмотрщик же рисует свой буфер окна 3840×2160, целиком
// непрозрачный, — смешивать там нечего, и композиция Src (обычная копия
// строк, параллельно по полосам) в несколько раз быстрее.
//
// Узнать, что источник непрозрачен, движок сам не может дёшево: у
// *image.RGBA есть метод Opaque, но он сканирует все пиксели — на буфере в
// 33 МБ это столько же, сколько сама копия, и каждый кадр. Поэтому признак
// объявляет тот, кто знает, — владелец буфера:
//
//   - по типу непрозрачны *image.YCbCr, *image.Gray, *image.Gray16 и
//     *image.CMYK — их движок опознаёт сам;
//   - всё остальное (*image.RGBA, *image.NRGBA, ...) объявляется явно:
//     OpaqueImage(img) возвращает ту же картинку с пометкой, которую
//     понимает холст движка. Пометка — обещание: ВСЕ пиксели имеют альфу 255.
//     Нарушенное обещание — прозрачные места картинки перепишут фон
//     холста вместо того, чтобы пропустить его.
//
// Контекст, не знающий про пометку (печать, чужие реализации DrawContext),
// видит обычный image.Image и рисует его как раньше.

// OpaqueHinter — изображение, объявляющее себя непрозрачным. Отдельное имя
// метода (не Opaque): у *image.RGBA метод Opaque сканирует пиксели, а здесь
// ответ обязан быть мгновенным.
type OpaqueHinter interface {
	OpaqueHint() bool
}

// OpaqueSource — обёртка над изображением с пометкой «непрозрачно»
// (создаётся через OpaqueImage). Указатель на неё идентичен самому себе, так
// что кэш масштабирования холста работает как с обычным источником; холст
// кэширует и сбрасывает кэш по Image внутри обёртки.
type OpaqueSource struct {
	// Image — обёрнутое изображение.
	Image image.Image
}

// OpaqueImage помечает img как непрозрачное. Результат годится везде, где
// принимается image.Image. Если img уже непрозрачно по типу или помечено —
// возвращается как есть; nil даёт nil.
//
// Повторные вызовы для одного буфера создают новые обёртки: держите
// результат (например, рядом с буфером), а не оборачивайте на каждом кадре.
func OpaqueImage(img image.Image) image.Image {
	if img == nil {
		return nil
	}
	if OpaqueByType(img) {
		return img
	}
	if h, ok := img.(OpaqueHinter); ok && h.OpaqueHint() {
		return img
	}
	return &OpaqueSource{Image: img}
}

// OpaqueByType — изображение непрозрачно по самому типу (без сканирования).
func OpaqueByType(img image.Image) bool {
	switch img.(type) {
	case *image.YCbCr, *image.Gray, *image.Gray16, *image.CMYK:
		return true
	}
	return false
}

// OpaqueHint — пометка «непрозрачно».
func (*OpaqueSource) OpaqueHint() bool { return true }

// Unwrap возвращает обёрнутое изображение.
func (o *OpaqueSource) Unwrap() image.Image { return o.Image }

// ColorModel, Bounds, At — image.Image обёрнутого изображения.
func (o *OpaqueSource) ColorModel() color.Model { return o.Image.ColorModel() }
func (o *OpaqueSource) Bounds() image.Rectangle { return o.Image.Bounds() }
func (o *OpaqueSource) At(x, y int) color.Color { return o.Image.At(x, y) }

// SubImage — часть изображения, если оно умеет; пометка сохраняется.
func (o *OpaqueSource) SubImage(r image.Rectangle) image.Image {
	if s, ok := o.Image.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return &OpaqueSource{Image: s.SubImage(r)}
	}
	return &croppedImage{Image: o, r: r}
}

// RGBA64At — быстрое чтение пикселя для image/draw и растеризаторов, если
// обёрнутое изображение его умеет.
func (o *OpaqueSource) RGBA64At(x, y int) color.RGBA64 {
	if r, ok := o.Image.(image.RGBA64Image); ok {
		return r.RGBA64At(x, y)
	}
	cr, cg, cb, ca := o.Image.At(x, y).RGBA()
	return color.RGBA64{R: uint16(cr), G: uint16(cg), B: uint16(cb), A: uint16(ca)}
}

// IsOpaqueImage сообщает, объявлено ли изображение непрозрачным: по типу или
// пометкой OpaqueImage. Не сканирует пиксели. Возвращает и само изображение
// без обёртки (для обычного источника — его же).
func IsOpaqueImage(img image.Image) (inner image.Image, opaque bool) {
	if o, ok := img.(*OpaqueSource); ok && o != nil {
		return o.Image, true
	}
	if OpaqueByType(img) {
		return img, true
	}
	if h, ok := img.(OpaqueHinter); ok && h.OpaqueHint() {
		return img, true
	}
	return img, false
}

// croppedImage — изображение, видимое через прямоугольник r (у widget — свой
// такой же для cropImage).
type croppedImage struct {
	image.Image
	r image.Rectangle
}

func (c *croppedImage) Bounds() image.Rectangle { return c.r }
