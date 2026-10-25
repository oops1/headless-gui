package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// iconTopRow рисует кнопку IconTop (иконка 24, подпись кегля pt, высота 54,
// как на панели инструментов Go.Git) и возвращает строку верха иконки
// относительно кнопки.
func iconTopRow(t *testing.T, pt float64) int {
	t.Helper()
	icon := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			icon.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	white := color.RGBA{255, 255, 255, 255}
	eng := engine.New(120, 80, 30)
	root := widget.NewPanel(white)
	root.SetBounds(image.Rect(0, 0, 120, 80))
	b := widget.NewButton("Commit")
	b.FontSize = pt
	b.Background, b.BorderColor = white, white
	b.TextColor = color.RGBA{A: 255}
	b.Icon, b.IconSize, b.IconPos = icon, 24, widget.IconTop
	b.SetBounds(image.Rect(10, 10, 110, 64))
	root.AddChild(b)
	eng.SetRoot(root)
	img := eng.RenderOnce()
	for y := 10; y < 64; y++ {
		if c := img.RGBAAt(60, y); c.R > 200 && c.G < 60 && c.B < 60 {
			return y - 10
		}
	}
	t.Fatalf("%.1f pt: иконка не найдена", pt)
	return 0
}

// GG-94: у кнопки IconTop с подписью мельче шрифта по умолчанию блок
// «иконка + подпись» стоит там же, где в v3.39 (и где у шрифта по
// умолчанию), а не уезжает вниз. Крупная подпись по-прежнему поднимает блок
// (calc): её строка выше.
func TestButtonIconTop_SmallFontKeepsPlace(t *testing.T) {
	def := widget.DefaultFontSize()
	base := iconTopRow(t, def)
	for _, pt := range []float64{7, 8, 9} {
		if got := iconTopRow(t, pt); got != base {
			t.Errorf("%.0f pt: верх иконки в строке %d, у шрифта по умолчанию %d — блок съехал", pt, got, base)
		}
	}
	if got := iconTopRow(t, def*1.4); got >= base {
		t.Errorf("крупная подпись: верх иконки %d, ждали выше %d", got, base)
	}
}
