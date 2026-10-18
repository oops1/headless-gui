package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// E15: панорамирование большой картинки. Просмотрщик отдаёт движку свой
// непрозрачный буфер окна размером с холст, и в каждом кадре у него новое
// содержимое: стоимость кадра = копирование буфера в холст + сравнение кадров
// + сборка изменённых тайлов (в 4K это 33 МБ на кадр).

// panBuffers — два буфера окна с разным содержимым (чередуются по кадрам).
func panBuffers(w, h int) [2]*image.RGBA {
	var out [2]*image.RGBA
	for k := range out {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			row := img.Pix[y*img.Stride : y*img.Stride+w*4]
			for x := 0; x < w; x++ {
				row[x*4+0] = uint8(x*3 + k*40)
				row[x*4+1] = uint8(y*5 + k*70)
				row[x*4+2] = uint8((x ^ y) + k)
				row[x*4+3] = 255
			}
		}
		out[k] = img
	}
	return out
}

func benchPan(b *testing.B, w, h int, stretch widget.ImageStretch, opaqueHint, recycle bool) {
	eng := New(w, h, 20)
	b.Cleanup(eng.Stop)
	eng.SetFrameRecycling(recycle)
	bufs := panBuffers(w, h)
	var imgs [2]image.Image
	for k := range imgs {
		imgs[k] = bufs[k]
		if opaqueHint {
			imgs[k] = widget.OpaqueImage(bufs[k])
		}
	}
	iw := widget.NewImageWidget()
	iw.Stretch = stretch
	iw.SetBounds(image.Rect(0, 0, w, h))
	iw.SetImage(imgs[0])
	eng.SetRoot(iw)
	eng.renderFrame().Release()
	b.ReportAllocs()
	b.SetBytes(int64(w * h * 4))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iw.SetImage(imgs[(i+1)&1])
		fr := eng.renderFrame()
		if len(fr.Tiles) == 0 {
			b.Fatal("кадр без тайлов")
		}
		fr.Release()
	}
}

func BenchmarkPan4K_Over(b *testing.B) {
	benchPan(b, 3840, 2160, widget.ImageStretchNone, false, false)
}
func BenchmarkPan4K_Opaque(b *testing.B) {
	benchPan(b, 3840, 2160, widget.ImageStretchNone, true, false)
}
func BenchmarkPan4K_RecycleOnly(b *testing.B) {
	benchPan(b, 3840, 2160, widget.ImageStretchNone, false, true)
}
func BenchmarkPan4K_OpaqueRecycle(b *testing.B) {
	benchPan(b, 3840, 2160, widget.ImageStretchNone, true, true)
}
func BenchmarkPan4K_FillOver(b *testing.B) {
	benchPan(b, 3840, 2160, widget.ImageStretchFill, false, false)
}
func BenchmarkPan4K_FillOpaqueRecycle(b *testing.B) {
	benchPan(b, 3840, 2160, widget.ImageStretchFill, true, true)
}
