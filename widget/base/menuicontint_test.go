package base

import (
	"image"
	"image/color"
	"testing"
)

func iconOf(fill func(x, y int) color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, fill(x, y))
		}
	}
	return img
}

// Флаг профиля menu.icon.tint перекрашивает только одноцветные значки. Цветные
// — жёлтая папка, картинка программы — остаются как есть: в «Пуске»
// Windows 2000 они выходили чёрными силуэтами (замечание WinLine после
// v3.38.0). Явный MenuItem.IconTint перекрашивает любой значок.
func TestMenuIconTint_OnlyMonochromeUnderFlag(t *testing.T) {
	// Чёрный глиф со сглаженным краем (полупрозрачные точки, предумножены).
	glyph := iconOf(func(x, y int) color.RGBA {
		switch {
		case x >= 4 && x < 12 && y >= 4 && y < 12:
			return color.RGBA{A: 255}
		case x == 3 || x == 12:
			return color.RGBA{A: 90}
		}
		return color.RGBA{}
	})
	// Папка: жёлтое тело и коричневый край.
	folder := iconOf(func(x, y int) color.RGBA {
		if y < 4 {
			return color.RGBA{R: 140, G: 90, B: 20, A: 255}
		}
		return color.RGBA{R: 250, G: 210, B: 60, A: 255}
	})
	ink := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	m := NewPopupMenu()
	m.TintIcons = true

	if got := m.iconFor(MenuItem{Icon: folder}, false, ink); got != image.Image(folder) {
		t.Error("цветная папка перекрашена флагом профиля")
	}
	tinted := m.iconFor(MenuItem{Icon: glyph}, false, ink)
	if tinted == image.Image(glyph) {
		t.Fatal("одноцветный глиф не перекрашен")
	}
	if c := tinted.(*image.RGBA).RGBAAt(8, 8); c != ink {
		t.Errorf("глиф перекрашен в %v, ждали цвет текста %v", c, ink)
	}
	if got := m.iconFor(MenuItem{Icon: folder, IconTint: true}, false, ink); got == image.Image(folder) {
		t.Error("явный IconTint не перекрасил цветной значок")
	}
	m.TintIcons = false
	if got := m.iconFor(MenuItem{Icon: glyph}, false, ink); got != image.Image(glyph) {
		t.Error("без флага и без IconTint значок перекрашен")
	}
}
