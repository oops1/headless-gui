package widget

import (
	"image"
	"image/color"
	"testing"
)

func TestOpaqueImage_Marks(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(2, 3, 12, 9))
	rgba.SetRGBA(5, 5, color.RGBA{R: 10, G: 20, B: 30, A: 255})

	if _, ok := IsOpaqueImage(rgba); ok {
		t.Fatal("обычный RGBA считается непрозрачным без объявления")
	}
	w := OpaqueImage(rgba)
	inner, ok := IsOpaqueImage(w)
	if !ok || inner != image.Image(rgba) {
		t.Fatalf("пометка не распознана или внутреннее изображение потеряно: ok=%v", ok)
	}
	if w.Bounds() != rgba.Bounds() || w.At(5, 5) != rgba.At(5, 5) || w.ColorModel() != rgba.ColorModel() {
		t.Fatal("обёртка искажает изображение")
	}
	if r := w.(image.RGBA64Image).RGBA64At(5, 5); r != rgba.RGBA64At(5, 5) {
		t.Fatalf("RGBA64At = %v", r)
	}
	if again := OpaqueImage(w); again != w {
		t.Error("повторная пометка создала вторую обёртку")
	}
	if OpaqueImage(nil) != nil {
		t.Error("nil должен остаться nil")
	}
}

func TestOpaqueImage_ByType(t *testing.T) {
	for name, img := range map[string]image.Image{
		"YCbCr":  image.NewYCbCr(image.Rect(0, 0, 4, 4), image.YCbCrSubsampleRatio420),
		"Gray":   image.NewGray(image.Rect(0, 0, 4, 4)),
		"Gray16": image.NewGray16(image.Rect(0, 0, 4, 4)),
		"CMYK":   image.NewCMYK(image.Rect(0, 0, 4, 4)),
	} {
		if _, ok := IsOpaqueImage(img); !ok {
			t.Errorf("%s непрозрачен по типу, но не распознан", name)
		}
		if OpaqueImage(img) != img {
			t.Errorf("%s не должен оборачиваться", name)
		}
	}
	for name, img := range map[string]image.Image{
		"NRGBA": image.NewNRGBA(image.Rect(0, 0, 4, 4)),
		"Alpha": image.NewAlpha(image.Rect(0, 0, 4, 4)),
	} {
		if _, ok := IsOpaqueImage(img); ok {
			t.Errorf("%s не непрозрачен по типу", name)
		}
	}
}

func TestOpaqueImage_SubImageKeepsMark(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(0, 0, 20, 20))
	sub := OpaqueImage(rgba).(interface {
		SubImage(image.Rectangle) image.Image
	}).SubImage(image.Rect(5, 5, 10, 10))
	inner, ok := IsOpaqueImage(sub)
	if !ok || inner.Bounds() != image.Rect(5, 5, 10, 10) {
		t.Fatalf("SubImage потерял пометку или границы: ok=%v", ok)
	}
	// Запасной путь DrawImageRect вырезает часть через cropImage: пометка жива.
	if _, ok := IsOpaqueImage(cropImage(OpaqueImage(rgba), image.Rect(1, 1, 4, 4))); !ok {
		t.Error("cropImage потерял пометку")
	}
}
