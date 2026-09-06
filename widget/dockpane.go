// Package widget — dockpane.go: DockPane, панель докинга с хромом
// (титлбар + кнопки pin/float/close) для DockManager.
//
// DockPane — «инструментальное окно» в стиле Visual Studio Toolbox. Панель
// имеет титлбар с заголовком и тремя кнопками управления (release-семантика,
// как у Window) и одну область содержимого (Content, произвольный Widget).
//
// Состояния (DockPaneState):
//   - PaneDocked     — пришвартована к стороне менеджера (Left/Right/Top/Bottom);
//   - PaneAutoHidden — свёрнута в полоску-ярлык у своего края (pin отключён);
//   - PaneFloating   — плавающая поверх зоны докинга (drag/resize мышью);
//   - PaneClosed     — скрыта (можно вернуть через Show).
//
// Управление состоянием ведёт DockManager: публичные методы Pin/Unpin/Float/
// Dock/Close/Show делегируют менеджеру (если панель добавлена в менеджер).
package widget

import (
	"image"
	"image/color"
)

// DockPaneState — состояние панели докинга.
type DockPaneState int

const (
	// PaneDocked — пришвартована к стороне (side).
	PaneDocked DockPaneState = iota
	// PaneAutoHidden — свёрнута в ярлык у края (auto-hide).
	PaneAutoHidden
	// PaneFloating — плавающая поверх зоны докинга.
	PaneFloating
	// PaneClosed — скрыта.
	PaneClosed
)

// String — человекочитаемое имя состояния (для отладки/сериализации логов).
func (s DockPaneState) String() string {
	switch s {
	case PaneDocked:
		return "docked"
	case PaneAutoHidden:
		return "autohidden"
	case PaneFloating:
		return "floating"
	case PaneClosed:
		return "closed"
	}
	return "?"
}

// dockPaneBtn — «взведённая» кнопка титлбара (release-семантика).
type dockPaneBtn int

const (
	dockBtnNone dockPaneBtn = iota
	dockBtnClose
	dockBtnFloat
	dockBtnPin
)

const (
	dockPaneTitleH  = 24 // высота титлбара панели (px)
	dockPaneBtnSize = 18 // сторона кнопки титлбара (px)
	dockPaneBtnGap  = 2  // зазор между кнопками (px)
)

// DockPane — панель докинга с титлбаром и содержимым.
type DockPane struct {
	Base

	// ID — стабильный идентификатор для сериализации раскладки (SaveLayout).
	ID string
	// Title — текст заголовка в титлбаре.
	Title string

	// TitleBarHeight — высота титлбара (0 → dockPaneTitleH).
	TitleBarHeight int

	// Цвета (берутся из темы в NewDockPane; ApplyTheme обновляет).
	TitleBG       color.RGBA // фон титлбара неактивной панели
	TitleActiveBG color.RGBA // фон титлбара активной панели (Accent)
	TitleText     color.RGBA // цвет заголовка/глифов

	// TitleTextActive — цвет заголовка АКТИВНОЙ панели.
	//
	// Фонов у титлбара два (TitleBG и TitleActiveBG), а цвет текста был один,
	// и на акцентном фоне активной панели он оказывался подобран не под тот
	// фон, поверх которого нарисован. Заметно это там, где цвет текста
	// вычисляется из фона — например, инверсией.
	//
	// Нулевая альфа означает «как у неактивной»: прежнее поведение остаётся
	// у всех, кто это поле не трогал.
	TitleTextActive color.RGBA

	Background  color.RGBA // фон области содержимого
	BorderColor color.RGBA // рамка панели

	// OnStateChanged вызывается при смене состояния (docked/floating/…).
	OnStateChanged func(p *DockPane)
	// OnFloatNative — хук отрыва в нативное ОС-окно (фаза 2). Если задан,
	// при Float виджетный floating НЕ включается: менеджер вызывает колбэк и
	// оставляет панель менеджеру для отсоединения.
	OnFloatNative func(p *DockPane)
	// OnDragMove, если задан, вызывается при перетаскивании за титлбар ВМЕСТО
	// менеджерного drag&dock (по образцу Dialog.OnDragMove). Используется, когда
	// панель показана как корень собственного нативного окна ОС
	// (window.dockFloatHost): сам виджет в своём холсте неподвижен, двигается
	// нативное окно. Пока хук задан, виджетный ресайз краёв тоже отключён —
	// размером плавающего окна управляет рамка ОС.
	OnDragMove func(dx, dy int)

	content Widget
	mgr     *DockManager
	state   DockPaneState
	side    DockSide // текущая/последняя сторона (для AutoHidden и возврата)

	// active — панель является активной в своей стопке (подсветка титлбара).
	// Для floating/flyout всегда true. Выставляется менеджером в layout.
	active bool

	// floatBounds — запомненные bounds в плавающем состоянии.
	floatBounds image.Rectangle

	capMgr CaptureManager

	// Взведённая кнопка титлбара (release-семантика).
	armedBtn dockPaneBtn
	hoverBtn dockPaneBtn

	// Перетаскивание за титлбар (drag&dock ведёт менеджер).
	dragging  bool
	dragMoved bool
	grabDX    int // смещение курсора от левого края панели при захвате
	grabDY    int
	// Последние координаты мыши при нативном drag (OnDragMove-режим).
	dragLastX int
	dragLastY int

	// Ресайз плавающей панели за кромки (переиспользует winEdge из window.go).
	resizing     bool
	resizeDir    winEdge
	resizeStart  image.Rectangle
	resizeStartX int
	resizeStartY int
}

// NewDockPane создаёт панель докинга с идентификатором id, заголовком title и
// содержимым content (может быть nil). По умолчанию — состояние PaneDocked
// (реальную сторону назначает DockManager.AddPane).
func NewDockPane(id, title string, content Widget) *DockPane {
	p := &DockPane{
		ID:             id,
		Title:          title,
		TitleBarHeight: dockPaneTitleH,
		TitleBG:        win10.TitleBG,
		TitleActiveBG:  win10.Accent,
		TitleText:      win10.TitleText,
		Background:     win10.PanelBG,
		BorderColor:    win10.Border,
		content:        content,
		state:          PaneDocked,
	}
	if content != nil {
		p.Base.AddChild(content)
	}
	return p
}

// Content возвращает виджет содержимого панели (или nil).
func (p *DockPane) Content() Widget { return p.content }

// SetContent заменяет содержимое панели.
func (p *DockPane) SetContent(w Widget) {
	p.children = nil
	p.content = w
	if w != nil {
		p.Base.AddChild(w)
	}
	if cm := p.capMgr; cm != nil && w != nil {
		injectCaptureManagerTree(w, cm)
	}
	p.layoutContent()
	p.Invalidate()
}

// State возвращает текущее состояние панели.
func (p *DockPane) State() DockPaneState { return p.state }

// Side возвращает текущую/последнюю сторону докинга панели.
func (p *DockPane) Side() DockSide { return p.side }

// IsPinned сообщает, «приколота» ли панель (не в режиме auto-hide).
func (p *DockPane) IsPinned() bool { return p.state != PaneAutoHidden }

// ─── Публичные операции состояния (делегируют менеджеру) ────────────────────

// Dock пришвартовывает панель к стороне side (добавляет в её стопку).
func (p *DockPane) Dock(side DockSide) {
	if p.mgr != nil {
		p.mgr.dockPane(p, side)
		return
	}
	p.state, p.side = PaneDocked, side
}

// Float делает панель плавающей. Если задан OnFloatNative — вызывает его
// вместо включения виджетного floating.
func (p *DockPane) Float() {
	if p.mgr != nil {
		p.mgr.floatPane(p, p.floatBounds)
		return
	}
	p.state = PaneFloating
}

// Pin возвращает панель из auto-hide в пришвартованное состояние.
func (p *DockPane) Pin() {
	if p.mgr != nil {
		p.mgr.pinPane(p)
		return
	}
	p.state = PaneDocked
}

// Unpin переводит панель в режim auto-hide (ярлык у края).
func (p *DockPane) Unpin() {
	if p.mgr != nil {
		p.mgr.unpinPane(p)
		return
	}
	p.state = PaneAutoHidden
}

// Close скрывает панель (состояние PaneClosed).
func (p *DockPane) Close() {
	if p.mgr != nil {
		p.mgr.closePane(p)
		return
	}
	p.state = PaneClosed
	p.SetVisible(false)
}

// Show возвращает закрытую панель на её последнюю сторону.
func (p *DockPane) Show() {
	if p.mgr != nil {
		p.mgr.showPane(p)
		return
	}
	p.state = PaneDocked
	p.SetVisible(true)
}

// Dismiss реализует Dismissable: закрывает flyout (auto-hide выезд), если ЭТА
// панель сейчас выехала. Движок вызывает Dismiss у виджетов вне пути клика
// (dismissOutside), поэтому клик по центру/другой панели сворачивает flyout —
// как клик мимо dropdown. Для не-flyout панелей — no-op.
func (p *DockPane) Dismiss() {
	if p.mgr != nil && p.mgr.flyoutPane == p {
		p.mgr.closeFlyout()
	}
}

// ─── Геометрия / раскладка ──────────────────────────────────────────────────

func (p *DockPane) titleH() int {
	if p.TitleBarHeight > 0 {
		return p.TitleBarHeight
	}
	return dockPaneTitleH
}

// titleBarRect — прямоугольник титлбара панели.
func (p *DockPane) titleBarRect() image.Rectangle {
	b := p.bounds
	return image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+p.titleH())
}

// contentRect — область содержимого (под титлбаром, внутри рамки).
func (p *DockPane) contentRect() image.Rectangle {
	b := p.bounds
	return image.Rect(b.Min.X+1, b.Min.Y+p.titleH(), b.Max.X-1, b.Max.Y-1)
}

// buttonRects возвращает прямоугольники кнопок close/float/pin (справа налево).
func (p *DockPane) buttonRects() (closeR, floatR, pinR image.Rectangle) {
	tb := p.titleBarRect()
	if tb.Empty() {
		return
	}
	s := dockPaneBtnSize
	if s > tb.Dy()-4 {
		s = tb.Dy() - 4
	}
	y := tb.Min.Y + (tb.Dy()-s)/2
	x := tb.Max.X - 3 - s
	closeR = image.Rect(x, y, x+s, y+s)
	x -= s + dockPaneBtnGap
	floatR = image.Rect(x, y, x+s, y+s)
	x -= s + dockPaneBtnGap
	pinR = image.Rect(x, y, x+s, y+s)
	return
}

// SetBounds задаёт границы панели и раскладывает содержимое.
func (p *DockPane) SetBounds(r image.Rectangle) {
	p.Base.SetBounds(r)
	p.layoutContent()
}

// layoutContent раскладывает содержимое в contentRect (учёт HasOwnLayout).
func (p *DockPane) layoutContent() {
	if p.content == nil {
		return
	}
	cr := p.contentRect()
	old := p.content.Bounds()
	p.content.SetBounds(cr)
	if !HasOwnLayout(p.content) && !old.Empty() {
		dx := cr.Min.X - old.Min.X
		dy := cr.Min.Y - old.Min.Y
		if dx != 0 || dy != 0 {
			shiftDescendants(p.content, dx, dy)
		}
	}
}

// ─── Отрисовка ──────────────────────────────────────────────────────────────

// Draw рисует титлбар (с кнопками) и содержимое панели.
func (p *DockPane) Draw(ctx DrawContext) {
	b := p.bounds
	if b.Empty() {
		return
	}
	st := currentStyle()

	// Фон области содержимого.
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), p.Background)

	// Титлбар.
	tb := p.titleBarRect()
	tbg, ttext := p.TitleBG, p.TitleText
	if p.active {
		tbg = p.TitleActiveBG
		if p.TitleTextActive.A > 0 {
			ttext = p.TitleTextActive
		}
	}
	ctx.FillRect(tb.Min.X, tb.Min.Y, tb.Dx(), tb.Dy(), tbg)

	// Заголовок (обрезаем эллипсисом до левого края кнопок).
	_, _, pinR := p.buttonRects()
	textX := tb.Min.X + 6
	textY := tb.Min.Y + (tb.Dy()-13)/2
	title := p.Title
	if maxW := pinR.Min.X - 4 - textX; maxW <= 0 {
		title = ""
	} else {
		title = ellipsizeText(ctx, title, maxW, DefaultFontSizePt)
	}
	ctx.DrawText(title, textX, textY, ttext)

	// Кнопки титлбара.
	p.drawButtons(ctx)

	// Содержимое.
	if p.content != nil && IsWidgetVisible(p.content) {
		cr := p.contentRect()
		save := ctx.Clip()
		ctx.SetClip(cr.Intersect(save))
		p.content.Draw(ctx)
		ctx.SetClip(save)
	}

	// Рамка панели (в классике — bevel, иначе 1px).
	if st.Classic3D {
		drawBevelRaised(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
	} else {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), p.BorderColor)
	}
}

// IsActive сообщает, активна ли панель в своей стопке.
//
// Активная рисуется фоном TitleActiveBG. Наружу состояние нужно тому, кто
// вычисляет цвет заголовка из фона: без него нельзя понять, какой из двух
// фонов сейчас под текстом.
func (p *DockPane) IsActive() bool { return p.active }

// drawButtons рисует три кнопки титлбара (pin/float/close) с глифами.
func (p *DockPane) drawButtons(ctx DrawContext) {
	closeR, floatR, pinR := p.buttonRects()
	col := p.TitleText
	if p.active && p.TitleTextActive.A > 0 {
		col = p.TitleTextActive
	}

	drawBtnBG := func(r image.Rectangle, kind dockPaneBtn) {
		if p.hoverBtn == kind || p.armedBtn == kind {
			hb := color.RGBA{R: 255, G: 255, B: 255, A: 40}
			if kind == dockBtnClose && p.hoverBtn == dockBtnClose {
				hb = color.RGBA{R: 232, G: 17, B: 35, A: 255}
			}
			ctx.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), hb)
		}
	}

	// pin: вертикальная черта = «приколото», горизонтальная = auto-hide.
	drawBtnBG(pinR, dockBtnPin)
	pcx, pcy := pinR.Min.X+pinR.Dx()/2, pinR.Min.Y+pinR.Dy()/2
	if p.state == PaneAutoHidden {
		ctx.DrawHLine(pcx-4, pcy, 9, col)
		ctx.DrawHLine(pcx-4, pcy+1, 9, col)
	} else {
		ctx.DrawVLine(pcx, pcy-4, 9, col)
		ctx.DrawVLine(pcx+1, pcy-4, 9, col)
	}

	// float/dock: квадрат-рамка (окошко).
	drawBtnBG(floatR, dockBtnFloat)
	fx, fy := floatR.Min.X+floatR.Dx()/2-4, floatR.Min.Y+floatR.Dy()/2-4
	ctx.DrawBorder(fx, fy, 9, 8, col)
	ctx.DrawHLine(fx, fy+1, 9, col)

	// close: крест ✕.
	drawBtnBG(closeR, dockBtnClose)
	ccol := col
	if p.hoverBtn == dockBtnClose {
		ccol = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	ccx, ccy := closeR.Min.X+closeR.Dx()/2, closeR.Min.Y+closeR.Dy()/2
	for i := -3; i <= 3; i++ {
		ctx.SetPixel(ccx+i, ccy+i, ccol)
		ctx.SetPixel(ccx+i, ccy-i, ccol)
	}
}

// ─── Мышь / capture ─────────────────────────────────────────────────────────

// SetCaptureManager сохраняет менеджер захвата и раздаёт его содержимому.
func (p *DockPane) SetCaptureManager(cm CaptureManager) {
	p.capMgr = cm
	if p.content != nil {
		injectCaptureManagerTree(p.content, cm)
	}
}

// floatingResizeEdgeAt возвращает край(а) плавающей панели под точкой для
// ресайза (полоса winResizeBorder). edgeNone для не-floating или вне зоны.
func (p *DockPane) floatingResizeEdgeAt(x, y int) winEdge {
	if p.state != PaneFloating || p.OnDragMove != nil {
		// В нативном ОС-окне (OnDragMove задан) ресайз — задача рамки окна ОС,
		// виджетный ресайз краёв отключаем (иначе он менял бы bounds внутри
		// фиксированного холста вторичного движка).
		return edgeNone
	}
	b := p.bounds
	if !image.Pt(x, y).In(b) {
		return edgeNone
	}
	m := winResizeBorder
	var e winEdge
	if x < b.Min.X+m {
		e |= edgeW
	}
	if x >= b.Max.X-m {
		e |= edgeE
	}
	if y < b.Min.Y+m {
		e |= edgeN
	}
	if y >= b.Max.Y-m {
		e |= edgeS
	}
	return e
}

// titleDragHit — точка в перетаскиваемой зоне титлбара (титлбар минус кнопки).
// Не повторяем багу f4aff89: press ВНЕ этой зоны не начинает drag.
func (p *DockPane) titleDragHit(x, y int) bool {
	pt := image.Pt(x, y)
	if !pt.In(p.titleBarRect()) {
		return false
	}
	closeR, floatR, pinR := p.buttonRects()
	if pt.In(closeR) || pt.In(floatR) || pt.In(pinR) {
		return false
	}
	return true
}

// WantsCapture захватывает мышь при нажатии на кнопку, титлбар или кромку
// ресайза (release-семантика / drag / resize).
func (p *DockPane) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	pt := image.Pt(e.X, e.Y)
	closeR, floatR, pinR := p.buttonRects()
	if pt.In(closeR) || pt.In(floatR) || pt.In(pinR) {
		return true
	}
	if p.floatingResizeEdgeAt(e.X, e.Y) != edgeNone {
		return true
	}
	return p.titleDragHit(e.X, e.Y)
}

// Cursor возвращает resize-курсор над кромками плавающей панели.
func (p *DockPane) Cursor(x, y int) Cursor {
	switch p.floatingResizeEdgeAt(x, y) {
	case edgeN, edgeS:
		return CursorSizeNS
	case edgeW, edgeE:
		return CursorSizeWE
	case edgeN | edgeW, edgeS | edgeE:
		return CursorSizeNWSE
	case edgeN | edgeE, edgeS | edgeW:
		return CursorSizeNESW
	}
	return CursorArrow
}

// OnMouseButton обрабатывает кнопки титлбара (release-семантика), начало/конец
// drag за титлбар и ресайза плавающей панели.
func (p *DockPane) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	pt := image.Pt(e.X, e.Y)
	closeR, floatR, pinR := p.buttonRects()

	if e.Pressed {
		// Кнопки — «взводим» (колбэк на release).
		switch {
		case pt.In(closeR):
			p.armedBtn = dockBtnClose
			p.Invalidate()
			return true
		case pt.In(floatR):
			p.armedBtn = dockBtnFloat
			p.Invalidate()
			return true
		case pt.In(pinR):
			p.armedBtn = dockBtnPin
			p.Invalidate()
			return true
		}
		// Ресайз плавающей панели за кромку.
		if dir := p.floatingResizeEdgeAt(e.X, e.Y); dir != edgeNone {
			p.resizing = true
			p.resizeDir = dir
			p.resizeStart = p.bounds
			p.resizeStartX = e.X
			p.resizeStartY = e.Y
			return true
		}
		// Drag за титлбар.
		if p.titleDragHit(e.X, e.Y) {
			// Закрываем dropdown/popup ВНУТРИ содержимого панели, но НЕ её
			// собственный flyout (DismissAll(p) вызвал бы p.Dismiss()→closeFlyout,
			// панель спряталась бы ДО снимка призрака — дефект «сломанный призрак»).
			// Завершение flyout-состояния для drag'а берёт на себя beginPaneDrag.
			for _, c := range p.Children() {
				DismissAll(c)
			}
			p.dragging = true
			p.dragMoved = false
			p.grabDX = e.X - p.bounds.Min.X
			p.grabDY = e.Y - p.bounds.Min.Y
			if p.OnDragMove != nil {
				// Нативный режим: панель — корень своего ОС-окна, титлбар двигает
				// окно ОС; менеджерный drag&dock не запускаем.
				p.dragLastX, p.dragLastY = e.X, e.Y
			} else if p.mgr != nil {
				p.mgr.beginPaneDrag(p, e.X, e.Y)
			}
			return true
		}
		return false
	}

	// release
	if p.resizing {
		p.resizing = false
		p.resizeDir = edgeNone
		if p.capMgr != nil {
			p.capMgr.ReleaseCapture()
		}
		return true
	}
	if p.dragging {
		p.dragging = false
		moved := p.dragMoved
		p.dragMoved = false
		// В нативном режиме (OnDragMove) окно уже двигалось за титлбар — drag&dock
		// менеджера не завершаем (drag-возврат на направляющие — не в этой фазе).
		if p.OnDragMove == nil && p.mgr != nil {
			p.mgr.endPaneDrag(p, e.X, e.Y, moved)
		}
		if p.capMgr != nil {
			p.capMgr.ReleaseCapture()
		}
		return true
	}
	if p.armedBtn != dockBtnNone {
		armed := p.armedBtn
		p.armedBtn = dockBtnNone
		p.Invalidate()
		over := false
		switch armed {
		case dockBtnClose:
			over = pt.In(closeR)
		case dockBtnFloat:
			over = pt.In(floatR)
		case dockBtnPin:
			over = pt.In(pinR)
		}
		if p.capMgr != nil {
			p.capMgr.ReleaseCapture()
		}
		if over {
			switch armed {
			case dockBtnClose:
				p.Close()
			case dockBtnFloat:
				if p.state == PaneFloating {
					p.Dock(p.side)
				} else {
					p.Float()
				}
			case dockBtnPin:
				if p.state == PaneAutoHidden {
					p.Pin()
				} else {
					p.Unpin()
				}
			}
		}
		return true
	}
	return false
}

// OnMouseMove ведёт drag/resize и обновляет hover кнопок.
func (p *DockPane) OnMouseMove(x, y int) {
	if p.resizing {
		p.applyFloatingResize(x, y)
		return
	}
	if p.dragging {
		p.dragMoved = true
		if p.OnDragMove != nil {
			// Нативный режим: двигаем окно ОС на дельту мыши. dragLast НЕ
			// обновляем — координаты относительны окну, после его сдвига курсор
			// возвращается к точке захвата (как у Dialog.OnMouseMove).
			dx, dy := x-p.dragLastX, y-p.dragLastY
			if dx != 0 || dy != 0 {
				p.OnDragMove(dx, dy)
			}
			return
		}
		if p.mgr != nil {
			p.mgr.updatePaneDrag(p, x, y)
		}
		return
	}
	// hover кнопок.
	pt := image.Pt(x, y)
	closeR, floatR, pinR := p.buttonRects()
	hb := dockBtnNone
	switch {
	case pt.In(closeR):
		hb = dockBtnClose
	case pt.In(floatR):
		hb = dockBtnFloat
	case pt.In(pinR):
		hb = dockBtnPin
	}
	if hb != p.hoverBtn {
		p.hoverBtn = hb
		p.Invalidate()
	}
}

// applyFloatingResize пересчитывает bounds плавающей панели по стартовому
// прямоугольнику и дельте мыши (мин. размер 120×60).
func (p *DockPane) applyFloatingResize(x, y int) {
	b := p.resizeStart
	nx0, ny0, nx1, ny1 := b.Min.X, b.Min.Y, b.Max.X, b.Max.Y
	dx := x - p.resizeStartX
	dy := y - p.resizeStartY
	if p.resizeDir&edgeW != 0 {
		nx0 = b.Min.X + dx
	}
	if p.resizeDir&edgeE != 0 {
		nx1 = b.Max.X + dx
	}
	if p.resizeDir&edgeN != 0 {
		ny0 = b.Min.Y + dy
	}
	if p.resizeDir&edgeS != 0 {
		ny1 = b.Max.Y + dy
	}
	const minW, minH = 120, 60
	if nx1-nx0 < minW {
		if p.resizeDir&edgeW != 0 {
			nx0 = nx1 - minW
		} else {
			nx1 = nx0 + minW
		}
	}
	if ny1-ny0 < minH {
		if p.resizeDir&edgeN != 0 {
			ny0 = ny1 - minH
		} else {
			ny1 = ny0 + minH
		}
	}
	nb := image.Rect(nx0, ny0, nx1, ny1)
	if nb != p.bounds {
		p.floatBounds = nb
		p.SetBounds(nb)
		notifyUIChanged()
	}
}

// ─── Тема ───────────────────────────────────────────────────────────────────

// ApplyTheme перекрашивает панель из темы.
func (p *DockPane) ApplyTheme(t *Theme) {
	p.TitleBG = t.TitleBG
	p.TitleActiveBG = t.Accent
	p.TitleText = t.TitleText
	p.Background = t.PanelBG
	p.BorderColor = t.Border
}

// SetTitle задаёт текст заголовка панели.
//
// Не просто присваивание поля: заголовок панели виден ещё и на корешке вкладки,
// а ширина корешка считается по нему. Без перекладки стопки корешок сохранял бы
// ширину прежней подписи — при смене языка это разъезжающаяся полоса вкладок.
func (p *DockPane) SetTitle(s string) {
	if p.Title == s {
		return
	}
	p.Title = s
	if p.mgr != nil {
		p.mgr.layout()
	}
	p.Invalidate()
}
