package base

// hexcolor.go — разбор «#RRGGBB»/«#RRGGBBAA»: общий для JSON-загрузчика
// (фасад widget) и XAML.

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// ParseRGBAHex разбирает строку "#RRGGBB" или "#RRGGBBAA" в color.RGBA.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func ParseRGBAHex(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(s, "#")
	switch len(s) {
	case 6:
		v, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			return color.RGBA{}, err
		}
		return color.RGBA{
			R: uint8(v >> 16),
			G: uint8(v >> 8),
			B: uint8(v),
			A: 255,
		}, nil
	case 8:
		v, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			return color.RGBA{}, err
		}
		return color.RGBA{
			R: uint8(v >> 24),
			G: uint8(v >> 16),
			B: uint8(v >> 8),
			A: uint8(v),
		}, nil
	default:
		return color.RGBA{}, fmt.Errorf("invalid color %q", s)
	}
}
