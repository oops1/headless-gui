// Package widget — dockmanager.go: DockManager, корневой контейнер зоны докинга
// в стиле Visual Studio (Toolbox / инструментальные окна).
//
// DockManager раскладывает документную область (Center) и четыре стороны
// (Left/Right/Top/Bottom), к каждой из которых можно пришвартовать одну или
// несколько панелей DockPane. Несколько панелей на стороне образуют стопку
// (заголовки-табы снизу региона + активная панель сверху). Размер каждой
// стороны — поле в пикселях + ресайз перетаскиванием кромки между регионом и
// центром (механика SplitPanel: hit-зона, курсор SizeWE/NS, capture, MinSize).
//
// Порядок раскладки (как в VS): Left и Right занимают полную высоту, Top и
// Bottom — между ними; центр — остаток. Панели auto-hide сворачиваются в
// полоски-ярлыки у своих краёв (снаружи регионов), клик по ярлыку выдвигает
// панель поверх центра (flyout), уход мыши прячет её.
//
// Drag&Dock: перетаскивание панели за титлбар (capture идёт панели) рисует
// НАПРАВЛЯЮЩИЕ (dockguides.go) через overlay менеджера (OverlayDrawer БЕЗ
// OverlayBoundsProvider — оверлей остаётся в холсте). Отпускание над стрелкой —
// Dock на сторону (повторный док на ту же сторону набирает стопку); мимо —
// панель становится плавающей.
//
// ── Интеграция (для владельца canvas.go / xaml.go) ──────────────────────────
//
// canvas.go: в switch функции HasOwnLayout добавить *DockManager, т.е. строка
//
//	case *Canvas, *Grid, *DockPanel, *TabControl, *StackPanel, *Window, *WrapPanel, *UniformGrid, *GroupBox, *Expander, *SplitPanel, *DockManager:
//
// (DockManager сам перекладывает детей в SetBounds — иначе вложение в
// Canvas/Grid задвоит сдвиг). DockPane трогать в HasOwnLayout НЕ нужно: он
// живёт только внутри DockManager, который управляет его bounds напрямую.
//
// ── Формат сериализации раскладки (SaveLayout / RestoreLayout) ───────────────
//
// JSON-объект:
//
//	{
//	  "sizes": [left, top, bottom, right],      // пиксельные размеры сторон
//	  "panes": [
//	     {"id":"tools","state":0,"side":0,"active":true,"float":[x0,y0,x1,y1]},
//	     ...
//	  ]
//	}
//
// state: 0=docked 1=autohidden 2=floating 3=closed; side: 0=left 1=top 2=bottom
// 3=right (см. DockSide). Панели матчатся по id; неизвестные id игнорируются,
// панели менеджера, отсутствующие в раскладке, остаются в текущем состоянии.
package widget

import (
	"encoding/json"
	"image"
	"image/color"
	"time"
)

const (
	dockGutterSize     = 6   // толщина кромки-ресайза между регионом и центром
	dockGrabPad        = 3   // расширение hit-зоны кромки по перпендикуляру
	dockMinSideSize    = 60  // минимальный размер стороны (px)
	dockDefaultSide    = 200 // размер стороны по умолчанию
	dockStripThickness = 22  // толщина полоски-ярлыка auto-hide (px)
	dockTabStripHeight = 22  // высота полосы табов стопки (px)
	dockCenterMin      = 40  // минимальный размер центра вдоль каждой оси
	dockFlyoutMs       = 140 // длительность анимации выезда flyout (мс)

	// dockFlyoutHold — гистерезис зоны удержания flyout (px): union прямоугольников
	// ярлыка и выехавшей панели расширяется на столько во все стороны, чтобы
	// дрожание курсора на границе (и переход ярлык→панель) не схлопывал flyout.
	dockFlyoutHold = 8
	// dockGhostAlpha — числитель альфы призрака перетаскивания (из 255).
	// Снимок панели непрозрачен (premultiplied A=255); умножение всех каналов
	// на dockGhostAlpha/255 даёт premultiplied-цвет с ~70% альфой — при блите
	// Over это честное 70%-смешение снимка с фоном под призраком.
	dockGhostAlpha = 178
)

// dockTabInfo — прямоугольник таба/ярлыка и связанная панель (для hit-теста).
type dockTabInfo struct {
	rect image.Rectangle
	pane *DockPane
}

// DockManager — корневой контейнер зоны докинга.
type DockManager struct {
	Base

	// Настраиваемые параметры.
	SplitterSize   int // толщина кромки-ресайза (0 → dockGutterSize)
	MinSideSize    int // минимальный размер стороны (0 → dockMinSideSize)
	StripThickness int // толщина полоски auto-hide (0 → dockStripThickness)
	TabStripHeight int // высота полосы табов (0 → dockTabStripHeight)

	// Цвета (из темы; ApplyTheme обновляет).
	Background      color.RGBA
	GutterColor     color.RGBA
	GutterHoverBG   color.RGBA
	StripBG         color.RGBA
	TabBG           color.RGBA
	TabActiveBG     color.RGBA
	TabText         color.RGBA
	AccentColor     color.RGBA
	BorderColor     color.RGBA
	GuideFace       color.RGBA

	// OnPaneAdded, если задан, вызывается в конце AddPane после регистрации
	// панели. Используется window.dockFloatHost (фаза 2), чтобы навесить хук
	// OnFloatNative на панели, добавленные уже после EnableDockFloating.
	OnPaneAdded func(p *DockPane)

	// NativeFloating — декларация из XAML (<DockManager NativeFloating="True">):
	// панели этого менеджера разрешено отрывать в отдельные нативные окна ОС.
	// widget-пакет только хранит намерение; window.Window.Run() обходит дерево,
	// находит менеджеры с NativeFloating=true и вызывает EnableDockFloating(dm).
	// Явный вызов EnableDockFloating приложением имеет приоритет.
	NativeFloating bool

	center     Widget
	panes      []*DockPane   // мастер-реестр всех панелей (в т.ч. закрытых)
	sides      [4][]*DockPane // Docked+AutoHidden по сторонам (индекс = DockSide)
	activePane [4]*DockPane   // активная панель стопки на стороне
	sizes      [4]int         // пиксельный размер региона стороны
	floating   []*DockPane    // плавающие панели (поверх центра)

	capMgr CaptureManager

	// Кэш раскладки (заполняется layout).
	workRect   image.Rectangle
	centerRect image.Rectangle
	regions    [4]image.Rectangle
	gutters    [4]image.Rectangle
	strips     [4]image.Rectangle
	tabInfo    [4][]dockTabInfo
	stripInfo  [4][]dockTabInfo

	// Ресайз кромки.
	resizing    bool
	resizeSide  DockSide
	hoverGutter DockSide
	hoverGutOK  bool

	// Drag&Dock.
	dragPane  *DockPane
	dragX     int
	dragY     int
	dragGuide DockSide
	dragGuOK  bool

	// Призрак перетаскивания: снимок панели, взятый ОДИН раз в первом кадре
	// drag'а (когда панель ещё нарисована на своём месте в back) и кэшируемый
	// до конца drag'а. ghostImg — предумноженный (×dockGhostAlpha) снимок,
	// ghostW/ghostH — его ЛОГИЧЕСКИЙ размер (для DrawImageScaled при HiDPI).
	ghostImg  *image.RGBA
	ghostW    int
	ghostH    int
	ghostPane *DockPane

	// Auto-hide flyout. flyoutReveal ∈ [0,1] — ТОЛЬКО косметика анимации выезда
	// (Draw закрывает нераскрытую часть фоном); bounds панели ставятся сразу на
	// полный размер, поэтому логическое состояние/hit не зависят от прогресса.
	flyoutPane   *DockPane
	flyoutReveal float64
	flyoutAnim   *Animation
}

// NewDockManager создаёт пустой менеджер докинга с цветами активной темы.
func NewDockManager() *DockManager {
	m := &DockManager{
		SplitterSize:   dockGutterSize,
		MinSideSize:    dockMinSideSize,
		StripThickness: dockStripThickness,
		TabStripHeight: dockTabStripHeight,
		Background:     win10.WindowBG,
		GutterColor:    win10.SplitterBG,
		GutterHoverBG:  win10.SplitterHoverBG,
		StripBG:        win10.PanelBG,
		TabBG:          win10.TabBG,
		TabActiveBG:    win10.TabActiveBG,
		TabText:        win10.TabText,
		AccentColor:    win10.Accent,
		BorderColor:    win10.Border,
		GuideFace:      win10.PanelBG,
	}
	for i := range m.sizes {
		m.sizes[i] = dockDefaultSide
	}
	return m
}

// ─── Параметры-геттеры ──────────────────────────────────────────────────────

func (m *DockManager) gutterSize() int {
	if m.SplitterSize > 0 {
		return m.SplitterSize
	}
	return dockGutterSize
}

func (m *DockManager) minSide() int {
	if m.MinSideSize > 0 {
		return m.MinSideSize
	}
	return dockMinSideSize
}

func (m *DockManager) stripThick() int {
	if m.StripThickness > 0 {
		return m.StripThickness
	}
	return dockStripThickness
}

func (m *DockManager) tabStripH() int {
	if m.TabStripHeight > 0 {
		return m.TabStripHeight
	}
	return dockTabStripHeight
}

// validSide сообщает, является ли side одной из четырёх сторон докинга.
func validSide(s DockSide) bool { return s >= DockLeft && s <= DockRight }

// horizontalSide сообщает, что сторона отъедает место по горизонтали (Left/Right).
func horizontalSide(s DockSide) bool { return s == DockLeft || s == DockRight }

// ─── Публичный API ──────────────────────────────────────────────────────────

// SetCenter задаёт документную область (единственный виджет центра).
func (m *DockManager) SetCenter(w Widget) {
	m.center = w
	if w != nil && m.capMgr != nil {
		injectCaptureManagerTree(w, m.capMgr)
	}
	m.layout()
	m.Invalidate()
}

// Center возвращает документную область.
func (m *DockManager) Center() Widget { return m.center }

// Panes возвращает все панели менеджера (в т.ч. плавающие/закрытые).
func (m *DockManager) Panes() []*DockPane {
	out := make([]*DockPane, len(m.panes))
	copy(out, m.panes)
	return out
}

// FindPane возвращает панель по id (или nil).
func (m *DockManager) FindPane(id string) *DockPane {
	for _, p := range m.panes {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// AddPane регистрирует панель и пришвартовывает её к стороне side.
func (m *DockManager) AddPane(p *DockPane, side DockSide) {
	if p == nil {
		return
	}
	if !validSide(side) {
		side = DockLeft
	}
	p.mgr = m
	if !m.hasPane(p) {
		m.panes = append(m.panes, p)
	}
	if m.capMgr != nil {
		injectCaptureManagerTree(p, m.capMgr)
	}
	m.removeFromSides(p)
	m.removeFromFloating(p)
	p.state = PaneDocked
	p.side = side
	m.sides[int(side)] = append(m.sides[int(side)], p)
	m.activePane[int(side)] = p
	m.layout()
	m.Invalidate()
	if m.OnPaneAdded != nil {
		m.OnPaneAdded(p)
	}
}

// SideSize возвращает пиксельный размер региона стороны.
func (m *DockManager) SideSize(side DockSide) int {
	if !validSide(side) {
		return 0
	}
	return m.sizes[int(side)]
}

// SetSideSize задаёт пиксельный размер региона стороны (клэмпится раскладкой).
func (m *DockManager) SetSideSize(side DockSide, px int) {
	if !validSide(side) {
		return
	}
	m.sizes[int(side)] = m.clampSideSize(side, px)
	m.layout()
	m.Invalidate()
}

// ─── Внутренние операции состояния ──────────────────────────────────────────

func (m *DockManager) hasPane(p *DockPane) bool {
	for _, q := range m.panes {
		if q == p {
			return true
		}
	}
	return false
}

func (m *DockManager) removeFromSides(p *DockPane) {
	for s := range m.sides {
		out := m.sides[s][:0]
		for _, q := range m.sides[s] {
			if q != p {
				out = append(out, q)
			}
		}
		m.sides[s] = out
		if m.activePane[s] == p {
			m.activePane[s] = nil
		}
	}
}

func (m *DockManager) removeFromFloating(p *DockPane) {
	out := m.floating[:0]
	for _, q := range m.floating {
		if q != p {
			out = append(out, q)
		}
	}
	m.floating = out
}

// dockedPanes возвращает пришвартованные (Docked) панели стороны в порядке.
func (m *DockManager) dockedPanes(s DockSide) []*DockPane {
	var out []*DockPane
	for _, p := range m.sides[int(s)] {
		if p.state == PaneDocked {
			out = append(out, p)
		}
	}
	return out
}

// autoHiddenPanes возвращает свёрнутые (AutoHidden) панели стороны.
func (m *DockManager) autoHiddenPanes(s DockSide) []*DockPane {
	var out []*DockPane
	for _, p := range m.sides[int(s)] {
		if p.state == PaneAutoHidden {
			out = append(out, p)
		}
	}
	return out
}

func (m *DockManager) fireStateChanged(p *DockPane) {
	if p.OnStateChanged != nil {
		p.OnStateChanged(p)
	}
}

// dockPane пришвартовывает p к стороне side (набирает стопку).
func (m *DockManager) dockPane(p *DockPane, side DockSide) {
	if !validSide(side) {
		side = p.side
	}
	if !validSide(side) {
		side = DockLeft
	}
	m.removeFromSides(p)
	m.removeFromFloating(p)
	if m.flyoutPane == p {
		m.closeFlyout()
	}
	p.state = PaneDocked
	p.side = side
	m.sides[int(side)] = append(m.sides[int(side)], p)
	m.activePane[int(side)] = p
	m.layout()
	m.Invalidate()
	m.fireStateChanged(p)
}

// floatPane делает p плавающей с bounds b (пустой → по умолчанию по центру).
func (m *DockManager) floatPane(p *DockPane, b image.Rectangle) {
	if p.OnFloatNative != nil {
		// Хук отрыва в нативное окно: виджетный floating НЕ включаем.
		m.removeFromSides(p)
		m.removeFromFloating(p)
		if m.flyoutPane == p {
			m.closeFlyout()
		}
		p.state = PaneFloating
		m.layout()
		m.Invalidate()
		p.OnFloatNative(p)
		m.fireStateChanged(p)
		return
	}
	if b.Empty() {
		b = p.floatBounds
	}
	if b.Empty() {
		mb := m.bounds
		w, h := 260, 180
		cx := (mb.Min.X + mb.Max.X) / 2
		cy := (mb.Min.Y + mb.Max.Y) / 2
		b = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy+h/2)
	}
	m.removeFromSides(p)
	m.removeFromFloating(p)
	if m.flyoutPane == p {
		m.closeFlyout()
	}
	p.state = PaneFloating
	p.floatBounds = b
	m.floating = append(m.floating, p)
	m.layout()
	m.Invalidate()
	m.fireStateChanged(p)
}

// pinPane возвращает p из auto-hide в пришвартованное состояние.
func (m *DockManager) pinPane(p *DockPane) {
	if p.state != PaneAutoHidden {
		return
	}
	if m.flyoutPane == p {
		m.closeFlyout()
	}
	p.state = PaneDocked
	if !m.inSide(p, p.side) {
		if !validSide(p.side) {
			p.side = DockLeft
		}
		m.sides[int(p.side)] = append(m.sides[int(p.side)], p)
	}
	m.activePane[int(p.side)] = p
	m.layout()
	m.Invalidate()
	m.fireStateChanged(p)
}

// unpinPane переводит p в режим auto-hide (ярлык у края).
func (m *DockManager) unpinPane(p *DockPane) {
	if p.state == PaneAutoHidden {
		return
	}
	m.removeFromFloating(p)
	if !validSide(p.side) {
		p.side = DockLeft
	}
	if !m.inSide(p, p.side) {
		m.sides[int(p.side)] = append(m.sides[int(p.side)], p)
	}
	p.state = PaneAutoHidden
	if m.activePane[int(p.side)] == p {
		m.activePane[int(p.side)] = nil
	}
	m.layout()
	m.Invalidate()
	m.fireStateChanged(p)
}

// closePane скрывает p (состояние Closed).
func (m *DockManager) closePane(p *DockPane) {
	m.removeFromSides(p)
	m.removeFromFloating(p)
	if m.flyoutPane == p {
		m.closeFlyout()
	}
	p.state = PaneClosed
	p.SetVisible(false)
	m.layout()
	m.Invalidate()
	m.fireStateChanged(p)
}

// showPane возвращает закрытую панель на её последнюю сторону.
func (m *DockManager) showPane(p *DockPane) {
	if p.state != PaneClosed {
		return
	}
	if !validSide(p.side) {
		p.side = DockLeft
	}
	p.state = PaneDocked
	if !m.inSide(p, p.side) {
		m.sides[int(p.side)] = append(m.sides[int(p.side)], p)
	}
	m.activePane[int(p.side)] = p
	m.layout()
	m.Invalidate()
	m.fireStateChanged(p)
}

func (m *DockManager) inSide(p *DockPane, s DockSide) bool {
	if !validSide(s) {
		return false
	}
	for _, q := range m.sides[int(s)] {
		if q == p {
			return true
		}
	}
	return false
}

// setActive делает p активной панелью стопки на стороне side.
func (m *DockManager) setActive(side DockSide, p *DockPane) {
	if !validSide(side) || m.activePane[int(side)] == p {
		return
	}
	m.activePane[int(side)] = p
	m.layout()
	m.Invalidate()
}

// ─── Раскладка ──────────────────────────────────────────────────────────────

// SetBounds задаёт границы менеджера и перекладывает всё содержимое.
func (m *DockManager) SetBounds(r image.Rectangle) {
	m.Base.SetBounds(r)
	m.layout()
}

// clampSideSize клэмпит размер стороны в [minSide, доступное вдоль оси].
func (m *DockManager) clampSideSize(side DockSide, size int) int {
	b := m.bounds
	ext := b.Dx()
	if !horizontalSide(side) {
		ext = b.Dy()
	}
	maxS := ext - m.gutterSize() - dockCenterMin
	if maxS < m.minSide() {
		maxS = m.minSide()
	}
	if size < m.minSide() {
		size = m.minSide()
	}
	if size > maxS {
		size = maxS
	}
	if size < 0 {
		size = 0
	}
	return size
}

// layout вычисляет регионы сторон, центр, полоски auto-hide и раскладывает
// панели/центр/плавающие.
func (m *DockManager) layout() {
	b := m.bounds
	// сброс кэша
	for i := range m.regions {
		m.regions[i] = image.Rectangle{}
		m.gutters[i] = image.Rectangle{}
		m.strips[i] = image.Rectangle{}
		m.tabInfo[i] = nil
		m.stripInfo[i] = nil
	}
	if b.Empty() {
		return
	}

	// 1) Полоски auto-hide по внешним краям.
	work := b
	st := m.stripThick()
	if len(m.autoHiddenPanes(DockLeft)) > 0 {
		m.strips[int(DockLeft)] = image.Rect(work.Min.X, work.Min.Y, work.Min.X+st, work.Max.Y)
		work.Min.X += st
	}
	if len(m.autoHiddenPanes(DockRight)) > 0 {
		m.strips[int(DockRight)] = image.Rect(work.Max.X-st, work.Min.Y, work.Max.X, work.Max.Y)
		work.Max.X -= st
	}
	if len(m.autoHiddenPanes(DockTop)) > 0 {
		m.strips[int(DockTop)] = image.Rect(work.Min.X, work.Min.Y, work.Max.X, work.Min.Y+st)
		work.Min.Y += st
	}
	if len(m.autoHiddenPanes(DockBottom)) > 0 {
		m.strips[int(DockBottom)] = image.Rect(work.Min.X, work.Max.Y-st, work.Max.X, work.Max.Y)
		work.Max.Y -= st
	}
	m.workRect = work

	// 2) Регионы сторон (VS-порядок: Left/Right полной высоты, Top/Bottom между).
	g := m.gutterSize()
	x0, y0, x1, y1 := work.Min.X, work.Min.Y, work.Max.X, work.Max.Y

	// Размер стороны для РАСКЛАДКИ клэмпится под доступное место, но сохранённый
	// m.sizes НЕ переписывается — так при ресайзе менеджера вниз-вверх размеры
	// сторон восстанавливаются (менять их могут лишь SetSideSize/ресайз кромки).
	if len(m.dockedPanes(DockLeft)) > 0 {
		sz := m.clampSideSizeFor(DockLeft, x1-x0)
		m.regions[int(DockLeft)] = image.Rect(x0, y0, x0+sz, y1)
		m.gutters[int(DockLeft)] = image.Rect(x0+sz, y0, x0+sz+g, y1)
		x0 += sz + g
	}
	if len(m.dockedPanes(DockRight)) > 0 {
		sz := m.clampSideSizeFor(DockRight, x1-x0)
		m.regions[int(DockRight)] = image.Rect(x1-sz, y0, x1, y1)
		m.gutters[int(DockRight)] = image.Rect(x1-sz-g, y0, x1-sz, y1)
		x1 -= sz + g
	}
	if len(m.dockedPanes(DockTop)) > 0 {
		sz := m.clampSideSizeFor(DockTop, y1-y0)
		m.regions[int(DockTop)] = image.Rect(x0, y0, x1, y0+sz)
		m.gutters[int(DockTop)] = image.Rect(x0, y0+sz, x1, y0+sz+g)
		y0 += sz + g
	}
	if len(m.dockedPanes(DockBottom)) > 0 {
		sz := m.clampSideSizeFor(DockBottom, y1-y0)
		m.regions[int(DockBottom)] = image.Rect(x0, y1-sz, x1, y1)
		m.gutters[int(DockBottom)] = image.Rect(x0, y1-sz-g, x1, y1-sz)
		y1 -= sz + g
	}
	m.centerRect = image.Rect(x0, y0, x1, y1)

	// 3) Центр.
	if m.center != nil {
		setDockChildBounds(m.center, m.centerRect)
	}

	// 4) Панели сторон (+ табы стопки).
	for _, s := range []DockSide{DockLeft, DockRight, DockTop, DockBottom} {
		m.layoutSideRegion(s)
	}

	// 5) Ярлыки полосок auto-hide.
	for _, s := range []DockSide{DockLeft, DockRight, DockTop, DockBottom} {
		m.layoutStripLabels(s)
	}

	// 6) Плавающие панели.
	for _, p := range m.floating {
		if !p.floatBounds.Empty() {
			setDockChildBounds(p, p.floatBounds)
		}
	}

	// 7) Flyout.
	if m.flyoutPane != nil {
		setDockChildBounds(m.flyoutPane, m.flyoutRect())
	}

	// 8) Видимость/активность.
	m.applyVisibility()
}

// clampSideSizeFor клэмпит размер стороны так, чтобы вдоль оси осталось место
// под кромку и минимальный центр (avail — оставшийся размер оси до клэмпа).
func (m *DockManager) clampSideSizeFor(side DockSide, avail int) int {
	sz := m.sizes[int(side)]
	if sz < m.minSide() {
		sz = m.minSide()
	}
	maxS := avail - m.gutterSize() - dockCenterMin
	if maxS < m.minSide() {
		maxS = m.minSide()
	}
	if sz > maxS {
		sz = maxS
	}
	if sz < 0 {
		sz = 0
	}
	return sz
}

// layoutSideRegion раскладывает активную панель стороны и вычисляет табы стопки.
func (m *DockManager) layoutSideRegion(s DockSide) {
	region := m.regions[int(s)]
	if region.Empty() {
		return
	}
	docked := m.dockedPanes(s)
	if len(docked) == 0 {
		return
	}
	// Гарантируем корректную активную панель.
	active := m.activePane[int(s)]
	if active == nil || active.state != PaneDocked || !m.inSide(active, s) {
		active = docked[0]
		m.activePane[int(s)] = active
	}

	if len(docked) == 1 {
		setDockChildBounds(active, region)
		return
	}

	// Стопка: активная панель сверху, полоса табов снизу региона.
	tsh := m.tabStripH()
	content := image.Rect(region.Min.X, region.Min.Y, region.Max.X, region.Max.Y-tsh)
	setDockChildBounds(active, content)

	// Табы: слева направо по нижней полосе.
	// Ширина не измеряется здесь (нет DrawContext) — используем равные слоты,
	// уточняясь по MeasureUIText для читаемости заголовков.
	var infos []dockTabInfo
	x := region.Min.X
	tabY0 := region.Max.Y - tsh
	for _, p := range docked {
		w := MeasureUIText(p.Title, DefaultFontSizePt) + 16
		if w < 40 {
			w = 40
		}
		if x+w > region.Max.X {
			w = region.Max.X - x
		}
		if w <= 0 {
			break
		}
		infos = append(infos, dockTabInfo{
			rect: image.Rect(x, tabY0, x+w, region.Max.Y),
			pane: p,
		})
		x += w
	}
	m.tabInfo[int(s)] = infos
}

// layoutStripLabels вычисляет прямоугольники ярлыков auto-hide на полоске.
func (m *DockManager) layoutStripLabels(s DockSide) {
	strip := m.strips[int(s)]
	if strip.Empty() {
		return
	}
	panes := m.autoHiddenPanes(s)
	if len(panes) == 0 {
		return
	}
	var infos []dockTabInfo
	if horizontalSide(s) {
		// Вертикальная полоса: ярлыки стопкой сверху вниз, высота ≈ длине текста.
		y := strip.Min.Y + 2
		for _, p := range panes {
			h := MeasureUIText(p.Title, DefaultFontSizePt) + 16
			if h < 24 {
				h = 24
			}
			if y+h > strip.Max.Y {
				h = strip.Max.Y - y
			}
			if h <= 0 {
				break
			}
			infos = append(infos, dockTabInfo{
				rect: image.Rect(strip.Min.X+1, y, strip.Max.X-1, y+h),
				pane: p,
			})
			y += h + 4
		}
	} else {
		// Горизонтальная полоса: ярлыки слева направо.
		x := strip.Min.X + 2
		for _, p := range panes {
			w := MeasureUIText(p.Title, DefaultFontSizePt) + 16
			if w < 40 {
				w = 40
			}
			if x+w > strip.Max.X {
				w = strip.Max.X - x
			}
			if w <= 0 {
				break
			}
			infos = append(infos, dockTabInfo{
				rect: image.Rect(x, strip.Min.Y+1, x+w, strip.Max.Y-1),
				pane: p,
			})
			x += w + 4
		}
	}
	m.stripInfo[int(s)] = infos
}

// applyVisibility выставляет видимость/активность панелей по их состоянию.
func (m *DockManager) applyVisibility() {
	for _, p := range m.panes {
		vis, active := false, false
		switch p.state {
		case PaneDocked:
			vis = m.activePane[int(p.side)] == p && m.inSide(p, p.side)
			active = vis
		case PaneAutoHidden:
			vis = m.flyoutPane == p
			active = vis
		case PaneFloating:
			vis = true
			active = true
		case PaneClosed:
			vis = false
		}
		p.active = active
		p.SetVisible(vis)
	}
}

// flyoutRect возвращает ПОЛНЫЙ прямоугольник выехавшей панели auto-hide.
// Прогресс анимации на bounds не влияет (см. flyoutReveal) — панель сразу
// доступна для ввода на полном размере.
func (m *DockManager) flyoutRect() image.Rectangle {
	p := m.flyoutPane
	if p == nil {
		return image.Rectangle{}
	}
	s := p.side
	size := m.sizes[int(s)]
	if size <= 0 {
		size = dockDefaultSide
	}
	strip := m.strips[int(s)]
	wr := m.workRect
	switch s {
	case DockLeft:
		x0 := strip.Max.X
		return image.Rect(x0, wr.Min.Y, x0+size, wr.Max.Y)
	case DockRight:
		x1 := strip.Min.X
		return image.Rect(x1-size, wr.Min.Y, x1, wr.Max.Y)
	case DockTop:
		y0 := strip.Max.Y
		return image.Rect(wr.Min.X, y0, wr.Max.X, y0+size)
	case DockBottom:
		y1 := strip.Min.Y
		return image.Rect(wr.Min.X, y1-size, wr.Max.X, y1)
	}
	return image.Rectangle{}
}

// ─── Flyout (auto-hide выезд) ───────────────────────────────────────────────

func (m *DockManager) toggleFlyout(p *DockPane) {
	if m.flyoutPane == p {
		m.closeFlyout()
		return
	}
	m.openFlyout(p)
}

func (m *DockManager) openFlyout(p *DockPane) {
	if m.flyoutAnim != nil {
		m.flyoutAnim.Stop()
		m.flyoutAnim = nil
	}
	m.flyoutPane = p
	m.layout() // bounds панели — сразу полный размер (логически открыта)
	if currentStyle().Classic3D {
		m.flyoutReveal = 1
		m.Invalidate()
		return
	}
	// Косметический выезд: анимируем только долю «раскрытия» для Draw-обрезки.
	m.flyoutReveal = 0
	m.flyoutAnim = AnimateOwned(m, "flyout", dockFlyoutMs*time.Millisecond, EaseOutCubic, func(t float64) {
		m.flyoutReveal = t
		notifyUIChanged()
	})
}

func (m *DockManager) closeFlyout() {
	if m.flyoutAnim != nil {
		m.flyoutAnim.Stop()
		m.flyoutAnim = nil
	}
	if m.flyoutPane == nil {
		return
	}
	m.flyoutPane = nil
	m.flyoutReveal = 0
	m.layout()
	m.Invalidate()
}

// ─── Drag&Dock (вызывается из DockPane) ─────────────────────────────────────

func (m *DockManager) beginPaneDrag(p *DockPane, x, y int) {
	m.dragPane = p
	m.dragX, m.dragY = x, y
	m.dragGuide, m.dragGuOK = dockGuideHit(m.bounds, x, y)
	// Снимок берётся лениво в первом кадре DrawOverlay (нужен DrawContext);
	// сбрасываем кэш, чтобы новый drag не переиспользовал старый призрак.
	m.ghostImg = nil
	m.ghostPane = nil
	notifyUIChanged()
}

func (m *DockManager) updatePaneDrag(p *DockPane, x, y int) {
	if m.dragPane != p {
		return
	}
	m.dragX, m.dragY = x, y
	m.dragGuide, m.dragGuOK = dockGuideHit(m.bounds, x, y)
	// Плавающую панель ведём вживую под курсором (призрак не нужен).
	if p.state == PaneFloating {
		b := p.floatBounds
		if b.Empty() {
			b = p.bounds
		}
		nb := image.Rect(x-p.grabDX, y-p.grabDY, x-p.grabDX+b.Dx(), y-p.grabDY+b.Dy())
		p.floatBounds = nb
		setDockChildBounds(p, nb)
	}
	notifyUIChanged()
}

func (m *DockManager) endPaneDrag(p *DockPane, x, y int, moved bool) {
	m.dragPane = nil
	m.ghostImg = nil
	m.ghostPane = nil
	side, ok := dockGuideHit(m.bounds, x, y)
	m.dragGuOK = false
	if ok {
		m.dockPane(p, side)
		return
	}
	if moved {
		w, h := 260, 180
		if !p.floatBounds.Empty() {
			w, h = p.floatBounds.Dx(), p.floatBounds.Dy()
		} else if !p.bounds.Empty() {
			w, h = p.bounds.Dx(), p.bounds.Dy()
		}
		nb := image.Rect(x-p.grabDX, y-p.grabDY, x-p.grabDX+w, y-p.grabDY+h)
		m.floatPane(p, nb)
		return
	}
	// Без движения — просто перерисуем (снятие оверлея).
	m.Invalidate()
}

// ─── Ресайз кромок / hit-тесты ──────────────────────────────────────────────

// gutterGrabRect расширяет кромку по перпендикуляру для удобного захвата.
func (m *DockManager) gutterGrabRect(s DockSide) image.Rectangle {
	g := m.gutters[int(s)]
	if g.Empty() {
		return g
	}
	if horizontalSide(s) {
		return image.Rect(g.Min.X-dockGrabPad, g.Min.Y, g.Max.X+dockGrabPad, g.Max.Y)
	}
	return image.Rect(g.Min.X, g.Min.Y-dockGrabPad, g.Max.X, g.Max.Y+dockGrabPad)
}

func (m *DockManager) gutterAt(x, y int) (DockSide, bool) {
	pt := image.Pt(x, y)
	for _, s := range []DockSide{DockLeft, DockRight, DockTop, DockBottom} {
		if !m.gutters[int(s)].Empty() && pt.In(m.gutterGrabRect(s)) {
			return s, true
		}
	}
	return DockLeft, false
}

func (m *DockManager) stripLabelAt(x, y int) *DockPane {
	pt := image.Pt(x, y)
	for _, s := range []DockSide{DockLeft, DockRight, DockTop, DockBottom} {
		for _, info := range m.stripInfo[int(s)] {
			if pt.In(info.rect) {
				return info.pane
			}
		}
	}
	return nil
}

func (m *DockManager) tabAt(x, y int) (DockSide, *DockPane) {
	pt := image.Pt(x, y)
	for _, s := range []DockSide{DockLeft, DockRight, DockTop, DockBottom} {
		for _, info := range m.tabInfo[int(s)] {
			if pt.In(info.rect) {
				return s, info.pane
			}
		}
	}
	return DockLeft, nil
}

// ─── CaptureAware ───────────────────────────────────────────────────────────

// SetCaptureManager сохраняет менеджер захвата и раздаёт его центру и панелям.
func (m *DockManager) SetCaptureManager(cm CaptureManager) {
	m.capMgr = cm
	if m.center != nil {
		injectCaptureManagerTree(m.center, cm)
	}
	for _, p := range m.panes {
		injectCaptureManagerTree(p, cm)
	}
}

// WantsCapture захватывает мышь при нажатии на кромку-ресайз.
func (m *DockManager) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	_, ok := m.gutterAt(e.X, e.Y)
	return ok
}

// Cursor возвращает resize-курсор над кромками.
func (m *DockManager) Cursor(x, y int) Cursor {
	if s, ok := m.gutterAt(x, y); ok {
		if horizontalSide(s) {
			return CursorSizeWE
		}
		return CursorSizeNS
	}
	return CursorArrow
}

// OnMouseButton обрабатывает ресайз кромок, клики по табам и ярлыкам auto-hide.
func (m *DockManager) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	if e.Pressed {
		// Кромка-ресайз (сюда доходит по capture, WantsCapture уже вернул true).
		if s, ok := m.gutterAt(e.X, e.Y); ok {
			m.resizing = true
			m.resizeSide = s
			return true
		}
		// Ярлык auto-hide → выезд/сворачивание flyout.
		if p := m.stripLabelAt(e.X, e.Y); p != nil {
			m.toggleFlyout(p)
			return true
		}
		// Таб стопки → смена активной панели.
		if s, p := m.tabAt(e.X, e.Y); p != nil {
			m.setActive(s, p)
			return true
		}
		return false
	}
	// release
	if m.resizing {
		m.resizing = false
		if m.capMgr != nil {
			m.capMgr.ReleaseCapture()
		}
		return true
	}
	return false
}

// OnMouseMove ведёт ресайз кромки, hover и прячет flyout при уходе мыши.
func (m *DockManager) OnMouseMove(x, y int) {
	if m.resizing {
		m.applyGutterResize(x, y)
		return
	}
	// hover кромки (для перерисовки).
	s, ok := m.gutterAt(x, y)
	if ok != m.hoverGutOK || s != m.hoverGutter {
		m.hoverGutter, m.hoverGutOK = s, ok
		m.Invalidate()
	}
	// Прячем flyout при уходе мыши за пределы зоны удержания (union ярлыка и
	// выехавшей панели, расширенный на гистерезис). Пока курсор в этой зоне —
	// flyout остаётся открытым, чтобы можно было дойти до содержимого и кнопки
	// pin (📌), не схлопнув панель по дороге от ярлыка.
	if m.flyoutPane != nil && !m.pointInFlyoutHold(x, y) {
		m.closeFlyout()
	}
}

// pointInFlyoutHold сообщает, находится ли точка (x, y) в зоне удержания flyout:
// объединение прямоугольников выехавшей панели и её ярлыка, каждый расширен на
// dockFlyoutHold во все стороны (гистерезис против дрожания на границе и провала
// в зазор ярлык→панель).
func (m *DockManager) pointInFlyoutHold(x, y int) bool {
	p := m.flyoutPane
	if p == nil {
		return false
	}
	pt := image.Pt(x, y)
	if fr := m.flyoutRect(); !fr.Empty() && pt.In(fr.Inset(-dockFlyoutHold)) {
		return true
	}
	for _, info := range m.stripInfo[int(p.side)] {
		if info.pane == p && pt.In(info.rect.Inset(-dockFlyoutHold)) {
			return true
		}
	}
	return false
}

// applyGutterResize пересчитывает размер стороны по позиции курсора.
func (m *DockManager) applyGutterResize(x, y int) {
	s := m.resizeSide
	region := m.regions[int(s)]
	if region.Empty() {
		return
	}
	var sz int
	switch s {
	case DockLeft:
		sz = x - region.Min.X
	case DockRight:
		sz = region.Max.X - x
	case DockTop:
		sz = y - region.Min.Y
	case DockBottom:
		sz = region.Max.Y - y
	}
	sz = m.clampSideSize(s, sz)
	if sz != m.sizes[int(s)] {
		m.sizes[int(s)] = sz
		m.layout()
		notifyUIChanged()
	}
}

// ─── Children / Draw ────────────────────────────────────────────────────────

// Children возвращает центр и панели (плавающие/flyout последними — поверх
// остальных по Z). Скрытые панели (не активные в стопке, closed) остаются в
// списке, но пропускаются движком по IsWidgetVisible — так тема и capture
// доходят до всех панелей.
func (m *DockManager) Children() []Widget {
	out := make([]Widget, 0, len(m.panes)+2)
	if m.center != nil {
		out = append(out, m.center)
	}
	for _, p := range m.panes {
		if p.state == PaneFloating || p == m.flyoutPane {
			continue
		}
		out = append(out, p)
	}
	for _, p := range m.floating {
		out = append(out, p)
	}
	if m.flyoutPane != nil {
		out = append(out, m.flyoutPane)
	}
	return out
}

// AddChild не поддерживается напрямую — используйте SetCenter/AddPane.
func (m *DockManager) AddChild(w Widget) {
	if m.center == nil {
		m.SetCenter(w)
	}
}

// Draw рисует фон, хром (кромки, полоски, табы) и детей.
func (m *DockManager) Draw(ctx DrawContext) {
	b := m.bounds
	if b.Empty() {
		return
	}
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), m.Background)

	// Хром: полоски auto-hide, кромки, табы стопок — под панелями.
	m.drawStrips(ctx)
	m.drawGutters(ctx)
	m.drawTabStrips(ctx)

	// Дети (центр + панели; плавающие/flyout поверх). ВАЖНО: обход через
	// переопределённый Children() — панели хранятся в m.panes/m.center, а не
	// в Base.children, и Base.drawChildren их не видит (рисовалось бы пусто).
	for _, child := range m.Children() {
		if child.Bounds().Empty() || !IsWidgetVisible(child) {
			continue
		}
		child.Draw(ctx)
	}

	// Косметический выезд flyout: закрываем ещё не «раскрытую» часть фоном,
	// создавая эффект выезда от края (bounds панели уже полные — hit не страдает).
	if m.flyoutPane != nil && m.flyoutReveal < 1 {
		m.drawFlyoutCover(ctx)
	}
}

// drawFlyoutCover заливает фоном нераскрытую (по анимации) часть flyout-панели.
func (m *DockManager) drawFlyoutCover(ctx DrawContext) {
	fr := m.flyoutRect()
	if fr.Empty() {
		return
	}
	rev := m.flyoutReveal
	if rev < 0 {
		rev = 0
	}
	switch m.flyoutPane.side {
	case DockLeft:
		hid := int(float64(fr.Dx()) * (1 - rev))
		ctx.FillRect(fr.Max.X-hid, fr.Min.Y, hid, fr.Dy(), m.Background)
	case DockRight:
		hid := int(float64(fr.Dx()) * (1 - rev))
		ctx.FillRect(fr.Min.X, fr.Min.Y, hid, fr.Dy(), m.Background)
	case DockTop:
		hid := int(float64(fr.Dy()) * (1 - rev))
		ctx.FillRect(fr.Min.X, fr.Max.Y-hid, fr.Dx(), hid, m.Background)
	case DockBottom:
		hid := int(float64(fr.Dy()) * (1 - rev))
		ctx.FillRect(fr.Min.X, fr.Min.Y, fr.Dx(), hid, m.Background)
	}
}

func (m *DockManager) drawGutters(ctx DrawContext) {
	for _, s := range []DockSide{DockLeft, DockRight, DockTop, DockBottom} {
		g := m.gutters[int(s)]
		if g.Empty() {
			continue
		}
		col := m.GutterColor
		if m.hoverGutOK && m.hoverGutter == s || m.resizing && m.resizeSide == s {
			col = m.GutterHoverBG
		}
		ctx.FillRect(g.Min.X, g.Min.Y, g.Dx(), g.Dy(), col)
	}
}

func (m *DockManager) drawStrips(ctx DrawContext) {
	for _, s := range []DockSide{DockLeft, DockRight, DockTop, DockBottom} {
		strip := m.strips[int(s)]
		if strip.Empty() {
			continue
		}
		ctx.FillRect(strip.Min.X, strip.Min.Y, strip.Dx(), strip.Dy(), m.StripBG)
		for _, info := range m.stripInfo[int(s)] {
			r := info.rect
			ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), m.TabBG)
			ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), m.BorderColor)
			save := ctx.Clip()
			ctx.SetClip(r.Intersect(save))
			ctx.DrawText(info.pane.Title, r.Min.X+3, r.Min.Y+(min2(r.Dy(), 18)-13)/2+2, m.TabText)
			ctx.SetClip(save)
		}
	}
}

func (m *DockManager) drawTabStrips(ctx DrawContext) {
	for _, s := range []DockSide{DockLeft, DockRight, DockTop, DockBottom} {
		infos := m.tabInfo[int(s)]
		if len(infos) == 0 {
			continue
		}
		active := m.activePane[int(s)]
		for _, info := range infos {
			r := info.rect
			bg := m.TabBG
			txt := m.TabText
			if info.pane == active {
				bg = m.TabActiveBG
				txt = win10.TabActiveText
			}
			ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), bg)
			ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), m.BorderColor)
			save := ctx.Clip()
			ctx.SetClip(r.Intersect(save))
			ctx.DrawText(info.pane.Title, r.Min.X+6, r.Min.Y+(r.Dy()-13)/2, txt)
			ctx.SetClip(save)
		}
	}
}

// ─── OverlayDrawer (направляющие + призрак при drag) ────────────────────────

// HasOverlay реализует OverlayDrawer — true во время перетаскивания панели.
func (m *DockManager) HasOverlay() bool { return m.dragPane != nil }

// DrawOverlay рисует направляющие докинга и призрак перетаскиваемой панели.
// НЕ реализуем OverlayBoundsProvider: оверлей остаётся в холсте (как локаль-меню
// Window), а не выносится в нативный попап-хост.
func (m *DockManager) DrawOverlay(ctx DrawContext) {
	if m.dragPane == nil {
		return
	}
	side, ok := dockGuideHit(m.bounds, m.dragX, m.dragY)
	prevSize := 0
	if ok {
		prevSize = m.sizes[int(side)]
	}
	// Призрак: полупрозрачный СНИМОК панели у курсора, как в Visual Studio
	// (для docked-источника — панель на месте; floating ведём вживую без призрака).
	p := m.dragPane
	if p.state != PaneFloating {
		// Снимок делаем один раз в первом кадре drag'а: DrawOverlay идёт ПОСЛЕ
		// Draw, поэтому панель гарантированно нарисована в back на своём месте.
		if m.ghostImg == nil || m.ghostPane != p {
			m.captureGhost(ctx, p)
		}
		gx, gy := m.dragX-p.grabDX, m.dragY-p.grabDY
		if m.ghostImg != nil {
			// DrawImageScaled с ЛОГИЧЕСКИМ размером: снимок физический, при HiDPI
			// он корректно растягивается обратно на scale. Снимок предумножен
			// (×dockGhostAlpha) → Over даёт честную полупрозрачность.
			ctx.DrawImageScaled(m.ghostImg, gx, gy, m.ghostW, m.ghostH)
			ctx.DrawBorder(gx, gy, m.ghostW, m.ghostH, m.AccentColor)
		} else {
			// Фолбэк: DrawContext без Snapshotter (теоретически) — прежний
			// прямоугольник-призрак.
			w, h := 200, 140
			if !p.bounds.Empty() {
				w, h = p.bounds.Dx(), p.bounds.Dy()
			}
			ghost := color.RGBA{R: m.AccentColor.R, G: m.AccentColor.G, B: m.AccentColor.B, A: 60}
			ctx.FillRectAlpha(gx, gy, w, h, ghost)
			ctx.DrawBorder(gx, gy, w, h, m.AccentColor)
		}
	}
	drawDockGuides(ctx, m.bounds, side, ok, prevSize, m.AccentColor, m.GuideFace, m.BorderColor)
}

// captureGhost делает снимок области панели p и кэширует полупрозрачный призрак
// для отрисовки во время drag'а. Требует, чтобы ctx реализовывал Snapshotter
// (engine.Canvas); иначе ghostImg остаётся nil и DrawOverlay откатывается на
// прямоугольник-призрак.
func (m *DockManager) captureGhost(ctx DrawContext, p *DockPane) {
	sn, ok := ctx.(Snapshotter)
	if !ok {
		return
	}
	b := p.bounds
	if b.Empty() {
		return
	}
	img := sn.Snapshot(b)
	if img == nil {
		return
	}
	// Честная полупрозрачность: back непрозрачен (premultiplied A=255).
	// Умножаем ВСЕ каналы на dockGhostAlpha/255 → premultiplied-цвет с ~70%
	// альфой; при блите Over это настоящее 70%-смешение с фоном под призраком.
	pix := img.Pix
	for i := range pix {
		pix[i] = uint8(uint32(pix[i]) * dockGhostAlpha / 255)
	}
	m.ghostImg = img
	m.ghostW = b.Dx()
	m.ghostH = b.Dy()
	m.ghostPane = p
}

// ─── Тема ───────────────────────────────────────────────────────────────────

// ApplyTheme перекрашивает менеджер и (через общий обход) панели/центр.
func (m *DockManager) ApplyTheme(t *Theme) {
	m.Background = t.WindowBG
	m.GutterColor = t.SplitterBG
	m.GutterHoverBG = t.SplitterHoverBG
	m.StripBG = t.PanelBG
	m.TabBG = t.TabBG
	m.TabActiveBG = t.TabActiveBG
	m.TabText = t.TabText
	m.AccentColor = t.Accent
	m.BorderColor = t.Border
	m.GuideFace = t.PanelBG
}

// ─── Сериализация раскладки ─────────────────────────────────────────────────

type paneLayoutJSON struct {
	ID     string `json:"id"`
	State  int    `json:"state"`
	Side   int    `json:"side"`
	Active bool   `json:"active"`
	Float  [4]int `json:"float"`
}

type dockLayoutJSON struct {
	Sizes [4]int           `json:"sizes"`
	Panes []paneLayoutJSON `json:"panes"`
}

// SaveLayout сериализует текущую раскладку в JSON (см. формат в шапке файла).
func (m *DockManager) SaveLayout() []byte {
	var dl dockLayoutJSON
	dl.Sizes = m.sizes
	for _, p := range m.panes {
		fb := p.floatBounds
		info := paneLayoutJSON{
			ID:     p.ID,
			State:  int(p.state),
			Side:   int(p.side),
			Active: validSide(p.side) && m.activePane[int(p.side)] == p,
			Float:  [4]int{fb.Min.X, fb.Min.Y, fb.Max.X, fb.Max.Y},
		}
		dl.Panes = append(dl.Panes, info)
	}
	data, _ := json.Marshal(dl)
	return data
}

// RestoreLayout восстанавливает раскладку из JSON. Панели матчатся по id;
// неизвестные id игнорируются, панели менеджера, отсутствующие в раскладке,
// сохраняют текущее состояние.
func (m *DockManager) RestoreLayout(data []byte) error {
	var dl dockLayoutJSON
	if err := json.Unmarshal(data, &dl); err != nil {
		return err
	}
	// Сбрасываем принадлежность к сторонам/плавающим; реконструируем из данных.
	for i := range m.sides {
		m.sides[i] = nil
		m.activePane[i] = nil
	}
	m.floating = nil
	if m.flyoutAnim != nil {
		m.flyoutAnim.Stop()
		m.flyoutAnim = nil
	}
	m.flyoutPane = nil
	m.flyoutReveal = 0
	m.sizes = dl.Sizes
	for i := range m.sizes {
		if m.sizes[i] <= 0 {
			m.sizes[i] = dockDefaultSide
		}
	}

	byID := make(map[string]paneLayoutJSON, len(dl.Panes))
	order := make([]string, 0, len(dl.Panes))
	for _, pj := range dl.Panes {
		byID[pj.ID] = pj
		order = append(order, pj.ID)
	}

	// Восстанавливаем панели в порядке раскладки (важно для порядка в стопке).
	for _, id := range order {
		pj := byID[id]
		p := m.FindPane(id)
		if p == nil {
			continue // неизвестный id — игнорируем
		}
		side := DockSide(pj.Side)
		if !validSide(side) {
			side = DockLeft
		}
		p.side = side
		p.floatBounds = image.Rect(pj.Float[0], pj.Float[1], pj.Float[2], pj.Float[3])
		st := DockPaneState(pj.State)
		p.state = st
		switch st {
		case PaneDocked:
			m.sides[int(side)] = append(m.sides[int(side)], p)
			if pj.Active || m.activePane[int(side)] == nil {
				m.activePane[int(side)] = p
			}
			p.SetVisible(true)
		case PaneAutoHidden:
			m.sides[int(side)] = append(m.sides[int(side)], p)
			p.SetVisible(false)
		case PaneFloating:
			m.floating = append(m.floating, p)
			p.SetVisible(true)
		case PaneClosed:
			p.SetVisible(false)
		}
	}

	m.layout()
	m.Invalidate()
	return nil
}

// ─── Вспомогательные ────────────────────────────────────────────────────────

// setDockChildBounds задаёт bounds ребёнку и сдвигает потомков, если у ребёнка
// нет собственной раскладки (как в Canvas.layoutChild / TabControl). DockPane
// сам раскладывает содержимое в SetBounds — для него повторный сдвиг не нужен.
func setDockChildBounds(w Widget, r image.Rectangle) {
	old := w.Bounds()
	w.SetBounds(r)
	if _, ok := w.(*DockPane); ok {
		return
	}
	if !HasOwnLayout(w) && !old.Empty() {
		dx := r.Min.X - old.Min.X
		dy := r.Min.Y - old.Min.Y
		if dx != 0 || dy != 0 {
			shiftDescendants(w, dx, dy)
		}
	}
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
