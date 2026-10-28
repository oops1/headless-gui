package engine

import (
	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"image/color"
	"os"
	"testing"
)

// Растеризация глифа без кэша (промах кэша масок): прежний путь opentype,
// хинтинг, хинтинг+LCD. Кэш глифов в бою спасает, но холодный старт, смена
// DPI и новые кегли идут именно этим путём.

// Отрисовка строки с кэшем масок: цена слоя на кадре (таблица контраста, LCD).
func benchDrawLine(b *testing.B, q TextQuality) {
	data, err := os.ReadFile("../assets/fonts/OpenSans-Regular.ttf")
	if err != nil {
		b.Skip(err)
	}
	fc := hgtext.NewFontCacheFromData(data, DefaultDPI)
	fc.SetQuality(q)
	c := tqRender(fc, 12, 20, tqLines...) // прогрев кэша
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j, s := range tqLines {
			c.DrawTextSize(s, 4, 4+j*22, 9, color.RGBA{20, 20, 20, 255})
		}
	}
}

func BenchmarkDrawLineLegacy(b *testing.B) { benchDrawLine(b, TextQuality{}) }
func BenchmarkDrawLineContrast(b *testing.B) {
	benchDrawLine(b, TextQuality{Hinting: true, Contrast: DefaultTextContrast})
}
func BenchmarkDrawLineLCD(b *testing.B) {
	benchDrawLine(b, TextQuality{Hinting: true, Contrast: DefaultTextContrast, LCD: true})
}
