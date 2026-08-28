package engine

import (
	"image"
	"image/color"
	"testing"
)

// shadowColor — базовый цвет тени для тестов: полупрозрачный чёрный.
var shadowColor = color.RGBA{A: 120}

// newWhiteTestCanvas — холст с БЕЛЫМ фоном. newTestCanvas (clipround_test.go)
// заливает чёрным, на котором чёрная (premultiplied) тень не даёт видимой
// разницы в RGB — Over чёрного по чёрному не меняет цвет, только альфу,
// которая у непрозрачного фона и так уже 255. Белый фон делает затемнение
// от тени измеримым по каналам R/G/B.
func newWhiteTestCanvas(w, h int) *Canvas {
	c := newCanvas(w, h, newFontCache("assets"))
	c.blitBackground()
	c.FillRect(0, 0, w, h, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	return c
}

// darkness возвращает «затемнённость» пикселя относительно белого фона:
// 0 — пиксель нетронут (белый), больше — темнее.
func darkness(c *Canvas, x, y int) int {
	p := pixAt(c, x, y)
	return 255*3 - int(p.R) - int(p.G) - int(p.B)
}

// TestSoftShadow_DarkerUnderCenterFadesOutward — тень темнее всего под
// центром прямоугольника и монотонно светлеет по мере ухода в сторону от
// него по горизонтали.
func TestSoftShadow_DarkerUnderCenterFadesOutward(t *testing.T) {
	c := newWhiteTestCanvas(300, 200) // шире обычного — точка "далеко" должна остаться в границах холста
	r := image.Rect(60, 60, 140, 100) // 80×40
	c.DrawSoftShadow(r, 8, 16, shadowColor)

	cy := r.Max.Y // чуть ниже нижней грани, где тень заметнее всего
	cx := (r.Min.X + r.Max.X) / 2

	if darkness(c, cx, cy) <= 0 {
		t.Fatal("под центром прямоугольника тень не темнее фона")
	}

	prev := darkness(c, cx, cy)
	for _, dx := range []int{10, 20, 40, 70} {
		d := darkness(c, cx+dx, cy)
		if d > prev {
			t.Errorf("тень не монотонно светлеет наружу: на dx=%d темнее (%d), чем на предыдущей точке (%d)", dx, d, prev)
		}
		prev = d
	}
	if darkness(c, cx+100, cy) != 0 {
		t.Errorf("далеко от прямоугольника фон должен остаться нетронутым, получили затемнение %d", darkness(c, cx+100, cy))
	}
}

// TestSoftShadow_OffsetDown — тень смещена вниз: под нижней гранью она
// заметнее, чем над верхней (на равном расстоянии от границ прямоугольника).
func TestSoftShadow_OffsetDown(t *testing.T) {
	c := newWhiteTestCanvas(200, 200)
	r := image.Rect(60, 60, 140, 120)
	c.DrawSoftShadow(r, 8, 20, shadowColor)

	cx := (r.Min.X + r.Max.X) / 2
	const off = 15
	below := darkness(c, cx, r.Max.Y+off)
	above := darkness(c, cx, r.Min.Y-off)

	if below <= above {
		t.Errorf("тень должна быть заметнее под нижней гранью: below=%d, above=%d", below, above)
	}
}

// TestSoftShadow_NoOpWhenElevationZeroOrTransparent — elevation<=0 либо
// полностью прозрачный цвет не меняют холст ни на пиксель.
func TestSoftShadow_NoOpWhenElevationZeroOrTransparent(t *testing.T) {
	r := image.Rect(50, 50, 150, 130)

	t.Run("elevation<=0", func(t *testing.T) {
		c := newWhiteTestCanvas(200, 200)
		before := append([]byte(nil), c.back.Pix...)
		c.DrawSoftShadow(r, 8, 0, shadowColor)
		c.DrawSoftShadow(r, 8, -5, shadowColor)
		if string(before) != string(c.back.Pix) {
			t.Error("elevation<=0 изменил холст")
		}
	})

	t.Run("прозрачный цвет", func(t *testing.T) {
		c := newWhiteTestCanvas(200, 200)
		before := append([]byte(nil), c.back.Pix...)
		c.DrawSoftShadow(r, 8, 16, color.RGBA{})
		if string(before) != string(c.back.Pix) {
			t.Error("полностью прозрачный цвет изменил холст")
		}
	})
}

// TestSoftShadow_RespectsRectClip — тень не рисуется за пределами
// прямоугольного отсечения, установленного SetClip.
func TestSoftShadow_RespectsRectClip(t *testing.T) {
	c := newWhiteTestCanvas(200, 200)
	r := image.Rect(60, 60, 140, 120)

	// Клип — точно прямоугольник тени: тень «хотела бы» выйти за него с
	// запасом на размытие, но не должна нарисовать там ничего.
	clip := image.Rect(60, 60, 140, 120)
	c.SetClip(clip)
	c.DrawSoftShadow(r, 8, 30, shadowColor)
	c.ClearClip()

	// Точки снаружи клипа, но внутри «естественного» радиуса размытия тени.
	probes := []image.Point{
		{clip.Min.X - 5, (clip.Min.Y + clip.Max.Y) / 2},
		{clip.Max.X + 5, (clip.Min.Y + clip.Max.Y) / 2},
		{(clip.Min.X + clip.Max.X) / 2, clip.Min.Y - 5},
		{(clip.Min.X + clip.Max.X) / 2, clip.Max.Y + 5},
	}
	for _, p := range probes {
		if darkness(c, p.X, p.Y) != 0 {
			t.Errorf("тень нарисована за пределами SetClip в точке %v", p)
		}
	}
}

// TestSoftShadow_RespectsRoundClip — тень не рисуется в углу, срезанном
// SetRoundClip.
func TestSoftShadow_RespectsRoundClip(t *testing.T) {
	c := newWhiteTestCanvas(200, 200)
	r := image.Rect(20, 20, 180, 180)

	c.SetRoundClip(r, 40)
	c.DrawSoftShadow(r, 8, 30, shadowColor)
	c.ClearClip()
	c.ClearRoundClip()

	// Самая крайняя точка угла клипа — заведомо срезана кривой скругления.
	corner := image.Pt(r.Min.X+1, r.Min.Y+1)
	if darkness(c, corner.X, corner.Y) != 0 {
		t.Errorf("тень нарисована в срезанном скруглённым клипом углу %v: %v", corner, pixAt(c, corner.X, corner.Y))
	}
}
