package engine

import (
	"image/color"
	"os"
	"testing"
)

// TestShaping_VisualSample рендерит мультиязычную сцену в PNG для ручной
// проверки (путь в env HEADLESS_GUI_SHAPER_PNG; без него тест пропускается).
func TestShaping_VisualSample(t *testing.T) {
	out := os.Getenv("HEADLESS_GUI_SHAPER_PNG")
	if out == "" {
		t.Skip("HEADLESS_GUI_SHAPER_PNG не задан")
	}
	eng := New(560, 320, 20)
	c := eng.canvas
	c.blitBackground()

	white := color.RGBA{R: 235, G: 235, B: 235, A: 255}
	green := color.RGBA{R: 120, G: 220, B: 120, A: 255}

	lines := []string{
		"Latin: Hello, world! Кириллица: Привет!",
		"Arabic: مرحبا بالعالم — لا إله إلا الله",
		"Hebrew: שלום עולם",
		"Devanagari: नमस्ते दुनिया",
		"Thai: สวัสดีชาวโลก",
		"Mixed: User أحمد (role: מנהל) — ok",
	}
	y := 20
	for _, s := range lines {
		c.DrawTextSize(s, 16, y, 16, white)
		w := c.MeasureText(s, 16)
		c.DrawHLine(16, y+24, w, green) // подчёркивание = измеренная ширина
		y += 46
	}

	savePNG(c.back, out)
	t.Logf("сцена сохранена: %s", out)
}
