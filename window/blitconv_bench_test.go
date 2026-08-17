//go:build windows

package window

import (
	"image"
	"image/color"
	"testing"
)

// blitconv_bench_test.go — бенчмарки конвертации кадра RGBA→BGRA (PERF-2).
//
// Сравниваются три варианта:
//   - Old      — прежний код BlitRGBADirty: побайтовая перестановка с индексной
//     арифметикой на каждый пиксель + переворот по Y (bottom-up DIB);
//   - RowsFlip — новый построчный 32-битный swap с переворотом по Y;
//   - TopDown  — новый построчный 32-битный swap БЕЗ переворота (BiHeight<0).
//
// Размер кадра — 1920×1080 (≈2.07 млн пикселей, 8.3 МБ записи).

const (
	benchFrameW = 1920
	benchFrameH = 1080
)

// benchFrame готовит исходный RGBA-кадр и приёмный буфер нужного размера.
func benchFrame(tb testing.TB) (*image.RGBA, []byte) {
	tb.Helper()
	img := image.NewRGBA(image.Rect(0, 0, benchFrameW, benchFrameH))
	for i := 0; i < len(img.Pix); i++ {
		img.Pix[i] = byte(i * 7)
	}
	return img, make([]byte, benchFrameW*benchFrameH*4)
}

// convertOld — точная копия прежнего цикла конвертации (эталон «до»).
func convertOld(dst []byte, img *image.RGBA, dirty image.Rectangle, width, height int) {
	src := img.Pix
	stride := img.Stride
	for y := dirty.Min.Y; y < dirty.Max.Y; y++ {
		srcRow := src[y*stride:]
		dstOff := (height - 1 - y) * width * 4
		for x := dirty.Min.X; x < dirty.Max.X; x++ {
			si := x * 4
			di := dstOff + x*4
			dst[di+0] = srcRow[si+2] // B
			dst[di+1] = srcRow[si+1] // G
			dst[di+2] = srcRow[si+0] // R
			dst[di+3] = srcRow[si+3] // A
		}
	}
}

// convertRowsFlip — новый построчный swap, но с переворотом по Y (bottom-up).
func convertRowsFlip(dst []byte, img *image.RGBA, dirty image.Rectangle, width, height int) {
	src := img.Pix
	stride := img.Stride
	x0, x1 := dirty.Min.X*4, dirty.Max.X*4
	for y := dirty.Min.Y; y < dirty.Max.Y; y++ {
		s := src[y*stride+x0 : y*stride+x1]
		d0 := (height-1-y)*width*4 + x0
		swapRBRow(dst[d0:d0+len(s)], s)
	}
}

func BenchmarkBlitConvertOld(b *testing.B) {
	img, dst := benchFrame(b)
	r := img.Bounds()
	b.SetBytes(int64(len(dst)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		convertOld(dst, img, r, benchFrameW, benchFrameH)
	}
}

func BenchmarkBlitConvertRowsFlip(b *testing.B) {
	img, dst := benchFrame(b)
	r := img.Bounds()
	b.SetBytes(int64(len(dst)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		convertRowsFlip(dst, img, r, benchFrameW, benchFrameH)
	}
}

func BenchmarkBlitConvertTopDown(b *testing.B) {
	img, dst := benchFrame(b)
	r := img.Bounds()
	b.SetBytes(int64(len(dst)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		convertFrameToBGRA(dst, img, r, benchFrameW)
	}
}

// BenchmarkBlitConvertDirtyStripe — типовой частичный кадр: полоса 1920×64
// (одна строка тайлов). Проверяет, что dirty-путь не деградировал.
func BenchmarkBlitConvertDirtyStripe(b *testing.B) {
	img, dst := benchFrame(b)
	r := image.Rect(0, 500, benchFrameW, 564)
	b.SetBytes(int64(r.Dx() * r.Dy() * 4))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		convertFrameToBGRA(dst, img, r, benchFrameW)
	}
}

// TestConvertFrameToBGRA_MatchesReference — новый конвертер даёт БАЙТ-В-БАЙТ те
// же пиксели, что и эталонная побайтовая формула (порядок строк top-down).
func TestConvertFrameToBGRA_MatchesReference(t *testing.T) {
	const w, h = 37, 19 // нечётные размеры — ловим ошибки хвоста строки
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x*7 + y), G: uint8(y*3 + x), B: uint8(x * y), A: uint8(200 + x%56),
			})
		}
	}

	rects := []image.Rectangle{
		img.Bounds(),
		image.Rect(0, 0, 1, 1),
		image.Rect(5, 3, 30, 15),
		image.Rect(w-1, h-1, w, h),
	}
	for _, r := range rects {
		got := make([]byte, w*h*4)
		convertFrameToBGRA(got, img, r, w)

		want := make([]byte, w*h*4)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				si := y*img.Stride + x*4
				di := (y*w + x) * 4 // top-down
				want[di+0] = img.Pix[si+2]
				want[di+1] = img.Pix[si+1]
				want[di+2] = img.Pix[si+0]
				want[di+3] = img.Pix[si+3]
			}
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("rect %v: байт %d: got %d, want %d", r, i, got[i], want[i])
			}
		}
	}
}
