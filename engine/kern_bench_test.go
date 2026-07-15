package engine

import "testing"

// kernPairs — представительный набор пар для бенчмарка кернинга (латиница,
// повторяющиеся в UI-строках).
var kernPairs = []struct{ a, b rune }{
	{'A', 'V'}, {'V', 'A'}, {'T', 'o'}, {'W', 'a'}, {'Y', 'o'}, {'r', 'n'},
	{'e', 'l'}, {'l', 'o'}, {'o', ' '}, {' ', 'W'}, {'P', '.'}, {'F', 'a'},
}

// BenchmarkKernCached — fc.Kern с кэшем пар (текущая реализация).
func BenchmarkKernCached(b *testing.B) {
	fc := New(10, 10, 20).canvas.fontCache // шрифт по умолчанию движка
	for _, p := range kernPairs {               // прогрев кэша
		fc.Kern(12, p.a, p.b)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range kernPairs {
			fc.Kern(12, p.a, p.b)
		}
	}
}

// BenchmarkKernUncached — прямой face.Kern (обращение в sfnt на каждую пару,
// поведение ДО кэша).
func BenchmarkKernUncached(b *testing.B) {
	fc := New(10, 10, 20).canvas.fontCache
	face := fc.Face(12)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range kernPairs {
			face.Kern(p.a, p.b)
		}
	}
}
