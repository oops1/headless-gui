package diffview

import (
	"strings"
	"unicode"
)

// TabWidth — шаг табуляции на экране.
const TabWidth = 4

// RuneClass — класс руны для перемещения по словам: 0 — пробел, 1 — буква,
// цифра или «_», 2 — прочее (знаки). Слово — непрерывная серия одного класса.
func RuneClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return 1
	}
	return 2
}

// IndentOf — ведущие пробелы и табуляции строки.
func IndentOf(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s
}

// DisplayCol — экранная колонка позиции col строки с учётом табуляции.
func DisplayCol(rs []rune, col int) int {
	d := 0
	for i := 0; i < col && i < len(rs); i++ {
		if rs[i] == '\t' {
			d += TabWidth - d%TabWidth
		} else {
			d++
		}
	}
	return d + max(0, col-len(rs))
}

// RawCol — ближайшая позиция в строке к экранной колонке dc: обратная к
// DisplayCol. Щелчок по середине табуляции попадает к ближайшему её краю.
func RawCol(rs []rune, dc int) int {
	d := 0
	for i, r := range rs {
		w := 1
		if r == '\t' {
			w = TabWidth - d%TabWidth
		}
		if dc < d+(w+1)/2 {
			return i
		}
		d += w
	}
	return len(rs)
}

// ExpandTabs раскрывает табуляции в пробелы — строка в том виде, в каком её
// видно на экране.
func ExpandTabs(s string) []rune {
	if !strings.ContainsRune(s, '\t') {
		return []rune(s)
	}
	out := make([]rune, 0, len(s)+8)
	for _, r := range s {
		if r == '\t' {
			for n := TabWidth - len(out)%TabWidth; n > 0; n-- {
				out = append(out, ' ')
			}
			continue
		}
		out = append(out, r)
	}
	return out
}
