package window

import (
	"image"

	"github.com/oops1/headless-gui/v3/internal/pixsimd"
)

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
	// Перестановка — в pixsimd: там та же формула, а при сборке с
	// GOEXPERIMENT=simd на процессоре с AVX2 — восемь пикселей за шаг. Вывод
	// кадра 1080p в окно ОС это миллисекунда на каждом кадре.
	pixsimd.SwapRB(dst, src)
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
		// Перестановка и непрозрачная «альфа» — одним проходом, а не вторым
		// циклом по той же строке.
		pixsimd.SwapRBOpaque(dst[do:do+rowLen], src[so:so+rowLen])
	}
}
