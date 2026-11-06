package raster

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"

	hgtext "github.com/oops1/headless-gui/v4/core/text"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// Тень с вырезом под слоем (DrawShadowAround): слой, отбрасывающий тень,
// закрывает свою середину сам, и собирать и смешивать тень под ним незачем
// (пилот сцены WinLine, ENGINE_ISSUES «A window's shadow is laid over its whole
// shape, under the window too»). Картинка обязана остаться прежней.

func scaledCanvas(w, h int, scale float64) *Canvas {
	c := NewCanvasScaled(w, h, scale, hgtext.NewFontCache("assets"))
	c.AddDefaultFonts()
	return c
}

// paintWindow рисует фон, тень слоя r и сам слой — скруглённую непрозрачную
// заливку; around — тень с вырезом.
func paintWindow(c *Canvas, r image.Rectangle, corner int, around bool) {
	lw, lh := c.LogicalSize()
	for y := 0; y < lh; y += 8 { // полосатый фон: тень над ним зависит от места
		c.FillRect(0, y, lw, 8, color.RGBA{R: uint8(40 + y), G: uint8(200 - y), B: 90, A: 255})
	}
	col := color.RGBA{A: 110}
	if around {
		c.DrawShadowAround(r, corner, 14, 3, 7, col, base.ShadowCovered(r, corner))
	} else {
		c.DrawShadow(r, corner, 14, 3, 7, col)
	}
	c.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), corner, color.RGBA{R: 250, G: 250, B: 252, A: 255})
}

// Кадр с вырезом побайтно равен кадру с тенью целиком: масштабы, форматы,
// углы, клип посреди тени.
func TestShadowAround_SamePictureAsWholeShadow(t *testing.T) {
	testreset.Globals(t)
	r := image.Rect(60, 40, 300, 220)
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		for _, format := range []PixelFormat{FormatRGBA, FormatBGRX} {
			for _, corner := range []int{0, 8} {
				for _, clip := range []image.Rectangle{{}, image.Rect(40, 100, 190, 260), image.Rect(120, 90, 200, 150)} {
					name := fmt.Sprintf("scale=%v format=%d corner=%d clip=%v", scale, format, corner, clip)
					draw := func(around bool) []byte {
						c := scaledCanvas(360, 280, scale)
						c.SetPixelFormat(format, nil)
						if !clip.Empty() {
							c.SetClip(clip)
						}
						paintWindow(c, r, corner, around)
						return append([]byte(nil), c.Back().Pix...)
					}
					if whole, ring := draw(false), draw(true); !bytes.Equal(whole, ring) {
						t.Errorf("%s: кадр с вырезом тени отличается от кадра с тенью целиком", name)
					}
				}
			}
		}
	}
}

// Вырез действительно пропускается: без слоя сверху фон под ним остаётся
// нетронутым, а кольцо вокруг тень получает.
func TestShadowAround_SkipsCoveredArea(t *testing.T) {
	testreset.Globals(t)
	r := image.Rect(60, 40, 300, 220)
	for _, scale := range []float64{1, 1.5} {
		c := scaledCanvas(360, 280, scale)
		bg := color.RGBA{R: 200, G: 210, B: 220, A: 255}
		c.FillRect(0, 0, 360, 280, bg)
		covered := base.ShadowCovered(r, 8)
		c.DrawShadowAround(r, 8, 14, 0, 7, color.RGBA{A: 110}, covered)

		at := func(lx, ly int) color.RGBA { return c.Back().RGBAAt(c.PhysX(lx), c.PhysX(ly)) }
		if got := at(180, 130); got != bg {
			t.Errorf("scale=%v: середина выреза затронута тенью: %v", scale, got)
		}
		if got := at(covered.Min.X+2, covered.Min.Y+2); got != bg {
			t.Errorf("scale=%v: угол выреза затронут тенью: %v", scale, got)
		}
		if got := at(180, r.Max.Y+6); got == bg {
			t.Errorf("scale=%v: под нижним краем слоя тени нет", scale)
		}
		if got := at(r.Max.X+4, 130); got == bg {
			t.Errorf("scale=%v: справа от слоя тени нет", scale)
		}
		// Строки скругления в вырез не входят: там тень есть.
		if got := at(180, r.Min.Y+2); got == bg {
			t.Errorf("scale=%v: в строках скругления тени нет — вырез шире обещанного", scale)
		}
	}
}

// Пустой вырез и вырез мимо тени — обычная тень.
func TestShadowAround_EmptyCoveredIsWholeShadow(t *testing.T) {
	testreset.Globals(t)
	r := image.Rect(60, 40, 300, 220)
	draw := func(fn func(c *Canvas)) []byte {
		c := engineCanvas(360, 280)
		c.FillRect(0, 0, 360, 280, color.RGBA{R: 200, G: 210, B: 220, A: 255})
		fn(c)
		return append([]byte(nil), c.Back().Pix...)
	}
	col := color.RGBA{A: 110}
	whole := draw(func(c *Canvas) { c.DrawShadow(r, 8, 14, 0, 7, col) })
	empty := draw(func(c *Canvas) { c.DrawShadowAround(r, 8, 14, 0, 7, col, image.Rectangle{}) })
	away := draw(func(c *Canvas) { c.DrawShadowAround(r, 8, 14, 0, 7, col, image.Rect(340, 260, 350, 270)) })
	if !bytes.Equal(whole, empty) {
		t.Error("пустой вырез изменил тень")
	}
	if !bytes.Equal(whole, away) {
		t.Error("вырез мимо тени изменил её")
	}
}

// Отрисовка тени окна, которое целиком закрывает клип (тайл под окном), не
// собирает ничего: рабочий буфер тени не растёт.
func TestShadowAround_TileUnderWindowCostsNothing(t *testing.T) {
	testreset.Globals(t)
	r := image.Rect(20, 20, 620, 420)
	c := engineCanvas(640, 440)
	c.SetClip(image.Rect(256, 192, 320, 256)) // тайл в середине окна
	c.DrawShadowAround(r, 8, 16, 0, 8, color.RGBA{A: 110}, base.ShadowCovered(r, 8))
	if n := cap(c.shadowBuf); n != 0 {
		t.Errorf("тайл под окном собрал тень: рабочий буфер %d байт", n)
	}
	// Без выреза тот же тайл собирается и смешивается.
	c.DrawShadow(r, 8, 16, 0, 8, color.RGBA{A: 110})
	if cap(c.shadowBuf) == 0 {
		t.Fatal("тайл без выреза тень не собрал — проверка выше ничего не доказывает")
	}
}

func BenchmarkShadowUnderWindow(b *testing.B) {
	r := image.Rect(40, 40, 1240, 760)
	col := color.RGBA{A: 110}
	for _, around := range []bool{false, true} {
		name := "целиком"
		if around {
			name = "с вырезом"
		}
		b.Run(name, func(b *testing.B) {
			c := engineCanvas(1280, 800)
			covered := image.Rectangle{}
			if around {
				covered = base.ShadowCovered(r, 8)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.DrawShadowAround(r, 8, 16, 0, 8, col, covered)
			}
		})
	}
}
