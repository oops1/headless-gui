package native

// dnd.go — приём файлов: разбор text/uri-list и предел данных DnD.

import "strings"

// MaxDnDBytes — потолок на данные перетаскивания от пира (16 МиБ).
//
//facadegen:skip
const MaxDnDBytes = 16 << 20

// ParseURIList разбирает тело text/uri-list (RFC 2483): строки, разделённые
// CRLF или LF; строки-комментарии (начинаются с '#') и пустые пропускаются;
// каждая непустая строка — URI. Возвращаются только локальные пути из file://
// URI: hostname отбрасывается, %XX-последовательности декодируются (кириллица
// и пробелы в путях восстанавливаются как UTF-8 байты). Прочие схемы (http://
// и т.п.) игнорируются — перетаскивание ФАЙЛОВ работает с локальными путями.
//
// Чистая функция без побочных эффектов — тестируется в dnd_test.go.
//
//facadegen:skip
func ParseURIList(data string) []string {
	var out []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if p, ok := FileURIToPath(line); ok && p != "" {
			out = append(out, p)
		}
	}
	return out
}

// FileURIToPath переводит один file:// URI в локальный путь. ok=false, если
// схема не file. Форматы: "file:///path", "file://localhost/path", "file:/path".
// Чужой hostname отклоняется — файл не локальный. %XX декодируется.
//
//facadegen:skip
func FileURIToPath(uri string) (string, bool) {
	const scheme = "file:"
	if !strings.HasPrefix(uri, scheme) {
		return "", false
	}
	rest := uri[len(scheme):]
	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
		host := rest
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			host, rest = rest[:i], rest[i:]
		} else {
			rest = ""
		}
		if host != "" && !strings.EqualFold(host, "localhost") {
			return "", false
		}
	}
	return percentDecode(rest), true
}

// percentDecode декодирует %XX-последовательности в байты. Некорректные
// последовательности оставляются как есть.
func percentDecode(s string) string {
	if !strings.ContainsRune(s, '%') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi := unhex(s[i+1])
			lo := unhex(s[i+2])
			if hi >= 0 && lo >= 0 {
				b.WriteByte(byte(hi<<4 | lo))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// unhex возвращает значение hex-цифры или -1.
func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
