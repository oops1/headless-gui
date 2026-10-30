package raster

import (
	"bytes"
	hgdraw "github.com/oops1/headless-gui/v4/core/draw"
	"github.com/oops1/headless-gui/v4/core/geom"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"image"
	"image/color"
	"testing"
)

// Непрозрачный источник, положенный быстрым путём (Src, полосами по ядрам),
// даёт тот же кадр, что и композиция Over, — для каждого типа источника,
// порядка каналов, масштаба, клипа и скруглённого клипа, малых и больших
// (параллельных) областей.
func TestOpaqueBlit_EqualsOver(t *testing.T) {
	testreset.Globals(t)
	type srcMaker struct {
		name string
		make func(w, h int) image.Image
	}
	makers := []srcMaker{
		{"RGBA", func(w, h int) image.Image { return opaqueNoise(w, h, 1) }},
		{"NRGBA", func(w, h int) image.Image {
			n := image.NewNRGBA(image.Rect(0, 0, w, h))
			copy(n.Pix, opaqueNoise(w, h, 2).Pix)
			return n
		}},
		{"YCbCr", func(w, h int) image.Image {
			y := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
			for i := range y.Y {
				y.Y[i] = uint8(i * 3)
			}
			for i := range y.Cb {
				y.Cb[i], y.Cr[i] = uint8(i*5), uint8(255-i)
			}
			return y
		}},
		{"Gray", func(w, h int) image.Image {
			g := image.NewGray(image.Rect(0, 0, w, h))
			for i := range g.Pix {
				g.Pix[i] = uint8(i * 13)
			}
			return g
		}},
		{"RGBA со смещением Min", func(w, h int) image.Image {
			return opaqueNoise(w+9, h+7, 3).SubImage(image.Rect(4, 3, 4+w, 3+h))
		}},
	}
	sizes := [][2]int{{64, 40}, {700, 500}} // второй — больше порога параллельности
	for _, mk := range makers {
		for _, f := range []PixelFormat{FormatRGBA, FormatBGRX} {
			for _, sz := range sizes {
				for _, mode := range []string{"plain", "clip", "round"} {
					w, h := sz[0], sz[1]
					cw, ch := w+60, h+50
					fast := imageRoundCanvas(cw, ch, 1, f)
					ref := imageRoundCanvas(cw, ch, 1, f)
					src := mk.make(w, h)
					prep := func(c *Canvas) {
						switch mode {
						case "clip":
							c.SetClip(image.Rect(20, 15, cw-25, ch-20))
						case "round":
							c.SetRoundClip(image.Rect(20, 15, cw-25, ch-20), 18)
						}
					}
					prep(fast)
					fast.DrawImage(hgdraw.OpaqueImage(src), 17, 11)
					prep(ref)
					ref.DrawImage(src, 17, 11)
					for _, c := range []*Canvas{fast, ref} {
						c.ClearRoundClip()
						c.ClearClip()
					}
					if !bytes.Equal(fast.back.Pix, ref.back.Pix) {
						t.Errorf("%s fmt=%v %dx%d %s: быстрый путь расходится с Over (макс. разница %d)",
							mk.name, f, w, h, mode, maxPixDiff(fast.back, ref.back))
					}
				}
			}
		}
	}
}

// Путь действительно быстрый: полупрозрачные пиксели, объявленные
// непрозрачными, ПЕРЕЗАПИСЫВАЮТ фон (Src), а без объявления смешиваются (Over).
// Различие и доказывает, что выбрана композиция Src — для DrawImage,
// DrawImageScaled 1:1 и DrawImageRect 1:1, малой и большой (параллельной)
// области, обоих форматов.
func TestOpaqueBlit_UsesSrcComposition(t *testing.T) {
	testreset.Globals(t)
	half := color.RGBA{R: 100, G: 50, B: 20, A: 128}
	bg := color.RGBA{R: 30, G: 60, B: 90, A: 255}
	draws := map[string]func(c *Canvas, img image.Image, w, h int){
		"DrawImage": func(c *Canvas, img image.Image, w, h int) { c.DrawImage(img, 0, 0) },
		"DrawImageScaled": func(c *Canvas, img image.Image, w, h int) {
			c.DrawImageScaled(img, 0, 0, w, h)
		},
		"DrawImageRect": func(c *Canvas, img image.Image, w, h int) {
			c.DrawImageRect(img, image.Rect(0, 0, w, h), image.Rect(0, 0, w, h), geom.FilterBilinear)
		},
	}
	for name, draw := range draws {
		for _, f := range []PixelFormat{FormatRGBA, FormatBGRX} {
			for _, sz := range [][2]int{{40, 30}, {800, 600}} {
				w, h := sz[0], sz[1]
				src := flatImage(w, h, half)

				c := imageRoundCanvas(w, h, 1, f)
				draw(c, hgdraw.OpaqueImage(src), w, h)
				want := half
				if f == FormatBGRX {
					want.R, want.B = want.B, want.R
				}
				for _, p := range [][2]int{{0, 0}, {w - 1, h - 1}, {w / 2, h / 2}, {3, h - 2}} {
					o := c.back.PixOffset(p[0], p[1])
					got := color.RGBA{c.back.Pix[o], c.back.Pix[o+1], c.back.Pix[o+2], c.back.Pix[o+3]}
					if got != want {
						t.Fatalf("%s fmt=%v %dx%d: пиксель %v = %v, ожидался Src %v", name, f, w, h, p, got, want)
					}
				}

				// Без объявления — прежний Over: фон просвечивает.
				c2 := imageRoundCanvas(w, h, 1, f)
				draw(c2, src, w, h)
				o := c2.back.PixOffset(w/2, h/2)
				got := color.RGBA{c2.back.Pix[o], c2.back.Pix[o+1], c2.back.Pix[o+2], c2.back.Pix[o+3]}
				if got == want {
					t.Fatalf("%s fmt=%v %dx%d: без объявления тоже получен Src %v — путь Over потерян", name, f, w, h, got)
				}
				_ = bg
			}
		}
	}
}

// DrawImageRect при масштабе 1:1 даёт то же, что полный пересчёт ядром
// (тождество), для *image.RGBA — в том числе с альфой.
func TestDrawImageRect_OneToOneMatchesResample(t *testing.T) {
	testreset.Globals(t)
	src := opaqueNoise(90, 70, 4)
	for i := 3; i < len(src.Pix); i += 4 * 5 { // немного полупрозрачных
		src.Pix[i-3], src.Pix[i-2], src.Pix[i-1], src.Pix[i] = 20, 30, 40, 100
	}
	for _, f := range []geom.ImageFilter{geom.FilterNearest, geom.FilterBilinear, geom.FilterCatmullRom} {
		c := imageRoundCanvas(150, 120, 1, FormatRGBA)
		ref := imageRoundCanvas(150, 120, 1, FormatRGBA)
		sr := image.Rect(5, 3, 85, 63)
		dr := image.Rect(30, 20, 110, 80)
		c.SetClip(image.Rect(40, 25, 100, 70))
		c.DrawImageRect(src, sr, dr, f)
		c.ClearClip()

		ref.SetClip(image.Rect(40, 25, 100, 70))
		vis := ref.clampRect(ref.PhysRect(dr)).Intersect(ref.back.Bounds())
		sc := &rectScratch{}
		tmp := sc.resampleRect(src, sr, ref.PhysRect(dr), vis, f)
		ref.blitOver(vis, tmp, tmp.Rect.Min)
		ref.ClearClip()
		if !bytes.Equal(c.back.Pix, ref.back.Pix) {
			t.Errorf("фильтр %v: 1:1 расходится с пересчётом (на %d)", f, maxPixDiff(c.back, ref.back))
		}
	}
}

// Кэш масштабирования ключуется по обёрнутому изображению: сброс по любому из
// двух указателей — по обёртке или по самой картинке — убирает запись.
func TestOpaqueImage_ScaledCacheKeyedByInner(t *testing.T) {
	testreset.Globals(t)
	c := imageRoundCanvas(100, 100, 1, FormatRGBA)
	src := opaqueNoise(32, 32, 1)
	wrapped := hgdraw.OpaqueImage(src)
	c.DrawImageScaled(wrapped, 0, 0, 50, 50)
	c.DrawImageScaled(src, 0, 0, 50, 50)
	if len(c.scaledCache) != 1 {
		t.Fatalf("обёртка и сама картинка должны делить запись кэша, записей: %d", len(c.scaledCache))
	}
	c.InvalidateImageCache(wrapped)
	if len(c.scaledCache) != 0 {
		t.Fatalf("сброс по обёртке не убрал запись, осталось %d", len(c.scaledCache))
	}
}

// BlitPixels на холсте: тот же результат, что DrawImage на масштабе 1, и
// физический 1:1 на масштабе 2, где DrawImage растянул бы.
func TestCanvasBlitPixels(t *testing.T) {
	testreset.Globals(t)
	src := opaqueNoise(40, 30, 3)

	a := NewCanvas(100, 80, nil)
	b := NewCanvas(100, 80, nil)
	a.BlitPixels(src, src.Rect, 10, 12, true)
	b.DrawImage(hgdraw.OpaqueImage(src), 10, 12)
	if !bytes.Equal(a.back.Pix, b.back.Pix) {
		t.Fatal("BlitPixels расходится с DrawImage на масштабе 1")
	}

	// Часть буфера и клип.
	c := NewCanvas(100, 80, nil)
	c.SetClip(image.Rect(0, 0, 25, 100))
	c.BlitPixels(src, image.Rect(5, 5, 35, 25), 10, 12, true)
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			got := c.back.RGBAAt(x, y)
			in := x >= 15 && x < 25 && y >= 17 && y < 37 // sr.Min (5,5) → (10+5, 12+5)
			if in && got != src.RGBAAt(x-10, y-12) {
				t.Fatalf("(%d,%d): %v, ждали пиксель буфера", x, y, got)
			}
			if !in && got != (color.RGBA{}) {
				t.Fatalf("(%d,%d) вне клипа или части: %v", x, y, got)
			}
		}
	}

	// Масштаб 2: пиксели 1:1, начало — физическая позиция логической точки.
	d := NewCanvasScaled(50, 40, 2, nil)
	d.BlitPixels(src, src.Rect, 5, 6, true)
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			if got := d.back.RGBAAt(10+x, 12+y); got != src.RGBAAt(x, y) {
				t.Fatalf("scale 2 (%d,%d): %v", x, y, got)
			}
		}
	}
	if d.back.RGBAAt(9, 12) != (color.RGBA{}) || d.back.RGBAAt(50, 12) != (color.RGBA{}) {
		t.Fatal("scale 2: пиксели за пределами буфера")
	}

	// Порядок байт BGRX: R и B переставляются, как у остальных блитов.
	e := NewCanvas(20, 20, nil)
	e.setOwnFormat(FormatBGRX)
	px := image.NewRGBA(image.Rect(0, 0, 2, 2))
	px.SetRGBA(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	e.BlitPixels(px, px.Rect, 0, 0, true)
	if o := e.back.Pix[:4]; o[0] != 3 || o[1] != 2 || o[2] != 1 {
		t.Fatalf("BGRX: %v", o)
	}

	// Пустое/nil/вне холста — без паники и без записи.
	e2 := NewCanvas(20, 20, nil)
	e2.BlitPixels(nil, image.Rect(0, 0, 5, 5), 0, 0, true)
	e2.BlitPixels(px, image.Rectangle{}, 0, 0, true)
	e2.BlitPixels(px, image.Rect(10, 10, 20, 20), 0, 0, true)
	e2.BlitPixels(px, px.Rect, 500, 500, true)
	e2.BlitPixels(px, px.Rect, -500, -500, false)
	for _, v := range e2.back.Pix {
		if v != 0 {
			t.Fatal("запись при пустых/внешних аргументах")
		}
	}
}

// opaqueNoise — непрозрачное RGBA-изображение с неоднородным содержимым.
func opaqueNoise(w, h int, seed int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := img.PixOffset(x, y)
			img.Pix[o+0] = uint8(x*7 + seed)
			img.Pix[o+1] = uint8(y*11 + seed*3)
			img.Pix[o+2] = uint8((x ^ y) + seed*5)
			img.Pix[o+3] = 255
		}
	}
	return img
}
