package engine

import (
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestAA_VisualSample рендерит сцену с AA-примитивами в PNG для ручной
// проверки (путь в env HEADLESS_GUI_AA_PNG; без него тест пропускается).
func TestAA_VisualSample(t *testing.T) {
	out := os.Getenv("HEADLESS_GUI_AA_PNG")
	if out == "" {
		t.Skip("HEADLESS_GUI_AA_PNG не задан")
	}
	eng := New(560, 360, 20)
	c := eng.canvas
	c.blitBackground()

	accent := color.RGBA{R: 0, G: 120, B: 212, A: 255}
	white := color.RGBA{R: 235, G: 235, B: 235, A: 255}
	green := color.RGBA{R: 90, G: 200, B: 120, A: 255}
	orange := color.RGBA{R: 240, G: 150, B: 60, A: 255}

	// Скруглённые прямоугольники разных радиусов (кнопки/панели).
	c.DrawTextSize("FillRoundRect / DrawRoundBorder:", 16, 12, 12, white)
	for i, r := range []int{4, 8, 14} {
		x := 16 + i*120
		c.FillRoundRect(x, 36, 100, 36, r, accent)
		c.DrawRoundBorder(x, 90, 100, 36, r, white)
	}

	// Эллипсы: заливка + контуры разной толщины.
	c.DrawTextSize("Ellipse fill/stroke:", 16, 140, 12, white)
	c.FillEllipseAA(60, 195, 40, 28, green)
	c.StrokeEllipseAA(160, 195, 40, 28, 1, white)
	c.StrokeEllipseAA(260, 195, 40, 28, 4, orange)
	c.FillEllipseAA(340, 195, 22, 22, accent) // круг (RadioButton-подобный)
	c.StrokeEllipseAA(340, 195, 22, 22, 1.2, white)

	// Линии под разными углами и толщинами.
	c.DrawTextSize("DrawLineAA:", 16, 236, 12, white)
	for i := 0; i <= 6; i++ {
		c.DrawLineAA(16+i*10, 310, 76+i*20, 258, 1+float64(i)*0.5, white)
	}

	// Полигон-звезда и ломаная-галочка.
	star := []image.Point{
		{X: 320, Y: 258}, {X: 335, Y: 292}, {X: 372, Y: 294},
		{X: 344, Y: 316}, {X: 355, Y: 350}, {X: 320, Y: 330},
		{X: 285, Y: 350}, {X: 296, Y: 316}, {X: 268, Y: 294}, {X: 305, Y: 292},
	}
	c.FillPolygonAA(star, orange)
	c.StrokePolylineAA([]image.Point{
		{X: 420, Y: 300}, {X: 445, Y: 330}, {X: 500, Y: 265},
	}, 4, false, green)

	// Ступенчатый фолбэк для сравнения — legacy-путь (полупрозрачный цвет).
	semi := color.RGBA{R: 0, G: 120, B: 212, A: 254}
	c.FillRoundRect(400, 36, 100, 36, 14, semi)

	savePNG(c.back, out)
	t.Logf("сцена сохранена: %s", out)
	_ = widget.DefaultFontSizePt // прижать импорт (сцена не использует виджеты напрямую)
}
