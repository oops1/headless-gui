package widget

// clipboard_image.go — изображение в буфере обмена.
//
// Буфер умел текст, оформленный текст и список файлов; картинку «Копировать
// изображение» просмотрщика положить было некуда, ни одна программа её не
// получала. Здесь буфер получает настоящее изображение — то, что понимают
// Paint, GIMP, LibreOffice и браузеры.
//
// Устроено так же, как файлы: необязательное расширение провайдера, которое
// реализуют системные буферы (Windows, Wayland, X11) и буфер в памяти;
// провайдер приложения может его не поддерживать — тогда ClipboardSetImage
// честно возвращает ErrClipboardImageUnsupported.
//
// На проводе:
//   - Windows: CF_DIBV5 (32 бита BGRA, не премультиплицированный), CF_DIB
//     (24 бита) и зарегистрированный формат «PNG»; читается в порядке
//     PNG → CF_DIBV5 → CF_DIB;
//   - X11 и Wayland: image/png, отдаётся лениво — кодируется, когда у нас его
//     попросили; читается image/png, а при его отсутствии image/bmp.

import (
	"errors"
	"image"

	"github.com/oops1/headless-gui/v3/internal/clipimage"
)

// ErrClipboardImageUnsupported возвращается ClipboardSetImage, когда текущий
// провайдер буфера обмена изображений не умеет (приложение подставило свой,
// реализующий только ClipboardProvider, либо платформа без поддержки).
var ErrClipboardImageUnsupported = errors.New("widget: буфер обмена не поддерживает изображения")

// ErrClipboardImageEmpty возвращается ClipboardSetImage для nil и пустой
// картинки: класть в буфер нечего.
var ErrClipboardImageEmpty = errors.New("widget: пустое изображение для буфера обмена")

// ClipboardImageProvider — необязательное расширение ClipboardProvider: буфер,
// умеющий изображения.
type ClipboardImageProvider interface {
	// SetImage кладёт изображение в буфер, заменяя его содержимое. Картинка
	// копируется: менять её после вызова можно, в буфер уйдёт то, что было.
	SetImage(img image.Image) error
	// GetImage возвращает изображение из буфера; ok == false, если в буфере
	// не изображение (или оно не декодируется).
	GetImage() (img image.Image, ok bool)
}

// ClipboardSetImage кладёт изображение в системный буфер обмена, заменяя его
// содержимое, как ClipboardSetText.
//
// Прозрачность сохраняется там, где формат её несёт (PNG, CF_DIBV5); в
// 24-битном CF_DIB, который берут старые программы, она смешивается с белым.
// Возвращает ErrClipboardImageUnsupported, если провайдер изображений не
// умеет, и ErrClipboardImageEmpty для пустой картинки.
func ClipboardSetImage(img image.Image) error {
	if img == nil || img.Bounds().Empty() {
		return ErrClipboardImageEmpty
	}
	if p, ok := defaultClipboard.(ClipboardImageProvider); ok {
		return p.SetImage(img)
	}
	return ErrClipboardImageUnsupported
}

// ClipboardImage читает изображение из системного буфера обмена. ok == false,
// если в буфере не изображение или провайдер их не умеет.
//
// Вызов может ЖДАТЬ: владелец буфера отдаёт данные, когда готов (на X11 и
// Wayland — до 300 мс на ответ владельца, дальше столько же на каждый кусок
// большой картинки), а картинка потом декодируется. Поэтому звать его стоит не
// на горутине движка (там рисуется кадр), а в своей, с результатом через
// Engine.Post — как ClipboardFiles.
func ClipboardImage() (img image.Image, ok bool) {
	if p, isImg := defaultClipboard.(ClipboardImageProvider); isImg {
		return p.GetImage()
	}
	return nil, false
}

// SetImage — реализация ClipboardImageProvider в памяти.
func (c *memoryClipboard) SetImage(img image.Image) error {
	if img == nil || img.Bounds().Empty() {
		return ErrClipboardImageEmpty
	}
	cp := clipimage.Clone(img)
	c.mu.Lock()
	old := c.virt
	c.text, c.html, c.files, c.cut, c.virt = "", "", nil, false, nil
	c.img = cp
	c.mu.Unlock()
	old.drop()
	return nil
}

// GetImage — реализация ClipboardImageProvider в памяти. Отдаёт копию.
func (c *memoryClipboard) GetImage() (image.Image, bool) {
	c.mu.Lock()
	img := c.img
	c.mu.Unlock()
	if img == nil {
		return nil, false
	}
	return clipimage.Clone(img), true
}

// clipImageSource — один из форматов, в которых изображение лежит в буфере: как
// его достать (пусто — такого формата нет) и как разобрать.
type clipImageSource struct {
	get    func() []byte
	decode func([]byte) (image.Image, error)
}

// firstDecodableImage перебирает форматы по порядку предпочтения и возвращает
// первое изображение, которое удалось разобрать. Формат, который есть, но
// битый, не мешает следующему: чужая программа могла положить хороший CF_DIB
// рядом с испорченным «PNG». Следующие источники не запрашиваются, как только
// нашёлся годный.
func firstDecodableImage(sources ...clipImageSource) (image.Image, bool) {
	for _, s := range sources {
		data := s.get()
		if len(data) == 0 {
			continue
		}
		if img, err := s.decode(data); err == nil && img != nil && !img.Bounds().Empty() {
			return img, true
		}
	}
	return nil, false
}
