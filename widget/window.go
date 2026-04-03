// Package widget — виджет Window: корневой элемент для режима нативного окна.
//
// Window отличается от Canvas/Panel архитектурно:
//   - Canvas — виртуальный рабочий стол (headless/RDP): единый буфер, внутри Panel'ы-окна
//   - Window — одно нативное окно ОС: собственный chrome (заголовок, рамка, кнопки управления)
//
// В XAML элемент <Window> нельзя использовать одновременно с <Canvas> как корневой:
// это два взаимоисключающих режима.
//
// WPF-совместимые атрибуты:
//
//	<Window Title="Настройки" Width="800" Height="600"
//	        WindowStyle="SingleBorderWindow" TitleStyle="Win"
//	        ResizeMode="CanResize" Background="#1E1E2E">
//	    <Grid>...</Grid>
//	</Window>
package widget

import (
	"image"
	"image/color"
	"sync/atomic"
)

// ─── Перечисления (совместимы с WPF) ────────────────────────────────────────

// WindowStyle определяет стиль обрамления окна (WPF WindowStyle).
type WindowStyle int

const (
	// WindowStyleSingleBorder — стандартное окно с рамкой и заголовком.
	WindowStyleSingleBorder WindowStyle = iota
	// WindowStyleNone — окно без обрамления и заголовка (borderless).
	WindowStyleNone
	// WindowStyleToolWindow — компактное окно-утилита с уменьшённым заголовком.
	WindowStyleToolWindow
)

// WindowTitleStyle определяет визуальный стиль кнопок и текста заголовка.
type WindowTitleStyle int

const (
	// WindowTitleWin — Windows: текст слева, кнопки ─ □ × справа.
	WindowTitleWin WindowTitleStyle = iota
	// WindowTitleMac — macOS: traffic lights ● ● ● слева, текст по центру.
	WindowTitleMac
)

// ResizeMode определяет режим изменения размера окна (WPF ResizeMode).
type ResizeMode int

const (
	// ResizeModeCanResize — окно можно изменять и сворачивать.
	ResizeModeCanResize ResizeMode = iota
	// ResizeModeNoResize — размер фиксирован, кнопки min/max скрыты.
	ResizeModeNoResize
	// ResizeModeCanMinimize — только сворачивание (maximize отключён).
	ResizeModeCanMinimize
)

// ─── Window ─────────────────────────────────────────────────────────────────

// Window — корневой виджет, представляющий независимое окно приложения.
//
// В отличие от Panel/Canvas (рабочий стол с панелями-окнами внутри),
// Window представляет одно окно ОС с собственным chrome.
// При использовании с Ebiten-бэкэндом (window.Window) определяет заголовок,
// размер и стиль нативного окна.
//
// Дочерние виджеты размещаются в клиентской области (ContentBounds) —
// под заголовком, внутри рамки.
type Window struct {
	Base

	// Title — текст заголовка окна.
	Title string

	// Style — стиль обрамления (SingleBorder, None, ToolWindow).
	Style WindowStyle

	// TitleStyle — визуальный стиль заголовка (Win или Mac).
	TitleStyle WindowTitleStyle

	// Resize — режим изменения размера окна.
	Resize ResizeMode

	// Background — цвет фона клиентской области.
	Background color.RGBA

	// BorderColor — цвет рамки окна.
	BorderColor color.RGBA

	// CornerRadius — радиус скругления углов (0 = острые).
	CornerRadius int

	// ── Настройки заголовка ──────────────────────────────────────────────────

	// TitleBarHeight — высота заголовка в пикселях.
	// 0 → авто: 32 для SingleBorder, 24 для ToolWindow, 0 для None.
	TitleBarHeight int

	// TitleBG — цвет фона заголовка (A=0 → из темы: win10.TitleBG).
	TitleBG color.RGBA

	// TitleColor — цвет текста заголовка (A=0 → из темы: win10.TitleText).
	TitleColor color.RGBA

	// ── Callbacks ────────────────────────────────────────────────────────────

	// OnClose вызывается при клике по кнопке закрытия (×).
	OnClose func()
	// OnMinimize вызывается при клике по кнопке сворачивания (─).
	OnMinimize func()
	// OnMaximize вызывается при клике по кнопке развёртывания (□).
	OnMaximize func()

	// ── Drag окна (для borderless-режима) ───────────────────────────────────

	// OnDragMove вызывается при перетаскивании за заголовок.
	// dx, dy — смещение мыши с предыдущего кадра.
	// Используется window.Window для SetWindowPosition.
	OnDragMove func(dx, dy int)

	dragging   bool
	dragStartX int
	dragStartY int

	// ── Внутреннее состояние ─────────────────────────────────────────────────

	hoverClose atomic.Int32 // 1 = курсор над ×
	hoverMin   atomic.Int32 // 1 = курсор над ─
	hoverMax   atomic.Int32 // 1 = курсор над □
}

// NewWindow создаёт окно с заданным заголовком и размером.
func NewWindow(title string, width, height int) *Window {
	w := &Window{
		Title:       title,
		Style:       WindowStyleSingleBorder,
		TitleStyle:  WindowTitleWin,
		Resize:      ResizeModeCanResize,
		Background:  win10.WindowBG,
		BorderColor: win10.Border,
	}
	w.SetBounds(image.Rect(0, 0, width, height))
	return w
}

// SetBounds обновляет bounds окна и перестраивает дочерние виджеты
// в клиентской области (ContentBounds).
// Вызывается при создании и при resize нативного окна.
func (w *Window) SetBounds(r image.Rectangle) {
	w.Base.SetBounds(r)

	// Перестраиваем дочерние виджеты — заполняют ContentBounds
	cb := w.ContentBounds()
	for _, child := range w.Children() {
		child.SetBounds(cb)
	}
}

// ─── Geometry ───────────────────────────────────────────────────────────────

// titleH возвращает фактическую высоту заголовка.
func (w *Window) titleH() int {
	if w.Style == WindowStyleNone {
		return 0
	}
	if w.TitleBarHeight > 0 {
		return w.TitleBarHeight
	}
	if w.Style == WindowStyleToolWindow {
		return 24
	}
	return 32
}

// borderW возвращает ширину рамки (0 для borderless).
func (w *Window) borderW() int {
	if w.Style == WindowStyleNone {
		return 0
	}
	return 1
}

// ContentBounds возвращает клиентскую область — прямоугольник для дочерних виджетов.
// Расположена под заголовком, внутри рамки.
func (w *Window) ContentBounds() image.Rectangle {
	b := w.Bounds()
	th := w.titleH()
	bw := w.borderW()
	return image.Rect(
		b.Min.X+bw,
		b.Min.Y+th,
		b.Max.X-bw,
		b.Max.Y-bw,
	)
}

// ─── Кнопки заголовка: геометрия и hit-test ─────────────────────────────────

const (
	winBtnW     = 46 // ширина кнопки управления (Windows-стиль)
	toolBtnW    = 32 // ширина кнопки для ToolWindow
	macCircleR  = 6  // радиус traffic light (macOS)
	macStartX   = 18 // отступ первого кружка от левого края
	macSpacing  = 22 // расстояние между центрами кружков
	macHitSlop  = 10 // допуск клика по кружку
)

// btnWidth возвращает ширину одной кнопки управления для Windows-стиля.
func (w *Window) btnWidth() int {
	if w.Style == WindowStyleToolWindow {
		return toolBtnW
	}
	return winBtnW
}

// btnCount возвращает количество кнопок управления (зависит от Style/Resize).
func (w *Window) btnCount() int {
	if w.Style == WindowStyleNone {
		return 0
	}
	if w.Style == WindowStyleToolWindow || w.Resize == ResizeModeNoResize {
		return 1 // только ×
	}
	if w.Resize == ResizeModeCanMinimize {
		return 2 // ─ и ×
	}
	return 3 // ─ □ ×
}

// CloseBtnRect возвращает bounds кнопки закрытия (×).
func (w *Window) CloseBtnRect() image.Rectangle {
	if w.TitleStyle == WindowTitleMac {
		b := w.Bounds()
		th := w.titleH()
		cx := b.Min.X + macStartX
		cy := b.Min.Y + th/2
		return image.Rect(cx-macHitSlop, cy-macHitSlop, cx+macHitSlop, cy+macHitSlop)
	}
	b := w.Bounds()
	th := w.titleH()
	bw := w.btnWidth()
	x := b.Max.X - bw
	return image.Rect(x, b.Min.Y, b.Max.X, b.Min.Y+th)
}

// MinBtnRect возвращает bounds кнопки сворачивания (─).
func (w *Window) MinBtnRect() image.Rectangle {
	if w.btnCount() < 2 {
		return image.Rectangle{}
	}
	if w.TitleStyle == WindowTitleMac {
		b := w.Bounds()
		th := w.titleH()
		cx := b.Min.X + macStartX + macSpacing
		cy := b.Min.Y + th/2
		return image.Rect(cx-macHitSlop, cy-macHitSlop, cx+macHitSlop, cy+macHitSlop)
	}
	b := w.Bounds()
	th := w.titleH()
	bw := w.btnWidth()
	n := w.btnCount()
	x := b.Max.X - bw*n
	return image.Rect(x, b.Min.Y, x+bw, b.Min.Y+th)
}

// MaxBtnRect возвращает bounds кнопки развёртывания (□).
func (w *Window) MaxBtnRect() image.Rectangle {
	if w.btnCount() < 3 {
		return image.Rectangle{}
	}
	if w.TitleStyle == WindowTitleMac {
		b := w.Bounds()
		th := w.titleH()
		cx := b.Min.X + macStartX + macSpacing*2
		cy := b.Min.Y + th/2
		return image.Rect(cx-macHitSlop, cy-macHitSlop, cx+macHitSlop, cy+macHitSlop)
	}
	b := w.Bounds()
	th := w.titleH()
	bw := w.btnWidth()
	x := b.Max.X - bw*2 // вторая справа
	return image.Rect(x, b.Min.Y, x+bw, b.Min.Y+th)
}

// ─── Draw ───────────────────────────────────────────────────────────────────

func (w *Window) Draw(ctx DrawContext) {
	b := w.Bounds()
	x, y := b.Min.X, b.Min.Y
	bw, bh := b.Dx(), b.Dy()
	th := w.titleH()
	cr := w.CornerRadius

	// ── Фон клиентской области ──────────────────────────────────────────────
	if cr > 0 {
		ctx.FillRoundRect(x, y, bw, bh, cr, w.Background)
	} else {
		ctx.FillRect(x, y, bw, bh, w.Background)
	}

	// ── Заголовок ───────────────────────────────────────────────────────────
	if th > 0 {
		switch w.TitleStyle {
		case WindowTitleWin:
			w.drawWinTitleBar(ctx)
		case WindowTitleMac:
			w.drawMacTitleBar(ctx)
		}
	}

	// ── Рамка ───────────────────────────────────────────────────────────────
	// Для borderless нативного окна рисуем только боковые линии и низ:
	// верхняя линия не нужна — заголовок уже заполняет верхний край.
	if w.Style != WindowStyleNone {
		bc := w.resolveColor(w.BorderColor, win10.Border)
		if cr > 0 {
			ctx.DrawRoundBorder(x, y, bw, bh, cr, bc)
		} else {
			// Левая, правая и нижняя линии (без верхней — там заголовок)
			ctx.DrawVLine(x, y+th, bh-th, bc)        // левая (от низа заголовка)
			ctx.DrawVLine(x+bw-1, y+th, bh-th, bc)   // правая
			ctx.DrawHLine(x, y+bh-1, bw, bc)          // нижняя
		}
		// Разделитель под заголовком
		if th > 0 {
			ctx.DrawHLine(x, y+th-1, bw, bc)
		}
	}

	// ── Дочерние виджеты ────────────────────────────────────────────────────
	w.drawChildren(ctx)
}

// ─── Windows-стиль заголовка ────────────────────────────────────────────────

func (w *Window) drawWinTitleBar(ctx DrawContext) {
	b := w.Bounds()
	x, y, bw := b.Min.X, b.Min.Y, b.Dx()
	th := w.titleH()
	cr := w.CornerRadius

	tbg := w.resolveColor(w.TitleBG, win10.TitleBG)
	tc := w.resolveColor(w.TitleColor, win10.TitleText)

	// Фон заголовка (со скруглёнными верхними углами)
	if cr > 0 {
		ctx.FillRoundRect(x, y, bw, th+cr, cr, tbg)
		ctx.FillRect(x, y+th-cr, bw, cr, tbg)
		// Восстанавливаем фон клиентской области под заголовком
		ctx.FillRect(x+1, y+th, bw-2, cr, w.Background)
	} else {
		ctx.FillRect(x, y, bw, th, tbg)
	}

	// Текст заголовка: вертикально по центру, отступ 12px слева
	textY := y + (th-13)/2
	ctx.DrawText(w.Title, x+12, textY, tc)

	// Кнопки управления
	btnW := w.btnWidth()
	btnH := th - 1
	lineColor := color.RGBA{R: 180, G: 180, B: 180, A: 255}

	nc := w.btnCount()
	if nc == 0 {
		return
	}

	// Кнопки рисуются справа налево: ×, □, ─
	bx := b.Max.X - btnW

	// × (закрыть) — всегда присутствует
	closeBG := w.closeBtnBG()
	if closeBG.A > 0 {
		ctx.FillRect(bx, y, btnW, btnH, closeBG)
	}
	cx, cy := bx+btnW/2, y+btnH/2
	closeLC := lineColor
	if w.hoverClose.Load() != 0 {
		closeLC = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	for i := -5; i <= 5; i++ {
		ctx.SetPixel(cx+i, cy+i, closeLC)
		ctx.SetPixel(cx+i, cy-i, closeLC)
		ctx.SetPixel(cx+i+1, cy+i, closeLC)
		ctx.SetPixel(cx+i+1, cy-i, closeLC)
	}

	if nc < 2 {
		return
	}

	// □ (развернуть) — если 3 кнопки
	if nc >= 3 {
		bx -= btnW
		if w.hoverMax.Load() != 0 {
			hoverBG := color.RGBA{R: 80, G: 80, B: 80, A: 100}
			ctx.FillRect(bx, y, btnW, btnH, hoverBG)
		}
		ry := y + btnH/2 - 5
		ctx.DrawBorder(bx+btnW/2-5, ry, 11, 11, lineColor)
	}

	// ─ (свернуть) — если 2+ кнопки
	bx2 := b.Max.X - btnW*nc
	if w.hoverMin.Load() != 0 {
		hoverBG := color.RGBA{R: 80, G: 80, B: 80, A: 100}
		ctx.FillRect(bx2, y, btnW, btnH, hoverBG)
	}
	my := y + btnH/2
	ctx.DrawHLine(bx2+btnW/2-7, my, 14, lineColor)
}

// closeBtnBG возвращает фон кнопки закрытия (красный при hover).
func (w *Window) closeBtnBG() color.RGBA {
	if w.hoverClose.Load() != 0 {
		return color.RGBA{R: 232, G: 17, B: 35, A: 255} // Win10 close hover
	}
	return color.RGBA{}
}

// ─── macOS-стиль заголовка ──────────────────────────────────────────────────

func (w *Window) drawMacTitleBar(ctx DrawContext) {
	b := w.Bounds()
	x, y, bw := b.Min.X, b.Min.Y, b.Dx()
	th := w.titleH()
	cr := w.CornerRadius

	tbg := w.resolveColor(w.TitleBG, win10.TitleBG)
	tc := w.resolveColor(w.TitleColor, win10.TitleText)

	// Фон заголовка
	if cr > 0 {
		ctx.FillRoundRect(x, y, bw, th+cr, cr, tbg)
		ctx.FillRect(x, y+th-cr, bw, cr, tbg)
		ctx.FillRect(x+1, y+th, bw-2, cr, w.Background)
	} else {
		ctx.FillRect(x, y, bw, th, tbg)
	}

	// Traffic lights: красный (close), жёлтый (minimize), зелёный (maximize)
	cy := y + th/2
	nc := w.btnCount()

	type trafficLight struct {
		col     color.RGBA
		hoverFn func() int32
	}

	lights := []trafficLight{
		{color.RGBA{R: 255, G: 95, B: 86, A: 255}, func() int32 { return w.hoverClose.Load() }},
	}
	if nc >= 2 {
		lights = append(lights, trafficLight{
			color.RGBA{R: 255, G: 189, B: 46, A: 255},
			func() int32 { return w.hoverMin.Load() },
		})
	}
	if nc >= 3 {
		lights = append(lights, trafficLight{
			color.RGBA{R: 39, G: 201, B: 63, A: 255},
			func() int32 { return w.hoverMax.Load() },
		})
	}

	for i, lt := range lights {
		ccx := x + macStartX + i*macSpacing
		col := lt.col
		if lt.hoverFn() != 0 {
			// Чуть ярче при hover
			col = brighten(col, 30)
		}
		fillCircle(ctx, ccx, cy, macCircleR, col)
	}

	// Текст заголовка: по центру
	textW := ctx.MeasureText(w.Title, 10)
	textX := x + (bw-textW)/2
	textY := y + (th-13)/2
	ctx.DrawText(w.Title, textX, textY, tc)
}

// ─── Mouse events ───────────────────────────────────────────────────────────

// OnMouseMove обновляет hover-состояние кнопок заголовка и обрабатывает drag.
func (w *Window) OnMouseMove(x, y int) {
	// Drag за заголовок: перемещение нативного окна
	if w.dragging {
		dx := x - w.dragStartX
		dy := y - w.dragStartY
		if w.OnDragMove != nil && (dx != 0 || dy != 0) {
			w.OnDragMove(dx, dy)
		}
		// Не обновляем dragStart — координаты мыши относительны окна,
		// а окно уже сдвинулось, так что следующий OnMouseMove будет
		// с теми же локальными координатами если мышь стоит.
		return
	}

	pt := image.Pt(x, y)

	var hc, hm, hx int32
	if pt.In(w.CloseBtnRect()) {
		hc = 1
	}
	if r := w.MinBtnRect(); !r.Empty() && pt.In(r) {
		hm = 1
	}
	if r := w.MaxBtnRect(); !r.Empty() && pt.In(r) {
		hx = 1
	}
	w.hoverClose.Store(hc)
	w.hoverMin.Store(hm)
	w.hoverMax.Store(hx)
}

// titleBarRect возвращает прямоугольник заголовка (без кнопок управления).
func (w *Window) titleBarRect() image.Rectangle {
	b := w.Bounds()
	th := w.titleH()
	if th == 0 {
		return image.Rectangle{}
	}
	// Вся полоса заголовка (кнопки обрабатываются отдельно)
	return image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+th)
}

// OnMouseButton обрабатывает клик по кнопкам заголовка и начало drag.
func (w *Window) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	pt := image.Pt(e.X, e.Y)

	// Отпускание кнопки — прекращаем drag
	if !e.Pressed {
		if w.dragging {
			w.dragging = false
			return true
		}
		return false
	}

	// Нажатие — проверяем кнопки управления
	if pt.In(w.CloseBtnRect()) {
		if w.OnClose != nil {
			w.OnClose()
		}
		return true
	}
	if r := w.MinBtnRect(); !r.Empty() && pt.In(r) {
		if w.OnMinimize != nil {
			w.OnMinimize()
		}
		return true
	}
	if r := w.MaxBtnRect(); !r.Empty() && pt.In(r) {
		if w.OnMaximize != nil {
			w.OnMaximize()
		}
		return true
	}

	// Нажатие на заголовок (не на кнопку) — начинаем drag
	if pt.In(w.titleBarRect()) {
		w.dragging = true
		w.dragStartX = e.X
		w.dragStartY = e.Y
		return true
	}

	return false
}

// ─── Themeable ──────────────────────────────────────────────────────────────

// ApplyTheme обновляет цвета Window из темы.
func (w *Window) ApplyTheme(t *Theme) {
	w.Background = t.WindowBG
	w.BorderColor = t.Border
	// TitleBG и TitleColor обновляются только если пользователь не задал явно (A=0)
}

// ─── Вспомогательные ────────────────────────────────────────────────────────

// resolveColor возвращает c, если он не прозрачный; иначе fallback.
func (w *Window) resolveColor(c, fallback color.RGBA) color.RGBA {
	if c.A > 0 {
		return c
	}
	return fallback
}

// fillCircle рисует закрашенный круг (для traffic lights).
func fillCircle(ctx DrawContext, cx, cy, r int, col color.RGBA) {
	ctx.FillRoundRect(cx-r, cy-r, r*2, r*2, r, col)
}

// brighten увеличивает яркость каждого канала на delta (без переполнения).
func brighten(c color.RGBA, delta uint8) color.RGBA {
	add := func(v, d uint8) uint8 {
		s := uint16(v) + uint16(d)
		if s > 255 {
			return 255
		}
		return uint8(s)
	}
	return color.RGBA{R: add(c.R, delta), G: add(c.G, delta), B: add(c.B, delta), A: c.A}
}
