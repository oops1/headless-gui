// startmenu_tiles_input.go — открытие и закрытие, мышь, клавиатура, поиск и
// перетаскивание плиток меню «Пуск» с плитками.
//
// Всё, что зовётся из горутины кадра. Правила замков те же, что у остальных
// компонентов: замок меню держится только вокруг состояния, а обработчики
// потребителя (запуск, колбэки, контекстное меню) и перерисовка зовутся после
// его отпускания.
package desktop

import (
	"image"
	"strings"
	"unicode"

	"github.com/oops1/headless-gui/v3/internal/focusreq"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

var (
	_ widget.Focusable        = (*StartMenu)(nil)
	_ widget.TabAcceptor      = (*StartMenu)(nil)
	_ widget.CaptureRequester = (*StartMenu)(nil)
	_ widget.CaptureAware     = (*StartMenu)(nil)
)

// ─── Открытие и закрытие ─────────────────────────────────────────────────────

// resetForOpen возвращает меню в исходное состояние: панель свёрнута, списки
// сверху, ни наведения, ни выбора. Старое состояние не должно пережить
// закрытие — иначе первая же стрелка прыгнула бы туда, где пользователь её не
// ждёт.
func (m *StartMenu) resetForOpen() {
	// Плоский вид: выделение прошлого раза не должно пережить закрытие, и когда
	// меню открывают через Toggle (а не StartMenu.Open) тоже.
	m.mu.Lock()
	m.hasSel = false
	m.mu.Unlock()
	v := m.v
	v.mu.Lock()
	v.sideOpen = false
	v.listScroll, v.tileScroll = 0, 0
	v.hover, v.press = "", ""
	v.sel = map[startArea]string{}
	v.kbd = false
	v.area = areaList
	v.drag = tileDrag{}
	v.bar = barDrag{}
	v.grid, v.gridFrom = false, ""
	if !v.keepFocus {
		// Меню открывает не строка поиска: запрос прошлого раза не нужен.
		v.query, v.results = "", nil
	}
	v.rev++
	v.mu.Unlock()
	v.sideW.Set(m.sidebarTarget(false))
	v.gridFade.Set(0)
}

// attachOnOpen подписывается на всё, что способно изменить открытое меню
// (тема, язык, данные потребителя), и просит фокус клавиатуры. Подписки живут
// ровно пока меню открыто: закрытому оно не нужно, а забытая подписка будила
// бы кадр.
func (m *StartMenu) attachOnOpen() {
	m.resubscribe()
	v := m.v
	v.mu.Lock()
	keep := v.keepFocus
	v.keepFocus = false
	v.mu.Unlock()
	if !keep && m.tiled() {
		focusreq.Request(m)
	}
	if m.Query() != "" {
		m.refreshResults()
	}
}

// resubscribe заново оформляет подписки открытого меню.
func (m *StartMenu) resubscribe() {
	v := m.v
	v.mu.Lock()
	old := v.unsubs
	v.unsubs = nil
	provider := v.provider
	v.mu.Unlock()
	for _, u := range old {
		u()
	}

	var subs []func()
	if m.tm != nil {
		subs = append(subs, m.tm.Subscribe(theme.ObserverFunc(func(*theme.Theme) {
			v.mu.Lock()
			v.rev++
			v.tilesRev++
			open := v.sideOpen
			v.mu.Unlock()
			v.sideW.Set(m.sidebarTarget(open))
			m.Invalidate()
		})))
	}
	id := widget.AddLanguageListener(func(string) { m.Invalidate() })
	subs = append(subs, func() { widget.RemoveLanguageListener(id) })
	subs = append(subs, m.startSource().Subscribe(func() {
		v.mu.Lock()
		v.rev++
		v.mu.Unlock()
		m.Invalidate()
	}))
	if provider != nil {
		subs = append(subs, provider.Subscribe(m.refreshResults))
	}
	v.mu.Lock()
	v.unsubs = subs
	v.mu.Unlock()
}

// detachOnClose прибирает за закрытым меню: подписки, запрос, всплывающее
// меню, перетаскивание, фокус. Зовётся из Flyout.Close, а значит и когда меню
// закрыли со стороны — Esc, клик мимо, открытие соседней панели.
func (m *StartMenu) detachOnClose() {
	v := m.v
	v.mu.Lock()
	unsubs := v.unsubs
	v.unsubs = nil
	box := v.box
	hadQuery := v.query != ""
	v.query, v.results = "", nil
	v.drag = tileDrag{}
	v.bar = barDrag{}
	v.grid, v.gridFrom = false, ""
	v.hover, v.press = "", ""
	popup := v.popup
	cm := v.capture
	v.rev++
	v.mu.Unlock()
	for _, u := range unsubs {
		u()
	}
	v.listBar.hide()
	v.tileBar.hide()
	v.gridFade.Set(0)
	if popup != nil {
		popup.Close()
	}
	if cm != nil {
		cm.ReleaseCapture()
	}
	if hadQuery && box != nil {
		box.clearSilently()
	}
	focusreq.Return(m)
}

// DismissAt закрывает меню кликом мимо — но не кликом по привязанной строке
// поиска: она часть меню, нажатие на неё лишь переносит в неё ввод.
func (m *StartMenu) DismissAt(x, y int) {
	m.v.mu.Lock()
	box := m.v.box
	m.v.mu.Unlock()
	if box != nil && m.tiled() && image.Pt(x, y).In(box.Bounds()) {
		return
	}
	m.Flyout.DismissAt(x, y)
}

// openForSearch открывает меню от строки поиска: фокус остаётся у строки.
func (m *StartMenu) openForSearch(anchor image.Rectangle) {
	if m.IsOpen() {
		return
	}
	m.v.mu.Lock()
	m.v.keepFocus = true
	m.v.mu.Unlock()
	m.Open(anchor)
}

// ─── Фокус ───────────────────────────────────────────────────────────────────

// SetFocused реализует widget.Focusable.
func (m *StartMenu) SetFocused(f bool) {
	if m.v.focused.Swap(f) != f {
		m.v.mu.Lock()
		m.v.kbd = m.v.kbd && f
		m.v.mu.Unlock()
		if m.IsOpen() {
			m.Invalidate()
		}
	}
}

// IsFocused реализует widget.Focusable.
func (m *StartMenu) IsFocused() bool { return m.v.focused.Load() }

// TabIndex исключает меню из обхода Tab движка, пока оно закрыто: невидимой
// остановки быть не должно. Открытое меню с плитками обходит области само.
func (m *StartMenu) TabIndex() int {
	if m.IsOpen() && m.tiled() {
		return 0
	}
	return -1
}

// AcceptsTab реализует widget.TabAcceptor: открытому меню Tab нужен для обхода
// трёх областей, а не для перехода фокуса движка.
func (m *StartMenu) AcceptsTab() bool { return m.IsOpen() && m.tiled() }

// SetCaptureManager реализует widget.CaptureAware.
func (m *StartMenu) SetCaptureManager(cm widget.CaptureManager) {
	m.v.mu.Lock()
	m.v.capture = cm
	m.v.mu.Unlock()
}

// WantsCapture реализует widget.CaptureRequester: нажатие на плитку захватывает
// мышь, чтобы перетаскивание не обрывалось на границе меню.
func (m *StartMenu) WantsCapture(e widget.MouseEvent) bool {
	if !m.IsOpen() || !m.tiled() || e.Button != widget.MouseLeft || !e.Pressed {
		return false
	}
	h := m.hitTest(image.Pt(e.X, e.Y))
	// Плитка — для перетаскивания, дорожка полосы — чтобы бегунок можно было
	// вести и за пределами меню.
	return (h.area == areaTiles && h.key != "") || isBarKey(h.key)
}

// ─── Попадание ───────────────────────────────────────────────────────────────

// startHit — что находится под точкой.
type startHit struct {
	area startArea
	key  string // ключ наведения; "" — пустое место области
}

// hitTest определяет область и объект под точкой. Боковая панель проверяется
// первой: развёрнутая, она лежит поверх списка.
func (m *StartMenu) hitTest(pt image.Point) startHit {
	inner := m.contentRect()
	if inner.Empty() || !pt.In(inner) {
		return startHit{}
	}
	g := m.startGeometry(inner)
	v := m.v
	v.mu.Lock()
	items := v.sidebar
	listScroll, tileScroll := v.listScroll, v.tileScroll
	v.mu.Unlock()

	if pt.In(g.sidebar) {
		burger, rects := m.sidebarRects(g, len(items))
		if pt.In(burger) {
			return startHit{areaSidebar, keySideMenu}
		}
		for i, r := range rects {
			if pt.In(r) {
				return startHit{areaSidebar, prefSide + items[i].ID}
			}
		}
		return startHit{area: areaSidebar}
	}
	if pt.In(g.list) {
		vp := m.listViewport(g)
		if !pt.In(vp) {
			return startHit{area: areaList}
		}
		if grid := m.gridOpen(); grid {
			if c, ok := m.gridCellAt(vp, pt); ok && c.active {
				return startHit{areaList, prefGrid + c.letter}
			}
			return startHit{area: areaList}
		}
		if k := m.barHit(g, areaList, pt); k != "" {
			return startHit{areaList, k}
		}
		rows, contentH := m.listRows()
		scroll := clampScroll(listScroll, contentH, vp.Dy())
		if i := firstRowAt(rows, scroll+pt.Y-vp.Min.Y); i < len(rows) && rows[i].kind.focusable() {
			return startHit{areaList, prefRow + rows[i].key}
		}
		return startHit{area: areaList}
	}
	if pt.In(g.tiles) {
		if g.cols == 0 {
			return startHit{area: areaTiles}
		}
		if k := m.barHit(g, areaTiles, pt); k != "" {
			return startHit{areaTiles, k}
		}
		l := m.layoutTiles(g)
		scroll := clampScroll(tileScroll, l.height, tilesViewHeight(g))
		origin := image.Pt(g.tinner.Min.X, g.tinner.Min.Y-scroll)
		if pt.In(g.tinner) {
			for _, t := range l.tiles {
				if pt.In(t.rect.Add(origin)) {
					return startHit{areaTiles, prefTile + string(t.id)}
				}
			}
		}
		return startHit{area: areaTiles}
	}
	return startHit{}
}

// keyRect возвращает прямоугольник объекта с ключом key (пустой, если его нет
// на экране): им перерисовывается наведение.
func (m *StartMenu) keyRect(key string) image.Rectangle {
	if key == "" {
		return image.Rectangle{}
	}
	inner := m.contentRect()
	if inner.Empty() {
		return image.Rectangle{}
	}
	g := m.startGeometry(inner)
	switch {
	case strings.HasPrefix(key, prefTile):
		return m.tileRectAbs(TileID(strings.TrimPrefix(key, prefTile)))
	case strings.HasPrefix(key, prefRow):
		rows, contentH := m.listRows()
		vp := m.listViewport(g)
		m.v.mu.Lock()
		scroll := clampScroll(m.v.listScroll, contentH, vp.Dy())
		m.v.mu.Unlock()
		if i := rowIndex(rows, strings.TrimPrefix(key, prefRow)); i >= 0 {
			return m.rowRectAbs(g, rows, i, scroll).Intersect(vp)
		}
	case isBarKey(key):
		return m.barRectAbs(key)
	case strings.HasPrefix(key, prefGrid):
		if c, ok := m.gridCellByLetter(m.listViewport(g), strings.TrimPrefix(key, prefGrid)); ok {
			return c.rect
		}
	case strings.HasPrefix(key, prefSide):
		items := m.SidebarItems()
		burger, rects := m.sidebarRects(g, len(items))
		if key == keySideMenu {
			return burger
		}
		for i, it := range items {
			if prefSide+it.ID == key {
				return rects[i]
			}
		}
	}
	return image.Rectangle{}
}

// invalidateKeys перерисовывает объекты с указанными ключами.
func (m *StartMenu) invalidateKeys(keys ...string) {
	for _, k := range keys {
		if r := m.keyRect(k); !r.Empty() {
			widget.InvalidateRect(r)
		}
	}
}

// areaRect — прямоугольник области на экране.
func (m *StartMenu) areaRect(a startArea) image.Rectangle {
	inner := m.contentRect()
	if inner.Empty() {
		return image.Rectangle{}
	}
	g := m.startGeometry(inner)
	switch a {
	case areaSidebar:
		return g.sidebar
	case areaList:
		return g.list
	case areaTiles:
		return g.tiles
	}
	return image.Rectangle{}
}

// ─── Мышь ────────────────────────────────────────────────────────────────────

// setHover меняет наведение и перерисовывает старое и новое место.
func (m *StartMenu) setHover(key string) {
	v := m.v
	v.mu.Lock()
	old := v.hover
	changed := old != key
	v.hover = key
	if changed {
		v.kbd = false
	}
	v.mu.Unlock()
	if changed {
		m.invalidateKeys(old, key)
		if isBarKey(old) || isBarKey(key) {
			m.syncBarPins()
		}
	}
}

func (m *StartMenu) mouseMoveTiled(x, y int) {
	pt := image.Pt(x, y)
	v := m.v

	v.mu.Lock()
	v.mouse, v.mouseOK = pt, pt.In(m.rect())
	dragging := v.drag.pending || v.drag.active
	barDragging := v.bar.active
	v.mu.Unlock()
	if barDragging {
		// Бегунок ведут за мышью, наведение на остальное не обновляется.
		m.barMove(pt)
		return
	}
	if dragging {
		m.dragMove(pt)
	}

	h := m.hitTest(pt)
	if h.area == areaList && m.scrollable(areaList) {
		v.listBar.poke(m.invalListBar)
	}
	if h.area == areaTiles && m.scrollable(areaTiles) {
		v.tileBar.poke(m.invalTileBar)
	}
	if dragging {
		return
	}
	m.setHover(h.key)
}

// scrollable — есть ли в области что прокручивать: полоса нужна только тогда.
func (m *StartMenu) scrollable(a startArea) bool {
	inner := m.contentRect()
	if inner.Empty() {
		return false
	}
	g := m.startGeometry(inner)
	switch a {
	case areaList:
		_, contentH := m.listRows()
		return contentH > m.listViewport(g).Dy()
	case areaTiles:
		return g.cols > 0 && m.layoutTiles(g).height > tilesViewHeight(g)
	}
	return false
}

// invalListBar и invalTileBar перерисовывают колонку полосы прокрутки — и
// только её.
func (m *StartMenu) invalListBar() { m.invalBar(areaList) }
func (m *StartMenu) invalTileBar() { m.invalBar(areaTiles) }

func (m *StartMenu) invalBar(a startArea) {
	if !m.IsOpen() {
		return
	}
	r := m.areaRect(a)
	if r.Empty() {
		return
	}
	w := m.metricInt(KeyScrollThinHoverWidth)
	if w <= 0 {
		w = 8
	}
	widget.InvalidateRect(image.Rect(r.Max.X-w-2, r.Min.Y, r.Max.X, r.Max.Y))
}

// mouseButtonTiled разбирает кнопки мыши. Второй результат — «разобрано
// здесь»; если ложь, событие отдаётся встроенной панели (она закрывает меню
// кликом мимо).
func (m *StartMenu) mouseButtonTiled(e widget.MouseEvent) (handled, ok bool) {
	if !m.IsOpen() {
		return false, false
	}
	pt := image.Pt(e.X, e.Y)
	inside := pt.In(m.rect())
	if !inside {
		// Отпускание кнопки за пределами меню всё равно заканчивает перенос
		// плитки или перетаскивание бегунка: мышь захвачена, и без этого
		// состояние осталось бы «зажатым».
		if e.Button == widget.MouseLeft && !e.Pressed && m.holding() {
			return m.leftButton(e, pt), true
		}
		return false, false
	}

	switch e.Button {
	case widget.MouseWheelUp, widget.MouseWheelDown:
		if e.Pressed {
			dy := 1.0
			if e.Button == widget.MouseWheelUp {
				dy = -1
			}
			m.scrollAt(pt, dy*float64(m.wheelStep(pt)))
		}
		return true, true
	case widget.MouseLeft:
		return m.leftButton(e, pt), true
	case widget.MouseRight:
		return m.rightButton(e, pt), true
	}
	return false, false
}

// leftButton: нажатие запоминает объект (и готовит перетаскивание плитки),
// отпускание на том же объекте — активирует его.
func (m *StartMenu) leftButton(e widget.MouseEvent, pt image.Point) bool {
	v := m.v
	h := m.hitTest(pt)
	if e.Pressed {
		m.closeGridOnPress(h, pt)
		v.mu.Lock()
		v.press = h.key
		v.kbd = false
		if h.area != areaNone {
			v.area = h.area
		}
		isBar := isBarKey(h.key)
		if h.area == areaTiles && h.key != "" && !isBar {
			v.drag = tileDrag{pending: true, id: TileID(strings.TrimPrefix(h.key, prefTile)), start: pt, pos: pt}
		}
		v.mu.Unlock()
		// Нажатие вне боковой панели сворачивает развёрнутую, как в Windows.
		if h.area != areaSidebar && m.SidebarExpanded() {
			m.SetSidebarExpanded(false)
		}
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
	drag := v.drag
	v.drag = tileDrag{}
	cm := v.capture
	v.mu.Unlock()
	if cm != nil && (drag.pending || drag.active) {
		cm.ReleaseCapture()
	}
	m.invalidateKeys(press)
	if drag.active {
		m.dropDrag(drag)
		return true
	}
	if press != "" && press == h.key {
		m.activate(press)
	}
	return press != ""
}

// rightButton: отпускание правой кнопки на объекте показывает контекстное меню.
func (m *StartMenu) rightButton(e widget.MouseEvent, pt image.Point) bool {
	if e.Pressed {
		return true
	}
	h := m.hitTest(pt)
	if h.key == "" {
		return true
	}
	if t, ok := m.targetFor(h.key); ok {
		m.showContext(t, pt)
	}
	return true
}

// wheelStep — на сколько пикселей сдвигает один щелчок колеса в области под
// точкой: три строки списка или две ячейки сетки плиток.
func (m *StartMenu) wheelStep(pt image.Point) int {
	if h := m.hitTest(pt); h.area == areaTiles {
		return 2 * (m.metricInt(KeyTileUnit) + m.metricInt(KeyTileGap))
	}
	return 3 * m.metricInt(KeyStartMenuRowHeight)
}

// OnMouseWheelPixels принимает точную дельту прокрутки (тачпад, колесо с
// пикселями).
func (m *StartMenu) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	if !m.IsOpen() || !m.tiled() || !image.Pt(x, y).In(m.rect()) {
		return false
	}
	m.scrollAt(image.Pt(x, y), dy)
	return true
}

// scrollAt прокручивает область под точкой на dy пикселей (вниз — положительно).
func (m *StartMenu) scrollAt(pt image.Point, dy float64) {
	h := m.hitTest(pt)
	switch h.area {
	case areaSidebar, areaList:
		if m.gridOpen() {
			return // сетка букв не прокручивается: колесо ничего не делает
		}
		m.scrollList(int(dy))
	case areaTiles:
		m.scrollTiles(int(dy))
	}
}

func (m *StartMenu) scrollList(d int) {
	inner := m.contentRect()
	if inner.Empty() {
		return
	}
	g := m.startGeometry(inner)
	vp := m.listViewport(g)
	_, contentH := m.listRows()
	v := m.v
	v.mu.Lock()
	next := clampScroll(v.listScroll+d, contentH, vp.Dy())
	changed := next != v.listScroll
	v.listScroll = next
	v.mu.Unlock()
	v.listBar.poke(m.invalListBar)
	if changed {
		widget.InvalidateRect(g.list)
		m.refreshHover()
	}
}

func (m *StartMenu) scrollTiles(d int) {
	inner := m.contentRect()
	if inner.Empty() {
		return
	}
	g := m.startGeometry(inner)
	if g.cols == 0 {
		return
	}
	l := m.layoutTiles(g)
	v := m.v
	v.mu.Lock()
	// Пока плитку несут, снизу есть запас: на нём отпускают плитку в новую
	// группу под последней.
	next := clampScroll(v.tileScroll+d, l.height+m.dragExtraLocked(), tilesViewHeight(g))
	changed := next != v.tileScroll
	v.tileScroll = next
	v.mu.Unlock()
	v.tileBar.poke(m.invalTileBar)
	if changed {
		widget.InvalidateRect(g.tiles)
		m.refreshHover()
	}
}

// refreshHover пересчитывает наведение после прокрутки: под неподвижным
// курсором оказались другие строки. Позиция мыши запоминается обработчиком
// движения.
func (m *StartMenu) refreshHover() {
	v := m.v
	v.mu.Lock()
	pt, ok := v.mouse, v.mouseOK
	v.mu.Unlock()
	if ok {
		m.setHover(m.hitTest(pt).key)
	}
}

// ─── Активация ───────────────────────────────────────────────────────────────

// activate выполняет действие объекта с ключом key: запуск, раскрытие папки,
// пункт боковой панели.
func (m *StartMenu) activate(key string) {
	switch {
	case key == keySideMenu:
		m.SetSidebarExpanded(!m.SidebarExpanded())
	case strings.HasPrefix(key, prefSide):
		id := strings.TrimPrefix(key, prefSide)
		keepOpen := false
		for _, it := range m.SidebarItems() {
			if it.ID == id {
				keepOpen = it.KeepOpen
			}
		}
		if m.OnSidebarActivate != nil {
			m.OnSidebarActivate(id)
		}
		if !keepOpen {
			m.Close()
		}
	case strings.HasPrefix(key, prefRow):
		rows, _ := m.listRows()
		i := rowIndex(rows, strings.TrimPrefix(key, prefRow))
		if i < 0 {
			return
		}
		switch r := rows[i]; r.kind {
		case rowApp, rowChild:
			m.launch(r.app)
		case rowFolder:
			m.toggleFolder(r.folder)
		case rowResult:
			m.activateResult(r.result)
		case rowLetter:
			m.openLetterGridFrom(r.key, r.label)
		}
	case strings.HasPrefix(key, prefGrid):
		m.JumpToLetter(strings.TrimPrefix(key, prefGrid))
	case strings.HasPrefix(key, prefTile):
		id := TileID(strings.TrimPrefix(key, prefTile))
		for _, g := range m.TileGroups() {
			for _, t := range g.Tiles {
				if t.ID != id {
					continue
				}
				if t.App != "" {
					m.launch(t.App)
				} else {
					if m.OnTileLaunch != nil {
						m.OnTileLaunch(id)
					}
					m.Close()
				}
				return
			}
		}
	}
}

// toggleFolder раскрывает или сворачивает папку на месте.
func (m *StartMenu) toggleFolder(key string) {
	v := m.v
	v.mu.Lock()
	v.folders[key] = !v.folders[key]
	v.rev++
	v.mu.Unlock()
	widget.InvalidateRect(m.areaRect(areaList))
}

// SetFolderExpanded раскрывает или сворачивает папку списка программно.
func (m *StartMenu) SetFolderExpanded(key string, open bool) {
	v := m.v
	v.mu.Lock()
	if v.folders[key] == open {
		v.mu.Unlock()
		return
	}
	v.folders[key] = open
	v.rev++
	v.mu.Unlock()
	widget.InvalidateRect(m.areaRect(areaList))
}

func (m *StartMenu) activateResult(r SearchResult) {
	p := m.SearchProvider()
	if p != nil {
		_ = p.Activate(r)
	}
	m.Close()
}

// targetFor описывает объект с ключом key для контекстного меню.
func (m *StartMenu) targetFor(key string) (StartTarget, bool) {
	switch {
	case strings.HasPrefix(key, prefSide):
		if key == keySideMenu {
			return StartTarget{}, false
		}
		return StartTarget{Kind: StartTargetSidebar, Sidebar: strings.TrimPrefix(key, prefSide)}, true
	case strings.HasPrefix(key, prefRow):
		rows, _ := m.listRows()
		if i := rowIndex(rows, strings.TrimPrefix(key, prefRow)); i >= 0 {
			switch r := rows[i]; r.kind {
			case rowApp, rowChild:
				return StartTarget{Kind: StartTargetApp, App: r.app, Folder: r.folder}, true
			case rowFolder:
				return StartTarget{Kind: StartTargetFolder, Folder: r.folder}, true
			case rowResult:
				return StartTarget{Kind: StartTargetResult, Result: r.result}, true
			}
		}
	case strings.HasPrefix(key, prefTile):
		id := TileID(strings.TrimPrefix(key, prefTile))
		for _, g := range m.TileGroups() {
			for _, t := range g.Tiles {
				if t.ID == id {
					return StartTarget{Kind: StartTargetTile, Tile: id, App: t.App}, true
				}
			}
		}
	}
	return StartTarget{}, false
}

// showContext показывает контекстное меню цели в точке at.
func (m *StartMenu) showContext(t StartTarget, at image.Point) {
	if m.ContextMenu != nil {
		if items := m.ContextMenu(t); len(items) > 0 {
			p := m.ensurePopup()
			p.SetItems(items)
			p.Show(at.X, at.Y)
		}
		return
	}
	if m.OnContextMenu != nil {
		m.OnContextMenu(t, at)
	}
}

// ensurePopup создаёт всплывающее меню при первой надобности и кладёт его в
// дочерние: клик по его пунктам тогда не считается кликом мимо «Пуска».
func (m *StartMenu) ensurePopup() *widget.PopupMenu {
	v := m.v
	v.mu.Lock()
	p := v.popup
	v.mu.Unlock()
	if p != nil {
		return p
	}
	p = widget.NewPopupMenu()
	v.mu.Lock()
	if v.popup == nil {
		v.popup = p
	} else {
		p = v.popup
	}
	v.mu.Unlock()
	m.AddChild(p)
	return p
}

// ─── Поиск ───────────────────────────────────────────────────────────────────

// SetSearchProvider задаёт поставщика результатов. Пока запрос не пуст, список
// приложений заменяется его результатами.
func (m *StartMenu) SetSearchProvider(p SearchProvider) {
	v := m.v
	v.mu.Lock()
	v.provider = p
	v.rev++
	open := m.IsOpen()
	v.mu.Unlock()
	if open {
		m.resubscribe()
		m.refreshResults()
	}
}

// SearchProvider возвращает поставщика результатов (nil — не задан).
func (m *StartMenu) SearchProvider() SearchProvider {
	m.v.mu.Lock()
	defer m.v.mu.Unlock()
	return m.v.provider
}

// Query возвращает текущий запрос.
func (m *StartMenu) Query() string {
	m.v.mu.Lock()
	defer m.v.mu.Unlock()
	return m.v.query
}

// SetQuery ставит запрос: список приложений уступает место результатам. Пустой
// запрос возвращает список. Запрос сообщается поставщику; связанная строка
// поиска на панели показывает тот же текст.
func (m *StartMenu) SetQuery(q string) {
	m.applyQuery(q)
	m.v.mu.Lock()
	box := m.v.box
	m.v.mu.Unlock()
	if box != nil {
		box.setText(q, false)
	}
}

// setQueryFromBox — запрос пришёл из строки поиска, текст в ней уже стоит.
func (m *StartMenu) setQueryFromBox(q string) { m.applyQuery(q) }

func (m *StartMenu) applyQuery(q string) {
	v := m.v
	v.mu.Lock()
	changed := v.query != q
	v.query = q
	v.listScroll = 0
	v.sel[areaList] = ""
	if q != "" {
		// У результатов поиска букв нет: сетка перехода закрывается.
		v.grid, v.gridFrom = false, ""
	}
	v.rev++
	provider := v.provider
	v.mu.Unlock()
	if q != "" {
		v.gridFade.Set(0)
	}
	if !changed {
		return
	}
	if provider != nil {
		provider.SetQuery(q)
	}
	m.refreshResults()
}

// refreshResults перечитывает результаты у поставщика.
func (m *StartMenu) refreshResults() {
	v := m.v
	v.mu.Lock()
	provider, q := v.provider, v.query
	v.mu.Unlock()
	var res []SearchResult
	if provider != nil && q != "" {
		res = provider.Results()
	}
	v.mu.Lock()
	v.results = res
	v.rev++
	v.mu.Unlock()
	if m.IsOpen() {
		widget.InvalidateRect(m.areaRect(areaList))
	}
}

// bindSearchBox запоминает привязанную строку поиска.
func (m *StartMenu) bindSearchBox(b *SearchBox) {
	m.v.mu.Lock()
	m.v.box = b
	m.v.mu.Unlock()
}

// typeToSearch переводит набранный на меню символ в запрос: в строку на панели,
// если она привязана (фокус уходит к ней), иначе в запрос самого меню.
func (m *StartMenu) typeToSearch(r rune) {
	v := m.v
	v.mu.Lock()
	box := v.box
	v.mu.Unlock()
	if box != nil {
		moveFocusTo(m, box)
		box.insertRune(r)
		return
	}
	m.SetQuery(m.Query() + string(r))
}

// SearchKey разбирает навигационные клавиши, пришедшие из строки поиска:
// Вверх, Вниз, PageUp, PageDown ходят по результатам, Enter открывает
// выбранный (первый, если выбора нет). Возвращает true, если клавиша
// использована.
func (m *StartMenu) SearchKey(e widget.KeyEvent) bool {
	if !m.IsOpen() || !m.tiled() || !e.Pressed {
		return false
	}
	m.v.mu.Lock()
	m.v.area = areaList
	m.v.kbd = true
	m.v.mu.Unlock()
	switch e.Code {
	case widget.KeyUp, widget.KeyDown, widget.KeyPageUp, widget.KeyPageDown:
		m.keyList(e)
		return true
	case widget.KeyEnter:
		if m.Query() == "" {
			return false
		}
		rows, _ := m.listRows()
		m.v.mu.Lock()
		cur := m.v.sel[areaList]
		m.v.mu.Unlock()
		if i := rowIndex(rows, strings.TrimPrefix(cur, prefRow)); cur != "" && i >= 0 {
			m.activate(cur)
			return true
		}
		for _, r := range rows {
			if r.kind == rowResult {
				m.activate(prefRow + r.key)
				return true
			}
		}
		return true
	}
	return false
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

// keyTiled разбирает клавишу. Возвращает true, если она использована; иначе
// её получит встроенная панель (Esc закрывает меню).
func (m *StartMenu) keyTiled(e widget.KeyEvent) bool {
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
	if e.Code == widget.KeyEscape && m.gridOpen() {
		m.CloseLetterGrid() // первый Esc закрывает сетку букв, второй — меню
		return true
	}
	if e.Mod&(widget.ModCtrl|widget.ModAlt|widget.ModMeta) != 0 || e.Code == widget.KeyEscape {
		return false // Esc закрывает меню: его разбирает встроенная панель
	}

	if e.Code == widget.KeyTab {
		m.CloseLetterGrid() // уходя из списка, сетка закрывается
		m.cycleArea(e.Mod&widget.ModShift != 0)
		return true
	}
	// Набор буквы — переход в поиск; пробел остаётся активацией.
	if e.Rune > ' ' && e.Rune != 127 && !unicode.IsControl(e.Rune) {
		m.typeToSearch(e.Rune)
		return true
	}
	if e.Code == widget.KeyBackspace && m.Query() != "" {
		v.mu.Lock()
		box := v.box
		v.mu.Unlock()
		if box == nil {
			q := []rune(m.Query())
			m.SetQuery(string(q[:len(q)-1]))
			return true
		}
	}

	v.mu.Lock()
	area := v.area
	v.kbd = true
	v.mu.Unlock()
	switch area {
	case areaSidebar:
		m.keySidebar(e)
	case areaTiles:
		m.keyTiles(e)
	default:
		if m.gridOpen() {
			m.keyGrid(e)
		} else {
			m.keyList(e)
		}
	}
	return true
}

// cycleArea переводит клавиатурный фокус на следующую (предыдущую) область.
// Пустые области — без плиток или без пунктов — пропускаются.
func (m *StartMenu) cycleArea(reverse bool) {
	order := []startArea{areaSidebar, areaList, areaTiles}
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
	for n := 0; n < len(order); n++ {
		idx = (idx + step) % len(order)
		if m.areaHasItems(order[idx]) {
			break
		}
	}
	next := order[idx]
	v.mu.Lock()
	v.area = next
	v.kbd = true
	sel := v.sel[next]
	v.mu.Unlock()
	if sel == "" {
		m.selectFirst(next)
	} else {
		m.setSel(next, sel)
	}
	widget.InvalidateRect(m.areaRect(areaSidebar))
	widget.InvalidateRect(m.areaRect(areaList))
	widget.InvalidateRect(m.areaRect(areaTiles))
}

// areaHasItems — есть ли в области что выбирать.
func (m *StartMenu) areaHasItems(a startArea) bool {
	switch a {
	case areaSidebar:
		return true // гамбургер есть всегда
	case areaList:
		return len(m.interactiveRows()) > 0
	case areaTiles:
		inner := m.contentRect()
		if inner.Empty() {
			return false
		}
		g := m.startGeometry(inner)
		return g.cols > 0 && len(m.layoutTiles(g).tiles) > 0
	}
	return false
}

// selectFirst выбирает первый объект области.
func (m *StartMenu) selectFirst(a startArea) {
	switch a {
	case areaSidebar:
		m.setSel(a, keySideMenu)
	case areaList:
		if rows := m.interactiveRows(); len(rows) > 0 {
			m.setSel(a, prefRow+rows[0].key)
		}
	case areaTiles:
		if keys := m.tileKeys(); len(keys) > 0 {
			m.setSel(a, keys[0])
		}
	}
}

// setSel запоминает выбор области и показывает выбранное: прокручивает так,
// чтобы объект был виден.
func (m *StartMenu) setSel(a startArea, key string) {
	v := m.v
	v.mu.Lock()
	old := v.sel[a]
	v.sel[a] = key
	v.mu.Unlock()
	m.ensureVisible(a, key)
	m.invalidateKeys(old, key)
}

// ensureVisible прокручивает область так, чтобы объект key был виден целиком.
func (m *StartMenu) ensureVisible(a startArea, key string) {
	inner := m.contentRect()
	if inner.Empty() || key == "" {
		return
	}
	g := m.startGeometry(inner)
	v := m.v
	switch a {
	case areaList:
		rows, contentH := m.listRows()
		i := rowIndex(rows, strings.TrimPrefix(key, prefRow))
		if i < 0 {
			return
		}
		vp := m.listViewport(g)
		v.mu.Lock()
		scroll := clampScroll(v.listScroll, contentH, vp.Dy())
		top, bottom := rows[i].y, rows[i].y+rows[i].h
		// Заголовок буквы над первой строкой группы тоже остаётся виден.
		if i > 0 && rows[i-1].kind == rowLetter && top-rows[i-1].h < scroll {
			top = rows[i-1].y
		}
		if top < scroll {
			scroll = top
		} else if bottom > scroll+vp.Dy() {
			scroll = bottom - vp.Dy()
		}
		scroll = clampScroll(scroll, contentH, vp.Dy())
		changed := scroll != v.listScroll
		v.listScroll = scroll
		v.mu.Unlock()
		if changed {
			v.listBar.poke(m.invalListBar)
			widget.InvalidateRect(g.list)
		}
	case areaTiles:
		if g.cols == 0 {
			return
		}
		l := m.layoutTiles(g)
		id := TileID(strings.TrimPrefix(key, prefTile))
		for _, t := range l.tiles {
			if t.id != id {
				continue
			}
			v.mu.Lock()
			scroll := clampScroll(v.tileScroll, l.height, tilesViewHeight(g))
			if t.rect.Min.Y < scroll {
				scroll = t.rect.Min.Y
			} else if t.rect.Max.Y > scroll+tilesViewHeight(g) {
				scroll = t.rect.Max.Y - tilesViewHeight(g)
			}
			scroll = clampScroll(scroll, l.height, tilesViewHeight(g))
			changed := scroll != v.tileScroll
			v.tileScroll = scroll
			v.mu.Unlock()
			if changed {
				v.tileBar.poke(m.invalTileBar)
				widget.InvalidateRect(g.tiles)
			}
			return
		}
	}
}

// focusRows возвращает строки списка, по которым ходят стрелки: приложения,
// папки и заголовки букв (Enter и Пробел на букве открывают сетку перехода).
func (m *StartMenu) focusRows() []listRow {
	rows, _ := m.listRows()
	out := make([]listRow, 0, len(rows))
	for _, r := range rows {
		if r.kind.focusable() {
			out = append(out, r)
		}
	}
	return out
}

// firstLastApp возвращает индексы первой и последней строки, которая не буква:
// Home и End, как и первый выбор, встают на приложения, а не на заголовки.
func firstLastApp(rows []listRow) (first, last int) {
	first, last = -1, -1
	for i, r := range rows {
		if r.kind == rowLetter {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	if first < 0 { // одни заголовки
		first, last = 0, len(rows)-1
	}
	return first, last
}

// interactiveRows возвращает строки списка, на которые можно встать.
func (m *StartMenu) interactiveRows() []listRow {
	rows, _ := m.listRows()
	out := make([]listRow, 0, len(rows))
	for _, r := range rows {
		if r.kind.interactive() {
			out = append(out, r)
		}
	}
	return out
}

// tileKeys возвращает ключи плиток в порядке показа.
func (m *StartMenu) tileKeys() []string {
	inner := m.contentRect()
	if inner.Empty() {
		return nil
	}
	g := m.startGeometry(inner)
	if g.cols == 0 {
		return nil
	}
	l := m.layoutTiles(g)
	keys := make([]string, len(l.tiles))
	for i, t := range l.tiles {
		keys[i] = prefTile + string(t.id)
	}
	return keys
}

// keySidebar: стрелки ходят по пунктам, Home и End — к краям, Enter и Space
// нажимают.
func (m *StartMenu) keySidebar(e widget.KeyEvent) {
	items := m.SidebarItems()
	keys := []string{keySideMenu}
	for _, it := range items {
		keys = append(keys, prefSide+it.ID)
	}
	m.v.mu.Lock()
	cur := m.v.sel[areaSidebar]
	m.v.mu.Unlock()
	idx := indexOfKey(keys, cur)
	switch e.Code {
	case widget.KeyUp, widget.KeyLeft:
		idx--
	case widget.KeyDown, widget.KeyRight:
		idx++
	case widget.KeyHome:
		idx = 0
	case widget.KeyEnd:
		idx = len(keys) - 1
	case widget.KeyEnter, widget.KeySpace:
		if idx >= 0 && !e.Repeat {
			m.activate(keys[idx])
		}
		return
	default:
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(keys) {
		idx = len(keys) - 1
	}
	m.setSel(areaSidebar, keys[idx])
}

func indexOfKey(keys []string, key string) int {
	for i, k := range keys {
		if k == key {
			return i
		}
	}
	return -1
}

// keyList: Вверх и Вниз — по строкам, PageUp и PageDown — на страницу, Home и
// End — к краям, Вправо раскрывает папку, Влево сворачивает папку (или
// переходит из её содержимого на саму папку), Enter и Space запускают.
func (m *StartMenu) keyList(e widget.KeyEvent) {
	rows := m.focusRows()
	if len(rows) == 0 {
		return
	}
	v := m.v
	v.mu.Lock()
	cur := v.sel[areaList]
	v.mu.Unlock()
	idx := -1
	for i, r := range rows {
		if prefRow+r.key == cur {
			idx = i
		}
	}
	first, last := firstLastApp(rows)
	if idx < 0 && e.Code == widget.KeyDown {
		// Первое нажатие вниз встаёт на первое приложение, а не на заголовок
		// буквы над ним.
		idx = first - 1
	}
	page := 1
	if inner := m.contentRect(); !inner.Empty() {
		if rh := m.metricInt(KeyStartMenuRowHeight); rh > 0 {
			page = m.listViewport(m.startGeometry(inner)).Dy() / rh
		}
	}
	if page < 1 {
		page = 1
	}
	switch e.Code {
	case widget.KeyDown:
		idx++
	case widget.KeyUp:
		idx--
	case widget.KeyPageDown:
		idx += page
	case widget.KeyPageUp:
		idx -= page
	case widget.KeyHome:
		idx = first
	case widget.KeyEnd:
		idx = last
	case widget.KeyRight:
		if idx >= 0 && rows[idx].kind == rowFolder && !rows[idx].expanded {
			m.toggleFolder(rows[idx].folder)
		}
		return
	case widget.KeyLeft:
		if idx < 0 {
			return
		}
		switch r := rows[idx]; {
		case r.kind == rowFolder && r.expanded:
			m.toggleFolder(r.folder)
		case r.kind == rowChild:
			for _, p := range rows {
				if p.kind == rowFolder && p.folder == r.folder {
					m.setSel(areaList, prefRow+p.key)
				}
			}
		}
		return
	case widget.KeyEnter, widget.KeySpace:
		if idx >= 0 && !e.Repeat {
			m.activate(prefRow + rows[idx].key)
		}
		return
	default:
		return
	}
	if idx < 0 {
		// Первое нажатие уже что-то выбирает, а не требует лишнего повтора.
		if e.Code == widget.KeyUp || e.Code == widget.KeyPageUp || e.Code == widget.KeyEnd {
			idx = last
		} else {
			idx = first
		}
	}
	if idx >= len(rows) {
		idx = len(rows) - 1
	}
	if idx < 0 {
		idx = 0
	}
	m.setSel(areaList, prefRow+rows[idx].key)
}

// keyTiles: стрелки ходят по геометрии сетки — к ближайшей плитке в нужную
// сторону, Home и End — к первой и последней, Enter и Space запускают.
func (m *StartMenu) keyTiles(e widget.KeyEvent) {
	inner := m.contentRect()
	if inner.Empty() {
		return
	}
	g := m.startGeometry(inner)
	if g.cols == 0 {
		return
	}
	l := m.layoutTiles(g)
	if len(l.tiles) == 0 {
		return
	}
	v := m.v
	v.mu.Lock()
	cur := v.sel[areaTiles]
	v.mu.Unlock()
	idx := -1
	for i, t := range l.tiles {
		if prefTile+string(t.id) == cur {
			idx = i
		}
	}
	switch e.Code {
	case widget.KeyEnter, widget.KeySpace:
		if idx >= 0 && !e.Repeat {
			m.activate(cur)
		}
		return
	case widget.KeyHome:
		idx = 0
	case widget.KeyEnd:
		idx = len(l.tiles) - 1
	case widget.KeyLeft, widget.KeyRight, widget.KeyUp, widget.KeyDown:
		if idx < 0 {
			idx = 0
		} else if n := nearestTile(l.tiles, idx, e.Code); n >= 0 {
			idx = n
		}
	default:
		return
	}
	m.setSel(areaTiles, prefTile+string(l.tiles[idx].id))
}

// nearestTile ищет плитку, на которую перейти от from в направлении code:
// среди лежащих в нужную сторону выбирается ближайшая по взвешенному
// расстоянию — смещение поперёк оси стоит вдвое дороже смещения вдоль, поэтому
// «вправо» остаётся в своей строке, а «вниз» — в своём столбце. -1 — идти
// некуда.
func nearestTile(tiles []tileGeom, from int, code widget.KeyCode) int {
	c := centerOf(tiles[from].rect)
	best, bestScore := -1, 0
	for i, t := range tiles {
		if i == from {
			continue
		}
		tc := centerOf(t.rect)
		dx, dy := tc.X-c.X, tc.Y-c.Y
		var along, across int
		switch code {
		case widget.KeyRight:
			if dx <= 0 {
				continue
			}
			along, across = dx, dy
		case widget.KeyLeft:
			if dx >= 0 {
				continue
			}
			along, across = -dx, dy
		case widget.KeyDown:
			if dy <= 0 {
				continue
			}
			along, across = dy, dx
		case widget.KeyUp:
			if dy >= 0 {
				continue
			}
			along, across = -dy, dx
		}
		if across < 0 {
			across = -across
		}
		score := along + 2*across
		if best < 0 || score < bestScore {
			best, bestScore = i, score
		}
	}
	return best
}

func centerOf(r image.Rectangle) image.Point {
	return image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
}

// ─── Перетаскивание плиток ───────────────────────────────────────────────────

// dragMove продвигает перетаскивание: за порогом в несколько пикселей оно
// начинается, затем плитка следует за курсором, а целевое место пересчитывается.
func (m *StartMenu) dragMove(pt image.Point) {
	v := m.v
	v.mu.Lock()
	d := v.drag
	v.mu.Unlock()
	if !d.pending && !d.active {
		return
	}
	if d.pending {
		thr := m.metricInt(KeyTileGap)
		if thr < 1 {
			thr = 1
		}
		if absInt(pt.X-d.start.X) < thr && absInt(pt.Y-d.start.Y) < thr {
			return
		}
		d.pending, d.active = false, true
	}
	d.pos = pt
	inner := m.contentRect()
	if inner.Empty() {
		return
	}
	g := m.startGeometry(inner)
	d.toGroup, d.toIndex, d.toNew = m.dropTarget(g, d.id, pt)
	v.mu.Lock()
	v.drag = d
	v.hover = ""
	v.mu.Unlock()
	// Края области прокручивают её, чтобы можно было дотянуться до дальних групп.
	if edge := m.metricInt(KeyTileUnit) / 2; edge > 0 {
		switch {
		case pt.Y < g.tinner.Min.Y+edge:
			m.scrollTiles(-edge)
		case pt.Y > g.tinner.Max.Y-edge:
			m.scrollTiles(edge)
		}
	}
	widget.InvalidateRect(g.tiles)
	widget.InvalidateRect(g.inner)
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// sameOrder — порядок плиток и групп не изменился.
func sameOrder(a, b []TileGroup) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i].Tiles) != len(b[i].Tiles) {
			return false
		}
		for j := range a[i].Tiles {
			if a[i].Tiles[j].ID != b[i].Tiles[j].ID {
				return false
			}
		}
	}
	return true
}
