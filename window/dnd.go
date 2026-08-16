package window

import "strings"

// maxDnDBytes — потолок на данные перетаскивания от пира (16 МиБ).
const maxDnDBytes = 16 << 20

// filesDropTarget — опциональная возможность нативного бэкенда принимать файлы,
// перетащенные из ОС (проводник / файловый менеджер) в окно. Реализуется
// бэкендами Win32 (WM_DROPFILES), X11 (XDND) и Wayland (wl_data_device);
// на прочих платформах интерфейс не реализован (проброс — no-op).
//
// Координаты колбэка — ФИЗИЧЕСКИЕ пиксели клиентской области окна (как и у
// прочих SetOnMouse*). window.setupFilesDrop переводит их в логические для
// публичного колбэка приложения и пробрасывает физические в движок.
type filesDropTarget interface {
	// SetOnFilesDropped регистрирует колбэк сброса файлов: paths — абсолютные
	// пути, x/y — точка сброса в клиентских физических пикселях. Вызывается
	// из потока цикла событий бэкенда.
	SetOnFilesDropped(fn func(paths []string, x, y int))
}

// parseURIList разбирает тело text/uri-list (RFC 2483): строки, разделённые
// CRLF или LF; строки-комментарии (начинаются с '#') и пустые пропускаются;
// каждая непустая строка — URI. Возвращаются только локальные пути из file://
// URI: hostname отбрасывается, %XX-последовательности декодируются (кириллица
// и пробелы в путях восстанавливаются как UTF-8 байты). Прочие схемы (http://
// и т.п.) игнорируются — перетаскивание ФАЙЛОВ работает с локальными путями.
//
// Чистая функция без побочных эффектов — тестируется в window/dnd_test.go.
func parseURIList(data string) []string {
	var out []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if p, ok := fileURIToPath(line); ok && p != "" {
			out = append(out, p)
		}
	}
	return out
}

// fileURIToPath переводит один file:// URI в локальный путь. ok=false, если
// схема не file. Форматы: "file:///path", "file://localhost/path", "file:/path".
// Чужой hostname отклоняется — файл не локальный. %XX декодируется.
func fileURIToPath(uri string) (string, bool) {
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
