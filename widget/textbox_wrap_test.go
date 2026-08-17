package widget

import (
	"image"
	"math/rand"
	"strings"
	"testing"
)

// runeW — ширина руны в тестовом измерителе (неравномерная, как у шрифта).
func runeW(r rune) int {
	switch {
	case r == ' ':
		return 4
	case r == 'i' || r == 'l':
		return 3
	case r >= 'а' && r <= 'я':
		return 8
	default:
		return 5 + int(r)%4
	}
}

// useTestMeasurer ставит детерминированный измеритель на время теста.
func useTestMeasurer(t testing.TB) {
	t.Helper()
	prev := uiTextMeasurer.Load()
	SetTextMeasurer(func(text string, sizePt float64) int {
		w := 0
		for _, r := range text {
			w += runeW(r)
		}
		return int(float64(w) * sizePt / 14)
	})
	t.Cleanup(func() {
		if prev != nil {
			uiTextMeasurer.Store(prev)
		}
		BumpTextMetricsRev()
	})
}

// rawMeasure — зарегистрированный измеритель без мемоизации.
func rawMeasure(text string, sizePt float64) int {
	return uiTextMeasurer.Load().(textMeasurer)(text, sizePt)
}

// refWrapParagraph — прежний посимвольный перенос, эталон для сравнения.
func refWrapParagraph(runes []rune, start, end, maxW int, fs float64, measure func(string, float64) int) []tbLine {
	var lines []tbLine
	if start >= end {
		return append(lines, tbLine{start: start, end: end})
	}
	lineStart := start
	lastSpace := -1
	for i := start; i < end; i++ {
		if runes[i] == ' ' {
			lastSpace = i
		}
		if measure(string(runes[lineStart:i+1]), fs) <= maxW {
			continue
		}
		switch {
		case lastSpace > lineStart:
			lines = append(lines, tbLine{start: lineStart, end: lastSpace})
			lineStart = lastSpace + 1
			lastSpace = -1
		case i > lineStart:
			lines = append(lines, tbLine{start: lineStart, end: i})
			lineStart = i
			lastSpace = -1
		default:
			lines = append(lines, tbLine{start: lineStart, end: i + 1})
			lineStart = i + 1
			lastSpace = -1
		}
	}
	return append(lines, tbLine{start: lineStart, end: end})
}

// refLayout — прежняя раскладка всего текста.
func refLayout(text string, maxW int, fs float64, measure func(string, float64) int) []tbLine {
	runes := []rune(text)
	var lines []tbLine
	n := len(runes)
	parStart := 0
	for i := 0; i <= n; i++ {
		if i < n && runes[i] != '\n' {
			continue
		}
		lines = append(lines, refWrapParagraph(runes, parStart, i, maxW, fs, measure)...)
		parStart = i + 1
	}
	if len(lines) == 0 {
		lines = []tbLine{{}}
	}
	return lines
}

// refColAtX — прежний посимвольный поиск колонки.
func refColAtX(runes []rune, ln tbLine, x int, fs float64) int {
	length := ln.end - ln.start
	prev := 0
	for c := 1; c <= length; c++ {
		w := MeasureUIText(string(runes[ln.start:ln.start+c]), fs)
		if x < (prev+w)/2 {
			return c - 1
		}
		prev = w
	}
	return length
}

// newWrapBox создаёт TextBox с шириной текстовой области ровно maxW.
func newWrapBox(maxW int) *TextBox {
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, maxW+2*tb.PaddingX+tbScrollbarW, 400))
	return tb
}

// Перенос совпадает с прежним посимвольным алгоритмом.
func TestTextBox_WrapMatchesReference(t *testing.T) {
	useTestMeasurer(t)
	rnd := rand.New(rand.NewSource(20260816))
	words := []string{"слово", "a", "ii", "длинноеслово", "мама", "x", "llll",
		"переносимоеслововнутристроки", "0", "тест", "abc"}

	for tc := 0; tc < 200; tc++ {
		var sb strings.Builder
		for w := 0; w < 1+rnd.Intn(60); w++ {
			sb.WriteString(words[rnd.Intn(len(words))])
			switch rnd.Intn(6) {
			case 0:
				sb.WriteString("\n")
			default:
				sb.WriteString(" ")
			}
		}
		text := sb.String()
		maxW := 20 + rnd.Intn(200)

		tb := newWrapBox(maxW)
		tb.SetText(text)
		got := tb.LineCount()
		want := refLayout(text, maxW, tb.fontSize(), MeasureUIText)
		if got != len(want) {
			t.Fatalf("случай %d: строк %d, эталон %d (maxW=%d)", tc, got, len(want), maxW)
		}
		for i, ln := range tb.lines {
			if ln != want[i] {
				t.Fatalf("случай %d: строка %d = %v, эталон %v (maxW=%d, текст %q)",
					tc, i, ln, want[i], maxW, text)
			}
		}
	}
}

// colAtX совпадает с прежним посимвольным поиском.
func TestTextBox_ColAtXMatchesReference(t *testing.T) {
	useTestMeasurer(t)
	tb := newWrapBox(160)
	text := "первая строка текста\nвторая строка с длинным словом переносимоеслово\nтретья"
	tb.SetText(text)
	tb.LineCount()
	runes := []rune(text)
	fs := tb.fontSize()
	for li, ln := range tb.lines {
		for x := -10; x < 220; x += 3 {
			got := tb.colAtX(li, x)
			want := refColAtX(runes, ln, x, fs)
			if got != want {
				t.Fatalf("строка %d, x=%d: col=%d, эталон %d", li, x, got, want)
			}
		}
	}
}

// Смена ревизии метрик пересчитывает раскладку.
func TestTextBox_RelayoutOnMetricsRev(t *testing.T) {
	useTestMeasurer(t)
	tb := newWrapBox(120)
	tb.SetText(strings.Repeat("слово ", 20))
	n1 := tb.LineCount()

	SetTextMeasurer(func(text string, sizePt float64) int {
		w := 0
		for range text {
			w += 30
		}
		return w
	})
	BumpTextMetricsRev()
	if n2 := tb.LineCount(); n2 <= n1 {
		t.Fatalf("после смены метрик строк %d, было %d — раскладка не пересчитана", n2, n1)
	}
}

func benchLayout(b *testing.B, text string, maxW int, fresh func(*TextBox)) {
	b.Helper()
	tb := newWrapBox(maxW)
	tb.SetText(text)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fresh(tb)
		tb.LineCount()
	}
}

// Раскладка абзаца в 2000 рун (текущий алгоритм).
func BenchmarkTextBoxLayout2000(b *testing.B) {
	useTestMeasurer(b)
	text := strings.TrimSpace(strings.Repeat("слово тест ", 200))
	benchLayout(b, text, 400, func(tb *TextBox) {
		tb.mu.Lock()
		tb.dirty = true
		tb.mu.Unlock()
	})
}

// То же, но без попаданий в кэш замеров (метрики меняются каждый раз).
func BenchmarkTextBoxLayout2000Cold(b *testing.B) {
	useTestMeasurer(b)
	text := strings.TrimSpace(strings.Repeat("слово тест ", 200))
	benchLayout(b, text, 400, func(tb *TextBox) {
		BumpTextMetricsRev()
		tb.mu.Lock()
		tb.dirty = true
		tb.mu.Unlock()
	})
}

// Раскладка того же абзаца прежним посимвольным алгоритмом.
func BenchmarkTextBoxLayout2000Ref(b *testing.B) {
	useTestMeasurer(b)
	text := strings.TrimSpace(strings.Repeat("слово тест ", 200))
	tb := newWrapBox(400)
	fs := tb.fontSize()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		refLayout(text, 400, fs, rawMeasure)
	}
}
