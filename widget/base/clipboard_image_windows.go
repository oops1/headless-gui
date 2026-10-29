//go:build windows

package base

// clipboard_image_windows.go — изображение в буфере обмена Windows.
//
// Кладутся сразу три формата, от самого описательного к самому бедному — так
// программа, перебирающая форматы по порядку, возьмёт лучший из понятных ей:
//
//   - «PNG» — зарегистрированный формат (его понимают Chrome, Firefox, GIMP,
//     Office, Telegram): без потерь, с альфой;
//   - CF_DIBV5 — 32 бита BGRA, BI_BITFIELDS, альфа НЕ премультиплицирована
//     (Paint, Photoshop);
//   - CF_DIB — 24 бита, прозрачность смешана с белым (старые программы).
//
// Читается в том же порядке: PNG → CF_DIBV5 → CF_DIB. Блоки DIB собирает и
// разбирает internal/dib — чистые функции, проверяемые общим прогоном тестов.

import (
	"errors"
	"image"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v3/internal/clipimage"
	"github.com/oops1/headless-gui/v3/internal/dib"
)

const (
	cfDib   = 8
	cfDibV5 = 17
)

// pngFormat — идентификатор формата «PNG» (регистрируется при первом обращении).
var (
	pngFormatOnce sync.Once
	pngFormatID   uintptr
)

func pngFormat() uintptr {
	pngFormatOnce.Do(func() {
		name, _ := syscall.UTF16PtrFromString("PNG")
		pngFormatID, _, _ = procRegisterFormat.Call(uintptr(unsafe.Pointer(name)))
	})
	return pngFormatID
}

// openClipboardRetry открывает буфер обмена, повторяя попытку: сразу после
// изменения его на миг занимают слушатели (история буфера, удалённый рабочий
// стол), и OpenClipboard отвечает отказом, хотя через десяток миллисекунд
// всё открылось бы. Картинка — не текст: потерять «Копировать изображение» из-за
// такого мига обиднее.
func openClipboardRetry() bool {
	for i := 0; i < 20; i++ {
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// SetImage кладёт изображение в буфер: «PNG», CF_DIBV5 и CF_DIB за одно
// открытие (по той же причине, что SetHTML: иначе между открытиями вклинился
// бы чужой процесс).
//
// Реализует widget.ClipboardImageProvider. Кодирование идёт сразу, а не по
// запросу: у буфера без окна-владельца отложенной выдачи (WM_RENDERFORMAT)
// нет. Для больших картинок это десятки и сотни миллисекунд — звать из
// своей горутины, не из горутины движка.
func (c *winClipboard) SetImage(img image.Image) error {
	if img == nil || img.Bounds().Empty() {
		return ErrClipboardImageEmpty
	}
	snap := clipimage.Clone(img)
	pngData, err := clipimage.EncodePNG(snap)
	if err != nil {
		return err
	}
	v5 := dib.EncodeV5(snap)
	plain := dib.EncodeDIB(snap)

	format := pngFormat()
	if !openClipboardRetry() {
		return errors.New("widget: не удалось открыть буфер обмена")
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()
	if format != 0 {
		setClipboardBytes(format, pngData)
	}
	setClipboardBytes(cfDibV5, v5)
	setClipboardBytes(cfDib, plain)
	return nil
}

// GetImage читает изображение: PNG → CF_DIBV5 → CF_DIB.
//
// Реализует widget.ClipboardImageProvider. Чужой блок, который не
// разобрался, не останавливает перебор — следующий формат может оказаться
// целым.
func (c *winClipboard) GetImage() (image.Image, bool) {
	format := pngFormat()
	if !openClipboardRetry() {
		return nil, false
	}
	defer procCloseClipboard.Call()

	block := func(f uintptr) func() []byte {
		return func() []byte {
			if f == 0 {
				return nil
			}
			h, _, _ := procGetClipboardData.Call(f)
			if h == 0 {
				return nil
			}
			return clipboardBlock(h)
		}
	}
	return firstDecodableImage(
		clipImageSource{block(format), clipimage.DecodePNG},
		clipImageSource{block(cfDibV5), dib.Decode},
		clipImageSource{block(cfDib), dib.Decode},
	)
}
