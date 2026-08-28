// radialgradient.go — радиальный градиент: цвет меняется от центра к краю.
//
// Линейный градиент (gradient.go) описывает переход вдоль оси, и подсветку
// под значком дока им не выразить: там свет расходится кругом от точки. То же
// нужно кнопке под курсором в «стеклянных» темах и любому ореолу.
//
// Рисуется не попиксельно, а образом с последующим растяжением: движок
// разворачивает маленькую картинку билинейной интерполяцией, и это и быстрее
// попиксельной записи, и глаже на дробном HiDPI-масштабе — тем же приёмом
// пользуется линейный градиент (drawGradientScaled).
package widget

import (
	"image"
	"image/color"
	"math"
)

// RadialGradient — круговой градиент внутри прямоугольника.
//
// Центр и радиус заданы ДОЛЯМИ размера области, а не пикселями: одна и та же
// подсветка должна одинаково лечь и под значок 24 точки, и под значок 64, и
// пересчитывать её на каждый размер — работа для вызывающего, которой можно
// не быть.
type RadialGradient struct {
	// Stops — опорные точки от центра (0) к краю (1).
	Stops []GradientStop
	// CenterX, CenterY — центр в долях ширины и высоты (0.5, 0.5 — середина).
	CenterX, CenterY float64
	// Radius — радиус в долях ПОЛОВИНЫ большей стороны (1 — до края).
	Radius float64
}

// NewRadialGradient создаёт подсветку из центра цветом col: непрозрачный в
// середине, полностью прозрачный к краю.
//
// Самый частый случай, ради которого радиальный градиент и нужен, — и в нём
// легко ошибиться с альфой: цвета в движке хранятся с предумноженной альфой,
// поэтому прозрачный край — это НУЛЕВОЙ цвет, а не тот же цвет с A=0.
func NewRadialGradient(col color.RGBA) *RadialGradient {
	return &RadialGradient{
		Stops:   []GradientStop{{Color: col, Offset: 0}, {Color: color.RGBA{}, Offset: 1}},
		CenterX: 0.5, CenterY: 0.5, Radius: 1,
	}
}

// colorAt возвращает цвет на расстоянии t от центра (0..1).
func (g *RadialGradient) colorAt(t float64) color.RGBA {
	return colorAtStops(g.Stops, t)
}

// radialSteps — сторона образа, который растягивается на всю область.
//
// 64 точки хватает: дальше разница не видна, а образ строится на каждый
// вызов, и его цена — квадрат стороны.
const radialSteps = 64

// DrawRadialGradient заполняет прямоугольник r радиальным градиентом.
//
// Область вне радиуса заливается последним цветом стопов — обычно
// прозрачным, то есть не закрашивается вовсе.
func DrawRadialGradient(ctx DrawContext, r image.Rectangle, g *RadialGradient) {
	if g == nil || len(g.Stops) == 0 || r.Empty() {
		return
	}
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	radius := g.Radius
	if radius <= 0 {
		radius = 1
	}

	n := radialSteps
	if n > w {
		n = w
	}
	if n > h {
		n = h
	}
	if n < 2 {
		ctx.FillRectAlpha(r.Min.X, r.Min.Y, w, h, g.colorAt(0))
		return
	}

	img := image.NewRGBA(image.Rect(0, 0, n, n))
	cx := g.CenterX * float64(n-1)
	cy := g.CenterY * float64(n-1)
	// Радиус в единицах образа: доля от половины стороны.
	rad := radius * float64(n-1) / 2
	if rad <= 0 {
		rad = 1
	}
	for y := 0; y < n; y++ {
		dy := float64(y) - cy
		for x := 0; x < n; x++ {
			dx := float64(x) - cx
			t := math.Sqrt(dx*dx+dy*dy) / rad
			if t > 1 {
				t = 1
			}
			img.SetRGBA(x, y, g.colorAt(t))
		}
	}
	ctx.DrawImageScaled(img, r.Min.X, r.Min.Y, w, h)
}
