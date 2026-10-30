package base

import "github.com/oops1/headless-gui/v4/core/text"

// Веса шрифта по шкале CSS/OpenType (usWeightClass). Движок подбирает
// ближайшее начертание зарегистрированного семейства.
const (
	FontWeightThin     = 100
	FontWeightLight    = 300
	FontWeightRegular  = 400
	FontWeightMedium   = 500
	FontWeightSemiBold = 600
	FontWeightBold     = 700
	FontWeightBlack    = 900
)

// FontFace собирает имя шрифта по семейству, весу и наклону; см.
// text.FontFace.
func FontFace(family string, weight int, italic bool) string {
	return text.FontFace(family, weight, italic)
}

// ParseFontFace — обратное к FontFace; см. text.ParseFontFace.
func ParseFontFace(name string) (family string, weight int, italic bool, composite bool) {
	return text.ParseFontFace(name)
}
