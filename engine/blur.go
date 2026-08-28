// blur.go — box-размытие изображения. Основа для будущих эффектов
// acrylic/mica (размытая подложка панелей) и мягких теней.
//
// Алгоритм: разделяемое (separable) box-размытие — отдельно горизонтальный
// и вертикальный проход. Несколько таких проходов подряд быстро сходятся
// к гауссову размытию (уже 2-3 прохода дают визуально гладкий результат,
// в отличие от одного box-прохода с его характерными "плоскими" краями).
//
// Каждый проход считается через скользящее окно (running sum): сумма для
// следующего пикселя получается из суммы предыдущего добавлением
// входящего элемента и вычитанием выходящего. Стоимость одного прохода —
// O(ширина*высота) и НЕ зависит от радиуса — в отличие от наивной
// реализации с вложенным циклом по радиусу, которая была бы O(w*h*radius)
// и неприемлема при больших радиусах.
//
// Цвет image.RGBA хранится premultiplied-альфой (см. пакет image/color:
// значения R,G,B уже умножены на A/255). Для premultiplied-цвета усреднение
// всех четырёх каналов одинаковым образом корректно: среднее premultiplied
// цветов равно premultiplied-представлению среднего straight-цвета с
// соответствующей средней альфой. Для straight-альфы это было бы неверно
// (пришлось бы отдельно взвешивать по альфе) — поэтому усреднять "в лоб"
// можно только благодаря premultiplied-модели Go.
//
// Края: координаты, выходящие за границы области размытия, зажимаются
// (clamp to edge) — крайний пиксель размножается наружу. Это не даёт
// затемнения/тёмной каймы у краёв, которое возникло бы при трактовке
// пикселей за границей как прозрачных (premultiplied (0,0,0,0) утянул бы
// среднее к чёрному) или при заворачивании (wrap), которое подмешивало бы
// пиксели с противоположного края области.
package engine

import "image"

// BlurRGBA размывает изображение целиком, на месте.
//
// radius — радиус box-ядра в пикселях (окно шириной 2*radius+1).
// passes — число последовательных горизонтальный+вертикальный проходов
// (2-3 прохода дают приближение гауссова размытия).
//
// radius <= 0 или passes <= 0 — no-op, изображение не меняется.
func BlurRGBA(img *image.RGBA, radius, passes int) {
	if img == nil {
		return
	}
	BlurRegion(img, img.Bounds(), radius, passes)
}

// BlurRegion размывает прямоугольную область изображения r, на месте.
// Пиксели за пределами r не читаются и не изменяются — при вычислении
// суммы для крайних пикселей области координаты зажимаются к границам r
// (а не к границам всего img и не за пределы изображения).
//
// r пересекается с границами img; если пересечение пусто, radius <= 0 или
// passes <= 0 — no-op.
func BlurRegion(img *image.RGBA, r image.Rectangle, radius, passes int) {
	if img == nil || radius <= 0 || passes <= 0 {
		return
	}
	r = r.Intersect(img.Bounds())
	w := r.Dx()
	h := r.Dy()
	if w <= 0 || h <= 0 {
		return
	}

	// Общий временный буфер под одну строку/столбец каналов — переиспользуется
	// на всех проходах и в обоих направлениях, чтобы не аллоцировать на
	// каждый вызов boxBlur*.
	maxLen := w
	if h > maxLen {
		maxLen = h
	}
	tmp := make([]uint8, maxLen*4)

	for p := 0; p < passes; p++ {
		boxBlurHorizontal(img, r, radius, tmp)
		boxBlurVertical(img, r, radius, tmp)
	}
}

// boxBlurHorizontal размывает каждую строку области r по горизонтали
// скользящим окном шириной 2*radius+1, с зажимом координат к границам r.
// tmp — переиспользуемый буфер вместимостью не меньше r.Dx()*4 байт.
func boxBlurHorizontal(img *image.RGBA, r image.Rectangle, radius int, tmp []uint8) {
	w := r.Dx()
	row := tmp[:w*4]
	winLen := 2*radius + 1

	for y := r.Min.Y; y < r.Max.Y; y++ {
		off := img.PixOffset(r.Min.X, y)
		stride := 4

		// Начальная сумма окна для x = r.Min.X: индексы от -radius до +radius
		// относительно первого пикселя строки, зажатые к [0, w-1].
		var sumR, sumG, sumB, sumA uint32
		for k := -radius; k <= radius; k++ {
			xi := clampInt(k, 0, w-1)
			p := off + xi*stride
			sumR += uint32(img.Pix[p])
			sumG += uint32(img.Pix[p+1])
			sumB += uint32(img.Pix[p+2])
			sumA += uint32(img.Pix[p+3])
		}

		for x := 0; x < w; x++ {
			ti := x * 4
			row[ti] = uint8(sumR / uint32(winLen))
			row[ti+1] = uint8(sumG / uint32(winLen))
			row[ti+2] = uint8(sumB / uint32(winLen))
			row[ti+3] = uint8(sumA / uint32(winLen))

			// Сдвигаем окно на один пиксель вправо: убираем выходящий
			// (левый) элемент, добавляем входящий (правый), оба зажаты.
			outX := clampInt(x-radius, 0, w-1)
			inX := clampInt(x+radius+1, 0, w-1)
			po := off + outX*stride
			pi := off + inX*stride
			sumR += uint32(img.Pix[pi]) - uint32(img.Pix[po])
			sumG += uint32(img.Pix[pi+1]) - uint32(img.Pix[po+1])
			sumB += uint32(img.Pix[pi+2]) - uint32(img.Pix[po+2])
			sumA += uint32(img.Pix[pi+3]) - uint32(img.Pix[po+3])
		}

		// Записываем размытую строку обратно в изображение.
		for x := 0; x < w; x++ {
			p := off + x*stride
			ti := x * 4
			img.Pix[p] = row[ti]
			img.Pix[p+1] = row[ti+1]
			img.Pix[p+2] = row[ti+2]
			img.Pix[p+3] = row[ti+3]
		}
	}
}

// boxBlurVertical размывает каждый столбец области r по вертикали —
// зеркало boxBlurHorizontal со сменой осей. tmp — переиспользуемый буфер
// вместимостью не меньше r.Dy()*4 байт.
func boxBlurVertical(img *image.RGBA, r image.Rectangle, radius int, tmp []uint8) {
	h := r.Dy()
	col := tmp[:h*4]
	winLen := 2*radius + 1
	stride := img.Stride

	for x := r.Min.X; x < r.Max.X; x++ {
		off := img.PixOffset(x, r.Min.Y)

		var sumR, sumG, sumB, sumA uint32
		for k := -radius; k <= radius; k++ {
			yi := clampInt(k, 0, h-1)
			p := off + yi*stride
			sumR += uint32(img.Pix[p])
			sumG += uint32(img.Pix[p+1])
			sumB += uint32(img.Pix[p+2])
			sumA += uint32(img.Pix[p+3])
		}

		for y := 0; y < h; y++ {
			ti := y * 4
			col[ti] = uint8(sumR / uint32(winLen))
			col[ti+1] = uint8(sumG / uint32(winLen))
			col[ti+2] = uint8(sumB / uint32(winLen))
			col[ti+3] = uint8(sumA / uint32(winLen))

			outY := clampInt(y-radius, 0, h-1)
			inY := clampInt(y+radius+1, 0, h-1)
			po := off + outY*stride
			pi := off + inY*stride
			sumR += uint32(img.Pix[pi]) - uint32(img.Pix[po])
			sumG += uint32(img.Pix[pi+1]) - uint32(img.Pix[po+1])
			sumB += uint32(img.Pix[pi+2]) - uint32(img.Pix[po+2])
			sumA += uint32(img.Pix[pi+3]) - uint32(img.Pix[po+3])
		}

		for y := 0; y < h; y++ {
			p := off + y*stride
			ti := y * 4
			img.Pix[p] = col[ti]
			img.Pix[p+1] = col[ti+1]
			img.Pix[p+2] = col[ti+2]
			img.Pix[p+3] = col[ti+3]
		}
	}
}

// clampInt зажимает v к диапазону [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
