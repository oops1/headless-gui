package svg

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

// rasterKey — ключ кэша растеризаций (иконка неявно — сам Document).
type rasterKey struct {
	w, h    int
	r, g, b uint8
	a       uint8
	tint    bool
}

type rasterEntry struct {
	img *image.RGBA
}

// maxCacheEntries ограничивает рост кэша (иконка × размеры × цвета).
const maxCacheEntries = 48

// RasterizeCached возвращает растеризацию иконки размера w×h с подстановкой
// currentColor=current. Результат кэшируется по (размер, цвет, tint).
// tint=true перекрашивает ВЕСЬ контент в current (монохромная перекраска).
//
// Возвращаемый *image.RGBA принадлежит кэшу — вызывающий не должен его менять.
func (d *Document) RasterizeCached(w, h int, current color.RGBA, tint bool) *image.RGBA {
	if w <= 0 || h <= 0 {
		return nil
	}
	key := rasterKey{w: w, h: h, r: current.R, g: current.G, b: current.B, a: current.A, tint: tint}
	d.mu.Lock()
	if d.cache != nil {
		if e, ok := d.cache[key]; ok {
			d.mu.Unlock()
			return e.img
		}
	}
	d.mu.Unlock()

	img := d.Rasterize(w, h, current, tint)

	d.mu.Lock()
	if d.cache == nil {
		d.cache = make(map[rasterKey]*rasterEntry)
	}
	if len(d.cache) >= maxCacheEntries {
		// Простейшая политика: очистить при переполнении.
		d.cache = make(map[rasterKey]*rasterEntry)
	}
	d.cache[key] = &rasterEntry{img: img}
	d.mu.Unlock()
	return img
}

// InvalidateCache сбрасывает кэш растеризаций (например, после мутации иконки).
func (d *Document) InvalidateCache() {
	d.mu.Lock()
	d.cache = nil
	d.mu.Unlock()
}

// Rasterize растеризует иконку в новый *image.RGBA размера w×h (прозрачный
// фон), сохраняя пропорции viewBox и центрируя содержимое. current
// подставляется вместо currentColor; tint=true перекрашивает всё в current.
func (d *Document) Rasterize(w, h int, current color.RGBA, tint bool) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return dst
	}
	vbW := d.ViewBox[2]
	vbH := d.ViewBox[3]
	if vbW <= 0 || vbH <= 0 {
		return dst
	}
	// Масштаб «uniform» (вписать с сохранением пропорций) + центрирование.
	sx := float64(w) / vbW
	sy := float64(h) / vbH
	s := sx
	if sy < s {
		s = sy
	}
	tx := (float64(w)-vbW*s)/2 - d.ViewBox[0]*s
	ty := (float64(h)-vbH*s)/2 - d.ViewBox[1]*s

	mapPt := func(p Point) (float32, float32) {
		return float32(p.X*s + tx), float32(p.Y*s + ty)
	}

	// Переиспользуемые буферы.
	acc := make([]float64, w*h) // для even-odd
	for i := range d.Shapes {
		sh := &d.Shapes[i]

		if sh.HasFill {
			col := sh.Fill
			if sh.FillCurrent || tint {
				col = current
			}
			col = applyOpacity(col, sh.FillOpacity)
			if col.A > 0 {
				var mask *image.Alpha
				if sh.EvenOdd {
					mask = rasterEvenOdd(w, h, sh.Paths, mapPt, acc)
				} else {
					mask = rasterNonzero(w, h, sh.Paths, mapPt)
				}
				blit(dst, mask, col)
			}
		}

		if sh.HasStroke {
			col := sh.Stroke
			if sh.StrokeCurrent || tint {
				col = current
			}
			col = applyOpacity(col, sh.StrokeOpacity)
			if col.A > 0 {
				sw := sh.StrokeWidth * s
				if sw < 0.75 {
					sw = 0.75 // минимальная видимая толщина
				}
				mask := rasterStroke(w, h, sh.Paths, sw, mapPt)
				blit(dst, mask, col)
			}
		}
	}
	return dst
}

// rasterNonzero растеризует контуры правилом ненулевого числа оборотов.
func rasterNonzero(w, h int, contours []Contour, mapPt func(Point) (float32, float32)) *image.Alpha {
	z := vector.NewRasterizer(w, h)
	for _, c := range contours {
		if len(c.Points) < 2 {
			continue
		}
		x, y := mapPt(c.Points[0])
		z.MoveTo(x, y)
		for _, p := range c.Points[1:] {
			px, py := mapPt(p)
			z.LineTo(px, py)
		}
		z.ClosePath()
	}
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// rasterEvenOdd растеризует контуры правилом чётности. Каждый контур
// растеризуется отдельно (ненулевым правилом), затем покрытия складываются
// XOR-формулой acc = acc + a - 2*acc*a — корректное AA-объединение для
// непересекающихся по одному пикселю рёбер (типичный случай иконок-колец).
func rasterEvenOdd(w, h int, contours []Contour, mapPt func(Point) (float32, float32), acc []float64) *image.Alpha {
	for i := range acc {
		acc[i] = 0
	}
	for _, c := range contours {
		if len(c.Points) < 2 {
			continue
		}
		z := vector.NewRasterizer(w, h)
		x, y := mapPt(c.Points[0])
		z.MoveTo(x, y)
		for _, p := range c.Points[1:] {
			px, py := mapPt(p)
			z.LineTo(px, py)
		}
		z.ClosePath()
		m := image.NewAlpha(image.Rect(0, 0, w, h))
		z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
		for idx, mv := range m.Pix {
			if mv == 0 {
				continue
			}
			a := float64(mv) / 255
			acc[idx] = acc[idx] + a - 2*acc[idx]*a
		}
	}
	out := image.NewAlpha(image.Rect(0, 0, w, h))
	for i, v := range acc {
		if v <= 0 {
			continue
		}
		if v >= 1 {
			out.Pix[i] = 255
			continue
		}
		out.Pix[i] = uint8(v*255 + 0.5)
	}
	return out
}

// rasterStroke растеризует обводку контуров толщиной sw (пиксели), рисуя
// каждый сегмент прямоугольником-квадом (без сложных стыков/капов).
func rasterStroke(w, h int, contours []Contour, sw float64, mapPt func(Point) (float32, float32)) *image.Alpha {
	z := vector.NewRasterizer(w, h)
	half := float32(sw / 2)
	for _, c := range contours {
		n := len(c.Points)
		if n < 2 {
			continue
		}
		last := n - 1
		if c.Closed {
			last = n
		}
		for i := 0; i < last; i++ {
			p1 := c.Points[i]
			p2 := c.Points[(i+1)%n]
			ax, ay := mapPt(p1)
			bx, by := mapPt(p2)
			dx, dy := bx-ax, by-ay
			l := float32(math.Hypot(float64(dx), float64(dy)))
			if l == 0 {
				continue
			}
			// Перпендикуляр половинной толщины.
			px, py := -dy/l*half, dx/l*half
			z.MoveTo(ax+px, ay+py)
			z.LineTo(bx+px, by+py)
			z.LineTo(bx-px, by-py)
			z.LineTo(ax-px, ay-py)
			z.ClosePath()
		}
	}
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// applyOpacity умножает альфу цвета на opacity (0..1).
func applyOpacity(c color.RGBA, opacity float64) color.RGBA {
	if opacity >= 1 {
		return c
	}
	if opacity <= 0 {
		return color.RGBA{}
	}
	c.A = uint8(float64(c.A)*opacity + 0.5)
	return c
}

// blit накладывает маску alpha цветом col (straight RGBA) на dst (premultiplied
// RGBA) операцией Over.
func blit(dst *image.RGBA, mask *image.Alpha, col color.RGBA) {
	if mask == nil {
		return
	}
	b := dst.Bounds()
	w, hgt := b.Dx(), b.Dy()
	for y := 0; y < hgt; y++ {
		mRow := y * mask.Stride
		dOff := dst.PixOffset(0, y)
		for x := 0; x < w; x++ {
			ma := mask.Pix[mRow+x]
			if ma == 0 {
				dOff += 4
				continue
			}
			// эффективная (straight) альфа источника
			a := uint32(col.A) * uint32(ma) / 255
			if a == 0 {
				dOff += 4
				continue
			}
			sr := uint32(col.R) * a / 255
			sg := uint32(col.G) * a / 255
			sb := uint32(col.B) * a / 255
			inv := 255 - a
			p := dst.Pix[dOff : dOff+4 : dOff+4]
			p[0] = uint8(sr + uint32(p[0])*inv/255)
			p[1] = uint8(sg + uint32(p[1])*inv/255)
			p[2] = uint8(sb + uint32(p[2])*inv/255)
			p[3] = uint8(a + uint32(p[3])*inv/255)
			dOff += 4
		}
	}
}
