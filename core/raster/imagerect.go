package raster

import (
	hgdraw "github.com/oops1/headless-gui/v4/core/draw"
	"github.com/oops1/headless-gui/v4/core/geom"
	"image"
	"image/color"
	"math"
)

// DrawImageRect с выбором фильтра — масштабирование только видимой части.
//
// DrawImageScaled растягивает ВЕСЬ источник до ПОЛНОГО целевого размера и
// только потом блитит под клип: на зуме 400 % картинки 12000×8000 это
// 48000×32000 пикселей на каждый кадр (кэшировать их нельзя — больше
// бюджета), хотя окно показывает кусок в 1200×800. Здесь клип проецируется
// обратно на источник: считаются только те столбцы и строки назначения, что
// попали под клип, и только те столбцы и строки источника, что им нужны.
//
// Устройство — раздельное ядро (горизонтальный проход, затем вертикальный),
// как в golang.org/x/image/draw, и с теми же коэффициентами: при уменьшении
// ядро расширяется на масштаб, чтобы усреднялся каждый накрытый пиксель
// источника, края нормируются по сумме весов. Отличие в одном — x/image
// строит промежуточный буфер на весь dw×sh и обходит все строки источника,
// а здесь горизонтально обрабатывается каждая нужная строка источника один
// раз и только в видимых столбцах, а результаты хранятся скользящим окном из
// нескольких строк (кольцо), а не целой картинкой. Память не зависит ни от
// размера источника, ни от размера dr.

// rectScratch — переиспользуемые рабочие буферы DrawImageRect (на кадр их
// десятки обращений — не аллоцировать каждый раз).
type rectScratch struct {
	out  []byte    // результат: premultiplied RGBA видимой части
	ring []float32 // скользящее окно строк после горизонтального прохода
	row  []float32 // одна строка источника, переведённая в float32 RGBA
	acc  []float32 // накопитель вертикального прохода
	// ops — сколько умножений-сложений на канал выполнено за всё время; нужен
	// тестам, чтобы проверить, что работа пропорциональна видимой части.
	ops int64
}

// growF32 возвращает s длиной n, переиспользуя ёмкость.
func growF32(s []float32, n int) []float32 {
	if cap(s) >= n {
		return s[:n]
	}
	return make([]float32, n)
}

// axisFilter — коэффициенты передискретизации одной оси для видимого
// диапазона пикселей назначения.
type axisFilter struct {
	first  []int32   // k-й видимый пиксель назначения: индекс первого пикселя источника
	off    []int32   // и смещение его весов в w
	n      []int32   // и их число
	w      []float32 // веса (нормированы: сумма по пикселю равна 1)
	lo, hi int       // охват источника: [lo, hi)
	maxN   int       // наибольшее число весов у одного пикселя
}

// kernelOf возвращает опору и функцию ядра; support==0 — ближайший сосед.
func kernelOf(f geom.ImageFilter) (support float64, at func(t float64) float64) {
	switch f {
	case geom.FilterBilinear:
		return 1, func(t float64) float64 { return 1 - t }
	case geom.FilterCatmullRom:
		return 2, func(t float64) float64 {
			if t < 1 {
				return (1.5*t-2.5)*t*t + 1
			}
			return ((-0.5*t+2.5)*t-4)*t + 2
		}
	}
	return 0, nil
}

// buildAxis строит коэффициенты для пикселей назначения [d0, d1) оси длиной
// dn, получающей sn пикселей источника (оба диапазона относительные).
//
// Центр пикселя назначения d проецируется в источник как (d+0.5)·sn/dn − 0.5;
// это та же формула, что у golang.org/x/image/draw, поэтому картинка
// совпадает с эталонной полным масштабированием.
func buildAxis(f geom.ImageFilter, d0, d1, dn, sn int) axisFilter {
	cnt := d1 - d0
	a := axisFilter{
		first: make([]int32, cnt),
		off:   make([]int32, cnt),
		n:     make([]int32, cnt),
		lo:    sn,
	}
	scale := float64(sn) / float64(dn)
	support, at := kernelOf(f)
	if support == 0 {
		a.w = make([]float32, cnt)
		for k := 0; k < cnt; k++ {
			s := int(math.Floor((float64(d0+k) + 0.5) * scale))
			if s < 0 {
				s = 0
			}
			if s > sn-1 {
				s = sn - 1
			}
			a.first[k], a.off[k], a.n[k], a.w[k] = int32(s), int32(k), 1, 1
			a.span(s, s+1)
		}
		a.maxN = 1
		return a
	}

	half, argScale := support, 1.0
	if scale > 1 { // при уменьшении ядро расширяется: учтён каждый пиксель источника
		half *= scale
		argScale = 1 / scale
	}
	for k := 0; k < cnt; k++ {
		center := (float64(d0+k)+0.5)*scale - 0.5
		i := int(math.Floor(center - half))
		if i < 0 {
			i = 0
		}
		j := int(math.Ceil(center + half))
		if j > sn {
			j = sn
		}
		if j <= i { // центр вне источника (не бывает при sn≥1, но ось не должна падать)
			i = sn - 1
			if i < 0 {
				i = 0
			}
			j = i + 1
		}
		start := len(a.w)
		total := 0.0
		for coord := i; coord < j; coord++ {
			t := math.Abs((center - float64(coord)) * argScale)
			wt := 0.0
			if t < support {
				wt = at(t)
			}
			total += wt
			a.w = append(a.w, float32(wt))
		}
		if total == 0 { // все веса нулевые — берём ближайший
			near := int(math.Floor(center + 0.5))
			if near < i {
				near = i
			}
			if near >= j {
				near = j - 1
			}
			for q := start; q < len(a.w); q++ {
				a.w[q] = 0
			}
			a.w[start+near-i] = 1
			total = 1
		}
		inv := float32(1 / total)
		for q := start; q < len(a.w); q++ {
			a.w[q] *= inv
		}
		a.first[k], a.off[k], a.n[k] = int32(i), int32(start), int32(j-i)
		if j-i > a.maxN {
			a.maxN = j - i
		}
		a.span(i, j)
	}
	return a
}

// span расширяет охват источника до [i, j).
func (a *axisFilter) span(i, j int) {
	if i < a.lo {
		a.lo = i
	}
	if j > a.hi {
		a.hi = j
	}
}

// rowReader читает строку y источника, столбцы [x0, x1), в float32
// premultiplied RGBA со шкалой 0…255 (по четыре значения на пиксель).
type rowReader func(y, x0, x1 int, out []float32)

// newRowReader выбирает чтение под тип источника. Типичные декодированные
// форматы (RGBA, NRGBA, YCbCr, Gray) читаются прямо из памяти, остальное —
// через интерфейс image.Image.
func newRowReader(src image.Image) rowReader {
	switch s := src.(type) {
	case *image.RGBA:
		return func(y, x0, x1 int, out []float32) {
			p := s.PixOffset(x0, y)
			pix := s.Pix[p : p+(x1-x0)*4]
			for i, v := range pix {
				out[i] = float32(v)
			}
		}
	case *image.NRGBA:
		return func(y, x0, x1 int, out []float32) {
			p := s.PixOffset(x0, y)
			pix := s.Pix[p : p+(x1-x0)*4]
			for i := 0; i < len(pix); i += 4 {
				a := float32(pix[i+3])
				k := a / 255
				out[i+0] = float32(pix[i+0]) * k
				out[i+1] = float32(pix[i+1]) * k
				out[i+2] = float32(pix[i+2]) * k
				out[i+3] = a
			}
		}
	case *image.Gray:
		return func(y, x0, x1 int, out []float32) {
			p := s.PixOffset(x0, y)
			pix := s.Pix[p : p+(x1-x0)]
			for i, v := range pix {
				f := float32(v)
				out[i*4+0], out[i*4+1], out[i*4+2], out[i*4+3] = f, f, f, 255
			}
		}
	case *image.YCbCr:
		return func(y, x0, x1 int, out []float32) {
			for x := x0; x < x1; x++ {
				yi := s.YOffset(x, y)
				ci := s.COffset(x, y)
				r, g, b := color.YCbCrToRGB(s.Y[yi], s.Cb[ci], s.Cr[ci])
				i := (x - x0) * 4
				out[i+0], out[i+1], out[i+2], out[i+3] = float32(r), float32(g), float32(b), 255
			}
		}
	}
	if s, ok := src.(image.RGBA64Image); ok {
		return func(y, x0, x1 int, out []float32) {
			for x := x0; x < x1; x++ {
				c := s.RGBA64At(x, y)
				i := (x - x0) * 4
				out[i+0] = float32(c.R) / 257
				out[i+1] = float32(c.G) / 257
				out[i+2] = float32(c.B) / 257
				out[i+3] = float32(c.A) / 257
			}
		}
	}
	return func(y, x0, x1 int, out []float32) {
		for x := x0; x < x1; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			i := (x - x0) * 4
			out[i+0] = float32(r) / 257
			out[i+1] = float32(g) / 257
			out[i+2] = float32(b) / 257
			out[i+3] = float32(a) / 257
		}
	}
}

// resampleRect масштабирует прямоугольник sr источника на dr и пишет в out
// только вид vis ⊂ dr (все три — в одной системе координат назначения для dr
// и vis; sr — в координатах src). out — premultiplied RGBA размером vis.
func (s *rectScratch) resampleRect(src image.Image, sr, dr, vis image.Rectangle, f geom.ImageFilter) *image.RGBA {
	vw, vh := vis.Dx(), vis.Dy()
	need := vw * vh * 4
	if cap(s.out) < need {
		s.out = make([]byte, need)
	}
	out := &image.RGBA{Pix: s.out[:need], Stride: vw * 4, Rect: image.Rect(0, 0, vw, vh)}

	ax := buildAxis(f, vis.Min.X-dr.Min.X, vis.Max.X-dr.Min.X, dr.Dx(), sr.Dx())
	ay := buildAxis(f, vis.Min.Y-dr.Min.Y, vis.Max.Y-dr.Min.Y, dr.Dy(), sr.Dy())
	read := newRowReader(src)

	rowLen := vw * 4
	capN := ay.maxN
	s.ring = growF32(s.ring, capN*rowLen)
	s.row = growF32(s.row, (ax.hi-ax.lo)*4)
	s.acc = growF32(s.acc, rowLen)
	slotRow := make([]int, capN) // какая строка источника лежит в слоте
	for i := range slotRow {
		slotRow[i] = -1
	}

	// hRow — горизонтальный проход одной строки источника (относительный
	// номер sy) в слот кольца.
	hRow := func(sy int, dst []float32) {
		read(sr.Min.Y+sy, sr.Min.X+ax.lo, sr.Min.X+ax.hi, s.row)
		row := s.row
		for k := 0; k < vw; k++ {
			wts := ax.w[ax.off[k] : ax.off[k]+ax.n[k]]
			base := (int(ax.first[k]) - ax.lo) * 4
			var r, g, b, a float32
			for q, wt := range wts {
				p := base + q*4
				r += wt * row[p+0]
				g += wt * row[p+1]
				b += wt * row[p+2]
				a += wt * row[p+3]
			}
			o := k * 4
			dst[o+0], dst[o+1], dst[o+2], dst[o+3] = r, g, b, a
		}
		s.ops += int64(len(ax.w)) + int64(ax.hi-ax.lo)
	}

	for m := 0; m < vh; m++ {
		first, n := int(ay.first[m]), int(ay.n[m])
		wts := ay.w[ay.off[m] : ay.off[m]+int32(n)]
		acc := s.acc
		for i := range acc {
			acc[i] = 0
		}
		for q, wt := range wts {
			sy := first + q
			if wt == 0 {
				continue
			}
			slot := sy % capN
			ring := s.ring[slot*rowLen : (slot+1)*rowLen]
			if slotRow[slot] != sy {
				hRow(sy, ring)
				slotRow[slot] = sy
			}
			for i, v := range ring {
				acc[i] += wt * v
			}
		}
		s.ops += int64(n) * int64(rowLen)
		dst := out.Pix[m*out.Stride : m*out.Stride+rowLen]
		for i := 0; i < rowLen; i += 4 {
			a := roundClamp255(acc[i+3])
			dst[i+3] = a
			// Premultiplied: цвет не может превышать альфу, а ядро с
			// отрицательными лепестками (Catmull-Rom) даёт и такое.
			dst[i+0] = min(roundClamp255(acc[i+0]), a)
			dst[i+1] = min(roundClamp255(acc[i+1]), a)
			dst[i+2] = min(roundClamp255(acc[i+2]), a)
		}
	}
	return out
}

// roundClamp255 округляет значение шкалы 0…255 до байта с отсечением.
func roundClamp255(v float32) uint8 {
	i := int32(v + 0.5)
	if i <= 0 {
		return 0
	}
	if i >= 255 {
		return 255
	}
	return uint8(i)
}

// DrawImageRect рисует часть sr изображения src в прямоугольник dr
// (логические координаты) с фильтром f, масштабируя только ту часть, что
// попала под клип (см. widget.ImageRectDrawer). Учитывает прямоугольный и
// скруглённый клип, формат пикселей и HiDPI-масштаб холста. Результат не
// кэшируется: стоимость пропорциональна видимой площади.
func (c *Canvas) DrawImageRect(src image.Image, sr, dr image.Rectangle, f geom.ImageFilter) {
	if src == nil {
		return
	}
	src, opaque := hgdraw.IsOpaqueImage(src)
	sr, dr = geom.ClampImageRect(src.Bounds(), sr, dr)
	if sr.Empty() || dr.Empty() {
		return
	}
	pdr := c.PhysRect(dr)
	if pdr.Empty() {
		return
	}
	c.MarkImage(pdr)
	vis := c.clampRect(pdr).Intersect(c.back.Bounds())
	if c.round.active {
		vis = vis.Intersect(c.round.rect)
	}
	if vis.Empty() {
		return
	}
	// Масштаб 1:1 — ядро любого фильтра вырождается в тождество, пересчитывать
	// нечего: пиксели источника кладутся как есть (непрозрачный — композицией
	// Src, параллельно по полосам; см. blit_opaque.go). Для *image.RGBA это
	// побайтно то же, что дал бы пересчёт.
	if sr.Dx() == pdr.Dx() && sr.Dy() == pdr.Dy() {
		_, isRGBA := src.(*image.RGBA)
		if isRGBA || (opaque && opaqueFastType(src)) {
			c.blitImage(vis, src, sr.Min.Add(vis.Min.Sub(pdr.Min)), opaque)
			return
		}
	}
	if c.rscr == nil {
		c.rscr = &rectScratch{}
	}
	tmp := c.rscr.resampleRect(src, sr, pdr, vis, f)
	c.blitOver(vis, tmp, tmp.Rect.Min)
}
