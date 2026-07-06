package window

// keysym.go — преобразование X11/xkb keysym в руны.
// Файл платформонезависим (чистые таблицы) — тестируется на всех ОС.
//
// Используется обоими Linux-бэкендами:
//   - X11: числовые keysym из GetKeyboardMapping → keysymToRune;
//   - Wayland: имена keysym из текстового xkb-keymap → keysymNameToRune.
//
// Покрытие: ASCII, Latin-1, Unicode-keysym (0x01000000|cp), кириллический
// блок keysymdef (0x6a1–0x6ff, порядок KOI8-R) и частые именованные
// европейские символы. Dead-keys и прочая экзотика дают 0 (символ не
// вводится) — движку это безопасно.

import "strconv"

// кириллица keysymdef: строки в порядке кодов.
// 0x6c0–0x6df (нижний регистр) и 0x6e0–0x6ff (верхний) — порядок KOI8-R.
const (
	cyrLower = "юабцдефгхийклмнопярстужвьызшэщчъ"
	cyrUpper = "ЮАБЦДЕФГХИЙКЛМНОПЯРСТУЖВЬЫЗШЭЩЧЪ"
	// 0x6a1–0x6af и 0x6b1–0x6bf: сербские/украинские/белорусские буквы.
	cyrExtLower = "ђѓёєѕіїјљњћќґўџ"
	cyrExtUpper = "ЂЃЁЄЅІЇЈЉЊЋЌҐЎЏ"
)

// keysymToRune переводит числовой keysym в руну (0 — непечатаемый/неизвестный).
func keysymToRune(sym uint32) rune {
	switch {
	case sym >= 0x20 && sym <= 0x7E: // ASCII
		return rune(sym)
	case sym >= 0xA0 && sym <= 0xFF: // Latin-1
		return rune(sym)
	case sym&0xFF000000 == 0x01000000: // Unicode keysym
		return rune(sym & 0x00FFFFFF)
	case sym >= 0x6C0 && sym <= 0x6DF: // кириллица, нижний регистр
		return []rune(cyrLower)[sym-0x6C0]
	case sym >= 0x6E0 && sym <= 0x6FF: // кириллица, верхний регистр
		return []rune(cyrUpper)[sym-0x6E0]
	case sym >= 0x6A1 && sym <= 0x6AF: // расширенная кириллица, нижний
		return []rune(cyrExtLower)[sym-0x6A1]
	case sym >= 0x6B1 && sym <= 0x6BF: // расширенная кириллица, верхний
		return []rune(cyrExtUpper)[sym-0x6B1]
	case sym == 0x6B0: // numerosign
		return '№'
	}
	return 0
}

// namedKeysyms — именованные keysym текстового xkb-формата → числовой keysym.
// ASCII-пунктуация + частые европейские имена (Latin-1).
var namedKeysyms = map[string]uint32{
	"space": 0x20, "exclam": 0x21, "quotedbl": 0x22, "numbersign": 0x23,
	"dollar": 0x24, "percent": 0x25, "ampersand": 0x26, "apostrophe": 0x27,
	"parenleft": 0x28, "parenright": 0x29, "asterisk": 0x2A, "plus": 0x2B,
	"comma": 0x2C, "minus": 0x2D, "period": 0x2E, "slash": 0x2F,
	"colon": 0x3A, "semicolon": 0x3B, "less": 0x3C, "equal": 0x3D,
	"greater": 0x3E, "question": 0x3F, "at": 0x40,
	"bracketleft": 0x5B, "backslash": 0x5C, "bracketright": 0x5D,
	"asciicircum": 0x5E, "underscore": 0x5F, "grave": 0x60,
	"braceleft": 0x7B, "bar": 0x7C, "braceright": 0x7D, "asciitilde": 0x7E,

	// Latin-1 (частые европейские раскладки)
	"nobreakspace": 0xA0, "exclamdown": 0xA1, "cent": 0xA2, "sterling": 0xA3,
	"currency": 0xA4, "yen": 0xA5, "section": 0xA7, "copyright": 0xA9,
	"guillemotleft": 0xAB, "guillemotright": 0xBB, "degree": 0xB0,
	"plusminus": 0xB1, "paragraph": 0xB6, "questiondown": 0xBF,
	"ssharp": 0xDF, "agrave": 0xE0, "aacute": 0xE1, "acircumflex": 0xE2,
	"adiaeresis": 0xE4, "aring": 0xE5, "ae": 0xE6, "ccedilla": 0xE7,
	"egrave": 0xE8, "eacute": 0xE9, "ecircumflex": 0xEA, "ediaeresis": 0xEB,
	"idiaeresis": 0xEF, "ntilde": 0xF1, "ograve": 0xF2, "oacute": 0xF3,
	"ocircumflex": 0xF4, "odiaeresis": 0xF6, "oslash": 0xF8, "ugrave": 0xF9,
	"uacute": 0xFA, "ucircumflex": 0xFB, "udiaeresis": 0xFC, "yacute": 0xFD,

	"numerosign": 0x6B0,
}

// cyrillicNames — суффиксы Cyrillic_* (нижний регистр) → руна.
var cyrillicNames = map[string]rune{
	"a": 'а', "be": 'б', "ve": 'в', "ghe": 'г', "de": 'д', "ie": 'е',
	"zhe": 'ж', "ze": 'з', "i": 'и', "shorti": 'й', "ka": 'к', "el": 'л',
	"em": 'м', "en": 'н', "o": 'о', "pe": 'п', "er": 'р', "es": 'с',
	"te": 'т', "u": 'у', "ef": 'ф', "ha": 'х', "tse": 'ц', "che": 'ч',
	"sha": 'ш', "shcha": 'щ', "hardsign": 'ъ', "yeru": 'ы', "softsign": 'ь',
	"e": 'э', "yu": 'ю', "ya": 'я', "io": 'ё',
	"dzhe": 'џ', "je": 'ј', "lje": 'љ', "nje": 'њ', "dze": 'ѕ',
	"ukrainian_i": 'і', "ukrainian_yi": 'ї', "ukrainian_ie": 'є',
	"ukranian_i": 'і', "ukranian_yi": 'ї', "ukranian_je": 'є', // старые алиасы keysymdef
	"ghe_bar": 'ґ', "shha": 'һ',
}

// keysymNameToRune переводит имя keysym (текстовый xkb) в руну.
// 0 — непечатаемый (dead_*, служебные) или неизвестный.
func keysymNameToRune(name string) rune {
	if name == "" || name == "NoSymbol" || name == "VoidSymbol" {
		return 0
	}
	// Одиночный ASCII-символ: "a", "Z", "1"…
	if len(name) == 1 {
		c := name[0]
		if c >= 0x20 && c <= 0x7E {
			return rune(c)
		}
		return 0
	}
	// U0416 / U+0416 — Unicode-имя.
	if name[0] == 'U' {
		hex := name[1:]
		if len(hex) > 0 && hex[0] == '+' {
			hex = hex[1:]
		}
		if v, err := strconv.ParseUint(hex, 16, 32); err == nil {
			return rune(v)
		}
	}
	// 0x1000416 — числовой keysym.
	if len(name) > 2 && name[0] == '0' && (name[1] == 'x' || name[1] == 'X') {
		if v, err := strconv.ParseUint(name[2:], 16, 32); err == nil {
			return keysymToRune(uint32(v))
		}
	}
	if sym, ok := namedKeysyms[name]; ok {
		return keysymToRune(sym)
	}
	// Cyrillic_*: нижний регистр — суффикс в нижнем, верхний — в верхнем
	// (Cyrillic_a → а, Cyrillic_A → А, Cyrillic_SHCHA → Щ).
	if len(name) > 9 && name[:9] == "Cyrillic_" {
		suf := name[9:]
		lower := toLowerASCII(suf)
		r, ok := cyrillicNames[lower]
		if !ok {
			return 0
		}
		if suf != lower { // имя содержит верхний регистр
			return toUpperRune(r)
		}
		return r
	}
	// Украинские/сербские имена без префикса Cyrillic_ в новых keymaps.
	if r, ok := cyrillicNames[toLowerASCII(name)]; ok {
		if name != toLowerASCII(name) {
			return toUpperRune(r)
		}
		return r
	}
	return 0
}

func toLowerASCII(s string) string {
	b := []byte(s)
	changed := false
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
			changed = true
		}
	}
	if !changed {
		return s
	}
	return string(b)
}

// toUpperRune — верхний регистр для наших кириллических рун.
func toUpperRune(r rune) rune {
	switch {
	case r >= 'а' && r <= 'я':
		return r - 32
	case r == 'ё':
		return 'Ё'
	case r == 'ѕ':
		return 'Ѕ'
	case r == 'і':
		return 'І'
	case r == 'ї':
		return 'Ї'
	case r == 'є':
		return 'Є'
	case r == 'ј':
		return 'Ј'
	case r == 'љ':
		return 'Љ'
	case r == 'њ':
		return 'Њ'
	case r == 'џ':
		return 'Џ'
	case r == 'ґ':
		return 'Ґ'
	case r == 'һ':
		return 'Һ'
	}
	return r
}
