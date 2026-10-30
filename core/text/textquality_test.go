package text

import (
	"image/color"
	"testing"
)

// Таблица контраста: тождественна при нуле; концы и монотонность сохраняются;
// тёмный текст плотнее, светлый на тёмном — слегка тоньше, кривая различна.
func TestTextQuality_ContrastLUT(t *testing.T) {
	if ContrastLUT(color.RGBA{0, 0, 0, 255}, 0) != nil {
		t.Error("при нулевом контрасте таблица не нужна")
	}
	black, white := color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 255}
	for _, steps := range []int{1, 16, 32, 64} {
		for _, col := range []color.RGBA{black, {20, 20, 20, 255}, {99, 99, 99, 255}, {190, 190, 190, 255}, white} {
			lut := ContrastLUT(col, steps)
			if lut == nil {
				continue // яркость сводит подъём на нет
			}
			if lut[0] != 0 || lut[255] != 255 {
				t.Errorf("%v/%d: концы сдвинуты: %d, %d", col, steps, lut[0], lut[255])
			}
			for a := 1; a < 256; a++ {
				if lut[a] < lut[a-1] {
					t.Errorf("%v/%d: таблица убывает на %d", col, steps, a)
				}
			}
		}
	}
	dark, light := ContrastLUT(black, 32), ContrastLUT(white, 32)
	if dark[128] <= 128 {
		t.Errorf("тёмный текст не подтянут: lut[128]=%d", dark[128])
	}
	if light[128] >= 128 {
		t.Errorf("светлый текст на тёмном не утоньшён: lut[128]=%d", light[128])
	}
	// Чем темнее текст, тем сильнее подъём.
	if a, b := ContrastLUT(color.RGBA{20, 20, 20, 255}, 32)[100], ContrastLUT(color.RGBA{99, 99, 99, 255}, 32)[100]; a <= b {
		t.Errorf("подъём не убывает с яркостью: %d vs %d", a, b)
	}
	// Определённость: одни и те же значения при повторном построении.
	if *ContrastLUT(black, 32) != *dark {
		t.Error("таблица нестабильна")
	}
	// Нормализация значения контраста.
	for in, want := range map[float64]float64{-1: 0, 0: 0, 0.5: 0.5, 2: 1, 0.3: 19.0 / 64} {
		if got := (TextQuality{Contrast: in}).normalized().Contrast; got != want {
			t.Errorf("Contrast %v → %v, ожидалось %v", in, got, want)
		}
	}
}
