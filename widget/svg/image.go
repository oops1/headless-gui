package svg

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"  // декодеры для data:-картинок
	_ "image/jpeg" // (регистрируются в image.Decode)
	_ "image/png"
	"math"
	"net/url"
	"strings"
)

// Image — растровая картинка из <image> с data:-ссылкой (PNG/JPEG/GIF).
// Внешние файлы и вложенные SVG не поддержаны.
type Image struct {
	// Pix — пиксели, премультиплицированный RGBA.
	Pix *image.RGBA
	// M переводит пиксели картинки (0..Dx, 0..Dy) в координаты viewBox.
	M Matrix
}

// maxImagePixels — предел размера одной вложенной картинки (в пикселях) и
// суммарно по документу: иначе маленький файл раздувается в сотни мегабайт.
const maxImagePixels = 8 << 20

// decodeDataURI декодирует data:image/...;base64,... в картинку.
func decodeDataURI(s string) (image.Image, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 6 || !strings.EqualFold(s[:5], "data:") {
		return nil, false
	}
	comma := strings.IndexByte(s, ',')
	if comma < 0 {
		return nil, false
	}
	head := strings.ToLower(s[5:comma])
	payload := s[comma+1:]
	var raw []byte
	if strings.HasSuffix(head, ";base64") {
		// В атрибуте base64 нередко разбит переносами строк.
		payload = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, payload)
		var err error
		raw, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(payload, "="))
			if err != nil {
				return nil, false
			}
		}
	} else {
		dec, err := url.PathUnescape(payload)
		if err != nil {
			return nil, false
		}
		raw = []byte(dec)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxImagePixels {
		return nil, false
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}
	return img, true
}

// addImage добавляет <image> как фигуру-картинку.
func (b *builder) addImage(n *xnode, st inherited) {
	if b.clipMode || st.hidden && !b.noEffects {
		return
	}
	href, ok := n.attr("href")
	if !ok {
		return
	}
	x, y := lenAttr(n, "x"), lenAttr(n, "y")
	w, h := lenAttr(n, "width"), lenAttr(n, "height")

	var pix *image.RGBA
	iw, ih := 0, 0
	if !b.noEffects {
		img, ok := decodeDataURI(href)
		if !ok {
			return
		}
		bnd := img.Bounds()
		iw, ih = bnd.Dx(), bnd.Dy()
		if b.shared.pixels += iw * ih; b.shared.pixels > maxImagePixels {
			return
		}
		pix = image.NewRGBA(image.Rect(0, 0, iw, ih))
		draw.Draw(pix, pix.Bounds(), img, bnd.Min, draw.Src)
	} else {
		// для габаритов достаточно заданного размера
		iw, ih = int(math.Max(w, 1)), int(math.Max(h, 1))
	}
	switch {
	case w <= 0 && h <= 0:
		w, h = float64(iw), float64(ih)
	case w <= 0:
		w = h * float64(iw) / float64(ih)
	case h <= 0:
		h = w * float64(ih) / float64(iw)
	}
	par, _ := n.attr("preserveAspectRatio")
	m := viewBoxMatrix([4]float64{0, 0, float64(iw), float64(ih)}, par, x, y, w, h)

	// Область, закрашиваемая картинкой: при meet — сама картинка, при slice —
	// весь viewport (картинка вылезает за него и обрезается им).
	r := [4]float64{0, 0, float64(iw), float64(ih)}
	tr := m
	if f := strings.Fields(par); len(f) > 0 && f[len(f)-1] == "slice" {
		r = [4]float64{x, y, w, h}
		tr = Identity()
	}
	rect := rectContours(r[0], r[1], r[2], r[3], 0, 0)
	paths := applyTransform(applyTransform(rect, tr), st.transform)
	if b.noEffects {
		b.doc.Shapes = append(b.doc.Shapes, Shape{Paths: paths, HasFill: true, FillOpacity: 1})
		return
	}
	b.doc.Shapes = append(b.doc.Shapes, Shape{
		Paths:       paths,
		HasFill:     true,
		FillOpacity: st.opacity,
		Clips:       st.clips,
		Masks:       st.masks,
		BlurX:       st.blurX,
		BlurY:       st.blurY,
		ColorMatrix: st.cmat,
		Image:       &Image{Pix: pix, M: st.transform.Mul(m)},
	})
}

// imgPainter красит пиксели устройства выборкой из картинки.
type imgPainter struct {
	src        *image.RGBA // премультиплицированный, возможно уменьшенный
	d2i        Matrix      // устройство → пиксели src
	current    color.RGBA
	tint       bool
	cm         *[20]float64
	maxX, maxY int
}

// newImgPainter готовит выборку. dev2vb — устройство → viewBox.
func newImgPainter(im *Image, dev2vb Matrix, current color.RGBA, tint bool, cm *[20]float64) *imgPainter {
	if im == nil || im.Pix == nil {
		return nil
	}
	inv, ok := im.M.inverse()
	if !ok {
		return nil
	}
	d2i := inv.Mul(dev2vb)
	src := im.Pix
	// Сильное уменьшение: усредняем блоками, иначе билинейная выборка даёт
	// муар. Масштаб — сколько пикселей картинки приходится на пиксель экрана.
	scale := math.Sqrt(math.Abs(d2i.Det()))
	if k := int(scale); k >= 2 {
		src = boxDownscale(src, k)
		f := 1 / float64(k)
		d2i = ScaleM(f, f).Mul(d2i)
	}
	b := src.Bounds()
	return &imgPainter{src: src, d2i: d2i, current: current, tint: tint, cm: cm, maxX: b.Dx() - 1, maxY: b.Dy() - 1}
}

// boxDownscale уменьшает картинку в k раз усреднением блоков k×k.
func boxDownscale(src *image.RGBA, k int) *image.RGBA {
	b := src.Bounds()
	w, h := (b.Dx()+k-1)/k, (b.Dy()+k-1)/k
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, bl, a, n uint32
			for yy := y * k; yy < (y+1)*k && yy < b.Dy(); yy++ {
				off := src.PixOffset(0, yy) + x*k*4
				for xx := x * k; xx < (x+1)*k && xx < b.Dx(); xx++ {
					r += uint32(src.Pix[off])
					g += uint32(src.Pix[off+1])
					bl += uint32(src.Pix[off+2])
					a += uint32(src.Pix[off+3])
					n++
					off += 4
				}
			}
			if n == 0 {
				continue
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o] = uint8(r / n)
			dst.Pix[o+1] = uint8(g / n)
			dst.Pix[o+2] = uint8(bl / n)
			dst.Pix[o+3] = uint8(a / n)
		}
	}
	return dst
}

// at возвращает не премультиплицированный цвет пикселя устройства (x,y).
func (p *imgPainter) at(x, y int) color.RGBA {
	px, py := float64(x)+0.5, float64(y)+0.5
	u := p.d2i.A*px + p.d2i.C*py + p.d2i.E - 0.5
	v := p.d2i.B*px + p.d2i.D*py + p.d2i.F - 0.5
	x0, y0 := int(math.Floor(u)), int(math.Floor(v))
	fx, fy := u-float64(x0), v-float64(y0)
	var acc [4]float64
	for dy := 0; dy < 2; dy++ {
		wy := fy
		if dy == 0 {
			wy = 1 - fy
		}
		yy := clampInt(y0+dy, 0, p.maxY)
		for dx := 0; dx < 2; dx++ {
			wx := fx
			if dx == 0 {
				wx = 1 - fx
			}
			xx := clampInt(x0+dx, 0, p.maxX)
			o := p.src.PixOffset(xx, yy)
			wgt := wx * wy
			acc[0] += float64(p.src.Pix[o]) * wgt
			acc[1] += float64(p.src.Pix[o+1]) * wgt
			acc[2] += float64(p.src.Pix[o+2]) * wgt
			acc[3] += float64(p.src.Pix[o+3]) * wgt
		}
	}
	a := acc[3]
	if a < 0.5 {
		return color.RGBA{}
	}
	c := color.RGBA{
		R: clampByte(acc[0] * 255 / a),
		G: clampByte(acc[1] * 255 / a),
		B: clampByte(acc[2] * 255 / a),
		A: clampByte(a),
	}
	if p.tint {
		c = color.RGBA{p.current.R, p.current.G, p.current.B, uint8((uint32(c.A)*uint32(p.current.A) + 127) / 255)}
	}
	if p.cm != nil {
		c = applyColorMatrix(c, p.cm)
	}
	return c
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
