package engine

// rasterhelpers_test.go — копии тестовых помощников core/raster, которые
// нужны и тестам движка (тестовые файлы другого пакета не импортируются).

import (
	hgraster "github.com/oops1/headless-gui/v3/core/raster"
	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"github.com/oops1/headless-gui/v3/internal/testhook"
	"image"
	"image/color"
	"math/rand"
	"os"
	"sync"
	"testing"
)

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

var color255 = color.RGBA{R: 255, G: 255, B: 255, A: 255}

// hookFallbackReads подменяет читалку запасных шрифтов счётчиком.
func hookFallbackReads(t *testing.T) *fbReads {
	t.Helper()
	r := &fbReads{}
	orig := testhook.ReadFallbackFont
	testhook.ReadFallbackFont = func(p string) ([]byte, error) {
		r.mu.Lock()
		r.paths = append(r.paths, p)
		r.mu.Unlock()
		return hgtext.ReadFontFile(p)
	}
	t.Cleanup(func() { testhook.ReadFallbackFont = orig })
	return r
}

// fbReads записывает, какие файлы запасных шрифтов были прочитаны.
type fbReads struct {
	mu    sync.Mutex
	paths []string
}

func (r *fbReads) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

func (r *fbReads) count() int { return len(r.list()) }

func testIcon(seed uint8) *image.RGBA {
	ic := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			ic.SetRGBA(x, y, color.RGBA{R: uint8(x*4) + seed, G: uint8(y * 4), B: 128, A: 255})
		}
	}
	return ic
}

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

// checkerCanvas — холст в крупную шахматную клетку: на нём размытие видно
// как усреднение соседних клеток.
func checkerCanvas(w, h, cell int) *Canvas {
	c := hgraster.NewCanvas(w, h, hgtext.NewFontCache("assets"))
	c.BlitBackground()
	black := color.RGBA{A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 0; y < h; y += cell {
		for x := 0; x < w; x += cell {
			col := black
			if ((x/cell)+(y/cell))%2 == 0 {
				col = white
			}
			c.FillRect(x, y, cell, cell, col)
		}
	}
	return c
}

const tqBG = 243

// tqFont читает Open Sans Regular и создаёт кэш шрифта с заданным слоем.
func tqFont(t *testing.T, q hgtext.TextQuality) *hgtext.FontCache {
	t.Helper()
	data, err := os.ReadFile("../assets/fonts/OpenSans-Regular.ttf")
	if err != nil {
		t.Skipf("Open Sans недоступен: %v", err)
	}
	fc := hgtext.NewFontCacheFromData(data, hgtext.DefaultDPI)
	fc.SetQuality(q)
	return fc
}

// tqRender рисует строки шрифтом fc серым цветом lum на фоне tqBG (кегль в
// пикселях при 96 dpi) и возвращает холст.
func tqRender(fc *hgtext.FontCache, sizePx int, lum uint8, texts ...string) *Canvas {
	w, h := 420, len(texts)*(sizePx+10)+8
	c := hgraster.NewCanvasScaled(w, h, 1, fc)
	c.FillRect(0, 0, w, h, color.RGBA{tqBG, tqBG, tqBG, 255})
	for i, s := range texts {
		c.DrawTextSize(s, 4, 4+i*(sizePx+10), float64(sizePx)*72/96, color.RGBA{lum, lum, lum, 255})
	}
	return c
}

// fillNoise заполняет back холста детерминированным шумом с полосами, чтобы
// часть тайлов изменилась, а часть — нет.
func fillNoise(c *Canvas, seed byte) {
	pix := c.Back().Pix
	for i := range pix {
		if (i/(c.Back().Stride*64))%3 == 0 {
			pix[i] = byte(i) ^ seed
		}
	}
}
