package pixsimd

// Box-размытие: проход скользящим окном шириной 2·radius+1 по строкам или по
// столбцам прямоугольной области, с зажимом координат к её краям. Перенесено
// из движка (engine/blur.go): там описано, почему усреднять premultiplied-цвет
// можно всеми каналами одинаково и зачем зажим, а не прозрачная кайма.
//
// Скалярная версия — прежний алгоритм, но деление суммы окна на его ширину
// заменено умножением на обратное: делитель тут переменный, и компилятор
// заменить деление сам не может — четыре деления на пиксель съедали больше
// половины прохода. Замена точная (см. divider), результат тот же бит в бит.

// BlurBuf — рабочие буферы размытия. Нулевое значение готово к работе; один
// BlurBuf переиспользуется всеми проходами одного размытия, чтобы не выделять
// память на каждый.
type BlurBuf struct{ b []byte }

func (bb *BlurBuf) get(n int) []byte {
	if cap(bb.b) < n {
		bb.b = make([]byte, n)
	}
	return bb.b[:n]
}

// BoxBlurRows — горизонтальный проход по области w×h. Строка y области —
// pix[y*stride : y*stride+w*4], пиксели RGBA. Пиксели вне области не читаются
// и не пишутся. radius ≤ 0 или пустая область — ничего не делает.
func BoxBlurRows(bb *BlurBuf, pix []byte, stride, w, h, radius int) {
	if !blurArgsOK(pix, stride, w, h, radius) {
		return
	}
	active.blurRows(bb, pix, stride, w, h, radius)
}

// BoxBlurCols — вертикальный проход по той же области.
func BoxBlurCols(bb *BlurBuf, pix []byte, stride, w, h, radius int) {
	if !blurArgsOK(pix, stride, w, h, radius) {
		return
	}
	active.blurCols(bb, pix, stride, w, h, radius)
}

func blurArgsOK(pix []byte, stride, w, h, radius int) bool {
	if radius <= 0 || w <= 0 || h <= 0 || stride < w*4 {
		return false
	}
	if len(pix) < (h-1)*stride+w*4 {
		panic("pixsimd: область размытия выходит за буфер")
	}
	return true
}

// divider — точное деление суммы окна на его ширину d умножением:
// n/d = n·m >> 40, m = ⌈2⁴⁰/d⌉.
//
// Почему точно: n·m/2⁴⁰ = n/d + n·e/(d·2⁴⁰), где e = m·d − 2⁴⁰ < d. Дробная
// часть n/d не больше (d−1)/d, поэтому целая часть не меняется, пока
// n·e < 2⁴⁰. Сумма окна не больше 255·d, и условие выполнено при
// 255·d² < 2⁴⁰, то есть при d < 65 664. Шире окна (радиус от 32 768) — обычное
// деление. Перебор — TestDividerExact.
type divider struct {
	d uint32
	m uint64 // 0 — делить честно
}

func newDivider(d int) divider {
	q := divider{d: uint32(d)}
	if d < 1<<16 {
		q.m = (1<<40 + uint64(d) - 1) / uint64(d)
	}
	return q
}

func (q divider) div(s uint32) uint8 {
	if q.m == 0 {
		return uint8(s / q.d)
	}
	return uint8(uint64(s) * q.m >> 40)
}

// maxVecWin и recip24 — деление в векторном пути (blur_amd64.go): умножение
// на ⌈2²⁴/d⌉ и сдвиг на 24 в 32-битной полосе, точное при d ≤ 256. Здесь, а
// не рядом с векторным кодом, — чтобы перебор TestRecip24Exact шёл в любой
// сборке.
const maxVecWin = 256

func recip24(d int) uint32 { return uint32((1<<24 + d - 1) / d) }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// blurRun размывает n пикселей p[0], p[step], p[2·step], … на месте. out —
// буфер не меньше n·4 байт: окно читает исходные пиксели, и писать результат
// прямо в p нельзя, пока окно их не прошло.
func blurRun(p []byte, step, n, radius int, q divider, out []byte) {
	var sumR, sumG, sumB, sumA uint32
	for k := -radius; k <= radius; k++ {
		i := clampInt(k, 0, n-1) * step
		sumR += uint32(p[i])
		sumG += uint32(p[i+1])
		sumB += uint32(p[i+2])
		sumA += uint32(p[i+3])
	}
	for x := 0; x < n; x++ {
		o := out[x*4 : x*4+4 : x*4+4]
		o[0] = q.div(sumR)
		o[1] = q.div(sumG)
		o[2] = q.div(sumB)
		o[3] = q.div(sumA)

		// Окно сдвигается на пиксель: выходящий убирается, входящий
		// добавляется, оба зажаты к краям.
		po := clampInt(x-radius, 0, n-1) * step
		pi := clampInt(x+radius+1, 0, n-1) * step
		sumR += uint32(p[pi]) - uint32(p[po])
		sumG += uint32(p[pi+1]) - uint32(p[po+1])
		sumB += uint32(p[pi+2]) - uint32(p[po+2])
		sumA += uint32(p[pi+3]) - uint32(p[po+3])
	}
	for x := 0; x < n; x++ {
		d := p[x*step : x*step+4 : x*step+4]
		o := out[x*4 : x*4+4 : x*4+4]
		d[0], d[1], d[2], d[3] = o[0], o[1], o[2], o[3]
	}
}

func boxBlurRowsGeneric(bb *BlurBuf, pix []byte, stride, w, h, radius int) {
	out := bb.get(w * 4)
	q := newDivider(2*radius + 1)
	for y := 0; y < h; y++ {
		blurRun(pix[y*stride:], 4, w, radius, q, out)
	}
}

func boxBlurColsGeneric(bb *BlurBuf, pix []byte, stride, w, h, radius int) {
	out := bb.get(h * 4)
	q := newDivider(2*radius + 1)
	for x := 0; x < w; x++ {
		blurRun(pix[x*4:], stride, h, radius, q, out)
	}
}
