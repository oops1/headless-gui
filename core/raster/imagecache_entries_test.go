package raster

import (
	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"golang.org/x/image/draw"
	"image"
	stdraw "image/draw"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// Запись кэша удерживает источник: пока она жива, сборщик мусора не освободит
// картинку, а значит, и не отдаст её адрес новому объекту тех же размеров
// (иначе тот получил бы чужой кадр из кэша по совпавшему ключу).
func TestScaledCache_EntryKeepsSourceAlive(t *testing.T) {
	testreset.Globals(t)
	c := NewCanvas(120, 120, hgtext.NewFontCache("assets"))
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
	c := NewCanvas(120, 120, hgtext.NewFontCache("assets"))
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

// Сброс всего кэша и переполнение журнала: отставший холст сбрасывает кэш
// целиком. Engine.InvalidateImageCache пишет в тот же журнал (add).
func TestImageInvalidator_AllAndOverflow(t *testing.T) {
	testreset.Globals(t)
	c := engineCanvas(200, 200)
	inv := &ImageInvalidator{}
	c.SetImageInvalidator(inv)
	ic := testIcon(0)
	other := testIcon(50)
	c.DrawImageScaled(ic, 0, 0, 24, 24)
	c.DrawImageScaled(other, 0, 0, 24, 24)

	inv.Add(0, true)
	c.syncImageInvalidations()
	if len(c.scaledCache) != 0 {
		t.Fatalf("после сброса всего в кэше %d записей", len(c.scaledCache))
	}

	// Журнал короче числа сбросов: отставший холст сбрасывает кэш целиком.
	c.DrawImageScaled(ic, 0, 0, 24, 24)
	for i := 0; i < maxInvLog*2; i++ {
		inv.Add(reflect.ValueOf(testIcon(uint8(i))).Pointer(), false)
	}
	c.syncImageInvalidations()
	if len(c.scaledCache) != 0 {
		t.Fatalf("после переполнения журнала в кэше осталось %d записей", len(c.scaledCache))
	}
}
