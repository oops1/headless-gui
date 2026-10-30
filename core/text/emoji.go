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
// масок, по (шрифт, GID, размер) — плюс цвет для слоёв foreground — в общем
// кэше процесса (glyphstore.go).

package text

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"sync"

	tsfont "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype/tables"
	"golang.org/x/image/draw"
	"golang.org/x/image/math/fixed"
)

// Бюджеты памяти на один цветной глиф — защита от враждебных шрифтов.
const (
	maxGlyphLayerPixels = 16 << 20 // суммарно по слоям глифа
	maxColorGlyphPixels = 4 << 20  // итоговая маска/RGBA глифа
	maxGlyphBitmapBytes = 8 << 20  // размер PNG растрового глифа
	maxGlyphDim         = 4096     // предел стороны в пикселях
)

// budgetWarned — шрифты, по которым превышение бюджета уже залогировано.
var budgetWarned sync.Map

// warnGlyphBudget логирует превышение бюджета один раз на шрифт.
func warnGlyphBudget(face *tsfont.Face) {
	if _, dup := budgetWarned.LoadOrStore(face, struct{}{}); !dup {
		log.Printf("engine: цветной глиф превысил бюджет пикселей — пропущен")
	}
}

// glyphBudgetOK проверяет размеры итоговой маски цветного глифа.
func glyphBudgetOK(w, h int) bool {
	return w > 0 && h > 0 && w <= maxGlyphDim && h <= maxGlyphDim && w*h <= maxColorGlyphPixels
}

// ColorGlyph — растеризованный цветной глиф эмодзи: RGBA-изображение
// (premultiplied-alpha) и смещение левого-верхнего угла от точки отрисовки
// (перо по X, базовая линия по Y). Immutable после создания.
type ColorGlyph struct {
	Img        *image.RGBA // nil — цветного глифа для этого GID нет
	OffX, OffY int
}

// ColorGlyphFor возвращает цветной глиф раскладки g размера sizePx
// (кэшируется в общем кэше процесса по шрифту, GID и размеру) или nil, если
// у глифа нет цветного представления. Глифы со слоями foreground (палитра
// 0xFFFF) зависят от textCol и кэшируются отдельно, с учётом цвета. Глиф без
// владельца (собранный не раскладкой) рендерится без кэша.
func (ts *Shaper) ColorGlyphFor(g PlacedGlyph, sizePx fixed.Int26_6, textCol color.RGBA) *ColorGlyph {
	if g.shard == nil {
		if !faceHasColor(g.Face) {
			return nil
		}
		cg, _ := renderColorGlyph(g.Face, g.GID, sizePx, textCol)
		return cg
	}
	// У шрифта без цветных глифов — сразу nil, без замков и записей: обычный
	// текст шейпера не плодит пустых записей в кэше (у глифа раскладки
	// typesetting уже разобран, признак достоверен).
	if g.shard.font.colorState.Load() != colorYes {
		return nil
	}
	return g.shard.colorGlyph(g.GID, keyOfColor(textCol))
}

// renderColorGlyph строит цветной глиф GID: сперва COLR, затем растровый
// (CBDT/sbix). Второе значение — «динамический» (использует цвет текста).
func renderColorGlyph(face *tsfont.Face, gid tsfont.GID, sizePx fixed.Int26_6, textCol color.RGBA) (*ColorGlyph, bool) {
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
func renderCOLR(face *tsfont.Face, gc tsfont.GlyphColor, sizePx fixed.Int26_6, textCol color.RGBA) (*ColorGlyph, bool) {
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
		m   *GlyphMask
		col color.RGBA // premultiplied
	}
	items := make([]layerRas, 0, len(layers))
	const big = 1 << 30
	minX, minY := big, big
	maxX, maxY := -big, -big
	dynamic := false
	pixels := 0

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
		m := rasterizeGlyph(face, tsfont.GID(layer.GlyphID), sizePx, nil)
		if m.Mask == nil {
			continue
		}
		w, h := m.Mask.Rect.Dx(), m.Mask.Rect.Dy()
		pixels += w * h
		if pixels > maxGlyphLayerPixels {
			warnGlyphBudget(face)
			return nil, dynamic
		}
		if m.OffX < minX {
			minX = m.OffX
		}
		if m.OffY < minY {
			minY = m.OffY
		}
		if m.OffX+w > maxX {
			maxX = m.OffX + w
		}
		if m.OffY+h > maxY {
			maxY = m.OffY + h
		}
		items = append(items, layerRas{m: m, col: col})
	}

	if len(items) == 0 || maxX <= minX || maxY <= minY {
		return nil, dynamic
	}
	if !glyphBudgetOK(maxX-minX, maxY-minY) {
		warnGlyphBudget(face)
		return nil, dynamic
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxX-minX, maxY-minY))
	// Слои рисуются в порядке следования: первый — нижний.
	for _, it := range items {
		compositeMaskColor(dst, it.m.Mask, it.m.OffX-minX, it.m.OffY-minY, it.col)
	}
	return &ColorGlyph{Img: dst, OffX: minX, OffY: minY}, dynamic
}

// renderColorBitmap декодирует растровый PNG-глиф (CBDT/sbix) и масштабирует
// под кегль. Размещение — из GlyphExtents (в font units, ppem уже выставлен).
func renderColorBitmap(face *tsfont.Face, gid tsfont.GID, gb tsfont.GlyphBitmap, sizePx fixed.Int26_6) *ColorGlyph {
	if gb.Format != tsfont.PNG || !glyphPNGSane(gb.Data) {
		return nil // поддерживаем только PNG (Noto Color Emoji и пр.)
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
	if !glyphBudgetOK(w, h) {
		return nil
	}
	src, err := png.Decode(bytes.NewReader(gb.Data))
	if err != nil {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return &ColorGlyph{Img: dst, OffX: floorF32(x0), OffY: floorF32(y0)}
}

// glyphPNGSane проверяет размер данных и заголовок PNG до декодирования.
func glyphPNGSane(data []byte) bool {
	if len(data) == 0 || len(data) > maxGlyphBitmapBytes {
		return false
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return false
	}
	return glyphBudgetOK(cfg.Width, cfg.Height)
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
