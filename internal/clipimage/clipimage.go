// Package clipimage — кодеки изображения для буфера обмена, общие для Windows,
// X11 и Wayland.
//
// На Linux картинка ходит как image/png (а читается ещё и image/bmp), на
// Windows — как зарегистрированный формат «PNG» наряду с DIB. Чистые функции
// без системных вызовов: выбор формата и разбор проверяет общий прогон тестов.
//
// Данные чужие, поэтому PNG перед разбором проверяется по заголовку: картинка,
// обещающая гигапиксели в сжатом килобайте, не должна заставить выделить
// память.
package clipimage

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"strings"

	"github.com/oops1/headless-gui/v4/internal/dib"
)

// MIME-типы изображения.
const (
	MimePNG = "image/png"
	MimeBMP = "image/bmp"
)

// ErrTooLarge — картинка по заголовку больше допустимого.
var ErrTooLarge = errors.New("clipimage: картинка слишком велика")

// ErrUnknownMime — тип изображения не поддержан.
var ErrUnknownMime = errors.New("clipimage: неизвестный тип изображения")

// IsPNGMime сообщает, что тип — PNG.
func IsPNGMime(m string) bool { return normMime(m) == MimePNG }

// IsBMPMime сообщает, что тип — BMP: помимо image/bmp встречаются
// image/x-bmp и image/x-MS-bmp.
func IsBMPMime(m string) bool {
	switch normMime(m) {
	case MimeBMP, "image/x-bmp", "image/x-ms-bmp":
		return true
	}
	return false
}

// normMime приводит тип к нижнему регистру без параметров.
func normMime(m string) string {
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = m[:i]
	}
	return strings.ToLower(strings.TrimSpace(m))
}

// PickMime выбирает из предложенных владельцем буфера типов тот, что читаем:
// image/png, а при его отсутствии image/bmp (и его синонимы). Возвращает тип
// в точности как он объявлен владельцем — именно его надо запрашивать. Пустая
// строка — читаемого изображения среди предложенного нет.
func PickMime(offered []string) string {
	bmp := ""
	for _, m := range offered {
		if IsPNGMime(m) {
			return m
		}
		if bmp == "" && IsBMPMime(m) {
			bmp = m
		}
	}
	return bmp
}

// DecodeMime разбирает изображение, полученное под типом mime.
func DecodeMime(mime string, data []byte) (image.Image, error) {
	switch {
	case IsPNGMime(mime):
		return DecodePNG(data)
	case IsBMPMime(mime):
		return dib.DecodeBMP(data)
	}
	return nil, ErrUnknownMime
}

// DecodePNG разбирает PNG, предварительно сверив размер по заголовку.
func DecodePNG(data []byte) (image.Image, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 1<<16 || cfg.Height > 1<<16 ||
		cfg.Width*cfg.Height > dib.MaxPixels {
		return nil, ErrTooLarge
	}
	return png.Decode(bytes.NewReader(data))
}

// EncodePNG кодирует картинку в PNG. Сжатие быстрое: буфер обмена — не архив,
// и ждать ради лишних процентов размера, пока пользователь нажал «вставить»,
// незачем.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Clone копирует картинку в *image.NRGBA с началом в (0,0): владеющий буфером
// хранит то, что было на момент копирования, а не то, что приложение рисует
// дальше.
func Clone(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}
