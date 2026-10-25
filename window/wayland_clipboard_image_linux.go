//go:build linux && !android

package window

// wayland_clipboard_image_linux.go — изображение в буфере обмена Wayland.
//
// Копирование: тот же wl_data_source, что и для текста, с единственным типом
// image/png. PNG кодируется лениво, когда компоновщик передал нам descriptor
// получателя (событие send), — см. handleSourceSend.
//
// Вставка: если offer предлагает image/png, спрашиваем его; иначе image/bmp
// (так отдают часть приложений Qt и X11-программы через XWayland). Чтение идёт
// кусками, с сроком 300 мс на ответ владельца и на каждый следующий кусок.

import (
	"image"

	"github.com/oops1/headless-gui/v3/internal/clipimage"
	"github.com/oops1/headless-gui/v3/widget"
)

var _ widget.ClipboardImageProvider = (*wlClipboard)(nil)

// SetImage отдаёт изображение в системный буфер обмена.
//
// Реализует widget.ClipboardImageProvider. Картинка копируется сразу, а в PNG
// превращается только по запросу получателя. Без serial'а недавнего ввода
// компоновщик set_selection не примет — тогда возвращается ошибка, а не
// тишина: «Копировать изображение» должно сообщить, что не вышло.
func (c *wlClipboard) SetImage(img image.Image) error {
	if img == nil || img.Bounds().Empty() {
		return widget.ErrClipboardImageEmpty
	}
	if c == nil || c.w == nil || c.w.dataDeviceID == 0 || c.w.dataDevMgrID == 0 || c.w.inputSerial.Load() == 0 {
		return widget.ErrClipboardImageUnsupported
	}
	c.setSelection("", "", nil, false, newOwnedImage(img), nil)
	return nil
}

// GetImage читает изображение из системного буфера обмена.
//
// Реализует widget.ClipboardImageProvider. Звать не на горутине движка:
// владелец отвечает, когда готов, а картинка потом декодируется.
func (c *wlClipboard) GetImage() (image.Image, bool) {
	if c == nil || c.w == nil {
		return nil, false
	}
	c.mu.Lock()
	own, offer, mime, src := c.ownedImg, c.offer, c.offerImage, c.source
	c.mu.Unlock()

	// Владеем сами — отвечаем из своей копии.
	if src != 0 && own != nil {
		return own.Image(), true
	}
	if offer == 0 || mime == "" {
		return nil, false
	}
	data := c.receiveIdle(offer, mime, clipboardReadTimeout, maxClipboardImageBytes)
	if len(data) == 0 {
		return nil, false
	}
	img, err := clipimage.DecodeMime(mime, data)
	if err != nil || img == nil || img.Bounds().Empty() {
		return nil, false
	}
	return img, true
}
