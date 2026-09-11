package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Сглаженные пути с дробными координатами — пункт 1 замечаний difftool.
//
// AAShapes принимал только целые точки: гладкую кривую приходилось округлять,
// и погрешность ±0,5 точки на тонкой линии давала «лесенку». А ломаная
// обводилась суммой прямоугольников без сопряжений — снаружи изгиба оставался
// зубец.

var black = color.RGBA{A: 255}

// pathCanvas — чистый белый холст заданного масштаба.
func pathCanvas(w, h int, scale float64) *Canvas {
	c := newCanvasScaled(w, h, scale, nil)
	c.FillRect(0, 0, w, h, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	return c
}

// ink — насколько точка затемнена (0 — белая, 255 — чёрная).
func ink(c *Canvas, x, y int) int {
	img := c.Snapshot(image.Rect(x, y, x+1, y+1))
	if img == nil {
		return 0
	}
	return 255 - int(img.Pix[0])
}

// Дробная координата доходит до холста: линия на y=10.25 и на y=10.75 ложится
// по-разному. С целыми точками обе округлились бы в одну.
func TestPathAA_FractionalCoordinates(t *testing.T) {
	draw := func(y float64) (int, int) {
		c := pathCanvas(40, 30, 1)
		c.StrokePathAA([]widget.Point2F{{X: 5, Y: y}, {X: 35, Y: y}}, 1, false, black)
		return ink(c, 20, 10), ink(c, 20, 11)
	}
	a10, a11 := draw(10.25)
	b10, b11 := draw(10.75)
	if a10 == b10 && a11 == b11 {
		t.Errorf("линии на 10.25 и 10.75 нарисованы одинаково (%d/%d) — дробь потерялась", a10, a11)
	}
	// Ниже лежащая линия темнит нижнюю строку сильнее.
	if b11 <= a11 {
		t.Errorf("линия на 10.75 темнит строку 11 не сильнее, чем на 10.25: %d против %d", b11, a11)
	}
}

// На изгибе нет зубца: точка снаружи угла закрыта сопряжением.
func TestPathAA_JoinClosesTheNotch(t *testing.T) {
	c := pathCanvas(60, 60, 1)
	// Прямой угол в (30,30): вправо, затем вниз. Снаружи угла — (31,29)…
	c.StrokePolylineAA([]image.Point{{X: 5, Y: 30}, {X: 30, Y: 30}, {X: 30, Y: 55}}, 6, false, black)

	// Внешний угол изгиба — на диагонали вверх-вправо от вершины, в пределах
	// половины толщины. Без сопряжения там белая щель между прямоугольниками.
	if v := ink(c, 32, 28); v < 128 {
		t.Errorf("снаружи изгиба щель: затемнение %d", v)
	}
}

// Концы незамкнутой ломаной прямые, как раньше: круглые удлинили бы каждую
// нарисованную линию на полтолщины.
func TestPathAA_EndsStayButt(t *testing.T) {
	c := pathCanvas(60, 30, 1)
	c.DrawLineAA(10, 15, 50, 15, 6, black)
	if v := ink(c, 7, 15); v > 16 {
		t.Errorf("за концом линии нарисовано (%d) — конец стал круглым", v)
	}
	if v := ink(c, 12, 15); v < 200 {
		t.Errorf("сама линия не нарисована: %d", v)
	}
}

// Кривая Безье ложится непрерывно: нет ни одного разрыва вдоль неё.
func TestPathAA_CubicIsContinuous(t *testing.T) {
	c := pathCanvas(120, 240, 1)
	// S-коннектор: пролёт 70 по x, 200 по y — как между блоками сравнения.
	c.StrokeCubicAA(20, 20, 60, 20, 60, 220, 90, 220, 2, black)

	for _, p := range widget.FlattenCubic(20, 20, 60, 20, 60, 220, 90, 220) {
		x, y := int(p.X+0.5), int(p.Y+0.5)
		if v := ink(c, x, y); v < 100 {
			t.Fatalf("разрыв кривой у (%d,%d): затемнение %d", x, y, v)
		}
	}
}

// Заливка дробного многоугольника: край, пришедшийся на середину точки, даёт
// полупрозрачную точку.
func TestPathAA_FillFractional(t *testing.T) {
	c := pathCanvas(40, 40, 1)
	// Целая координата — центр точки: правый край X=21 лежит в физических
	// 21.5, то есть делит точку 21 пополам.
	c.FillPathAA([]widget.Point2F{{X: 10, Y: 10}, {X: 21, Y: 10}, {X: 21, Y: 20}, {X: 10, Y: 20}}, black)
	if v := ink(c, 15, 15); v < 250 {
		t.Errorf("середина не залита: %d", v)
	}
	if v := ink(c, 21, 15); v < 60 || v > 200 {
		t.Errorf("край на середине точки не полупрозрачен: %d", v)
	}
}

// HiDPI учитывает холст: приложению больше не нужно передавать масштаб в
// виджет, чтобы кривая не вышла вдвое тоньше.
func TestPathAA_ScaledByCanvas(t *testing.T) {
	c := pathCanvas(80, 80, 2) // физически 160×160
	c.StrokePathAA([]widget.Point2F{{X: 5, Y: 20}, {X: 35, Y: 20}}, 2, false, black)
	// Snapshot берёт ЛОГИЧЕСКИЙ прямоугольник и отдаёт физические точки:
	// логические строки 18…22 — это физические 36…45. Линия толщиной 2 при
	// масштабе 2 закрывает четыре физических строки.
	img := c.Snapshot(image.Rect(20, 18, 21, 23))
	if img == nil {
		t.Fatal("снимок пуст")
	}
	dark := 0
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		i := img.PixOffset(img.Rect.Min.X, y)
		if img.Pix[i] < 128 {
			dark++
		}
	}
	if dark < 3 || dark > 5 {
		t.Errorf("при масштабе 2 линия толщиной 2 закрыла %d физических строк, ожидалось около 4", dark)
	}
}
