package engine

import (
	"image"
	"image/color"
	"testing"
)

// newTestCanvas — холст нужного размера с чёрным фоном.
func newTestCanvas(w, h int) *Canvas {
	c := newCanvas(w, h, newFontCache("assets"))
	c.blitBackground()
	return c
}

func pixAt(c *Canvas, x, y int) color.RGBA {
	return c.back.RGBAAt(x, y)
}

// TestRoundClip_CutsCornersKeepsMiddle — заливка, отсечённая скруглённым
// контуром, не попадает в углы, но занимает всё остальное.
func TestRoundClip_CutsCornersKeepsMiddle(t *testing.T) {
	c := newTestCanvas(100, 100)
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	c.SetRoundClip(image.Rect(10, 10, 90, 90), 20)
	c.FillRect(0, 0, 100, 100, white) // заливка заведомо шире клипа
	c.ClearClip()

	// Угол скруглённой области остаётся фоном.
	if got := pixAt(c, 11, 11); got == white {
		t.Error("верхний левый угол не отсечён кривой")
	}
	// Центр залит.
	if got := pixAt(c, 50, 50); got != white {
		t.Errorf("центр не залит: %v", got)
	}
	// Середина верхней грани — внутри контура, залита.
	if got := pixAt(c, 50, 11); got != white {
		t.Errorf("середина верхней грани не залита: %v", got)
	}
	// За пределами прямоугольной части клипа — фон.
	if got := pixAt(c, 5, 50); got == white {
		t.Error("заливка вышла за прямоугольную часть клипа")
	}
}

// TestRoundClip_MatchesFilledShape — граница отсечения совпадает с границей
// заливки скруглённого прямоугольника. Иначе на углу остаётся щель в
// пиксель или содержимое наползает на фон.
func TestRoundClip_MatchesFilledShape(t *testing.T) {
	const w, h, r = 80, 60, 16
	rect := image.Rect(0, 0, w, h)
	red := color.RGBA{R: 200, G: 0, B: 0, A: 255}
	blue := color.RGBA{R: 0, G: 0, B: 200, A: 255}

	// Эталон: фигура, нарисованная заливкой (полупрозрачный путь без
	// сглаживания — тот же, по которому считает клип).
	shape := newTestCanvas(w, h)
	shape.fillRoundRectLegacy(0, 0, w, h, r, red)

	// Опыт: сплошная заливка, отсечённая тем же контуром.
	clipped := newTestCanvas(w, h)
	clipped.SetRoundClip(rect, r)
	clipped.FillRect(0, 0, w, h, blue)
	clipped.ClearClip()

	diff := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			inShape := pixAt(shape, x, y) == red
			inClip := pixAt(clipped, x, y) == blue
			if inShape != inClip {
				diff++
			}
		}
	}
	if diff != 0 {
		t.Errorf("контур отсечения расходится с заливкой в %d пикселях", diff)
	}
}

// TestRoundClip_AppliesToTextAndPixels — кривая режет не только заливки:
// текст и отдельные пиксели тоже.
func TestRoundClip_AppliesToTextAndPixels(t *testing.T) {
	c := newTestCanvas(120, 60)
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	c.SetRoundClip(image.Rect(0, 0, 120, 60), 24)
	// Пиксель в отсечённом углу и пиксель в центре.
	c.SetPixel(1, 1, white)
	c.SetPixel(60, 30, white)
	// Текст, начинающийся в углу — часть букв должна срезаться.
	c.DrawText("ЖЖЖЖЖЖЖЖЖЖ", 0, 2, white)
	c.ClearClip()

	if pixAt(c, 1, 1) == white {
		t.Error("SetPixel в отсечённом углу не срезан")
	}
	if pixAt(c, 60, 30) != white {
		t.Error("SetPixel в центре не нарисован")
	}
	// В отсечённом углу не должно быть ни одного непустого пикселя текста.
	for y := 0; y < 6; y++ {
		for x := 0; x < 6; x++ {
			if pixAt(c, x, y) != (color.RGBA{A: 255}) {
				t.Errorf("текст попал в отсечённый угол в (%d,%d): %v", x, y, pixAt(c, x, y))
				return
			}
		}
	}
}

// TestRoundClip_ClearRestoresRectangular — снятие скруглённого отсечения
// возвращает обычное прямоугольное поведение.
func TestRoundClip_ClearRestoresRectangular(t *testing.T) {
	c := newTestCanvas(60, 60)
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	c.SetRoundClip(image.Rect(0, 0, 60, 60), 20)
	if !c.HasRoundClip() {
		t.Fatal("скруглённое отсечение не включилось")
	}
	c.ClearRoundClip()
	if c.HasRoundClip() {
		t.Fatal("ClearRoundClip не выключил отсечение")
	}
	c.FillRect(0, 0, 60, 60, white)
	if pixAt(c, 1, 1) != white {
		t.Error("после снятия отсечения угол остался срезанным")
	}
	c.ClearClip()
}

// TestRoundClip_ZeroRadiusIsRectangular — нулевой радиус равнозначен
// обычному прямоугольному отсечению, а не отсутствию отсечения.
func TestRoundClip_ZeroRadiusIsRectangular(t *testing.T) {
	c := newTestCanvas(60, 60)
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	c.SetRoundClip(image.Rect(10, 10, 50, 50), 0)
	if c.HasRoundClip() {
		t.Error("нулевой радиус включил скруглённое отсечение")
	}
	c.FillRect(0, 0, 60, 60, white)
	c.ClearClip()

	if pixAt(c, 11, 11) != white {
		t.Error("угол прямоугольного клипа срезан")
	}
	if pixAt(c, 5, 5) == white {
		t.Error("заливка вышла за прямоугольный клип")
	}
}

// TestRoundClip_HiDPI — при масштабе контур считается в физических
// пикселях: радиус и границы масштабируются вместе с остальным.
func TestRoundClip_HiDPI(t *testing.T) {
	c := newCanvasScaled(60, 60, 2, newFontCache("assets"))
	c.blitBackground()
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	c.SetRoundClip(image.Rect(0, 0, 60, 60), 10)
	c.FillRect(0, 0, 60, 60, white)
	c.ClearClip()

	// Физический холст 120×120; угол срезан, центр залит.
	if pixAt(c, 2, 2) == white {
		t.Error("угол не срезан при масштабе 2")
	}
	if pixAt(c, 60, 60) != white {
		t.Error("центр не залит при масштабе 2")
	}
	// Радиус масштабировался: точка (25,3) лежит внутри контура радиуса 20
	// физических пикселей и должна быть залита.
	if pixAt(c, 25, 3) != white {
		t.Error("радиус не масштабирован — срезано слишком много")
	}
}

// TestRoundClip_IntersectsWithBaseClip — скруглённое отсечение не отменяет
// базовый клип кадра (область частичной перерисовки).
func TestRoundClip_IntersectsWithBaseClip(t *testing.T) {
	c := newTestCanvas(100, 100)
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	c.setBaseClip(image.Rect(0, 0, 50, 100)) // перерисовываем только левую половину
	defer c.clearBaseClip()
	c.SetRoundClip(image.Rect(0, 0, 100, 100), 10)
	c.FillRect(0, 0, 100, 100, white)
	c.ClearClip()

	if pixAt(c, 25, 50) != white {
		t.Error("внутри damage-области не нарисовано")
	}
	if pixAt(c, 75, 50) == white {
		t.Error("нарисовано вне damage-области — базовый клип потерян")
	}
}

// Дуги полупрозрачной скруглённой рамки смешиваются с фоном, а не пишутся
// поверх него цветом с чужой альфой.
//
// Прямые стороны это уже умели, а четыре угла — нет: на стеклянной панели
// обводка ложилась плёнкой по сторонам и «дырами» на углах.
func TestRoundBorder_TranslucentCornersBlend(t *testing.T) {
	const w, h = 80, 60
	c := newCanvas(w, h, nil)

	// Ровный тёмный фон, поверх — светлая полупрозрачная рамка.
	c.FillRect(0, 0, w, h, color.RGBA{R: 20, G: 30, B: 40, A: 255})
	border := color.RGBA{R: 255, G: 255, B: 255, A: 60}
	c.DrawRoundBorder(4, 4, w-8, h-8, 10, border)

	// Точка на прямой стороне и точка на дуге обязаны лежать в одном
	// диапазоне: обе — та же плёнка поверх того же фона.
	side := c.back.RGBAAt(w/2, 4)
	if side.R == 255 {
		t.Fatalf("сторона рамки записана как есть (%v) — смешивания нет", side)
	}

	// Ищем самую светлую точку в угловой зоне: там лежит дуга.
	var corner color.RGBA
	for y := 4; y < 18; y++ {
		for x := 4; x < 18; x++ {
			p := c.back.RGBAAt(x, y)
			if int(p.R)+int(p.G)+int(p.B) > int(corner.R)+int(corner.G)+int(corner.B) {
				corner = p
			}
		}
	}
	if corner.A != 255 {
		t.Errorf("на дуге альфа %d — цвет записан вместе с чужой альфой", corner.A)
	}
	if diff := int(corner.R) - int(side.R); diff > 24 || diff < -24 {
		t.Errorf("дуга (%v) и сторона (%v) легли по-разному — угол не смешан как сторона",
			corner, side)
	}
}
