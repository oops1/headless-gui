package widget

// measure.go — замер ширины текста для кода компоновки, работающего ДО
// отрисовки (диалоги вычисляют свой размер при создании, когда DrawContext
// ещё недоступен). Движок регистрирует измеритель при старте
// (SetTextMeasurer); до регистрации действует грубая эвристика.

import "sync/atomic"

// textMeasurer — функция замера ширины строки шрифтом default (px).
type textMeasurer func(text string, sizePt float64) int

var uiTextMeasurer atomic.Value // textMeasurer

// SetTextMeasurer регистрирует точный измеритель текста (вызывает движок).
func SetTextMeasurer(m func(text string, sizePt float64) int) {
	if m != nil {
		uiTextMeasurer.Store(textMeasurer(m))
	}
}

// MeasureUIText возвращает ширину строки в пикселях шрифтом default.
// До регистрации измерителя — эвристика ~0.65·sizePt на символ.
func MeasureUIText(text string, sizePt float64) int {
	if m, ok := uiTextMeasurer.Load().(textMeasurer); ok {
		return m(text, sizePt)
	}
	return int(float64(len([]rune(text))) * sizePt * 0.65)
}

// wrapTextPx переносит текст по словам так, чтобы каждая строка была не шире
// maxW пикселей (шрифт default, sizePt). Явные \n сохраняются.
func wrapTextPx(text string, maxW int, sizePt float64) []string {
	var result []string
	for _, paragraph := range splitLines(text) {
		words := fieldsSpace(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if MeasureUIText(line+" "+w, sizePt) > maxW {
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

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func fieldsSpace(s string) []string {
	var out []string
	start := -1
	for i, r := range s {
		if r == ' ' || r == '\t' {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}
