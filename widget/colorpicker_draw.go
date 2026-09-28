package widget

import (
	"image"
	"image/color"
	"strconv"
)

// colorpicker_draw.go — как выглядят поле цвета и его палитра.
//
// Поле: образец, код и стрелка. Палитра: сетка образцов, три полосы каналов и
// строка предпросмотра. Геометрия считается чистыми функциями от
// прямоугольника поля — теми же, по которым ищет попадание мышь, иначе
// нарисованное и нажимаемое рано или поздно разъедутся.

const (
	cpButtonW    = 22 // кнопка со стрелкой у правого края поля
	cpSwatchW    = 26 // образец цвета в поле
	cpCellW      = 22 // сторона образца в палитре
	cpCellGap    = 4
	cpPadding    = 10
	cpBandH      = 14 // высота полосы канала
	cpBandGap    = 8
	cpBandLabelW = 16 // колонка с буквой канала
	cpBandValueW = 34 // колонка с числом канала
	cpPreviewH   = 26
)

// cpButtonRect — кнопка со стрелкой у правого края поля.
func cpButtonRect(field image.Rectangle) image.Rectangle {
	return image.Rect(field.Max.X-cpButtonW, field.Min.Y, field.Max.X, field.Max.Y)
}

// cpSwatchRect — образец цвета внутри поля.
func cpSwatchRect(field image.Rectangle) image.Rectangle {
	const pad = 4
	return image.Rect(field.Min.X+pad, field.Min.Y+pad,
		field.Min.X+pad+cpSwatchW, field.Max.Y-pad)
}

// cpDropRect — прямоугольник раскрытой палитры под полем.
//
// Ширина — по сетке образцов, а не по полю: узкое поле не должно сминать
// палитру, широкое — растягивать её на весь экран.
func cpDropRect(field image.Rectangle, rows int) image.Rectangle {
	w := cpPadding*2 + cpPaletteCols*cpCellW + (cpPaletteCols-1)*cpCellGap
	h := cpPadding*2 + rows*cpCellW + (rows-1)*cpCellGap + // сетка
		cpBandGap + 3*(cpBandH+cpBandGap) + // три канала
		cpPreviewH
	return image.Rect(field.Min.X, field.Max.Y+2, field.Min.X+w, field.Max.Y+2+h)
}

// cpRows — сколько рядов в сетке образцов.
func cpRows(n int) int {
	if n <= 0 {
		return 0
	}
	return (n + cpPaletteCols - 1) / cpPaletteCols
}

// cpCellRect — прямоугольник образца i в раскрытой палитре.
func cpCellRect(drop image.Rectangle, i int) image.Rectangle {
	row, col := i/cpPaletteCols, i%cpPaletteCols
	x := drop.Min.X + cpPadding + col*(cpCellW+cpCellGap)
	y := drop.Min.Y + cpPadding + row*(cpCellW+cpCellGap)
	return image.Rect(x, y, x+cpCellW, y+cpCellW)
}

// cpBandRect — полоса канала ch (0..2) в раскрытой палитре: только дорожка,
// без буквы и числа по краям.
func cpBandRect(drop image.Rectangle, rows, ch int) image.Rectangle {
	top := drop.Min.Y + cpPadding + rows*cpCellW + (rows-1)*cpCellGap + cpBandGap
	y := top + ch*(cpBandH+cpBandGap)
	x0 := drop.Min.X + cpPadding + cpBandLabelW
	x1 := drop.Max.X - cpPadding - cpBandValueW
	return image.Rect(x0, y, x1, y+cpBandH)
}

// cpPreviewRect — полоса предпросмотра внизу палитры.
func cpPreviewRect(drop image.Rectangle) image.Rectangle {
	return image.Rect(drop.Min.X+cpPadding, drop.Max.Y-cpPadding-cpPreviewH+cpBandGap,
		drop.Max.X-cpPadding, drop.Max.Y-cpPadding)
}

// cpValueAtX — значение канала (0..255) для точки x на дорожке.
func cpValueAtX(band image.Rectangle, x int) int {
	if band.Dx() <= 1 {
		return 0
	}
	v := (x - band.Min.X) * 255 / (band.Dx() - 1)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func (p *ColorPicker) Draw(ctx DrawContext) {
	b := p.Base.Bounds()
	if b.Empty() {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	pal := &p.pal
	st := currentStyle()
	size := fontSizeOrDefault(p.FontSize)

	border := pal.border
	switch {
	case p.invalid:
		border = pal.err
	case p.focused || p.open:
		border = pal.focus
	}
	switch {
	case st.Classic3D:
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), pal.bg)
		drawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
		if p.invalid {
			ctx.DrawBorder(b.Min.X+2, b.Min.Y+2, b.Dx()-4, b.Dy()-4, pal.err)
		}
	case st.ControlCorner > 0:
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st.ControlCorner, pal.bg)
		ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st.ControlCorner, border)
	default:
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), pal.bg)
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), border)
	}

	// Образец: рамкой всегда, иначе белый цвет на белом поле пропадает.
	sw := cpSwatchRect(b)
	cpDrawSwatch(ctx, sw, p.value, pal.border, st)

	// Код цвета.
	textX := sw.Max.X + 8
	textY := b.Min.Y + (b.Dy()-13)/2
	maxW := cpButtonRect(b).Min.X - 4 - textX
	text := p.textLocked()
	col := pal.text
	if p.invalid {
		col = pal.err
	}
	shown := ellipsizeText(ctx, text, maxW, size)
	if p.focused && p.selectAll && text != "" {
		w := min(ctx.MeasureText(shown, size), maxW)
		ctx.FillRectAlpha(textX-1, b.Min.Y+4, w+2, b.Dy()-8, premulAlpha(pal.sel, 200))
	}
	ctx.DrawTextSize(shown, textX, textY, size, col)
	if p.focused && p.editing && !p.selectAll {
		cx := textX + min(ctx.MeasureText(string(p.text), size), maxW)
		ctx.FillRect(cx, b.Min.Y+6, 1, b.Dy()-12, pal.text)
	}

	// Стрелка раскрытия — та же, что у выпадающего списка.
	br := cpButtonRect(b)
	glyph := pal.glyph
	if p.open {
		glyph = pal.focus
	}
	drawChevronDown(ctx, br.Min.X+br.Dx()/2, br.Min.Y+br.Dy()/2, glyph)

	p.drawChildren(ctx)
	p.drawDisabledOverlay(ctx)
}

// cpDrawSwatch рисует прямоугольник цвета с рамкой: без неё белый образец на
// белом фоне и чёрный на тёмном пропадают вовсе.
func cpDrawSwatch(ctx DrawContext, r image.Rectangle, c, border color.RGBA, st ThemeStyle) {
	if r.Empty() {
		return
	}
	if st.ControlCorner > 0 {
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 3, c)
		ctx.DrawRoundBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 3, border)
		return
	}
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), c)
	ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), border)
}

// HasOverlay — контракт OverlayDrawer: палитра рисуется поверх соседей.
func (p *ColorPicker) HasOverlay() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.open
}

// OverlayBounds — контракт OverlayBoundsProvider: прямоугольник палитры.
func (p *ColorPicker) OverlayBounds() image.Rectangle {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.open {
		return image.Rectangle{}
	}
	return cpDropRect(p.Base.Bounds(), cpRows(len(p.palette)))
}

// DrawOverlay рисует раскрытую палитру.
func (p *ColorPicker) DrawOverlay(ctx DrawContext) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.open {
		return
	}
	pal := &p.pal
	st := currentStyle()
	size := fontSizeOrDefault(p.FontSize)
	rows := cpRows(len(p.palette))
	drop := cpDropRect(p.Base.Bounds(), rows)

	if st.Classic3D {
		ctx.FillRect(drop.Min.X, drop.Min.Y, drop.Dx(), drop.Dy(), pal.dropBG)
		drawBevelRaised(ctx, drop.Min.X, drop.Min.Y, drop.Dx(), drop.Dy(), st)
	} else {
		if sd, ok := ctx.(ShadowDrawer); ok {
			sd.DrawSoftShadow(drop, 6, 2, color.RGBA{A: 40})
		}
		ctx.FillRoundRect(drop.Min.X, drop.Min.Y, drop.Dx(), drop.Dy(), 6, pal.dropBG)
		ctx.DrawRoundBorder(drop.Min.X, drop.Min.Y, drop.Dx(), drop.Dy(), 6, pal.dropBorder)
	}

	// Сетка образцов.
	for i, c := range p.palette {
		r := cpCellRect(drop, i)
		cpDrawSwatch(ctx, r, c, pal.dropBorder, st)
		switch {
		case c == p.value:
			// Выбранный обведён акцентом, а не галочкой: галочка на тёмном
			// образце сливается, а рамка видна на любом цвете.
			ctx.DrawBorder(r.Min.X-2, r.Min.Y-2, r.Dx()+4, r.Dy()+4, pal.accent)
		case i == p.hoverSwatch || (i == p.cursor && p.hoverSwatch < 0):
			ctx.DrawBorder(r.Min.X-2, r.Min.Y-2, r.Dx()+4, r.Dy()+4, pal.muted)
		}
	}

	// Полосы каналов: дорожка с переливом от нуля до максимума этого канала,
	// чтобы видно было, к чему ведёт движение ползунка.
	names := [3]string{"R", "G", "B"}
	for ch := 0; ch < 3; ch++ {
		band := cpBandRect(drop, rows, ch)
		ctx.DrawTextSize(names[ch], drop.Min.X+cpPadding, band.Min.Y+(cpBandH-13)/2+2,
			size, pal.muted)
		cpDrawBand(ctx, band, p.value, ch)
		ctx.DrawBorder(band.Min.X, band.Min.Y, band.Dx(), band.Dy(), pal.dropBorder)

		// Ползунок канала.
		v := cpChannel(p.value, ch)
		x := band.Min.X + v*(band.Dx()-1)/255
		ctx.FillRect(x-1, band.Min.Y-2, 3, band.Dy()+4, pal.dropText)
		ctx.DrawBorder(x-2, band.Min.Y-3, 5, band.Dy()+6, pal.dropBG)

		ctx.DrawTextSize(strconv.Itoa(v), band.Max.X+8, band.Min.Y+(cpBandH-13)/2+2,
			size, pal.dropText)
	}

	// Предпросмотр: крупный образец и код рядом.
	pv := cpPreviewRect(drop)
	sw := image.Rect(pv.Min.X, pv.Min.Y, pv.Min.X+cpSwatchW*2, pv.Max.Y)
	cpDrawSwatch(ctx, sw, p.value, pal.dropBorder, st)
	ctx.DrawTextSize(HexColor(p.value), sw.Max.X+8, pv.Min.Y+(pv.Dy()-13)/2, size, pal.dropText)
}

// cpDrawBand рисует дорожку канала: столбцами по 4 точки — перелив без
// попиксельной отрисовки, которой в контексте всё равно нет.
func cpDrawBand(ctx DrawContext, band image.Rectangle, base color.RGBA, ch int) {
	const step = 4
	for x := band.Min.X; x < band.Max.X; x += step {
		v := cpValueAtX(band, x)
		c := base
		switch ch {
		case 0:
			c.R = uint8(v)
		case 1:
			c.G = uint8(v)
		case 2:
			c.B = uint8(v)
		}
		w := step
		if x+w > band.Max.X {
			w = band.Max.X - x
		}
		ctx.FillRect(x, band.Min.Y, w, band.Dy(), c)
	}
}
