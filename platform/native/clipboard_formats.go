package native

// clipboard_formats.go — «какие виды содержимого объявил владелец буфера»,
// общая часть Wayland и X11 (base.ClipboardFormats; чистые функции,
// проверяются общим прогоном тестов без соединения).
//
// Вопрос задаётся про ОБЪЯВЛЕННЫЕ типы, а не про данные: владелец назвал
// text/uri-list — значит, «файлы», даже если читать их он будет минуту.

import (
	"strings"

	"github.com/oops1/headless-gui/v4/internal/clipimage"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// formatsFromMimes переводит имена типов, объявленных владельцем буфера, в набор
// видов содержимого. Имена сравниваются так же, как при чтении: простой текст —
// text/plain (с параметрами или без) и X11-имена UTF8_STRING/STRING/TEXT, HTML —
// text/html с кодировкой или без, картинка — image/png и image/bmp, файлы —
// text/uri-list и x-special/gnome-copied-files.
func formatsFromMimes(mimes []string) base.ClipboardFormatSet {
	var s base.ClipboardFormatSet
	for _, m := range mimes {
		switch {
		case IsHTMLMime(m):
			s |= base.ClipboardFormatHTML
		case m == MimeTextURIList || m == MimeGnomeCopiedFiles:
			s |= base.ClipboardFormatFiles
		case clipimage.IsPNGMime(m) || clipimage.IsBMPMime(m):
			s |= base.ClipboardFormatImage
		case isTextTarget(m):
			s |= base.ClipboardFormatText
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

// OwnedFormats — виды содержимого, которыми мы сами владеем в буфере. Так же,
// как при выдаче: у картинки текста нет (иначе редактор вставил бы пустую
// строку), у файлов и оформленного текста простой текст есть всегда.
//
//facadegen:skip
func OwnedFormats(text, html string, files, img bool) base.ClipboardFormatSet {
	var s base.ClipboardFormatSet
	switch {
	case img:
		return base.ClipboardFormatImage
	case files:
		s |= base.ClipboardFormatFiles | base.ClipboardFormatText
	case html != "":
		s |= base.ClipboardFormatHTML | base.ClipboardFormatText
	case text != "":
		s |= base.ClipboardFormatText
	}
	return s
}
