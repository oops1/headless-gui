// Package text — шрифты и текст движка: разбор и кэш глифов, шейпинг,
// хинтинг, цветные глифы, качество сглаживания (задача 4.1 плана 4.0). О
// холсте пакет не знает: выводит глифы в пиксели core/raster.
package text

import (
	"strconv"
	"strings"
)

// Встроенные имена шрифтов для жирного, курсива и моноширинного
// (регистрируются движком из gofont). «$» — служебный префикс: настоящее
// семейство так не назовут.
const (
	BuiltinFontBold       = "$hg_bold"
	BuiltinFontItalic     = "$hg_italic"
	BuiltinFontBoldItalic = "$hg_bolditalic"
	// BuiltinFontMono — моноширинный Go Mono: для кода, где колонки обязаны
	// стоять ровно.
	BuiltinFontMono = "$hg_mono"
)

// fontWeightRegular — обычный вес (usWeightClass 400).
const fontWeightRegular = 400

// fontFacePrefix открывает составное имя шрифта. «$» — как у встроенных
// служебных имён (BuiltinFontBold): настоящее семейство так не назовут.
const fontFacePrefix = "$hg_face:"

// FontFace собирает имя шрифта для DrawTextFont / MeasureTextFont по
// семейству, весу и наклону. Движок разбирает его и подставляет начертание
// зарегистрированного семейства («Open Sans» + 600 → OpenSans-SemiBold).
//
// Пустое семейство — шрифт по умолчанию: FontFace("", 700, false) даёт
// жирное начертание того шрифта, которым движок пишет по умолчанию.
//
// Обычное начертание (вес 0 или 400, без наклона) возвращается как есть —
// самим семейством: код, не знающий про веса, и дальше передаёт в движок то
// же имя, что и раньше.
func FontFace(family string, weight int, italic bool) string {
	if (weight == 0 || weight == fontWeightRegular) && !italic {
		return family
	}
	var b strings.Builder
	b.WriteString(fontFacePrefix)
	b.WriteString(strconv.Itoa(weight))
	if italic {
		b.WriteString(":i:")
	} else {
		b.WriteString(":n:")
	}
	b.WriteString(family)
	return b.String()
}

// ParseFontFace — обратное к FontFace. composite=false: имя не составное
// (обычное имя шрифта), остальные результаты тогда пусты.
func ParseFontFace(name string) (family string, weight int, italic bool, composite bool) {
	rest, ok := strings.CutPrefix(name, fontFacePrefix)
	if !ok {
		return "", 0, false, false
	}
	// <вес>:<i|n>:<семейство>; семейство может содержать двоеточия.
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) != 3 {
		return "", 0, false, false
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", 0, false, false
	}
	return parts[2], w, parts[1] == "i", true
}
