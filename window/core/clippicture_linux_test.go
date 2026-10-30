//go:build linux && !android

package core

// clippicture_linux_test.go — картинки для тестов буфера обмена Wayland и X11
// (копии — в platform/x11/helpers_linux_test.go).

import (
	"image"
	"image/color"
	"testing"
)

// testClipPicture — картинка с прозрачностью: проверяет, что альфа доезжает.
func testClipPicture() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(60 * x), uint8(90 * y), 128, 255})
		}
	}
	img.SetNRGBA(1, 1, color.NRGBA{200, 100, 50, 77})
	return img
}

func samePicture(t *testing.T, got image.Image, want *image.NRGBA) {
	t.Helper()
	if got == nil || got.Bounds().Dx() != want.Bounds().Dx() || got.Bounds().Dy() != want.Bounds().Dy() {
		t.Fatalf("размер %v, ждал %v", got, want.Bounds())
	}
	for y := 0; y < want.Bounds().Dy(); y++ {
		for x := 0; x < want.Bounds().Dx(); x++ {
			g := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
			if w := want.NRGBAAt(x, y); g != w {
				t.Errorf("(%d,%d) = %v, ждал %v", x, y, g, w)
			}
		}
	}
}
