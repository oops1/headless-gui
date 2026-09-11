package diffview

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Text — содержимое файла в виде строк плюс то, что нужно, чтобы записать его
// обратно байт в байт: перевод строки, перевод в конце, метка BOM.
//
// Без этого сохранение правки молча меняло бы файл целиком — CRLF на LF,
// пропадающая BOM, лишний перевод в конце, — и в системе контроля версий
// одна исправленная буква выглядела бы как переписанный файл.
type Text struct {
	Lines   []string
	EOL     string // "\n" или "\r\n"
	FinalNL bool   // файл кончается переводом строки
	BOM     bool   // файл начинается с UTF-8 BOM
}

// Decode разбирает содержимое файла. Перевод строки определяется по первому
// же CRLF: смешанные файлы редки, и сохранять их придётся одним видом.
func Decode(data []byte) Text {
	var t Text
	t.BOM = bytes.HasPrefix(data, utf8BOM)
	if t.BOM {
		data = data[len(utf8BOM):]
	}
	s := string(data)
	t.EOL = "\n"
	if strings.Contains(s, "\r\n") {
		t.EOL = "\r\n"
	}
	t.FinalNL = strings.HasSuffix(s, "\n")
	if t.FinalNL {
		s = s[:len(s)-1]
	}
	t.Lines = strings.Split(s, "\n")
	for i, l := range t.Lines {
		t.Lines[i] = strings.TrimSuffix(l, "\r")
	}
	return t
}

// Encode собирает содержимое файла обратно — с тем же переводом строки, BOM и
// переводом в конце, что были при чтении.
func (t Text) Encode() []byte {
	var buf bytes.Buffer
	if t.BOM {
		buf.Write(utf8BOM)
	}
	eol := t.EOL
	if eol == "" {
		eol = "\n"
	}
	buf.WriteString(strings.Join(t.Lines, eol))
	if t.FinalNL {
		buf.WriteString(eol)
	}
	return buf.Bytes()
}

// BinaryError — файл двоичный, сравнивать его построчно нельзя.
type BinaryError struct{ Name string }

func (e *BinaryError) Error() string { return e.Name + ": binary file" }

// ReadFile читает текстовый файл. Нулевой байт в первых восьми килобайтах —
// признак двоичного файла (так же решает git): построчное сравнение картинки
// дало бы мусор, а правка — испортила бы её.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
		return nil, &BinaryError{filepath.Base(path)}
	}
	return data, nil
}

// WriteFile записывает файл, сохраняя права существующего: правка скрипта не
// должна снимать с него бит исполнения.
func WriteFile(path string, data []byte) error {
	perm := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}
	return os.WriteFile(path, data, perm)
}
