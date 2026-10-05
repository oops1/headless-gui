package desktop

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// fontCtx запоминает, каким именем шрифта и каким кеглем рисовали и мерили.
type fontCtx struct {
	recCtx
	drawn    []string
	drawSize []float64
	meas     []string
	measSize []float64
}

func (c *fontCtx) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	c.drawn = append(c.drawn, fontName)
	c.drawSize = append(c.drawSize, sizePt)
}

func (c *fontCtx) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.drawn = append(c.drawn, "")
	c.drawSize = append(c.drawSize, sizePt)
}

func (c *fontCtx) MeasureTextFont(text string, sizePt float64, fontName string) int {
	c.meas = append(c.meas, fontName)
	c.measSize = append(c.measSize, sizePt)
	return len([]rune(text)) * testCharW
}

// Вес и наклон из стиля доходят до движка составным именем. Раньше в
// DrawTextFont уходило одно Family: Bold и Italic терялись, и жирный
// заголовок темы рисовался обычным.
func TestFontFaceName(t *testing.T) {
	cases := []struct {
		name string
		f    theme.FontSpec
		want string
	}{
		{"пусто", theme.FontSpec{}, ""},
		{"размер без семейства", theme.FontSpec{Size: 9}, ""},
		{"обычное начертание — просто семейство", theme.FontSpec{Family: "Open Sans", Size: 8.5}, "Open Sans"},
		{"bold без семейства", theme.FontSpec{Bold: true}, widget.FontFace("", 700, false)},
		{"bold с семейством", theme.FontSpec{Family: "Open Sans", Bold: true}, widget.FontFace("Open Sans", 700, false)},
		{"semibold", theme.FontSpec{Family: "Open Sans", Weight: theme.WeightSemiBold}, widget.FontFace("Open Sans", 600, false)},
		{"weight важнее bold", theme.FontSpec{Family: "Open Sans", Bold: true, Weight: theme.WeightLight}, widget.FontFace("Open Sans", 300, false)},
		{"курсив", theme.FontSpec{Family: "Open Sans", Italic: true}, widget.FontFace("Open Sans", 0, true)},
	}
	for _, c := range cases {
		if got := FontFaceName(c.f); got != c.want {
			t.Errorf("%s: FontFaceName = %q, ждали %q", c.name, got, c.want)
		}
	}
	// Составное имя обратимо: по нему движок восстанавливает запрос.
	fam, w, it, ok := widget.ParseFontFace(FontFaceName(theme.FontSpec{Family: "Open Sans", Weight: 600, Italic: true}))
	if !ok || fam != "Open Sans" || w != 600 || !it {
		t.Errorf("ParseFontFace: %q %d %v %v", fam, w, it, ok)
	}
}

// Дробный кегль темы (8.5) доходит до контекста рисования целым, как есть, —
// без округления на пути от стиля до DrawTextFont и MeasureTextFont.
func TestDrawText_FractionalSizeAndWeightReachContext(t *testing.T) {
	ctx := &fontCtx{}
	s := &theme.Style{Font: theme.FontSpec{Family: "Open Sans", Size: 8.5, Weight: theme.WeightSemiBold}}
	r := image.Rect(0, 0, 200, 40)

	DrawTextCentered(ctx, r, "Пуск", s)
	DrawTextLeft(ctx, r, "Пуск", s)
	MeasureText(ctx, "Пуск", s)

	if len(ctx.drawn) != 2 {
		t.Fatalf("рисований %d, ждали 2", len(ctx.drawn))
	}
	want := widget.FontFace("Open Sans", 600, false)
	for i, n := range ctx.drawn {
		if n != want {
			t.Errorf("рисование %d: шрифт %q, ждали %q", i, n, want)
		}
		if ctx.drawSize[i] != 8.5 {
			t.Errorf("рисование %d: кегль %v, ждали 8.5", i, ctx.drawSize[i])
		}
	}
	for i, sz := range ctx.measSize {
		if sz != 8.5 {
			t.Errorf("измерение %d: кегль %v, ждали 8.5", i, sz)
		}
		if ctx.meas[i] != want {
			t.Errorf("измерение %d: шрифт %q, ждали %q", i, ctx.meas[i], want)
		}
	}
}

// Стиль без Family и без веса идёт прежней дорогой — DrawTextSize: вид
// существующих тем не должен зависеть от появления весов.
func TestDrawText_PlainStyleKeepsOldPath(t *testing.T) {
	ctx := &fontCtx{}
	s := &theme.Style{Font: theme.FontSpec{Size: 9}}
	DrawTextLeft(ctx, image.Rect(0, 0, 100, 30), "ab", s)
	if len(ctx.drawn) != 1 || ctx.drawn[0] != "" {
		t.Errorf("обычный стиль ушёл не в DrawTextSize: %q", ctx.drawn)
	}
}
