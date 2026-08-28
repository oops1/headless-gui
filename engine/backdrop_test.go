package engine

import (
	"image"
	"image/color"
	"testing"
)

// checkerCanvas — холст в крупную шахматную клетку: на нём размытие видно
// как усреднение соседних клеток.
func checkerCanvas(w, h, cell int) *Canvas {
	c := newCanvas(w, h, newFontCache("assets"))
	c.blitBackground()
	black := color.RGBA{A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 0; y < h; y += cell {
		for x := 0; x < w; x += cell {
			col := black
			if ((x/cell)+(y/cell))%2 == 0 {
				col = white
			}
			c.FillRect(x, y, cell, cell, col)
		}
	}
	return c
}

// TestBlurBehind_MixesWhatIsUnderneath — под слоем остаётся не чёрное и не
// белое, а усреднённое: это и значит «сквозь стекло видно размытое».
func TestBlurBehind_MixesWhatIsUnderneath(t *testing.T) {
	c := checkerCanvas(120, 120, 10)
	area := image.Rect(20, 20, 100, 100)

	c.BlurBehind(area, 12, color.RGBA{})

	mixed := 0
	for y := 40; y < 80; y += 4 {
		for x := 40; x < 80; x += 4 {
			p := c.back.RGBAAt(x, y)
			if p.R > 20 && p.R < 235 {
				mixed++
			}
		}
	}
	if mixed == 0 {
		t.Error("под слоем не осталось смешанных полутонов — размытие не сработало")
	}
}

// TestBlurBehind_LeavesOutsideAlone — за пределами области кадр не тронут.
func TestBlurBehind_LeavesOutsideAlone(t *testing.T) {
	c := checkerCanvas(120, 120, 10)
	before := append([]uint8(nil), c.back.Pix...)
	area := image.Rect(40, 40, 80, 80)

	c.BlurBehind(area, 8, color.RGBA{})

	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			if image.Pt(x, y).In(area) {
				continue
			}
			o := c.back.PixOffset(x, y)
			if c.back.Pix[o] != before[o] {
				t.Fatalf("пиксель (%d,%d) вне области изменён", x, y)
			}
		}
	}
}

// TestBlurBehind_TintPaintsOver — подкраска ложится поверх размытия: именно
// она отличает стекло одной темы от другой.
func TestBlurBehind_TintPaintsOver(t *testing.T) {
	c := checkerCanvas(80, 80, 8)
	area := image.Rect(10, 10, 70, 70)

	// Синяя подкраска, alpha-premultiplied.
	tint := color.RGBA{R: 0, G: 0, B: 120, A: 120}
	c.BlurBehind(area, 6, tint)

	p := c.back.RGBAAt(40, 40)
	if p.B <= p.R {
		t.Errorf("подкраска не легла: %v", p)
	}
}

// TestBlurBehind_NoRadiusIsJustTint — без радиуса остаётся простая
// полупрозрачная заливка, то есть прежнее поведение UseAlpha.
func TestBlurBehind_NoRadiusIsJustTint(t *testing.T) {
	c := checkerCanvas(60, 60, 6)
	before := c.back.RGBAAt(30, 30)

	c.BlurBehind(image.Rect(10, 10, 50, 50), 0, color.RGBA{})
	if got := c.back.RGBAAt(30, 30); got != before {
		t.Errorf("без радиуса и подкраски кадр изменился: %v → %v", before, got)
	}

	c.BlurBehind(image.Rect(10, 10, 50, 50), 0, color.RGBA{R: 100, A: 100})
	if got := c.back.RGBAAt(30, 30); got == before {
		t.Error("без радиуса подкраска не легла")
	}
}

// TestBlurBehind_RespectsClips — подложка уважает и прямоугольное, и
// скруглённое отсечение: стекло получает форму панели, а не её габарита.
func TestBlurBehind_RespectsClips(t *testing.T) {
	c := checkerCanvas(120, 120, 10)
	before := append([]uint8(nil), c.back.Pix...)

	c.SetClip(image.Rect(0, 0, 60, 120))
	c.BlurBehind(image.Rect(0, 0, 120, 120), 10, color.RGBA{})
	c.ClearClip()

	o := c.back.PixOffset(100, 60)
	if c.back.Pix[o] != before[o] {
		t.Error("подложка вышла за прямоугольное отсечение")
	}

	c2 := checkerCanvas(120, 120, 10)
	beforeCorner := c2.back.RGBAAt(2, 2)
	c2.SetRoundClip(image.Rect(0, 0, 120, 120), 30)
	c2.BlurBehind(image.Rect(0, 0, 120, 120), 10, color.RGBA{})
	c2.ClearClip()
	if c2.back.RGBAAt(2, 2) != beforeCorner {
		t.Error("подложка залезла в срезанный угол")
	}
}
