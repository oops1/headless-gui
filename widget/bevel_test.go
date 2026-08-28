package widget

import (
	"image"
	"image/color"
	"testing"
)

// Рамка Windows 2000 рисуется одинаково, откуда бы её ни попросили.
//
// Реализаций было две — DrawBevel для панели задач и drawBevelRaised/Sunken
// для виджетов, — и они разошлись: панель рисовала внутреннюю грань с обеих
// сторон, кнопки с одной. Рядом на экране это видно: у кнопки «Пуск» рамка
// выходила толще, чем у кнопки в окне.

// lineRec — контекст, записывающий нарисованные линии.
type lineRec struct {
	DrawContext
	lines []recLine
}

type recLine struct {
	horiz      bool
	x, y, span int
	col        color.RGBA
}

func (c *lineRec) DrawHLine(x, y, w int, col color.RGBA) {
	c.lines = append(c.lines, recLine{horiz: true, x: x, y: y, span: w, col: col})
}

func (c *lineRec) DrawVLine(x, y, h int, col color.RGBA) {
	c.lines = append(c.lines, recLine{x: x, y: y, span: h, col: col})
}

func bevelStyle() ThemeStyle {
	return ThemeStyle{
		BevelLight:  color.RGBA{R: 255, G: 255, B: 255, A: 255},
		BevelShadow: color.RGBA{R: 128, G: 128, B: 128, A: 255},
		BevelDark:   color.RGBA{A: 255},
	}
}

func sameLines(a, b []recLine) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBevel_WidgetAndThemePathsAgree(t *testing.T) {
	st := bevelStyle()
	r := image.Rect(10, 20, 90, 60)

	for _, tc := range []struct {
		name   string
		sunken bool
		draw   func(ctx DrawContext)
	}{
		{"выпуклая", false, func(ctx DrawContext) {
			drawBevelRaised(ctx, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), st)
		}},
		{"утопленная", true, func(ctx DrawContext) {
			drawBevelSunken(ctx, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), st)
		}},
	} {
		viaWidget := &lineRec{}
		tc.draw(viaWidget)

		viaTheme := &lineRec{}
		DrawBevel(viaTheme, r, st.BevelLight, st.BevelShadow, st.BevelDark, tc.sunken)

		if !sameLines(viaWidget.lines, viaTheme.lines) {
			t.Errorf("%s рамка: виджет рисует %v, тема — %v",
				tc.name, viaWidget.lines, viaTheme.lines)
		}
	}
}

// Внутренняя грань — только с одной стороны, как в настоящей Windows: с
// другой наружу смотрит лицо элемента. Грань с обеих сторон делает рамку
// двухпиксельной по всему периметру.
func TestBevel_InnerEdgeOnOneSideOnly(t *testing.T) {
	st := bevelStyle()
	r := image.Rect(0, 0, 40, 30)

	for _, tc := range []struct {
		name     string
		sunken   bool
		wantCol  color.RGBA
		wantSide string
	}{
		{"выпуклая", false, st.BevelShadow, "низ/право"},
		{"утопленная", true, st.BevelDark, "верх/лево"},
	} {
		ctx := &lineRec{}
		DrawBevel(ctx, r, st.BevelLight, st.BevelShadow, st.BevelDark, tc.sunken)

		// Четыре линии внешнего кольца плюс две внутренние — не четыре.
		if len(ctx.lines) != 6 {
			t.Errorf("%s рамка: %d линий, ждали 6 (кольцо и одна внутренняя грань)",
				tc.name, len(ctx.lines))
			continue
		}
		inner := ctx.lines[4:]
		for _, l := range inner {
			if l.col != tc.wantCol {
				t.Errorf("%s рамка: внутренняя грань цвета %v, ждали %v (%s)",
					tc.name, l.col, tc.wantCol, tc.wantSide)
			}
		}
	}
}

// Рамка, в которую внутренняя грань не помещается, рисует только кольцо —
// и не выходит за свои границы.
func TestBevel_TinyRectStaysInside(t *testing.T) {
	st := bevelStyle()
	r := image.Rect(5, 5, 8, 8) // 3x3: кольцо занимает всё

	ctx := &lineRec{}
	DrawBevel(ctx, r, st.BevelLight, st.BevelShadow, st.BevelDark, false)
	if len(ctx.lines) != 4 {
		t.Fatalf("на рамке 3x3 нарисовано %d линий, ждали 4", len(ctx.lines))
	}
	for _, l := range ctx.lines {
		end := l.x + l.span
		if !l.horiz {
			end = l.y + l.span
		}
		if l.x < r.Min.X || l.y < r.Min.Y || (l.horiz && end > r.Max.X) || (!l.horiz && end > r.Max.Y) {
			t.Errorf("линия %v вышла за границы %v", l, r)
		}
	}

	// Совсем вырожденная рамка не рисует ничего.
	ctx = &lineRec{}
	DrawBevel(ctx, image.Rect(0, 0, 1, 10), st.BevelLight, st.BevelShadow, st.BevelDark, false)
	if len(ctx.lines) != 0 {
		t.Errorf("рамка шириной в точку нарисовала %d линий", len(ctx.lines))
	}
}
