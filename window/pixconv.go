package window

import "image"

// pixconv.go — платформенно-независимые преобразования пикселей для
// презентации кадра (Win32 StretchDIBits, X11 PutImage — оба хотят BGRA).

// swapRBRow переставляет R и B в одной строке пикселей: RGBA → BGRA.
// dst и src должны быть одной длины, кратной 4 (по 4 байта на пиксель).
//
// PERF-2: вместо четырёх байтовых чтений/записей с индексной арифметикой на
// пиксель — одно 32-битное чтение, три ALU-операции и одна 32-битная запись.
// Слайсы с полной тройной формой (i:i+4:i+4) снимают проверки границ в цикле.
//
//	память RGBA (LE): v = R | G<<8 | B<<16 | A<<24
//	память BGRA (LE): u = B | G<<8 | R<<16 | A<<24
//	u = (v & 0xFF00FF00) | (v&0xFF)<<16 | (v>>16)&0xFF
func swapRBRow(dst, src []byte) {
	n := len(src)
	if len(dst) < n {
		n = len(dst)
	}
	n &^= 3
	for i := 0; i < n; i += 4 {
		s := src[i : i+4 : i+4]
		v := uint32(s[0]) | uint32(s[1])<<8 | uint32(s[2])<<16 | uint32(s[3])<<24
		u := (v & 0xFF00FF00) | (v&0x000000FF)<<16 | (v>>16)&0x000000FF
		d := dst[i : i+4 : i+4]
		d[0] = byte(u)
		d[1] = byte(u >> 8)
		d[2] = byte(u >> 16)
		d[3] = byte(u >> 24)
	}
}

// convRectBGRX конвертирует RGBA→BGRX только внутри r (координаты общие для
// src и dst). Старший байт ставится 0xFF: формат буфера XRGB8888.
func convRectBGRX(dst []byte, dstStride int, src []byte, srcStride int, r image.Rectangle) {
	if r.Empty() || dstStride <= 0 || srcStride <= 0 {
		return
	}
	rowLen := r.Dx() * 4
	for y := r.Min.Y; y < r.Max.Y; y++ {
		so := y*srcStride + r.Min.X*4
		do := y*dstStride + r.Min.X*4
		if so < 0 || do < 0 || so+rowLen > len(src) || do+rowLen > len(dst) {
			return
		}
		swapRBRow(dst[do:do+rowLen], src[so:so+rowLen])
		for x := 3; x < rowLen; x += 4 {
			dst[do+x] = 0xFF
		}
	}
}
