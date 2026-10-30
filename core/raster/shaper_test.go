package raster

import (
	hgtext "github.com/oops1/headless-gui/v4/core/text"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// newShapingCanvas — движок с системными fallback-шрифтами (arial/DejaVu
// подхватываются автоматически, см. systemFallbackFontPaths).
func newShapingCanvas(t *testing.T, probe rune) *Canvas {
	t.Helper()
	c := engineCanvas(400, 100)
	if _, found := c.fcForRune(c.fontCache, probe); !found {
		t.Skipf("нет шрифта с покрытием %q — пропуск", probe)
	}
	return c
}

func TestNeedsShaping(t *testing.T) {
	testreset.Globals(t)
	for _, s := range []string{"Hello", "Привет 123", "Ünïcödé", "καλημέρα"} {
		if hgtext.NeedsShaping(s) {
			t.Errorf("needsShaping(%q) = true, want false", s)
		}
	}
	for _, s := range []string{"שלום", "مرحبا", "नमस्ते", "สวัสดี", "abc‏def", "é"} {
		if !hgtext.NeedsShaping(s) {
			t.Errorf("needsShaping(%q) = false, want true", s)
		}
	}
}

// Лам-алеф «لا» обязан слиться в лигатуру: глифов меньше, чем рун,
// и ширина меньше суммы ширин изолированных букв.
func TestShaping_ArabicLigature(t *testing.T) {
	testreset.Globals(t)
	c := newShapingCanvas(t, 'ل')
	l := c.shaper.Layout(c.fontCache, c.fallbacks, "لا", 14)
	if l == nil {
		t.Fatal("layout == nil (шейпинг недоступен)")
	}
	if len(l.Glyphs) >= 2 {
		t.Errorf("лам-алеф: %d глифов, ожидалась лигатура (<2)", len(l.Glyphs))
	}
	wLam := c.measureWithFallback(c.fontCache, "ل", 14)
	wAlef := c.measureWithFallback(c.fontCache, "ا", 14)
	wLig := l.Width.Round()
	if wLig <= 0 || wLig >= wLam+wAlef {
		t.Errorf("ширина лигатуры %d, ожидалась в (0, %d)", wLig, wLam+wAlef)
	}
}

// Иврит: первый ВИДИМЫЙ (левый) глиф — последняя логическая руна (RTL).
func TestShaping_RTLVisualOrder(t *testing.T) {
	testreset.Globals(t)
	c := newShapingCanvas(t, 'א')
	l := c.shaper.Layout(c.fontCache, c.fallbacks, "אב", 14)
	if l == nil {
		t.Fatal("layout == nil")
	}
	if len(l.Glyphs) != 2 {
		t.Fatalf("ожидалось 2 глифа, got %d", len(l.Glyphs))
	}
	if l.Glyphs[0].Cluster != 1 || l.Glyphs[1].Cluster != 0 {
		t.Errorf("визуальный порядок кластеров %d,%d; ожидался 1,0 (RTL)",
			l.Glyphs[0].Cluster, l.Glyphs[1].Cluster)
	}
}

// Отрисовка арабского текста реально оставляет пиксели на холсте.
func TestShaping_DrawArabicPixels(t *testing.T) {
	testreset.Globals(t)
	c := newShapingCanvas(t, 'م')
	c.BlitBackground()
	c.DrawTextSize("مرحبا بالعالم", 10, 30, 16, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	lit := 0
	for i := 0; i < len(c.back.Pix); i += 4 {
		if c.back.Pix[i] > 0 {
			lit++
		}
	}
	if lit < 50 {
		t.Errorf("после отрисовки арабской строки закрашено %d пикселей — слишком мало", lit)
	}
}

// Смешанный LTR+RTL: layout строится, ширина согласована с measure,
// позиции рун монотонны в логическом порядке.
func TestShaping_MixedBidi(t *testing.T) {
	testreset.Globals(t)
	c := newShapingCanvas(t, 'ש')
	text := "id: שלום!"
	w := c.measureWithFallback(c.fontCache, text, 14)
	if w <= 0 {
		t.Fatalf("ширина %d", w)
	}
	pos := c.MeasureRunePositions(text, 14)
	runeCount := len([]rune(text))
	if len(pos) != runeCount+1 {
		t.Fatalf("позиций %d, ожидалось %d", len(pos), runeCount+1)
	}
	for i := 1; i < len(pos); i++ {
		if pos[i] < pos[i-1] {
			t.Fatalf("логические позиции не монотонны: pos[%d]=%d < pos[%d]=%d", i, pos[i], i-1, pos[i-1])
		}
	}
	if got := pos[runeCount]; got != w {
		// допускаем ±1 px на округление кластеров
		if got < w-1 || got > w+1 {
			t.Errorf("последняя позиция %d != ширина %d", got, w)
		}
	}
}

// Кириллица/латиница НЕ должны ходить через шейпер (гибридный контракт).
func TestShaping_SimpleTextBypasses(t *testing.T) {
	testreset.Globals(t)
	c := engineCanvas(200, 50)
	c.DrawTextSize("Привет World", 5, 5, 12, color.RGBA{A: 255, R: 200, G: 200, B: 200})
	n, _ := c.shaper.CacheSizes()
	if n != 0 {
		t.Errorf("простой текст попал в шейпер: %d layout(ов) в кэше", n)
	}
}

// Повторная отрисовка берёт layout из кэша (не растёт).
func TestShaping_LayoutCached(t *testing.T) {
	testreset.Globals(t)
	c := newShapingCanvas(t, 'م')
	col := color.RGBA{A: 255, R: 255}
	for i := 0; i < 5; i++ {
		c.DrawTextSize("مرحبا", 10, 30, 14, col)
	}
	n, _ := c.shaper.CacheSizes()
	if n != 1 {
		t.Errorf("в кэше %d layout(ов), ожидался 1", n)
	}
}

// BenchmarkDrawTextShaped — отрисовка арабской строки (layout и маски в кэше).
func BenchmarkDrawTextShaped(b *testing.B) {
	c := engineCanvas(640, 100)
	if _, found := c.fcForRune(c.fontCache, 'م'); !found {
		b.Skip("нет шрифта с арабским покрытием")
	}
	col := color.RGBA{A: 255, R: 255, G: 255, B: 255}
	c.DrawTextSize("مرحبا بالعالم", 10, 30, 14, col) // прогрев кэшей
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.DrawTextSize("مرحبا بالعالم", 10, 30, 14, col)
	}
}

// BenchmarkShapeUncached — полный цикл шейпинга без кэша layout'ов.
func BenchmarkShapeUncached(b *testing.B) {
	c := engineCanvas(640, 100)
	if _, found := c.fcForRune(c.fontCache, 'م'); !found {
		b.Skip("нет шрифта с арабским покрытием")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.shaper.DropLayouts() // сброс кэша — измеряем сам шейпинг
		c.shaper.Layout(c.fontCache, c.fallbacks, "مرحبا بالعالم", 14)
	}
}
