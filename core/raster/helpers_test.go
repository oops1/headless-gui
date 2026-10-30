package raster

import hgtext "github.com/oops1/headless-gui/v3/core/text"

// engineCanvas — холст, как его собирает engine.New: шрифт по умолчанию из
// assets (рядом с тестом его нет — встроенный Go Regular), запасная цепочка и
// встроенные начертания (AddDefaultFonts).
func engineCanvas(w, h int) *Canvas {
	c := NewCanvas(w, h, hgtext.NewFontCache("assets"))
	c.AddDefaultFonts()
	return c
}
