package engine

import (
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/oops1/headless-gui/v3/widget/svg"
)

// svgTextBridge — реализация svg.TextRasterizer поверх шрифтов движка: <text>
// внутри SVG рисуется теми же файлами шрифтов, что и остальной текст окна.
// Пакет svg шрифтов не знает — он просит у моста контуры букв и рисует их как
// обычные фигуры, поэтому fill, stroke, градиенты, clip-path и mask работают
// на тексте без отдельной поддержки.
//
// Мост берёт холст движка в момент вызова (как измерители текста): SetScale и
// SetResolution заменяют e.canvas, и привязанный к первому холсту отвечал бы
// по шрифтам, которых уже нет.
type svgTextBridge struct{ e *Engine }

// svgOutlineEm — размер em, в котором запрашиваются контуры: 2048 px на em
// даёт точность 1/64 от 1/2048 em — округление 26.6 не видно даже при
// стократном увеличении значка.
const svgOutlineEm = 2048

// svgFlattenTol — допуск разбиения кривых глифа на отрезки, в единицах
// пользователя SVG.
const svgFlattenTol = 0.01

// Outline реализует svg.TextRasterizer.
func (t svgTextBridge) Outline(spec svg.FontSpec, size float64, text string) ([]svg.Contour, float64, bool) {
	if t.e == nil || size <= 0 || text == "" {
		return nil, 0, false
	}
	c := t.e.canvas
	primary := c.svgFont(spec)
	if primary == nil || primary.ttf == nil {
		return nil, 0, false
	}
	k := size / (svgOutlineEm * 64) // 26.6 при em = svgOutlineEm → единицы пользователя
	ppem := fixed.Int26_6(svgOutlineEm * 64)

	var (
		contours []svg.Contour
		pen      float64
		prev     sfnt.GlyphIndex
		prevFont *FontCache
		buf      sfnt.Buffer
	)
	for _, r := range text {
		fc := primary
		if r != ' ' && !fc.HasGlyph(r) {
			for _, fb := range c.fallbacks {
				if fb != nil && fb.ttf != nil && fb.HasGlyph(r) {
					fc = fb
					break
				}
			}
		}
		idx, err := fc.ttf.GlyphIndex(&buf, r)
		if err != nil {
			continue
		}
		if prevFont == fc && prev != 0 && idx != 0 {
			if kern, err := fc.ttf.Kern(&buf, prev, idx, ppem, font.HintingNone); err == nil {
				pen += float64(kern) * k
			}
		}
		segs, err := fc.ttf.LoadGlyph(&buf, idx, ppem, nil)
		if err == nil {
			contours = appendGlyphContours(contours, segs, pen, k)
		}
		if adv, err := fc.ttf.GlyphAdvance(&buf, idx, ppem, font.HintingNone); err == nil {
			pen += float64(adv) * k
		}
		prev, prevFont = idx, fc
	}
	return contours, pen, true
}

// svgFont подбирает шрифт под (семейство, вес, наклон) SVG: первое
// знакомое семейство из списка, иначе шрифт по умолчанию (в нём тот же выбор
// веса, что и у остальных виджетов: жирный запрос даёт встроенный Go Bold,
// если у шрифта по умолчанию жирного нет).
func (c *Canvas) svgFont(spec svg.FontSpec) *FontCache {
	weight := spec.Weight
	if weight <= 0 {
		weight = 400
	}
	for _, fam := range spec.Families {
		if c.families == nil || c.svgGenericFamily(fam) {
			continue
		}
		c.families.mu.RLock()
		known := len(c.families.fam[familyKey(fam)]) > 0
		c.families.mu.RUnlock()
		if !known {
			if fc, ok := c.namedFonts[fam]; ok && fc != nil {
				return fc
			}
			continue
		}
		if name := c.resolveFace(fam, weight, spec.Italic); name != "" {
			if fc := c.namedFonts[name]; fc != nil {
				return fc
			}
		}
	}
	if c.families != nil {
		if name := c.resolveFace("", weight, spec.Italic); name != "" {
			if fc := c.namedFonts[name]; fc != nil {
				return fc
			}
		}
	}
	return c.fontCache
}

// svgGenericFamily — общие семейства CSS: у них нет файла, это «шрифт по
// умолчанию».
func (c *Canvas) svgGenericFamily(fam string) bool {
	switch strings.ToLower(fam) {
	case "serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui", "ui-sans-serif":
		return true
	}
	return false
}

// appendGlyphContours переводит сегменты глифа (26.6; sfnt уже отдаёт ось Y
// вниз, как в SVG) в замкнутые контуры в единицах пользователя, сдвинутые на penX.
func appendGlyphContours(out []svg.Contour, segs sfnt.Segments, penX, k float64) []svg.Contour {
	var cur []svg.Point
	var last svg.Point
	pt := func(p fixed.Point26_6) svg.Point {
		return svg.Point{X: penX + float64(p.X)*k, Y: float64(p.Y) * k}
	}
	flush := func() {
		if len(cur) >= 3 {
			out = append(out, svg.Contour{Points: cur, Closed: true})
		}
		cur = nil
	}
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			flush()
			last = pt(s.Args[0])
			cur = []svg.Point{last}
		case sfnt.SegmentOpLineTo:
			last = pt(s.Args[0])
			cur = append(cur, last)
		case sfnt.SegmentOpQuadTo:
			c1, p := pt(s.Args[0]), pt(s.Args[1])
			d := math.Hypot(last.X-2*c1.X+p.X, last.Y-2*c1.Y+p.Y) / 4
			n := flattenSteps(d)
			for i := 1; i <= n; i++ {
				t := float64(i) / float64(n)
				u := 1 - t
				cur = append(cur, svg.Point{
					X: u*u*last.X + 2*u*t*c1.X + t*t*p.X,
					Y: u*u*last.Y + 2*u*t*c1.Y + t*t*p.Y,
				})
			}
			last = p
		case sfnt.SegmentOpCubeTo:
			c1, c2, p := pt(s.Args[0]), pt(s.Args[1]), pt(s.Args[2])
			d1 := math.Hypot(last.X-2*c1.X+c2.X, last.Y-2*c1.Y+c2.Y)
			d2 := math.Hypot(c1.X-2*c2.X+p.X, c1.Y-2*c2.Y+p.Y)
			n := flattenSteps(math.Max(d1, d2) * 0.75)
			for i := 1; i <= n; i++ {
				t := float64(i) / float64(n)
				u := 1 - t
				cur = append(cur, svg.Point{
					X: u*u*u*last.X + 3*u*u*t*c1.X + 3*u*t*t*c2.X + t*t*t*p.X,
					Y: u*u*u*last.Y + 3*u*u*t*c1.Y + 3*u*t*t*c2.Y + t*t*t*p.Y,
				})
			}
			last = p
		}
	}
	flush()
	return out
}

// flattenSteps — число отрезков кривой по её максимальному отклонению от
// хорды d (грубая оценка с запасом).
func flattenSteps(d float64) int {
	n := int(math.Ceil(math.Sqrt(d / svgFlattenTol)))
	if n < 2 {
		return 2
	}
	if n > 48 {
		return 48
	}
	return n
}
