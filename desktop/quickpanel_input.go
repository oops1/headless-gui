// quickpanel_input.go — ввод в быстрых настройках Windows 11: мышь (плитки,
// «›», ползунки, бегунок прокрутки, перетаскивание плиток в режиме правки),
// колесо, клавиатура (Tab, стрелки, Enter, Esc) и подсказки.
//
// Все обработчики сначала спрашивают раскладку (quickpanel_layout.go) у
// прямоугольника панели в её текущем положении, так что нажимается то, что
// видно, даже пока панель выезжает.
package desktop

import (
	"fmt"
	"image"

	"github.com/oops1/headless-gui/v3/internal/focusreq"
	"github.com/oops1/headless-gui/v3/widget"
)

// qsSliderStep — шаг ползунка с клавиатуры; qsSliderBigStep — PageUp/PageDown;
// qsWheelStep — шаг колеса над ползунком (на каждое событие колеса).
const (
	qsSliderStep    = 0.05
	qsSliderBigStep = 0.10
	qsWheelStep     = 0.02
)

// qsAutoScroll — на сколько точек сетка прокручивается за одно движение мыши,
// пока плитку тянут у её края; qsEdgeZone — ширина этой краевой зоны.
const (
	qsAutoScroll = 10
	qsEdgeZone   = 14
)

// onDetailsPage сообщает, на какой странице стоит панель: true — на вложенной
// (переход дошёл до неё больше чем наполовину).
func (pn *qsPanel) onDetailsPage() bool {
	pn.mu.Lock()
	has := pn.details != nil && !pn.detailsEcho
	pn.mu.Unlock()
	return has && pn.page.Value() >= 0.5
}

// zoneAt находит зону главной страницы под точкой.
func (pn *qsPanel) zoneAt(pt image.Point) (qsZone, *qsLayout, []QuickAction) {
	l, list := pn.layout(pn.q.rect())
	pn.mu.Lock()
	chevrons := !pn.editing
	pn.mu.Unlock()
	return l.zoneAt(pt, list, chevrons), l, list
}

// ─── Мышь ────────────────────────────────────────────────────────────────────

// richMouseButton — кнопка мыши. Возвращает true, если событие принято
// панелью.
func (q *QuickSettings) richMouseButton(e widget.MouseEvent) bool {
	pn := q.pn
	pt := image.Pt(e.X, e.Y)
	panel := q.rect()
	if !e.Pressed {
		return pn.release(e, pt, panel)
	}
	if !pt.In(panel) {
		return q.Flyout.OnMouseButton(e)
	}
	focusreq.Request(q)
	if q.fs.NotePointer(e) {
		q.Invalidate()
	}
	if e.Button != widget.MouseLeft {
		return true
	}
	if pn.page.Value() > 0.01 && pn.page.Value() < 0.99 {
		return true // идёт переход: нажимать нечего
	}
	if pn.onDetailsPage() {
		return pn.pressDetails(e, pt)
	}
	z, l, list := pn.zoneAt(pt)
	pn.mu.Lock()
	pn.press = z
	pn.mu.Unlock()
	switch z.kind {
	case qzTile:
		pn.mu.Lock()
		editing := pn.editing
		if editing && z.idx < len(list) {
			body, _ := l.tileRects(z.idx)
			pn.drag = qsDrag{kind: qdTile, idx: z.idx, start: pt, cur: pt, grab: pt.Sub(body.Min)}
		}
		pn.mu.Unlock()
		pn.invalidateZone(z)
	case qzBright, qzVol:
		pn.beginSliderDrag(z, l, pt)
	case qzThumb:
		pn.mu.Lock()
		pn.drag = qsDrag{kind: qdThumb, start: pt, cur: pt, grab: image.Pt(0, pt.Y-l.thumb.Min.Y)}
		pn.mu.Unlock()
		pn.grabMouse()
		pn.invalidateZone(z)
	case qzNone:
	default:
		pn.invalidateZone(z)
	}
	return true
}

// release разбирает отпускание кнопки: завершает перетаскивание или
// засчитывает щелчок, если отпустили там же, где нажали.
func (pn *qsPanel) release(e widget.MouseEvent, pt image.Point, panel image.Rectangle) bool {
	q := pn.q
	if e.Button != widget.MouseLeft {
		return pt.In(panel)
	}
	pn.mu.Lock()
	drag, press := pn.drag, pn.press
	pn.drag, pn.press = qsDrag{}, qsZone{}
	pn.mu.Unlock()

	switch drag.kind {
	case qdTile:
		if drag.moved {
			pn.dropTile(drag)
			return true
		}
	case qdSlider, qdThumb:
		if drag.kind == qdThumb {
			pn.invalidateZone(qsZone{kind: qzThumb})
		} else {
			pn.invalidateZone(drag.zone)
		}
		return true
	}
	if press.kind == qzNone {
		return pt.In(panel)
	}
	if pn.onDetailsPage() {
		if press.kind == qzBack && pt.In(pn.backRect()) {
			q.CloseDetails()
		} else if press.kind == qzContent {
			pn.deliverButton(e)
		}
		return true
	}
	z, _, _ := pn.zoneAt(pt)
	pn.invalidateZone(press)
	if z == press {
		pn.activate(z)
	}
	return true
}

// backRect — стрелка «назад» вложенной страницы.
func (pn *qsPanel) backRect() image.Rectangle {
	l, _ := pn.layout(pn.q.rect())
	return l.back
}

// activate выполняет действие зоны: щелчок мышью или Enter/Space.
func (pn *qsPanel) activate(z qsZone) {
	q := pn.q
	pn.mu.Lock()
	var a QuickAction
	if z.idx >= 0 && z.idx < len(pn.list) {
		a = pn.list[z.idx]
	}
	m := pn.model
	editing := pn.editing
	pn.mu.Unlock()

	switch z.kind {
	case qzTile:
		if !editing && m != nil && !a.Disabled && !a.Unavailable {
			m.Toggle(a.ID)
		}
	case qzChevron:
		if !a.Disabled {
			q.OpenDetails(a.ID)
		}
	case qzVolIcon:
		if q.OnToggleMute != nil {
			q.OnToggleMute()
		}
	case qzVolChevron:
		q.OpenDetails(QuickVolumeID)
	case qzEdit:
		q.SetEditing(!editing)
	case qzSettings:
		// Параметры открываются поверх рабочего стола, а не под панелью:
		// сначала панель закрывается.
		q.Close()
		if q.OnSettings != nil {
			q.OnSettings()
		}
	case qzBack:
		q.CloseDetails()
	}
}

// grabMouse берёт мышь на время перетаскивания: курсор волен выходить за
// панель, события идут сюда, пока кнопка не отпущена.
func (pn *qsPanel) grabMouse() {
	pn.mu.Lock()
	cm := pn.capture
	pn.mu.Unlock()
	if cm != nil {
		cm.SetCapture(pn.q)
	}
}

// SetCaptureManager реализует widget.CaptureAware: менеджер захвата нужен,
// чтобы тянуть ползунок и плитку за пределы панели.
func (q *QuickSettings) SetCaptureManager(cm widget.CaptureManager) {
	q.pn.mu.Lock()
	q.pn.capture = cm
	q.pn.mu.Unlock()
}

// beginSliderDrag начинает перетаскивание ползунка и сразу переносит бегунок
// под курсор — клик по дорожке в стороне от бегунка выставляет значение.
func (pn *qsPanel) beginSliderDrag(z qsZone, l *qsLayout, pt image.Point) {
	pn.mu.Lock()
	pn.drag = qsDrag{kind: qdSlider, zone: z, start: pt, cur: pt}
	pn.mu.Unlock()
	pn.grabMouse()
	pn.dragSlider(z, l, pt.X)
}

// dragSlider ставит значение ползунка по координате x и сообщает потребителю.
func (pn *qsPanel) dragSlider(z qsZone, l *qsLayout, x int) {
	row := l.vol
	if z.kind == qzBright {
		row = l.bright
	}
	pn.setLevel(z, row.level(x, l.m.sliderThumb))
}

// setLevel ставит уровень ползунка z и зовёт колбэк потребителя. Уровень
// показывается сразу, не дожидаясь ответа потребителя: источник правды снаружи
// (SystemStatus, SetBrightness), но ползунок не должен отставать от руки.
func (pn *qsPanel) setLevel(z qsZone, level float64) {
	q := pn.q
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	pn.mu.Lock()
	var cur float64
	switch z.kind {
	case qzVol:
		cur = pn.vol.Level
		if pn.hasVolOver {
			cur = pn.volOver
		}
	case qzBright:
		cur = pn.bright
	default:
		pn.mu.Unlock()
		return
	}
	changed := level != cur
	if changed {
		if z.kind == qzVol {
			pn.volOver, pn.hasVolOver = level, true
		} else {
			pn.bright = level
		}
	}
	pn.mu.Unlock()
	if !changed {
		return
	}
	pn.invalidateZone(z)
	if z.kind == qzVol {
		if q.OnVolumeChange != nil {
			q.OnVolumeChange(level)
		}
	} else if q.OnBrightnessChange != nil {
		q.OnBrightnessChange(level)
	}
}

// richMouseMove — движение мыши: наведение, перетаскивание.
func (q *QuickSettings) richMouseMove(x, y int) {
	pn := q.pn
	if widget.CursorIsNowhere(x, y) {
		pn.setHover(qsZone{})
		pn.moveContent(x, y)
		return
	}
	pt := image.Pt(x, y)
	pn.mu.Lock()
	drag := pn.drag
	pn.mu.Unlock()
	switch drag.kind {
	case qdSlider:
		l, _ := pn.layout(q.rect())
		pn.dragSlider(drag.zone, l, x)
		return
	case qdThumb:
		pn.dragThumb(drag, y)
		return
	case qdTile:
		pn.dragTile(drag, pt)
		return
	}
	if pn.onDetailsPage() {
		l, _ := pn.layout(q.rect())
		z := qsZone{}
		if pt.In(l.back) {
			z = qsZone{kind: qzBack}
		}
		pn.setHover(z)
		pn.moveContent(x, y)
		return
	}
	z, _, _ := pn.zoneAt(pt)
	pn.setHover(z)
}

// setHover меняет зону под курсором и перерисовывает старую и новую.
func (pn *qsPanel) setHover(z qsZone) {
	pn.mu.Lock()
	old := pn.hover
	pn.hover = z
	pn.mu.Unlock()
	if old == z {
		return
	}
	if old.kind != qzNone {
		pn.invalidateZone(old)
	}
	if z.kind != qzNone {
		pn.invalidateZone(z)
	}
}

// dragThumb ведёт бегунок прокрутки сетки за курсором.
func (pn *qsPanel) dragThumb(d qsDrag, y int) {
	l, _ := pn.layout(pn.q.rect())
	span := l.grid.Dy() - l.thumb.Dy()
	if span <= 0 || l.maxScrl <= 0 {
		return
	}
	ratio := float64(y-d.grab.Y-l.grid.Min.Y) / float64(span)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	pn.scrollTo(int(ratio*float64(l.maxScrl) + 0.5))
}

// scrollTo прокручивает сетку к смещению v (с ограничением) и перерисовывает
// её окно.
func (pn *qsPanel) scrollTo(v int) {
	l, _ := pn.layout(pn.q.rect())
	if v > l.maxScrl {
		v = l.maxScrl
	}
	if v < 0 {
		v = 0
	}
	pn.mu.Lock()
	changed := pn.scroll != v
	pn.scroll = v
	pn.mu.Unlock()
	if changed {
		widget.InvalidateRect(l.grid.Union(l.scrollThumb()))
		l2, _ := pn.layout(pn.q.rect())
		widget.InvalidateRect(l2.thumb)
	}
}

// dragTile ведёт плитку за курсором: после порога плитка «взята», под ней
// остальные расступаются, у края сетки она прокручивается.
func (pn *qsPanel) dragTile(d qsDrag, pt image.Point) {
	l, _ := pn.layout(pn.q.rect())
	moved := d.moved || qsAbs(pt.X-d.start.X) >= qsDragThreshold || qsAbs(pt.Y-d.start.Y) >= qsDragThreshold
	if !moved {
		return
	}
	oldFloat := pn.floatRect(l, d)
	if !d.moved {
		pn.grabMouse()
	}
	if pt.Y < l.grid.Min.Y+qsEdgeZone && l.scroll > 0 {
		pn.scrollTo(l.scroll - qsAutoScroll)
	} else if pt.Y > l.grid.Max.Y-qsEdgeZone && l.scroll < l.maxScrl {
		pn.scrollTo(l.scroll + qsAutoScroll)
	}
	l, _ = pn.layout(pn.q.rect())
	centre := pt.Sub(d.grab).Add(image.Pt(l.m.tileW/2, l.m.tileH/2))
	slot := l.slotAt(centre)

	pn.mu.Lock()
	if pn.drag.kind != qdTile {
		pn.mu.Unlock()
		return
	}
	pn.drag.moved, pn.drag.cur, pn.drag.slot = true, pt, slot
	nd := pn.drag
	pn.mu.Unlock()

	widget.InvalidateRect(l.grid.Union(oldFloat).Union(pn.floatRect(l, nd)).Intersect(l.panel))
}

// floatRect — где рисуется плитка в руке.
func (pn *qsPanel) floatRect(l *qsLayout, d qsDrag) image.Rectangle {
	x, y := d.cur.X-d.grab.X, d.cur.Y-d.grab.Y
	return image.Rect(x, y, x+l.m.tileW, y+l.m.tileH).Inset(-2)
}

func qsAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// dropTile кладёт плитку на выбранное место и сообщает новый порядок.
func (pn *qsPanel) dropTile(d qsDrag) {
	pn.mu.Lock()
	list := pn.list
	pn.mu.Unlock()
	if d.idx >= len(list) {
		return
	}
	order := qsDisplayOrder(len(list), d.idx, d.slot)
	ids := make([]QuickActionID, len(order))
	same := true
	for p, i := range order {
		ids[p] = list[i].ID
		if i != p {
			same = false
		}
	}
	pn.q.Invalidate()
	if same {
		return
	}
	pn.commitOrder(ids)
	pn.q.Invalidate()
}

// ─── Колесо ──────────────────────────────────────────────────────────────────

// richWheel — колесо: над сеткой прокручивает её, над ползунком меняет
// значение, на вложенной странице отдаётся содержимому.
func (q *QuickSettings) richWheel(x, y int, dx, dy float64) bool {
	pn := q.pn
	pt := image.Pt(x, y)
	if !pt.In(q.rect()) {
		return false
	}
	if pn.onDetailsPage() {
		return pn.wheelContent(x, y, dx, dy)
	}
	z, l, _ := pn.zoneAt(pt)
	switch z.kind {
	case qzBright, qzBrightIcon, qzVol, qzVolIcon, qzVolChevron:
		kind := qzVol
		if z.kind == qzBright || z.kind == qzBrightIcon {
			kind = qzBright
		}
		step := qsWheelStep
		if dy > 0 {
			step = -step
		}
		pn.mu.Lock()
		cur := pn.bright
		if kind == qzVol {
			cur = pn.vol.Level
			if pn.hasVolOver {
				cur = pn.volOver
			}
		}
		pn.mu.Unlock()
		pn.setLevel(qsZone{kind: kind}, cur+step)
		return true
	}
	if l.maxScrl > 0 && pt.In(l.grid) {
		step := int(dy)
		if step == 0 && dy != 0 {
			step = 1
			if dy < 0 {
				step = -1
			}
		}
		pn.scrollTo(l.scroll + step)
	}
	return true
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

// richKey — клавиша. Возвращает true, если панель её разобрала.
func (q *QuickSettings) richKey(e widget.KeyEvent) bool {
	pn := q.pn
	if !e.Pressed {
		if pn.onDetailsPage() {
			pn.mu.Lock()
			content := pn.focus.kind == qzContent
			pn.mu.Unlock()
			if content {
				return pn.keyContent(e)
			}
		}
		return false
	}
	if e.Code == widget.KeyEscape {
		return q.escapeStep()
	}
	if pn.onDetailsPage() {
		return pn.keyDetails(e)
	}
	return pn.keyMain(e)
}

// escapeStep — Esc: сначала выходит из режима правки, затем со вложенной
// страницы назад, и только потом закрывает панель.
func (q *QuickSettings) escapeStep() bool {
	pn := q.pn
	if pn.onDetailsPage() {
		q.CloseDetails()
		return true
	}
	if q.Editing() {
		q.SetEditing(false)
		return true
	}
	return q.Flyout.DismissOnEscape()
}

// DismissOnEscape — Esc закрывает панель, но в варианте Windows 11 сначала
// отступает на шаг: из режима правки, со вложенной страницы.
func (q *QuickSettings) DismissOnEscape() bool {
	if q.IsOpen() && q.rich() {
		return q.escapeStep()
	}
	return q.Flyout.DismissOnEscape()
}

// OnKeyEvent — клавиатура панели: вариант Windows 11 обходит свои зоны, прежний
// вид, как и Flyout, реагирует только на Esc.
func (q *QuickSettings) OnKeyEvent(e widget.KeyEvent) {
	if !q.IsOpen() || !q.rich() {
		q.Flyout.OnKeyEvent(e)
		return
	}
	if q.richKey(e) && e.Pressed && q.fs.noteKey() {
		q.Invalidate()
	}
}

// focusZone переносит фокус на зону z, показывает её (прокручивая сетку) и
// перерисовывает старую и новую рамки.
func (pn *qsPanel) focusZone(z qsZone) {
	pn.mu.Lock()
	old := pn.focus
	pn.focus = z
	pn.mu.Unlock()
	if z.kind == qzTile || z.kind == qzChevron {
		pn.ensureVisible(z.idx)
	}
	if old != z {
		pn.invalidateZone(old)
		pn.invalidateZone(z)
	}
}

// ensureVisible прокручивает сетку так, чтобы плитка idx была видна целиком
// вместе с подписью.
func (pn *qsPanel) ensureVisible(idx int) {
	l, _ := pn.layout(pn.q.rect())
	if l.maxScrl <= 0 {
		return
	}
	body, label := l.tileRects(idx)
	switch {
	case body.Min.Y < l.grid.Min.Y:
		pn.scrollTo(l.scroll - (l.grid.Min.Y - body.Min.Y))
	case label.Max.Y > l.grid.Max.Y:
		pn.scrollTo(l.scroll + (label.Max.Y - l.grid.Max.Y))
	}
}

// keyMain разбирает клавишу на главной странице.
func (pn *qsPanel) keyMain(e widget.KeyEvent) bool {
	q := pn.q
	l, list := pn.layout(q.rect())
	pn.mu.Lock()
	focus, editing := pn.focus, pn.editing
	pn.mu.Unlock()
	order := l.focusOrder(list, editing)
	at := -1
	for i, z := range order {
		if z == focus {
			at = i
			break
		}
	}
	step := func(d int) {
		if len(order) == 0 {
			return
		}
		switch {
		case at < 0 && d > 0:
			at = 0
		case at < 0:
			at = len(order) - 1
		default:
			at = (at + d + len(order)) % len(order)
		}
		pn.focusZone(order[at])
	}

	switch e.Code {
	case widget.KeyTab:
		if e.Mod&widget.ModShift != 0 {
			step(-1)
		} else {
			step(1)
		}
		return true
	case widget.KeyEnter, widget.KeySpace:
		if focus.kind != qzNone {
			pn.activate(focus)
		}
		return true
	case widget.KeyLeft, widget.KeyRight, widget.KeyUp, widget.KeyDown,
		widget.KeyHome, widget.KeyEnd, widget.KeyPageUp, widget.KeyPageDown:
	default:
		return false
	}

	switch focus.kind {
	case qzNone:
		step(1)
	case qzTile, qzChevron:
		pn.arrowTile(l, list, focus, e, editing, order, at)
	case qzVol, qzBright:
		pn.arrowSlider(focus, e)
	default:
		switch e.Code {
		case widget.KeyLeft, widget.KeyUp:
			if at > 0 {
				pn.focusZone(order[at-1])
			}
		case widget.KeyRight, widget.KeyDown:
			if at >= 0 && at+1 < len(order) {
				pn.focusZone(order[at+1])
			}
		}
	}
	return true
}

// arrowTile — стрелки на плитке: переход по сетке, а с Alt или Ctrl в режиме
// правки — перестановка плитки.
func (pn *qsPanel) arrowTile(l *qsLayout, list []QuickAction, focus qsZone, e widget.KeyEvent,
	editing bool, order []qsZone, at int) {

	p := l.place(focus.idx)
	np := p
	switch e.Code {
	case widget.KeyLeft:
		if p%l.m.cols > 0 {
			np = p - 1
		}
	case widget.KeyRight:
		if p%l.m.cols < l.m.cols-1 && p+1 < l.n {
			np = p + 1
		}
	case widget.KeyUp:
		if p-l.m.cols >= 0 {
			np = p - l.m.cols
		}
	case widget.KeyDown:
		if p+l.m.cols < l.n {
			np = p + l.m.cols
		} else if p/l.m.cols < (l.n-1)/l.m.cols {
			np = l.n - 1 // нижний неполный ряд: к последней плитке
		} else if at >= 0 {
			// Под сеткой — следующая зона обхода.
			for i := at + 1; i < len(order); i++ {
				if order[i].kind != qzTile && order[i].kind != qzChevron {
					pn.focusZone(order[i])
					return
				}
			}
			return
		}
	case widget.KeyHome:
		np = 0
	case widget.KeyEnd:
		np = l.n - 1
	case widget.KeyPageUp:
		np = p - l.m.cols*l.m.rows
	case widget.KeyPageDown:
		np = p + l.m.cols*l.m.rows
	}
	if np < 0 {
		np = 0
	}
	if np >= l.n {
		np = l.n - 1
	}
	if np == p {
		return
	}
	if editing && e.Mod&(widget.ModAlt|widget.ModCtrl) != 0 {
		pn.moveTile(list, focus.idx, np)
		return
	}
	pn.focusZone(qsZone{kind: qzTile, idx: l.at(np)})
}

// moveTile переставляет плитку idx на место to (с клавиатуры) и сообщает
// новый порядок.
func (pn *qsPanel) moveTile(list []QuickAction, idx, to int) {
	order := qsDisplayOrder(len(list), idx, to)
	ids := make([]QuickActionID, len(order))
	for p, i := range order {
		ids[p] = list[i].ID
	}
	pn.commitOrder(ids)
	pn.mu.Lock()
	pn.focus = qsZone{kind: qzTile, idx: to}
	pn.mu.Unlock()
	pn.ensureVisible(to)
	pn.q.Invalidate()
}

// arrowSlider — стрелки на ползунке: шаг, Home и End — края.
func (pn *qsPanel) arrowSlider(z qsZone, e widget.KeyEvent) {
	pn.mu.Lock()
	cur := pn.bright
	if z.kind == qzVol {
		cur = pn.vol.Level
		if pn.hasVolOver {
			cur = pn.volOver
		}
	}
	pn.mu.Unlock()
	switch e.Code {
	case widget.KeyLeft, widget.KeyDown:
		cur -= qsSliderStep
	case widget.KeyRight, widget.KeyUp:
		cur += qsSliderStep
	case widget.KeyPageDown:
		cur -= qsSliderBigStep
	case widget.KeyPageUp:
		cur += qsSliderBigStep
	case widget.KeyHome:
		cur = 0
	case widget.KeyEnd:
		cur = 1
	}
	pn.setLevel(z, cur)
}

// keyDetails разбирает клавишу на вложенной странице: Tab ходит между
// стрелкой «назад» и содержимым, остальное — стрелке или содержимому.
func (pn *qsPanel) keyDetails(e widget.KeyEvent) bool {
	q := pn.q
	pn.mu.Lock()
	focus := pn.focus
	d := pn.details
	pn.mu.Unlock()
	hasContent := d != nil && d.Content != nil

	if e.Code == widget.KeyTab {
		next := qsZone{kind: qzBack}
		if hasContent && focus.kind != qzContent {
			next = qsZone{kind: qzContent}
		}
		pn.focusZone(next)
		return true
	}
	if e.Code == widget.KeyLeft && e.Mod&widget.ModAlt != 0 {
		q.CloseDetails()
		return true
	}
	if focus.kind == qzContent {
		return pn.keyContent(e)
	}
	switch e.Code {
	case widget.KeyEnter, widget.KeySpace, widget.KeyBackspace, widget.KeyLeft:
		q.CloseDetails()
		return true
	}
	return false
}

// ─── Содержимое вложенной страницы ───────────────────────────────────────────

// contentRoot возвращает корневой виджет вложенной страницы. Когда страница
// встала на место, ему сразу выдаются границы: событие может прийти раньше
// первой отрисовки, которая обычно их задаёт.
func (pn *qsPanel) contentRoot() widget.Widget {
	pn.mu.Lock()
	var c widget.Widget
	if pn.details != nil {
		c = pn.details.Content
	}
	pn.mu.Unlock()
	if c != nil && pn.page.Value() >= 0.99 {
		if l, _ := pn.layout(pn.q.rect()); c.Bounds() != l.body {
			c.SetBounds(l.body)
		}
	}
	return c
}

// qsPath возвращает цепочку виджетов от корня до самого вложенного, чьи
// границы содержат точку.
func qsPath(w widget.Widget, pt image.Point) []widget.Widget {
	var path []widget.Widget
	for depth := 0; w != nil && depth < 64; depth++ {
		if !widget.IsWidgetVisible(w) || !pt.In(w.Bounds()) {
			break
		}
		path = append(path, w)
		var next widget.Widget
		ch := w.Children()
		for i := len(ch) - 1; i >= 0; i-- {
			if widget.IsWidgetVisible(ch[i]) && pt.In(ch[i].Bounds()) {
				next = ch[i]
				break
			}
		}
		w = next
	}
	return path
}

// pressDetails — нажатие на вложенной странице: стрелка «назад» или
// содержимое потребителя.
func (pn *qsPanel) pressDetails(e widget.MouseEvent, pt image.Point) bool {
	l, _ := pn.layout(pn.q.rect())
	if pt.In(l.back) {
		pn.mu.Lock()
		pn.press = qsZone{kind: qzBack}
		pn.focus = qsZone{kind: qzBack}
		pn.mu.Unlock()
		pn.q.Invalidate()
		return true
	}
	root := pn.contentRoot()
	if root != nil && pt.In(root.Bounds()) {
		pn.mu.Lock()
		pn.press = qsZone{kind: qzContent}
		pn.focus = qsZone{kind: qzContent}
		pn.mu.Unlock()
		pn.deliverButton(e)
	}
	return true
}

// deliverButton отдаёт кнопку мыши самому вложенному виджету под курсором,
// а если он её не взял — его предкам.
func (pn *qsPanel) deliverButton(e widget.MouseEvent) {
	root := pn.contentRoot()
	if root == nil {
		return
	}
	path := qsPath(root, image.Pt(e.X, e.Y))
	for i := len(path) - 1; i >= 0; i-- {
		if h, ok := path[i].(widget.MouseClickHandler); ok && h.OnMouseButton(e) {
			break
		}
	}
	pn.q.Invalidate()
}

// moveContent отдаёт движение мыши содержимому: самому вложенному виджету под
// курсором, который его принимает, а прежнему — «курсор ушёл».
func (pn *qsPanel) moveContent(x, y int) {
	root := pn.contentRoot()
	if root == nil {
		return
	}
	var target widget.Widget
	if !widget.CursorIsNowhere(x, y) {
		path := qsPath(root, image.Pt(x, y))
		for i := len(path) - 1; i >= 0; i-- {
			if _, ok := path[i].(widget.MouseMoveHandler); ok {
				target = path[i]
				break
			}
		}
	}
	pn.mu.Lock()
	prev := pn.contentHover
	pn.contentHover = target
	pn.mu.Unlock()
	if prev != nil && prev != target {
		prev.(widget.MouseMoveHandler).OnMouseMove(widget.CursorNowhere, widget.CursorNowhere)
	}
	if target != nil {
		target.(widget.MouseMoveHandler).OnMouseMove(x, y)
	}
}

// wheelContent отдаёт колесо самому вложенному виджету, который его берёт.
func (pn *qsPanel) wheelContent(x, y int, dx, dy float64) bool {
	root := pn.contentRoot()
	if root == nil {
		return true
	}
	path := qsPath(root, image.Pt(x, y))
	for i := len(path) - 1; i >= 0; i-- {
		if h, ok := path[i].(interface {
			OnMouseWheelPixels(x, y int, dx, dy float64) bool
		}); ok && h.OnMouseWheelPixels(x, y, dx, dy) {
			pn.q.Invalidate()
			return true
		}
	}
	return true
}

// keyContent отдаёт клавишу корневому виджету содержимого.
func (pn *qsPanel) keyContent(e widget.KeyEvent) bool {
	root := pn.contentRoot()
	if root == nil {
		return false
	}
	if h, ok := root.(widget.KeyHandler); ok {
		h.OnKeyEvent(e)
		pn.q.Invalidate()
		return true
	}
	return false
}

// ─── Фокус и подсказки ───────────────────────────────────────────────────────

// OnMouseWheelPixels прокручивает сетку плиток, меняет громкость и яркость
// колесом над их ползунками, на вложенной странице отдаёт колесо содержимому.
func (q *QuickSettings) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	if !q.IsOpen() || !q.rich() {
		return false
	}
	return q.richWheel(x, y, dx, dy)
}

// AcceptsTab реализует widget.TabAcceptor: в открытой панели Tab ходит по её
// зонам, а не уводит фокус наружу (выйти можно по Esc).
func (q *QuickSettings) AcceptsTab() bool { return q.IsOpen() && q.rich() }

// SetFocused реализует widget.Focusable.
func (q *QuickSettings) SetFocused(focused bool) {
	if q.fs.Set(focused) {
		q.Invalidate()
	}
}

// IsFocused реализует widget.Focusable.
func (q *QuickSettings) IsFocused() bool { return q.fs.IsFocused() }

// FocusVisible — видна ли рамка фокуса (фокус пришёл с клавиатуры).
func (q *QuickSettings) FocusVisible() bool { return q.fs.FocusVisible() }

// TabIndex исключает панель из обхода Tab, пока она закрыта или рисуется
// прежним видом.
func (q *QuickSettings) TabIndex() int {
	if !q.IsOpen() || !q.rich() {
		return -1
	}
	return 0
}

// ToolTipAt возвращает подсказку зоны под точкой: у значков без подписи —
// что они делают.
func (q *QuickSettings) ToolTipAt(x, y int) string {
	if !q.IsOpen() || !q.rich() {
		return ""
	}
	pn := q.pn
	pt := image.Pt(x, y)
	if !pt.In(q.rect()) {
		return ""
	}
	l, _ := pn.layout(q.rect())
	if pn.onDetailsPage() {
		if pt.In(l.back) {
			return tr(StrQuickBack)
		}
		return ""
	}
	z, _, _ := pn.zoneAt(pt)
	pn.mu.Lock()
	editing, vol, pow := pn.editing, pn.vol, pn.pow
	pn.mu.Unlock()
	switch z.kind {
	case qzEdit:
		if editing {
			return ""
		}
		return tr(StrQuickEdit)
	case qzSettings:
		return tr(StrQuickSettings)
	case qzChevron, qzVolChevron:
		return tr(StrQuickMore)
	case qzBrightIcon, qzBright:
		return tr(StrQuickBrightness)
	case qzVolIcon:
		if vol.Muted {
			return tr(StrQuickUnmute)
		}
		return tr(StrQuickMute)
	case qzVol:
		return tr(StrQuickVolume)
	}
	if !l.battery.Empty() && pt.In(l.battery) {
		key := StrQuickBattery
		if pow.OnAC {
			key = StrQuickCharging
		}
		return fmt.Sprintf(tr(key), int(pow.Charge*100+0.5))
	}
	return ""
}
