package widget

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

const (
	dvHBarH      = 10 // высота полосы вместе с полями
	dvHBarMinW   = 24 // ползунок короче этого не ужимается — за него не схватиться
	dvHBarInsetX = 4  // запас хита слева и справа от трека
)

// hbarThumb — ползунок на треке track: его ширина пропорциональна тому, какая
// доля содержимого видна, а положение — текущему смещению.
//
// Пустой прямоугольник означает «полоса не нужна»: содержимое помещается
// целиком. Так же отвечает и трек нулевой ширины — рисовать в нём нечего.
func hbarThumb(track image.Rectangle, scroll, maxScroll, viewW, contentW float64) image.Rectangle {
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

// hbarScrollAt — смещение, при котором ползунок встанет серединой в точку x.
// Нужен для щелчка мимо ползунка: содержимое прыгает туда, куда ткнули.
func hbarScrollAt(track image.Rectangle, x int, maxScroll, viewW, contentW float64) float64 {
	th := hbarThumb(track, 0, maxScroll, viewW, contentW)
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

// hbarScrollForThumbX — смещение, при котором ЛЕВЫЙ край ползунка стоит в x.
// Нужен для перетаскивания: содержимое едет ровно на столько, на сколько
// протянули ползунок, без скачка под курсор.
func hbarScrollForThumbX(track image.Rectangle, x int, maxScroll, viewW, contentW float64) float64 {
	th := hbarThumb(track, 0, maxScroll, viewW, contentW)
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

// drawHBar рисует трек и ползунок в том же оформлении, что полоса-обзор.
func drawHBar(ctx DrawContext, track, thumb image.Rectangle, trackCol, thumbCol color.RGBA) {
	if track.Dx() <= 0 || track.Dy() <= 0 || thumb.Empty() {
		return
	}
	r := track.Dy() / 2
	ctx.FillRoundRect(track.Min.X, track.Min.Y, track.Dx(), track.Dy(), r, trackCol)
	ctx.FillRoundRect(thumb.Min.X, thumb.Min.Y, thumb.Dx(), thumb.Dy(), r, thumbCol)
}
