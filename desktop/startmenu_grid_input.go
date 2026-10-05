// startmenu_grid_input.go — мышь, клавиатура, поиск и перетаскивание закреплённых
// меню «Пуск» Windows 11.
//
// Всё, что зовётся из горутины кадра. Правила замков те же, что у остального
// меню: замок держится только вокруг состояния, а обработчики потребителя
// (запуск, колбэки, контекстное меню) и перерисовка зовутся после его
// отпускания. Общее с видом Windows 10 — запрос, результаты, список «Все
// приложения», наведение, всплывающее меню — живёт в startView и
// startmenu_tiles*.go и здесь не повторяется.
package desktop

import (
	"image"
	"strings"
	"time"
	"unicode"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// recRect — прямоугольник строки «Рекомендуем» с номером i.
func (g gridGeo) recRect(i int) image.Rectangle {
	if g.recCols < 1 {
		return image.Rectangle{}
	}
	colW := g.recList.Dx() / g.recCols
	return image.Rect(g.recList.Min.X+(i%g.recCols)*colW, g.recList.Min.Y+(i/g.recCols)*g.recRowH,
		g.recList.Min.X+(i%g.recCols+1)*colW, g.recList.Min.Y+(i/g.recCols+1)*g.recRowH)
}

// pageOf возвращает страницу, зажатую в допустимые пределы раскладки g.
func (m *StartMenu) pageOf(g gridGeo) int { return clampInt(m.pageNow(), 0, g.pages-1) }

// ─── Попадание ───────────────────────────────────────────────────────────────

// hitTestGrid определяет область и объект под точкой.
func (m *StartMenu) hitTestGrid(pt image.Point) startHit {
	panel := m.rect()
	if panel.Empty() || !pt.In(panel) {
		return startHit{}
	}
	g := m.gridGeometry(panel)
	switch {
	case pt.In(g.search):
		return startHit{areaSearch, keySearch}
	case pt.In(g.user):
		return startHit{areaFooter, keyUser}
	case pt.In(g.power):
		return startHit{areaFooter, keyPower}
	case pt.In(g.footer):
		return startHit{area: areaFooter}
	}
	if g.mode == gridMain {
		return m.hitMain(g, pt)
	}
	if (g.mode == gridList || g.mode == gridMore) && pt.In(g.backBtn) {
		return startHit{areaList, keyBack}
	}
	return m.hitList(g, pt)
}

func (m *StartMenu) hitMain(g gridGeo, pt image.Point) startHit {
	if pt.In(g.allBtn) {
		return startHit{areaPinned, keyAll}
	}
	if g.recRows > 0 && pt.In(g.moreBtn) {
		return startHit{areaRec, keyMore}
	}
	if g.pages > 1 && pt.In(g.dots) {
		d, gap := m.gm(KeyStartW11DotSize, 6), m.gm(KeyStartW11DotGap, 8)
		h := g.pages*d + (g.pages-1)*gap
		y0 := (g.dots.Min.Y+g.dots.Max.Y)/2 - h/2
		i := clampInt((pt.Y-y0+gap/2)/(d+gap), 0, g.pages-1)
		return startHit{areaPinned, prefDot + itoa(i)}
	}
	pinned := m.pinnedList()
	first := m.pageOf(g) * g.perPage
	for i := first; i < len(pinned) && i < first+g.perPage; i++ {
		if pt.In(g.cellRect(i - first)) {
			return startHit{areaPinned, prefPin + pinned[i].ID}
		}
	}
	if g.recRows > 0 {
		items := m.recommendedList()
		for i := 0; i < g.recCols*g.recRows && i < len(items); i++ {
			if pt.In(g.recRect(i)) {
				return startHit{areaRec, prefRec + items[i].ID}
			}
		}
	}
	return startHit{}
}

// hitList — попадание в общий список (буквы, строки, полоса прокрутки).
func (m *StartMenu) hitList(g gridGeo, pt image.Point) startHit {
	if !pt.In(g.list) {
		return startHit{}
	}
	if m.gridOpen() {
		if c, ok := m.gridCellAt(g.list, pt); ok && c.active {
			return startHit{areaList, prefGrid + c.letter}
		}
		return startHit{area: areaList}
	}
	sg := startGeo{inner: g.panel, list: g.list}
	if k := m.barHit(sg, areaList, pt); k != "" {
		return startHit{areaList, k}
	}
	rows, contentH := m.listRows()
	m.v.mu.Lock()
	scroll := clampScroll(m.v.listScroll, contentH, g.list.Dy())
	m.v.mu.Unlock()
	if i := firstRowAt(rows, scroll+pt.Y-g.list.Min.Y); i < len(rows) && rows[i].kind.focusable() {
		return startHit{areaList, prefRow + rows[i].key}
	}
	return startHit{area: areaList}
}

// gridKeyRect возвращает прямоугольник объекта вида Windows 11 для
// перерисовки; ok=false — ключ не этого вида (строки списка, дорожки полос и
// сетка букв ищет общий keyRect).
func (m *StartMenu) gridKeyRect(key string) (image.Rectangle, bool) {
	panel := m.rect()
	if panel.Empty() {
		return image.Rectangle{}, false
	}
	g := m.gridGeometry(panel)
	switch {
	case key == keySearch:
		return g.search.Inset(-2), true
	case key == keyUser:
		return g.user, true
	case key == keyPower:
		return g.power, true
	case key == keyAll:
		return g.allBtn, true
	case key == keyMore:
		return g.moreBtn, true
	case key == keyBack:
		return g.backBtn, true
	case strings.HasPrefix(key, prefDot):
		return g.dots, true
	case strings.HasPrefix(key, prefPin):
		id := strings.TrimPrefix(key, prefPin)
		first := m.pageOf(g) * g.perPage
		for i, p := range m.pinnedList() {
			if p.ID == id && i >= first && i < first+g.perPage && g.mode == gridMain {
				return g.cellRect(i - first), true
			}
		}
		return image.Rectangle{}, true
	case strings.HasPrefix(key, prefRec):
		id := strings.TrimPrefix(key, prefRec)
		for i, r := range m.recommendedList() {
			if r.ID == id && i < g.recCols*g.recRows && g.mode == gridMain {
				return g.recRect(i), true
			}
		}
		return image.Rectangle{}, true
	}
	return image.Rectangle{}, false
}

// ─── Мышь ────────────────────────────────────────────────────────────────────

func (m *StartMenu) mouseMoveGrid(x, y int) {
	pt := image.Pt(x, y)
	v := m.v
	v.mu.Lock()
	v.mouse, v.mouseOK = pt, pt.In(m.rect())
	barDragging := v.bar.active
	v.mu.Unlock()
	m.g.mu.Lock()
	dragging := m.g.drag.pending || m.g.drag.active
	m.g.mu.Unlock()
	if barDragging {
		m.barMove(pt)
		return
	}
	if dragging {
		m.dragMovePin(pt)
		return
	}
	h := m.hitTestGrid(pt)
	if h.area == areaList && m.scrollable(areaList) {
		v.listBar.poke(m.invalListBar)
	}
	m.setHover(h.key)
}

// holdingGrid — мышь «зажата» на чём-то, что ведут за курсором: ячейка или
// бегунок полосы.
func (m *StartMenu) holdingGrid() bool {
	m.g.mu.Lock()
	d := m.g.drag.pending || m.g.drag.active
	m.g.mu.Unlock()
	return d || m.holding()
}

// mouseButtonGrid разбирает кнопки мыши. Второй результат — «разобрано здесь»;
// если ложь, событие отдаётся встроенной панели (она закрывает меню кликом мимо).
func (m *StartMenu) mouseButtonGrid(e widget.MouseEvent) (handled, ok bool) {
	if !m.IsOpen() {
		return false, false
	}
	pt := image.Pt(e.X, e.Y)
	if !pt.In(m.rect()) {
		// Отпускание за пределами меню всё равно заканчивает перенос ячейки или
		// ведение бегунка: мышь захвачена, без этого состояние осталось бы зажатым.
		if e.Button == widget.MouseLeft && !e.Pressed && m.holdingGrid() {
			return m.leftButtonGrid(e, pt), true
		}
		return false, false
	}
	switch e.Button {
	case widget.MouseWheelUp, widget.MouseWheelDown:
		if e.Pressed {
			dir := 1
			if e.Button == widget.MouseWheelUp {
				dir = -1
			}
			m.wheelGrid(pt, dir, 3*m.metricInt(KeyStartMenuRowHeight))
		}
		return true, true
	case widget.MouseLeft:
		return m.leftButtonGrid(e, pt), true
	case widget.MouseRight:
		return m.rightButtonGrid(e, pt), true
	}
	return false, false
}

// wheelGrid — колесо: в главном виде листает страницы закреплённых (не чаще
// одного раза в wheelFlipGap — трекпад шлёт десятки событий на один жест), в
// списке прокручивает на step пикселей.
func (m *StartMenu) wheelGrid(pt image.Point, dir, step int) {
	g := m.gridGeometry(m.rect())
	if g.mode != gridMain {
		if m.gridOpen() {
			return // сетка букв не прокручивается: колесо ничего не делает
		}
		if h := m.hitTestGrid(pt); h.area == areaList {
			m.scrollList(dir * step)
		}
		return
	}
	if g.pages <= 1 || !(pt.In(g.pinGrid) || pt.In(g.dots) || pt.In(g.pinTitle)) {
		return
	}
	m.g.mu.Lock()
	if time.Since(m.g.lastWheel) < wheelFlipGap {
		m.g.mu.Unlock()
		return
	}
	m.g.lastWheel = time.Now()
	m.g.mu.Unlock()
	m.setPage(m.pageOf(g) + dir)
}

// wheelFlipGap — наименьший промежуток между двумя сменами страницы колесом.
var wheelFlipGap = 220 * time.Millisecond

// setPage переходит на страницу p закреплённых (в пределах раскладки) и
// перерисовывает сетку. Выбор клавишей, оставшийся на другой странице, снимается.
func (m *StartMenu) setPage(p int) {
	panel := m.rect()
	if panel.Empty() {
		return
	}
	g := m.gridGeometry(panel)
	p = clampInt(p, 0, g.pages-1)
	m.g.mu.Lock()
	changed := m.g.page != p
	m.g.page = p
	m.g.mu.Unlock()
	if !changed {
		return
	}
	pinned := m.pinnedList()
	m.v.mu.Lock()
	sel := m.v.sel[areaPinned]
	if strings.HasPrefix(sel, prefPin) {
		keep := false
		for i := p * g.perPage; i < len(pinned) && i < (p+1)*g.perPage; i++ {
			if prefPin+pinned[i].ID == sel {
				keep = true
			}
		}
		if !keep {
			m.v.sel[areaPinned] = ""
		}
	}
	m.v.mu.Unlock()
	widget.InvalidateRect(g.pinGrid.Union(g.dots).Inset(-4))
	m.refreshHover()
}

// leftButtonGrid: нажатие запоминает объект (и готовит перетаскивание ячейки),
// отпускание на том же объекте — активирует его.
func (m *StartMenu) leftButtonGrid(e widget.MouseEvent, pt image.Point) bool {
	v := m.v
	h := m.hitTestGrid(pt)
	if e.Pressed {
		m.closeGridOnPress(h, pt)
		v.mu.Lock()
		v.press = h.key
		v.kbd = false
		if h.area != areaNone {
			v.area = h.area
		}
		isBar := isBarKey(h.key)
		v.mu.Unlock()
		if strings.HasPrefix(h.key, prefPin) {
			m.g.mu.Lock()
			m.g.drag = pinDrag{pending: true, id: strings.TrimPrefix(h.key, prefPin), start: pt, pos: pt}
			m.g.mu.Unlock()
		}
		if h.key == keySearch {
			m.placeCaretAt(pt.X)
			m.Invalidate()
		}
		// Нажатие уводит ввод в область: поле поиска теряет подсветку фокуса.
		m.invalidateKeys(keySearch)
		if isBar {
			m.barPress(h.area, pt)
			return true
		}
		m.invalidateKeys(h.key)
		return true
	}

	if m.barRelease() {
		v.mu.Lock()
		v.press = ""
		v.mu.Unlock()
		return true
	}
	v.mu.Lock()
	press := v.press
	v.press = ""
	cm := v.capture
	v.mu.Unlock()
	m.g.mu.Lock()
	drag := m.g.drag
	m.g.drag = pinDrag{}
	m.g.mu.Unlock()
	if cm != nil && (drag.pending || drag.active) {
		cm.ReleaseCapture()
	}
	m.invalidateKeys(press)
	if drag.active {
		m.dropPin(drag)
		return true
	}
	if press != "" && press == h.key {
		m.activate(press)
	}
	return press != ""
}

// rightButtonGrid: отпускание правой кнопки на объекте показывает контекстное
// меню потребителя.
func (m *StartMenu) rightButtonGrid(e widget.MouseEvent, pt image.Point) bool {
	if e.Pressed {
		return true
	}
	h := m.hitTestGrid(pt)
	if h.key == "" {
		return true
	}
	if t, ok := m.targetFor(h.key); ok {
		m.showContext(t, pt)
	}
	return true
}

// placeCaretAt ставит каретку поля поиска по координате щелчка.
func (m *StartMenu) placeCaretAt(x int) {
	g := m.gridGeometry(m.rect())
	pad := m.gm(KeyStartW11SearchPad, 12)
	left := g.search.Min.X + pad + m.gm(KeyStartW11SearchIcon, 16) + 8
	s := m.tpart("search", theme.StateNormal)
	size := fontPt(s)
	text := []rune(m.Query())
	rel := x - left
	best := 0
	for i := 1; i <= len(text); i++ {
		if measureUI(string(text[:i]), size) > rel {
			break
		}
		best = i
	}
	if best < len(text) {
		lo, hi := measureUI(string(text[:best]), size), measureUI(string(text[:best+1]), size)
		if rel > (lo+hi)/2 {
			best++
		}
	}
	m.g.mu.Lock()
	m.g.caret = best
	m.g.mu.Unlock()
}

// ─── Активация ───────────────────────────────────────────────────────────────

// activateGrid выполняет действие объекта вида Windows 11; false — ключ чужой
// (строки списка, буквы — их разбирает общий activate).
func (m *StartMenu) activateGrid(key string) bool {
	switch {
	case key == keySearch:
		return true
	case key == keyAll:
		m.SetView(StartViewAllApps)
	case key == keyMore:
		m.SetView(StartViewRecommended)
	case key == keyBack:
		m.SetView(StartViewMain)
	case key == keyUser:
		m.footerAction(StartTargetUser, "user")
	case key == keyPower:
		m.footerAction(StartTargetPower, "power")
	case strings.HasPrefix(key, prefDot):
		n := 0
		for _, c := range strings.TrimPrefix(key, prefDot) {
			n = n*10 + int(c-'0')
		}
		m.setPage(n)
	case strings.HasPrefix(key, prefPin):
		if p, ok := m.pinnedByID(strings.TrimPrefix(key, prefPin)); ok {
			m.launchPinned(p)
		}
	case strings.HasPrefix(key, prefRec):
		if r, ok := m.recommendedByID(strings.TrimPrefix(key, prefRec)); ok {
			m.launchRecommended(r)
		}
	default:
		return false
	}
	return true
}

// footerAction обрабатывает нажатие на пользователя или питание нижней полосы:
// меню потребителя (UserMenu, PowerMenu) над кнопкой, а без него — событие
// боковой панели Windows 10 с идентификатором "user" или "power".
func (m *StartMenu) footerAction(kind StartTargetKind, id string) {
	fn := m.UserMenu
	if kind == StartTargetPower {
		fn = m.PowerMenu
	}
	if fn == nil {
		if m.OnSidebarActivate != nil {
			m.OnSidebarActivate(id)
		}
		m.Close()
		return
	}
	items := fn()
	if len(items) == 0 {
		return
	}
	g := m.gridGeometry(m.rect())
	btn := g.user
	if kind == StartTargetPower {
		btn = g.power
	}
	p := m.ensurePopup()
	p.SetItems(items)
	themeMenu(p, m.tm)
	// Меню открывается над кнопкой: высота известна только после показа, поэтому
	// сначала показываем, затем ставим по измеренной высоте.
	p.Show(btn.Min.X, btn.Min.Y)
	ob := p.OverlayBounds()
	if ob.Empty() {
		return
	}
	x := btn.Min.X
	if kind == StartTargetPower {
		x = btn.Max.X - ob.Dx()
	}
	p.Show(x, btn.Min.Y-4-ob.Dy())
}

// ─── Цели контекстного меню ──────────────────────────────────────────────────

// targetForGrid описывает объект вида Windows 11 для контекстного меню.
func (m *StartMenu) targetForGrid(key string) (StartTarget, bool) {
	switch {
	case strings.HasPrefix(key, prefPin):
		id := strings.TrimPrefix(key, prefPin)
		if p, ok := m.pinnedByID(id); ok {
			return StartTarget{Kind: StartTargetPinned, Pinned: id, App: p.App}, true
		}
	case strings.HasPrefix(key, prefRec):
		id := strings.TrimPrefix(key, prefRec)
		if r, ok := m.recommendedByID(id); ok {
			return StartTarget{Kind: StartTargetRecommended, Recommended: id, App: r.App}, true
		}
	case key == keyUser:
		return StartTarget{Kind: StartTargetUser}, true
	case key == keyPower:
		return StartTarget{Kind: StartTargetPower}, true
	}
	return StartTarget{}, false
}

// ─── Перетаскивание закреплённых ─────────────────────────────────────────────

// pinDragOrder возвращает порядок закреплённых с перенесённой ячейкой на месте
// d.to; исходный срез не меняется.
func pinDragOrder(items []StartPinned, d pinDrag) []StartPinned {
	out := make([]StartPinned, 0, len(items))
	var moved StartPinned
	found := false
	for _, p := range items {
		if p.ID == d.id {
			moved, found = p, true
			continue
		}
		out = append(out, p)
	}
	if !found {
		return items
	}
	to := clampInt(d.to, 0, len(out))
	out = append(out, StartPinned{})
	copy(out[to+1:], out[to:])
	out[to] = moved
	return out
}

// dragMovePin продвигает перетаскивание: за порогом оно начинается, затем
// ячейка следует за курсором, а место вставки пересчитывается. Точка страниц под
// курсором переключает страницу.
func (m *StartMenu) dragMovePin(pt image.Point) {
	m.g.mu.Lock()
	d := m.g.drag
	m.g.mu.Unlock()
	if !d.pending && !d.active {
		return
	}
	if d.pending {
		if absInt(pt.X-d.start.X) < 4 && absInt(pt.Y-d.start.Y) < 4 {
			return
		}
		d.pending, d.active = false, true
	}
	d.pos = pt
	panel := m.rect()
	if panel.Empty() {
		return
	}
	g := m.gridGeometry(panel)
	if g.mode != gridMain {
		return
	}
	pinned := m.pinnedList()
	if g.pages > 1 && pt.In(g.dots) {
		if h := m.hitMain(g, pt); strings.HasPrefix(h.key, prefDot) {
			m.activateGrid(h.key)
			g = m.gridGeometry(panel)
		}
	}
	first := m.pageOf(g) * g.perPage
	// Место вставки: ячейка под курсором; пустое место сетки — конец страницы.
	to := d.to
	for i := first; i < len(pinned) && i < first+g.perPage; i++ {
		if pt.In(g.cellRect(i - first)) {
			to = i
		}
	}
	if pt.In(g.pinGrid) && to == d.to {
		// Курсор в зазоре между ячейками или правее последней: ближайший край.
		if last := first + g.perPage - 1; last >= len(pinned) {
			last = len(pinned) - 1
			if pt.Y > g.cellRect(last-first).Min.Y {
				to = last
			}
		}
	}
	d.to = clampInt(to, 0, len(pinned)-1)
	m.g.mu.Lock()
	m.g.drag = d
	m.g.mu.Unlock()
	m.v.mu.Lock()
	m.v.hover = ""
	m.v.mu.Unlock()
	m.Invalidate()
}

// dropPin заканчивает перетаскивание: новый порядок запоминается и сообщается
// потребителю; тот же порядок — не событие.
func (m *StartMenu) dropPin(d pinDrag) {
	old := m.pinnedList()
	next := pinDragOrder(old, d)
	same := len(next) == len(old)
	for i := 0; same && i < len(old); i++ {
		same = next[i].ID == old[i].ID
	}
	if !same {
		m.g.mu.Lock()
		m.g.pinned, m.g.pinnedSet = next, true
		m.g.mu.Unlock()
		if m.OnPinnedChanged != nil {
			m.OnPinnedChanged(append([]StartPinned(nil), next...))
		}
	}
	m.Invalidate()
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

// gridAreas возвращает области в порядке обхода Tab для текущего вида; пустые
// пропускаются.
func (m *StartMenu) gridAreas() []startArea {
	g := m.gridGeometry(m.rect())
	out := []startArea{areaSearch}
	if g.mode == gridMain {
		out = append(out, areaPinned)
		if g.recRows > 0 {
			out = append(out, areaRec)
		}
	} else {
		out = append(out, areaList)
	}
	return append(out, areaFooter)
}

// firstKey возвращает ключ, на который встаёт выбор при входе в область.
func (m *StartMenu) firstKey(a startArea) string {
	g := m.gridGeometry(m.rect())
	switch a {
	case areaSearch:
		return keySearch
	case areaPinned:
		first := m.pageOf(g) * g.perPage
		if pinned := m.pinnedList(); first < len(pinned) {
			return prefPin + pinned[first].ID
		}
		return keyAll
	case areaRec:
		if items := m.recommendedList(); len(items) > 0 {
			return prefRec + items[0].ID
		}
		return keyMore
	case areaFooter:
		return keyUser
	case areaList:
		if g.mode == gridList || g.mode == gridMore {
			return keyBack
		}
		if rows := m.interactiveRows(); len(rows) > 0 {
			return prefRow + rows[0].key
		}
	}
	return ""
}

// keyGridView разбирает клавишу. Возвращает true, если она использована; иначе
// её получит встроенная панель (Esc закрывает меню).
func (m *StartMenu) keyGridView(e widget.KeyEvent) bool {
	if !m.IsOpen() || !e.Pressed {
		return false
	}
	v := m.v
	v.mu.Lock()
	popup := v.popup
	v.mu.Unlock()
	if popup != nil && popup.IsOpen() {
		if e.Code == widget.KeyEscape {
			popup.Close()
		} else {
			popup.OnKeyEvent(e)
		}
		return true
	}
	if e.Code == widget.KeyEscape {
		if m.gridOpen() {
			m.CloseLetterGrid() // первый Esc закрывает сетку букв, второй — меню
			return true
		}
		return false
	}
	if e.Mod&widget.ModCtrl != 0 && e.Code == widget.KeyV {
		m.pasteIntoQuery()
		return true
	}
	if e.Mod&(widget.ModCtrl|widget.ModAlt|widget.ModMeta) != 0 {
		return false
	}
	if e.Code == widget.KeyTab {
		m.CloseLetterGrid()
		m.cycleGridArea(e.Mod&widget.ModShift != 0)
		return true
	}
	v.mu.Lock()
	area := v.area
	v.mu.Unlock()
	if area == areaNone || area == areaTiles || area == areaSidebar {
		area = areaSearch
	}

	// Backspace вне поля поиска (и в пустом поле) — «Назад» из списка.
	if e.Code == widget.KeyBackspace && m.Query() == "" && m.View() != StartViewMain {
		m.SetView(StartViewMain)
		return true
	}
	// Контекстное меню клавишей: Menu или Shift+F10 на выбранном объекте.
	if e.Code == widget.KeyMenu || (e.Code == widget.KeyF10 && e.Mod&widget.ModShift != 0) {
		m.contextForSelection(area)
		return true
	}
	// Набор символа вне поля поиска — переход в поиск; пробел остаётся активацией.
	if area != areaSearch && e.Rune > ' ' && e.Rune != 127 && !unicode.IsControl(e.Rune) {
		m.typeToSearch(e.Rune)
		return true
	}

	v.mu.Lock()
	v.kbd = true
	v.mu.Unlock()
	switch area {
	case areaSearch:
		m.keySearchField(e)
	case areaPinned:
		m.keyPinned(e)
	case areaRec:
		m.keyRec(e)
	case areaFooter:
		m.keyFooter(e)
	default:
		m.keyListView(e)
	}
	return true
}

// cycleGridArea переводит клавиатурный фокус на следующую (предыдущую) область.
func (m *StartMenu) cycleGridArea(reverse bool) {
	order := m.gridAreas()
	v := m.v
	v.mu.Lock()
	cur := v.area
	v.mu.Unlock()
	idx := 0
	for i, a := range order {
		if a == cur {
			idx = i
		}
	}
	step := 1
	if reverse {
		step = len(order) - 1
	}
	idx = (idx + step) % len(order)
	next := order[idx]
	v.mu.Lock()
	v.area = next
	v.kbd = true
	sel := v.sel[next]
	v.mu.Unlock()
	if next == areaSearch {
		m.Invalidate()
		return
	}
	if sel == "" || m.gridKeyGone(next, sel) {
		sel = m.firstKey(next)
	}
	m.setSelGrid(next, sel)
	m.Invalidate()
}

// gridKeyGone — выбранный ключ области уже не показан (страница сменилась,
// строка пропала).
func (m *StartMenu) gridKeyGone(a startArea, key string) bool {
	if a == areaList {
		return false
	}
	r, ok := m.gridKeyRect(key)
	return ok && r.Empty()
}

// setSelGrid запоминает выбор области и показывает выбранное: общий setSel для
// списка (он прокручивает), перерисовка двух объектов для остального.
func (m *StartMenu) setSelGrid(a startArea, key string) {
	if a == areaList {
		m.setSel(a, key)
		return
	}
	v := m.v
	v.mu.Lock()
	old := v.sel[a]
	v.sel[a] = key
	v.mu.Unlock()
	m.invalidateKeys(old, key)
}

// ─── Строка поиска: ввод ─────────────────────────────────────────────────────

// pasteIntoQuery вставляет в запрос текст буфера обмена (без переводов строк).
func (m *StartMenu) pasteIntoQuery() {
	clip := strings.TrimSpace(strings.ReplaceAll(widget.ClipboardGetText(), "\n", " "))
	if clip == "" {
		return
	}
	m.insertQuery(clip)
}

// insertQuery вставляет s в позицию каретки.
func (m *StartMenu) insertQuery(s string) {
	text := []rune(m.Query())
	m.g.mu.Lock()
	caret := clampInt(m.g.caret, 0, len(text))
	m.g.mu.Unlock()
	ins := []rune(s)
	next := append(append(append([]rune(nil), text[:caret]...), ins...), text[caret:]...)
	m.setQueryCaret(string(next), caret+len(ins))
}

// setQueryCaret ставит запрос и каретку.
func (m *StartMenu) setQueryCaret(q string, caret int) {
	m.SetQuery(q)
	m.g.mu.Lock()
	m.g.caret = clampInt(caret, 0, len([]rune(q)))
	m.g.mu.Unlock()
	m.Invalidate()
}

// keySearchField: правка запроса в строке поиска; Вниз уходит в содержимое.
func (m *StartMenu) keySearchField(e widget.KeyEvent) {
	text := []rune(m.Query())
	m.g.mu.Lock()
	caret := clampInt(m.g.caret, 0, len(text))
	m.g.mu.Unlock()
	switch e.Code {
	case widget.KeyBackspace:
		if caret > 0 {
			m.setQueryCaret(string(append(append([]rune(nil), text[:caret-1]...), text[caret:]...)), caret-1)
		}
		return
	case widget.KeyDelete:
		if caret < len(text) {
			m.setQueryCaret(string(append(append([]rune(nil), text[:caret]...), text[caret+1:]...)), caret)
		}
		return
	case widget.KeyLeft, widget.KeyRight, widget.KeyHome, widget.KeyEnd:
		switch e.Code {
		case widget.KeyLeft:
			caret--
		case widget.KeyRight:
			caret++
		case widget.KeyHome:
			caret = 0
		case widget.KeyEnd:
			caret = len(text)
		}
		m.g.mu.Lock()
		m.g.caret = clampInt(caret, 0, len(text))
		m.g.mu.Unlock()
		m.invalidateKeys(keySearch)
		return
	case widget.KeyDown, widget.KeyUp, widget.KeyPageDown, widget.KeyPageUp, widget.KeyEnter:
		if m.Query() != "" {
			m.SearchKey(e) // результаты: стрелки по ним, Enter открывает
			return
		}
		if e.Code == widget.KeyDown || e.Code == widget.KeyPageDown {
			m.moveGridArea(1)
		}
		return
	}
	if e.Rune >= ' ' && e.Rune != 127 && !unicode.IsControl(e.Rune) {
		m.insertQuery(string(e.Rune))
	}
}

// moveGridArea переходит на соседнюю область (вниз — вперёд).
func (m *StartMenu) moveGridArea(step int) {
	order := m.gridAreas()
	v := m.v
	v.mu.Lock()
	cur := v.area
	v.mu.Unlock()
	idx := 0
	for i, a := range order {
		if a == cur {
			idx = i
		}
	}
	idx = clampInt(idx+step, 0, len(order)-1)
	next := order[idx]
	v.mu.Lock()
	v.area = next
	v.mu.Unlock()
	m.setSelGrid(next, m.firstKey(next))
	m.Invalidate()
}

// ─── Закреплённые, рекомендуемые, нижняя полоса, список ──────────────────────

// pinnedPage возвращает закреплённые текущей страницы и номер первого.
func (m *StartMenu) pinnedPage(g gridGeo) (items []StartPinned, first int) {
	pinned := m.pinnedList()
	first = m.pageOf(g) * g.perPage
	if first > len(pinned) {
		first = len(pinned)
	}
	end := first + g.perPage
	if end > len(pinned) {
		end = len(pinned)
	}
	return pinned[first:end], first
}

// keyPinned: стрелки по сетке страницы, вверх с первого ряда — на «Все
// приложения», вниз с последнего — в «Рекомендуем», PageUp/PageDown листают
// страницы, Enter и Пробел запускают.
func (m *StartMenu) keyPinned(e widget.KeyEvent) {
	g := m.gridGeometry(m.rect())
	items, _ := m.pinnedPage(g)
	m.v.mu.Lock()
	cur := m.v.sel[areaPinned]
	m.v.mu.Unlock()

	if cur == keyAll {
		switch e.Code {
		case widget.KeyEnter, widget.KeySpace:
			if !e.Repeat {
				m.activate(keyAll)
			}
		case widget.KeyDown:
			if len(items) > 0 {
				m.setSelGrid(areaPinned, prefPin+items[0].ID)
			}
		case widget.KeyUp:
			m.v.mu.Lock()
			m.v.area = areaSearch
			m.v.mu.Unlock()
			m.invalidateKeys(keyAll, keySearch)
		}
		return
	}
	idx := -1
	for i, p := range items {
		if prefPin+p.ID == cur {
			idx = i
		}
	}
	if idx < 0 {
		// Первое нажатие уже что-то выбирает.
		switch e.Code {
		case widget.KeyDown, widget.KeyUp, widget.KeyLeft, widget.KeyRight, widget.KeyHome, widget.KeyEnd:
			if k := m.firstKey(areaPinned); k != "" {
				m.setSelGrid(areaPinned, k)
			}
		}
		return
	}
	cols := g.cols
	switch e.Code {
	case widget.KeyEnter, widget.KeySpace:
		if !e.Repeat {
			m.activate(cur)
		}
		return
	case widget.KeyLeft:
		if idx%cols > 0 {
			idx--
		}
	case widget.KeyRight:
		if idx%cols < cols-1 && idx+1 < len(items) {
			idx++
		}
	case widget.KeyUp:
		if idx < cols {
			m.setSelGrid(areaPinned, keyAll)
			return
		}
		idx -= cols
	case widget.KeyDown:
		switch {
		case idx+cols < len(items):
			idx += cols
		case idx/cols < (len(items)-1)/cols:
			idx = len(items) - 1 // ряд ниже короче: на его последнюю ячейку
		default:
			if g.recRows > 0 {
				m.moveGridArea(1)
			}
			return
		}
	case widget.KeyHome:
		idx = 0
	case widget.KeyEnd:
		idx = len(items) - 1
	case widget.KeyPageDown, widget.KeyPageUp:
		step := 1
		if e.Code == widget.KeyPageUp {
			step = -1
		}
		m.setPage(m.pageOf(g) + step)
		g = m.gridGeometry(m.rect())
		if its, _ := m.pinnedPage(g); len(its) > 0 {
			m.setSelGrid(areaPinned, prefPin+its[0].ID)
		}
		return
	default:
		return
	}
	m.setSelGrid(areaPinned, prefPin+items[idx].ID)
}

// keyRec: стрелки по двум колонкам, вверх с первого ряда — на «Дополнительно»,
// вниз с последнего — в нижнюю полосу.
func (m *StartMenu) keyRec(e widget.KeyEvent) {
	g := m.gridGeometry(m.rect())
	items := m.recommendedList()
	if n := g.recCols * g.recRows; len(items) > n {
		items = items[:n]
	}
	m.v.mu.Lock()
	cur := m.v.sel[areaRec]
	m.v.mu.Unlock()
	if cur == keyMore {
		switch e.Code {
		case widget.KeyEnter, widget.KeySpace:
			if !e.Repeat {
				m.activate(keyMore)
			}
		case widget.KeyDown:
			if len(items) > 0 {
				m.setSelGrid(areaRec, prefRec+items[0].ID)
			}
		case widget.KeyUp:
			m.v.mu.Lock()
			m.v.area = areaPinned
			m.v.mu.Unlock()
			pinned, _ := m.pinnedPage(g)
			if len(pinned) > 0 {
				m.setSelGrid(areaPinned, prefPin+pinned[len(pinned)-1].ID)
			}
			m.invalidateKeys(keyMore)
		}
		return
	}
	idx := -1
	for i, r := range items {
		if prefRec+r.ID == cur {
			idx = i
		}
	}
	if idx < 0 {
		if k := m.firstKey(areaRec); k != "" && len(items) > 0 {
			m.setSelGrid(areaRec, k)
		}
		return
	}
	cols := g.recCols
	switch e.Code {
	case widget.KeyEnter, widget.KeySpace:
		if !e.Repeat {
			m.activate(cur)
		}
		return
	case widget.KeyLeft:
		if idx%cols > 0 {
			idx--
		}
	case widget.KeyRight:
		if idx%cols < cols-1 && idx+1 < len(items) {
			idx++
		}
	case widget.KeyUp:
		if idx < cols {
			m.setSelGrid(areaRec, keyMore)
			return
		}
		idx -= cols
	case widget.KeyDown:
		switch {
		case idx+cols < len(items):
			idx += cols
		case idx/cols < (len(items)-1)/cols:
			idx = len(items) - 1
		default:
			m.moveGridArea(1)
			return
		}
	case widget.KeyHome:
		idx = 0
	case widget.KeyEnd:
		idx = len(items) - 1
	default:
		return
	}
	m.setSelGrid(areaRec, prefRec+items[idx].ID)
}

// keyFooter: Влево и Вправо — между пользователем и питанием, Вверх — в
// содержимое, Enter и Пробел нажимают.
func (m *StartMenu) keyFooter(e widget.KeyEvent) {
	m.v.mu.Lock()
	cur := m.v.sel[areaFooter]
	m.v.mu.Unlock()
	if cur == "" {
		cur = keyUser
		m.setSelGrid(areaFooter, cur)
	}
	switch e.Code {
	case widget.KeyLeft, widget.KeyHome:
		m.setSelGrid(areaFooter, keyUser)
	case widget.KeyRight, widget.KeyEnd:
		m.setSelGrid(areaFooter, keyPower)
	case widget.KeyUp:
		m.moveGridArea(-1)
	case widget.KeyEnter, widget.KeySpace:
		if !e.Repeat {
			m.activate(cur)
		}
	}
}

// keyListView: клавиши списка («Все приложения», «Все рекомендации», результаты)
// — общий keyList меню Windows 10, плюс переход на кнопку «Назад» вверху.
func (m *StartMenu) keyListView(e widget.KeyEvent) {
	if m.gridOpen() {
		m.keyGrid(e)
		return
	}
	g := m.gridGeometry(m.rect())
	hasBack := g.mode == gridList || g.mode == gridMore
	m.v.mu.Lock()
	cur := m.v.sel[areaList]
	m.v.mu.Unlock()
	if cur == keyBack {
		switch e.Code {
		case widget.KeyEnter, widget.KeySpace:
			if !e.Repeat {
				m.activate(keyBack)
			}
		case widget.KeyDown:
			m.setSel(areaList, "")
			m.keyList(e)
		case widget.KeyUp:
			m.v.mu.Lock()
			m.v.area = areaSearch
			m.v.mu.Unlock()
			m.invalidateKeys(keyBack, keySearch)
		}
		return
	}
	if hasBack && e.Code == widget.KeyUp {
		rows := m.focusRows()
		if len(rows) > 0 && (cur == "" || cur == prefRow+rows[0].key) {
			m.setSel(areaList, keyBack)
			return
		}
	}
	m.keyList(e)
}

// contextForSelection показывает контекстное меню выбранного клавишей объекта у
// его левого нижнего угла.
func (m *StartMenu) contextForSelection(a startArea) {
	m.v.mu.Lock()
	key := m.v.sel[a]
	m.v.mu.Unlock()
	if key == "" {
		return
	}
	t, ok := m.targetFor(key)
	if !ok {
		return
	}
	r := m.keyRect(key)
	if r.Empty() {
		return
	}
	m.showContext(t, image.Pt(r.Min.X+r.Dx()/2, r.Max.Y))
}
