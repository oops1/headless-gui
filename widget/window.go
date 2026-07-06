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
//	        WindowStyle="SingleBorderWindow" TitleStyle="Auto"
//	        ResizeMode="CanResize" Background="#1E1E2E">
//	    <Grid>...</Grid>
//	</Window>
package widget

import (
	"image"
	"image/color"
	"sync"
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
	// WindowTitleAuto — автоматический выбор стиля по текущей ОС:
	// macOS → Mac-стиль, Windows/Linux → Windows-стиль.
	// Это значение по умолчанию (zero value).
	WindowTitleAuto WindowTitleStyle = iota
	// WindowTitleWin — Windows: текст слева, кнопки ─ □ × справа.
	WindowTitleWin
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

	// ShowLocaleIndicator — показывать индикатор текущей локали (напр. «EN»)
	// в заголовке окна. По умолчанию true; отключаемое свойство.
	ShowLocaleIndicator bool

	// inactive — окно без фокуса ОС: заголовок рисуется приглушённым
	// (Win2000 — серый градиент, Mac — серые «светофоры», прочие — dim).
	// Zero value = активно; хранится инвертированно.
	inactive bool

	// InputBindings — горячие клавиши окна (WPF Window.InputBindings).
	InputBindings []InputBinding

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
	dragWinX   int // позиция окна при начале drag (для headless self-drag)
	dragWinY   int

	// capMgr — CaptureManager для корректного освобождения захвата мыши.
	capMgr CaptureManager

	// ── Внутреннее состояние ─────────────────────────────────────────────────

	hoverClose atomic.Int32 // 1 = курсор над ×
	hoverMin   atomic.Int32 // 1 = курсор над ─
	hoverMax   atomic.Int32 // 1 = курсор над □

	// ── Контекстное меню выбора локали ───────────────────────────────────────
	localeMu        sync.Mutex
	localeBadgeRect image.Rectangle   // прямоугольник плашки локали (для hit-теста)
	localeMenuOpen  bool              // открыт ли выпадающий список выбора локали
	localeItemRects []image.Rectangle // прямоугольники пунктов меню (для hit-теста)
}

// NewWindow создаёт окно с заданным заголовком и размером.
// TitleStyle по умолчанию = WindowTitleAuto (определяется по текущей ОС).
func NewWindow(title string, width, height int) *Window {
	w := &Window{
		Title:               title,
		Style:               WindowStyleSingleBorder,
		TitleStyle:          WindowTitleAuto, // авто-определение по ОС
		Resize:              ResizeModeCanResize,
		Background:          win10.WindowBG,
		BorderColor:         win10.Border,
		ShowLocaleIndicator: true,
	}
	w.SetBounds(image.Rect(0, 0, width, height))
	return w
}

// SetTitle задаёт текст заголовка окна (для биндингов и программного
// обновления). При фактическом изменении инвалидирует область окна.
func (w *Window) SetTitle(s string) {
	if w.Title != s {
		w.Title = s
		w.Invalidate()
	}
}

// resolvedTitleStyle возвращает конкретный стиль заголовка.
// Если TitleStyle == WindowTitleAuto, определяет по текущей ОС.
func (w *Window) resolvedTitleStyle() WindowTitleStyle {
	if w.TitleStyle == WindowTitleAuto {
		return detectedTitleStyle()
	}
	return w.TitleStyle
}

// DetectedOS возвращает строковое имя текущей ОС ("windows", "darwin", "linux").
func DetectedOS() string {
	return detectedOS()
}

// DetectedTitleStyle возвращает стиль заголовка, соответствующий текущей ОС.
func DetectedTitleStyle() WindowTitleStyle {
	return detectedTitleStyle()
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

// ─── Активность окна (фокус ОС) ─────────────────────────────────────────────

// SetActive задаёт активность окна: неактивное (без фокуса ОС) рисует
// приглушённый заголовок в стиле темы. Вызывается window.Window по
// WM_ACTIVATE / X11 FocusIn/FocusOut.
func (w *Window) SetActive(v bool) {
	if w.inactive == !v {
		return
	}
	w.inactive = !v
	w.Invalidate()
}

// IsActive возвращает true, если окно активно (по умолчанию — да).
func (w *Window) IsActive() bool { return !w.inactive }

// titleColors возвращает эффективные цвета заголовка с учётом активности:
// для неактивного окна — явные Inactive-токены темы либо автоматическое
// приглушение (смешивание с серым/фоном).
func (w *Window) titleColors() (bg, bg2, text color.RGBA) {
	bg = w.resolveColor(w.TitleBG, win10.TitleBG)
	bg2 = win10.TitleBG2
	text = w.resolveColor(w.TitleColor, win10.TitleText)
	if !w.inactive {
		return bg, bg2, text
	}
	if t := win10.TitleBGInactive; t.A > 0 {
		bg = t
	} else {
		bg = dimColor(bg)
	}
	if t := win10.TitleBG2Inactive; t.A > 0 {
		bg2 = t
	} else if bg2.A > 0 {
		bg2 = dimColor(bg2)
	}
	if t := win10.TitleTextInactive; t.A > 0 {
		text = t
	} else {
		text = mixRGBA(text, bg, 0.45)
	}
	return bg, bg2, text
}

// dimColor приглушает цвет: снижает насыщенность и тянет к серому.
func dimColor(c color.RGBA) color.RGBA {
	gray := uint8((uint32(c.R)*299 + uint32(c.G)*587 + uint32(c.B)*114) / 1000)
	return mixRGBA(c, color.RGBA{R: gray, G: gray, B: gray, A: c.A}, 0.6)
}

// mixRGBA линейно смешивает a→b на долю t (0 = a, 1 = b).
func mixRGBA(a, b color.RGBA, t float64) color.RGBA {
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: a.A}
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

// classicTitleBtnRects возвращает прямоугольники кнопок ×, □, ─ для
// классического стиля Win2000 (общая геометрия для отрисовки и hit-test).
// Отсутствующие кнопки возвращаются как пустой Rectangle.
func (w *Window) classicTitleBtnRects() (closeR, maxR, minR image.Rectangle) {
	const cbw, cbh, gap = 16, 14, 2
	b := w.Bounds()
	th := w.titleH()
	nc := w.btnCount()
	by := b.Min.Y + (th-cbh)/2
	closeX := b.Max.X - 4 - cbw
	closeR = image.Rect(closeX, by, closeX+cbw, by+cbh)
	if nc >= 3 {
		mxX := closeX - gap - cbw
		maxR = image.Rect(mxX, by, mxX+cbw, by+cbh)
		minR = image.Rect(mxX-cbw, by, mxX, by+cbh)
	} else if nc == 2 {
		mnX := closeX - gap - cbw
		minR = image.Rect(mnX, by, mnX+cbw, by+cbh)
	}
	return closeR, maxR, minR
}

// CloseBtnRect возвращает bounds кнопки закрытия (×).
// Пустой прямоугольник для WindowStyleNone (кнопок нет). Явная проверка
// обязательна: mac-ветка (хит-зона «светофора») не вырождается в пустую
// при нулевой высоте заголовка — на darwin без неё кнопка «существовала».
func (w *Window) CloseBtnRect() image.Rectangle {
	if w.btnCount() == 0 {
		return image.Rectangle{}
	}
	if w.resolvedTitleStyle() == WindowTitleMac {
		b := w.Bounds()
		th := w.titleH()
		cx := b.Min.X + macStartX
		cy := b.Min.Y + th/2
		return image.Rect(cx-macHitSlop, cy-macHitSlop, cx+macHitSlop, cy+macHitSlop)
	}
	if currentStyle().Classic3D {
		r, _, _ := w.classicTitleBtnRects()
		return r
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
	if w.resolvedTitleStyle() == WindowTitleMac {
		b := w.Bounds()
		th := w.titleH()
		cx := b.Min.X + macStartX + macSpacing
		cy := b.Min.Y + th/2
		return image.Rect(cx-macHitSlop, cy-macHitSlop, cx+macHitSlop, cy+macHitSlop)
	}
	if currentStyle().Classic3D {
		_, _, r := w.classicTitleBtnRects()
		return r
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
	if w.resolvedTitleStyle() == WindowTitleMac {
		b := w.Bounds()
		th := w.titleH()
		cx := b.Min.X + macStartX + macSpacing*2
		cy := b.Min.Y + th/2
		return image.Rect(cx-macHitSlop, cy-macHitSlop, cx+macHitSlop, cy+macHitSlop)
	}
	if currentStyle().Classic3D {
		_, r, _ := w.classicTitleBtnRects()
		return r
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
		switch w.resolvedTitleStyle() {
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
		if w.inactive {
			// Рамка неактивного окна светлее (как в Win11).
			bc = mixRGBA(bc, w.Background, 0.5)
		}
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

	// Эффективные цвета заголовка (учёт активности окна: без фокуса ОС
	// заголовок приглушается — Win2000 серый градиент, прочие темы dim).
	tbg, tbg2, tc := w.titleColors()

	// Фон заголовка (со скруглёнными верхними углами)
	if cr > 0 {
		ctx.FillRoundRect(x, y, bw, th+cr, cr, tbg)
		ctx.FillRect(x, y+th-cr, bw, cr, tbg)
		// Восстанавливаем фон клиентской области под заголовком
		ctx.FillRect(x+1, y+th, bw-2, cr, w.Background)
	} else {
		// Классика Win2000 — градиент navy→голубой (серый у неактивного).
		fillTitleBarColors(ctx, image.Rect(x, y, x+bw, y+th), tbg, tbg2)
	}

	// Текст заголовка: вертикально по центру, отступ 12px слева
	// (в классике — жирный, как в Win2000).
	textY := y + (th-13)/2
	drawTitleText(ctx, w.Title, x+12, textY, tc)

	// Индикатор локали — слева от кнопок управления.
	if w.ShowLocaleIndicator {
		nc0 := w.btnCount()
		rightX := b.Max.X - 8
		if currentStyle().Classic3D {
			if _, _, minR := w.classicTitleBtnRects(); !minR.Empty() {
				rightX = minR.Min.X - 8
			} else if closeR, _, _ := w.classicTitleBtnRects(); !closeR.Empty() {
				rightX = closeR.Min.X - 8
			}
		} else if nc0 > 0 {
			rightX = b.Max.X - w.btnWidth()*nc0 - 8
		}
		w.setLocaleBadgeRect(drawLocaleBadge(ctx, rightX, y, th, tc))
	}

	nc := w.btnCount()
	if nc == 0 {
		return
	}

	// Классика Win2000: кнопки управления — выпуклые bevel-кнопки на «лице»
	// с чёрными глифами (иначе светло-серые глифы сливаются с navy-заголовком).
	if st := currentStyle(); st.Classic3D {
		w.drawClassicTitleButtons(ctx, st)
		return
	}

	// Кнопки управления
	btnW := w.btnWidth()
	btnH := th - 1
	lineColor := color.RGBA{R: 180, G: 180, B: 180, A: 255}

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

// drawClassicTitleButtons рисует кнопки ─ □ × в классическом стиле Win2000:
// маленькие выпуклые bevel-кнопки на «лице» с чёрными глифами.
func (w *Window) drawClassicTitleButtons(ctx DrawContext, st ThemeStyle) {
	face := win10.PanelBG // «лицо» Win2000 (#D4D0C8)
	glyph := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	closeR, maxR, minR := w.classicTitleBtnRects()

	// Рисует одну bevel-кнопку и возвращает её центр.
	drawBtn := func(r image.Rectangle) (int, int) {
		ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), face)
		drawBevelRaised(ctx, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), st)
		return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2
	}

	// × (закрыть).
	cx, cy := drawBtn(closeR)
	for i := -3; i <= 3; i++ {
		ctx.SetPixel(cx+i, cy+i, glyph)
		ctx.SetPixel(cx+i, cy-i, glyph)
		ctx.SetPixel(cx+i+1, cy+i, glyph)
	}
	// □ (развернуть).
	if !maxR.Empty() {
		mcx, mcy := drawBtn(maxR)
		ctx.DrawBorder(mcx-4, mcy-4, 9, 8, glyph)
		ctx.DrawHLine(mcx-4, mcy-3, 9, glyph) // двойная верхняя грань заголовка
	}
	// ─ (свернуть).
	if !minR.Empty() {
		ncx, ncy := drawBtn(minR)
		ctx.DrawHLine(ncx-4, ncy+3, 8, glyph)
		ctx.DrawHLine(ncx-4, ncy+4, 8, glyph)
	}
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

	tbg, _, tc := w.titleColors()

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
		if w.inactive {
			// Неактивное окно: серые «светофоры» (как в настоящем macOS).
			col = color.RGBA{R: 178, G: 178, B: 178, A: 255}
		} else if lt.hoverFn() != 0 {
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

	// Индикатор локали — в правом верхнем углу (traffic lights слева в Mac-стиле).
	if w.ShowLocaleIndicator {
		w.setLocaleBadgeRect(drawLocaleBadge(ctx, b.Max.X-8, y, th, tc))
	}
}

// ─── Drag / Capture ─────────────────────────────────────────────────────────

// WantsCapture возвращает true при клике в drag-зону заголовка.
// Это позволяет движку захватить мышь для Window (аналогично Panel.WantsCapture).
func (w *Window) WantsCapture(e MouseEvent) bool {
	if w.Style == WindowStyleNone {
		return false
	}
	pt := image.Pt(e.X, e.Y)
	tb := w.titleBarRect()
	if tb.Empty() || !pt.In(tb) {
		return false
	}
	// Не захватываем если клик по кнопкам управления
	if pt.In(w.CloseBtnRect()) {
		return false
	}
	if r := w.MinBtnRect(); !r.Empty() && pt.In(r) {
		return false
	}
	if r := w.MaxBtnRect(); !r.Empty() && pt.In(r) {
		return false
	}
	return true
}

// SetCaptureManager сохраняет CaptureManager для освобождения захвата при отпускании.
func (w *Window) SetCaptureManager(cm CaptureManager) {
	w.capMgr = cm
}

// ─── Mouse events ───────────────────────────────────────────────────────────

// OnMouseMove обновляет hover-состояние кнопок заголовка и обрабатывает drag.
func (w *Window) OnMouseMove(x, y int) {
	// Drag за заголовок
	if w.dragging {
		dx := x - w.dragStartX
		dy := y - w.dragStartY
		if dx == 0 && dy == 0 {
			return
		}
		if w.OnDragMove != nil {
			// Нативный режим: делегируем движение нативному окну.
			// Не обновляем dragStart — координаты мыши относительны окна.
			w.OnDragMove(dx, dy)
		} else {
			// Headless self-drag: перемещаем виджет по Canvas/Panel.
			// Используем абсолютные координаты: новая позиция = начальная + дельта мыши.
			newX := w.dragWinX + (x - w.dragStartX)
			newY := w.dragWinY + (y - w.dragStartY)
			b := w.Bounds()
			shiftX := newX - b.Min.X
			shiftY := newY - b.Min.Y
			if shiftX != 0 || shiftY != 0 {
				// Window.SetBounds сам пересчитает ContentBounds → дочерние виджеты.
				// Рекурсивный ShiftWidget не нужен — Window управляет layout.
				w.SetBounds(image.Rect(newX, newY, newX+b.Dx(), newY+b.Dy()))
			}
		}
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
	// Кнопки заголовка рисуются в bounds окна — при фактической смене
	// hover-состояния достаточно точечной инвалидации.
	c1 := w.hoverClose.Swap(hc) != hc
	c2 := w.hoverMin.Swap(hm) != hm
	c3 := w.hoverMax.Swap(hx) != hx
	if c1 || c2 || c3 {
		w.Invalidate()
	}
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
	// Контекстное меню выбора локали (правый клик по заголовку / клик по плашке).
	if consumed, handled := w.handleLocaleMouse(e); handled {
		return consumed
	}

	if e.Button != MouseLeft {
		return false
	}
	pt := image.Pt(e.X, e.Y)

	// Отпускание кнопки — прекращаем drag
	if !e.Pressed {
		if w.dragging {
			w.dragging = false
			if w.capMgr != nil {
				w.capMgr.ReleaseCapture()
			}
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
		DismissAll(w) // закрываем dropdown/popup перед drag
		w.dragging = true
		w.dragStartX = e.X
		w.dragStartY = e.Y
		b := w.Bounds()
		w.dragWinX = b.Min.X
		w.dragWinY = b.Min.Y
		return true
	}

	return false
}

// ─── Контекстное меню выбора локали ─────────────────────────────────────────

// setLocaleBadgeRect сохраняет прямоугольник плашки локали (для hit-теста).
func (w *Window) setLocaleBadgeRect(r image.Rectangle) {
	w.localeMu.Lock()
	w.localeBadgeRect = r
	w.localeMu.Unlock()
}

// localeMenuList возвращает список локалей для меню (из ОС или дефолт).
func localeMenuList() []string {
	items := AvailableLocales()
	if len(items) == 0 {
		items = []string{"EN", "RU"}
	}
	return items
}

// HasOverlay реализует OverlayDrawer — true, когда открыто меню выбора локали.
func (w *Window) HasOverlay() bool {
	w.localeMu.Lock()
	defer w.localeMu.Unlock()
	return w.localeMenuOpen && w.ShowLocaleIndicator
}

// DrawOverlay рисует выпадающий список выбора локали поверх всего окна.
func (w *Window) DrawOverlay(ctx DrawContext) {
	w.localeMu.Lock()
	open := w.localeMenuOpen
	badge := w.localeBadgeRect
	w.localeMu.Unlock()
	if !open || badge.Empty() {
		return
	}
	rects := drawLocaleMenu(ctx, badge, localeMenuList(), Locale())
	w.localeMu.Lock()
	w.localeItemRects = rects
	w.localeMu.Unlock()
}

// Dismiss реализует Dismissable — закрывает меню локали при клике в стороне.
func (w *Window) Dismiss() {
	w.localeMu.Lock()
	wasOpen := w.localeMenuOpen
	w.localeMenuOpen = false
	w.localeMu.Unlock()
	if wasOpen {
		notifyUIChanged() // закрытие overlay-меню локали
	}
}

// handleLocaleMouse обрабатывает клики, связанные с меню локали.
// Возвращает (consumed, handled): handled=true означает, что клик относится
// к меню локали и дальнейшая обработка в OnMouseButton не нужна.
func (w *Window) handleLocaleMouse(e MouseEvent) (consumed bool, handled bool) {
	if !w.ShowLocaleIndicator || !e.Pressed {
		return false, false
	}
	pt := image.Pt(e.X, e.Y)

	w.localeMu.Lock()
	open := w.localeMenuOpen
	badge := w.localeBadgeRect
	itemRects := append([]image.Rectangle(nil), w.localeItemRects...)
	w.localeMu.Unlock()

	if open {
		// Клик по пункту меню → выбор локали.
		for i, r := range itemRects {
			if pt.In(r) {
				items := localeMenuList()
				w.localeMu.Lock()
				w.localeMenuOpen = false
				w.localeMu.Unlock()
				notifyUIChanged() // закрытие overlay-меню локали
				if i < len(items) {
					RequestLocale(items[i])
				}
				return true, true
			}
		}
		// Клик мимо меню → закрыть и поглотить.
		w.localeMu.Lock()
		w.localeMenuOpen = false
		w.localeMu.Unlock()
		notifyUIChanged() // закрытие overlay-меню локали
		return true, true
	}

	// Открытие: правый клик по заголовку (контекстное меню) или клик по плашке.
	inBadge := !badge.Empty() && pt.In(badge)
	openReq := (e.Button == MouseRight && pt.In(w.titleBarRect())) ||
		(e.Button == MouseLeft && inBadge)
	if openReq {
		w.localeMu.Lock()
		w.localeMenuOpen = true
		w.localeMu.Unlock()
		notifyUIChanged() // открытие overlay-меню локали
		return true, true
	}
	return false, false
}

// HandleInputBinding выполняет команду, привязанную к горячей клавише (KeyBinding).
// Возвращает true, если клавиша обработана.
func (w *Window) HandleInputBinding(code KeyCode, mod KeyMod) bool {
	cmd, param, ok := matchInputBinding(w.InputBindings, code, mod)
	if ok && cmd.CanExecute(param) {
		cmd.Execute(param)
		return true
	}
	return false
}

// ─── Themeable ──────────────────────────────────────────────────────────────

// ApplyTheme обновляет цвета и форму Window из темы.
func (w *Window) ApplyTheme(t *Theme) {
	w.Background = t.WindowBG
	w.BorderColor = t.Border
	// TitleBG и TitleColor обновляются только если пользователь не задал явно (A=0)

	// Форма окна определяется темой: скругление углов (Win11/Mac) и стиль
	// заголовка (Mac → traffic-lights, остальные → Windows-стиль).
	w.CornerRadius = t.Style.WindowCorner
	if t.Style.MacTitleBar {
		w.TitleStyle = WindowTitleMac
	} else {
		w.TitleStyle = WindowTitleWin
	}
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
