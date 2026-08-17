package tests

import (
	"image"
	"math/rand"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// tbScrollbarW — зона скроллбара TextBox (widget/textbox.go).
const tbScrollbarW = 7

// refWrap — прежний посимвольный перенос абзаца, эталон границ строк.
func refWrap(runes []rune, start, end, maxW int, fs float64) []int {
	if start >= end {
		return []int{start}
	}
	starts := []int{start}
	lineStart := start
	lastSpace := -1
	for i := start; i < end; i++ {
		if runes[i] == ' ' {
			lastSpace = i
		}
		if widget.MeasureUIText(string(runes[lineStart:i+1]), fs) <= maxW {
			continue
		}
		switch {
		case lastSpace > lineStart:
			lineStart = lastSpace + 1
		case i > lineStart:
			lineStart = i
		default:
			lineStart = i + 1
		}
		lastSpace = -1
		starts = append(starts, lineStart)
	}
	return starts
}

// refLineStarts — эталонные начала всех визуальных строк текста.
func refLineStarts(text string, maxW int, fs float64) []int {
	runes := []rune(text)
	var starts []int
	parStart := 0
	for i := 0; i <= len(runes); i++ {
		if i < len(runes) && runes[i] != '\n' {
			continue
		}
		starts = append(starts, refWrap(runes, parStart, i, maxW, fs)...)
		parStart = i + 1
	}
	return starts
}

// tbLineH — высота строки TextBox при кегле по умолчанию.
const tbLineH = int(widget.DefaultFontSizePt*1.6) + 3

// lineStartsOf снимает начала строк кликом в левый край каждой строки.
func lineStartsOf(tb *widget.TextBox, n int) []int {
	b := tb.Bounds()
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		y := b.Min.Y + tb.PaddingY + i*tbLineH + 1
		tb.OnMouseButton(widget.MouseEvent{X: b.Min.X + 1, Y: y,
			Button: widget.MouseLeft, Pressed: true})
		tb.OnMouseButton(widget.MouseEvent{X: b.Min.X + 1, Y: y,
			Button: widget.MouseLeft, Pressed: false})
		out = append(out, tb.CaretPosition())
	}
	return out
}

// Перенос настоящим шрифтом совпадает с прежним алгоритмом.
func TestTextBox_WrapMatchesReference(t *testing.T) {
	_ = newDialogEngine() // точный измеритель
	words := []string{"слово", "a", "ii", "длинноеслово", "мама", "x",
		"переносимоеслововнутристроки", "0", "тест", "abc", "WWW"}
	rnd := rand.New(rand.NewSource(20260816))

	for tc := 0; tc < 60; tc++ {
		var sb strings.Builder
		for w := 0; w < 1+rnd.Intn(40); w++ {
			sb.WriteString(words[rnd.Intn(len(words))])
			if rnd.Intn(6) == 0 {
				sb.WriteString("\n")
			} else {
				sb.WriteString(" ")
			}
		}
		text := sb.String()

		tb := widget.NewTextBox("")
		width := 60 + rnd.Intn(240)
		tb.SetBounds(image.Rect(0, 0, width, 600))
		tb.SetFocused(true)
		tb.SetText(text)

		maxW := width - 2*tb.PaddingX - tbScrollbarW
		if maxW < 20 {
			maxW = 20
		}
		want := refLineStarts(text, maxW, widget.DefaultFontSizePt)
		if got := tb.LineCount(); got != len(want) {
			t.Fatalf("случай %d: строк %d, эталон %d (w=%d, текст %q)",
				tc, got, len(want), width, text)
		}
		probe := len(want)
		if probe > 25 {
			probe = 25 // ниже строки уходят за край
		}
		got := lineStartsOf(tb, probe)
		for i := 0; i < probe; i++ {
			if got[i] != want[i] {
				t.Fatalf("случай %d: начало строки %d = %d, эталон %d (w=%d, текст %q)",
					tc, i, got[i], want[i], width, text)
			}
		}
	}
}
