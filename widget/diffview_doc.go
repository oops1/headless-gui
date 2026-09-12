package widget

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oops1/headless-gui/v3/widget/diffview"
)

// diffview_doc.go — буфер одной стороны сравнения: текст, файл, каретка и
// кэши отображения.

// DiffSide — сторона сравнения.
type DiffSide int

const (
	DiffLeft DiffSide = iota
	DiffRight
)

// Other — противоположная сторона.
func (s DiffSide) Other() DiffSide { return 1 - s }

// dvPos — позиция в буфере: строка и колонка в рунах.
type dvPos struct{ line, col int }

func (a dvPos) before(b dvPos) bool { return a.line < b.line || (a.line == b.line && a.col < b.col) }

// dvRowKind — вид экранной строки.
type dvRowKind uint8

const (
	dvRowLine dvRowKind = iota // строка текста
	dvRowGap                   // пустая строка (разделяет карточки-абзацы)
	dvRowFold                  // свёртка одинаковых строк
	dvRowPad                   // строки на этой стороне нет: место под чужую
)

// dvRow — экранная строка: строка буфера (у свёртки — сколько скрыто) и
// участок сравнения, которому она принадлежит.
type dvRow struct {
	kind  dvRowKind
	line  int
	chunk int
}

// dvDoc — буфер одной стороны.
type dvDoc struct {
	title, note, path string
	text              diffview.Text // строки плюс перевод строки, BOM, перевод в конце
	readOnly          bool
	rev, savedRev     int
	diskTime          time.Time
	diskSize          int64

	caret, anchor dvPos
	wantCol       int // экранная колонка для движения вверх-вниз

	// Кэши отображения — пересобираются в DiffView.rebuildLocked.
	disp    [][]rune           // строки с раскрытыми табуляциями
	toks    [][]diffview.Token // подсветка
	hl      [][2]int           // внутристрочная разница, [-1,-1] — нет
	rows    []dvRow
	cards   [][2]int // карточки-абзацы: диапазоны экранных строк
	lineRow []int    // строка буфера → экранная строка, -1 — свёрнута
}

func newDvDoc() *dvDoc {
	return &dvDoc{text: diffview.Text{Lines: []string{""}, EOL: "\n"}, wantCol: -1}
}

func (s *dvDoc) modified() bool    { return s.rev != s.savedRev }
func (s *dvDoc) hasSel() bool      { return s.anchor != s.caret }
func (s *dvDoc) lineLen(i int) int { return utf8.RuneCountInString(s.text.Lines[i]) }
func (s *dvDoc) end() dvPos        { n := len(s.text.Lines) - 1; return dvPos{n, s.lineLen(n)} }

func (s *dvDoc) sel() (a, b dvPos) {
	a, b = s.anchor, s.caret
	if b.before(a) {
		a, b = b, a
	}
	return
}

func (s *dvDoc) clamp(p dvPos) dvPos {
	p.line = max(0, min(p.line, len(s.text.Lines)-1))
	p.col = max(0, min(p.col, s.lineLen(p.line)))
	return p
}

func (s *dvDoc) textRange(a, b dvPos) string {
	L := s.text.Lines
	if a.line == b.line {
		return string([]rune(L[a.line])[a.col:b.col])
	}
	var sb strings.Builder
	sb.WriteString(string([]rune(L[a.line])[a.col:]))
	for i := a.line + 1; i < b.line; i++ {
		sb.WriteByte('\n')
		sb.WriteString(L[i])
	}
	sb.WriteByte('\n')
	sb.WriteString(string([]rune(L[b.line])[:b.col]))
	return sb.String()
}

func (s *dvDoc) remove(a, b dvPos) {
	L := s.text.Lines
	la, lb := []rune(L[a.line]), []rune(L[b.line])
	joined := string(la[:a.col]) + string(lb[b.col:])
	L = append(L[:a.line+1], L[b.line+1:]...)
	L[a.line] = joined
	s.text.Lines = L
}

// insert вставляет текст в p и возвращает позицию за вставкой. Переводы строк
// любого вида приводятся к одному: вставленный из буфера обмена CRLF-текст не
// должен разрезать строку на части с «\r» в конце.
func (s *dvDoc) insert(p dvPos, text string) dvPos {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	parts := strings.Split(text, "\n")
	L := s.text.Lines
	rs := []rune(L[p.line])
	head, tail := string(rs[:p.col]), string(rs[p.col:])
	if len(parts) == 1 {
		L[p.line] = head + parts[0] + tail
		return dvPos{p.line, p.col + utf8.RuneCountInString(parts[0])}
	}
	last := len(parts) - 1
	endCol := utf8.RuneCountInString(parts[last])
	add := make([]string, len(parts))
	copy(add, parts)
	add[0] = head + parts[0]
	add[last] = parts[last] + tail
	s.text.Lines = append(L[:p.line], append(add, L[p.line+1:]...)...)
	return dvPos{p.line + last, endCol}
}

func (s *dvDoc) wordLeft(p dvPos) dvPos {
	if p.col == 0 {
		if p.line == 0 {
			return p
		}
		return dvPos{p.line - 1, s.lineLen(p.line - 1)}
	}
	rs := []rune(s.text.Lines[p.line])
	c := p.col
	for c > 0 && diffview.RuneClass(rs[c-1]) == 0 {
		c--
	}
	if c > 0 {
		k := diffview.RuneClass(rs[c-1])
		for c > 0 && diffview.RuneClass(rs[c-1]) == k {
			c--
		}
	}
	return dvPos{p.line, c}
}

func (s *dvDoc) wordRight(p dvPos) dvPos {
	rs := []rune(s.text.Lines[p.line])
	if p.col >= len(rs) {
		if p.line+1 >= len(s.text.Lines) {
			return p
		}
		return dvPos{p.line + 1, 0}
	}
	c := p.col
	k := diffview.RuneClass(rs[c])
	for c < len(rs) && diffview.RuneClass(rs[c]) == k {
		c++
	}
	for c < len(rs) && diffview.RuneClass(rs[c]) == 0 {
		c++
	}
	return dvPos{p.line, c}
}

func (s *dvDoc) wordAt(p dvPos) (a, b dvPos) {
	rs := []rune(s.text.Lines[p.line])
	if len(rs) == 0 {
		return p, p
	}
	c := min(p.col, len(rs)-1)
	k := diffview.RuneClass(rs[c])
	a0, b0 := c, c+1
	for a0 > 0 && diffview.RuneClass(rs[a0-1]) == k {
		a0--
	}
	for b0 < len(rs) && diffview.RuneClass(rs[b0]) == k {
		b0++
	}
	return dvPos{p.line, a0}, dvPos{p.line, b0}
}

// ─── Файл ───────────────────────────────────────────────────────────────────

func (s *dvDoc) setBytes(data []byte) {
	s.text = diffview.Decode(data)
	s.caret, s.anchor, s.wantCol = dvPos{}, dvPos{}, -1
}

func (s *dvDoc) bytes() []byte { return s.text.Encode() }

func (s *dvDoc) setPath(path string) {
	s.path = path
	s.title = filepath.Base(path)
	s.note = filepath.Dir(path)
}

func (s *dvDoc) stampDisk() {
	if st, err := os.Stat(s.path); err == nil {
		s.diskTime, s.diskSize = st.ModTime(), st.Size()
	}
}

// readDiffFile читает текстовый файл; двоичный — ошибка с понятным человеку
// текстом на языке интерфейса.
func readDiffFile(path string) ([]byte, error) {
	data, err := diffview.ReadFile(path)
	if be, ok := err.(*diffview.BinaryError); ok {
		return nil, &diffBinaryError{be.Name}
	}
	return data, err
}

// diffBinaryError — двоичный файл; текст переводится при каждом Error(), а не
// при создании: ошибку могут показать уже после смены языка.
type diffBinaryError struct{ name string }

func (e *diffBinaryError) Error() string { return Trf("diff.err.binary", e.name) }
