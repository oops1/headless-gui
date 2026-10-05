//go:build linux && !android

package window

import (
	"image"
	"image/color"
	"testing"
)

func solidImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{200, 100, 50, 255})
		}
	}
	return img
}

func shmPixel(w *WaylandWindow, buf, x, y int) [4]byte {
	o := buf*w.stride*w.poolH + y*w.stride + x*4
	return [4]byte{w.shmData[o], w.shmData[o+1], w.shmData[o+2], w.shmData[o+3]}
}

// Окно со скруглением: буфер ARGB8888, углы прозрачные, середина
// непрозрачная, компоновщику сообщена непрозрачная область.
func TestWayland_RoundedWindowHasAlphaCorners(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.width, c.w.height = 100, 80
	c.w.SetCornerRadius(8)
	c.w.BlitRGBA(solidImage(100, 80))
	msgs := c.readAllWl(t)

	if !c.w.poolARGB {
		t.Fatal("буфер окна со скруглением — не ARGB")
	}
	var formats []uint32
	opaque := false
	for _, m := range msgs {
		if m.obj == c.w.poolID && m.opcode == wlShmPoolCreateBuffer && len(m.args) == 6 {
			formats = append(formats, m.args[5])
		}
		if m.obj == c.w.surfaceID && m.opcode == wlSurfaceSetOpaqueRegion {
			opaque = true
		}
	}
	if len(formats) != 2 || formats[0] != wlShmFormatARGB8888 {
		t.Errorf("форматы буферов: %v, ждали ARGB8888", formats)
	}
	if !opaque {
		t.Error("set_opaque_region не отправлен")
	}
	if got := shmPixel(c.w, 0, 0, 0); got != [4]byte{} {
		t.Errorf("угол = %v, ждали (0,0,0,0)", got)
	}
	if got := shmPixel(c.w, 0, 50, 40); got[3] != 255 {
		t.Errorf("середина = %v, ждали непрозрачную", got)
	}
}

// Развёрнутое окно — без скругления: ни одного прозрачного пикселя.
func TestWayland_MaximizedWindowIsOpaque(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.width, c.w.height = 100, 80
	c.w.SetCornerRadius(8)
	c.w.maximized.Store(true)
	c.w.BlitRGBA(solidImage(100, 80))
	c.readAllWl(t)
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			if a := shmPixel(c.w, 0, x, y)[3]; a != 255 {
				t.Fatalf("(%d,%d) альфа %d у развёрнутого окна", x, y, a)
			}
		}
	}
}

// Без скругления — прежний XRGB.
func TestWayland_SquareWindowStaysXRGB(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.width, c.w.height = 100, 80
	c.w.BlitRGBA(solidImage(100, 80))
	c.readAllWl(t)
	if c.w.poolARGB {
		t.Error("окно без скругления получило буфер с альфой")
	}
}
