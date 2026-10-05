// quickpanel_layout.go — раскладка быстрых настроек Windows 11: метрики,
// прямоугольники плиток, ползунков, нижней строки и вложенной страницы, зоны
// для попадания мыши и обхода клавиатурой.
//
// Раскладка — чистая арифметика от прямоугольника панели и состояния
// (число плиток, прокрутка, яркость, режим правки), без обращений к
// рисованию: та же функция отдаёт и то, что рисуется, и то, во что попадает
// мышь, так что нажимается ровно то, что видно. Все размеры — метрики темы
// quicksettings.w11.* в логических пикселях.
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// qsKey — общий префикс метрик панели. Константа, чтобы ключи склеивались при
// компиляции, а не при каждой раскладке.
const qsKey = "quicksettings.w11."

// KeyQuickPanelWidth — ширина быстрых настроек Windows 11 (метрика темы).
const KeyQuickPanelWidth theme.Key = qsKey + "width"

// qsMetrics — размеры панели, прочитанные из темы.
type qsMetrics struct {
	width, margin, pad, padTop, cols, rows int
	tileW, tileH, colGap, rowGap           int
	labelGap, labelH, chevW, icon, gridGap int
	sliderH, sliderGap, sliderZone         int
	sliderIcon, sliderTrack, sliderThumb   int
	sliderValue, sliderBottom              int
	footerH, footerPad, footerBtn          int
	headerH, scrollW                       int
}

// pitch — шаг рядов сетки: плитка, подпись под ней и зазор до следующего ряда.
func (m qsMetrics) pitch() int { return m.tileH + m.labelGap + m.labelH + m.rowGap }

// readQSMetrics читает метрики панели. Нет метрики в теме (чужой профиль,
// назвавший презентер, но не размеры) — значения Windows 11 при 100 %.
func readQSMetrics(tm *theme.Manager) qsMetrics {
	g := func(k theme.Key, def int) int {
		if tm != nil {
			if v := int(tm.GetMetric(k)); v > 0 {
				return v
			}
		}
		return def
	}
	return qsMetrics{
		width: g(qsKey+"width", 360), margin: g(qsKey+"margin", 12),
		pad: g(qsKey+"pad", 24), padTop: g(qsKey+"pad.top", 24),
		cols: g(qsKey+"columns", 3), rows: g(qsKey+"rows", 2),
		tileW: g(qsKey+"tile.w", 96), tileH: g(qsKey+"tile.h", 48),
		colGap: g(qsKey+"col.gap", 12), rowGap: g(qsKey+"row.gap", 12),
		labelGap: g(qsKey+"label.gap", 6), labelH: g(qsKey+"label.h", 32),
		chevW: g(qsKey+"chev.w", 28), icon: g(qsKey+"icon", 20), gridGap: g(qsKey+"grid.gap", 16),
		sliderH: g(qsKey+"slider.h", 40), sliderGap: g(qsKey+"slider.gap", 4), sliderZone: g(qsKey+"slider.zone", 32),
		sliderIcon: g(qsKey+"slider.icon", 20), sliderTrack: g(qsKey+"slider.track", 4),
		sliderThumb: g(qsKey+"slider.thumb", 20), sliderValue: g(qsKey+"slider.value", 32),
		sliderBottom: g(qsKey+"slider.bottom", 16),
		footerH:      g(qsKey+"footer.h", 48), footerPad: g(qsKey+"footer.pad", 16), footerBtn: g(qsKey+"footer.btn", 32),
		headerH: g(qsKey+"header.h", 48), scrollW: g(qsKey+"scrollbar.w", 4),
	}
}

// qsZoneKind — вид зоны панели: то, что можно навести, нажать и обойти Tab.
type qsZoneKind uint8

const (
	qzNone       qsZoneKind = iota
	qzTile                  // плитка (idx — номер в списке)
	qzChevron               // «›» на плитке (idx — номер плитки)
	qzBrightIcon            // значок яркости
	qzBright                // ползунок яркости
	qzVolIcon               // значок громкости (выключает звук)
	qzVol                   // ползунок громкости
	qzVolChevron            // «›» у громкости
	qzEdit                  // карандаш / «Готово»
	qzSettings              // шестерёнка
	qzBack                  // «‹» вложенной страницы
	qzContent               // содержимое вложенной страницы
	qzThumb                 // бегунок прокрутки сетки
)

// qsZone — адрес зоны. Сравнимое значение: служит ключом переходов цвета.
type qsZone struct {
	kind qsZoneKind
	idx  int
}

// qsRow — строка ползунка: значок слева, дорожка, значение, «›».
type qsRow struct {
	row, icon, track, value, chev image.Rectangle
}

// qsLayout — раскладка панели для одного кадра или одного события.
type qsLayout struct {
	m     qsMetrics
	panel image.Rectangle
	n     int // плиток в списке

	rows     int // рядов плиток всего
	grid     image.Rectangle
	contentH int
	maxScrl  int
	scroll   int
	// pos[p] — номер плитки списка, стоящей на месте p. nil — по порядку;
	// при перетаскивании там порядок с учётом места, куда лягет плитка.
	pos []int

	bright, vol qsRow // пустые row — ползунка нет
	hint        image.Rectangle
	footer      image.Rectangle
	battery     image.Rectangle
	edit        image.Rectangle
	settings    image.Rectangle
	thumb       image.Rectangle

	header, back, body image.Rectangle
}

// qsSliderRows — сколько строк занимают ползунки (громкость, яркость).
func qsSliderRows(hasVol, hasBright bool) int {
	n := 0
	if hasVol {
		n++
	}
	if hasBright {
		n++
	}
	return n
}

// qsHeight — высота панели: поля, видимые ряды плиток, ползунки, нижняя
// строка. Не зависит от режима правки и страницы — панель не прыгает.
func qsHeight(m qsMetrics, n int, hasVol, hasBright bool) int {
	h := m.padTop
	if n > 0 {
		rows := (n + m.cols - 1) / m.cols
		if rows > m.rows {
			rows = m.rows
		}
		h += rows*m.pitch() - m.rowGap + m.gridGap
	}
	if k := qsSliderRows(hasVol, hasBright); k > 0 {
		h += k*m.sliderH + (k-1)*m.sliderGap
	}
	return h + m.sliderBottom + m.footerH
}

// qsViewState — то, что раскладке нужно знать о панели, помимо метрик.
type qsViewState struct {
	n         int
	scroll    int
	hasVol    bool
	volChev   bool
	hasBright bool
	editing   bool
	hasBat    bool
	editLabel string // подпись кнопки правки в режиме правки; пусто — карандаш
	pos       []int
}

// qsLayoutFor считает раскладку панели panel.
func qsLayoutFor(m qsMetrics, panel image.Rectangle, v qsViewState) *qsLayout {
	l := &qsLayout{m: m, panel: panel, n: v.n, pos: v.pos}
	x0, x1 := panel.Min.X+m.pad, panel.Max.X-m.pad
	y := panel.Min.Y + m.padTop

	if v.n > 0 {
		l.rows = (v.n + m.cols - 1) / m.cols
		vis := l.rows
		if vis > m.rows {
			vis = m.rows
		}
		l.contentH = l.rows*m.pitch() - m.rowGap
		viewH := vis*m.pitch() - m.rowGap
		l.grid = image.Rect(x0, y, x1, y+viewH)
		l.maxScrl = l.contentH - viewH
		if l.maxScrl < 0 {
			l.maxScrl = 0
		}
		l.scroll = v.scroll
		if l.scroll > l.maxScrl {
			l.scroll = l.maxScrl
		}
		if l.scroll < 0 {
			l.scroll = 0
		}
		y += viewH + m.gridGap
	}

	rowAt := func(top int, chev bool) qsRow {
		r := image.Rect(x0, top, x1, top+m.sliderH)
		zone := m.sliderZone
		cy := top + m.sliderH/2
		sq := func(x int) image.Rectangle { return image.Rect(x, cy-zone/2, x+zone, cy-zone/2+zone) }
		out := qsRow{row: r, icon: sq(x0)}
		right := x1
		if chev {
			out.chev = sq(x1 - zone)
			right = out.chev.Min.X
		}
		out.value = image.Rect(right-m.sliderValue, top, right, top+m.sliderH)
		out.track = image.Rect(out.icon.Max.X+8, top, out.value.Min.X-4, top+m.sliderH)
		return out
	}
	sliderTop := y
	if v.hasBright {
		l.bright = rowAt(y, false)
		y += m.sliderH + m.sliderGap
	}
	if v.hasVol {
		l.vol = rowAt(y, v.volChev)
		y += m.sliderH
	}
	if k := qsSliderRows(v.hasVol, v.hasBright); k > 0 {
		l.hint = image.Rect(x0, sliderTop, x1, sliderTop+k*m.sliderH+(k-1)*m.sliderGap)
	}

	l.footer = image.Rect(panel.Min.X, panel.Max.Y-m.footerH, panel.Max.X, panel.Max.Y)
	fy := l.footer.Min.Y + (m.footerH-m.footerBtn)/2
	btn := func(x int) image.Rectangle { return image.Rect(x, fy, x+m.footerBtn, fy+m.footerBtn) }
	l.settings = btn(l.footer.Max.X - m.footerPad - m.footerBtn)
	editW := m.footerBtn
	if v.editLabel != "" {
		editW = widget.MeasureUIText(v.editLabel, widget.DefaultFontSize()) + 24
		if editW < m.footerBtn {
			editW = m.footerBtn
		}
	}
	l.edit = image.Rect(l.settings.Min.X-4-editW, fy, l.settings.Min.X-4, fy+m.footerBtn)
	if v.hasBat {
		l.battery = image.Rect(l.footer.Min.X+m.footerPad, l.footer.Min.Y, l.edit.Min.X-8, l.footer.Max.Y)
	}

	l.header = image.Rect(panel.Min.X, panel.Min.Y, panel.Max.X, panel.Min.Y+m.headerH)
	bs := m.footerBtn
	l.back = image.Rect(panel.Min.X+m.footerPad, l.header.Min.Y+(m.headerH-bs)/2,
		panel.Min.X+m.footerPad+bs, l.header.Min.Y+(m.headerH-bs)/2+bs)
	l.body = image.Rect(x0, l.header.Max.Y, x1, panel.Max.Y-m.pad)

	l.thumb = l.scrollThumb()
	return l
}

// tile возвращает прямоугольники плитки, стоящей на месте p: сама плитка и
// подпись под ней.
func (l *qsLayout) tile(p int) (body, label image.Rectangle) {
	col, row := p%l.m.cols, p/l.m.cols
	x := l.panel.Min.X + l.m.pad + col*(l.m.tileW+l.m.colGap)
	y := l.grid.Min.Y + row*l.m.pitch() - l.scroll
	body = image.Rect(x, y, x+l.m.tileW, y+l.m.tileH)
	lx := x - l.m.colGap/2
	label = image.Rect(lx, y+l.m.tileH+l.m.labelGap, lx+l.m.tileW+l.m.colGap, y+l.m.tileH+l.m.labelGap+l.m.labelH)
	return
}

// chevron возвращает зону «›» правой части плитки.
func (l *qsLayout) chevron(body image.Rectangle) image.Rectangle {
	return image.Rect(body.Max.X-l.m.chevW, body.Min.Y, body.Max.X, body.Max.Y)
}

// place возвращает место плитки списка idx: p такое, что pos[p] == idx.
func (l *qsLayout) place(idx int) int {
	if l.pos == nil {
		return idx
	}
	for p, i := range l.pos {
		if i == idx {
			return p
		}
	}
	return idx
}

// at возвращает номер плитки списка, стоящей на месте p.
func (l *qsLayout) at(p int) int {
	if l.pos == nil {
		return p
	}
	return l.pos[p]
}

// tileRects возвращает прямоугольники плитки списка idx с учётом порядка
// показа.
func (l *qsLayout) tileRects(idx int) (body, label image.Rectangle) {
	return l.tile(l.place(idx))
}

// scrollThumb считает бегунок прокрутки в правом поле панели; пустой, если
// сетка помещается.
func (l *qsLayout) scrollThumb() image.Rectangle {
	if l.maxScrl <= 0 || l.grid.Empty() {
		return image.Rectangle{}
	}
	x := l.panel.Max.X - l.m.pad/2 - l.m.scrollW/2
	h := l.grid.Dy() * l.grid.Dy() / l.contentH
	if h < 24 {
		h = 24
	}
	if h > l.grid.Dy() {
		h = l.grid.Dy()
	}
	y := l.grid.Min.Y + (l.grid.Dy()-h)*l.scroll/l.maxScrl
	return image.Rect(x, y, x+l.m.scrollW, y+h)
}

// thumbHit — зона захвата бегунка: шире и выше самого бегунка, чтобы в него
// можно было попасть.
func (l *qsLayout) thumbHit() image.Rectangle {
	t := l.scrollThumb()
	if t.Empty() {
		return t
	}
	return image.Rect(t.Min.X-4, t.Min.Y, l.panel.Max.X, t.Max.Y)
}

// zoneAt находит зону главной страницы под точкой pt. chevrons — рисуются ли
// «›» на плитках (в режиме правки их нет), list — плитки списка (нужны для
// признака HasDetails).
func (l *qsLayout) zoneAt(pt image.Point, list []QuickAction, chevrons bool) qsZone {
	if !pt.In(l.panel) {
		return qsZone{}
	}
	if pt.In(l.footer) {
		switch {
		case pt.In(l.settings):
			return qsZone{kind: qzSettings}
		case pt.In(l.edit):
			return qsZone{kind: qzEdit}
		}
		return qsZone{}
	}
	if pt.In(l.thumbHit()) {
		return qsZone{kind: qzThumb}
	}
	if pt.In(l.grid) {
		for p := 0; p < l.n; p++ {
			body, _ := l.tile(p)
			if !pt.In(body) {
				continue
			}
			idx := l.at(p)
			if chevrons && idx < len(list) && list[idx].HasDetails && pt.In(l.chevron(body)) {
				return qsZone{kind: qzChevron, idx: idx}
			}
			return qsZone{kind: qzTile, idx: idx}
		}
		return qsZone{}
	}
	if !l.bright.row.Empty() && pt.In(l.bright.row) {
		if pt.In(l.bright.icon) {
			return qsZone{kind: qzBrightIcon}
		}
		return qsZone{kind: qzBright}
	}
	if !l.vol.row.Empty() && pt.In(l.vol.row) {
		switch {
		case pt.In(l.vol.icon):
			return qsZone{kind: qzVolIcon}
		case !l.vol.chev.Empty() && pt.In(l.vol.chev):
			return qsZone{kind: qzVolChevron}
		}
		return qsZone{kind: qzVol}
	}
	return qsZone{}
}

// zoneRect — область зоны, которую надо перерисовать, когда меняется её вид.
func (l *qsLayout) zoneRect(z qsZone) image.Rectangle {
	switch z.kind {
	case qzTile, qzChevron:
		body, _ := l.tileRects(z.idx)
		return body.Intersect(l.grid)
	case qzBrightIcon:
		return l.bright.icon
	case qzBright:
		return l.bright.row
	case qzVolIcon:
		return l.vol.icon
	case qzVol:
		return l.vol.row
	case qzVolChevron:
		return l.vol.chev
	case qzEdit:
		return l.edit
	case qzSettings:
		return l.settings
	case qzBack:
		return l.back
	case qzThumb:
		return l.thumb
	}
	return image.Rectangle{}
}

// focusRect — рамка клавиатурного фокуса зоны.
func (l *qsLayout) focusRect(z qsZone) image.Rectangle {
	switch z.kind {
	case qzTile:
		body, _ := l.tileRects(z.idx)
		return body
	case qzChevron:
		body, _ := l.tileRects(z.idx)
		return l.chevron(body)
	case qzBright:
		return l.bright.row
	case qzVol:
		return l.vol.row
	}
	return l.zoneRect(z)
}

// focusOrder перечисляет зоны в порядке обхода Tab: плитки (за каждой — её
// «›»), ползунки, нижняя строка. В режиме правки — плитки и «Готово».
func (l *qsLayout) focusOrder(list []QuickAction, editing bool) []qsZone {
	out := make([]qsZone, 0, l.n*2+8)
	for p := 0; p < l.n; p++ {
		idx := l.at(p)
		out = append(out, qsZone{kind: qzTile, idx: idx})
		if !editing && idx < len(list) && list[idx].HasDetails {
			out = append(out, qsZone{kind: qzChevron, idx: idx})
		}
	}
	if !editing {
		if !l.bright.row.Empty() {
			out = append(out, qsZone{kind: qzBright})
		}
		if !l.vol.row.Empty() {
			out = append(out, qsZone{kind: qzVolIcon}, qsZone{kind: qzVol})
			if !l.vol.chev.Empty() {
				out = append(out, qsZone{kind: qzVolChevron})
			}
		}
	}
	return append(out, qsZone{kind: qzEdit}, qsZone{kind: qzSettings})
}

// slotAt возвращает место сетки под точкой pt: куда ляжет плитка, брошенная
// здесь. Точка за пределами сетки приводится к ближайшему месту.
func (l *qsLayout) slotAt(pt image.Point) int {
	if l.n == 0 {
		return 0
	}
	col := (pt.X - (l.panel.Min.X + l.m.pad)) / (l.m.tileW + l.m.colGap)
	if col < 0 {
		col = 0
	}
	if col >= l.m.cols {
		col = l.m.cols - 1
	}
	row := (pt.Y - l.grid.Min.Y + l.scroll) / l.m.pitch()
	if row < 0 {
		row = 0
	}
	slot := row*l.m.cols + col
	if slot >= l.n {
		slot = l.n - 1
	}
	return slot
}

// qsDisplayOrder — порядок показа плиток при перетаскивании: плитка drag
// вынута и вставлена на место slot.
func qsDisplayOrder(n, drag, slot int) []int {
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if i != drag {
			out = append(out, i)
		}
	}
	if slot > len(out) {
		slot = len(out)
	}
	if slot < 0 {
		slot = 0
	}
	out = append(out, 0)
	copy(out[slot+1:], out[slot:])
	out[slot] = drag
	return out
}

// level переводит координату x над дорожкой в уровень 0..1: бегунок
// ходит на ширину дорожки минус свой диаметр.
func (r qsRow) level(x, thumb int) float64 {
	span := r.track.Dx() - thumb
	if span <= 0 {
		return 0
	}
	v := float64(x-(r.track.Min.X+thumb/2)) / float64(span)
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// thumbCenter — координата центра бегунка при уровне level.
func (r qsRow) thumbCenter(level float64, thumb int) int {
	span := r.track.Dx() - thumb
	if span < 0 {
		span = 0
	}
	return r.track.Min.X + thumb/2 + int(level*float64(span)+0.5)
}
