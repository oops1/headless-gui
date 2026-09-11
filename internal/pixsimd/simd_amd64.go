//go:build goexperiment.simd && amd64

package pixsimd

import "simd/archsimd"

// Векторные версии на AVX2: восемь пикселей за шаг, по одному в 32-битной
// полосе. Раскладывать пиксель по каналам не нужно — канал c в полосе это
// (v >> 8c) & 0xFF, и собирается пиксель обратно так же, сдвигами. Поэтому
// никаких перестановок байтов: только сдвиги, умножения и сложения.
//
// Все произведения укладываются в 32 бита: наибольшее из них — 255·0x101·0xFFFF
// = 4 294 836 225 < 2³², так что достаточно младшей половины умножения (VPMULLD).
// Каждый канал перед сборкой маскируется & 0xFF: скалярный код приводит
// результат к uint8 с отбрасыванием старших бит, и у непремультиплицированного
// цвета сумма может перевалить за 255 — без маски лишний бит утёк бы в соседний
// канал.

func init() {
	if !archsimd.X86.AVX2() || disabledByEnv() {
		return
	}
	blendMaskRow = blendMaskRowAVX2
	overSolidRow = overSolidRowAVX2
	swapRB = swapRBAVX2
	swapRBOpaque = swapRBOpaqueAVX2
	impl = "avx2"
}

// div65535 — точное x/65535 без деления: (x + x>>16 + 1) >> 16. Верно для всех
// x, которые здесь возникают (перебор — в TestDiv65535Exact).
func div65535(x, one archsimd.Uint32x8) archsimd.Uint32x8 {
	return x.Add(x.ShiftAllRight(16)).Add(one).ShiftAllRight(16)
}

func blendMaskRowAVX2(dst, mask []byte, sr, sg, sb, sa uint32) {
	n := len(mask)
	if n < 8 {
		blendMaskRowGeneric(dst, mask, sr, sg, sb, sa)
		return
	}
	one := archsimd.BroadcastUint32x8(1)
	ff := archsimd.BroadcastUint32x8(0xFF)
	x101 := archsimd.BroadcastUint32x8(0x101)
	full := archsimd.BroadcastUint32x8(m16)
	vr := archsimd.BroadcastUint32x8(sr)
	vg := archsimd.BroadcastUint32x8(sg)
	vb := archsimd.BroadcastUint32x8(sb)
	va := archsimd.BroadcastUint32x8(sa)

	var tail [16]uint8
	i := 0
	for ; i+8 <= n; i += 8 {
		// Восемь байт маски. Загрузка берёт шестнадцать — читать за концом
		// строки нельзя, поэтому у хвоста байты копируются в буфер.
		var m8 archsimd.Uint8x16
		if i+16 <= n {
			m8 = archsimd.LoadUint8x16(mask[i : i+16])
		} else {
			copy(tail[:8], mask[i:i+8])
			m8 = archsimd.LoadUint8x16Array(&tail)
		}
		ma := m8.ExtendLo8ToUint32().Mul(x101) // 0..0xffff, как ma | ma<<8

		// Там, где маска нулевая, скаляр пиксель пропускает. Вектор его
		// считает — и получает тот же пиксель: a = 0, inv = 0xffff, а
		// (p·0x101·0xffff / 0xffff) >> 8 = p.
		a := div65535(va.Mul(ma), one)
		inv := full.Sub(a)

		px := archsimd.LoadUint8x32(dst[i*4 : i*4+32]).AsUint32x8()
		ch := func(shift uint64, s archsimd.Uint32x8) archsimd.Uint32x8 {
			p := px.ShiftAllRight(shift).And(ff)
			bg := div65535(p.Mul(x101).Mul(inv), one)
			fg := div65535(s.Mul(ma), one)
			return bg.Add(fg).ShiftAllRight(8).And(ff).ShiftAllLeft(shift)
		}
		out := ch(0, vr).Or(ch(8, vg)).Or(ch(16, vb)).Or(ch(24, va))
		out.AsUint8x32().Store(dst[i*4 : i*4+32])
	}
	// Гасим верхние половины регистров (VZEROUPPER). Компилятор пока не
	// ставит его сам, а без него SSE-код, идущий следом, — в том числе
	// сравнение групп в map рантайма — платит за ложную зависимость от
	// грязных YMM. Замер: без этой строки текст медленнее на 40 %, с ней —
	// быстрее скалярной сборки. См. ClearAVXUpperBits в документации archsimd.
	archsimd.ClearAVXUpperBits()
	if i < n {
		blendMaskRowGeneric(dst[i*4:], mask[i:], sr, sg, sb, sa)
	}
}

func overSolidRowAVX2(row []byte, a, sr, sg, sb, sa uint32) {
	n := len(row) &^ 31
	if n > 0 {
		one := archsimd.BroadcastUint32x8(1)
		ff := archsimd.BroadcastUint32x8(0xFF)
		va := archsimd.BroadcastUint32x8(a)
		cr := archsimd.BroadcastUint32x8(sr)
		cg := archsimd.BroadcastUint32x8(sg)
		cb := archsimd.BroadcastUint32x8(sb)
		ca := archsimd.BroadcastUint32x8(sa)
		for x := 0; x < n; x += 32 {
			px := archsimd.LoadUint8x32(row[x : x+32]).AsUint32x8()
			ch := func(shift uint64, s archsimd.Uint32x8) archsimd.Uint32x8 {
				p := px.ShiftAllRight(shift).And(ff)
				return div65535(p.Mul(va), one).Add(s).ShiftAllRight(8).And(ff).ShiftAllLeft(shift)
			}
			out := ch(0, cr).Or(ch(8, cg)).Or(ch(16, cb)).Or(ch(24, ca))
			out.AsUint8x32().Store(row[x : x+32])
		}
		archsimd.ClearAVXUpperBits()
	}
	if n < len(row) {
		overSolidRowGeneric(row[n:], a, sr, sg, sb, sa)
	}
}

func swapRBAVX2(dst, src []byte) {
	n := len(src) &^ 31
	if n > 0 {
		keep := archsimd.BroadcastUint32x8(0xFF00FF00)
		lo := archsimd.BroadcastUint32x8(0xFF)
		for i := 0; i < n; i += 32 {
			v := archsimd.LoadUint8x32(src[i : i+32]).AsUint32x8()
			u := v.And(keep).Or(v.And(lo).ShiftAllLeft(16)).Or(v.ShiftAllRight(16).And(lo))
			u.AsUint8x32().Store(dst[i : i+32])
		}
		archsimd.ClearAVXUpperBits()
	}
	if n < len(src) {
		swapRBGeneric(dst[n:], src[n:])
	}
}

func swapRBOpaqueAVX2(dst, src []byte) {
	n := len(src) &^ 31
	if n > 0 {
		mid := archsimd.BroadcastUint32x8(0x0000FF00)
		lo := archsimd.BroadcastUint32x8(0xFF)
		alpha := archsimd.BroadcastUint32x8(0xFF000000)
		for i := 0; i < n; i += 32 {
			v := archsimd.LoadUint8x32(src[i : i+32]).AsUint32x8()
			u := v.And(mid).Or(v.And(lo).ShiftAllLeft(16)).Or(v.ShiftAllRight(16).And(lo)).Or(alpha)
			u.AsUint8x32().Store(dst[i : i+32])
		}
		archsimd.ClearAVXUpperBits()
	}
	if n < len(src) {
		swapRBOpaqueGeneric(dst[n:], src[n:])
	}
}
