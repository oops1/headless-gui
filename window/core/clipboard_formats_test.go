package core

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Имена типов, объявленных владельцем, переводятся в виды содержимого так же,
// как при чтении: текст — любым из текстовых имён, файлы — uri-list или
// gnome-copied-files, картинка — PNG или BMP.
func TestFormatsFromMimes(t *testing.T) {
	const (
		txt   = widget.ClipboardFormatText
		html  = widget.ClipboardFormatHTML
		img   = widget.ClipboardFormatImage
		files = widget.ClipboardFormatFiles
	)
	cases := []struct {
		name  string
		mimes []string
		want  widget.ClipboardFormatSet
	}{
		{"пусто", nil, 0},
		{"текст", []string{"text/plain;charset=utf-8", "UTF8_STRING"}, txt},
		{"X11-имена текста", []string{"STRING"}, txt},
		{"HTML с кодировкой", []string{"text/html;charset=utf-8", "text/plain"}, txt | html},
		{"файлы uri-list", []string{"text/uri-list", "text/plain"}, files | txt},
		{"файлы gnome", []string{"x-special/gnome-copied-files"}, files},
		{"картинка PNG", []string{"image/png"}, img},
		{"картинка BMP", []string{"image/bmp"}, img},
		{"чужое", []string{"application/x-foo", "image/gif"}, 0},
	}
	for _, c := range cases {
		if got := formatsFromMimes(c.mimes); got != c.want {
			t.Errorf("%s: %v, ждал %v", c.name, got, c.want)
		}
	}
}

// Свой буфер: у картинки текста нет, у файлов и HTML простой текст есть.
func TestOwnedFormats(t *testing.T) {
	if got := ownedFormats("t", "", false, true); got != widget.ClipboardFormatImage {
		t.Errorf("картинка: %v", got)
	}
	if got := ownedFormats("a\nb", "", true, false); got != widget.ClipboardFormatFiles|widget.ClipboardFormatText {
		t.Errorf("файлы: %v", got)
	}
	if got := ownedFormats("x", "<b>x</b>", false, false); got != widget.ClipboardFormatHTML|widget.ClipboardFormatText {
		t.Errorf("HTML: %v", got)
	}
	if got := ownedFormats("x", "", false, false); got != widget.ClipboardFormatText {
		t.Errorf("текст: %v", got)
	}
	if got := ownedFormats("", "", false, false); got != 0 {
		t.Errorf("пусто: %v", got)
	}
}
