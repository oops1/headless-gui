package raster

// singlebuf_review_test.go — случаи из ревью 4а.4 (ADR-0007): окно back не
// трогает тайлы при задании, объединение живого окна без новой аллокации,
// цена сброса на прямом пути.

import (
	"image"
	"runtime"
	"testing"

	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"github.com/oops1/headless-gui/v3/internal/testreset"
)

func reviewCanvas(w, h int) *Canvas {
	c := NewCanvas(w, h, hgtext.NewFontCache("assets"))
	c.SetSingleBuffer(true)
	return c
}

func countTouched(c *Canvas) int {
	n := 0
	for _, v := range c.touched {
		if v {
			n++
		}
	}
	return n
}

// SetBackWindow, вернувший уже накрывающее чистое окно, помечает тронутыми
// все тайлы r: прямой путь задаёт окно размером с кадр на каждом частичном
// кадре — и сброс сравнивает с front весь кадр, а не повреждение.
func TestSingleBufferReview_SetBackWindowKeptCleanTouchesAll(t *testing.T) {
	testreset.Globals(t)
	c := reviewCanvas(1280, 800)
	c.SetBackWindow(c.Frame())
	c.FlushBackWindow(nil)
	if n := countTouched(c); n != 0 {
		t.Fatalf("после сброса тронуто %d тайлов", n)
	}
	c.SetBackWindow(c.Frame()) // окно чистое и уже накрывает r
	n, total := countTouched(c), c.tilesX*c.tilesY
	t.Logf("повторный SetBackWindow(Frame) на чистом окне: тронуто %d тайлов из %d", n, total)
	if n != 0 {
		t.Errorf("чистое окно, равное front, не нуждается в сравнении: тронуто %d тайлов", n)
	}
	// То же — при окне, созданном заново с заполнением из front (prefill):
	// содержимое равно front, сравнивать его незачем.
	c.FlushBackWindow(nil)
	c.releaseBackWindow()
	c.SetBackWindow(image.Rect(100, 100, 300, 300))
	t.Logf("новое окно с префиллом из front: тронуто %d тайлов (окно %v, тайлов в нём 16)", countTouched(c), c.BackWindow())
}

// Цена сброса на прямом пути частичного кадра: как в renderFrame
// (SetBackWindow(Frame) + Touch(damage) + FlushBackWindow(damageRects))
// против одного Touch(damage).
func BenchmarkSingleBufferReview_FlushDirectPartial(b *testing.B) {
	dmg := image.Rect(400, 400, 600, 424)
	rects := []image.Rectangle{dmg}
	b.Run("SetBackWindow(Frame)+Touch(dmg)", func(b *testing.B) {
		c := reviewCanvas(1280, 800)
		c.SetBackWindow(c.Frame())
		c.FlushBackWindow(nil)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.SetBackWindow(c.Frame())
			c.Touch(dmg)
			c.FlushBackWindow(rects)
			c.ChangedTiles()
		}
	})
	b.Run("Touch(dmg)", func(b *testing.B) {
		c := reviewCanvas(1280, 800)
		c.SetBackWindow(c.Frame())
		c.FlushBackWindow(nil)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Touch(dmg)
			c.FlushBackWindow(rects)
			c.ChangedTiles()
		}
	})
	b.Run("4K SetBackWindow(Frame)+Touch(dmg)", func(b *testing.B) {
		c := reviewCanvas(3840, 2160)
		c.SetBackWindow(c.Frame())
		c.FlushBackWindow(nil)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.SetBackWindow(c.Frame())
			c.Touch(dmg)
			c.FlushBackWindow(rects)
			c.ChangedTiles()
		}
	})
	b.Run("4K Touch(dmg)", func(b *testing.B) {
		c := reviewCanvas(3840, 2160)
		c.SetBackWindow(c.Frame())
		c.FlushBackWindow(nil)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Touch(dmg)
			c.FlushBackWindow(rects)
			c.ChangedTiles()
		}
	})
}

// Живое окно при объединении всегда заводит новый буфер, даже когда старый
// вмещает объединение.
func TestSingleBufferReview_LiveWindowUnionAllocates(t *testing.T) {
	testreset.Globals(t)
	c := reviewCanvas(640, 480)
	c.SetBackWindow(c.Frame()) // большой буфер
	c.FlushBackWindow(nil)
	c.releaseBackWindow()
	c.SetBackWindow(image.Rect(0, 0, 64, 64))
	c.FlushBackWindow(nil)
	// Малое окно, в этом кадре нарисовано (живое), растёт на соседний тайл.
	c.SetBackWindow(image.Rect(0, 0, 64, 64))
	c.Touch(image.Rect(0, 0, 64, 64))
	bytes := func(fn func()) uint64 {
		var a, b runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&a)
		fn()
		runtime.ReadMemStats(&b)
		return b.TotalAlloc - a.TotalAlloc
	}
	n := bytes(func() { c.SetBackWindow(image.Rect(64, 0, 128, 64)) })
	t.Logf("объединение живого окна 64×64 с соседним тайлом: выделено %d байт (окно %v)", n, c.BackWindow())
}
