package engine

import (
	"image"
	"image/color"
	"testing"
)

// solidRGBA создаёт изображение w×h, целиком залитое цветом c.
func solidRGBA(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// TestBlurRGBA_UniformFillUnchanged проверяет, что равномерная заливка
// после размытия остаётся ровно той же — в том числе у краёв изображения.
// Если бы края заворачивались или трактовались как прозрачные, у границ
// появилась бы тёмная кайма; при clamp-to-edge заливка остаётся точной,
// поскольку окно скользящей суммы всегда видит только сам цвет заливки.
func TestBlurRGBA_UniformFillUnchanged(t *testing.T) {
	c := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	img := solidRGBA(40, 30, c)

	BlurRGBA(img, 6, 3)

	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			got := img.RGBAAt(x, y)
			if got != c {
				t.Fatalf("пиксель (%d,%d) = %+v, ожидалось %+v (равномерная заливка не должна меняться)", x, y, got, c)
			}
		}
	}
}

// TestBlurRGBA_UniformFillUnchanged_Alpha проверяет то же самое для
// полупрозрачной premultiplied заливки — усреднение всех четырёх каналов
// одинаковым образом должно вернуть точно тот же premultiplied цвет.
func TestBlurRGBA_UniformFillUnchanged_Alpha(t *testing.T) {
	// Premultiplied: straight (200,100,50,128) -> premultiplied ~ (100,50,25,128).
	c := color.RGBA{R: 100, G: 50, B: 25, A: 128}
	img := solidRGBA(25, 25, c)

	BlurRGBA(img, 4, 2)

	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if got := img.RGBAAt(x, y); got != c {
				t.Fatalf("пиксель (%d,%d) = %+v, ожидалось %+v", x, y, got, c)
			}
		}
	}
}

// TestBlurRGBA_SharpEdgeSymmetric проверяет размытие резкой вертикальной
// границы белое/чёрное: слева от границы (было белым) яркость после
// размытия падает, справа (было чёрным) — растёт, а суммарная яркость
// строки сохраняется приблизительно (с точностью до ошибки округления
// целочисленного деления в скользящем окне).
func TestBlurRGBA_SharpEdgeSymmetric(t *testing.T) {
	const w, h, border = 60, 8, 30
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < border {
				img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			} else {
				img.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}

	// Сумма яркости (по R-каналу — изображение серое) строки y=0 до размытия.
	sumBefore := 0
	for x := 0; x < w; x++ {
		sumBefore += int(img.RGBAAt(x, 0).R)
	}

	BlurRGBA(img, 6, 2)

	leftVal := int(img.RGBAAt(border-2, 0).R)  // было 255, чуть левее границы
	rightVal := int(img.RGBAAt(border+1, 0).R) // было 0, чуть правее границы

	if leftVal >= 255 {
		t.Errorf("слева от границы (x=%d) яркость не упала: %d (ожидалось < 255)", border-2, leftVal)
	}
	if rightVal <= 0 {
		t.Errorf("справа от границы (x=%d) яркость не выросла: %d (ожидалось > 0)", border+1, rightVal)
	}

	sumAfter := 0
	for x := 0; x < w; x++ {
		sumAfter += int(img.RGBAAt(x, 0).R)
	}
	diff := sumBefore - sumAfter
	if diff < 0 {
		diff = -diff
	}
	// Допускаем небольшую погрешность из-за целочисленного округления
	// скользящей суммы на каждом из проходов.
	tolerance := sumBefore / 20 // 5%
	if diff > tolerance {
		t.Errorf("суммарная яркость строки не сохранилась: было %d, стало %d (расхождение %d > допуска %d)",
			sumBefore, sumAfter, diff, tolerance)
	}
}

// TestBlurRGBA_NoOpOnZeroRadiusOrPasses проверяет, что radius<=0 или
// passes<=0 не меняют изображение (и не паникуют).
func TestBlurRGBA_NoOpOnZeroRadiusOrPasses(t *testing.T) {
	mk := func() *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, 12, 9))
		for y := 0; y < 9; y++ {
			for x := 0; x < 12; x++ {
				img.SetRGBA(x, y, color.RGBA{uint8(x * 10), uint8(y * 20), 50, 255})
			}
		}
		return img
	}

	cases := []struct {
		name           string
		radius, passes int
	}{
		{"radius=0", 0, 3},
		{"radius=-1", -1, 3},
		{"passes=0", 5, 0},
		{"passes=-1", 5, -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := mk()
			orig := mk()
			BlurRGBA(img, tc.radius, tc.passes)
			b := img.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					if got, want := img.RGBAAt(x, y), orig.RGBAAt(x, y); got != want {
						t.Fatalf("(%d,%d) = %+v, ожидалось без изменений %+v", x, y, got, want)
					}
				}
			}
		})
	}
}

// TestBlurRegion_DoesNotAffectOutsidePixels проверяет, что размытие
// прямоугольной области не задевает пиксели за её пределами.
func TestBlurRegion_DoesNotAffectOutsidePixels(t *testing.T) {
	const w, h = 50, 40
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	orig := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Шахматный узор высокого контраста — чувствителен к любой утечке размытия.
			c := color.RGBA{0, 0, 0, 255}
			if (x+y)%2 == 0 {
				c = color.RGBA{255, 255, 255, 255}
			}
			img.SetRGBA(x, y, c)
			orig.SetRGBA(x, y, c)
		}
	}

	region := image.Rect(10, 8, 30, 25)
	BlurRegion(img, region, 5, 3)

	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			pt := image.Pt(x, y)
			if pt.In(region) {
				continue
			}
			if got, want := img.RGBAAt(x, y), orig.RGBAAt(x, y); got != want {
				t.Fatalf("пиксель (%d,%d) вне области размытия изменился: было %+v, стало %+v", x, y, want, got)
			}
		}
	}

	// А внутри области узор действительно должен был смешаться (не остаться
	// шахматкой из чистых чёрного/белого).
	changed := false
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			if got, want := img.RGBAAt(x, y), orig.RGBAAt(x, y); got != want {
				changed = true
			}
		}
	}
	if !changed {
		t.Fatal("внутри области размытия ничего не изменилось")
	}
}

// BenchmarkBlurRGBA измеряет стоимость box-размытия на типичном для
// панели изображении 480x120 с радиусом 8 (2 прохода, как для acrylic/mica).
func BenchmarkBlurRGBA(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 480, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 480; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), uint8(x + y), 255})
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BlurRGBA(img, 8, 2)
	}
}
