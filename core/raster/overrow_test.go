package raster

import (
	"bytes"
	"image"
	"image/color"
	"math/rand"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Смешивание Over (overrow.go) обязано давать те же байты, что цикл по
// каналам, стоявший в drawColorGlyph: тень, эмодзи и цветные глифы
// не должны сдвинуться ни на единицу.

// overRef — прежний цикл drawColorGlyph, дословно.
func overRef(dst, src []byte, swap bool) {
	for i := 0; i+4 <= len(src); i += 4 {
		sa := uint32(src[i+3])
		if sa == 0 {
			continue
		}
		sr, sg, sb := src[i+0], src[i+1], src[i+2]
		if swap {
			sr, sb = sb, sr
		}
		inv := 255 - sa
		p := dst[i : i+4 : i+4]
		p[0] = uint8(uint32(sr) + uint32(p[0])*inv/255)
		p[1] = uint8(uint32(sg) + uint32(p[1])*inv/255)
		p[2] = uint8(uint32(sb) + uint32(p[2])*inv/255)
		p[3] = uint8(sa + uint32(p[3])*inv/255)
	}
}

// Замена деления на 255 сдвигами точна на всём диапазоне произведения двух
// байт.
func TestOverRow_DivisionBy255IsExact(t *testing.T) {
	testreset.Globals(t)
	for x := uint32(0); x <= 255*255; x++ {
		if got, want := (x+1+x>>8)>>8, x/255; got != want {
			t.Fatalf("x=%d: сдвигами %d, делением %d", x, got, want)
		}
	}
}

// Заливка одним пикселем: все альфы источника, все значения канала холста и
// характерные значения канала источника — каждая дорожка слова считает то же,
// что прежний цикл. Длина строки нечётная: последний пиксель идёт без пары.
func TestOverFill_EveryChannelValue(t *testing.T) {
	testreset.Globals(t)
	dst := make([]byte, 257*4)
	for d := 0; d < 257; d++ {
		dst[d*4+0], dst[d*4+1], dst[d*4+2], dst[d*4+3] = byte(d), byte(d*3), byte(255-d), byte(d^0x5a)
	}
	for sa := 0; sa < 256; sa++ {
		// Каналы источника не обязательно ≤ альфы: переполнение канала обязано
		// остаться в своём байте.
		for _, ch := range []int{0, 1, sa / 2, sa, 127, 128, 254, 255} {
			px := []byte{byte(ch), byte(255 - ch), byte(ch*7 + sa), byte(sa)}
			same := bytes.Repeat(px, 257)
			for _, swap := range []bool{false, true} {
				want := append([]byte(nil), dst...)
				overRef(want, same, swap)
				got := append([]byte(nil), dst...)
				overFill(got, px, swap)
				if !bytes.Equal(got, want) {
					t.Fatalf("пиксель %v swap=%v: заливка разошлась с прежним циклом", px, swap)
				}
			}
		}
	}
}

// Случайные строки разной длины, включая пустую, и заливка одним пикселем.
func TestOverRow_RandomRowsAndFill(t *testing.T) {
	testreset.Globals(t)
	rng := rand.New(rand.NewSource(7))
	for n := 0; n < 300; n++ {
		w := rng.Intn(70)
		src := make([]byte, w*4)
		dst := make([]byte, w*4)
		rng.Read(src)
		rng.Read(dst)
		if n%3 == 0 { // доля прозрачных пикселей
			for i := 3; i < len(src); i += 8 {
				src[i] = 0
			}
		}
		for _, swap := range []bool{false, true} {
			want := append([]byte(nil), dst...)
			overRef(want, src, swap)
			got := append([]byte(nil), dst...)
			overRow(got, src, swap)
			if !bytes.Equal(got, want) {
				t.Fatalf("строка %d (ширина %d, swap=%v): разошлось с прежним циклом", n, w, swap)
			}

			px := []byte{byte(rng.Intn(256)), byte(rng.Intn(256)), byte(rng.Intn(256)), byte(rng.Intn(256))}
			if n%5 == 0 {
				px[3] = 0
			}
			same := bytes.Repeat(px, w)
			want = append(want[:0], dst...)
			overRef(want, same, swap)
			got = append(got[:0], dst...)
			overFill(got, px, swap)
			if !bytes.Equal(got, want) {
				t.Fatalf("заливка %d (ширина %d, swap=%v, пиксель %v): разошлась с прежним циклом", n, w, swap, px)
			}
		}
	}
}

// Тень из шаблона, смешанная прямо в холст, побайтно равна тени прямого пути
// (размытие целого силуэта и drawColorGlyph) — под прямоугольным и скруглённым
// клипом, в обоих форматах, на дробном масштабе.
func TestShadowTemplate_DirectBlendMatchesBlurPath(t *testing.T) {
	testreset.Globals(t)
	r := image.Rect(60, 40, 300, 220)
	for _, scale := range []float64{1, 1.5} {
		for _, format := range []PixelFormat{FormatRGBA, FormatBGRX} {
			for _, round := range []bool{false, true} {
				draw := func(cache bool) []byte {
					c := scaledCanvas(360, 280, scale)
					c.SetPixelFormat(format, nil)
					c.SetShadowCacheEnabled(cache)
					lw, lh := c.LogicalSize()
					for y := 0; y < lh; y += 8 {
						c.FillRect(0, y, lw, 8, color.RGBA{R: uint8(40 + y), G: uint8(200 - y), B: 90, A: 255})
					}
					if round {
						c.SetRoundClip(image.Rect(30, 20, 330, 250), 40)
					} else {
						c.SetClip(image.Rect(45, 30, 250, 245))
					}
					c.DrawShadow(r, 8, 14, 3, 7, color.RGBA{R: 10, G: 20, B: 60, A: 120})
					return append([]byte(nil), c.Back().Pix...)
				}
				if a, b := draw(true), draw(false); !bytes.Equal(a, b) {
					t.Errorf("scale=%v format=%d round=%v: тень из шаблона отличается от прямого размытия", scale, format, round)
				}
			}
		}
	}
}

func BenchmarkOverRow(b *testing.B) {
	src := make([]byte, 1024*4)
	dst := make([]byte, 1024*4)
	rng := rand.New(rand.NewSource(1))
	rng.Read(src)
	rng.Read(dst)
	b.Run("строка", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		for i := 0; i < b.N; i++ {
			overRow(dst, src, false)
		}
	})
	px := []byte{0, 0, 0, 90}
	same := bytes.Repeat(px, 1024)
	b.Run("заливка", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		for i := 0; i < b.N; i++ {
			overFill(dst, px, false)
		}
	})
	b.Run("заливка прежним циклом", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		for i := 0; i < b.N; i++ {
			overRef(dst, same, false)
		}
	})
}
