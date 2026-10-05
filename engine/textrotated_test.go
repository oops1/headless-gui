package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// rotatedTextCanvas — белый холст логического размера (w, h) с масштабом scale.
func rotatedTextCanvas(w, h int, scale float64) *Canvas {
	cv := newCanvasScaled(w, h, scale, newFontCache("assets"))
	if scale != 1 {
		cv.setDPIAll(DefaultDPI * scale)
	}
	cv.blitBackground()
	cv.FillRect(0, 0, w, h, color.RGBA{255, 255, 255, 255})
	return cv
}

// Повёрнутая строка — ровно та же, что горизонтальная, с пикселями,
// переставленными на прямой угол вокруг точки (x, y): без передискретизации,
// на любом масштабе.
func TestDrawTextRotated_IsExactPixelRotation(t *testing.T) {
	const text = "WinLine Project"
	black := color.RGBA{0, 0, 0, 255}
	for _, scale := range []float64{1, 2} {
		for _, angle := range []int{90, 180, 270} {
			x, y := 60, 120
			h := rotatedTextCanvas(200, 200, scale)
			h.DrawTextFont(text, x, y, 12, "", black)
			r := rotatedTextCanvas(200, 200, scale)
			if !r.DrawTextRotated(text, x, y, 12, "", angle, black) {
				t.Fatalf("scale %v angle %d: false", scale, angle)
			}
			px, py := h.sx(x), h.sx(y)
			bad, ink := 0, 0
			for hy := 0; hy < h.H; hy++ {
				for hx := 0; hx < h.W; hx++ {
					u, v := hx-px, hy-py
					var rx, ry int
					switch angle {
					case 90:
						rx, ry = px+v, py-u-1
					case 270:
						rx, ry = px-v-1, py+u
					default:
						rx, ry = px-u-1, py-v-1
					}
					if rx < 0 || ry < 0 || rx >= r.W || ry >= r.H {
						continue
					}
					want := h.back.RGBAAt(hx, hy)
					if want.R != 255 {
						ink++
					}
					if got := r.back.RGBAAt(rx, ry); got != want {
						bad++
					}
				}
			}
			if ink == 0 {
				t.Fatalf("scale %v angle %d: в горизонтальной строке нет пикселей текста", scale, angle)
			}
			if bad != 0 {
				t.Errorf("scale %v angle %d: %d пикселей из %d не совпали с поворотом горизонтального текста", scale, angle, bad, ink)
			}
		}
	}
}

// Расположение: 90° — надпись левее-выше точки не выходит, растёт вверх от
// (x, y) и вправо на высоту строки; 270° — вниз и влево.
func TestDrawTextRotated_Placement(t *testing.T) {
	black := color.RGBA{0, 0, 0, 255}
	inkBounds := func(c *Canvas) image.Rectangle {
		var b image.Rectangle
		for y := 0; y < c.H; y++ {
			for x := 0; x < c.W; x++ {
				if c.back.RGBAAt(x, y).R != 255 {
					b = b.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		return b
	}
	c := rotatedTextCanvas(120, 200, 1)
	c.DrawTextRotated("Hello", 50, 150, 12, "", 90, black)
	b := inkBounds(c)
	if b.Min.X < 50 || b.Max.Y > 150 || b.Dy() < c.MeasureText("Hello", 12)-4 || b.Dx() > 20 {
		t.Errorf("90°: надпись %v должна лежать правее x=50, выше y=150 и вытянуться вверх", b)
	}
	c = rotatedTextCanvas(120, 200, 1)
	c.DrawTextRotated("Hello", 50, 40, 12, "", 270, black)
	b = inkBounds(c)
	if b.Max.X > 50 || b.Min.Y < 40 || b.Dy() < c.MeasureText("Hello", 12)-4 {
		t.Errorf("270°: надпись %v должна лежать левее x=50, ниже y=40 и вытянуться вниз", b)
	}
}

func TestDrawTextRotated_ClipAndOddAngles(t *testing.T) {
	black := color.RGBA{0, 0, 0, 255}
	c := rotatedTextCanvas(120, 200, 1)
	if c.DrawTextRotated("x", 10, 10, 12, "", 45, black) {
		t.Error("угол 45° не поддерживается: ждали false")
	}
	if !c.DrawTextRotated("x", 10, 10, 12, "", -90, black) {
		t.Error("-90° — это 270°")
	}
	// Клип режет повёрнутый текст так же, как обычный.
	c = rotatedTextCanvas(120, 200, 1)
	c.SetClip(image.Rect(0, 0, 120, 100))
	c.DrawTextRotated("Hello world", 50, 180, 12, "", 90, black)
	c.ClearClip()
	for y := 100; y < 200; y++ {
		for x := 0; x < 120; x++ {
			if c.back.RGBAAt(x, y).R != 255 {
				t.Fatalf("пиксель (%d,%d) вне клипа закрашен", x, y)
			}
		}
	}
	// Цвет с прозрачностью даёт серый, а не чёрный.
	c = rotatedTextCanvas(120, 200, 1)
	c.DrawTextRotated("HHH", 50, 100, 14, "", 90, color.RGBA{0, 0, 0, 128})
	min := uint8(255)
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			if v := c.back.RGBAAt(x, y).R; v < min {
				min = v
			}
		}
	}
	if min < 100 || min > 140 {
		t.Errorf("текст с A=128 на белом: самый тёмный пиксель %d, ждали около 127", min)
	}
}

// Контексты-обёртки пробрасывают поворот с учётом своего сдвига.
func TestDrawTextRotated_WrappersForward(t *testing.T) {
	black := color.RGBA{0, 0, 0, 255}
	direct := rotatedTextCanvas(200, 200, 1)
	direct.DrawTextRotated("Hello", 80, 150, 12, "", 90, black)

	off := rotatedTextCanvas(200, 200, 1)
	var ctx widget.DrawContext = off
	ctx = widget.OffsetContext(ctx, 30, 20)
	rt, ok := ctx.(widget.RotatedTextDrawer)
	if !ok || !rt.DrawTextRotated("Hello", 110, 170, 12, "", 90, black) {
		t.Fatal("OffsetContext должен пробрасывать повёрнутый текст")
	}
	if string(direct.back.Pix) != string(off.back.Pix) {
		t.Error("через OffsetContext(30, 20) результат отличается от прямого")
	}

	// translatingContext: попап с началом координат (dx, dy).
	pop := rotatedTextCanvas(200, 200, 1)
	tc := &translatingContext{inner: pop, dx: 30, dy: 20}
	if !tc.DrawTextRotated("Hello", 110, 170, 12, "", 90, black) {
		t.Fatal("translatingContext не нарисовал")
	}
	if string(direct.back.Pix) != string(pop.back.Pix) {
		t.Error("через translatingContext результат отличается от прямого")
	}
}

// Контекст без поворота: обёртка честно говорит «false».
type noRotateCtx struct{ widget.DrawContext }

func TestDrawTextRotated_WrapperOverPlainContextReportsFalse(t *testing.T) {
	c := rotatedTextCanvas(50, 50, 1)
	w := widget.OffsetContext(noRotateCtx{c}, 5, 5)
	rt, ok := w.(widget.RotatedTextDrawer)
	if !ok {
		t.Fatal("обёртка скрывает интерфейс")
	}
	if rt.DrawTextRotated("x", 10, 10, 12, "", 90, color.RGBA{A: 255}) {
		t.Error("внутренний контекст не умеет поворот: ждали false")
	}
}
