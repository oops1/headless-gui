package widget

import (
	"image"
	"image/color"
	"testing"
)

// Стрелка выпадающего списка — запрос GG-55.
//
// Список рисовал залитый треугольник и отделял его вертикальной чертой: в теме
// со скруглёнными контролами поле выглядело разрезанным надвое, рядом с полем
// ввода той же высоты — как контрол другого семейства.

// arrowCtx запоминает, как нарисована стрелка.
type arrowCtx struct {
	DrawContext
	vlines    int
	polylines int
}

func (c *arrowCtx) DrawVLine(int, int, int, color.RGBA)                     { c.vlines++ }
func (c *arrowCtx) DrawHLine(int, int, int, color.RGBA)                     {}
func (c *arrowCtx) SetPixel(int, int, color.RGBA)                           {}
func (c *arrowCtx) FillRect(int, int, int, int, color.RGBA)                 {}
func (c *arrowCtx) FillRoundRect(int, int, int, int, int, color.RGBA)       {}
func (c *arrowCtx) DrawBorder(int, int, int, int, color.RGBA)               {}
func (c *arrowCtx) DrawRoundBorder(int, int, int, int, int, color.RGBA)     {}
func (c *arrowCtx) DrawTextSize(string, int, int, float64, color.RGBA)      {}
func (c *arrowCtx) MeasureText(s string, _ float64) int                     { return len(s) * 6 }
func (c *arrowCtx) FillRectAlpha(int, int, int, int, color.RGBA)            {}
func (c *arrowCtx) DrawLineAA(int, int, int, int, float64, color.RGBA)      {}
func (c *arrowCtx) FillEllipseAA(int, int, int, int, color.RGBA)            {}
func (c *arrowCtx) StrokeEllipseAA(int, int, int, int, float64, color.RGBA) {}
func (c *arrowCtx) FillPolygonAA([]image.Point, color.RGBA)                 {}
func (c *arrowCtx) StrokePolylineAA([]image.Point, float64, bool, color.RGBA) {
	c.polylines++
}

// drawDropdownIn рисует список в заданной теме и возвращает, что нарисовано.
func drawDropdownIn(t *testing.T, themeName string, style ArrowStyle) *arrowCtx {
	t.Helper()
	saved := win10
	t.Cleanup(func() { win10 = saved })
	ApplyGlobalTheme(ThemeByName(themeName))

	d := NewDropdown("Русский", "English")
	d.ArrowStyle = style
	d.SetBounds(image.Rect(10, 10, 210, 40))
	ctx := &arrowCtx{}
	d.Draw(ctx)
	return ctx
}

// Скруглённая тема по умолчанию рисует шеврон и не режет поле чертой.
func TestDropdownArrow_AutoChevronInRoundedTheme(t *testing.T) {
	for _, name := range []string{"Win11 Light", "Win11 Dark"} {
		ctx := drawDropdownIn(t, name, ArrowAuto)
		if ctx.polylines != 1 {
			t.Errorf("%s: шеврон не нарисован (ломаных %d)", name, ctx.polylines)
		}
		if ctx.vlines != 0 {
			t.Errorf("%s: поле разрезано чертой (%d вертикалей)", name, ctx.vlines)
		}
	}
}

// Плоская тема по умолчанию остаётся прежней: треугольник и черта.
func TestDropdownArrow_AutoTriangleInFlatTheme(t *testing.T) {
	ctx := drawDropdownIn(t, "Win10 Light", ArrowAuto)
	if ctx.polylines != 0 {
		t.Error("в плоской теме вместо треугольника нарисован шеврон")
	}
	if ctx.vlines == 0 {
		t.Error("в плоской теме пропал разделитель перед стрелкой")
	}
}

// Явный выбор сильнее темы — в обе стороны.
func TestDropdownArrow_ExplicitStyleWins(t *testing.T) {
	if ctx := drawDropdownIn(t, "Win10 Light", ArrowChevron); ctx.polylines != 1 || ctx.vlines != 0 {
		t.Errorf("ArrowChevron в плоской теме: ломаных %d, вертикалей %d", ctx.polylines, ctx.vlines)
	}
	if ctx := drawDropdownIn(t, "Win11 Light", ArrowTriangle); ctx.polylines != 0 || ctx.vlines == 0 {
		t.Errorf("ArrowTriangle в скруглённой теме: ломаных %d, вертикалей %d", ctx.polylines, ctx.vlines)
	}
}

// Классика остаётся классикой: выпуклая кнопка со стрелкой — часть образа
// Win9x, шеврону в ней не место при любом выборе.
func TestDropdownArrow_ClassicIgnoresChevron(t *testing.T) {
	ctx := drawDropdownIn(t, "Win2000", ArrowChevron)
	if ctx.polylines != 0 {
		t.Error("в классической теме нарисован шеврон")
	}
}
