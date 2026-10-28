package engine

import (
	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"image"
	"image/color"
	stdraw "image/draw"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/image/draw"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// Запись кэша удерживает источник: пока она жива, сборщик мусора не освободит
// картинку, а значит, и не отдаст её адрес новому объекту тех же размеров
// (иначе тот получил бы чужой кадр из кэша по совпавшему ключу).
func TestScaledCache_EntryKeepsSourceAlive(t *testing.T) {
	testreset.Globals(t)
	c := newCanvas(120, 120, hgtext.NewFontCache("assets"))
	var freed atomic.Bool
	func() {
		ic := testIcon(0)
		runtime.SetFinalizer(ic, func(*image.RGBA) { freed.Store(true) })
		c.DrawImageScaled(ic, 10, 10, 24, 24)
	}() // ic вышел из области видимости: ссылается на него только кэш
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	if freed.Load() {
		t.Fatal("источник собран сборщиком, пока его запись жива в кэше: адрес мог уйти другому объекту")
	}
	// Когда записи нет, источник отпускается — кэш не течёт.
	c.InvalidateImageCache(nil)
	for i := 0; i < 50 && !freed.Load(); i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	if !freed.Load() {
		t.Fatal("после сброса кэша источник так и остался удержан")
	}
}

// Запись с чужим src под тем же ключом не отдаётся: подделываем ключ второго
// изображения записью первого — ровно то, что случилось бы при переезде адреса.
func TestScaledCache_ForeignEntryNotReused(t *testing.T) {
	testreset.Globals(t)
	c := newCanvas(120, 120, hgtext.NewFontCache("assets"))
	a, b := testIcon(0), testIcon(100)
	ka, _ := scaledCacheKey(a, 24, 24)
	kb, _ := scaledCacheKey(b, 24, 24)
	c.DrawImageScaled(a, 10, 10, 24, 24)
	entA := c.scaledCache[ka]
	c.scaledCache[kb] = entA // под ключом b лежит запись от a

	got := c.scaledFor(b, 24, 24)
	if got == entA.img {
		t.Fatal("изображению отдан чужой кадр из кэша")
	}
	want := image.NewRGBA(image.Rect(0, 0, 24, 24))
	draw.BiLinear.Scale(want, want.Bounds(), b, b.Bounds(), stdraw.Src, nil)
	for i := range want.Pix {
		if got.Pix[i] != want.Pix[i] {
			t.Fatalf("кадр b не соответствует b: байт %d", i)
		}
	}
	// Подмена заменена настоящей записью b, а не осталась висеть.
	if c.scaledCache[kb].src != image.Image(b) {
		t.Error("запись под ключом b не принадлежит b")
	}
}

// Engine.InvalidateImageCache доходит до основного холста и холстов попапов.
func TestEngine_InvalidateImageCache(t *testing.T) {
	testreset.Globals(t)
	e := New(200, 200, 20)
	popup := e.canvas.cloneForSize(60, 60, 1, nil)

	ic := testIcon(0)
	probe := func(c *Canvas) color.RGBA {
		clear(c.back.Pix)
		c.DrawImageScaled(ic, 0, 0, 32, 32)
		return c.back.RGBAAt(5, 5)
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

// Сброс целого кэша (nil) и переполнение журнала.
func TestEngine_InvalidateImageCache_AllAndOverflow(t *testing.T) {
	testreset.Globals(t)
	e := New(200, 200, 20)
	ic := testIcon(0)
	other := testIcon(50)
	e.canvas.DrawImageScaled(ic, 0, 0, 24, 24)
	e.canvas.DrawImageScaled(other, 0, 0, 24, 24)

	e.InvalidateImageCache(nil)
	e.canvas.syncImageInvalidations()
	if len(e.canvas.scaledCache) != 0 {
		t.Fatalf("после InvalidateImageCache(nil) в кэше %d записей", len(e.canvas.scaledCache))
	}

	// Журнал короче числа сбросов: отставший холст сбрасывает кэш целиком.
	e.canvas.DrawImageScaled(ic, 0, 0, 24, 24)
	for i := 0; i < maxInvLog*2; i++ {
		e.InvalidateImageCache(testIcon(uint8(i)))
	}
	e.canvas.syncImageInvalidations()
	if len(e.canvas.scaledCache) != 0 {
		t.Fatalf("после переполнения журнала в кэше осталось %d записей", len(e.canvas.scaledCache))
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
	if len(small.scaledCache) == 0 {
		t.Fatal("предпосылка: запись должна попасть в кэш")
	}
	widget.InvalidateImageCache(ctx, src)
	if len(small.scaledCache) != 0 {
		t.Error("сброс кэша не дошёл через контекст попапа до холста")
	}
}
