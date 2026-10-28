package engine

// canvas_text.go — методы холста, которые выводят текст в его пиксели и ведут
// его шрифты: они живут с холстом (core/raster), а шрифты и глифы — в
// core/text (задача 4.1 плана 4.0).

import (
	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"image"
	"image/color"
)

// eachFallback обходит цепочку по порядку, загружая записи по мере надобности,
// пока fn не вернёт true (найдено). ref — шрифт холста для настроек
// загружаемых записей.
func (c *Canvas) eachFallback(ref *FontCache, fn func(*FontCache) bool) {
	for _, fb := range c.fallbacks {
		if fc := fb.Get(ref); fc != nil && fn(fc) {
			return
		}
	}
}

// addFallbackFontFile добавляет в цепочку шрифт-файл, не читая его: файл
// прочитается при первом обращении. Несуществующий, слишком большой файл или
// не файл в цепочку не попадает (newFallbackFile).
func (c *Canvas) addFallbackFontFile(path string) bool {
	fb := hgtext.NewFallbackFile(path)
	if fb == nil {
		return false
	}
	c.fallbacks = append(c.fallbacks, fb)
	c.fontRev.Add(1)
	return true
}

// HasGlyph сообщает, есть ли глиф руны r в шрифте по умолчанию холста — без
// шрифтов-замен (widget.GlyphCoverage).
func (c *Canvas) HasGlyph(r rune) bool { return c.fontCache.HasGlyph(r) }

// registerFace записывает только что зарегистрированный шрифт в таблицу
// семейств (fontFamilies.register).
func (c *Canvas) registerFace(name string, fc *FontCache) { c.families.Register(name, fc) }

// faceFor разбирает имя шрифта, которого нет в реестре буквально
// (fontFamilies.faceFor). nil — подходящего нет, вызывающий берёт шрифт по
// умолчанию.
func (c *Canvas) faceFor(req string) *FontCache { return c.families.FaceFor(req, c.namedFonts) }

// resolveFace подбирает имя шрифта в реестре под (семейство, вес, наклон)
// (fontFamilies.resolveFace).
func (c *Canvas) resolveFace(family string, weight int, italic bool) string {
	return c.families.ResolveFace(family, weight, italic, c.namedFonts)
}

// setTextQuality задаёт слой качества текста всем шрифтам канваса. Шрифты,
// зарегистрированные позже, наследуют его от основного (RegisterFont,
// AddFallbackFont, ленивые запасные). Шаг и кернинг не меняются, раскладки
// шейпера остаются верными, но их маски хранятся в шрифтах — сбрасываются там.
func (c *Canvas) setTextQuality(q TextQuality) {
	c.fontCache.SetQuality(q)
	for _, fc := range c.namedFonts.All() {
		fc.SetQuality(q)
	}
	for _, fb := range c.fallbacks {
		fb.Apply(func(fc *FontCache) { fc.SetQuality(q) })
	}
	c.fontRev.Add(1)
}

// setTextSubpixel включает дробное позиционирование глифов у всех шрифтов
// канваса: основного, именованных и запасных. Шрифты, зарегистрированные
// позже, наследуют режим от основного (RegisterFont, AddFallbackFont).
func (c *Canvas) setTextSubpixel(on bool) {
	c.fontCache.SetSubpixel(on)
	for _, fc := range c.namedFonts.All() {
		fc.SetSubpixel(on)
	}
	for _, fb := range c.fallbacks {
		fb.Apply(func(fc *FontCache) { fc.SetSubpixel(on) }) // незагруженные возьмут режим при загрузке
	}
	c.shaper.DropLayouts()
	c.fontRev.Add(1)
}

// drawLCDMask смешивает цвет с фоном по каналам (субпиксельное сглаживание).
// Канал смешивается по своему покрытию только там, где фон непрозрачен
// (A=255): подпиксели имеют смысл лишь на известном фоне. На прозрачных и
// полупрозрачных пикселях глиф рисуется обычным серым покрытием — средним по
// каналам. Формулы смешивания те же, что у pixsimd.BlendMaskRow.
func (c *Canvas) drawLCDMask(m *hgtext.LCDMask, gx, gy int, col color.RGBA, lut *[256]uint8) {
	col = c.enc(col)
	c.markKind(image.Rect(gx, gy, gx+m.W, gy+m.H))
	r := c.clampRect(image.Rect(gx, gy, gx+m.W, gy+m.H))
	if r.Empty() {
		return
	}
	const m16 = 0xFFFF
	sc := [3]uint32{uint32(col.R) * 0x101, uint32(col.G) * 0x101, uint32(col.B) * 0x101}
	sa := uint32(col.A) * 0x101
	// Логический канал байта j буфера: у BGRX байты R и B переставлены (enc).
	var logical [3]int
	for j := range logical {
		logical[j] = j
		if c.format == FormatBGRX {
			logical[j] = 2 - j
		}
	}
	dst := c.back.Pix
	for yy := r.Min.Y; yy < r.Max.Y; yy++ {
		lx, rx, ok := c.round.spanX(yy, r.Min.X, r.Max.X)
		if !ok {
			continue
		}
		src := m.Pix[(yy-gy)*m.W*3:]
		dOff := c.back.PixOffset(lx, yy)
		for xx := lx; xx < rx; xx, dOff = xx+1, dOff+4 {
			cov := src[(xx-gx)*3 : (xx-gx)*3+3 : (xx-gx)*3+3]
			if cov[0]|cov[1]|cov[2] == 0 {
				continue
			}
			c0, c1, c2 := uint32(cov[0]), uint32(cov[1]), uint32(cov[2])
			if lut != nil {
				c0, c1, c2 = uint32(lut[c0]), uint32(lut[c1]), uint32(lut[c2])
			}
			p := dst[dOff : dOff+4 : dOff+4]
			if p[3] == 255 {
				cs := [3]uint32{c0, c1, c2}
				var avg uint32
				for j := 0; j < 3; j++ {
					ma := cs[logical[j]]
					ma |= ma << 8
					inv := m16 - sa*ma/m16
					p[j] = uint8((uint32(p[j])*0x101*inv/m16 + sc[j]*ma/m16) >> 8)
					avg += cs[j]
				}
				// Альфа пикселя — как у серого покрытия среднего значения.
				ma := (avg + 1) / 3
				ma |= ma << 8
				inv := m16 - sa*ma/m16
				p[3] = uint8((uint32(p[3])*0x101*inv/m16 + sa*ma/m16) >> 8)
				continue
			}
			ma := (c0 + c1 + c2 + 1) / 3
			ma |= ma << 8
			a := sa * ma / m16
			inv := m16 - a
			for j := 0; j < 3; j++ {
				p[j] = uint8((uint32(p[j])*0x101*inv/m16 + sc[j]*ma/m16) >> 8)
			}
			p[3] = uint8((uint32(p[3])*0x101*inv/m16 + sa*ma/m16) >> 8)
		}
	}
}
