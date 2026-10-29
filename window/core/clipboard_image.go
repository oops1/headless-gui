package core

// clipboard_image.go — изображение в буфере обмена: общая часть Wayland и X11
// (тестируется в общем прогоне, без соединения).
//
// Картинка ходит как image/png. Отдаётся ЛЕНИВО: пока в буфере лежит снимок
// картинки, PNG не кодируется — он нужен только тому, кто попросил вставить, а
// «Копировать изображение» чаще остаётся без вставки или вставляется один раз.
// Закодированный PNG запоминается: несколько получателей подряд (например,
// менеджер буфера и сама программа) не заставляют кодировать заново.

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/internal/clipimage"
)

const (
	// maxClipboardImageBytes — предел на изображение, принимаемое из буфера
	// (PNG или BMP в сыром виде): снимок экрана 4K в BMP — 32 МиБ, поэтому
	// общий предел текста (16 МиБ) для картинок мал.
	maxClipboardImageBytes = 64 << 20

	// x11IncrChunk — размер куска INCR и порог, выше которого картинка уходит
	// по частям. Запрос ChangeProperty без BIG-REQUESTS ограничен 256 КиБ
	// целиком (длина в 16 битах слов), поэтому кусок заведомо меньше.
	x11IncrChunk = 128 << 10
)

// ownedImage — изображение, которым мы владеем в буфере обмена: снимок
// картинки и лениво закодированный PNG.
type ownedImage struct {
	img *image.NRGBA

	once sync.Once
	png  []byte
	err  error
}

// newOwnedImage копирует картинку: правка оригинала после «Копировать» не
// должна попасть в буфер.
func newOwnedImage(src image.Image) *ownedImage {
	return &ownedImage{img: clipimage.Clone(src)}
}

// PNG возвращает закодированную картинку; кодирует при первом обращении.
// Безопасно из нескольких горутин.
func (o *ownedImage) PNG() ([]byte, error) {
	o.once.Do(func() { o.png, o.err = clipimage.EncodePNG(o.img) })
	return o.png, o.err
}

// Image отдаёт копию снимка: вызывающий вправе её менять.
func (o *ownedImage) Image() image.Image { return clipimage.Clone(o.img) }
