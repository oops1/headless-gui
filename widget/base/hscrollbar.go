package base

import (
	"image"
	"image/color"
	"math"
)

// hscrollbar.go — горизонтальная полоса прокрутки сравнения и слияния.
//
// Смещать код вбок оба виджета умели и раньше, но задать это смещение можно
// было единственным способом — горизонтальным колесом или жестом тачпада. На
// обычной мыши правая часть длинной строки оказывалась недоступна: видно, что
// строка обрезана, а добраться до её конца нечем.
//
// Геометрия вынесена сюда отдельными функциями: она одна на оба виджета и
// проверяется тестами без отрисовки.

// Размеры полосы. HBarHeight и HBarInsetX — служебные: для подпакетов
// widget, в фасад не входят.
const (
	//facadegen:skip
	HBarHeight = 10 // высота полосы вместе с полями
	dvHBarMinW = 24 // ползунок короче этого не ужимается — за него не схватиться
	//facadegen:skip
	HBarInsetX = 4 // запас хита слева и справа от трека
)

// HBarThumb — ползунок на треке track: его ширина пропорциональна тому, какая
// доля содержимого видна, а положение — текущему смещению.
//
// Пустой прямоугольник означает «полоса не нужна»: содержимое помещается
// целиком. Так же отвечает и трек нулевой ширины — рисовать в нём нечего.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func HBarThumb(track image.Rectangle, scroll, maxScroll, viewW, contentW float64) image.Rectangle {
	if track.Dx() <= 0 || maxScroll <= 0 || contentW <= 0 || viewW <= 0 {
		return image.Rectangle{}
	}
	w := int(float64(track.Dx()) * viewW / contentW)
	if w < dvHBarMinW {
		w = dvHBarMinW
	}
	if w >= track.Dx() {
		return image.Rectangle{} // видно всё — прокручивать нечего
	}
	x := track.Min.X + int(math.Round(float64(track.Dx()-w)*clamp01(scroll/maxScroll)))
	return image.Rect(x, track.Min.Y, x+w, track.Max.Y)
}

// HBarScrollAt — смещение, при котором ползунок встанет серединой в точку x.
// Нужен для щелчка мимо ползунка: содержимое прыгает туда, куда ткнули.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func HBarScrollAt(track image.Rectangle, x int, maxScroll, viewW, contentW float64) float64 {
	th := HBarThumb(track, 0, maxScroll, viewW, contentW)
	if th.Empty() {
		return 0
	}
	span := track.Dx() - th.Dx()
	if span <= 0 {
		return 0
	}
	v := float64(x-th.Dx()/2-track.Min.X) / float64(span) * maxScroll
	return math.Max(0, math.Min(v, maxScroll))
}

// HBarScrollForThumbX — смещение, при котором ЛЕВЫЙ край ползунка стоит в x.
// Нужен для перетаскивания: содержимое едет ровно на столько, на сколько
// протянули ползунок, без скачка под курсор.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func HBarScrollForThumbX(track image.Rectangle, x int, maxScroll, viewW, contentW float64) float64 {
	th := HBarThumb(track, 0, maxScroll, viewW, contentW)
	if th.Empty() {
		return 0
	}
	span := track.Dx() - th.Dx()
	if span <= 0 {
		return 0
	}
	v := float64(x-track.Min.X) / float64(span) * maxScroll
	return math.Max(0, math.Min(v, maxScroll))
}

// clamp01 зажимает долю в [0,1]: смещение за краем содержимого не должно
// выносить ползунок за трек.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// DrawHBar рисует трек и ползунок в том же оформлении, что полоса-обзор.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func DrawHBar(ctx DrawContext, track, thumb image.Rectangle, trackCol, thumbCol color.RGBA) {
	if track.Dx() <= 0 || track.Dy() <= 0 || thumb.Empty() {
		return
	}
	r := track.Dy() / 2
	ctx.FillRoundRect(track.Min.X, track.Min.Y, track.Dx(), track.Dy(), r, trackCol)
	ctx.FillRoundRect(thumb.Min.X, thumb.Min.Y, thumb.Dx(), thumb.Dy(), r, thumbCol)
}
