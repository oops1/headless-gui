package raster

// overrow.go — построчное смешивание Over премультиплицированных пикселей.
//
// Тень, цветной глиф и эмодзи приходят готовой картинкой R,G,B,A и ложатся на
// холст одной формулой: d = s + d·(255−sa)/255 по каждому каналу, деление с
// отбрасыванием. Под окном, которое тащат, кольцо тени — главное, что остаётся
// смешивать (пилот сцены WinLine, ENGINE_ISSUES «A window's shadow is laid over
// its whole shape»: «blend the edges with the premultiplied fast path»).
//
// Большая часть кольца — середины сторон: один пиксель шаблона, повторённый на
// всю длину стороны. Множитель (255−sa) у такой строки один, и она считается
// парами пикселей: восемь каналов разложены по 16-битным дорожкам двух слов,
// два умножения на два пикселя (overFill). Деление на 255 заменено сдвигами —
// (x + 1 + x>>8) >> 8 равно x/255 для всех x ≤ 255·255 (тест перебирает все),
// поэтому байты те же, что у цикла по каналам, включая переполнение канала у
// источника, который не премультиплицирован.
//
// Строка с разной альфой (углы и торцы тени, глифы) идёт циклом по каналам:
// раскладка по дорожкам с умножением на пиксель там выигрыша не дала (замер).

import "encoding/binary"

// overRow смешивает строку src (R,G,B,A, премультиплицированную) поверх dst.
// swap — у dst порядок B,G,R,X (FormatBGRX): R и B источника переставляются.
// Длина — по короткому из срезов.
func overRow(dst, src []byte, swap bool) {
	n := min(len(dst), len(src)) &^ 3
	dst, src = dst[:n], src[:n]
	for i := 0; i < n; i += 4 {
		s := src[i : i+4 : i+4]
		sa := uint32(s[3])
		if sa == 0 {
			continue // прозрачный пиксель холст не меняет
		}
		sr, sg, sb := s[0], s[1], s[2]
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

// overFill смешивает один и тот же пиксель px (R,G,B,A) поверх всей строки
// dst, парами пикселей. Байты те же, что у overRow с повторённым px.
func overFill(dst []byte, px []byte, swap bool) {
	s := binary.LittleEndian.Uint32(px[:4])
	if s>>24 == 0 {
		return
	}
	if swap {
		s = s&0xff00ff00 | s>>16&0xff | s&0xff<<16 // R,G,B,A → B,G,R,A
	}
	const (
		lanes = 0x00ff00ff00ff00ff
		ones  = 0x0001000100010001
	)
	inv := uint64(255 - s>>24)
	// Каналы источника по дорожкам: R и B в одном слове, G и A в другом, для
	// обоих пикселей пары.
	srb, sga := uint64(s)&0x00ff00ff, uint64(s>>8)&0x00ff00ff
	srb, sga = srb|srb<<32, sga|sga<<32
	n := len(dst) &^ 3
	i := 0
	for ; i+8 <= n; i += 8 {
		p := dst[i : i+8 : i+8]
		d := binary.LittleEndian.Uint64(p)
		rb := (d & lanes) * inv
		ga := (d >> 8 & lanes) * inv
		// Деление дорожек на 255, канал источника и отбрасывание переполнения —
		// каждое в своей дорожке: сумма не выходит за 16 бит.
		rb = ((rb+ones+(rb>>8)&lanes)>>8&lanes + srb) & lanes
		ga = ((ga+ones+(ga>>8)&lanes)>>8&lanes + sga) & lanes
		binary.LittleEndian.PutUint64(p, rb|ga<<8)
	}
	if i < n { // нечётный пиксель — те же дорожки в одном слове
		p := dst[i : i+4 : i+4]
		d := uint64(binary.LittleEndian.Uint32(p))
		rb := (d & lanes) * inv
		ga := (d >> 8 & lanes) * inv
		rb = ((rb+ones+(rb>>8)&lanes)>>8&lanes + srb) & 0x00ff00ff
		ga = ((ga+ones+(ga>>8)&lanes)>>8&lanes + sga) & 0x00ff00ff
		binary.LittleEndian.PutUint32(p, uint32(rb|ga<<8))
	}
}
