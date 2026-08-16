package widget

// measure.go — замер ширины текста для кода компоновки, работающего ДО
// отрисовки (диалоги вычисляют свой размер при создании, когда DrawContext
// ещё недоступен). Движок регистрирует измеритель при старте
// (SetTextMeasurer); до регистрации действует грубая эвристика.

import (
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// textMeasurer — функция замера ширины строки шрифтом default (px).
type textMeasurer func(text string, sizePt float64) int

var uiTextMeasurer atomic.Value // textMeasurer

// SetTextMeasurer регистрирует точный измеритель текста (вызывает движок).
func SetTextMeasurer(m func(text string, sizePt float64) int) {
	if m != nil {
		uiTextMeasurer.Store(textMeasurer(m))
		BumpTextMetricsRev() // прежние замеры больше не годятся
	}
}

// Мемоизация замеров: ключ — кегль и текст; сброс при смене ревизии метрик
// и при переполнении.
const measureMemoMax = 4096

var (
	measureMu    sync.Mutex
	measureMemo  map[float64]map[string]int
	measureRev   uint64
	measureCount int
)

// MeasureUIText возвращает ширину строки в пикселях шрифтом default.
// До регистрации измерителя — эвристика ~0.65·sizePt на символ.
func MeasureUIText(text string, sizePt float64) int {
	m, ok := uiTextMeasurer.Load().(textMeasurer)
	if !ok {
		return int(float64(len([]rune(text))) * sizePt * 0.65)
	}
	rev := TextMetricsRev()
	measureMu.Lock()
	if measureRev != rev {
		measureMemo, measureCount, measureRev = nil, 0, rev
	}
	bySize := measureMemo[sizePt]
	if w, hit := bySize[text]; hit {
		measureMu.Unlock()
		return w
	}
	measureMu.Unlock()
	w := m(text, sizePt)
	measureStore(rev, sizePt, text, w)
	return w
}

// measureUIBytes — как MeasureUIText, но по байтам: попадание в кэш
// не аллоцирует строку.
func measureUIBytes(b []byte, sizePt float64) int {
	m, ok := uiTextMeasurer.Load().(textMeasurer)
	if !ok {
		return int(float64(utf8.RuneCount(b)) * sizePt * 0.65)
	}
	rev := TextMetricsRev()
	measureMu.Lock()
	if measureRev != rev {
		measureMemo, measureCount, measureRev = nil, 0, rev
	}
	bySize := measureMemo[sizePt]
	if w, hit := bySize[string(b)]; hit {
		measureMu.Unlock()
		return w
	}
	measureMu.Unlock()
	text := string(b)
	w := m(text, sizePt)
	measureStore(rev, sizePt, text, w)
	return w
}

func measureStore(rev uint64, sizePt float64, text string, w int) {
	measureMu.Lock()
	defer measureMu.Unlock()
	if measureRev != rev {
		return
	}
	if measureCount >= measureMemoMax {
		measureMemo, measureCount = nil, 0
	}
	if measureMemo == nil {
		measureMemo = make(map[float64]map[string]int, 4)
	}
	bySize := measureMemo[sizePt]
	if bySize == nil {
		bySize = make(map[string]int, 64)
		measureMemo[sizePt] = bySize
	}
	if _, dup := bySize[text]; !dup {
		measureCount++
	}
	bySize[text] = w
}

// wrapTextPx переносит текст по словам так, чтобы каждая строка была не шире
// maxW пикселей (шрифт default, sizePt). Явные \n сохраняются.
func wrapTextPx(text string, maxW int, sizePt float64) []string {
	var result []string
	var line []byte
	for _, paragraph := range splitLines(text) {
		words := fieldsSpace(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line = append(line[:0], words[0]...)
		for _, w := range words[1:] {
			n := len(line)
			line = append(line, ' ')
			line = append(line, w...)
			if measureUIBytes(line, sizePt) > maxW {
				result = append(result, string(line[:n]))
				line = append(line[:0], w...)
			}
		}
		result = append(result, string(line))
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
