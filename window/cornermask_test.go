package window

import (
	"image"
	"testing"
)

func opaqueBGRX(w, h int) []byte {
	b := make([]byte, w*h*4)
	for i := 0; i < len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = 200, 150, 100, 255
	}
	return b
}

func px(b []byte, w, x, y int) [4]byte {
	o := (y*w + x) * 4
	return [4]byte{b[o], b[o+1], b[o+2], b[o+3]}
}

func TestCornerMask_TransparentOutsideOpaqueInside(t *testing.T) {
	const w, h, r = 40, 30, 8
	b := opaqueBGRX(w, h)
	applyCornerMask(b, w*4, w, h, r, image.Rect(0, 0, w, h))

	// Самые углы — полностью прозрачные, все четыре.
	for _, p := range [][2]int{{0, 0}, {w - 1, 0}, {0, h - 1}, {w - 1, h - 1}} {
		if got := px(b, w, p[0], p[1]); got != [4]byte{} {
			t.Errorf("угол %v = %v, ждали (0,0,0,0)", p, got)
		}
	}
	// Внутри и на прямых краях — нетронуты.
	for _, p := range [][2]int{{w / 2, 0}, {0, h / 2}, {w / 2, h / 2}, {r, r}, {w - 1, h / 2}} {
		if got := px(b, w, p[0], p[1]); got != [4]byte{200, 150, 100, 255} {
			t.Errorf("пиксель %v = %v, ждали нетронутый", p, got)
		}
	}
	// На дуге — частичная альфа, цвет домножен (не больше альфы).
	partial := 0
	for y := 0; y < r; y++ {
		for x := 0; x < r; x++ {
			p := px(b, w, x, y)
			if p[3] > 0 && p[3] < 255 {
				partial++
				if p[0] > p[3] || p[1] > p[3] || p[2] > p[3] {
					t.Fatalf("(%d,%d) = %v: цвет больше альфы — не домножен", x, y, p)
				}
			}
		}
	}
	if partial == 0 {
		t.Error("дуга без сглаживания")
	}
}

// Маска трогает только переданную область: соседние пиксели угла остаются
// такими, какими их оставил прошлый блит.
func TestCornerMask_OnlyArea(t *testing.T) {
	const w, h, r = 40, 30, 8
	b := opaqueBGRX(w, h)
	applyCornerMask(b, w*4, w, h, r, image.Rect(20, 10, 40, 30))
	if px(b, w, 0, 0)[3] != 255 {
		t.Error("угол вне области изменён")
	}
	if px(b, w, w-1, h-1) != [4]byte{} {
		t.Error("угол в области не замаскирован")
	}
	// Радиус больше половины окна — урезается, без выхода за буфер.
	small := opaqueBGRX(6, 4)
	applyCornerMask(small, 6*4, 6, 4, 50, image.Rect(0, 0, 6, 4))
	if px(small, 6, 0, 0)[3] == 255 {
		t.Error("маленькое окно: угол не тронут")
	}
}
