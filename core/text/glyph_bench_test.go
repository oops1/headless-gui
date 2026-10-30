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
	sh := fc.font.shard(9)
	for i := 0; i < b.N; i++ {
		// Каждый проход — промах кэша масок на обоих уровнях; face и зоны
		// шарда остаются (меряется растеризация, а не их построение).
		fc.mu.Lock()
		fc.glyphs = nil
		fc.mu.Unlock()
		sh.dropGlyphsForBench()
		for _, r := range runes {
			fc.GlyphAt(9, r, 0)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(runes)), "ns/glyph")
}

// dropGlyphsForBench сбрасывает маски шарда, снимая их с учёта байт.
func (s *shard) dropGlyphsForBench() {
	var n int64
	s.mu.Lock()
	for _, g := range s.glyphs {
		n += glyphCost(g)
	}
	s.glyphs = nil
	s.bytes -= n
	s.mu.Unlock()
	store.bytes.Add(-n)
}

func BenchmarkGlyphRasterLegacy(b *testing.B) { benchGlyphRaster(b, TextQuality{}) }

func BenchmarkGlyphRasterHinting(b *testing.B) { benchGlyphRaster(b, TextQuality{Hinting: true}) }

func BenchmarkGlyphRasterHintingLCD(b *testing.B) {
	benchGlyphRaster(b, TextQuality{Hinting: true, LCD: true})
}
