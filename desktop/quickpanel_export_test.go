package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// Окно в раскладку быстрых настроек для внешних тестов (package desktop_test):
// где что лежит на экране, чтобы нажимать в нужные точки, не вшивая числа.

// QSPanelRect — прямоугольник панели в её текущем положении.
func QSPanelRect(q *QuickSettings) image.Rectangle { return q.rect() }

// QSTileRect — прямоугольник плитки id (пустой, если такой нет).
func QSTileRect(q *QuickSettings, id QuickActionID) image.Rectangle {
	l, list := q.pn.layout(q.rect())
	for i, a := range list {
		if a.ID == id {
			body, _ := l.tileRects(i)
			return body
		}
	}
	return image.Rectangle{}
}

// QSChevronRect — зона «›» плитки id.
func QSChevronRect(q *QuickSettings, id QuickActionID) image.Rectangle {
	l, list := q.pn.layout(q.rect())
	for i, a := range list {
		if a.ID == id {
			body, _ := l.tileRects(i)
			return l.chevron(body)
		}
	}
	return image.Rectangle{}
}

// QSLabelRect — подпись под плиткой id.
func QSLabelRect(q *QuickSettings, id QuickActionID) image.Rectangle {
	l, list := q.pn.layout(q.rect())
	for i, a := range list {
		if a.ID == id {
			_, label := l.tileRects(i)
			return label
		}
	}
	return image.Rectangle{}
}

// QSZoneRect — прямоугольник зоны по имени: grid, vol, volicon, volchev,
// voltrack, bright, brighticon, brighttrack, edit, settings, back, thumb,
// footer, battery, body.
func QSZoneRect(q *QuickSettings, name string) image.Rectangle {
	l, _ := q.pn.layout(q.rect())
	switch name {
	case "grid":
		return l.grid
	case "vol":
		return l.vol.row
	case "volicon":
		return l.vol.icon
	case "volchev":
		return l.vol.chev
	case "voltrack":
		return l.vol.track
	case "bright":
		return l.bright.row
	case "brighticon":
		return l.bright.icon
	case "brighttrack":
		return l.bright.track
	case "edit":
		return l.edit
	case "settings":
		return l.settings
	case "back":
		return l.back
	case "thumb":
		return l.thumb
	case "footer":
		return l.footer
	case "battery":
		return l.battery
	case "body":
		return l.body
	case "pager":
		return l.pager
	case "pageup":
		return l.up
	case "pagedown":
		return l.down
	}
	return image.Rectangle{}
}

// QSDotRect — точка страницы i столбца страниц.
func QSDotRect(q *QuickSettings, i int) image.Rectangle {
	l, _ := q.pn.layout(q.rect())
	if i < 0 || i >= len(l.dots) {
		return image.Rectangle{}
	}
	return l.dots[i]
}

// QSWrapLabel разбивает подпись плитки на строки шириной до maxW (мерой служит
// ctx), как это делает панель.
func QSWrapLabel(ctx widget.DrawContext, text string, maxW, maxLines int) []string {
	return qsWrapLabel(ctx, text, &theme.Style{}, maxW, maxLines)
}

// QSRich сообщает, рисует ли панель вариант Windows 11.
func QSRich(q *QuickSettings) bool { return q.rich() }

// QSScroll возвращает смещение сетки плиток.
func QSScroll(q *QuickSettings) int {
	q.pn.mu.Lock()
	defer q.pn.mu.Unlock()
	return q.pn.scroll
}

// QSFocus возвращает вид и номер зоны с клавиатурным фокусом.
func QSFocus(q *QuickSettings) (kind int, idx int) {
	q.pn.mu.Lock()
	defer q.pn.mu.Unlock()
	return int(q.pn.focus.kind), q.pn.focus.idx
}

// Виды зон для проверки обхода клавиатурой.
const (
	QSKindTile    = int(qzTile)
	QSKindChevron = int(qzChevron)
	QSKindBright  = int(qzBright)
	QSKindVolIcon = int(qzVolIcon)
	QSKindVol     = int(qzVol)
	QSKindVolChev = int(qzVolChevron)
	QSKindEdit    = int(qzEdit)
	QSKindGear    = int(qzSettings)
	QSKindBack    = int(qzBack)
	QSKindPageUp  = int(qzPageUp)
	QSKindPageDn  = int(qzPageDown)
	QSKindPageDot = int(qzPageDot)
)

// QSSlide возвращает положение сетки плиток в страницах (дробное, пока она едет
// к новой странице) и признак «движение идёт».
func QSSlide(q *QuickSettings) (float64, bool) {
	return q.pn.pageAnim.Value(), q.pn.pageAnim.Animating()
}

// QSPage возвращает положение перехода между страницами: 0 — главная, 1 —
// вложенная.
func QSPage(q *QuickSettings) float64 { return q.pn.page.Value() }

// QSHeight возвращает высоту панели при заданных числе плиток и ползунках.
func QSHeight(q *QuickSettings, n int, hasVol, hasBright bool) int {
	return qsHeight(readQSMetrics(q.Theme()), n, hasVol, hasBright)
}

// QSMetrics возвращает размеры панели.
func QSMetrics(q *QuickSettings) (width, tileW, tileH, colGap, pad int) {
	m := readQSMetrics(q.Theme())
	return m.width, m.tileW, m.tileH, m.colGap, m.pad
}

// DPRect — прямоугольник строки sub элемента item страницы-строителя.
func DPRect(p *DetailPage, item, sub int) image.Rectangle {
	m := readQSMetrics(p.tm)
	p.mu.Lock()
	defer p.mu.Unlock()
	pl, _ := p.places(m)
	if item < 0 || item >= len(pl) {
		return image.Rectangle{}
	}
	return pl[item].e.rowRect(p, m, pl[item].r, sub)
}

// DPScroll — прокрутка страницы-строителя.
func DPScroll(p *DetailPage) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.scroll
}

// CalFocusAction — прямоугольник кнопки «Фокусировка» календаря Windows 11.
func CalFocusAction(c *CalendarFlyout) image.Rectangle { return c.w11Layout(c.rect()).faction }

// CalFocusMenuBounds — область открытого меню выбора длительности.
func CalFocusMenuBounds(c *CalendarFlyout) image.Rectangle {
	c.mu.Lock()
	p := c.focusMenu
	c.mu.Unlock()
	if p == nil {
		return image.Rectangle{}
	}
	return p.OverlayBounds()
}
