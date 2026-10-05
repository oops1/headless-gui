package engine

import (
	"image"
	"image/color"
	"testing"
)

// flatCanvas — холст, залитый одним серым: на нём шум виден как отклонение
// от среднего.
func flatCanvas(w, h int, v uint8) *Canvas {
	c := newCanvas(w, h, newFontCache("assets"))
	c.blitBackground()
	c.FillRect(0, 0, w, h, color.RGBA{R: v, G: v, B: v, A: 255})
	return c
}

// TestDrawNoise_BoundedAndGrainy — зерно есть (не все пиксели одинаковы),
// отклонение не больше заявленной амплитуды, а среднее почти не сдвигается.
func TestDrawNoise_BoundedAndGrainy(t *testing.T) {
	c := flatCanvas(100, 100, 128)
	area := image.Rect(10, 10, 90, 90)

	c.DrawNoise(area, 0.04) // пик: 0.04·255 ≈ 10 уровней

	const peak = 11
	distinct := map[uint8]bool{}
	sum, n := 0, 0
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			p := c.back.RGBAAt(x, y)
			if p.R != p.G || p.G != p.B {
				t.Fatalf("шум окрасил пиксель (%d,%d): %v — он должен быть монохромным", x, y, p)
			}
			d := int(p.R) - 128
			if d < -peak || d > peak {
				t.Fatalf("отклонение %d в (%d,%d) больше амплитуды", d, x, y)
			}
			distinct[p.R] = true
			sum += d
			n++
		}
	}
	if len(distinct) < 8 {
		t.Errorf("зерно бедное: всего %d разных значений", len(distinct))
	}
	if mean := float64(sum) / float64(n); mean < -1 || mean > 1 {
		t.Errorf("шум сдвигает среднюю яркость на %.2f", mean)
	}
}

// TestDrawNoise_Deterministic — одна и та же сцена даёт те же байты, а зерно
// привязано к пикселям кадра, а не к области: два пересекающихся слоя не
// спорят о значении в общем пикселе.
func TestDrawNoise_Deterministic(t *testing.T) {
	a := flatCanvas(80, 40, 100)
	b := flatCanvas(80, 40, 100)
	a.DrawNoise(image.Rect(0, 0, 80, 40), 0.05)
	b.DrawNoise(image.Rect(20, 0, 80, 40), 0.05)

	for y := 0; y < 40; y++ {
		for x := 20; x < 80; x++ {
			if a.back.RGBAAt(x, y) != b.back.RGBAAt(x, y) {
				t.Fatalf("зерно в (%d,%d) зависит от границ области", x, y)
			}
		}
	}
	if b.back.RGBAAt(5, 5) != (color.RGBA{R: 100, G: 100, B: 100, A: 255}) {
		t.Error("шум вышел за свою область")
	}
}

// TestDrawNoise_NoOpCases — нулевая амплитуда и пустая область ничего не
// меняют.
func TestDrawNoise_NoOpCases(t *testing.T) {
	c := flatCanvas(40, 40, 90)
	before := append([]uint8(nil), c.back.Pix...)
	c.DrawNoise(image.Rect(0, 0, 40, 40), 0)
	c.DrawNoise(image.Rect(0, 0, 40, 40), -1)
	c.DrawNoise(image.Rectangle{}, 0.1)
	for i := range before {
		if before[i] != c.back.Pix[i] {
			t.Fatal("пустой вызов DrawNoise изменил кадр")
		}
	}
}

// TestDrawNoise_RespectsClips — зерно не выходит ни за прямоугольное, ни за
// скруглённое отсечение.
func TestDrawNoise_RespectsClips(t *testing.T) {
	c := flatCanvas(120, 120, 100)
	c.SetClip(image.Rect(0, 0, 60, 120))
	c.DrawNoise(image.Rect(0, 0, 120, 120), 0.1)
	c.ClearClip()
	for y := 0; y < 120; y++ {
		if p := c.back.RGBAAt(100, y); p.R != 100 {
			t.Fatalf("шум вышел за прямоугольное отсечение: (100,%d) = %v", y, p)
		}
	}

	c2 := flatCanvas(120, 120, 100)
	c2.SetRoundClip(image.Rect(0, 0, 120, 120), 40)
	c2.DrawNoise(image.Rect(0, 0, 120, 120), 0.1)
	c2.ClearClip()
	if p := c2.back.RGBAAt(1, 1); p.R != 100 {
		t.Errorf("шум залез в срезанный угол: %v", p)
	}
	touched := false
	for x := 50; x < 70; x++ {
		if c2.back.RGBAAt(x, 60).R != 100 {
			touched = true
		}
	}
	if !touched {
		t.Error("внутри скруглённого клипа шума нет")
	}
}

// TestDrawNoise_KeepsPremultipliedInvariant — на полупрозрачном буфере канал
// не выходит за альфу, а полностью прозрачные пиксели остаются прозрачными.
func TestDrawNoise_KeepsPremultipliedInvariant(t *testing.T) {
	c := newCanvas(60, 20, newFontCache("assets"))
	c.blitBackground()
	for x := 0; x < 60; x++ {
		for y := 0; y < 20; y++ {
			switch {
			case x < 20: // прозрачный
				c.back.SetRGBA(x, y, color.RGBA{})
			case x < 40: // полупрозрачный белый: R,G,B == A
				c.back.SetRGBA(x, y, color.RGBA{R: 40, G: 40, B: 40, A: 40})
			default:
				c.back.SetRGBA(x, y, color.RGBA{R: 5, G: 5, B: 5, A: 255})
			}
		}
	}
	c.DrawNoise(image.Rect(0, 0, 60, 20), 0.3)
	for y := 0; y < 20; y++ {
		for x := 0; x < 60; x++ {
			p := c.back.RGBAAt(x, y)
			if p.R > p.A || p.G > p.A || p.B > p.A {
				t.Fatalf("(%d,%d): канал больше альфы: %v", x, y, p)
			}
			if x < 20 && p != (color.RGBA{}) {
				t.Fatalf("прозрачный пиксель (%d,%d) стал %v", x, y, p)
			}
		}
	}
}
