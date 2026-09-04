// opaquebands.go — какие части картинки вынесенного оверлея закрашены.
//
// Оверлей, вынесенный в отдельное окно ОС (SetPopupSink), занимает
// ПРЯМОУГОЛЬНИК — объединение всего, что он рисует. У каскадного меню это
// объединение полосы меню, раскрытого подменю и его дочернего подменю,
// стоящего правее и ниже. Между ними остаётся площадь, не принадлежащая
// никому: её никто не закрашивает, и в картинке она прозрачна.
//
// В холсте это незаметно — сквозь прозрачное виден рабочий стол. В отдельном
// окне видно ЧЕРНОТУ: окно непрозрачно, и «ничего» показывается чёрным
// прямоугольником. Заказчик Go.Git прислал это как ошибку вывода окна
// Windows; тайлы кадра при этом верны, и он прав, что дело не в них.
//
// Здесь движок сообщает потребителю, какие части картинки он закрасил.
// Потребитель выкраивает по ним окно (на Windows — SetWindowRgn), и черноте
// неоткуда взяться: за пределами закрашенного окна попросту нет.
package engine

import "image"

// OpaqueBands возвращает закрашенные части картинки — прямоугольные полосы,
// покрывающие ровно те точки, у которых альфа не ноль.
//
// Полосами по строкам, а не одним прямоугольником: у каскадного меню
// закрашенная площадь — ступенька, и прямоугольник вокруг неё вернул бы ту же
// дыру, ради которой всё и затевалось. Соседние строки с одинаковым набором
// отрезков склеиваются, поэтому у обычного меню полос выходит единицы, а не
// по одной на строку.
//
// nil означает «выкраивать нечего»: картинки нет или в ней нет ни одной
// закрашенной точки. Сплошная картинка даёт одну полосу во весь размер —
// значит окно остаётся обычным прямоугольником.
func OpaqueBands(img *image.RGBA) []image.Rectangle {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	if b.Empty() {
		return nil
	}

	var out []image.Rectangle
	// prev — отрезки предыдущей строки и её номер: пока набор отрезков не
	// меняется, полоса растёт вниз, а не превращается в строку на каждый ряд.
	var prev []span
	prevY := b.Min.Y
	cur := make([]span, 0, 4)

	flush := func(endY int) {
		for _, s := range prev {
			out = append(out, image.Rect(s.x0, prevY, s.x1, endY))
		}
	}

	for y := b.Min.Y; y < b.Max.Y; y++ {
		cur = rowSpans(cur[:0], img, y)
		if y == b.Min.Y {
			prev = append(prev[:0], cur...)
			prevY = y
			continue
		}
		if sameSpans(prev, cur) {
			continue
		}
		flush(y)
		prev = append(prev[:0], cur...)
		prevY = y
	}
	flush(b.Max.Y)
	if len(out) == 0 {
		return nil
	}
	return out
}

// span — отрезок закрашенных точек в строке: [x0, x1).
type span struct{ x0, x1 int }

// rowSpans собирает отрезки строки y с ненулевой альфой.
func rowSpans(dst []span, img *image.RGBA, y int) []span {
	b := img.Bounds()
	off := img.PixOffset(b.Min.X, y) + 3 // +3 — байт альфы
	start := -1
	for x := b.Min.X; x < b.Max.X; x++ {
		opaque := img.Pix[off] != 0
		off += 4
		switch {
		case opaque && start < 0:
			start = x
		case !opaque && start >= 0:
			dst = append(dst, span{start, x})
			start = -1
		}
	}
	if start >= 0 {
		dst = append(dst, span{start, b.Max.X})
	}
	return dst
}

func sameSpans(a, b []span) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
