package window

// a11ytext.go — чистая часть текстовой доступности: куски текста, которые
// скринридер просит вокруг заданного места.
//
// Скринридер не читает документ целиком. Он спрашивает символ, слово, строку
// или абзац ВОКРУГ смещения и следом — соседние: так он идёт по тексту за
// кареткой, не перечитывая всё заново. Границы этих кусков и считаются здесь.
//
// Файл без платформенного суффикса намеренно: разбиение текста не зависит от
// того, D-Bus за ним стоит или COM, и проверяется в общем прогоне тестов.

import "strings"

// a11yGranularity — какой кусок текста просят. Значения совпадают с
// AT-SPI TEXT_GRANULARITY_*.
type a11yGranularity int

const (
	a11yGranChar      a11yGranularity = 0
	a11yGranWord      a11yGranularity = 1
	a11yGranSentence  a11yGranularity = 2
	a11yGranLine      a11yGranularity = 3
	a11yGranParagraph a11yGranularity = 4
)

// a11yTextSlice возвращает кусок текста вокруг offset и его границы (в рунах).
//
// Смещение за концом текста — не ошибка: скринридер спрашивает и там, где
// каретка стоит после последнего символа. Ответом тогда будет пустой кусок с
// границами в конце.
func a11yTextSlice(text string, offset int, gran a11yGranularity) (string, int, int) {
	rs := []rune(text)
	n := len(rs)
	if offset < 0 {
		offset = 0
	}
	if offset > n {
		offset = n
	}
	switch gran {
	case a11yGranChar:
		if offset >= n {
			return "", n, n
		}
		return string(rs[offset]), offset, offset + 1
	case a11yGranWord:
		from, to := a11yWordBounds(rs, offset)
		return string(rs[from:to]), from, to
	case a11yGranSentence, a11yGranParagraph, a11yGranLine:
		from, to := a11yLineBounds(rs, offset)
		return string(rs[from:to]), from, to
	}
	return "", offset, offset
}

// a11yLineBounds — границы строки, в которой стоит offset, без переноса на
// конце: перенос принадлежит не строке, а промежутку между строками, и
// скринридер, прочитав его, объявил бы лишнюю пустую строку.
func a11yLineBounds(rs []rune, offset int) (int, int) {
	n := len(rs)
	from := offset
	for from > 0 && rs[from-1] != '\n' {
		from--
	}
	to := offset
	for to < n && rs[to] != '\n' {
		to++
	}
	return from, to
}

// a11yWordBounds — границы слова вокруг offset.
//
// Пробелы между словами — тоже «кусок»: скринридер, идущий по словам, не
// должен терять смещения, иначе следующий шаг начнётся не там.
func a11yWordBounds(rs []rune, offset int) (int, int) {
	n := len(rs)
	if offset >= n {
		return n, n
	}
	word := !a11yIsSpace(rs[offset])
	from := offset
	for from > 0 && !a11yIsSpace(rs[from-1]) == word {
		from--
	}
	to := offset
	for to < n && !a11yIsSpace(rs[to]) == word {
		to++
	}
	return from, to
}

func a11yIsSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// a11yLineAt — номер строки и смещение её начала для позиции offset. Нужен
// там, где скринридер спрашивает «какая это строка».
func a11yLineAt(text string, offset int) (line, start int) {
	rs := []rune(text)
	if offset > len(rs) {
		offset = len(rs)
	}
	for i := 0; i < offset; i++ {
		if rs[i] == '\n' {
			line++
			start = i + 1
		}
	}
	return line, start
}

// a11yTextBoundaryToGran переводит старый вид запроса (AT-SPI BOUNDARY_TYPE,
// которым пользуется GetTextAtOffset) в нынешнюю зернистость.
//
// Старый вид различает начало и конец слова или строки; разницу между ними
// скринридеры давно не используют — важно, какой кусок вернуть.
func a11yTextBoundaryToGran(boundary uint32) a11yGranularity {
	switch boundary {
	case 0: // CHAR
		return a11yGranChar
	case 1, 2: // WORD_START, WORD_END
		return a11yGranWord
	case 3, 4: // SENTENCE_START, SENTENCE_END
		return a11yGranSentence
	case 5, 6: // LINE_START, LINE_END
		return a11yGranLine
	}
	return a11yGranChar
}

// a11yTrimTrailingNewline убирает перенос в конце куска: при чтении абзацами
// он попадает в текст, и скринридер произносит лишнюю паузу.
func a11yTrimTrailingNewline(s string) string { return strings.TrimSuffix(s, "\n") }
