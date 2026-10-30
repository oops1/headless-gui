package raster

// canvas_svg.go — подбор шрифта холста по списку семейств CSS (текст SVG,
// svgtext.go).

import (
	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"strings"
)

// FontForCSS подбирает шрифт под (семейства, вес, наклон) в духе CSS
// font-family: первое знакомое семейство из списка, иначе шрифт по умолчанию (в нём тот же выбор
// веса, что и у остальных виджетов: жирный запрос даёт встроенный Go Bold,
// если у шрифта по умолчанию жирного нет).
func (c *Canvas) FontForCSS(families []string, weight int, italic bool) *hgtext.FontCache {
	if weight <= 0 {
		weight = 400
	}
	for _, fam := range families {
		if c.families == nil || cssGenericFamily(fam) {
			continue
		}
		known := c.families.HasFamily(fam)
		if !known {
			if fc, ok := c.namedFonts.Get(fam); ok && fc != nil {
				return fc
			}
			continue
		}
		if name := c.resolveFace(fam, weight, italic); name != "" {
			if fc, _ := c.namedFonts.Get(name); fc != nil {
				return fc
			}
		}
	}
	if c.families != nil {
		if name := c.resolveFace("", weight, italic); name != "" {
			if fc, _ := c.namedFonts.Get(name); fc != nil {
				return fc
			}
		}
	}
	return c.fontCache
}

// cssGenericFamily — общие семейства CSS: у них нет файла, это «шрифт по
// умолчанию».
func cssGenericFamily(fam string) bool {
	switch strings.ToLower(fam) {
	case "serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui", "ui-sans-serif":
		return true
	}
	return false
}
