package engine

import (
	"image"
	"image/color"
	"math/rand"
	"testing"

	"golang.org/x/image/draw"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// noiseRGBA — детерминированное шумное изображение: на гладком градиенте
// ошибки в коэффициентах не видны.
func noiseRGBA(w, h int, seed int64, alpha bool) *image.RGBA {
	rnd := rand.New(rand.NewSource(seed))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		a := uint8(255)
		if alpha {
			a = uint8(rnd.Intn(256))
		}
		img.Pix[i+3] = a
		for k := 0; k < 3; k++ { // premultiplied: цвет не больше альфы
			img.Pix[i+k] = uint8(rnd.Intn(int(a) + 1))
		}
	}
	return img
}

func toNRGBA(src *image.RGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

func toYCbCr(src *image.RGBA, ratio image.YCbCrSubsampleRatio) *image.YCbCr {
	dst := image.NewYCbCr(src.Bounds(), ratio)
	b := src.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := src.RGBAAt(x, y)
			yy, cb, cr := color.RGBToYCbCr(c.R, c.G, c.B)
			dst.Y[dst.YOffset(x, y)] = yy
			ci := dst.COffset(x, y)
			dst.Cb[ci], dst.Cr[ci] = cb, cr
		}
	}
	return dst
}

func kernelFor(f widget.ImageFilter) draw.Interpolator {
	switch f {
	case widget.FilterNearest:
		return draw.NearestNeighbor
	case widget.FilterBilinear:
		return draw.BiLinear
	}
	return draw.CatmullRom
}

// diffMax — наибольшее расхождение байтов двух RGBA в области r.
func diffMax(a, b *image.RGBA, r image.Rectangle) int {
	m := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			pa, pb := a.PixOffset(x, y), b.PixOffset(x, y)
			for k := 0; k < 4; k++ {
				d := int(a.Pix[pa+k]) - int(b.Pix[pb+k])
				if d < 0 {
					d = -d
				}
				if d > m {
					m = d
				}
			}
		}
	}
	return m
}

// Видимая часть совпадает с эталоном: полное масштабирование golang.org/x/image
// того же sr на тот же dr, обрезанное по клипу. Для каждого фильтра, типа
// источника и направления масштабирования; допуск — на округление (x/image
// отбрасывает младший байт 16-битного значения, здесь округление).
func TestDrawImageRect_MatchesFullScale(t *testing.T) {
	testreset.Globals(t)
	base := noiseRGBA(97, 71, 1, false)
	baseA := noiseRGBA(97, 71, 2, true)
	sources := []struct {
		name string
		img  image.Image
		tol  int
	}{
		{"RGBA", base, 2},
		{"RGBA/альфа", baseA, 2},
		{"NRGBA/альфа", toNRGBA(baseA), 3},
		{"YCbCr444", toYCbCr(base, image.YCbCrSubsampleRatio444), 3},
		{"YCbCr420", toYCbCr(base, image.YCbCrSubsampleRatio420), 3},
		{"Gray", func() image.Image {
			g := image.NewGray(base.Bounds())
			draw.Draw(g, g.Bounds(), base, image.Point{}, draw.Src)
			return g
		}(), 3},
	}
	cases := []struct {
		name string
		sr   image.Rectangle
		dr   image.Rectangle // назначение (физические = логические, масштаб 1)
		clip image.Rectangle
	}{
		{"увеличение", image.Rect(0, 0, 97, 71), image.Rect(20, 15, 320, 240), image.Rect(80, 60, 200, 150)},
		{"уменьшение", image.Rect(0, 0, 97, 71), image.Rect(10, 10, 41, 29), image.Rect(15, 12, 38, 27)},
		{"часть источника", image.Rect(13, 9, 80, 55), image.Rect(0, 0, 201, 150), image.Rect(30, 20, 170, 130)},
		{"клип накрывает всё", image.Rect(0, 0, 97, 71), image.Rect(5, 5, 130, 100), image.Rect(0, 0, 400, 300)},
		{"сильное уменьшение", image.Rect(0, 0, 97, 71), image.Rect(50, 50, 59, 56), image.Rect(0, 0, 400, 300)},
		{"анизотропия", image.Rect(0, 0, 97, 71), image.Rect(10, 10, 210, 40), image.Rect(60, 12, 160, 38)},
	}
	for _, f := range []widget.ImageFilter{widget.FilterNearest, widget.FilterBilinear, widget.FilterCatmullRom} {
		for _, s := range sources {
			for _, cs := range cases {
				ref := image.NewRGBA(image.Rect(0, 0, 400, 300))
				kernelFor(f).Scale(ref, cs.dr, s.img, cs.sr, draw.Src, nil)

				c := newTestCanvas(400, 300)
				c.back = image.NewRGBA(image.Rect(0, 0, 400, 300)) // прозрачный фон: Over == Src
				c.SetClip(cs.clip)
				c.DrawImageRect(s.img, cs.sr, cs.dr, f)
				c.ClearClip()

				vis := cs.dr.Intersect(cs.clip)
				tol := s.tol
				if f == widget.FilterNearest {
					tol = 0
					switch s.name {
					case "NRGBA/альфа":
						tol = 1 // умножение на альфу округляется иначе, чем у x/image
					case "YCbCr444", "YCbCr420", "Gray":
						tol = 3 // разные формулы YCbCr→RGB у источника и эталона
					}
				}
				if d := diffMax(c.back, ref, vis); d > tol {
					t.Errorf("%v/%s/%s: расхождение с эталоном %d (допуск %d)", f, s.name, cs.name, d, tol)
				}
				// Вне клипа и вне dr ничего не нарисовано.
				for y := 0; y < 300; y++ {
					for x := 0; x < 400; x++ {
						if image.Pt(x, y).In(vis) {
							continue
						}
						if off := c.back.PixOffset(x, y); c.back.Pix[off+3] != 0 || c.back.Pix[off] != 0 {
							t.Fatalf("%v/%s/%s: пиксель (%d,%d) вне видимой части изменён", f, s.name, cs.name, x, y)
						}
					}
				}
			}
		}
	}
}

// Гигантский dr (×1000), клип — небольшое окно: рисуется мгновенно и гладко.
func TestDrawImageRect_HugeDestination(t *testing.T) {
	testreset.Globals(t)
	src := noiseRGBA(60, 40, 3, false)
	sr := image.Rect(0, 0, 60, 40)
	dr := image.Rect(-30000, -20000, 30000, 20000) // ×1000
	clip := image.Rect(100, 100, 220, 180)

	c := newTestCanvas(300, 240)
	c.back = image.NewRGBA(image.Rect(0, 0, 300, 240))
	c.SetClip(clip)
	c.DrawImageRect(src, sr, dr, widget.FilterBilinear)
	c.ClearClip()

	// Эталон полным масштабированием здесь не построить (60000×40000), поэтому
	// проверяется плавность: при увеличении ×1000 соседние пиксели окна
	// отличаются не больше чем на шаг ядра (шум источника — 0…255, на 1000
	// пикселей назначения приходится один перепад).
	maxStep := 0
	for y := 100; y < 180; y++ {
		for x := 101; x < 220; x++ {
			a, b := c.back.PixOffset(x-1, y), c.back.PixOffset(x, y)
			for k := 0; k < 3; k++ {
				d := int(c.back.Pix[a+k]) - int(c.back.Pix[b+k])
				if d < 0 {
					d = -d
				}
				if d > maxStep {
					maxStep = d
				}
			}
		}
	}
	if maxStep > 8 {
		t.Fatalf("увеличение ×1000 не гладкое: шаг между соседями %d", maxStep)
	}
	if c.back.Pix[c.back.PixOffset(150, 140)+3] != 255 {
		t.Fatal("окно в середине гигантского dr не залито")
	}
}

// Работа пропорциональна клипу: тот же dr с узким клипом дороже совсем
// чуть-чуть, а не как полный размер.
func TestDrawImageRect_WorkProportionalToClip(t *testing.T) {
	testreset.Globals(t)
	src := noiseRGBA(2000, 1500, 4, false)
	sr := image.Rect(0, 0, 2000, 1500)
	dr := image.Rect(-4000, -3000, 12000, 9000) // ×8

	ops := func(clip image.Rectangle) int64 {
		c := newTestCanvas(800, 600)
		c.SetClip(clip)
		c.DrawImageRect(src, sr, dr, widget.FilterCatmullRom)
		return c.rscr.ops
	}
	small := ops(image.Rect(100, 100, 200, 180)) // 100×80
	full := ops(image.Rect(0, 0, 800, 600))      // окно целиком

	// Площади относятся как 8000 : 480000 = 1 : 60; работа — не хуже 1 : 20
	// (накладные расходы: охват источника и окно ядра).
	if small*20 > full {
		t.Fatalf("работа не пропорциональна клипу: узкий клип %d, окно %d", small, full)
	}
	// И в абсолютном выражении далека от полного размера dr (16000×12000).
	if small > 5_000_000 {
		t.Fatalf("узкий клип посчитал %d операций — как будто масштабируется всё", small)
	}
}

// Клип целиком вне dr — ничего не делает и ничего не выделяет.
func TestDrawImageRect_NothingVisible(t *testing.T) {
	testreset.Globals(t)
	src := noiseRGBA(32, 32, 5, false)
	c := newTestCanvas(100, 100)
	before := append([]byte(nil), c.back.Pix...)
	c.SetClip(image.Rect(60, 60, 90, 90))
	c.DrawImageRect(src, src.Bounds(), image.Rect(0, 0, 40, 40), widget.FilterBilinear)
	c.DrawImageRect(src, image.Rectangle{}, image.Rect(0, 0, 40, 40), widget.FilterBilinear)
	c.DrawImageRect(nil, src.Bounds(), image.Rect(0, 0, 40, 40), widget.FilterBilinear)
	for i := range before {
		if before[i] != c.back.Pix[i] {
			t.Fatal("рисование вне клипа изменило буфер")
		}
	}
}

// sr за границами источника обрезается вместе с пропорциональной частью dr:
// картинка не растягивается.
func TestDrawImageRect_SrcOutOfBounds(t *testing.T) {
	testreset.Globals(t)
	src := noiseRGBA(50, 50, 6, false)
	a := newTestCanvas(200, 200)
	a.back = image.NewRGBA(image.Rect(0, 0, 200, 200))
	b := newTestCanvas(200, 200)
	b.back = image.NewRGBA(image.Rect(0, 0, 200, 200))

	// sr вдвое шире источника справа: правая половина dr отбрасывается.
	a.DrawImageRect(src, image.Rect(0, 0, 100, 50), image.Rect(0, 0, 200, 100), widget.FilterNearest)
	b.DrawImageRect(src, image.Rect(0, 0, 50, 50), image.Rect(0, 0, 100, 100), widget.FilterNearest)
	if d := diffMax(a.back, b.back, image.Rect(0, 0, 200, 200)); d != 0 {
		t.Fatalf("обрезка sr не пропорциональна: расхождение %d", d)
	}
}

// Скруглённый клип и порядок каналов BGRX работают как у остальных
// изображений.
func TestDrawImageRect_RoundClipAndBGRX(t *testing.T) {
	testreset.Globals(t)
	src := noiseRGBA(64, 48, 7, false)
	sr := src.Bounds()
	dr := image.Rect(10, 10, 90, 70)
	rect := image.Rect(10, 10, 90, 70)

	rgba := imageRoundCanvas(120, 100, 1, FormatRGBA)
	bgrx := imageRoundCanvas(120, 100, 1, FormatBGRX)
	plain := imageRoundCanvas(120, 100, 1, FormatRGBA)
	for _, c := range []*Canvas{rgba, bgrx} {
		c.SetRoundClip(rect, 16)
		c.DrawImageRect(src, sr, dr, widget.FilterBilinear)
		c.ClearRoundClip()
		c.ClearClip()
	}
	plain.DrawImageRect(src, sr, dr, widget.FilterBilinear)

	// BGRX — те же пиксели с переставленными R и B.
	for i := 0; i < len(rgba.back.Pix); i += 4 {
		if rgba.back.Pix[i] != bgrx.back.Pix[i+2] || rgba.back.Pix[i+1] != bgrx.back.Pix[i+1] ||
			rgba.back.Pix[i+2] != bgrx.back.Pix[i] {
			t.Fatalf("BGRX расходится с RGBA в байте %d", i)
		}
	}
	// Углы обрезаны, середина совпадает с рисованием без скругления.
	if rgba.back.RGBAAt(10, 10) != (color.RGBA{R: 30, G: 60, B: 90, A: 255}) {
		t.Error("угол под скруглённым клипом закрашен изображением")
	}
	if rgba.back.RGBAAt(50, 40) != plain.back.RGBAAt(50, 40) {
		t.Error("середина под скруглённым клипом отличается от обычной")
	}
}

// На обычном масштабе и на HiDPI DrawImageRect с билинейным ядром и полным sr
// даёт то же, что DrawImageScaled (оба — билинейное ядро x/image), — это и
// есть «DrawImageRect рисует так же, как раньше, только дешевле».
func TestDrawImageRect_AgreesWithDrawImageScaled(t *testing.T) {
	testreset.Globals(t)
	src := noiseRGBA(80, 60, 8, false)
	for _, scale := range []float64{1, 1.5, 2} {
		a := imageRoundCanvas(160, 120, scale, FormatRGBA)
		b := imageRoundCanvas(160, 120, scale, FormatRGBA)
		a.DrawImageScaled(src, 10, 12, 100, 70)
		b.DrawImageRect(src, src.Bounds(), image.Rect(10, 12, 110, 82), widget.FilterBilinear)
		if d := diffMax(a.back, b.back, a.back.Bounds()); d > 2 {
			t.Errorf("scale=%v: DrawImageRect расходится с DrawImageScaled на %d", scale, d)
		}
	}
}

// Бенчмарки: стоимость кадра зума от размера окна, а не от размера картинки.
func BenchmarkDrawImageRect_Zoom(b *testing.B) {
	src := noiseRGBA(4000, 3000, 9, false)
	for _, f := range []widget.ImageFilter{widget.FilterNearest, widget.FilterBilinear, widget.FilterCatmullRom} {
		b.Run(f.String(), func(b *testing.B) {
			c := newCanvas(1200, 800, newFontCache("assets"))
			dr := image.Rect(-8000, -6000, 8000, 6000) // зум ×4
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.DrawImageRect(src, src.Bounds(), dr, f)
			}
		})
	}
}

func BenchmarkDrawImageRect_FitWindow(b *testing.B) {
	src := noiseRGBA(4000, 3000, 10, false)
	c := newCanvas(1200, 800, newFontCache("assets"))
	dr := image.Rect(0, 0, 1200, 800)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.DrawImageRect(src, src.Bounds(), dr, widget.FilterBilinear)
	}
}
