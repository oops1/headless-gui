package text

import (
	"os"
	"testing"
)

const tqBenchRunes = "oops-hp.localПодключениеTandemWinLine"

func benchGlyphRaster(b *testing.B, q TextQuality) {
	data, err := os.ReadFile("../../assets/fonts/OpenSans-Regular.ttf")
	if err != nil {
		b.Skip(err)
	}
	fc := NewFontCacheFromData(data, DefaultDPI)
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

func BenchmarkGlyphRasterLegacy(b *testing.B) { benchGlyphRaster(b, TextQuality{}) }

func BenchmarkGlyphRasterHinting(b *testing.B) { benchGlyphRaster(b, TextQuality{Hinting: true}) }

func BenchmarkGlyphRasterHintingLCD(b *testing.B) {
	benchGlyphRaster(b, TextQuality{Hinting: true, LCD: true})
}
