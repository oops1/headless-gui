package native

// tray.go — общее для трея бэкендов: отказ без поддержки, пиксели значка.

import (
	"errors"
	"image"
)

// ErrTrayUnsupported возвращается методами трея на платформах без поддержки.
//
//facadegen:skip
var ErrTrayUnsupported = errors.New("window: иконка в трее поддерживается на Windows и Linux")

// IconColorBuffer конвертирует премультиплицированный RGBA в top-down 32bpp
// BGRA-буфер цветного DIB иконки (длина = w*h*4). Порядок R,G,B,A → B,G,R,A.
//
//facadegen:skip
func IconColorBuffer(img *image.RGBA) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			di := (y*w + x) * 4
			out[di+0] = img.Pix[si+2] // B
			out[di+1] = img.Pix[si+1] // G
			out[di+2] = img.Pix[si+0] // R
			out[di+3] = img.Pix[si+3] // A
		}
	}
	return out
}
