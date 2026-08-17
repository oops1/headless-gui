package window

import (
	"image"
	"testing"
)

// TestConvRectBGRX проверяет, что конвертируется ровно прямоугольник dirty:
// внутри — RGBA→BGRX с X=0xFF, снаружи — байты не тронуты.
func TestConvRectBGRX(t *testing.T) {
	const w, h = 8, 6
	srcStride := w * 4
	dstStride := w * 4

	src := make([]byte, srcStride*h)
	for i := range src {
		src[i] = byte(i*7 + 1)
	}
	dst := make([]byte, dstStride*h)
	for i := range dst {
		dst[i] = 0xAA
	}

	r := image.Rect(2, 1, 6, 4)
	convRectBGRX(dst, dstStride, src, srcStride, r)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := y*dstStride + x*4
			if !image.Pt(x, y).In(r) {
				for k := 0; k < 4; k++ {
					if dst[o+k] != 0xAA {
						t.Fatalf("пиксель (%d,%d) вне dirty затёрт: %v", x, y, dst[o:o+4])
					}
				}
				continue
			}
			s := y*srcStride + x*4
			want := [4]byte{src[s+2], src[s+1], src[s+0], 0xFF}
			got := [4]byte{dst[o], dst[o+1], dst[o+2], dst[o+3]}
			if got != want {
				t.Fatalf("пиксель (%d,%d) = %v, ожидалось %v", x, y, got, want)
			}
		}
	}
}

// TestConvRectBGRXStrides проверяет разные шаги строк src и dst.
func TestConvRectBGRXStrides(t *testing.T) {
	srcStride, dstStride := 6*4, 10*4
	src := make([]byte, srcStride*4)
	dst := make([]byte, dstStride*4)
	for i := range src {
		src[i] = byte(i + 3)
	}
	r := image.Rect(1, 1, 4, 3)
	convRectBGRX(dst, dstStride, src, srcStride, r)

	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			s := y*srcStride + x*4
			o := y*dstStride + x*4
			if dst[o] != src[s+2] || dst[o+2] != src[s] || dst[o+3] != 0xFF {
				t.Fatalf("(%d,%d): dst=%v src=%v", x, y, dst[o:o+4], src[s:s+4])
			}
		}
	}
}

// TestConvRectBGRXGuards проверяет, что пустой прямоугольник и выход за буфер
// не приводят к панике.
func TestConvRectBGRXGuards(t *testing.T) {
	src := make([]byte, 4*4)
	dst := make([]byte, 4*4)
	convRectBGRX(dst, 16, src, 16, image.Rectangle{})
	convRectBGRX(dst, 16, src, 16, image.Rect(0, 0, 100, 100))
	convRectBGRX(dst, 0, src, 16, image.Rect(0, 0, 1, 1))
	convRectBGRX(nil, 16, nil, 16, image.Rect(0, 0, 1, 1))
}
