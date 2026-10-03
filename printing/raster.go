// raster.go — чистая геометрия и пиксели для печати на растровые устройства
// (GDI на Windows): куда на физическом листе положить страницу-картинку, когда
// повернуть, как нарезать на полосы и перевести в порядок каналов DIB. Без
// системных вызовов, поэтому проверяется тестами на любой платформе, а на
// Windows остаётся только тонкая обвязка вызовов GDI.
package printing

import (
	"image"
	"image/color"
	"math"
)

// fitRect вписывает страницу srcW×srcH в физический лист physW×physH с
// сохранением пропорций и центрированием. Возвращает прямоугольник в
// координатах физического листа (от его левого верхнего угла).
//
// Беда, которую это предотвращает: StretchDIBits на всю печатную область
// растягивает картинку под размер области независимо по осям. Если
// пользователь в диалоге выбрал другую бумагу (A4 вместо Letter), страница
// получила бы заметное искажение — круги стали бы эллипсами, текст сплющился. С
// сохранением пропорций страница просто чуть меньше листа с пустой каймой.
func fitRect(srcW, srcH, physW, physH int) image.Rectangle {
	if srcW <= 0 || srcH <= 0 || physW <= 0 || physH <= 0 {
		return image.Rectangle{}
	}
	// Сравнение перекрёстным умножением, без деления с плавающей точкой: при
	// совпадающих пропорциях страница должна занять лист ровно, без полоски в
	// один пиксель из-за погрешности округления.
	if int64(srcW)*int64(physH) == int64(srcH)*int64(physW) {
		return image.Rect(0, 0, physW, physH)
	}
	w, h := physW, physH
	if int64(srcW)*int64(physH) > int64(srcH)*int64(physW) {
		// Страница «шире» листа: упираемся в ширину.
		h = int(math.Round(float64(physW) * float64(srcH) / float64(srcW)))
	} else {
		w = int(math.Round(float64(physH) * float64(srcW) / float64(srcH)))
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	x := (physW - w) / 2
	y := (physH - h) / 2
	return image.Rect(x, y, x+w, y+h)
}

// needRotate — нужно ли повернуть страницу на 90°, чтобы она легла на лист той же
// ориентации. Это бывает, когда задание альбомное, а в диалоге/драйвере
// выбрана книжная подача (или наоборот): без поворота страница ужалась бы
// вдвое, оставив на листе узкую полосу. Квадратные страницы и листы не
// поворачиваются: у них ориентации нет.
func needRotate(srcW, srcH, physW, physH int) bool {
	if srcW == srcH || physW == physH {
		return false
	}
	return (srcW > srcH) != (physW > physH)
}

// rotate90 поворачивает картинку на 90° по часовой стрелке в новый RGBA.
func rotate90(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Точка (x, y) исходной картинки оказывается в (h-1-y, x).
			c := color.RGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.RGBA)
			dst.SetRGBA(h-1-y, x, c)
		}
	}
	return dst
}

// bandSpan — одна полоса: строки [SrcY0,SrcY1) исходной картинки ложатся в строки
// [DstY0,DstY1) устройства.
type bandSpan struct {
	SrcY0, SrcY1 int
	DstY0, DstY1 int
}

// bandSpans нарезает картинку высотой srcH на полосы по bandRows строк и
// сопоставляет каждой полосе строки устройства в диапазоне [dstY, dstY+dstH).
//
// Зачем полосы. Отправлять драйверу страницу целиком одним StretchDIBits —
// 35 МБ растра для A4 при 300 dpi: часть драйверов (особенно PostScript и
// сетевые) на таких вызовах падает или молча печатает пустую страницу. Полосы по
// несколько сотен строк надёжны везде и не требуют много памяти на стороне
// драйвера.
//
// Границы полос на устройстве считаются от одной и той же функции округления
// (граница = округление(dstY + srcRow·scale)), поэтому нижняя граница одной
// полосы равна верхней границе следующей: ни зазоров, ни наложения —
// белая полоска посреди страницы выглядела бы как брак печати.
func bandSpans(srcH, bandRows, dstY, dstH int) []bandSpan {
	if srcH <= 0 || bandRows <= 0 || dstH <= 0 {
		return nil
	}
	edge := func(srcRow int) int {
		return dstY + int(math.Round(float64(srcRow)*float64(dstH)/float64(srcH)))
	}
	var out []bandSpan
	for y := 0; y < srcH; y += bandRows {
		y1 := y + bandRows
		if y1 > srcH {
			y1 = srcH
		}
		s := bandSpan{SrcY0: y, SrcY1: y1, DstY0: edge(y), DstY1: edge(y1)}
		if s.DstY1 <= s.DstY0 {
			// Полоса уместилась в ноль строк (сильное уменьшение): для
			// StretchDIBits нулевая высота — ошибка, и такую полосу лучше
			// пропустить, ничего не потеряв: её строки отдал соседям округление.
			continue
		}
		out = append(out, s)
	}
	return out
}

// fillBGRX записывает строки [y0,y1) картинки в порядке канала DIB (B, G, R, 0),
// смешивая с белым: прозрачного на бумаге нет. dst должен вмещать 4·w·(y1-y0)
// байт. Для *image.RGBA — быстрый путь по срезам; остальное — через At.
func fillBGRX(dst []byte, img image.Image, y0, y1 int) {
	b := img.Bounds()
	w := b.Dx()
	if m, ok := img.(*image.RGBA); ok {
		for y := y0; y < y1; y++ {
			src := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):]
			o := 4 * w * (y - y0)
			for x := 0; x < w; x++ {
				na := 255 - int(src[4*x+3])
				dst[o+4*x] = clamp255(int(src[4*x+2]) + na)
				dst[o+4*x+1] = clamp255(int(src[4*x+1]) + na)
				dst[o+4*x+2] = clamp255(int(src[4*x]) + na)
				dst[o+4*x+3] = 0
			}
		}
		return
	}
	for y := y0; y < y1; y++ {
		o := 4 * w * (y - y0)
		for x := 0; x < w; x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			na := 0xffff - a
			dst[o+4*x] = clamp255(int((bl + na) >> 8))
			dst[o+4*x+1] = clamp255(int((g + na) >> 8))
			dst[o+4*x+2] = clamp255(int((r + na) >> 8))
			dst[o+4*x+3] = 0
		}
	}
}

func clamp255(v int) byte {
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return byte(v)
}
