package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// Engine.InvalidateImageCache доходит до основного холста и холстов попапов.
func TestEngine_InvalidateImageCache(t *testing.T) {
	testreset.Globals(t)
	e := New(200, 200, 20)
	popup := e.canvas.CloneForSize(60, 60, 1, nil)

	ic := testIcon(0)
	probe := func(c *Canvas) color.RGBA {
		clear(c.Back().Pix)
		c.DrawImageScaled(ic, 0, 0, 32, 32)
		return c.Back().RGBAAt(5, 5)
	}
	before := []color.RGBA{probe(e.canvas), probe(popup)}

	// Пиксели переписаны на месте — без сброса кэш отдаёт старое.
	for i := 0; i < len(ic.Pix); i += 4 { // альфа остаётся 255
		ic.Pix[i], ic.Pix[i+1], ic.Pix[i+2] = 255-ic.Pix[i], 255-ic.Pix[i+1], 255-ic.Pix[i+2]
	}
	if probe(e.canvas) != before[0] {
		t.Fatal("до сброса кэш обязан был вернуть прежний кадр (предпосылка теста)")
	}

	e.InvalidateImageCache(ic)
	if got := probe(e.canvas); got == before[0] {
		t.Error("основной холст отдал устаревший кадр после Engine.InvalidateImageCache")
	}
	if got := probe(popup); got == before[1] {
		t.Error("холст попапа отдал устаревший кадр после Engine.InvalidateImageCache")
	}
}

// Сброс целого кэша (nil) и переполнение журнала: оба доходят от Engine до
// основного холста и холстов попапов.
func TestEngine_InvalidateImageCache_AllAndOverflow(t *testing.T) {
	testreset.Globals(t)
	e := New(200, 200, 20)
	popup := e.canvas.CloneForSize(60, 60, 1, nil)

	ic := testIcon(0)
	probe := func(c *Canvas) color.RGBA {
		clear(c.Back().Pix)
		c.DrawImageScaled(ic, 0, 0, 32, 32)
		return c.Back().RGBAAt(5, 5)
	}
	invert := func() { // альфа остаётся 255
		for i := 0; i < len(ic.Pix); i += 4 {
			ic.Pix[i], ic.Pix[i+1], ic.Pix[i+2] = 255-ic.Pix[i], 255-ic.Pix[i+1], 255-ic.Pix[i+2]
		}
	}
	before := []color.RGBA{probe(e.canvas), probe(popup)}
	invert()
	if probe(e.canvas) != before[0] {
		t.Fatal("до сброса кэш обязан был вернуть прежний кадр (предпосылка теста)")
	}

	e.InvalidateImageCache(nil)
	if got := probe(e.canvas); got == before[0] {
		t.Error("основной холст отдал устаревший кадр после InvalidateImageCache(nil)")
	}
	if got := probe(popup); got == before[1] {
		t.Error("холст попапа отдал устаревший кадр после InvalidateImageCache(nil)")
	}

	// Журнал короче числа сбросов (сбрасываются другие картинки): отставший
	// холст обязан сбросить кэш целиком, иначе ic останется устаревшей.
	before = []color.RGBA{probe(e.canvas), probe(popup)}
	invert()
	for i := 1; i <= 200; i++ {
		e.InvalidateImageCache(testIcon(uint8(i)))
	}
	if got := probe(e.canvas); got == before[0] {
		t.Error("основной холст отдал устаревший кадр после переполнения журнала")
	}
	if got := probe(popup); got == before[1] {
		t.Error("холст попапа отдал устаревший кадр после переполнения журнала")
	}
}

// Сброс можно заказывать с любой горутины, пока холст рисует.
func TestEngine_InvalidateImageCache_Concurrent(t *testing.T) {
	testreset.Globals(t)
	e := New(200, 200, 20)
	ic := testIcon(0)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			e.InvalidateImageCache(ic)
		}
	}()
	for i := 0; i < 2000; i++ {
		e.canvas.DrawImageScaled(ic, 0, 0, 24, 24)
	}
	<-done
}

// Обёртка попапа пробрасывает DrawImageRect со сдвигом и сброс кэша.
func TestTranslatingContext_ImageRectAndInvalidate(t *testing.T) {
	testreset.Globals(t)
	rect := image.Rect(40, 20, 140, 120)
	big, tc, small := popupPair(rect)
	src := noiseRGBA(40, 30, 11, false)

	big.DrawImageRect(src, src.Bounds(), image.Rect(50, 30, 110, 75), widget.FilterBilinear)
	tc.DrawImageRect(src, src.Bounds(), image.Rect(50, 30, 110, 75), widget.FilterBilinear)
	samePixels(t, big, small, rect, rect)

	var ctx widget.DrawContext = tc
	if _, ok := ctx.(widget.ImageRectDrawer); !ok {
		t.Error("контекст попапа не реализует ImageRectDrawer")
	}
	small.DrawImageScaled(src, 0, 0, 20, 20)
	if small.ScaledCacheLen() == 0 {
		t.Fatal("предпосылка: запись должна попасть в кэш")
	}
	widget.InvalidateImageCache(ctx, src)
	if small.ScaledCacheLen() != 0 {
		t.Error("сброс кэша не дошёл через контекст попапа до холста")
	}
}
