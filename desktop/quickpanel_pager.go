// quickpanel_pager.go — страницы плиток быстрых настроек Windows 11: точки и
// стрелки ▲▼ справа от сетки, листание колесом, нажатием и при перетаскивании
// плитки к краю, неактивные ползунки, подпись плитки в две строки.
//
// Постраничный режим включает флаг профиля theme.KeyQuickPaged; без него сетка
// прокручивается пиксель за пикселем, как до v3.34. Страница — rows рядов
// плиток: прокрутка всегда встаёт на границу страницы, строка не обрезается
// посередине.
package desktop

import (
	"image"
	"image/color"
	"strings"

	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// Размеры, которые не меняют вид панели, а подбирают фигуры под метрики:
// сторона стрелки страниц, ячейка точки и сама точка, ширина зоны столбца,
// порог колеса (в пикселях колеса) на одно листание, поля кнопки «Изменить».
const (
	qsPagerArrow  = 16
	qsPagerCell   = 12
	qsPagerDot    = 6
	qsPagerWidth  = 20
	qsPagerGap    = 2
	qsWheelPage   = 30.0
	qsEditPadL    = 8
	qsEditPadR    = 12
	qsEditGap     = 6
	qsLabelLines  = 2
	qsLabelMargin = 4
)

// layoutPager расставляет стрелки и точки страниц столбцом по центру правого
// поля панели, по высоте — на середине окна сетки. Страниц меньше двух —
// столбца нет.
func (l *qsLayout) layoutPager() {
	if l.pages < 2 || l.grid.Empty() {
		return
	}
	cx := l.panel.Max.X - l.m.pad/2
	dotsH := l.pages * qsPagerCell
	total := qsPagerArrow + qsPagerGap + dotsH + qsPagerGap + qsPagerArrow
	showDots := total <= l.grid.Dy()
	if !showDots {
		total = qsPagerArrow + qsPagerGap + qsPagerArrow
	}
	top := l.grid.Min.Y + (l.grid.Dy()-total)/2
	x0, x1 := cx-qsPagerWidth/2, cx-qsPagerWidth/2+qsPagerWidth
	y := top
	l.up = image.Rect(x0, y, x1, y+qsPagerArrow)
	y += qsPagerArrow + qsPagerGap
	if showDots {
		l.dots = make([]image.Rectangle, l.pages)
		for i := range l.dots {
			l.dots[i] = image.Rect(x0, y, x1, y+qsPagerCell)
			y += qsPagerCell
		}
		y += qsPagerGap
	}
	l.down = image.Rect(x0, y, x1, y+qsPagerArrow)
	l.pager = l.up.Union(l.down)
}

// pagerOrder перечисляет зоны столбца страниц для обхода Tab: ▲ (если есть куда
// листать назад), точки, ▼. Нет столбца — пусто.
func (l *qsLayout) pagerOrder() []qsZone {
	if l.pager.Empty() {
		return nil
	}
	var out []qsZone
	if l.page > 0 {
		out = append(out, qsZone{kind: qzPageUp})
	}
	for i := range l.dots {
		out = append(out, qsZone{kind: qzPageDot, idx: i})
	}
	if l.page < l.pages-1 {
		out = append(out, qsZone{kind: qzPageDown})
	}
	return out
}

// isPager — зона принадлежит столбцу страниц.
func (z qsZone) isPager() bool {
	return z.kind == qzPageUp || z.kind == qzPageDown || z.kind == qzPageDot
}

// pagerZoneAt находит зону столбца страниц под точкой.
func (l *qsLayout) pagerZoneAt(pt image.Point) qsZone {
	if l.pager.Empty() || !pt.In(l.pager) {
		return qsZone{}
	}
	switch {
	case pt.In(l.up):
		return qsZone{kind: qzPageUp}
	case pt.In(l.down):
		return qsZone{kind: qzPageDown}
	}
	for i, d := range l.dots {
		if pt.In(d) {
			return qsZone{kind: qzPageDot, idx: i}
		}
	}
	return qsZone{}
}

// ─── Публичные методы ────────────────────────────────────────────────────────

// paged сообщает, идут ли плитки страницами: профиль поднял флаг
// theme.KeyQuickPaged.
func (q *QuickSettings) paged() bool {
	tm := q.Theme()
	return tm != nil && tm.GetFlag(theme.KeyQuickPaged, false)
}

// editLeft сообщает, стоит ли «Изменить» слева (theme.KeyQuickFooterEditLeft).
func (q *QuickSettings) editLeft() bool {
	tm := q.Theme()
	return tm != nil && tm.GetFlag(theme.KeyQuickFooterEditLeft, false)
}

// Pages возвращает число страниц плиток: единицу, когда всё помещается или
// панель листается прокруткой.
func (q *QuickSettings) Pages() int {
	if !q.rich() || !q.paged() {
		return 1
	}
	l, _ := q.pn.layout(q.rect())
	if l.pages < 1 {
		return 1
	}
	return l.pages
}

// Page возвращает номер показанной страницы плиток (с нуля).
func (q *QuickSettings) Page() int {
	q.pn.mu.Lock()
	defer q.pn.mu.Unlock()
	return q.pn.pageIdx
}

// SetPage показывает страницу плиток p (с нуля); номер за пределами приводится
// к ближайшей странице. Для панели с прокруткой ничего не делает.
func (q *QuickSettings) SetPage(p int) {
	if !q.rich() || !q.paged() {
		return
	}
	q.pn.setPage(p)
}

// SetVolumeEnabled включает и выключает ползунок громкости. Неактивный рисуется
// серым, не нажимается, не двигается колесом и клавишами и выпадает из обхода
// Tab — для машины без звуковой карты. Включён по умолчанию.
func (q *QuickSettings) SetVolumeEnabled(on bool) { q.pn.setSliderEnabled(qzVol, on) }

// VolumeEnabled сообщает, активен ли ползунок громкости.
func (q *QuickSettings) VolumeEnabled() bool {
	q.pn.mu.Lock()
	defer q.pn.mu.Unlock()
	return !q.pn.volOff
}

// SetBrightnessEnabled включает и выключает ползунок яркости (см.
// SetVolumeEnabled). Ползунок есть, только если потребитель задал яркость
// (SetBrightness).
func (q *QuickSettings) SetBrightnessEnabled(on bool) { q.pn.setSliderEnabled(qzBright, on) }

// BrightnessEnabled сообщает, активен ли ползунок яркости.
func (q *QuickSettings) BrightnessEnabled() bool {
	q.pn.mu.Lock()
	defer q.pn.mu.Unlock()
	return !q.pn.brightOff
}

func (pn *qsPanel) setSliderEnabled(kind qsZoneKind, on bool) {
	pn.mu.Lock()
	cur := &pn.volOff
	if kind == qzBright {
		cur = &pn.brightOff
	}
	if *cur == !on {
		pn.mu.Unlock()
		return
	}
	*cur = !on
	if !on {
		// Тянули ползунок и выключили на лету: перетаскивание оборвано.
		if pn.drag.kind == qdSlider && pn.drag.zone.kind == kind {
			pn.drag = qsDrag{}
		}
		if f := pn.focus.kind; (kind == qzVol && (f == qzVol || f == qzVolIcon || f == qzVolChevron)) ||
			(kind == qzBright && f == qzBright) {
			pn.focus = qsZone{}
		}
		if h := pn.hover.kind; (kind == qzVol && (h == qzVol || h == qzVolIcon || h == qzVolChevron)) ||
			(kind == qzBright && (h == qzBright || h == qzBrightIcon)) {
			pn.hover = qsZone{}
		}
	}
	pn.mu.Unlock()
	row := qsZone{kind: kind}
	pn.invalidateZone(row)
	if kind == qzVol {
		pn.invalidateZone(qsZone{kind: qzVolIcon})
		pn.invalidateZone(qsZone{kind: qzVolChevron})
	} else {
		pn.invalidateZone(qsZone{kind: qzBrightIcon})
	}
}

// sliderOff сообщает, что ползунок зоны z сейчас неактивен.
func (pn *qsPanel) sliderOff(z qsZone) bool {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	switch z.kind {
	case qzVol, qzVolIcon, qzVolChevron:
		return pn.volOff
	case qzBright, qzBrightIcon:
		return pn.brightOff
	}
	return false
}

// ─── Листание ────────────────────────────────────────────────────────────────

// setPage показывает страницу p (с ограничением по числу страниц) и заявляет
// окно сетки и столбец страниц.
func (pn *qsPanel) setPage(p int) {
	l, _ := pn.layout(pn.q.rect())
	if !l.paged {
		return
	}
	if p >= l.pages {
		p = l.pages - 1
	}
	if p < 0 {
		p = 0
	}
	pn.mu.Lock()
	changed := pn.pageIdx != p
	pn.pageIdx = p
	pn.mu.Unlock()
	if changed {
		widget.InvalidateRect(l.grid.Union(l.pager).Inset(-2).Intersect(l.panel))
		// Плитки едут к новой странице; «меньше движения» и нулевая длительность
		// токена ставят её сразу.
		pn.pageAnim.To(float64(p))
	}
}

// keepPagerFocus держит клавиатурный фокус в столбце страниц: стрелка, на
// которой он стоял, дошла до крайней страницы и приглушилась (из обхода Tab она
// уходит) — фокус переходит на точку показанной страницы, а без точек на
// противоположную стрелку.
func (pn *qsPanel) keepPagerFocus(z qsZone) {
	pn.mu.Lock()
	on := pn.focus == z
	pn.mu.Unlock()
	if !on {
		return
	}
	l, _ := pn.layout(pn.q.rect())
	if !((z.kind == qzPageUp && l.page <= 0) || (z.kind == qzPageDown && l.page >= l.pages-1)) {
		return
	}
	switch {
	case l.page < len(l.dots):
		pn.focusZone(qsZone{kind: qzPageDot, idx: l.page})
	case z.kind == qzPageUp:
		pn.focusZone(qsZone{kind: qzPageDown})
	default:
		pn.focusZone(qsZone{kind: qzPageUp})
	}
}

// stepPage листает на delta страниц.
func (pn *qsPanel) stepPage(delta int) {
	l, _ := pn.layout(pn.q.rect())
	if !l.paged {
		return
	}
	pn.setPage(l.page + delta)
}

// wheelPage листает страницы колесом: пиксели колеса копятся до порога, чтобы
// сенсорная панель, шлющая много мелких событий, не пролистывала всё разом, а
// смена направления сбрасывает накопленное.
func (pn *qsPanel) wheelPage(dy float64) {
	if dy == 0 {
		return
	}
	pn.mu.Lock()
	if (dy > 0) != (pn.wheelAcc > 0) {
		pn.wheelAcc = 0
	}
	pn.wheelAcc += dy
	flip := 0
	switch {
	case pn.wheelAcc >= qsWheelPage:
		flip, pn.wheelAcc = 1, 0
	case pn.wheelAcc <= -qsWheelPage:
		flip, pn.wheelAcc = -1, 0
	}
	pn.mu.Unlock()
	if flip != 0 {
		pn.stepPage(flip)
	}
}

// dragPageEdge листает страницу, когда плитку тянут к верхнему или нижнему
// краю сетки; один заход в краевую зону — одна страница (защёлка снимается,
// когда курсор покинул зону).
func (pn *qsPanel) dragPageEdge(l *qsLayout, pt image.Point) {
	if !l.paged {
		return
	}
	dir := 0
	switch {
	case pt.Y < l.grid.Min.Y+qsEdgeZone && l.page > 0:
		dir = -1
	case pt.Y > l.grid.Max.Y-qsEdgeZone && l.page < l.pages-1:
		dir = 1
	}
	pn.mu.Lock()
	if pn.drag.kind != qdTile {
		pn.mu.Unlock()
		return
	}
	latched := pn.drag.latch
	pn.drag.latch = dir != 0
	pn.mu.Unlock()
	if dir != 0 && !latched {
		pn.setPage(l.page + dir)
	}
}

// ─── Рисование ───────────────────────────────────────────────────────────────

// drawPager рисует столбец страниц: ▲, точки (текущая — цвет текста), ▼. На
// крайней странице соответствующая стрелка приглушена и не нажимается.
func (pn *qsPanel) drawPager(ctx widget.DrawContext, l *qsLayout, u qsUI) {
	if l.pager.Empty() {
		return
	}
	arrow := func(z qsZone, r image.Rectangle, up, off bool) {
		hover := !off && u.hover == z
		st := StateOf(hover, hover && u.press == z, false, off, false)
		s := pn.style(z, r, "w11.pager.arrow", st)
		pn.paint(ctx, r, s)
		drawGlyphTriangle(ctx, r, up, s.Text, 8)
	}
	arrow(qsZone{kind: qzPageUp}, l.up, true, l.page <= 0)
	for i, d := range l.dots {
		st := theme.StateNormal
		if i == l.page {
			st = theme.StateActive
		}
		ds := pn.partStyle("w11.pager.dot", st)
		side := qsPagerDot
		dr := image.Rect(d.Min.X+(d.Dx()-side)/2, d.Min.Y+(d.Dy()-side)/2, d.Min.X+(d.Dx()-side)/2+side, d.Min.Y+(d.Dy()-side)/2+side)
		pn.paint(ctx, dr, ds)
	}
	arrow(qsZone{kind: qzPageDown}, l.down, false, l.page >= l.pages-1)
}

// drawGlyphTriangle рисует закрашенный треугольник ▲ (up) или ▼ шириной w в
// центре r.
func drawGlyphTriangle(ctx widget.DrawContext, r image.Rectangle, up bool, col color.RGBA, w int) {
	if w < 3 || col.A == 0 || r.Empty() {
		return
	}
	if w%2 == 0 {
		w++
	}
	h := (w + 1) / 2
	cx, top := r.Min.X+r.Dx()/2, r.Min.Y+(r.Dy()-h)/2
	for i := 0; i < h; i++ {
		rowW := 1 + 2*i
		if !up {
			rowW = w - 2*i
		}
		ctx.FillRectAlpha(cx-rowW/2, top+i, rowW, 1, col)
	}
}

// qsWrapLabel разбивает подпись плитки по словам на строки не шире maxW, не
// больше maxLines; то, что не поместилось, уходит в многоточие на последней
// строке. Слово шире строки укорачивается многоточием само.
func qsWrapLabel(ctx widget.DrawContext, text string, s *theme.Style, maxW, maxLines int) []string {
	words := strings.Fields(text)
	if len(words) == 0 || maxW <= 0 || maxLines <= 0 {
		return nil
	}
	var all []string
	cur := ""
	for _, w := range words {
		cand := w
		if cur != "" {
			cand = cur + " " + w
		}
		if cur == "" || MeasureText(ctx, cand, s) <= maxW {
			cur = cand
			continue
		}
		all = append(all, cur)
		cur = w
	}
	all = append(all, cur)
	if len(all) > maxLines {
		last := strings.Join(all[maxLines-1:], " ")
		all = append(all[:maxLines-1], last)
	}
	for i, line := range all {
		all[i] = Elide(ctx, line, s, maxW)
	}
	return all
}
