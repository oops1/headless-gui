// systemtray_vertical.go — трей в столбце боковой панели.
//
// В ряду значки идут слева направо и не поместившиеся уходят за шеврон
// справа. В столбце они идут сеткой сверху вниз (колонок столько, сколько
// помещается в толщину панели), а шеврон стоит внизу.
package desktop

import "image"

var _ VerticalItem = (*SystemTray)(nil)

// SetVertical реализует VerticalItem.
func (t *SystemTray) SetVertical(v bool) {
	if t.vertical == v {
		return
	}
	t.vertical = v
	t.relayout()
	t.Invalidate()
}

// verticalCell — размер ячейки сетки: самый крупный из значков.
func (t *SystemTray) verticalCell(avail image.Point) image.Point {
	cell := image.Pt(0, 0)
	for _, it := range t.items {
		sz := it.PreferredSize(avail)
		if sz.X > cell.X {
			cell.X = sz.X
		}
		if sz.Y > cell.Y {
			cell.Y = sz.Y
		}
	}
	if cell.Y <= 0 {
		cell.Y = trayIconSize(t.tm)
	}
	if cell.X <= 0 {
		cell.X = cell.Y
	}
	return cell
}

// verticalGrid считает сетку: сколько колонок помещается в ширину w и сколько
// строк нужно на n значков.
func verticalGrid(w, n int, cell image.Point, gap int) (cols, rows int) {
	cols = 1
	if cell.X > 0 {
		if c := (w + gap) / (cell.X + gap); c > 1 {
			cols = c
		}
	}
	if cols > n {
		cols = n
	}
	if cols < 1 {
		cols = 1
	}
	rows = (n + cols - 1) / cols
	return cols, rows
}

// preferredVertical — желаемый размер в столбце: ширина панели, высота сетки
// (и кнопки раскрытия, если сетка не помещается).
func (t *SystemTray) preferredVertical(avail image.Point) image.Point {
	gap := t.metric(KeyTrayGap)
	cell := t.verticalCell(avail)
	_, rows := verticalGrid(avail.X, len(t.items), cell, gap)
	h := rows*cell.Y + (rows-1)*gap
	if avail.Y > 0 && h > avail.Y {
		h = avail.Y
	}
	return image.Pt(avail.X, h)
}

// relayoutVertical раскладывает значки сеткой сверху вниз; не поместившиеся
// прячутся за шеврон, как в ряду (шеврон занимает место первым — по той же
// причине, что и там).
func (t *SystemTray) relayoutVertical(b image.Rectangle) {
	gap := t.metric(KeyTrayGap)
	avail := image.Pt(b.Dx(), b.Dy())
	cell := t.verticalCell(avail)
	cols, rows := verticalGrid(b.Dx(), len(t.items), cell, gap)

	room := b.Dy()
	if rows*cell.Y+(rows-1)*gap > room {
		room -= t.chevronWidth() + gap
	}
	fit := 0
	if cell.Y+gap > 0 {
		fit = (room + gap) / (cell.Y + gap)
	}
	if fit < 0 {
		fit = 0
	}
	visible := fit * cols
	if visible > len(t.items) {
		visible = len(t.items)
	}

	gridW := cols*cell.X + (cols-1)*gap
	x0 := b.Min.X + (b.Dx()-gridW)/2
	for i, it := range t.items {
		if i >= visible {
			t.hidden = append(t.hidden, it)
			it.SetBounds(image.Rectangle{}) // спрятанный не рисуется и не ловит мышь
			continue
		}
		col, row := i%cols, i/cols
		cx := x0 + col*(cell.X+gap)
		cy := b.Min.Y + row*(cell.Y+gap)
		sz := it.PreferredSize(avail)
		if sz.X <= 0 || sz.X > cell.X {
			sz.X = cell.X
		}
		if sz.Y <= 0 || sz.Y > cell.Y {
			sz.Y = cell.Y
		}
		// Значок поменьше ячейки встаёт по её центру.
		x := cx + (cell.X-sz.X)/2
		y := cy + (cell.Y-sz.Y)/2
		it.SetBounds(image.Rect(x, y, x+sz.X, y+sz.Y).Intersect(b))
	}

	if len(t.hidden) > 0 {
		t.chevron = image.Rect(b.Min.X, b.Max.Y-t.chevronWidth(), b.Max.X, b.Max.Y)
	}
}
