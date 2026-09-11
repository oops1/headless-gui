//go:build goexperiment.simd && amd64

package pixsimd

import "simd/archsimd"

// Размытие на AVX2: восемь пикселей за шаг, по одному в 32-битной полосе,
// каждый канал — своя сумма в своём векторе.
//
// Вертикальный проход векторизуется сам собой: соседние пиксели строки — это
// восемь независимых столбцов, и окно для них сдвигается одновременно.
// Горизонтальный так не сделать — соседние пиксели строки лежат в ОДНОМ окне.
// Поэтому полоса из восьми строк транспонируется блоками 8×8 в буфер, где
// пиксель x всех восьми строк — одна 32-байтная «строка», по ней идёт то же
// окно, что у вертикального прохода, и результат транспонируется обратно.
// Транспонирование — 24 перестановки на 64 пикселя, окно — около тридцати
// операций на восемь: оба заметно дешевле скаляра с его четырьмя делениями и
// зажимами на каждый пиксель.
//
// Деление суммы на ширину окна d — умножение на m = ⌈2²⁴/d⌉ и сдвиг на 24 в
// 32-битной полосе (VPMULLD даёт младшие 32 бита произведения, а сумма ·m
// укладывается в них: 255·d·⌈2²⁴/d⌉ < 255·2²⁴ + 255·d < 2³²). Точно при
// d ≤ 256 — то же рассуждение, что у divider, с 2²⁴ вместо 2⁴⁰: 255·d·(d−1)
// < 2²⁴. Перебор — TestRecip24Exact. Шире окно (радиус от 128) — скалярный
// путь целиком.

func load8(b []byte, off int) archsimd.Uint32x8 {
	return archsimd.LoadUint8x32(b[off : off+32]).AsUint32x8()
}

func store8(v archsimd.Uint32x8, b []byte, off int) {
	v.AsUint8x32().Store(b[off : off+32])
}

// px4 копирует один пиксель. Не copy: memmove рантайма на коротких длинах
// берёт SSE-регистры, а посреди векторного участка верхние половины YMM ещё
// грязные — это тот самый штраф перехода, из-за которого без VZEROUPPER текст
// рисовался на 40 % медленнее.
func px4(dst, src []byte) {
	d, p := dst[:4:4], src[:4:4]
	d[0], d[1], d[2], d[3] = p[0], p[1], p[2], p[3]
}

// slide8 — окно по n «строкам» src по 32 байта (восемь пикселей). Результат
// строки i пишется в dst[i*dstStep:]. src и dst не пересекаются: окно читает
// исходные пиксели и после того, как соседние уже посчитаны.
func slide8(src []byte, n, radius int, m uint32, dst []byte, dstStep int) {
	ff := archsimd.BroadcastUint32x8(0xFF)
	hi := archsimd.BroadcastUint32x8(0xFF000000)
	vm := archsimd.BroadcastUint32x8(m)
	var sR, sG, sB, sA archsimd.Uint32x8
	for k := -radius; k <= radius; k++ {
		v := load8(src, clampInt(k, 0, n-1)*32)
		sR = sR.Add(v.And(ff))
		sG = sG.Add(v.ShiftAllRight(8).And(ff))
		sB = sB.Add(v.ShiftAllRight(16).And(ff))
		sA = sA.Add(v.ShiftAllRight(24))
	}
	for x := 0; x < n; x++ {
		// Частное не больше 255, поэтому (s·m >> 24) << 24 — это просто
		// s·m & 0xFF000000: у альфы сдвиги не нужны.
		out := sR.Mul(vm).ShiftAllRight(24).
			Or(sG.Mul(vm).ShiftAllRight(24).ShiftAllLeft(8)).
			Or(sB.Mul(vm).ShiftAllRight(24).ShiftAllLeft(16)).
			Or(sA.Mul(vm).And(hi))
		store8(out, dst, x*dstStep)

		vi := load8(src, clampInt(x+radius+1, 0, n-1)*32)
		vo := load8(src, clampInt(x-radius, 0, n-1)*32)
		sR = sR.Add(vi.And(ff)).Sub(vo.And(ff))
		sG = sG.Add(vi.ShiftAllRight(8).And(ff)).Sub(vo.ShiftAllRight(8).And(ff))
		sB = sB.Add(vi.ShiftAllRight(16).And(ff)).Sub(vo.ShiftAllRight(16).And(ff))
		sA = sA.Add(vi.ShiftAllRight(24)).Sub(vo.ShiftAllRight(24))
	}
}

// transpose8 транспонирует матрицу 8×8 32-битных элементов: на входе строки,
// на выходе столбцы. Классическая схема AVX2 — перемежение 32-битных, затем
// 64-битных элементов внутри 128-битных половин и обмен половинами.
func transpose8(a0, a1, a2, a3, a4, a5, a6, a7 archsimd.Uint32x8) (c0, c1, c2, c3, c4, c5, c6, c7 archsimd.Uint32x8) {
	t0 := a0.InterleaveLoGrouped(a1).AsUint64x4() // a0[0] a1[0] a0[1] a1[1] | a0[4] a1[4] a0[5] a1[5]
	t1 := a0.InterleaveHiGrouped(a1).AsUint64x4() // a0[2] a1[2] a0[3] a1[3] | a0[6] a1[6] a0[7] a1[7]
	t2 := a2.InterleaveLoGrouped(a3).AsUint64x4()
	t3 := a2.InterleaveHiGrouped(a3).AsUint64x4()
	t4 := a4.InterleaveLoGrouped(a5).AsUint64x4()
	t5 := a4.InterleaveHiGrouped(a5).AsUint64x4()
	t6 := a6.InterleaveLoGrouped(a7).AsUint64x4()
	t7 := a6.InterleaveHiGrouped(a7).AsUint64x4()

	u0 := t0.InterleaveLoGrouped(t2).AsUint32x8() // строки 0..3, столбцы 0 | 4
	u1 := t0.InterleaveHiGrouped(t2).AsUint32x8() // 1 | 5
	u2 := t1.InterleaveLoGrouped(t3).AsUint32x8() // 2 | 6
	u3 := t1.InterleaveHiGrouped(t3).AsUint32x8() // 3 | 7
	u4 := t4.InterleaveLoGrouped(t6).AsUint32x8() // строки 4..7
	u5 := t4.InterleaveHiGrouped(t6).AsUint32x8()
	u6 := t5.InterleaveLoGrouped(t7).AsUint32x8()
	u7 := t5.InterleaveHiGrouped(t7).AsUint32x8()

	// Младшая половина u — столбец j, старшая — j+4; собираем столбец из
	// половин строк 0..3 и 4..7.
	c0 = u0.ConcatPermute128Scalars(0, 2, u4)
	c4 = u0.ConcatPermute128Scalars(1, 3, u4)
	c1 = u1.ConcatPermute128Scalars(0, 2, u5)
	c5 = u1.ConcatPermute128Scalars(1, 3, u5)
	c2 = u2.ConcatPermute128Scalars(0, 2, u6)
	c6 = u2.ConcatPermute128Scalars(1, 3, u6)
	c3 = u3.ConcatPermute128Scalars(0, 2, u7)
	c7 = u3.ConcatPermute128Scalars(1, 3, u7)
	return
}

func boxBlurColsAVX2(bb *BlurBuf, pix []byte, stride, w, h, radius int) {
	d := 2*radius + 1
	w8 := w &^ 7
	if d > maxVecWin || w8 == 0 {
		boxBlurColsGeneric(bb, pix, stride, w, h, radius)
		return
	}
	t := bb.get(h * 32)
	m := recip24(d)
	for x0 := 0; x0 < w8; x0 += 8 {
		// Полоса из восьми столбцов — в буфер: окно читает исходные
		// пиксели, а результат пишется прямо в кадр.
		for y := 0; y < h; y++ {
			store8(load8(pix, y*stride+x0*4), t, y*32)
		}
		slide8(t, h, radius, m, pix[x0*4:], stride)
	}
	// VZEROUPPER перед скалярным кодом — см. blendMaskRowAVX2.
	archsimd.ClearAVXUpperBits()
	if w8 < w {
		boxBlurColsGeneric(bb, pix[w8*4:], stride, w-w8, h, radius)
	}
}

func boxBlurRowsAVX2(bb *BlurBuf, pix []byte, stride, w, h, radius int) {
	d := 2*radius + 1
	h8 := h &^ 7
	if d > maxVecWin || h8 == 0 {
		boxBlurRowsGeneric(bb, pix, stride, w, h, radius)
		return
	}
	buf := bb.get(w * 64)
	t, o := buf[:w*32], buf[w*32:]
	m := recip24(d)
	w8 := w &^ 7
	for y0 := 0; y0 < h8; y0 += 8 {
		s := pix[y0*stride:]
		r0, r1, r2, r3 := 0, stride, 2*stride, 3*stride
		r4, r5, r6, r7 := 4*stride, 5*stride, 6*stride, 7*stride

		// Полоса → t: пиксель x строк y0..y0+7 — строка t[x*32:].
		for x := 0; x < w8; x += 8 {
			c := x * 4
			c0, c1, c2, c3, c4, c5, c6, c7 := transpose8(
				load8(s, r0+c), load8(s, r1+c), load8(s, r2+c), load8(s, r3+c),
				load8(s, r4+c), load8(s, r5+c), load8(s, r6+c), load8(s, r7+c))
			b := x * 32
			store8(c0, t, b)
			store8(c1, t, b+32)
			store8(c2, t, b+64)
			store8(c3, t, b+96)
			store8(c4, t, b+128)
			store8(c5, t, b+160)
			store8(c6, t, b+192)
			store8(c7, t, b+224)
		}
		for x := w8; x < w; x++ {
			for i := 0; i < 8; i++ {
				px4(t[x*32+i*4:], s[i*stride+x*4:])
			}
		}

		slide8(t, w, radius, m, o, 32)

		// И обратно: столбцы o — в строки кадра.
		for x := 0; x < w8; x += 8 {
			b := x * 32
			c0, c1, c2, c3, c4, c5, c6, c7 := transpose8(
				load8(o, b), load8(o, b+32), load8(o, b+64), load8(o, b+96),
				load8(o, b+128), load8(o, b+160), load8(o, b+192), load8(o, b+224))
			c := x * 4
			store8(c0, s, r0+c)
			store8(c1, s, r1+c)
			store8(c2, s, r2+c)
			store8(c3, s, r3+c)
			store8(c4, s, r4+c)
			store8(c5, s, r5+c)
			store8(c6, s, r6+c)
			store8(c7, s, r7+c)
		}
		for x := w8; x < w; x++ {
			for i := 0; i < 8; i++ {
				px4(s[i*stride+x*4:], o[x*32+i*4:])
			}
		}
	}
	archsimd.ClearAVXUpperBits()
	if h8 < h {
		boxBlurRowsGeneric(bb, pix[h8*stride:], stride, w, h-h8, radius)
	}
}
