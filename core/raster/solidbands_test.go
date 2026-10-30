package raster

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Картинка вынесенного каскадного меню: два тела со сплошной альфой, вокруг
// каждого полупрозрачная тень, между ними прозрачная дыра.
func shadowedCascade() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	paint := func(r image.Rectangle, a uint8) {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if img.RGBAAt(x, y).A < a {
					img.SetRGBA(x, y, color.RGBA{A: a})
				}
			}
		}
	}
	menu, sub := image.Rect(8, 8, 120, 150), image.Rect(128, 40, 290, 120)
	paint(menu.Inset(-6), 60) // тень
	paint(sub.Inset(-6), 60)
	paint(menu, 255)
	paint(sub, 255)
	return img
}

// Сплошные полосы — только точки с альфой 255: тени в них нет. Именно их
// Wayland-попап объявляет непрозрачной областью (замечание WinLine после
// v3.35.1: тень меню Блокнота превращалась в чёрную рамку).
func TestSolidBands_ExcludeShadow(t *testing.T) {
	testreset.Globals(t)
	img := shadowedCascade()
	solid := SolidBands(img)
	if len(solid) == 0 {
		t.Fatal("сплошных полос нет")
	}
	in := func(x, y int, rs []image.Rectangle) bool {
		for _, r := range rs {
			if (image.Point{x, y}).In(r) {
				return true
			}
		}
		return false
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			a := img.RGBAAt(x, y).A
			if got := in(x, y, solid); got != (a == 255) {
				t.Fatalf("(%d,%d) альфа %d: в сплошных полосах %v", x, y, a, got)
			}
		}
	}
	// А закрашенные полосы (форма окна и область ввода) тень включают, как
	// и раньше.
	if bands := OpaqueBands(img); !in(3, 3, bands) {
		t.Fatalf("тень пропала из OpaqueBands: %v", bands)
	}
	// У картинки без дыр сплошные полосы тоже есть (OpaqueBands там nil).
	full := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for i := 3; i < len(full.Pix); i += 4 {
		full.Pix[i] = 255
	}
	if got := SolidBands(full); len(got) != 1 || got[0] != full.Rect {
		t.Fatalf("сплошная картинка: %v", got)
	}
	if SolidBands(nil) != nil || SolidBands(image.NewRGBA(image.Rect(0, 0, 4, 4))) != nil {
		t.Fatal("у пустой картинки нашлись сплошные полосы")
	}
}
