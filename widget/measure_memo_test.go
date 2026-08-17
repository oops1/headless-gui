package widget

import (
	"strings"
	"testing"
)

// Мемоизация возвращает те же ширины, что и измеритель.
func TestMeasureUIText_MemoMatchesMeasurer(t *testing.T) {
	useTestMeasurer(t)
	for _, s := range []string{"", "a", "слово тест", strings.Repeat("ii ", 40)} {
		want := rawMeasure(s, DefaultFontSizePt)
		for i := 0; i < 3; i++ {
			if got := MeasureUIText(s, DefaultFontSizePt); got != want {
				t.Fatalf("%q: %d, want %d", s, got, want)
			}
		}
		if got := measureUIBytes([]byte(s), DefaultFontSizePt); got != want {
			t.Fatalf("%q по байтам: %d, want %d", s, got, want)
		}
	}
}

// Смена ревизии метрик сбрасывает кэш.
func TestMeasureUIText_MemoResetOnRev(t *testing.T) {
	useTestMeasurer(t)
	const s = "мама мыла раму"
	first := MeasureUIText(s, DefaultFontSizePt)

	SetTextMeasurer(func(text string, sizePt float64) int { return 1234 })
	if got := MeasureUIText(s, DefaultFontSizePt); got != 1234 {
		t.Fatalf("после смены измерителя %d (было %d)", got, first)
	}
}

// Переполнение кэша не ломает замеры.
func TestMeasureUIText_MemoOverflow(t *testing.T) {
	useTestMeasurer(t)
	for i := 0; i < measureMemoMax+100; i++ {
		s := strings.Repeat("a", i%50) + string(rune('a'+i%26)) + itoaTest(i)
		if got, want := MeasureUIText(s, DefaultFontSizePt), rawMeasure(s, DefaultFontSizePt); got != want {
			t.Fatalf("i=%d: %d, want %d", i, got, want)
		}
	}
	measureMu.Lock()
	n := measureCount
	measureMu.Unlock()
	if n > measureMemoMax {
		t.Fatalf("кэш вырос до %d записей", n)
	}
}

func itoaTest(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// Перенос по словам не изменился после ухода от конкатенации.
func TestWrapTextPx_Boundaries(t *testing.T) {
	useTestMeasurer(t)
	cases := []struct {
		text string
		maxW int
	}{
		{"короткая строка", 500},
		{"мама мыла раму и ещё немного текста для переноса", 120},
		{"одно\nдва слова\n\nпусто", 60},
		{"", 100},
		{"оченьдлинноесловобезпробелов", 40},
	}
	for _, c := range cases {
		got := wrapTextPx(c.text, c.maxW, DefaultFontSizePt)
		want := refWrapTextPx(c.text, c.maxW, DefaultFontSizePt)
		if len(got) != len(want) {
			t.Fatalf("%q: строк %d, эталон %d (%q / %q)", c.text, len(got), len(want), got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%q: строка %d = %q, эталон %q", c.text, i, got[i], want[i])
			}
		}
	}
}

// refWrapTextPx — прежний перенос через конкатенацию строк.
func refWrapTextPx(text string, maxW int, sizePt float64) []string {
	var result []string
	for _, paragraph := range splitLines(text) {
		words := fieldsSpace(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if rawMeasure(line+" "+w, sizePt) > maxW {
				result = append(result, line)
				line = w
			} else {
				line += " " + w
			}
		}
		result = append(result, line)
	}
	if len(result) == 0 {
		result = []string{""}
	}
	return result
}

// Перенос длинного текста (текущий вариант).
func BenchmarkWrapTextPx(b *testing.B) {
	useTestMeasurer(b)
	text := strings.TrimSpace(strings.Repeat("мама мыла раму ", 100))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapTextPx(text, 300, DefaultFontSizePt)
	}
}

// Тот же текст прежним алгоритмом (конкатенация, без кэша).
func BenchmarkWrapTextPxRef(b *testing.B) {
	useTestMeasurer(b)
	text := strings.TrimSpace(strings.Repeat("мама мыла раму ", 100))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		refWrapTextPx(text, 300, DefaultFontSizePt)
	}
}
