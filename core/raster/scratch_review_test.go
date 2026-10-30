package raster

// scratch_review_test.go — случаи из ревью 5.3 (заготовка оверлея,
// ResizeScratch): области размытия не копятся.

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Области размытия, записанные BlurBehind в заготовку, ResizeScratch не
// сбрасывает, а кадрового FinishBackdrops у заготовки нет: срез растёт с
// каждой перерисовкой акрилового попапа, пока открыт хоть один попап.
func TestPopupArenaReview_BackdropsGrowAcrossResize(t *testing.T) {
	testreset.Globals(t)
	src := NewCanvas(400, 300, nil)
	c := src.CloneForSize(120, 80, 1, nil)
	for i := 0; i < 1000; i++ {
		c.ResizeScratch(120+i%2*40, 80)
		c.FillRect(0, 0, 120, 80, color.RGBA{R: 200, A: 255})
		c.BlurBehind(image.Rect(0, 0, 120, 40), 12, color.RGBA{A: 40})
	}
	if n := len(c.backdropsNext) + len(c.backdrops); n > 4 {
		t.Errorf("после 1000 перерисовок у заготовки %d областей размытия", n)
	}
}

// Заготовка после большого размера и ResizeScratch на малый — то же, что
// свежий клон, по полям, влияющим на пиксели.
func TestPopupArenaReview_ResizeScratchState(t *testing.T) {
	testreset.Globals(t)
	src := NewCanvasScaled(400, 300, 1.25, nil)
	c := src.CloneForSize(300, 200, 1.25, nil)
	c.SetClip(image.Rect(10, 10, 50, 50))
	c.SetRoundClip(image.Rect(10, 10, 50, 50), 6)
	c.SetBaseClip(image.Rect(0, 0, 20, 20))
	c.ResizeScratch(90, 70)
	f := src.CloneForSize(90, 70, 1.25, nil)
	if c.W != f.W || c.H != f.H || c.tilesX != f.tilesX || c.tilesY != f.tilesY ||
		c.hasClip != f.hasClip || c.hasBase != f.hasBase || c.round.active != f.round.active ||
		c.Back().Rect != f.Back().Rect || c.Back().Stride != f.Back().Stride || len(c.marks) != len(f.marks) {
		t.Errorf("заготовка %dx%d тайлы %dx%d клип %v/%v/%v, свежий %dx%d %dx%d %v/%v/%v",
			c.W, c.H, c.tilesX, c.tilesY, c.hasClip, c.hasBase, c.round.active,
			f.W, f.H, f.tilesX, f.tilesY, f.hasClip, f.hasBase, f.round.active)
	}
	for i, p := range c.Back().Pix {
		if p != 0 {
			t.Fatalf("байт %d не очищен", i)
		}
	}
}
