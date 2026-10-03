package window

// clipboard_html.go — чистая логика оформленного текста в буфере обмена,
// общая для Wayland и X11 (тестируется в общем прогоне, без соединения).

import (
	"strings"
	"unicode/utf16"
)

// mimeTextHTML — тип, под которым в буфере обмена ходит HTML.
const mimeTextHTML = "text/html"

// isHTMLMime сообщает, объявлен ли тип как HTML. Помимо голого text/html
// встречается вариант с указанием кодировки — его шлют часть тулкитов, и без
// него их HTML остался бы невидимым.
func isHTMLMime(mime string) bool {
	mime = strings.ToLower(strings.ReplaceAll(mime, " ", ""))
	return mime == mimeTextHTML || strings.HasPrefix(mime, mimeTextHTML+";")
}

// clipboardHTMLToString превращает байты text/html из чужого буфера в строку.
//
// Кодировка у text/html из буфера не гарантирована: Firefox и часть
// приложений Qt кладут UTF-16 (с меткой порядка байт или без неё), остальные —
// UTF-8, иногда с BOM. Без разбора UTF-16 превращался бы в строку с нулевыми
// байтами через один — редактор показал бы «<\x00h\x00t\x00m\x00l\x00».
func clipboardHTMLToString(data []byte) string {
	switch {
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
		return utf16ToString(data[2:], false)
	case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
		return utf16ToString(data[2:], true)
	case len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF:
		data = data[3:]
	case len(data) >= 4 && data[0] != 0 && data[1] == 0 && data[2] != 0 && data[3] == 0:
		// UTF-16LE без метки: разметка начинается с ASCII-символа «<», так
		// что нули стоят на нечётных позициях.
		return utf16ToString(data, false)
	}
	return strings.TrimRight(string(data), "\x00")
}

// utf16ToString декодирует UTF-16 (порядок байт задан флагом) в строку.
func utf16ToString(b []byte, bigEndian bool) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if bigEndian {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return strings.TrimRight(string(utf16.Decode(u)), "\x00")
}
