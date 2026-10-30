package native

import (
	"image"

	"github.com/oops1/headless-gui/v4/internal/pixsimd"
)

// pixconv.go — платформенно-независимые преобразования пикселей для
// презентации кадра (Win32 StretchDIBits, X11 PutImage — оба хотят BGRA).

// SwapRBRow переставляет R и B в одной строке пикселей: RGBA → BGRA.
// dst и src должны быть одной длины, кратной 4 (по 4 байта на пиксель).
//
// PERF-2: вместо четырёх байтовых чтений/записей с индексной арифметикой на
// пиксель — одно 32-битное чтение, три ALU-операции и одна 32-битная запись.
// Слайсы с полной тройной формой (i:i+4:i+4) снимают проверки границ в цикле.
//
//	память RGBA (LE): v = R | G<<8 | B<<16 | A<<24
//	память BGRA (LE): u = B | G<<8 | R<<16 | A<<24
//	u = (v & 0xFF00FF00) | (v&0xFF)<<16 | (v>>16)&0xFF
//
//facadegen:skip
func SwapRBRow(dst, src []byte) {
	// Перестановка — в pixsimd: там та же формула, а при сборке с
	// GOEXPERIMENT=simd на процессоре с AVX2 — восемь пикселей за шаг. Вывод
	// кадра 1080p в окно ОС это миллисекунда на каждом кадре.
	pixsimd.SwapRB(dst, src)
}

// ConvRectBGRX конвертирует RGBA→BGRX только внутри r (координаты общие для
// src и dst). Старший байт ставится 0xFF: формат буфера XRGB8888.
//
//facadegen:skip
func ConvRectBGRX(dst []byte, dstStride int, src []byte, srcStride int, r image.Rectangle) {
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

// ConvRectBGRAPremul конвертирует RGBA→BGRA внутри r для буфера с альфой,
// который компоновщик смешивает как premultiplied (Wayland ARGB8888, окно
// X11 глубины 32).
//
// Картинка движка УЖЕ premultiplied (image.RGBA по определению Go, весь back
// холста): сглаженный край белого меню — (28,28,28,28). Прежняя версия
// умножала цвет на альфу ещё раз и давала (3,3,3,28) — тёмную кайму по
// скруглённым углам. Но и просто переставить каналы мало: точка, в которой
// цвет больше альфы (straight-цвет, переданный как premultiplied, — светлый
// ореол у углов, ради которого умножение когда-то и добавили), у компоновщика
// становится пересветом. Поэтому каждый канал обрезается альфой: корректная
// premultiplied-точка остаётся как есть, а straight-белый край (255,255,255,28)
// становится ровно правильным (28,28,28,28).
//
//facadegen:skip
func ConvRectBGRAPremul(dst []byte, dstStride int, src []byte, srcStride int, r image.Rectangle) {
	if r.Empty() || dstStride <= 0 || srcStride <= 0 {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		so := y*srcStride + r.Min.X*4
		do := y*dstStride + r.Min.X*4
		if so < 0 || do < 0 || so+r.Dx()*4 > len(src) || do+r.Dx()*4 > len(dst) {
			return
		}
		for x := 0; x < r.Dx(); x++ {
			s := src[so+x*4 : so+x*4+4 : so+x*4+4]
			d := dst[do+x*4 : do+x*4+4 : do+x*4+4]
			a := uint32(s[3])
			switch a {
			case 255:
				d[0], d[1], d[2], d[3] = s[2], s[1], s[0], 255
			case 0:
				d[0], d[1], d[2], d[3] = 0, 0, 0, 0
			default:
				d[0], d[1], d[2], d[3] = minByte(s[2], s[3]), minByte(s[1], s[3]), minByte(s[0], s[3]), s[3]
			}
		}
	}
}

// SwapRBRowPremul — строка RGBA → BGRA для буфера с альфой: перестановка и
// обрезка канала альфой, как у ConvRectBGRAPremul.
//
//facadegen:skip
func SwapRBRowPremul(dst, src []byte) {
	n := len(src) &^ 3
	for i := 0; i < n; i += 4 {
		a := src[i+3]
		if a == 255 {
			dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i+2], src[i+1], src[i], 255
			continue
		}
		dst[i], dst[i+1], dst[i+2], dst[i+3] = minByte(src[i+2], a), minByte(src[i+1], a), minByte(src[i], a), a
	}
}

func minByte(v, lim byte) byte {
	if v > lim {
		return lim
	}
	return v
}
