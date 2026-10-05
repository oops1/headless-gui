// notifyview_input.go — ввод в центре уведомлений Windows 10: мышь, колесо,
// клавиатура и действия карточек.
//
// Нажимается то, что нарисовано: попадание ищется по той же раскладке, что и
// рисование. Кнопки, как везде в репозитории, срабатывают на отпускании над
// той же зоной, над которой было нажатие.
package desktop

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// invalRect — область, которую надо перерисовать при смене состояния зоны k:
// вся карточка или заголовок группы (в них меняются и крестик, и подложка),
// иначе сама зона.
func (l *richLayout) invalRect(k zoneKey) image.Rectangle {
	if k.kind == zoneNone {
		return image.Rectangle{}
	}
	var base zoneKey
	switch {
	case k.note != 0:
		base = zoneKey{kind: zoneCard, note: k.note}
	case k.kind == zoneGroup || k.kind == zoneGroupClose || k.kind == zoneGroupToggle:
		base = zoneKey{kind: zoneGroup, app: k.app}
	default:
		base = k
	}
	if z, ok := l.find(base); ok {
		return z.hit
	}
	if z, ok := l.find(k); ok {
		return z.hit
	}
	return l.panel
}

// invalidate заявляет перерисовку области. Пустая область — не событие.
func (v *richView) invalidate(r image.Rectangle) {
	if !r.Empty() && v.src.invalid != nil {
		v.src.invalid(r)
	}
}

// invalidateAll заявляет перерисовку всей панели.
func (v *richView) invalidateAll() {
	if v.src.invalidAll != nil {
		v.src.invalidAll()
	}
}

// onMove отслеживает наведение. pt — точка курсора, nowhere — курсора над
// панелью больше нет.
func (v *richView) onMove(panel image.Rectangle, pt image.Point, nowhere bool) {
	l := v.layout(panel)
	var key zoneKey
	if !nowhere {
		if z, ok := l.zoneAt(pt); ok {
			key = z.key
		}
	}
	over := !nowhere && pt.In(l.viewport) && l.maxScrl > 0

	v.mu.Lock()
	old := v.hover
	v.hover = key
	hot := false
	if key.kind == zoneOption && v.dropHot != key.index {
		v.dropHot, hot = key.index, true
	}
	v.mu.Unlock()

	if over {
		v.poke()
	}
	if old != key {
		v.invalidate(l.invalRect(old))
		v.invalidate(l.invalRect(key))
	}
	if hot {
		v.invalidate(l.dropRect)
	}
}

// poke показывает бегунок полосы прокрутки и ставит таймер его скрытия.
//
// Перерисовывается только полоса бегунка и только в момент появления: движение
// мыши над списком не должно заставлять его перерисовываться на каждом шаге.
func (v *richView) poke() {
	v.mu.Lock()
	now := time.Now()
	was := now.Before(v.thumbUntil)
	v.thumbUntil = now.Add(ncThumbHold)
	if v.thumbTimer == nil {
		v.thumbTimer = time.AfterFunc(ncThumbHold, v.thumbExpired)
	}
	strip := v.lastLayout.thumbStrip()
	v.mu.Unlock()
	if !was {
		v.invalidate(strip)
	}
}

// thumbStrip — полоса списка, в которой рисуется бегунок.
func (l *richLayout) thumbStrip() image.Rectangle {
	vp := l.viewport
	return image.Rect(vp.Max.X-l.m.sbW-4, vp.Min.Y, vp.Max.X, vp.Max.Y)
}

// thumbExpired перезапускает таймер на остаток времени или гасит бегунок.
// Один таймер на вид, а не по таймеру на каждое движение мыши.
func (v *richView) thumbExpired() {
	v.mu.Lock()
	left := time.Until(v.thumbUntil)
	if left > 0 {
		v.thumbTimer.Reset(left)
		v.mu.Unlock()
		return
	}
	v.thumbTimer = nil
	strip := v.lastLayout.thumbStrip()
	v.mu.Unlock()
	v.invalidate(strip)
}

// scrollBy сдвигает список на dy точек (положительное — вниз) и сообщает,
// сдвинулся ли он.
func (v *richView) scrollBy(dy int) bool {
	v.mu.Lock()
	max := v.lastLayout.maxScrl
	old := v.scroll
	v.scroll += dy
	if v.scroll > max {
		v.scroll = max
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
	moved := v.scroll != old
	vp := v.lastLayout.viewport
	v.mu.Unlock()
	if moved {
		v.poke()
		v.invalidate(vp)
	}
	return moved
}

// onWheel прокручивает список колесом; возвращает true, если событие принято.
func (v *richView) onWheel(panel image.Rectangle, pt image.Point, dy float64) bool {
	if v.mode != ncModeCenter {
		return false
	}
	l := v.layout(panel)
	if !pt.In(l.panel) {
		return false
	}
	if l.maxScrl <= 0 || !pt.In(l.viewport) {
		return pt.In(l.panel)
	}
	step := int(dy)
	if step == 0 && dy != 0 {
		step = 1
		if dy < 0 {
			step = -1
		}
	}
	v.scrollBy(step)
	return true
}

// onButton разбирает кнопку мыши. Что панель принимает все нажатия в своих
// границах (иначе они проваливались бы сквозь неё), решает хозяин.
func (v *richView) onButton(panel image.Rectangle, e widget.MouseEvent) {
	l := v.layout(panel)
	pt := image.Pt(e.X, e.Y)
	z, hit := l.zoneAt(pt)

	if e.Pressed {
		// Нажатие мимо раскрытого списка закрывает его (и только его).
		v.mu.Lock()
		dropOpen := v.drop != (ncInputKey{})
		v.mu.Unlock()
		if dropOpen && !(hit && z.key.kind == zoneOption) && !(hit && z.key.kind == zoneSelect) {
			v.closeDrop(l)
			if !hit {
				return
			}
		}
		// Полоса прокрутки: щелчок по ней переносит список.
		if v.mode == ncModeCenter && l.maxScrl > 0 && pt.In(l.viewport) && pt.X >= l.viewport.Max.X-l.m.sbW-8 {
			frac := float64(pt.Y-l.viewport.Min.Y) / float64(l.viewport.Dy())
			v.mu.Lock()
			v.scroll = int(frac * float64(l.maxScrl))
			v.mu.Unlock()
			v.poke()
			v.invalidate(l.viewport)
			return
		}
		v.mu.Lock()
		v.pressed = zoneKey{}
		if hit {
			v.pressed = z.key
			if z.focusable {
				v.focus = z.key
			}
		}
		v.mu.Unlock()
		if hit {
			if z.key.kind == zoneReply {
				v.placeCaret(l, z, pt.X)
			}
			v.invalidate(l.invalRect(z.key))
		}
		return
	}

	v.mu.Lock()
	was := v.pressed
	v.pressed = zoneKey{}
	v.mu.Unlock()
	if was.kind == zoneNone {
		return
	}
	v.invalidate(l.invalRect(was))
	if hit && z.key == was {
		v.activate(l, z)
	}
}

// placeCaret ставит каретку поля ответа под точку x.
func (v *richView) placeCaret(l *richLayout, z zone, x int) {
	k := ncInputKey{z.key.note, z.key.action}
	v.mu.Lock()
	rp := v.reply[k]
	if rp == nil {
		v.mu.Unlock()
		return
	}
	pad := l.m.cardPad - 4
	rel := x - (z.rect.Min.X + pad)
	pos := 0
	for pos < len(rp.text) && l.fonts.body.width(string(rp.text[:pos+1])) <= rel {
		pos++
	}
	rp.caret = pos
	v.mu.Unlock()
}

// findNote ищет уведомление в текущих данных.
func (v *richView) findNote(id NotificationID) (Notification, bool) {
	if v.src.notes == nil {
		return Notification{}, false
	}
	for _, n := range v.src.notes() {
		if n.ID == id {
			return n, true
		}
	}
	return Notification{}, false
}

// closeDrop закрывает выпадающий список.
func (v *richView) closeDrop(l *richLayout) {
	v.mu.Lock()
	had := v.drop != (ncInputKey{})
	v.drop, v.dropHot = ncInputKey{}, 0
	v.mu.Unlock()
	if had {
		v.invalidate(l.panel)
	}
}

// inputs собирает текущие значения полей уведомления для события.
func (v *richView) inputs(n Notification) map[string]string {
	var out map[string]string
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, a := range n.Actions {
		switch a.Kind {
		case NotificationActionSelect:
			if len(a.Options) == 0 {
				continue
			}
			if out == nil {
				out = map[string]string{}
			}
			out[a.ID] = a.Options[v.selected(n, a)]
		case NotificationActionReply:
			if rp := v.reply[ncInputKey{n.ID, a.ID}]; rp != nil && len(rp.text) > 0 {
				if out == nil {
					out = map[string]string{}
				}
				out[a.ID] = string(rp.text)
			}
		}
	}
	return out
}

// fire отдаёт потребителю действие пользователя.
func (v *richView) fire(n Notification, a NotificationAction, value string, index int, keep bool) {
	ev := NotificationActionEvent{
		Notification: n.ID, Action: a.ID, Kind: a.Kind,
		Value: value, Index: index, Inputs: v.inputs(n),
	}
	if v.src.action != nil {
		v.src.action(ev, keep)
	}
}

// activate выполняет действие зоны: нажатие мышью либо Enter и пробел.
func (v *richView) activate(l *richLayout, z zone) {
	k := z.key
	switch k.kind {
	case zoneManage:
		if v.src.manage != nil {
			v.src.manage()
		}
	case zoneClear:
		v.dismissWhere(func(Notification) bool { return true })
	case zoneExpand:
		v.mu.Lock()
		v.quickOpen = !v.quickOpen
		v.mu.Unlock()
		v.invalidate(l.panel)
	case zoneGroup, zoneGroupToggle:
		v.mu.Lock()
		if v.collapsed == nil {
			v.collapsed = map[AppID]bool{}
		}
		v.collapsed[k.app] = !v.collapsed[k.app]
		v.mu.Unlock()
		v.invalidate(l.viewport)
	case zoneGroupClose:
		v.dismissWhere(func(n Notification) bool { return n.AppID == k.app })
	case zoneCard:
		if n, ok := v.findNote(k.note); ok {
			v.fire(n, NotificationAction{}, "", -1, false)
		}
	case zoneCardClose:
		if v.src.dismiss != nil {
			v.src.dismiss(k.note)
		}
	case zoneCardToggle:
		n, ok := v.findNote(k.note)
		if !ok {
			return
		}
		v.mu.Lock()
		if v.bodyOpen == nil {
			v.bodyOpen = map[NotificationID]bool{}
		}
		v.bodyOpen[k.note] = !v.cardOpen(n)
		v.mu.Unlock()
		v.invalidate(l.viewport)
	case zoneAction:
		n, ok := v.findNote(k.note)
		if !ok {
			return
		}
		for _, a := range n.Actions {
			if a.ID == k.action {
				v.fire(n, a, "", -1, a.Keep)
				return
			}
		}
	case zoneSelect:
		n, ok := v.findNote(k.note)
		if !ok {
			return
		}
		v.mu.Lock()
		key := ncInputKey{k.note, k.action}
		if v.drop == key {
			v.drop, v.dropHot = ncInputKey{}, 0
		} else {
			v.drop = key
			for _, a := range n.Actions {
				if a.ID == k.action {
					v.dropHot = v.selected(n, a)
				}
			}
		}
		v.mu.Unlock()
		v.invalidate(l.panel)
	case zoneOption:
		n, ok := v.findNote(k.note)
		if !ok {
			return
		}
		for _, a := range n.Actions {
			if a.ID != k.action || k.index < 0 || k.index >= len(a.Options) {
				continue
			}
			v.mu.Lock()
			if v.sel == nil {
				v.sel = map[ncInputKey]int{}
			}
			v.sel[ncInputKey{k.note, k.action}] = k.index
			v.drop, v.dropHot = ncInputKey{}, 0
			v.mu.Unlock()
			v.invalidate(l.panel)
			v.fire(n, a, a.Options[k.index], k.index, true)
			return
		}
	case zoneReply:
		// Нажатие в поле только ставит в него фокус (см. onButton).
	case zoneReplySend:
		v.sendReply(k.note, k.action)
	case zoneTile:
		if v.src.toggle != nil {
			v.src.toggle(QuickActionID(k.action))
		}
	}
}

// dismissWhere снимает уведомления, подходящие под условие.
func (v *richView) dismissWhere(match func(Notification) bool) {
	if v.src.notes == nil || v.src.dismiss == nil {
		return
	}
	for _, n := range v.src.notes() {
		if match(n) {
			v.src.dismiss(n.ID)
		}
	}
}

// sendReply отправляет введённый ответ.
func (v *richView) sendReply(note NotificationID, action string) {
	n, ok := v.findNote(note)
	if !ok {
		return
	}
	for _, a := range n.Actions {
		if a.ID != action {
			continue
		}
		key := ncInputKey{note, action}
		v.mu.Lock()
		rp := v.reply[key]
		if rp == nil || len(rp.text) == 0 {
			v.mu.Unlock()
			return
		}
		text := string(rp.text)
		v.mu.Unlock()
		// Ответ уходит вместе с текстом; поле очищается после, чтобы событие
		// успело забрать его через Inputs.
		v.fire(n, a, text, -1, a.Keep)
		v.mu.Lock()
		delete(v.reply, key)
		v.mu.Unlock()
		v.invalidateAll()
		return
	}
}

// prune забывает состояние снятых уведомлений и опустевших групп.
func (v *richView) prune(live []Notification) {
	ids := make(map[NotificationID]bool, len(live))
	apps := map[AppID]bool{}
	for _, n := range live {
		ids[n.ID] = true
		apps[n.AppID] = true
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for id := range v.bodyOpen {
		if !ids[id] {
			delete(v.bodyOpen, id)
		}
	}
	for k := range v.sel {
		if !ids[k.note] {
			delete(v.sel, k)
		}
	}
	for k := range v.reply {
		if !ids[k.note] {
			delete(v.reply, k)
		}
	}
	for app := range v.collapsed {
		if !apps[app] {
			delete(v.collapsed, app)
		}
	}
	if v.drop != (ncInputKey{}) && !ids[v.drop.note] {
		v.drop, v.dropHot = ncInputKey{}, 0
	}
	if v.focus.note != 0 && !ids[v.focus.note] {
		v.focus = zoneKey{}
	}
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

// onKey разбирает клавишу. Возвращает true, если клавиша обработана. Esc, не
// закрывающий выпадающий список, остаётся хозяину панели.
func (v *richView) onKey(panel image.Rectangle, e widget.KeyEvent) bool {
	if !e.Pressed {
		return false
	}
	l := v.layout(panel)

	v.mu.Lock()
	foc := v.focus
	dropOpen := v.drop != (ncInputKey{})
	v.mu.Unlock()

	if e.Code == widget.KeyEscape {
		if dropOpen {
			v.closeDrop(l)
			return true
		}
		return false
	}

	if dropOpen && v.dropKey(l, e) {
		return true
	}
	if foc.kind == zoneReply && v.replyKey(l, foc, e) {
		return true
	}

	plain := e.Mod&(widget.ModCtrl|widget.ModAlt|widget.ModMeta) == 0
	switch e.Code {
	case widget.KeyTab:
		if e.Mod&widget.ModShift != 0 {
			v.moveFocus(l, -1)
		} else {
			v.moveFocus(l, +1)
		}
		return true
	case widget.KeyDown:
		if foc.kind == zoneTile && v.moveTile(l, foc, +l.m.qCols) {
			return true
		}
		v.moveFocus(l, +1)
		return true
	case widget.KeyUp:
		if foc.kind == zoneTile && v.moveTile(l, foc, -l.m.qCols) {
			return true
		}
		v.moveFocus(l, -1)
		return true
	case widget.KeyRight:
		switch foc.kind {
		case zoneTile:
			v.moveTile(l, foc, +1)
		case zoneCard:
			v.setCardOpen(l, foc.note, true)
		case zoneGroup:
			v.setGroupCollapsed(l, foc.app, false)
		default:
			v.moveFocus(l, +1)
		}
		return true
	case widget.KeyLeft:
		switch foc.kind {
		case zoneTile:
			v.moveTile(l, foc, -1)
		case zoneCard:
			v.setCardOpen(l, foc.note, false)
		case zoneGroup:
			v.setGroupCollapsed(l, foc.app, true)
		default:
			v.moveFocus(l, -1)
		}
		return true
	case widget.KeyHome:
		v.focusEdge(l, false)
		return true
	case widget.KeyEnd:
		v.focusEdge(l, true)
		return true
	case widget.KeyPageDown:
		v.scrollBy(l.viewport.Dy() * 9 / 10)
		return true
	case widget.KeyPageUp:
		v.scrollBy(-l.viewport.Dy() * 9 / 10)
		return true
	case widget.KeyDelete:
		if plain {
			switch foc.kind {
			case zoneCard:
				v.activate(l, zone{key: zoneKey{kind: zoneCardClose, note: foc.note}})
				return true
			case zoneGroup:
				v.activate(l, zone{key: zoneKey{kind: zoneGroupClose, app: foc.app}})
				return true
			}
		}
	case widget.KeyEnter, widget.KeySpace:
		if !plain || e.Repeat || foc.kind == zoneNone {
			return foc.kind != zoneNone
		}
		if z, ok := l.find(foc); ok {
			v.activate(l, z)
		}
		return true
	}
	return false
}

// moveFocus переносит фокус на соседнюю остановку; по кругу.
func (v *richView) moveFocus(l *richLayout, delta int) {
	stops := l.stops()
	if len(stops) == 0 {
		return
	}
	v.mu.Lock()
	cur := -1
	for i, s := range stops {
		if s.key == v.focus {
			cur = i
			break
		}
	}
	var next int
	switch {
	case cur < 0 && delta > 0:
		next = 0
	case cur < 0:
		next = len(stops) - 1
	default:
		next = ((cur+delta)%len(stops) + len(stops)) % len(stops)
	}
	old := v.focus
	v.focus = stops[next].key
	v.mu.Unlock()
	v.reveal(l, stops[next])
	v.invalidate(l.invalRect(old))
	v.invalidate(l.invalRect(stops[next].key))
}

// focusEdge — фокус на первую или последнюю остановку.
func (v *richView) focusEdge(l *richLayout, last bool) {
	stops := l.stops()
	if len(stops) == 0 {
		return
	}
	z := stops[0]
	if last {
		z = stops[len(stops)-1]
	}
	v.mu.Lock()
	old := v.focus
	v.focus = z.key
	v.mu.Unlock()
	v.reveal(l, z)
	v.invalidate(l.invalRect(old))
	v.invalidate(l.invalRect(z.key))
}

// moveTile переносит фокус по сетке плиток на shift позиций; ложь — выхода за
// сетку нет и фокус остался.
func (v *richView) moveTile(l *richLayout, foc zoneKey, shift int) bool {
	idx := -1
	for i, t := range l.tiles {
		if string(t.a.ID) == foc.action {
			idx = i
		}
	}
	if idx < 0 {
		return false
	}
	// Влево и вправо ходим по ряду, не заходя в соседний.
	if shift == 1 || shift == -1 {
		if (shift < 0 && idx%l.m.qCols == 0) || (shift > 0 && idx%l.m.qCols == l.m.qCols-1) {
			return true
		}
	}
	for j := idx + shift; j >= 0 && j < len(l.tiles); j += shift {
		if l.tiles[j].a.Disabled {
			if shift == 1 || shift == -1 {
				continue
			}
			return false
		}
		v.mu.Lock()
		old := v.focus
		v.focus = zoneKey{kind: zoneTile, action: string(l.tiles[j].a.ID)}
		now := v.focus
		v.mu.Unlock()
		v.invalidate(l.invalRect(old))
		v.invalidate(l.invalRect(now))
		return true
	}
	return shift == 1 || shift == -1
}

// reveal прокручивает список так, чтобы зона z была видна целиком.
func (v *richView) reveal(l *richLayout, z zone) {
	if v.mode != ncModeCenter || z.rect.Empty() {
		return
	}
	k := z.key.kind
	if k == zoneManage || k == zoneClear || k == zoneExpand || k == zoneTile {
		return
	}
	vp := l.viewport
	switch {
	case z.rect.Min.Y < vp.Min.Y:
		v.scrollBy(z.rect.Min.Y - vp.Min.Y)
	case z.rect.Max.Y > vp.Max.Y:
		d := z.rect.Max.Y - vp.Max.Y
		if z.rect.Dy() > vp.Dy() {
			d = z.rect.Min.Y - vp.Min.Y
		}
		v.scrollBy(d)
	}
}

func (v *richView) setCardOpen(l *richLayout, id NotificationID, open bool) {
	n, ok := v.findNote(id)
	if !ok {
		return
	}
	v.mu.Lock()
	if v.cardOpen(n) == open {
		v.mu.Unlock()
		return
	}
	if v.bodyOpen == nil {
		v.bodyOpen = map[NotificationID]bool{}
	}
	v.bodyOpen[id] = open
	v.mu.Unlock()
	v.invalidate(l.viewport)
}

func (v *richView) setGroupCollapsed(l *richLayout, app AppID, c bool) {
	v.mu.Lock()
	if v.collapsed == nil {
		v.collapsed = map[AppID]bool{}
	}
	if v.collapsed[app] == c {
		v.mu.Unlock()
		return
	}
	v.collapsed[app] = c
	v.mu.Unlock()
	v.invalidate(l.viewport)
}

// dropKey разбирает клавиши раскрытого списка: стрелки двигают подсветку,
// Enter и пробел выбирают пункт.
func (v *richView) dropKey(l *richLayout, e widget.KeyEvent) bool {
	n := len(l.dropItems)
	if n == 0 {
		return false
	}
	switch e.Code {
	case widget.KeyDown, widget.KeyUp:
		d := 1
		if e.Code == widget.KeyUp {
			d = -1
		}
		v.mu.Lock()
		v.dropHot = (v.dropHot + d + n) % n
		v.mu.Unlock()
		v.invalidate(l.dropRect)
		return true
	case widget.KeyEnter, widget.KeySpace:
		if e.Repeat {
			return true
		}
		v.mu.Lock()
		hot := v.dropHot
		v.mu.Unlock()
		v.activate(l, zone{key: zoneKey{kind: zoneOption, note: l.dropNote, action: l.dropAct, index: hot}})
		return true
	case widget.KeyTab:
		v.closeDrop(l)
		return false
	}
	return false
}

// replyKey редактирует поле ответа.
func (v *richView) replyKey(l *richLayout, foc zoneKey, e widget.KeyEvent) bool {
	key := ncInputKey{foc.note, foc.action}
	ctrl := e.Mod&(widget.ModCtrl|widget.ModMeta) != 0
	v.mu.Lock()
	if v.reply == nil {
		v.reply = map[ncInputKey]*ncReply{}
	}
	rp := v.reply[key]
	if rp == nil {
		rp = &ncReply{}
		v.reply[key] = rp
	}
	changed := true
	handled := true
	switch {
	case e.Code == widget.KeyEnter:
		v.mu.Unlock()
		v.sendReply(foc.note, foc.action)
		return true
	case e.Code == widget.KeyBackspace:
		if rp.caret > 0 {
			rp.text = append(rp.text[:rp.caret-1], rp.text[rp.caret:]...)
			rp.caret--
		}
	case e.Code == widget.KeyDelete:
		if rp.caret < len(rp.text) {
			rp.text = append(rp.text[:rp.caret], rp.text[rp.caret+1:]...)
		}
	case e.Code == widget.KeyLeft:
		if rp.caret > 0 {
			rp.caret--
		}
	case e.Code == widget.KeyRight:
		if rp.caret < len(rp.text) {
			rp.caret++
		}
	case e.Code == widget.KeyHome:
		rp.caret = 0
	case e.Code == widget.KeyEnd:
		rp.caret = len(rp.text)
	case ctrl && (e.Code == widget.KeyV || e.Rune == 'v' || e.Rune == 'V'):
		ins := []rune(widget.ClipboardGetText())
		clean := ins[:0]
		for _, r := range ins {
			if r >= 32 {
				clean = append(clean, r)
			}
		}
		rp.text = append(rp.text[:rp.caret], append(append([]rune(nil), clean...), rp.text[rp.caret:]...)...)
		rp.caret += len(clean)
	case !ctrl && e.Mod&widget.ModAlt == 0 && e.Rune >= 32:
		rp.text = append(rp.text[:rp.caret], append([]rune{e.Rune}, rp.text[rp.caret:]...)...)
		rp.caret++
	default:
		changed, handled = false, false
	}
	v.mu.Unlock()
	if changed {
		v.invalidate(l.invalRect(foc))
	}
	return handled
}
