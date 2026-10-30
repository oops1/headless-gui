package native

import (
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// Имена типов, объявленных владельцем, переводятся в виды содержимого так же,
// как при чтении: текст — любым из текстовых имён, файлы — uri-list или
// gnome-copied-files, картинка — PNG или BMP.
func TestFormatsFromMimes(t *testing.T) {
	const (
		txt   = base.ClipboardFormatText
		html  = base.ClipboardFormatHTML
		img   = base.ClipboardFormatImage
		files = base.ClipboardFormatFiles
	)
	cases := []struct {
		name  string
		mimes []string
		want  base.ClipboardFormatSet
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
	if got := OwnedFormats("t", "", false, true); got != base.ClipboardFormatImage {
		t.Errorf("картинка: %v", got)
	}
	if got := OwnedFormats("a\nb", "", true, false); got != base.ClipboardFormatFiles|base.ClipboardFormatText {
		t.Errorf("файлы: %v", got)
	}
	if got := OwnedFormats("x", "<b>x</b>", false, false); got != base.ClipboardFormatHTML|base.ClipboardFormatText {
		t.Errorf("HTML: %v", got)
	}
	if got := OwnedFormats("x", "", false, false); got != base.ClipboardFormatText {
		t.Errorf("текст: %v", got)
	}
	if got := OwnedFormats("", "", false, false); got != 0 {
		t.Errorf("пусто: %v", got)
	}
}
