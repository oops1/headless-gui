package window

import "testing"

// Чистая логика оформленного текста: типы и кодировки. Без соединения и без
// системного буфера обмена.

func TestIsHTMLMime(t *testing.T) {
	for _, m := range []string{"text/html", "text/html;charset=utf-8", "text/html; charset=UTF-8", "TEXT/HTML"} {
		if !isHTMLMime(m) {
			t.Errorf("тип %q не признан HTML", m)
		}
	}
	for _, m := range []string{"text/plain", "text/htmlx", "text/uri-list", "image/png", "text/_moz_htmlcontext", ""} {
		if isHTMLMime(m) {
			t.Errorf("тип %q признан HTML", m)
		}
	}
}

func TestClipboardHTMLToString(t *testing.T) {
	const want = "<p>Привет</p>"
	utf16 := func(bom []byte, bigEndian bool, s string) []byte {
		out := append([]byte{}, bom...)
		for _, r := range s {
			if bigEndian {
				out = append(out, byte(r>>8), byte(r))
			} else {
				out = append(out, byte(r), byte(r>>8))
			}
		}
		return out
	}
	for name, in := range map[string][]byte{
		"UTF-8":                []byte(want),
		"UTF-8 с BOM":          append([]byte{0xEF, 0xBB, 0xBF}, want...),
		"UTF-8 с NUL в конце":  append([]byte(want), 0, 0),
		"UTF-16LE с BOM":       utf16([]byte{0xFF, 0xFE}, false, want),
		"UTF-16BE с BOM":       utf16([]byte{0xFE, 0xFF}, true, want),
		"UTF-16LE без BOM":     utf16(nil, false, want),
		"UTF-16LE с NUL в хв.": append(utf16(nil, false, want), 0, 0),
	} {
		if got := clipboardHTMLToString(in); got != want {
			t.Errorf("%s: %q, ждал %q", name, got, want)
		}
	}
	if got := clipboardHTMLToString(nil); got != "" {
		t.Errorf("пустой вход дал %q", got)
	}
	// Нечётная длина UTF-16 не должна паниковать.
	clipboardHTMLToString([]byte{0xFF, 0xFE, 0x3C})
}
