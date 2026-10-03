package window

// clipboard_files.go — список файлов в буфере обмена: сборка и разбор
// форматов, общие для Wayland и X11 (тестируются в общем прогоне, без
// соединения).
//
// Файлы в буфере программы Linux понимают двумя типами:
//
//   - text/uri-list (RFC 2483) — URI через CRLF; его берут почти все;
//   - x-special/gnome-copied-files — первая строка «copy» или «cut», дальше
//     URI через LF; только он говорит, что файлы вырезали, а не скопировали
//     (Nautilus, Dolphin, Thunar и компоновщик WinLine его понимают).
//
// Источник объявляет оба и простой текст с путями — для программ, которые
// берут только текст.

import (
	"strings"
)

const (
	// mimeTextUriList — список файлов: и в буфере обмена, и при перетаскивании.
	mimeTextUriList = "text/uri-list"
	// mimeGnomeCopiedFiles — список файлов с пометкой copy/cut.
	mimeGnomeCopiedFiles = "x-special/gnome-copied-files"
)

// fileURI превращает абсолютный путь в file:// URI. Байты вне безопасного
// набора кодируются %XX — так же, как это делает g_filename_to_uri: кириллица
// и пробелы в именах доходят до файлового менеджера без потерь.
func fileURI(path string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len("file://") + len(path))
	b.WriteString("file://")
	for i := 0; i < len(path); i++ {
		c := path[i]
		if uriSafeByte(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// uriSafeByte — байт, который в пути URI остаётся как есть: незарезервированные
// символы, разделитель пути и подразделители, допустимые в сегменте пути.
func uriSafeByte(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("-._~/!$&'()*+,;=:@", c) >= 0
}

// buildURIList собирает тело text/uri-list: URI через CRLF, как требует
// RFC 2483, с завершающим CRLF.
func buildURIList(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(fileURI(p))
		b.WriteString("\r\n")
	}
	return b.String()
}

// buildGnomeCopiedFiles собирает тело x-special/gnome-copied-files: «copy» или
// «cut», затем URI через LF. Завершающего перевода строки нет — так пишет
// Nautilus, и часть читателей принимает лишнюю пустую строку за пустой URI.
func buildGnomeCopiedFiles(paths []string, cut bool) string {
	var b strings.Builder
	if cut {
		b.WriteString("cut")
	} else {
		b.WriteString("copy")
	}
	for _, p := range paths {
		b.WriteByte('\n')
		b.WriteString(fileURI(p))
	}
	return b.String()
}

// filesPlainText — те же файлы простым текстом: пути через перевод строки.
func filesPlainText(paths []string) string { return strings.Join(paths, "\n") }

// parseClipboardURIList разбирает text/uri-list из буфера обмена.
//
// Строже, чем при перетаскивании: там чужие ссылки просто пропускаются, а
// здесь список, где есть хоть один не локальный файл (https://, чужой хост),
// — это не файлы вовсе. Иначе «вставить» в проводнике молча потеряло бы
// часть скопированного, а браузерная ссылка в буфере стала бы «файлом».
func parseClipboardURIList(data string) ([]string, bool) {
	var out []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r\x00"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p, ok := fileURIToPath(line)
		if !ok || !strings.HasPrefix(p, "/") {
			return nil, false
		}
		out = append(out, p)
	}
	return out, len(out) > 0
}

// parseGnomeCopiedFiles разбирает x-special/gnome-copied-files: первая строка
// — действие, дальше список URI с теми же правилами, что у uri-list.
func parseGnomeCopiedFiles(data string) (paths []string, cut, ok bool) {
	data = strings.TrimRight(data, "\x00")
	head, rest, _ := strings.Cut(data, "\n")
	switch strings.TrimSpace(strings.TrimRight(head, "\r")) {
	case "copy":
	case "cut":
		cut = true
	default:
		return nil, false, false
	}
	paths, ok = parseClipboardURIList(rest)
	if !ok {
		return nil, false, false
	}
	return paths, cut, true
}
