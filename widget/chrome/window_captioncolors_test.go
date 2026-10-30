package chrome

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// Цвет значков кнопок заголовка: прежний серый, пока он читается; иначе цвет
// текста; иначе чёрный или белый по контрасту.
func TestCaptionGlyphColor(t *testing.T) {
	black, white := color.RGBA{A: 255}, color.RGBA{R: 255, G: 255, B: 255, A: 255}
	orange := color.RGBA{R: 255, G: 185, B: 0, A: 255}
	for _, tc := range []struct {
		name     string
		text, bg color.RGBA
		want     color.RGBA
	}{
		{"тёмный заголовок — прежний серый", white, color.RGBA{R: 32, G: 32, B: 32, A: 255}, base.CaptionGlyphLegacy},
		{"светлый акцент — цвет текста", black, orange, black},
		{"светлый заголовок, серый текст", color.RGBA{R: 120, G: 120, B: 120, A: 255}, color.RGBA{R: 243, G: 243, B: 243, A: 255}, color.RGBA{R: 120, G: 120, B: 120, A: 255}},
		// Текст сам не читается на своём фоне: чёрный/белый, что контрастнее.
		{"светлый текст на светлом", color.RGBA{R: 240, G: 240, B: 240, A: 255}, color.RGBA{R: 250, G: 250, B: 250, A: 255}, black},
		{"тёмно-синий текст на синем", color.RGBA{G: 60, B: 150, A: 255}, color.RGBA{G: 100, B: 200, A: 255}, white},
	} {
		if got := base.CaptionGlyphColor(tc.text, tc.bg); got != tc.want {
			t.Errorf("%s: значок %v, ждали %v", tc.name, got, tc.want)
		}
	}
}

// Подсветка: плёнка 15 % — чёрная на светлом, белая на тёмном; premultiplied.
func TestCaptionHoverPlate(t *testing.T) {
	light := captionHoverPlate(color.RGBA{R: 255, G: 185, B: 0, A: 255})
	if light != (color.RGBA{A: captionHoverAlpha}) {
		t.Errorf("на светлом заголовке плёнка %v, ждали чёрную", light)
	}
	dark := captionHoverPlate(color.RGBA{R: 32, G: 32, B: 32, A: 255})
	if dark.R != captionHoverAlpha || dark.G != captionHoverAlpha || dark.B != captionHoverAlpha || dark.A != captionHoverAlpha {
		t.Errorf("на тёмном заголовке плёнка %v, ждали белую (premultiplied)", dark)
	}
	if pct := float64(captionHoverAlpha) / 255 * 100; pct < 10 || pct > 20 {
		t.Errorf("доля подсветки %.0f %%, ждали 10–20 %%", pct)
	}
}
