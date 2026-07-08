package widget

import (
	"image"
	"image/color"
	"math"
	"time"
)

// Easing — кривая анимации: отображает линейный прогресс t∈[0,1] в
// сглаженный. Вне [0,1] вход зажимается (clamp) ДО применения кривой.
type Easing func(t float64) float64

// clampT ограничивает вход кривой отрезком [0,1]. Все кривые ниже
// пропускают t через этот хелпер прежде, чем применять формулу —
// это гарантирует f(-1)==f(0) и f(2)==f(1) для любой кривой.
func clampT(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// ─── Линейная ────────────────────────────────────────────────────────────────

// EaseLinear — без сглаживания, f(t) = t.
var EaseLinear Easing = func(t float64) float64 {
	return clampT(t)
}

// ─── Quad (t²) ───────────────────────────────────────────────────────────────

// EaseInQuad — медленный старт, ускорение к концу.
var EaseInQuad Easing = func(t float64) float64 {
	t = clampT(t)
	return t * t
}

// EaseOutQuad — быстрый старт, замедление к концу.
var EaseOutQuad Easing = func(t float64) float64 {
	t = clampT(t)
	return 1 - (1-t)*(1-t)
}

// EaseInOutQuad — медленный старт и медленный конец, ускорение в середине.
var EaseInOutQuad Easing = func(t float64) float64 {
	t = clampT(t)
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - math.Pow(-2*t+2, 2)/2
}

// ─── Cubic (t³) ──────────────────────────────────────────────────────────────

// EaseInCubic — медленный старт, ускорение к концу (сильнее Quad).
var EaseInCubic Easing = func(t float64) float64 {
	t = clampT(t)
	return t * t * t
}

// EaseOutCubic — быстрый старт, замедление к концу (сильнее Quad).
var EaseOutCubic Easing = func(t float64) float64 {
	t = clampT(t)
	return 1 - math.Pow(1-t, 3)
}

// EaseInOutCubic — медленный старт и медленный конец, ускорение в середине.
var EaseInOutCubic Easing = func(t float64) float64 {
	t = clampT(t)
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - math.Pow(-2*t+2, 3)/2
}

// ─── Sine ────────────────────────────────────────────────────────────────────

// EaseInSine — мягкий медленный старт на основе синуса.
var EaseInSine Easing = func(t float64) float64 {
	t = clampT(t)
	return 1 - math.Cos(t*math.Pi/2)
}

// EaseOutSine — мягкое замедление к концу на основе синуса.
var EaseOutSine Easing = func(t float64) float64 {
	t = clampT(t)
	return math.Sin(t * math.Pi / 2)
}

// EaseInOutSine — мягкий разгон и торможение, самая плавная из базовых кривых.
var EaseInOutSine Easing = func(t float64) float64 {
	t = clampT(t)
	return -(math.Cos(math.Pi*t) - 1) / 2
}

// ─── Back / Elastic / Bounce ─────────────────────────────────────────────────

// EaseOutBack — быстрый подлёт с лёгким выбросом за 1.0 (overshoot) и
// возвратом к 1.0. f(0)=0 и f(1)=1 строго; выброс — только в середине.
var EaseOutBack Easing = func(t float64) float64 {
	t = clampT(t)
	const c1 = 1.70158
	const c3 = c1 + 1
	x := t - 1
	return 1 + c3*x*x*x + c1*x*x
}

// EaseOutElastic — эффект «пружины»: колебания затухающей амплитуды
// вокруг 1.0, характерные для физической анимации (масса на пружине).
var EaseOutElastic Easing = func(t float64) float64 {
	t = clampT(t)
	if t == 0 || t == 1 {
		return t
	}
	const c4 = 2 * math.Pi / 3
	return math.Pow(2, -10*t)*math.Sin((t*10-0.75)*c4) + 1
}

// EaseOutBounce — эффект «мячика», подпрыгивающего с затуханием амплитуды.
var EaseOutBounce Easing = func(t float64) float64 {
	t = clampT(t)
	const n1 = 7.5625
	const d1 = 2.75
	switch {
	case t < 1/d1:
		return n1 * t * t
	case t < 2/d1:
		t -= 1.5 / d1
		return n1*t*t + 0.75
	case t < 2.5/d1:
		t -= 2.25 / d1
		return n1*t*t + 0.9375
	default:
		t -= 2.625 / d1
		return n1*t*t + 0.984375
	}
}

// ─── Интерполяции ────────────────────────────────────────────────────────────

// LerpF — линейная интерполяция между a и b по прогрессу t (не клампится:
// значения t вне [0,1] дают экстраполяцию, что нужно для Back/Elastic).
func LerpF(a, b, t float64) float64 {
	return a + (b-a)*t
}

// LerpInt — линейная интерполяция целых с округлением (не усечением):
// LerpInt(0, 3, 0.5) == 2, т.к. 1.5 округляется вверх.
func LerpInt(a, b int, t float64) int {
	return int(math.Round(LerpF(float64(a), float64(b), t)))
}

// LerpRect — покомпонентная линейная интерполяция прямоугольника
// (все четыре координаты через LerpInt).
func LerpRect(a, b image.Rectangle, t float64) image.Rectangle {
	return image.Rect(
		LerpInt(a.Min.X, b.Min.X, t),
		LerpInt(a.Min.Y, b.Min.Y, t),
		LerpInt(a.Max.X, b.Max.X, t),
		LerpInt(a.Max.Y, b.Max.Y, t),
	)
}

// LerpColor — линейная интерполяция цвета по каналам с округлением.
//
// color.RGBA в Go — альфа-премультиплицированный формат: R/G/B уже
// умножены на альфу. Благодаря этому обычная покомпонентная линейная
// интерполяция премультиплицированных каналов (в т.ч. альфы) КОРРЕКТНА
// и даёт визуально верный результат — в отличие от straight-alpha цветов,
// где так интерполировать нельзя (получится потемнение на полупрозрачных
// краях). Поэтому здесь просто лерпим R, G, B, A по отдельности.
func LerpColor(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(math.Round(LerpF(float64(a.R), float64(b.R), t))),
		G: uint8(math.Round(LerpF(float64(a.G), float64(b.G), t))),
		B: uint8(math.Round(LerpF(float64(a.B), float64(b.B), t))),
		A: uint8(math.Round(LerpF(float64(a.A), float64(b.A), t))),
	}
}

// ─── Обёртки поверх Animate (см. anim.go) ───────────────────────────────────

// AnimateFloat анимирует значение от from до to за dur по кривой curve,
// вызывая set на каждом кадре (последний вызов — ровно to).
func AnimateFloat(from, to float64, dur time.Duration, curve Easing, set func(v float64)) *Animation {
	return Animate(dur, curve, func(t float64) { set(LerpF(from, to, t)) })
}

// AnimateRect анимирует границы виджета w от текущего w.Bounds() до to
// за dur по кривой curve, на каждом кадре вызывая w.SetBounds(...).
// Использует AnimateOwned, чтобы повторный вызов для того же w отменял
// предыдущую анимацию границ ("bounds" — тег владения).
func AnimateRect(w Widget, to image.Rectangle, dur time.Duration, curve Easing) *Animation {
	from := w.Bounds()
	return AnimateOwned(w, "bounds", dur, curve, func(t float64) {
		w.SetBounds(LerpRect(from, to, t))
	})
}
