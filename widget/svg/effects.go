package svg

import (
	"image"
	"image/color"
	"math"
	"strings"
)

// ── feColorMatrix ────────────────────────────────────────────────────────────

// parseColorMatrix разбирает <feColorMatrix> в матрицу 4×5 (по строкам R,G,B,A;
// пятый столбец — сдвиг). Поддержаны type=matrix (по умолчанию), saturate,
// hueRotate и luminanceToAlpha.
func parseColorMatrix(n *xnode) *[20]float64 {
	typ := "matrix"
	if v, ok := n.attr("type"); ok {
		typ = strings.ToLower(strings.TrimSpace(v))
	}
	vals := []float64(nil)
	if v, ok := n.attr("values"); ok {
		vals = parseFloats(v)
	}
	var m [20]float64
	switch typ {
	case "matrix":
		if len(vals) != 20 {
			return nil
		}
		copy(m[:], vals)
	case "saturate":
		s := 1.0
		if len(vals) > 0 {
			s = vals[0]
		}
		m = [20]float64{
			0.213 + 0.787*s, 0.715 - 0.715*s, 0.072 - 0.072*s, 0, 0,
			0.213 - 0.213*s, 0.715 + 0.285*s, 0.072 - 0.072*s, 0, 0,
			0.213 - 0.213*s, 0.715 - 0.715*s, 0.072 + 0.928*s, 0, 0,
			0, 0, 0, 1, 0,
		}
	case "huerotate":
		a := 0.0
		if len(vals) > 0 {
			a = vals[0] * math.Pi / 180
		}
		c, s := math.Cos(a), math.Sin(a)
		m = [20]float64{
			0.213 + c*0.787 - s*0.213, 0.715 - c*0.715 - s*0.715, 0.072 - c*0.072 + s*0.928, 0, 0,
			0.213 - c*0.213 + s*0.143, 0.715 + c*0.285 + s*0.140, 0.072 - c*0.072 - s*0.283, 0, 0,
			0.213 - c*0.213 - s*0.787, 0.715 - c*0.715 + s*0.715, 0.072 + c*0.928 + s*0.072, 0, 0,
			0, 0, 0, 1, 0,
		}
	case "luminancetoalpha":
		m = [20]float64{
			0, 0, 0, 0, 0,
			0, 0, 0, 0, 0,
			0, 0, 0, 0, 0,
			0.2125, 0.7154, 0.0721, 0, 0,
		}
	default:
		return nil
	}
	return &m
}

// applyColorMatrix применяет матрицу к не премультиплицированному цвету.
func applyColorMatrix(c color.RGBA, m *[20]float64) color.RGBA {
	r, g, b, a := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255, float64(c.A)/255
	out := func(i int) uint8 {
		v := m[i*5]*r + m[i*5+1]*g + m[i*5+2]*b + m[i*5+3]*a + m[i*5+4]
		return clampByte(v * 255)
	}
	return color.RGBA{out(0), out(1), out(2), out(3)}
}

// ── покрытия ─────────────────────────────────────────────────────────────────

// mulAlpha умножает покрытие dst на src (оба одного размера) на месте.
func mulAlpha(dst, src *image.Alpha) {
	for i, s := range src.Pix {
		d := dst.Pix[i]
		if d == 0 {
			continue
		}
		if s == 0 {
			dst.Pix[i] = 0
			continue
		}
		dst.Pix[i] = uint8((uint32(d)*uint32(s) + 127) / 255)
	}
}

// unionAlpha объединяет покрытия: dst = dst + src − dst·src.
func unionAlpha(dst, src *image.Alpha) {
	for i, s := range src.Pix {
		if s == 0 {
			continue
		}
		d := uint32(dst.Pix[i])
		dst.Pix[i] = uint8(d + (uint32(s)*(255-d)+127)/255)
	}
}

// blurAlpha размывает покрытие по Гауссу (приближение тремя проходами box
// blur) с отклонениями sx, sy в пикселях. Возвращает новую маску; при
// пренебрежимо малом размытии — исходную.
func blurAlpha(m *image.Alpha, sx, sy float64) *image.Alpha {
	sx, sy = clampSigma(sx), clampSigma(sy)
	if sx < 0.3 && sy < 0.3 {
		return m
	}
	b := m.Bounds()
	w, h := b.Dx(), b.Dy()
	buf := make([]float32, w*h)
	for y := 0; y < h; y++ {
		row := m.Pix[y*m.Stride : y*m.Stride+w]
		for x, v := range row {
			buf[y*w+x] = float32(v)
		}
	}
	tmp := make([]float32, maxInt(w, h))
	line := make([]float32, maxInt(w, h))
	if sx >= 0.3 {
		for _, size := range boxSizes(sx) {
			r := (size - 1) / 2
			for y := 0; y < h; y++ {
				boxBlur1D(buf[y*w:(y+1)*w], tmp[:w], r)
			}
		}
	}
	if sy >= 0.3 {
		for _, size := range boxSizes(sy) {
			r := (size - 1) / 2
			for x := 0; x < w; x++ {
				for y := 0; y < h; y++ {
					line[y] = buf[y*w+x]
				}
				boxBlur1D(line[:h], tmp[:h], r)
				for y := 0; y < h; y++ {
					buf[y*w+x] = line[y]
				}
			}
		}
	}
	out := image.NewAlpha(m.Rect)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := buf[y*w+x] + 0.5
			if v > 255 {
				v = 255
			}
			out.Pix[y*out.Stride+x] = uint8(v)
		}
	}
	return out
}

// maxBlurSigma — предел размытия в пикселях: дальше картинка и так
// превращается в пятно, а окно box blur перестаёт помещаться в кадр.
const maxBlurSigma = 128

// clampSigma отбрасывает NaN/Inf/отрицательные значения и режет слишком большие.
func clampSigma(s float64) float64 {
	if !(s > 0) || math.IsInf(s, 0) {
		return 0
	}
	if s > maxBlurSigma {
		return maxBlurSigma
	}
	return s
}

// boxSizes — три нечётных размера окна, дающих в сумме гауссиану с
// отклонением sigma.
func boxSizes(sigma float64) [3]int {
	const n = 3
	ideal := math.Sqrt(12*sigma*sigma/n + 1)
	wl := int(math.Floor(ideal))
	if wl%2 == 0 {
		wl--
	}
	if wl < 1 {
		wl = 1
	}
	wu := wl + 2
	mi := int(math.Round((12*sigma*sigma - n*float64(wl*wl) - 4*n*float64(wl) - 3*n) / (-4*float64(wl) - 4)))
	var out [3]int
	for i := range out {
		if i < mi {
			out[i] = wl
		} else {
			out[i] = wu
		}
	}
	return out
}

// boxBlur1D размывает строку на месте окном 2r+1; за краями — нули. tmp —
// рабочий буфер той же длины.
func boxBlur1D(line, tmp []float32, r int) {
	n := len(line)
	if r <= 0 || n == 0 {
		return
	}
	copy(tmp, line)
	inv := 1 / float32(2*r+1)
	var sum float32
	for i := 0; i <= r && i < n; i++ {
		sum += tmp[i]
	}
	for i := 0; i < n; i++ {
		line[i] = sum * inv
		if add := i + r + 1; add < n {
			sum += tmp[add]
		}
		if sub := i - r; sub >= 0 {
			sum -= tmp[sub]
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
