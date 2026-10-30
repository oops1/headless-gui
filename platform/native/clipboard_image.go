package native

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

	"github.com/oops1/headless-gui/v4/internal/clipimage"
)

const (
	// MaxClipboardImageBytes — предел на изображение, принимаемое из буфера
	// (PNG или BMP в сыром виде): снимок экрана 4K в BMP — 32 МиБ, поэтому
	// общий предел текста (16 МиБ) для картинок мал.
	//
	//facadegen:skip
	MaxClipboardImageBytes = 64 << 20

	// X11IncrChunk — размер куска INCR и порог, выше которого картинка уходит
	// по частям. Запрос ChangeProperty без BIG-REQUESTS ограничен 256 КиБ
	// целиком (длина в 16 битах слов), поэтому кусок заведомо меньше.
	//
	//facadegen:skip
	X11IncrChunk = 128 << 10
)

// OwnedImage — изображение, которым мы владеем в буфере обмена: снимок
// картинки и лениво закодированный PNG.
//
//facadegen:skip
type OwnedImage struct {
	img *image.NRGBA

	once sync.Once
	png  []byte
	err  error
}

// NewOwnedImage копирует картинку: правка оригинала после «Копировать» не
// должна попасть в буфер.
//
//facadegen:skip
func NewOwnedImage(src image.Image) *OwnedImage {
	return &OwnedImage{img: clipimage.Clone(src)}
}

// PNG возвращает закодированную картинку; кодирует при первом обращении.
// Безопасно из нескольких горутин.
func (o *OwnedImage) PNG() ([]byte, error) {
	o.once.Do(func() { o.png, o.err = clipimage.EncodePNG(o.img) })
	return o.png, o.err
}

// Encoded сообщает, закодирован ли уже PNG (тесты ленивой отдачи).
func (o *OwnedImage) Encoded() bool { return o.png != nil }

// Image отдаёт копию снимка: вызывающий вправе её менять.
func (o *OwnedImage) Image() image.Image { return clipimage.Clone(o.img) }
