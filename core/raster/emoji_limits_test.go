package raster

import (
	hgtext "github.com/oops1/headless-gui/v4/core/text"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Повторная отрисовка эмодзи тем же цветом не растит кэши цветных глифов.
func TestEmoji_ColorGlyphCacheStable(t *testing.T) {
	testreset.Globals(t)
	c := engineCanvas(120, 80)
	if _, found := c.fcForRune(c.fontCache, '🙂'); !found {
		t.Skip("нет системного шрифта с цветными эмодзи — пропуск")
	}
	col := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	c.BlitBackground()
	c.DrawTextSize("🙂", 8, 8, 32, col)

	_, n1 := c.shaper.CacheSizes()
	if n1 == 0 {
		t.Skip("цветной глиф не построен — нечего кэшировать")
	}

	for i := 0; i < 5; i++ {
		c.DrawTextSize("🙂", 8, 8, 32, col)
	}
	_, n2 := c.shaper.CacheSizes()
	if n2 != n1 {
		t.Errorf("после повторных отрисовок в кэше %d записей, было %d", n2, n1)
	}
}

// Смена DPI сразу меняет ширину сложного текста: раскладки сброшены.
func TestShaping_SetDPIDropsLayouts(t *testing.T) {
	testreset.Globals(t)
	c := newShapingCanvas(t, 'م')
	const text = "مرحبا"
	w1 := c.measureWithFallback(c.fontCache, text, 14)
	if w1 <= 0 {
		t.Fatalf("ширина при DPI %v = %d", hgtext.DefaultDPI, w1)
	}
	c.SetDPIAll(hgtext.DefaultDPI * 2)
	w2 := c.measureWithFallback(c.fontCache, text, 14)
	if w2 <= w1 {
		t.Errorf("после удвоения DPI ширина %d, была %d — раскладка не пересчитана", w2, w1)
	}
	c.SetDPIAll(hgtext.DefaultDPI)
	if w3 := c.measureWithFallback(c.fontCache, text, 14); w3 != w1 {
		t.Errorf("возврат к DPI %v дал ширину %d, ожидалась %d", hgtext.DefaultDPI, w3, w1)
	}
}
