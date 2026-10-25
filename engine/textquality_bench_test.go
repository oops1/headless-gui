package engine

import (
	"image/color"
	"os"
	"testing"
)

// Растеризация глифа без кэша (промах кэша масок): прежний путь opentype,
// хинтинг, хинтинг+LCD. Кэш глифов в бою спасает, но холодный старт, смена
// DPI и новые кегли идут именно этим путём.

const tqBenchRunes = "oops-hp.localПодключениеTandemWinLine"

func benchGlyphRaster(b *testing.B, q TextQuality) {
	data, err := os.ReadFile("../assets/fonts/OpenSans-Regular.ttf")
	if err != nil {
		b.Skip(err)
	}
	fc := newFontCacheFromData(data, DefaultDPI)
	fc.SetQuality(q)
	runes := []rune(tqBenchRunes)
	fc.GlyphAt(9, 'o', 0) // прогрев: face и зоны
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fc.mu.Lock()
		fc.glyphs = nil // каждый проход — промах кэша масок
		fc.mu.Unlock()
		for _, r := range runes {
			fc.GlyphAt(9, r, 0)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(runes)), "ns/glyph")
}

func BenchmarkGlyphRasterLegacy(b *testing.B)  { benchGlyphRaster(b, TextQuality{}) }
func BenchmarkGlyphRasterHinting(b *testing.B) { benchGlyphRaster(b, TextQuality{Hinting: true}) }
func BenchmarkGlyphRasterHintingLCD(b *testing.B) {
	benchGlyphRaster(b, TextQuality{Hinting: true, LCD: true})
}

// Отрисовка строки с кэшем масок: цена слоя на кадре (таблица контраста, LCD).
func benchDrawLine(b *testing.B, q TextQuality) {
	data, err := os.ReadFile("../assets/fonts/OpenSans-Regular.ttf")
	if err != nil {
		b.Skip(err)
	}
	fc := newFontCacheFromData(data, DefaultDPI)
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
