package svg

import (
	"image/color"
	"strconv"
	"strings"
)

// PaintKind — способ задания цвета заливки/обводки.
type PaintKind uint8

const (
	// PaintNone — fill="none" (не рисовать).
	PaintNone PaintKind = iota
	// PaintColor — конкретный цвет (#hex / rgb() / имя).
	PaintColor
	// PaintCurrent — fill="currentColor": подставить цвет из виджета/темы.
	PaintCurrent
	// PaintInherit — значение не задано на элементе (наследуется от родителя).
	PaintInherit
)

// Paint — разобранное значение fill/stroke.
type Paint struct {
	Kind  PaintKind
	Color color.RGBA // валиден при Kind==PaintColor
}

// namedColors — небольшое подмножество именованных цветов SVG/CSS.
var namedColors = map[string]color.RGBA{
	"black":       {0, 0, 0, 255},
	"white":       {255, 255, 255, 255},
	"red":         {255, 0, 0, 255},
	"green":       {0, 128, 0, 255},
	"lime":        {0, 255, 0, 255},
	"blue":        {0, 0, 255, 255},
	"yellow":      {255, 255, 0, 255},
	"cyan":        {0, 255, 255, 255},
	"aqua":        {0, 255, 255, 255},
	"magenta":     {255, 0, 255, 255},
	"fuchsia":     {255, 0, 255, 255},
	"gray":        {128, 128, 128, 255},
	"grey":        {128, 128, 128, 255},
	"silver":      {192, 192, 192, 255},
	"maroon":      {128, 0, 0, 255},
	"olive":       {128, 128, 0, 255},
	"navy":        {0, 0, 128, 255},
	"teal":        {0, 128, 128, 255},
	"purple":      {128, 0, 128, 255},
	"orange":      {255, 165, 0, 255},
	"transparent": {0, 0, 0, 0},
}

// ParsePaint разбирает значение атрибута fill/stroke.
// Пустая строка → PaintInherit. Неразобранное значение → PaintInherit
// (безопасный откат: цвет унаследуется/останется прежним).
func ParsePaint(s string) Paint {
	s = strings.TrimSpace(s)
	if s == "" {
		return Paint{Kind: PaintInherit}
	}
	switch strings.ToLower(s) {
	case "none":
		return Paint{Kind: PaintNone}
	case "currentcolor":
		return Paint{Kind: PaintCurrent}
	case "inherit":
		return Paint{Kind: PaintInherit}
	}
	if c, ok := ParseColor(s); ok {
		return Paint{Kind: PaintColor, Color: c}
	}
	return Paint{Kind: PaintInherit}
}

// ParseColor разбирает цвет CSS/SVG: #rgb, #rgba, #rrggbb, #rrggbbaa,
// rgb(r,g,b), rgba(r,g,b,a) (компоненты 0..255 или проценты), имя из
// namedColors. Возвращает (цвет, true) при успехе.
func ParseColor(s string) (color.RGBA, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return color.RGBA{}, false
	}
	if s[0] == '#' {
		return parseHex(s[1:])
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "rgb") {
		return parseRGBFunc(low)
	}
	if c, ok := namedColors[low]; ok {
		return c, true
	}
	return color.RGBA{}, false
}

func parseHex(h string) (color.RGBA, bool) {
	switch len(h) {
	case 3: // #rgb
		r := hexNibble(h[0])
		g := hexNibble(h[1])
		b := hexNibble(h[2])
		if r < 0 || g < 0 || b < 0 {
			return color.RGBA{}, false
		}
		return color.RGBA{R: uint8(r * 17), G: uint8(g * 17), B: uint8(b * 17), A: 255}, true
	case 4: // #rgba
		r := hexNibble(h[0])
		g := hexNibble(h[1])
		b := hexNibble(h[2])
		a := hexNibble(h[3])
		if r < 0 || g < 0 || b < 0 || a < 0 {
			return color.RGBA{}, false
		}
		return color.RGBA{R: uint8(r * 17), G: uint8(g * 17), B: uint8(b * 17), A: uint8(a * 17)}, true
	case 6: // #rrggbb
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return color.RGBA{}, false
		}
		return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, true
	case 8: // #rrggbbaa
		v, err := strconv.ParseUint(h, 16, 64)
		if err != nil {
			return color.RGBA{}, false
		}
		return color.RGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}, true
	}
	return color.RGBA{}, false
}

func hexNibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// parseRGBFunc разбирает rgb(...) / rgba(...).
func parseRGBFunc(s string) (color.RGBA, bool) {
	open := strings.IndexByte(s, '(')
	cl := strings.IndexByte(s, ')')
	if open < 0 || cl < 0 || cl < open {
		return color.RGBA{}, false
	}
	inner := s[open+1 : cl]
	// Разделители — запятые и/или пробелы (CSS4 допускает пробелы).
	parts := strings.FieldsFunc(inner, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '/'
	})
	if len(parts) < 3 {
		return color.RGBA{}, false
	}
	r, ok1 := parseChannel(parts[0])
	g, ok2 := parseChannel(parts[1])
	b, ok3 := parseChannel(parts[2])
	if !ok1 || !ok2 || !ok3 {
		return color.RGBA{}, false
	}
	a := uint8(255)
	if len(parts) >= 4 {
		if av, ok := parseAlpha(parts[3]); ok {
			a = av
		}
	}
	return color.RGBA{R: r, G: g, B: b, A: a}, true
}

func parseChannel(s string) (uint8, bool) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return 0, false
		}
		return clampByte(f / 100 * 255), true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return clampByte(f), true
}

func parseAlpha(s string) (uint8, bool) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return 0, false
		}
		return clampByte(f / 100 * 255), true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return clampByte(f * 255), true
}

func clampByte(f float64) uint8 {
	if f <= 0 {
		return 0
	}
	if f >= 255 {
		return 255
	}
	return uint8(f + 0.5)
}
