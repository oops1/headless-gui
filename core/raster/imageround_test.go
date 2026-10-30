package raster

import (
	hgtext "github.com/oops1/headless-gui/v4/core/text"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// flatImage — однотонное изображение w×h.
func flatImage(w, h int, col color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = col.R, col.G, col.B, col.A
	}
	return img
}

// imageRoundCanvas — холст с однотонным фоном нужного формата: нетронутые
// углы на нём не спутать с залитыми.
func imageRoundCanvas(w, h int, scale float64, f PixelFormat) *Canvas {
	c := NewCanvasScaled(w, h, scale, hgtext.NewFontCache("assets"))
	if f != FormatRGBA {
		c.setOwnFormat(f)
	}
	c.BlitBackground()
	c.FillRect(0, 0, w, h, color.RGBA{R: 30, G: 60, B: 90, A: 255})
	return c
}

func maxPixDiff(a, b *image.RGBA) int {
	m := 0
	for i := range a.Pix {
		d := int(a.Pix[i]) - int(b.Pix[i])
		if d < 0 {
			d = -d
		}
		if d > m {
			m = d
		}
	}
	return m
}

// Изображение под SetRoundClip оставляет углы фона нетронутыми и ложится в
// ту же форму, что и заливка под тем же клипом, — для каждого пути отрисовки,
// масштаба и порядка каналов.
func TestBlitOver_RoundClip(t *testing.T) {
	testreset.Globals(t)
	opaque := color.RGBA{R: 220, G: 10, B: 10, A: 255}
	// Полупрозрачное изображение: premultiplied (110/255 от 220 ≈ 95).
	halfImg := color.RGBA{R: 95, G: 4, B: 4, A: 110}
	halfFill := halfImg

	type path struct {
		name string
		// размер источника при физическом размере цели pw×ph
		src  func(pw, ph int) (int, int)
		draw func(c *Canvas, img *image.RGBA)
	}
	paths := []path{
		{"DrawImage", func(pw, ph int) (int, int) { return 80, 60 },
			func(c *Canvas, img *image.RGBA) { c.DrawImage(img, 10, 10) }},
		{"DrawImageScaled/растяжение", func(pw, ph int) (int, int) { return 40, 30 },
			func(c *Canvas, img *image.RGBA) { c.DrawImageScaled(img, 10, 10, 80, 60) }},
		// размер источника равен физическому размеру цели — быстрый путь 1:1
		{"DrawImageScaled/1:1", func(pw, ph int) (int, int) { return pw, ph },
			func(c *Canvas, img *image.RGBA) { c.DrawImageScaled(img, 10, 10, 80, 60) }},
	}
	bg := color.RGBA{R: 30, G: 60, B: 90, A: 255}
	for _, scale := range []float64{1, 2} {
		for _, f := range []PixelFormat{FormatRGBA, FormatBGRX} {
			for _, p := range paths {
				for _, alpha := range []bool{false, true} {
					c := imageRoundCanvas(120, 100, scale, f)
					ref := imageRoundCanvas(120, 100, scale, f)
					tag := func(msg string) string {
						return p.name + " " + msg
					}

					sw, sh := p.src(c.sl(10, 80), c.sl(10, 60))
					col, fill := opaque, opaque
					if alpha {
						col, fill = halfImg, halfFill
					}
					img := flatImage(sw, sh, col)

					rect := image.Rect(10, 10, 90, 70)
					c.SetRoundClip(rect, 16)
					p.draw(c, img)
					c.ClearRoundClip()
					c.ClearClip()

					// Эталон: заливка тем же цветом под тем же клипом.
					ref.SetRoundClip(rect, 16)
					if alpha {
						ref.FillRectAlpha(10, 10, 80, 60, fill)
					} else {
						ref.FillRect(10, 10, 80, 60, fill)
					}
					ref.ClearRoundClip()
					ref.ClearClip()

					// Угол клипа остался фоном.
					cx, cy := c.PhysX(10), c.PhysX(10)
					off := c.back.PixOffset(cx, cy)
					want := bg
					if f == FormatBGRX {
						want.R, want.B = want.B, want.R
					}
					got := color.RGBA{R: c.back.Pix[off], G: c.back.Pix[off+1], B: c.back.Pix[off+2], A: c.back.Pix[off+3]}
					if got != want {
						t.Errorf("%s scale=%v fmt=%v alpha=%v: угол = %v, ожидался фон %v",
							tag(""), scale, f, alpha, got, want)
					}
					// Форма: пиксели «изменён/не изменён» совпадают с заливкой.
					for i := 0; i < len(c.back.Pix); i += 4 {
						a := c.back.Pix[i] != want.R || c.back.Pix[i+1] != want.G || c.back.Pix[i+2] != want.B
						b := ref.back.Pix[i] != want.R || ref.back.Pix[i+1] != want.G || ref.back.Pix[i+2] != want.B
						if a != b {
							t.Errorf("%s scale=%v fmt=%v alpha=%v: форма расходится с клипом в байте %d",
								tag(""), scale, f, alpha, i)
							break
						}
					}
					// Цвет — допуск на округление Over/билинейной выборки.
					if d := maxPixDiff(c.back, ref.back); d > 3 {
						t.Errorf("%s scale=%v fmt=%v alpha=%v: цвет расходится с заливкой на %d",
							tag(""), scale, f, alpha, d)
					}
				}
			}
		}
	}
}

// Картинка частично за пределами контура — обрезается так же, как заливка.
func TestBlitOver_RoundClipPartialOverlap(t *testing.T) {
	testreset.Globals(t)
	c := imageRoundCanvas(120, 100, 1, FormatRGBA)
	ref := imageRoundCanvas(120, 100, 1, FormatRGBA)
	col := color.RGBA{R: 200, G: 200, B: 0, A: 255}

	c.SetRoundClip(image.Rect(20, 20, 100, 80), 14)
	c.DrawImage(flatImage(60, 70, col), 5, 5)
	c.ClearRoundClip()
	c.ClearClip()

	ref.SetRoundClip(image.Rect(20, 20, 100, 80), 14)
	ref.FillRect(5, 5, 60, 70, col)
	ref.ClearRoundClip()
	ref.ClearClip()

	if d := maxPixDiff(c.back, ref.back); d != 0 {
		t.Fatalf("расхождение с заливкой под тем же клипом: %d", d)
	}
}

// Без скругления блит идёт прежним путём: кадр тот же, что у прямого
// blitOverRect.
func TestBlitOver_NoRoundUnchanged(t *testing.T) {
	testreset.Globals(t)
	c := imageRoundCanvas(120, 100, 1, FormatRGBA)
	ref := imageRoundCanvas(120, 100, 1, FormatRGBA)
	img := testIcon(3)

	c.SetClip(image.Rect(10, 10, 90, 70))
	c.DrawImage(img, 5, 5)
	c.ClearClip()

	ref.SetClip(image.Rect(10, 10, 90, 70))
	r := ref.clampRect(image.Rect(5, 5, 5+64, 5+64))
	ref.blitOverRect(r, img, img.Bounds().Min.Add(image.Pt(r.Min.X-5, r.Min.Y-5)))
	ref.ClearClip()

	if d := maxPixDiff(c.back, ref.back); d != 0 {
		t.Fatalf("без скругления кадр изменился: %d", d)
	}
}

// Бенчмарк скруглённого блита: накладная только на угловых строках.
func BenchmarkDrawImage_RoundClip(b *testing.B) {
	c := NewCanvas(400, 300, hgtext.NewFontCache("assets"))
	img := flatImage(300, 200, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	c.SetRoundClip(image.Rect(10, 10, 390, 290), 12)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.DrawImage(img, 20, 20)
	}
}

func BenchmarkDrawImage_NoRoundClip(b *testing.B) {
	c := NewCanvas(400, 300, hgtext.NewFontCache("assets"))
	img := flatImage(300, 200, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.DrawImage(img, 20, 20)
	}
}
