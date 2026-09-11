package widget

import "image/color"

// diffview_theme.go — палитра контрола сравнения из темы.
//
// Цвета строк и подсветки берутся из полей Theme (DiffAddBG…, Syntax*), которые
// заполнены во всех пресетах. Вывод из поля ввода «на глаз» остался только
// запасным путём — для темы, собранной не из пресета (профиль из файла, своя
// тема приложения), где этих полей нет.

type dvPalette struct {
	bg, card, cardEdge, text, muted, num   color.RGBA
	delBand, delStrong, addBand, addStrong color.RGBA
	del, delHover, add, addHover           color.RGBA
	accent, caret, sel, track, thumb       color.RGBA
	kw, str, com, numLit, fn, dirty        color.RGBA
	radius                                 int
	shadow                                 bool
}

func dvRGB(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

func dvOr(c, def color.RGBA) color.RGBA {
	if c.A == 0 {
		return def
	}
	return c
}

// dvPaletteFrom выводит цвета контрола из темы.
func dvPaletteFrom(t *Theme) dvPalette {
	var p dvPalette
	gray := dvRGB(0x808080)
	p.card = dvOr(t.InputBG, dvOr(t.PanelBG, dvRGB(0xFFFFFF)))
	p.card.A = 255
	p.bg = dvOr(t.WindowBG, mixRGBA(p.card, gray, 0.08))
	if p.bg == p.card {
		p.bg = mixRGBA(p.card, gray, 0.08)
	}
	dark := luminance(p.card) < 128
	p.cardEdge = dvOr(t.Border, mixRGBA(p.card, gray, 0.3))
	p.text = dvOr(t.InputText, dvOr(t.LabelText, dvRGB(0x1F2328)))
	p.muted = dvOr(t.SecondaryText, mixRGBA(p.text, p.card, 0.45))
	p.num = mixRGBA(p.muted, p.card, 0.25)
	p.accent = dvOr(t.Accent, dvRGB(0x7C5CFA))
	p.accent.A = 255
	p.caret = dvOr(t.InputCaret, p.text)
	p.track = dvOr(t.ScrollTrackBG, mixRGBA(p.card, p.text, 0.08))
	p.thumb = dvOr(t.ScrollThumbBG, mixRGBA(p.card, p.text, 0.3))

	red, green := dvRGB(0xE5484D), dvRGB(0x2F9E62)
	if dark {
		red, green = dvRGB(0xF2555A), dvRGB(0x3FB950)
	}
	black := dvRGB(0)
	p.del, p.add = red, green
	p.delHover, p.addHover = mixRGBA(red, black, 0.18), mixRGBA(green, black, 0.18)

	// Полосы строк и выделение — из темы; без них — теми же формулами, по
	// которым их заполняют пресеты (withCodeColors).
	derived := withCodeColors(&Theme{InputBG: p.card, Accent: p.accent})
	p.delBand = dvOr(t.DiffDelBG, derived.DiffDelBG)
	p.delStrong = dvOr(t.DiffDelStrong, derived.DiffDelStrong)
	p.addBand = dvOr(t.DiffAddBG, derived.DiffAddBG)
	p.addStrong = dvOr(t.DiffAddStrong, derived.DiffAddStrong)
	p.sel = dvOr(t.TextSelectionBG, derived.TextSelectionBG)
	p.kw = dvOr(t.SyntaxKeyword, derived.SyntaxKeyword)
	p.str = dvOr(t.SyntaxString, derived.SyntaxString)
	p.com = dvOr(t.SyntaxComment, derived.SyntaxComment)
	p.numLit = dvOr(t.SyntaxNumber, derived.SyntaxNumber)
	p.fn = dvOr(t.SyntaxFunc, derived.SyntaxFunc)
	p.dirty = dvRGB(0xD97706)
	if dark {
		p.dirty = dvRGB(0xF0A63A)
	}

	if c := t.Style.ControlCorner; c > 0 {
		p.radius = c + 2
	}
	// Классика — плоские карточки без тени: объёмной тени в образе Win9x нет.
	p.shadow = !t.Style.Classic3D
	return p
}

// ApplyTheme — контракт Themeable: движок зовёт его при смене темы.
func (d *DiffView) ApplyTheme(t *Theme) {
	d.mu.Lock()
	d.pal = dvPaletteFrom(t)
	d.mu.Unlock()
	d.Invalidate()
}
