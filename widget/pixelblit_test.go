package widget

import (
	"image"
	"testing"
)

// fbCtx — контекст без PixelBlitter: записывает, как помощник BlitPixels
// выкрутился запасным путём.
type fbCtx struct {
	DrawContext
	scale        float64
	drawn        image.Image
	x, y, w, h   int
	scaledCalled bool
	imageCalled  bool
}

func (c *fbCtx) Scale() float64 { return c.scale }
func (c *fbCtx) DrawImage(src image.Image, x, y int) {
	c.imageCalled, c.drawn, c.x, c.y = true, src, x, y
}
func (c *fbCtx) DrawImageScaled(src image.Image, x, y, w, h int) {
	c.scaledCalled, c.drawn, c.x, c.y, c.w, c.h = true, src, x, y, w, h
}

func TestBlitPixels_FallbackScale1(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 30))
	c := &fbCtx{scale: 1}
	BlitPixels(c, src, image.Rect(5, 6, 25, 20), 100, 200, true)
	if !c.imageCalled || c.scaledCalled {
		t.Fatalf("масштаб 1: DrawImage=%v DrawImageScaled=%v", c.imageCalled, c.scaledCalled)
	}
	if c.x != 105 || c.y != 206 {
		t.Fatalf("позиция (%d,%d), ждали (105,206)", c.x, c.y)
	}
	if c.drawn.Bounds().Dx() != 20 || c.drawn.Bounds().Dy() != 14 {
		t.Fatalf("размер части %v", c.drawn.Bounds())
	}
	if _, ok := IsOpaqueImage(c.drawn); !ok {
		t.Fatal("пометка непрозрачности потеряна на запасном пути")
	}
	BlitPixels(c, src, src.Rect, 0, 0, false)
	if _, ok := IsOpaqueImage(c.drawn); ok {
		t.Fatal("непрозрачность появилась из ничего")
	}
}

func TestBlitPixels_FallbackScale2(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 30))
	c := &fbCtx{scale: 2}
	BlitPixels(c, src, src.Rect, 10, 20, true)
	if !c.scaledCalled || c.x != 10 || c.y != 20 || c.w != 20 || c.h != 15 {
		t.Fatalf("масштаб 2: (%d,%d) %dx%d (scaled=%v)", c.x, c.y, c.w, c.h, c.scaledCalled)
	}
}

func TestBlitPixels_Degenerate(t *testing.T) {
	c := &fbCtx{scale: 1}
	BlitPixels(c, nil, image.Rect(0, 0, 5, 5), 0, 0, true)
	BlitPixels(c, image.NewRGBA(image.Rect(0, 0, 5, 5)), image.Rect(10, 10, 20, 20), 0, 0, true)
	BlitPixels(c, image.NewRGBA(image.Rect(0, 0, 5, 5)), image.Rectangle{}, 0, 0, true)
	if c.imageCalled || c.scaledCalled {
		t.Fatal("пустая часть дошла до контекста")
	}
}
