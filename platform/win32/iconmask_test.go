package win32

import (
	"image"
	"image/color"
	"testing"
)

// TestIconMaskBuffer проверяет AND-маску из альфы и WORD-выравнивание stride.
func TestIconMaskBuffer(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{A: 255}) // непрозрачный
		}
	}
	img.SetRGBA(0, 0, color.RGBA{}) // прозрачный (a=0)

	mask, stride := iconMaskBuffer(img, 128)
	// 16 пикселей → stride = ceil(16/16)*2 = 2 байта.
	if stride != 2 {
		t.Fatalf("stride=%d, want 2", stride)
	}
	if len(mask) != stride*2 {
		t.Fatalf("len(mask)=%d, want %d", len(mask), stride*2)
	}
	// (0,0) прозрачный → старший бит первого байта = 1.
	if mask[0]&0x80 == 0 {
		t.Errorf("mask bit (0,0) not set: %#x", mask[0])
	}
	// (1,0) непрозрачный → следующий бит = 0.
	if mask[0]&0x40 != 0 {
		t.Errorf("mask bit (1,0) should be clear: %#x", mask[0])
	}
	// Вся строка 1 непрозрачна → байты второй строки == 0.
	for i := stride; i < 2*stride; i++ {
		if mask[i] != 0 {
			t.Errorf("row1 byte %d = %#x, want 0", i, mask[i])
		}
	}
}
