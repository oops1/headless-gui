package widget

import (
	"image"
	"image/color"
	"strconv"
	"time"

	"github.com/oops1/headless-gui/v3/internal/calendar"
)

// datepicker_draw.go — геометрия и отрисовка поля даты и календаря.
//
// Раскладка календаря считается ОДНОЙ функцией (dpLayoutFor) и для отрисовки, и
// для разбора мыши: день под щелчком гарантированно тот, что нарисован в этой
// клетке.

// Геометрия календаря в логических точках.
const (
	dpPad     = 8
	dpHeaderH = 32
	dpWeekH   = 22
	dpCellW   = 32
	dpCellH   = 26
	dpRows    = 6 // места всегда на шесть недель: календарь не прыгает по высоте
	dpButtonW = 28
)

// Что под точкой в календаре.
const (
	dpHitNone = iota
	dpHitPrev
	dpHitNext
	dpHitDay
)

// dpCalendarRect — прямоугольник календаря под полем.
func dpCalendarRect(field image.Rectangle) image.Rectangle {
	w := max(field.Dx(), 7*dpCellW+2*dpPad)
	h := dpPad + dpHeaderH + dpWeekH + dpRows*dpCellH + dpPad
	return image.Rect(field.Min.X, field.Max.Y+2, field.Min.X+w, field.Max.Y+2+h)
}

type dpCell struct {
	rect image.Rectangle
	day  calendar.Day
}

type dpLayout struct {
	cal, prev, next, title image.Rectangle
	weekday                [7]image.Rectangle
	cells                  [][]dpCell
}

// dpLayoutFor раскладывает календарь месяца view.
func dpLayoutFor(field image.Rectangle, view time.Time, first time.Weekday) dpLayout {
	var l dpLayout
	l.cal = dpCalendarRect(field)
	x0 := l.cal.Min.X + (l.cal.Dx()-7*dpCellW)/2
	y := l.cal.Min.Y + dpPad
	l.prev = image.Rect(x0, y, x0+dpCellW, y+dpHeaderH)
	l.next = image.Rect(x0+6*dpCellW, y, x0+7*dpCellW, y+dpHeaderH)
	l.title = image.Rect(l.prev.Max.X, y, l.next.Min.X, y+dpHeaderH)
	y += dpHeaderH
	for c := 0; c < 7; c++ {
		l.weekday[c] = image.Rect(x0+c*dpCellW, y, x0+(c+1)*dpCellW, y+dpWeekH)
	}
	y += dpWeekH
	grid := calendar.MonthGrid(view.Year(), view.Month(), view.Location(), first)
	l.cells = make([][]dpCell, len(grid))
	for r, week := range grid {
		l.cells[r] = make([]dpCell, 7)
		for c, d := range week {
			cx, cy := x0+c*dpCellW, y+r*dpCellH
			l.cells[r][c] = dpCell{rect: image.Rect(cx, cy, cx+dpCellW, cy+dpCellH), day: d}
		}
	}
	return l
}

// hitLocked — что под точкой в открытом календаре.
func (p *DatePicker) hitLocked(x, y int) (int, time.Time) {
	l := dpLayoutFor(p.Base.Bounds(), p.view, p.firstDayLocked())
	pt := image.Pt(x, y)
	switch {
	case pt.In(l.prev):
		return dpHitPrev, time.Time{}
	case pt.In(l.next):
		return dpHitNext, time.Time{}
	}
	for _, week := range l.cells {
		for _, c := range week {
			if pt.In(c.rect) {
				return dpHitDay, c.day.Date
			}
		}
	}
	return dpHitNone, time.Time{}
}

// buttonRect — кнопка календаря у правого края поля.
func dpButtonRect(field image.Rectangle) image.Rectangle {
	return image.Rect(field.Max.X-dpButtonW, field.Min.Y, field.Max.X, field.Max.Y)
}

func (p *DatePicker) Draw(ctx DrawContext) {
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

	// Текст или подсказка — до кнопки календаря, с многоточием.
	textX := b.Min.X + 8
	textY := b.Min.Y + (b.Dy()-13)/2
	maxW := dpButtonRect(b).Min.X - 4 - textX
	text, col := p.textLocked(), pal.text
	if text == "" && !p.editing {
		text, col = p.Placeholder, pal.placeholder
		if text == "" {
			text = Tr("date.placeholder")
		}
	}
	shown := ellipsizeText(ctx, text, maxW, size)
	if p.focused && p.selectAll && p.textLocked() != "" {
		w := min(ctx.MeasureText(shown, size), maxW)
		ctx.FillRectAlpha(textX-1, b.Min.Y+4, w+2, b.Dy()-8, premulAlpha(pal.sel, 200))
	}
	ctx.DrawTextSize(shown, textX, textY, size, col)
	if p.focused && p.editing && !p.selectAll {
		cx := textX + min(ctx.MeasureText(string(p.text), size), maxW)
		ctx.FillRect(cx, b.Min.Y+6, 1, b.Dy()-12, pal.text)
	}

	p.drawGlyph(ctx, dpButtonRect(b))
	p.drawChildren(ctx)
	p.drawDisabledOverlay(ctx)
}

// drawGlyph рисует значок календаря: лист с полосой сверху, двумя кольцами и
// клетками дней. Прямоугольниками, а не символом шрифта: в встроенных шрифтах
// календарного глифа нет.
func (p *DatePicker) drawGlyph(ctx DrawContext, r image.Rectangle) {
	col := p.pal.glyph
	if p.open {
		col = p.pal.focus
	}
	w, h := 14, 13
	x := r.Min.X + (r.Dx()-w)/2
	y := r.Min.Y + (r.Dy()-h)/2 + 1
	ctx.DrawBorder(x, y, w, h, col)
	ctx.FillRect(x, y, w, 3, col)
	ctx.FillRect(x+3, y-2, 1, 3, col)
	ctx.FillRect(x+w-4, y-2, 1, 3, col)
	for row := 0; row < 2; row++ {
		for c := 0; c < 3; c++ {
			ctx.FillRect(x+2+c*4, y+5+row*3, 2, 2, col)
		}
	}
}

// HasOverlay — контракт OverlayDrawer: календарь рисуется поверх соседей.
func (p *DatePicker) HasOverlay() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.open
}

// OverlayBounds — контракт OverlayBoundsProvider: прямоугольник календаря.
func (p *DatePicker) OverlayBounds() image.Rectangle {
	p.mu.Lock()
	open := p.open
	p.mu.Unlock()
	if !open {
		return image.Rectangle{}
	}
	return dpCalendarRect(p.Base.Bounds())
}

// DrawOverlay рисует открытый календарь.
func (p *DatePicker) DrawOverlay(ctx DrawContext) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.open {
		return
	}
	pal := &p.pal
	st := currentStyle()
	size := fontSizeOrDefault(p.FontSize)
	today := calendar.DateOnly(p.now())
	first := p.firstDayLocked()
	l := dpLayoutFor(p.Base.Bounds(), p.view, first)
	c := l.cal

	corner := 0
	if !st.Classic3D {
		corner = st.ControlCorner
		if sd, ok := ctx.(ShadowDrawer); ok {
			sd.DrawSoftShadow(c, corner, 8, pal.dropBorder)
		}
	}
	if corner > 0 {
		ctx.FillRoundRect(c.Min.X, c.Min.Y, c.Dx(), c.Dy(), corner, pal.dropBG)
		ctx.DrawRoundBorder(c.Min.X, c.Min.Y, c.Dx(), c.Dy(), corner, pal.dropBorder)
	} else {
		ctx.FillRect(c.Min.X, c.Min.Y, c.Dx(), c.Dy(), pal.dropBG)
		ctx.DrawBorder(c.Min.X, c.Min.Y, c.Dx(), c.Dy(), pal.dropBorder)
	}

	// Шапка: стрелки месяцев и «Сентябрь 2026».
	for i, r := range []image.Rectangle{l.prev, l.next} {
		delta := -1 + 2*i
		col := pal.dropText
		switch {
		case !p.canShiftLocked(delta):
			col = pal.disabled
		case p.hoverAt == dpHitPrev+i:
			ctx.FillRoundRect(r.Min.X+2, r.Min.Y+3, r.Dx()-4, r.Dy()-6, 4, pal.hoverBG)
			col = pal.hoverText
		}
		dpChevron(ctx, r, delta < 0, col)
	}
	title := Tr("date.month."+strconv.Itoa(int(p.view.Month()))) + " " + strconv.Itoa(p.view.Year())
	tw := ctx.MeasureText(title, size)
	ctx.DrawTextSize(title, l.title.Min.X+(l.title.Dx()-tw)/2, l.title.Min.Y+(l.title.Dy()-13)/2, size, pal.dropText)

	// Дни недели — от первого дня культуры.
	for col := 0; col < 7; col++ {
		name := Tr("date.wd." + strconv.Itoa((int(first)+col)%7))
		r := l.weekday[col]
		nw := ctx.MeasureText(name, size-1)
		ctx.DrawTextSize(name, r.Min.X+(r.Dx()-nw)/2, r.Min.Y+(r.Dy()-12)/2, size-1, pal.muted)
	}

	for _, week := range l.cells {
		for _, cell := range week {
			d := cell.day.Date
			r := cell.rect.Inset(2)
			enabled := p.inRangeLocked(d)
			text := pal.dropText
			switch {
			case !enabled:
				text = pal.disabled
			case !cell.day.InMonth:
				text = pal.muted
			}
			switch {
			case enabled && !p.selected.IsZero() && calendar.SameDay(d, p.selected):
				ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, pal.accent)
				text = pal.accentText
			case enabled && !p.hover.IsZero() && calendar.SameDay(d, p.hover):
				ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, pal.hoverBG)
				text = pal.hoverText
			}
			if calendar.SameDay(d, today) {
				ctx.DrawRoundBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, pal.accent)
			}
			if p.focused && calendar.SameDay(d, p.cursor) && !calendar.SameDay(d, p.selected) {
				ctx.DrawRoundBorder(r.Min.X+1, r.Min.Y+1, r.Dx()-2, r.Dy()-2, 3, pal.focus)
			}
			num := strconv.Itoa(d.Day())
			nw := ctx.MeasureText(num, size)
			ctx.DrawTextSize(num, cell.rect.Min.X+(cell.rect.Dx()-nw)/2,
				cell.rect.Min.Y+(cell.rect.Dy()-13)/2, size, text)
		}
	}
}

// dpChevron — стрелка месяца «‹» или «›» одной ломаной.
func dpChevron(ctx DrawContext, r image.Rectangle, left bool, col color.RGBA) {
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	dir := 1
	if left {
		dir = -1
	}
	strokePolyline(ctx, []image.Point{
		{X: cx - dir*2, Y: cy - 5},
		{X: cx + dir*3, Y: cy},
		{X: cx - dir*2, Y: cy + 5},
	}, 1.5, false, col)
}
