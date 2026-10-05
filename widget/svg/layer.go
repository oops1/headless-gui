package svg

import (
	"image"
	"image/color"
)

// Group — группа, у которой есть эффект над результатом целиком: mask, filter
// (размытие, матрица цвета) и непрозрачность. Группой считается контейнер (g,
// svg, use, symbol, text) и фигура с заливкой И обводкой одновременно (у неё
// opacity тоже относится к склейке заливки и обводки).
//
// Фигуры группы лежат в Document.Shapes подряд, а каждая хранит в Shape.Groups
// цепочку своих групп (внешняя первой). Растеризатор открывает слой, когда
// цепочка требует его, рисует в него фигуры, а на смене цепочки накладывает
// слой на родителя с эффектами группы.
//
// clip-path в группу не входит: вырезка по границе применяется к фигурам
// по отдельности (результат тот же, а слой не нужен).
type Group struct {
	// Opacity — opacity самой группы (0..1; 1 — нет). Слоем рисуется только
	// при Options.GroupLayers; иначе она, как и раньше, уже умножена в
	// FillOpacity/StrokeOpacity потомков.
	Opacity float64
	// Masks — mask группы; результат — произведение.
	Masks []*Mask
	// BlurX/BlurY — feGaussianBlur в единицах viewBox; 0 — без размытия.
	BlurX, BlurY float64
	// ColorMatrix — feColorMatrix над цветом всего слоя.
	ColorMatrix *[20]float64
	// Filter — фильтр со сдвигом, заливкой, композицией и т. п. (см.
	// FilterGraph); выполняется после Blur/ColorMatrix, до масок.
	Filter *FilterGraph
}

// hasEffects — нужен ли группе слой независимо от режима (всё, кроме
// непрозрачности).
func (g *Group) hasEffects() bool {
	return len(g.Masks) > 0 || g.BlurX > 0 || g.BlurY > 0 || g.ColorMatrix != nil || g.Filter != nil
}

// maxLayerDepth — сколько слоёв открыто одновременно. Каждый весит w×h×4
// байта; глубже — группы просто теряют эффект (патологические документы).
const maxLayerDepth = 16

// maxSpareLayers — сколько закрытых слоёв держим для повторного использования.
const maxSpareLayers = 4

type layerFrame struct {
	g   *Group
	img *image.RGBA
}

// layerState — открытые слои одного прохода drawShapes.
type layerState struct {
	stack  []layerFrame
	wanted []*Group
}

// worthy — требует ли группа слоя при текущих опциях.
func (c *rctx) worthy(g *Group) bool {
	return g.hasEffects() || (c.opts.GroupLayers && g.Opacity < 1)
}

// syncLayers приводит стек открытых слоёв к цепочке групп фигуры sh и
// возвращает, куда рисовать. c.div — произведение непрозрачностей слоёв,
// которое уже лежит в прозрачности фигуры и потому снимается: слой наложит
// его сам. c.div == 0 — фигура невидима.
func (c *rctx) syncLayers(ls *layerState, dst *image.RGBA, sh *Shape) *image.RGBA {
	ls.wanted = ls.wanted[:0]
	div := 1.0
	for _, g := range sh.Groups {
		if g == nil || !c.worthy(g) || len(ls.wanted) >= maxLayerDepth {
			continue
		}
		ls.wanted = append(ls.wanted, g)
		if c.opts.GroupLayers && g.Opacity < 1 {
			div *= g.Opacity
		}
	}
	c.div = div

	j := 0
	for j < len(ls.stack) && j < len(ls.wanted) && ls.stack[j].g == ls.wanted[j] {
		j++
	}
	for len(ls.stack) > j {
		c.closeLayer(ls, dst)
	}
	for ; j < len(ls.wanted); j++ {
		ls.stack = append(ls.stack, layerFrame{g: ls.wanted[j], img: c.newLayer()})
	}
	if n := len(ls.stack); n > 0 {
		return ls.stack[n-1].img
	}
	return dst
}

// closeLayer снимает верхний слой стека и накладывает его на родителя.
func (c *rctx) closeLayer(ls *layerState, dst *image.RGBA) {
	n := len(ls.stack)
	top := ls.stack[n-1]
	ls.stack = ls.stack[:n-1]
	parent := dst
	if n > 1 {
		parent = ls.stack[n-2].img
	}
	img := c.finishLayer(top.img, top.g)
	compositeOver(parent, img)
	if img != top.img {
		c.freeLayer(img)
	}
	c.freeLayer(top.img)
}

// newLayer отдаёт чистый прозрачный слой размера растеризации.
func (c *rctx) newLayer() *image.RGBA {
	if n := len(c.spare); n > 0 {
		img := c.spare[n-1]
		c.spare = c.spare[:n-1]
		clear(img.Pix)
		return img
	}
	return image.NewRGBA(image.Rect(0, 0, c.w, c.h))
}

func (c *rctx) freeLayer(img *image.RGBA) {
	if len(c.spare) < maxSpareLayers {
		c.spare = append(c.spare, img)
	}
}

// finishLayer применяет эффекты группы к готовому слою: размытие и матрицу
// цвета, затем маски и, при GroupLayers, непрозрачность. Может вернуть другой
// образ (размытие создаёт новый).
func (c *rctx) finishLayer(img *image.RGBA, g *Group) *image.RGBA {
	if g.BlurX > 0 || g.BlurY > 0 {
		img = blurRGBA(img, g.BlurX*c.s, g.BlurY*c.s)
	}
	if g.ColorMatrix != nil {
		matrixRGBA(img, g.ColorMatrix)
	}
	if g.Filter != nil {
		img = g.Filter.run(img, c.s)
	}
	for _, m := range g.Masks {
		mulRGBAAlpha(img, c.maskAlpha(m))
	}
	if c.opts.GroupLayers && g.Opacity < 1 {
		scaleRGBA(img, g.Opacity)
	}
	return img
}

// compositeOver накладывает src (premultiplied) на dst операцией Over.
func compositeOver(dst, src *image.RGBA) {
	for i := 0; i+3 < len(src.Pix); i += 4 {
		sa := uint32(src.Pix[i+3])
		if sa == 0 {
			continue
		}
		s := src.Pix[i : i+4 : i+4]
		d := dst.Pix[i : i+4 : i+4]
		if sa == 255 {
			copy(d, s)
			continue
		}
		inv := 255 - sa
		d[0] = uint8(uint32(s[0]) + uint32(d[0])*inv/255)
		d[1] = uint8(uint32(s[1]) + uint32(d[1])*inv/255)
		d[2] = uint8(uint32(s[2]) + uint32(d[2])*inv/255)
		d[3] = uint8(sa + uint32(d[3])*inv/255)
	}
}

// scaleRGBA умножает все каналы (premultiplied) на k — групповая непрозрачность.
func scaleRGBA(img *image.RGBA, k float64) {
	if k >= 1 {
		return
	}
	if k < 0 {
		k = 0
	}
	m := uint32(k*256 + 0.5)
	for i, v := range img.Pix {
		if v != 0 {
			img.Pix[i] = uint8((uint32(v)*m + 128) >> 8)
		}
	}
}

// mulRGBAAlpha умножает слой на покрытие маски.
func mulRGBAAlpha(img *image.RGBA, mask *image.Alpha) {
	for i := 0; i < len(mask.Pix); i++ {
		o := i * 4
		if img.Pix[o+3] == 0 {
			continue
		}
		m := uint32(mask.Pix[i])
		if m == 255 {
			continue
		}
		if m == 0 {
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = 0, 0, 0, 0
			continue
		}
		for k := 0; k < 4; k++ {
			img.Pix[o+k] = uint8((uint32(img.Pix[o+k])*m + 127) / 255)
		}
	}
}

// matrixRGBA применяет feColorMatrix ко всему слою (над не премультиплицированным
// цветом).
func matrixRGBA(img *image.RGBA, m *[20]float64) {
	for i := 0; i+3 < len(img.Pix); i += 4 {
		a := uint32(img.Pix[i+3])
		var c color.RGBA
		if a != 0 {
			c = color.RGBA{
				R: uint8(uint32(img.Pix[i]) * 255 / a),
				G: uint8(uint32(img.Pix[i+1]) * 255 / a),
				B: uint8(uint32(img.Pix[i+2]) * 255 / a),
				A: uint8(a),
			}
		}
		c = applyColorMatrix(c, m)
		na := uint32(c.A)
		img.Pix[i] = uint8((uint32(c.R)*na + 127) / 255)
		img.Pix[i+1] = uint8((uint32(c.G)*na + 127) / 255)
		img.Pix[i+2] = uint8((uint32(c.B)*na + 127) / 255)
		img.Pix[i+3] = uint8(na)
	}
}

// blurRGBA размывает слой по Гауссу поканально (premultiplied, так размытие
// линейно по цвету и альфе вместе).
func blurRGBA(img *image.RGBA, sx, sy float64) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(b)
	ch := image.NewAlpha(b)
	for k := 0; k < 4; k++ {
		for i := range ch.Pix {
			ch.Pix[i] = img.Pix[i*4+k]
		}
		bl := blurAlpha(ch, sx, sy)
		for i, v := range bl.Pix {
			out.Pix[i*4+k] = v
		}
	}
	// Покрытие не может быть меньше любого цветового канала (округления).
	for i := 0; i+3 < len(out.Pix); i += 4 {
		a := out.Pix[i+3]
		for k := 0; k < 3; k++ {
			if out.Pix[i+k] > a {
				out.Pix[i+k] = a
			}
		}
	}
	return out
}
