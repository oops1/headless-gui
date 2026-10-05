package desktop

import "image"

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
	}
	return image.Rectangle{}
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
)

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
