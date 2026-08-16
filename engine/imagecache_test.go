package engine

import (
	"image"
	"image/color"
	"testing"
)

func testIcon(seed uint8) *image.RGBA {
	ic := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			ic.SetRGBA(x, y, color.RGBA{R: uint8(x*4) + seed, G: uint8(y * 4), B: 128, A: 255})
		}
	}
	return ic
}

// Кэш масштабирования не меняет пиксели результата.
func TestScaledCache_PixelIdentity(t *testing.T) {
	fc := newFontCache("assets")
	a := newCanvas(120, 120, fc)
	b := newCanvas(120, 120, fc)
	ic := testIcon(0)

	a.DrawImageScaled(ic, 10, 10, 24, 24)
	for i := 0; i < 5; i++ { // из кэша
		b.DrawImageScaled(ic, 10, 10, 24, 24)
	}
	for i := range a.back.Pix {
		if a.back.Pix[i] != b.back.Pix[i] {
			t.Fatalf("байт %d: %d != %d", i, a.back.Pix[i], b.back.Pix[i])
		}
	}
}

// Повторный вызов берёт результат из кэша, не растеризуя заново.
func TestScaledCache_ReusesResult(t *testing.T) {
	c := newCanvas(120, 120, newFontCache("assets"))
	ic := testIcon(0)
	c.DrawImageScaled(ic, 10, 10, 24, 24)
	first := c.scaledFor(ic, 24, 24)
	second := c.scaledFor(ic, 24, 24)
	if first != second {
		t.Fatal("повторный вызов не переиспользовал кэш")
	}
	if c.scaledFor(ic, 32, 32) == first {
		t.Fatal("другой размер обязан дать другой буфер")
	}
}

// InvalidateImageCache выбрасывает записи изменённой картинки.
func TestScaledCache_Invalidate(t *testing.T) {
	c := newCanvas(120, 120, newFontCache("assets"))
	ic := testIcon(0)
	c.DrawImageScaled(ic, 10, 10, 24, 24)
	if len(c.scaledCache) != 1 {
		t.Fatalf("в кэше %d записей, ожидалась 1", len(c.scaledCache))
	}
	c.InvalidateImageCache(ic)
	if len(c.scaledCache) != 0 || c.scaledPixels != 0 {
		t.Fatalf("после инвалидации: записей %d, пикселей %d", len(c.scaledCache), c.scaledPixels)
	}
}

// Кэш не растёт сверх предела записей.
func TestScaledCache_EvictsByCount(t *testing.T) {
	c := newCanvas(120, 120, newFontCache("assets"))
	for i := 0; i < maxScaledEntries*2; i++ {
		c.DrawImageScaled(testIcon(uint8(i)), 10, 10, 24, 24)
	}
	if len(c.scaledCache) > maxScaledEntries {
		t.Fatalf("в кэше %d записей, предел %d", len(c.scaledCache), maxScaledEntries)
	}
	if c.scaledPixels > maxScaledPixels {
		t.Fatalf("бюджет пикселей превышен: %d", c.scaledPixels)
	}
}
