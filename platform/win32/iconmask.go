package win32

// iconmask.go — AND-маска значка и курсора из альфы: переносимая, проверяется
// на любой машине.

import (
	"image"
)

// iconMaskBuffer строит 1-битную AND-маску иконки из альфы: бит=1 → пиксель
// прозрачный (a < threshold). Сканлайны выровнены на границу WORD (16 бит),
// как требует GDI CreateBitmap. Возвращает буфер и stride в байтах.
func iconMaskBuffer(img *image.RGBA, threshold uint8) ([]byte, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	stride := ((w + 15) / 16) * 2
	out := make([]byte, stride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := img.Pix[img.PixOffset(b.Min.X+x, b.Min.Y+y)+3]
			if a < threshold {
				out[y*stride+x/8] |= 0x80 >> (uint(x) % 8)
			}
		}
	}
	return out, stride
}
