package engine

import (
	"bytes"
	"image"
	"image/color"
	"math/rand"
	"testing"
)

// Уменьшение и растяжение подложки переписаны ради скорости; кадр обязан
// остаться прежним до пикселя. Эталон — прежний код дословно.

func legacyDownscale(c *Canvas, src image.Rectangle) *image.RGBA {
	k := backdropDownscale
	sw, sh := (src.Dx()+k-1)/k, (src.Dy()+k-1)/k
	small := image.NewRGBA(image.Rect(0, 0, max(sw, 1), max(sh, 1)))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			var sr, sg, sb, sa, n uint32
			for dy := 0; dy < k; dy++ {
				py := src.Min.Y + y*k + dy
				if py >= src.Max.Y {
					break
				}
				off := c.back.PixOffset(src.Min.X+x*k, py)
				for dx := 0; dx < k; dx++ {
					if src.Min.X+x*k+dx >= src.Max.X {
						break
					}
					p := c.back.Pix[off : off+4 : off+4]
					sr, sg, sb, sa = sr+uint32(p[0]), sg+uint32(p[1]), sb+uint32(p[2]), sa+uint32(p[3])
					n++
					off += 4
				}
			}
			if n == 0 {
				continue
			}
			o := small.PixOffset(x, y)
			small.Pix[o], small.Pix[o+1], small.Pix[o+2], small.Pix[o+3] = uint8(sr/n), uint8(sg/n), uint8(sb/n), uint8(sa/n)
		}
	}
	return small
}

func legacyUpscale(c *Canvas, small *image.RGBA, src, dst image.Rectangle) {
	k := backdropDownscale
	for y := 0; y < small.Rect.Dy(); y++ {
		top := src.Min.Y + y*k
		if top >= dst.Max.Y {
			break
		}
		if top+k <= dst.Min.Y {
			continue
		}
		for x := 0; x < small.Rect.Dx(); x++ {
			left := src.Min.X + x*k
			if left >= dst.Max.X {
				break
			}
			if left+k <= dst.Min.X {
				continue
			}
			o := small.PixOffset(x, y)
			col := color.RGBA{R: small.Pix[o], G: small.Pix[o+1], B: small.Pix[o+2], A: small.Pix[o+3]}
			c.fillRectPx(image.Rect(left, top, left+k, top+k).Intersect(dst), col, false)
		}
	}
}

func noiseCanvas(r *rand.Rand, w, h int) *Canvas {
	c := newCanvas(w, h, newFontCache("assets"))
	r.Read(c.back.Pix)
	return c
}

func TestBackdropScaleMatchesLegacy(t *testing.T) {
	r := rand.New(rand.NewSource(21))
	const W, H = 157, 93
	for iter := 0; iter < 300; iter++ {
		a := noiseCanvas(r, W, H)
		b := newCanvas(W, H, newFontCache("assets"))
		copy(b.back.Pix, a.back.Pix)

		// Область слоя — где угодно, в том числе у самых краёв холста.
		x0, y0 := r.Intn(W-1), r.Intn(H-1)
		pr := image.Rect(x0, y0, x0+1+r.Intn(W-x0), y0+1+r.Intn(H-y0))
		src := pr.Inset(-r.Intn(12)).Intersect(a.back.Bounds())

		sa, sb := legacyDownscale(a, src), func() *image.RGBA { s, _ := b.downscaleForBlur(src, 8); return s }()
		if !bytes.Equal(sa.Pix, sb.Pix) {
			t.Fatalf("итерация %d, src %v: уменьшение разошлось с прежним", iter, src)
		}

		// Отсечение: то прямоугольное, то скруглённое, то оба.
		for _, cv := range []*Canvas{a, b} {
			cv.ClearClip()
			cv.ClearRoundClip()
		}
		switch iter % 3 {
		case 1:
			clip := image.Rect(r.Intn(W), r.Intn(H), W-r.Intn(W/2), H-r.Intn(H/2))
			a.SetClip(clip)
			b.SetClip(clip)
		case 2:
			rc := pr.Inset(r.Intn(4))
			rad := r.Intn(14)
			a.SetRoundClip(rc, rad)
			b.SetRoundClip(rc, rad)
		}
		legacyUpscale(a, sa, src, pr)
		b.upscaleInto(sb, src, pr)
		if !bytes.Equal(a.back.Pix, b.back.Pix) {
			t.Fatalf("итерация %d, pr %v, src %v: растяжение разошлось с прежним", iter, pr, src)
		}
	}
}
