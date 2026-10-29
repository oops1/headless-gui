package core

// clipboard_formats.go — «какие виды содержимого объявил владелец буфера»,
// общая часть Wayland и X11 (widget.ClipboardFormats; чистые функции,
// проверяются общим прогоном тестов без соединения).
//
// Вопрос задаётся про ОБЪЯВЛЕННЫЕ типы, а не про данные: владелец назвал
// text/uri-list — значит, «файлы», даже если читать их он будет минуту.

import (
	"strings"

	"github.com/oops1/headless-gui/v3/internal/clipimage"
	"github.com/oops1/headless-gui/v3/widget"
)

// formatsFromMimes переводит имена типов, объявленных владельцем буфера, в набор
// видов содержимого. Имена сравниваются так же, как при чтении: простой текст —
// text/plain (с параметрами или без) и X11-имена UTF8_STRING/STRING/TEXT, HTML —
// text/html с кодировкой или без, картинка — image/png и image/bmp, файлы —
// text/uri-list и x-special/gnome-copied-files.
func formatsFromMimes(mimes []string) widget.ClipboardFormatSet {
	var s widget.ClipboardFormatSet
	for _, m := range mimes {
		switch {
		case isHTMLMime(m):
			s |= widget.ClipboardFormatHTML
		case m == mimeTextUriList || m == mimeGnomeCopiedFiles:
			s |= widget.ClipboardFormatFiles
		case clipimage.IsPNGMime(m) || clipimage.IsBMPMime(m):
			s |= widget.ClipboardFormatImage
		case isTextTarget(m):
			s |= widget.ClipboardFormatText
		}
	}
	return s
}

// isTextTarget — имя типа простого текста: MIME-имя и X11-имена целей.
func isTextTarget(m string) bool {
	switch m {
	case "UTF8_STRING", "STRING", "TEXT":
		return true
	}
	return strings.HasPrefix(strings.ToLower(m), "text/plain")
}

// ownedFormats — виды содержимого, которыми мы сами владеем в буфере. Так же,
// как при выдаче: у картинки текста нет (иначе редактор вставил бы пустую
// строку), у файлов и оформленного текста простой текст есть всегда.
func ownedFormats(text, html string, files, img bool) widget.ClipboardFormatSet {
	var s widget.ClipboardFormatSet
	switch {
	case img:
		return widget.ClipboardFormatImage
	case files:
		s |= widget.ClipboardFormatFiles | widget.ClipboardFormatText
	case html != "":
		s |= widget.ClipboardFormatHTML | widget.ClipboardFormatText
	case text != "":
		s |= widget.ClipboardFormatText
	}
	return s
}
