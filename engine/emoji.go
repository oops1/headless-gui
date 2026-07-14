// emoji.go — рендеринг цветных эмодзи в текстовом тракте.
//
// Две формы цветных глифов OpenType:
//
//   - COLRv0 + CPAL — векторные слои (так устроен Segoe UI Emoji на Windows):
//     базовый глиф раскладывается в список слоёв, каждый слой — обычный
//     outline-глиф, залитый цветом из палитры CPAL. Слои композитятся снизу
//     вверх в единую RGBA-маску (premultiplied-alpha).
//
//   - CBDT/CBLC и sbix — растровые PNG-глифы (Noto Color Emoji и др.):
//     PNG нужного страйка декодируется и масштабируется под кегль.
//
// Разбор таблиц берётся из go-text/typesetting (Face.GlyphData отдаёт готовые
// GlyphColor/GlyphBitmap), новые зависимости не вводятся. COLRv1 (граф paint,
// градиенты, трансформации) НЕ поддержан — такие глифы пропускаются.
//
// Ключевое отличие от одноцветных масок: цветной глиф блиттится «как есть»
// (premultiplied-over), цвет текста на него не влияет; кэш — отдельный от
// масок, по (face, GID, размер), без учёта цвета текста (см. colorGlyphFor).
package engine

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	tsfont "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype/tables"
	"golang.org/x/image/draw"
	"golang.org/x/image/math/fixed"
)

// colorGlyph — растеризованный цветной глиф эмодзи: RGBA-изображение
// (premultiplied-alpha) и смещение левого-верхнего угла от точки отрисовки
// (перо по X, базовая линия по Y). Immutable после создания.
type colorGlyph struct {
	img        *image.RGBA // nil — цветного глифа для этого GID нет
	offX, offY int
}

// faceHasColorGlyphs сообщает, есть ли у шрифта цветные глифы (COLR или
// растровые CBDT/sbix). Кэшируется по face. Вызывается под ts.mu.
func (ts *textShaper) faceHasColorGlyphs(face *tsfont.Face) bool {
	if v, ok := ts.faceColor[face]; ok {
		return v
	}
	has := faceHasColor(face)
	if ts.faceColor == nil {
		ts.faceColor = make(map[*tsfont.Face]bool)
	}
	ts.faceColor[face] = has
	return has
}

// colorGlyphFor возвращает цветной глиф (кэшируется по face+GID+размеру) или
// nil, если у глифа нет цветного представления. textCol используется только
// для COLR-слоёв с палитрой 0xFFFF (foreground) — глифы с такими слоями
// зависят от цвета текста и не кэшируются.
func (ts *textShaper) colorGlyphFor(face *tsfont.Face, gid tsfont.GID, sizePx fixed.Int26_6, textCol color.RGBA) *colorGlyph {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if !ts.faceHasColorGlyphs(face) {
		return nil
	}
	key := maskKey{face: face, gid: gid, sizePx: sizePx}
	if cg, ok := ts.colorGlyphs[key]; ok {
		return cg
	}
	cg, dynamic := renderColorGlyph(face, gid, sizePx, textCol)
	if dynamic {
		return cg // зависит от цвета текста — не кэшируем
	}
	if ts.colorGlyphs == nil {
		ts.colorGlyphs = make(map[maskKey]*colorGlyph)
	} else if len(ts.colorGlyphs) >= maxShapedMasks {
		ts.colorGlyphs = make(map[maskKey]*colorGlyph, maxShapedMasks/4)
	}
	ts.colorGlyphs[key] = cg
	return cg
}

// renderColorGlyph строит цветной глиф GID: сперва COLR, затем растровый
// (CBDT/sbix). Второе значение — «динамический» (использует цвет текста).
func renderColorGlyph(face *tsfont.Face, gid tsfont.GID, sizePx fixed.Int26_6, textCol color.RGBA) (*colorGlyph, bool) {
	// Растровые страйки выбираются по ppem — выставляем под целевой кегль.
	if len(face.BitmapSizes()) > 0 {
		em := uint16((int(sizePx) + 32) / 64)
		if em == 0 {
			em = 1
		}
		face.SetPpem(em, em)
	}
	// Face.GlyphData отдаёт предпочтение цвету (COLR → bitmap → SVG → outline).
	switch gd := face.GlyphData(gid).(type) {
	case tsfont.GlyphColor:
		return renderCOLR(face, gd, sizePx, textCol)
	case tsfont.GlyphBitmap:
		return renderColorBitmap(face, gid, gd, sizePx), false
	default:
		return nil, false // outline/SVG/пусто — цветного глифа нет
	}
}

// renderCOLR композитит цветной глиф COLR. COLRv0 — плоские слои
// (PaintColrLayersResolved); всё остальное — граф COLRv1 (см. colrv1.go).
func renderCOLR(face *tsfont.Face, gc tsfont.GlyphColor, sizePx fixed.Int26_6, textCol color.RGBA) (*colorGlyph, bool) {
	layers, ok := gc.Paint.(tables.PaintColrLayersResolved)
	if !ok {
		// COLRv1: граф paint-таблиц (Segoe UI Emoji на Windows 11).
		return renderCOLRv1(face, face.COLR, gc.Paint, sizePx, textCol)
	}
	var pal []tables.ColorRecord
	if cp := face.CPAL; len(cp) > 0 {
		pal = cp[0] // палитра 0 — по умолчанию
	}

	type layerRas struct {
		m   *glyphMask
		col color.RGBA // premultiplied
	}
	items := make([]layerRas, 0, len(layers))
	const big = 1 << 30
	minX, minY := big, big
	maxX, maxY := -big, -big
	dynamic := false

	for _, layer := range layers {
		var col color.RGBA
		switch {
		case layer.PaletteIndex == 0xFFFF:
			col = premulColor(textCol) // foreground — цвет текста
			dynamic = true
		case int(layer.PaletteIndex) < len(pal):
			cr := pal[layer.PaletteIndex]
			col = premulColor(color.RGBA{R: cr.Red, G: cr.Green, B: cr.Blue, A: cr.Alpha})
		default:
			continue // индекс за пределами палитры — пропускаем слой
		}
		if col.A == 0 {
			continue // полностью прозрачный слой
		}
		m := rasterizeGlyph(face, tsfont.GID(layer.GlyphID), sizePx)
		if m.mask == nil {
			continue
		}
		w, h := m.mask.Rect.Dx(), m.mask.Rect.Dy()
		if m.offX < minX {
			minX = m.offX
		}
		if m.offY < minY {
			minY = m.offY
		}
		if m.offX+w > maxX {
			maxX = m.offX + w
		}
		if m.offY+h > maxY {
			maxY = m.offY + h
		}
		items = append(items, layerRas{m: m, col: col})
	}

	if len(items) == 0 || maxX <= minX || maxY <= minY {
		return nil, dynamic
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxX-minX, maxY-minY))
	// Слои рисуются в порядке следования: первый — нижний.
	for _, it := range items {
		compositeMaskColor(dst, it.m.mask, it.m.offX-minX, it.m.offY-minY, it.col)
	}
	return &colorGlyph{img: dst, offX: minX, offY: minY}, dynamic
}

// renderColorBitmap декодирует растровый PNG-глиф (CBDT/sbix) и масштабирует
// под кегль. Размещение — из GlyphExtents (в font units, ppem уже выставлен).
func renderColorBitmap(face *tsfont.Face, gid tsfont.GID, gb tsfont.GlyphBitmap, sizePx fixed.Int26_6) *colorGlyph {
	if gb.Format != tsfont.PNG || len(gb.Data) == 0 {
		return nil // поддерживаем только PNG (Noto Color Emoji и пр.)
	}
	src, err := png.Decode(bytes.NewReader(gb.Data))
	if err != nil {
		return nil
	}
	ext, ok := face.GlyphExtents(gid)
	if !ok || ext.Width == 0 || ext.Height == 0 {
		return nil
	}
	scale := float32(sizePx) / 64.0 / float32(face.Upem())
	x0 := ext.XBearing * scale
	y0 := -ext.YBearing * scale // верх глифа относительно базовой линии (ось вниз)
	w := ceilF32(ext.Width * scale)
	h := ceilF32(-ext.Height * scale) // ext.Height < 0 (ось вверх)
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return &colorGlyph{img: dst, offX: floorF32(x0), offY: floorF32(y0)}
}

// premulColor переводит straight-alpha RGBA в premultiplied.
func premulColor(c color.RGBA) color.RGBA {
	if c.A == 0xFF {
		return c
	}
	a := uint32(c.A)
	return color.RGBA{
		R: uint8(uint32(c.R) * a / 255),
		G: uint8(uint32(c.G) * a / 255),
		B: uint8(uint32(c.B) * a / 255),
		A: c.A,
	}
}

// compositeMaskColor Over-композитит сплошной premultiplied-цвет col, промодулированный
// альфа-маской mask, в premultiplied-приёмник dst со смещением (ox, oy).
func compositeMaskColor(dst *image.RGBA, mask *image.Alpha, ox, oy int, col color.RGBA) {
	mw, mh := mask.Rect.Dx(), mask.Rect.Dy()
	sr, sg, sb, sa := uint32(col.R), uint32(col.G), uint32(col.B), uint32(col.A)
	for y := 0; y < mh; y++ {
		mRow := y * mask.Stride
		for x := 0; x < mw; x++ {
			ma := uint32(mask.Pix[mRow+x])
			if ma == 0 {
				continue
			}
			// src = col * ma/255 (остаётся premultiplied)
			r := sr * ma / 255
			g := sg * ma / 255
			b := sb * ma / 255
			a := sa * ma / 255
			inv := 255 - a
			off := dst.PixOffset(ox+x, oy+y)
			if off < 0 || off+4 > len(dst.Pix) {
				continue
			}
			p := dst.Pix[off : off+4 : off+4]
			p[0] = uint8(r + uint32(p[0])*inv/255)
			p[1] = uint8(g + uint32(p[1])*inv/255)
			p[2] = uint8(b + uint32(p[2])*inv/255)
			p[3] = uint8(a + uint32(p[3])*inv/255)
		}
	}
}
