package tests

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// namedCurve — кривая с именем для параметризованных проверок.
type namedCurve struct {
	name  string
	curve widget.Easing
	// monotone — ожидается ли неубывание f(t) на [0,1] (не требуется
	// для Back/Elastic/Bounce, у которых есть выбросы/колебания).
	monotone bool
	// rangeMax — верхняя граница "разумного" диапазона для этой кривой.
	// Для большинства кривых это 1.2. Каноническая (easings.net) формула
	// EaseOutElastic даёт пружинный выброс до ~1.37 на сетке шагом 0.01 —
	// это не баг, а суть эффекта "пружины", поэтому у неё отдельный, более
	// широкий, но всё равно ограниченный предел.
	rangeMax float64
}

// allCurves — полный набор кривых из widget/easing.go.
func allCurves() []namedCurve {
	return []namedCurve{
		{"Linear", widget.EaseLinear, true, 1.2},
		{"InQuad", widget.EaseInQuad, true, 1.2},
		{"OutQuad", widget.EaseOutQuad, true, 1.2},
		{"InOutQuad", widget.EaseInOutQuad, true, 1.2},
		{"InCubic", widget.EaseInCubic, true, 1.2},
		{"OutCubic", widget.EaseOutCubic, true, 1.2},
		{"InOutCubic", widget.EaseInOutCubic, true, 1.2},
		{"InSine", widget.EaseInSine, true, 1.2},
		{"OutSine", widget.EaseOutSine, true, 1.2},
		{"InOutSine", widget.EaseInOutSine, true, 1.2},
		{"OutBack", widget.EaseOutBack, false, 1.2},
		{"OutElastic", widget.EaseOutElastic, false, 1.4},
		{"OutBounce", widget.EaseOutBounce, false, 1.2},
	}
}

const epsilon = 1e-9

// TestEasingBoundaries проверяет f(0)==0 и f(1)==1 для каждой кривой.
func TestEasingBoundaries(t *testing.T) {
	for _, nc := range allCurves() {
		t.Run(nc.name, func(t *testing.T) {
			if got := nc.curve(0); math.Abs(got-0) > epsilon {
				t.Errorf("%s(0) = %v, хотим 0 (допуск %v)", nc.name, got, epsilon)
			}
			if got := nc.curve(1); math.Abs(got-1) > epsilon {
				t.Errorf("%s(1) = %v, хотим 1 (допуск %v)", nc.name, got, epsilon)
			}
		})
	}
}

// TestEasingClamp проверяет, что вход вне [0,1] зажимается до применения кривой.
func TestEasingClamp(t *testing.T) {
	for _, nc := range allCurves() {
		t.Run(nc.name, func(t *testing.T) {
			if got := nc.curve(-1); math.Abs(got-0) > epsilon {
				t.Errorf("%s(-1) = %v, хотим 0 (clamp)", nc.name, got)
			}
			if got := nc.curve(2); math.Abs(got-1) > epsilon {
				t.Errorf("%s(2) = %v, хотим 1 (clamp)", nc.name, got)
			}
		})
	}
}

// TestEasingMonotone — для Linear/Quad/Cubic/Sine кривая не должна убывать
// при возрастании t. Back/Elastic/Bounce намеренно исключены (выбросы,
// колебания — это их суть, а не баг).
func TestEasingMonotone(t *testing.T) {
	const steps = 200
	for _, nc := range allCurves() {
		if !nc.monotone {
			continue
		}
		t.Run(nc.name, func(t *testing.T) {
			prev := nc.curve(0)
			for i := 1; i <= steps; i++ {
				tt := float64(i) / steps
				v := nc.curve(tt)
				if v < prev-epsilon {
					t.Fatalf("%s не монотонна: f(%v)=%v < f(%v)=%v", nc.name, tt, v, float64(i-1)/steps, prev)
				}
				prev = v
			}
		})
	}
}

// TestEasingRange проверяет, что значения всех кривых остаются в разумных
// пределах на сетке t = 0, 0.01, ..., 1 — защита от кривых формул с
// неограниченным выбросом/колебанием. Нижняя граница -0.2 общая для всех;
// верхняя — nc.rangeMax (см. комментарий в allCurves про EaseOutElastic).
func TestEasingRange(t *testing.T) {
	const rangeMin = -0.2
	for _, nc := range allCurves() {
		t.Run(nc.name, func(t *testing.T) {
			for i := 0; i <= 100; i++ {
				tt := float64(i) / 100
				v := nc.curve(tt)
				if v < rangeMin || v > nc.rangeMax {
					t.Fatalf("%s(%v) = %v вне диапазона [%v, %v]", nc.name, tt, v, rangeMin, nc.rangeMax)
				}
			}
		})
	}
}

// TestLerpF проверяет базовую линейную интерполяцию чисел.
func TestLerpF(t *testing.T) {
	cases := []struct {
		a, b, tt, want float64
	}{
		{0, 10, 0, 0},
		{0, 10, 1, 10},
		{0, 10, 0.5, 5},
		{10, 0, 0.5, 5},
		{-5, 5, 0.5, 0},
	}
	for _, c := range cases {
		if got := widget.LerpF(c.a, c.b, c.tt); math.Abs(got-c.want) > epsilon {
			t.Errorf("LerpF(%v, %v, %v) = %v, хотим %v", c.a, c.b, c.tt, got, c.want)
		}
	}
}

// TestLerpInt проверяет округление (а не усечение) при интерполяции целых.
func TestLerpInt(t *testing.T) {
	// 0 + (10-0)*0.5 = 5 — ровно, без эффекта округления.
	if got := widget.LerpInt(0, 10, 0.5); got != 5 {
		t.Errorf("LerpInt(0, 10, 0.5) = %d, хотим 5", got)
	}
	// 0 + (3-0)*0.5 = 1.5 — округление вверх до 2 (не усечение до 1).
	if got := widget.LerpInt(0, 3, 0.5); got != 2 {
		t.Errorf("LerpInt(0, 3, 0.5) = %d, хотим 2 (округление 1.5 -> 2)", got)
	}
	// Границы t=0 / t=1.
	if got := widget.LerpInt(2, 8, 0); got != 2 {
		t.Errorf("LerpInt(2, 8, 0) = %d, хотим 2", got)
	}
	if got := widget.LerpInt(2, 8, 1); got != 8 {
		t.Errorf("LerpInt(2, 8, 1) = %d, хотим 8", got)
	}
}

// TestLerpRect проверяет покомпонентную интерполяцию прямоугольника.
func TestLerpRect(t *testing.T) {
	a := image.Rect(0, 0, 10, 20)
	b := image.Rect(10, 20, 30, 60)
	got := widget.LerpRect(a, b, 0.5)
	want := image.Rect(5, 10, 20, 40)
	if got != want {
		t.Errorf("LerpRect(%v, %v, 0.5) = %v, хотим %v", a, b, got, want)
	}

	if got := widget.LerpRect(a, b, 0); got != a {
		t.Errorf("LerpRect(a, b, 0) = %v, хотим %v", got, a)
	}
	if got := widget.LerpRect(a, b, 1); got != b {
		t.Errorf("LerpRect(a, b, 1) = %v, хотим %v", got, b)
	}
}

// TestLerpColor проверяет покомпонентную интерполяцию premultiplied-цвета,
// в т.ч. случай, где альфа тоже интерполируется (полупрозрачный цвет).
func TestLerpColor(t *testing.T) {
	a := color.RGBA{0, 0, 0, 0}
	b := color.RGBA{100, 50, 200, 255}
	got := widget.LerpColor(a, b, 0.5)
	want := color.RGBA{50, 25, 100, 128} // 200*0.5=100 -> округление 127.5 не участвует, 255*0.5=127.5 -> 128
	if got != want {
		t.Errorf("LerpColor(%v, %v, 0.5) = %v, хотим %v", a, b, got, want)
	}

	// Границы t=0 / t=1 — ровно исходные цвета.
	if got := widget.LerpColor(a, b, 0); got != a {
		t.Errorf("LerpColor(a, b, 0) = %v, хотим %v", got, a)
	}
	if got := widget.LerpColor(a, b, 1); got != b {
		t.Errorf("LerpColor(a, b, 1) = %v, хотим %v", got, b)
	}
}
