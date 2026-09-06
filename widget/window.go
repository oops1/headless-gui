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

// OutlineDragStyle — из чего состоит контур перетаскивания (OutlineDrag).
type OutlineDragStyle int

const (
	// OutlineDragBorder — только рамка (по умолчанию): за кадр меняются
	// несколько тонких полос пикселей, дёшево по трафику.
	OutlineDragBorder OutlineDragStyle = iota
	// OutlineDragFilled — рамка плюс полупрозрачная заливка: меняется каждый
	// пиксель под контуром. Заметно дороже, см. Window.OutlineDragStyle.
	OutlineDragFilled
)

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

	// dragAreas — части содержимого, работающие как полоса заголовка
	// (dialog_chrome.go). Нужны borderless-окну: своей полосы у него нет.
	dragAreas []image.Rectangle

	// titleBarContent/navBtn — начинка штатной полосы заголовка (titlebar.go):
	// виджет приложения в её свободной части и кнопка сворачивания слева.
	titleBarContent Widget
	navBtn          *titleNavBtn
	navPanel        Widget

	// OnNavToggle вызывается кнопкой сворачивания (SetNavButton): collapsed —
	// состояние ПОСЛЕ нажатия. Сворачивает боковую область приложение.
	OnNavToggle func(collapsed bool)

	// Style — стиль обрамления (SingleBorder, None, ToolWindow).
	Style WindowStyle

	// TitleStyle — визуальный стиль заголовка (Win или Mac).
	TitleStyle WindowTitleStyle

	// ownStyle — стиль темы, которой окно принадлежит. nil означает «как у
	// всех» — берётся общий стиль.
	//
	// Своё поле, а не общая переменная, потому что геометрия окна зависит от
	// стиля (в классике Windows 2000 рамка 5px и заголовок 24px, в остальных
	// темах 1px и 32px), а спрашивают её НЕ ТОЛЬКО из Draw: хит-тест
	// заголовка, перекладка детей и ContentBounds зовутся из обработки ввода,
	// когда подмена стиля области темы (ThemeScope) уже снята. Окно в области
	// с темой Windows 2000 ловило мышь по границам заголовка Windows 11 —
	// промах в восемь точек по вертикали.
	ownStyle *ThemeStyle

	// opaqueBuf — буфер под ответ OpaqueRegion (см. oneRegion).
	opaqueBuf [1]image.Rectangle

	// Resize — режим изменения размера окна.
	Resize ResizeMode

	// MinWidth / MinHeight — минимальный размер окна в ЛОГИЧЕСКИХ пикселях.
	// 0 → дефолт: для виджетного edge-resize — winMinW×winMinH (120×80),
	// для нативного окна — движок ОС (Win32: 320×240). Задаётся из XAML
	// (атрибуты MinWidth/MinHeight) или программно.
	MinWidth  int
	MinHeight int

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

	// MainWindow — является ли окно «главным». У главного окна во всех темах,
	// кроме классической Win2000 (Classic3D), рисуется контрастная XOR-рамка
	// 1px (инверсия фона окна) — чтобы границы были видны на фоне канваса
	// того же тона. По умолчанию true; для вложенных окон в XAML задаётся
	// MainWindow="False".
	MainWindow bool

	// inactive — окно без фокуса ОС: заголовок рисуется приглушённым
	// (Win2000 — серый градиент, Mac — серые «светофоры», прочие — dim).
	// Zero value = активно; хранится инвертированно.
	inactive bool

	// InputBindings — горячие клавиши окна (WPF Window.InputBindings).
	InputBindings []InputBinding

	// ── Декларации трея (XAML <Window TrayIcon=… TrayTooltip=…> + <TrayMenu>) ──
	//
	// Эти поля — ТОЛЬКО декларация из XAML: widget-пакет не может импортировать
	// window (зависимость window→widget), поэтому окно складывает намерение сюда,
	// а window.Window подхватывает его в Run() и вызывает SetTrayIcon/SetTrayMenu.
	// Явные вызовы приложения (SetTrayIcon/SetTrayMenu до Run) имеют приоритет и
	// эти поля не перетирают.

	// TrayIconImage — уже загруженная иконка трея (из атрибута TrayIcon,
	// .png/.jpg декодируется, .svg растеризуется 32×32). nil — иконки нет.
	TrayIconImage image.Image
	// TrayTooltip — всплывающая подсказка иконки трея (по умолчанию = Title).
	TrayTooltip string
	// TrayMenu — контекстное меню трея (из дочернего тега <TrayMenu>). Хранится
	// ПОЛЕМ, а не ребёнком дерева (PopupMenu прямым ребёнком Window опасен —
	// см. Window.SetBounds, где *PopupMenu пропускается); window.attachTrayMenu
	// сам добавит его в дерево корректно.
	TrayMenu *PopupMenu

	// ── Callbacks ────────────────────────────────────────────────────────────

	// OnClose вызывается при ОТПУСКАНИИ кнопки закрытия (×), если курсор всё
	// ещё над ней (Windows-семантика: нажатие «взводит» кнопку, отпускание
	// в стороне отменяет действие).
	OnClose func()
	// OnMinimize вызывается при ОТПУСКАНИИ кнопки сворачивания (─), если
	// курсор всё ещё над ней. См. OnClose.
	OnMinimize func()
	// OnMaximize вызывается при ОТПУСКАНИИ кнопки развёртывания (□), если
	// курсор всё ещё над ней. См. OnClose.
	OnMaximize func()

	// ── Вкладки в заголовке (стиль Windows 11 Terminal) ─────────────────────

	// OnTitleTabChange вызывается при смене активной вкладки заголовка.
	OnTitleTabChange func(idx int, header string)
	// OnTitleTabClosed вызывается после закрытия вкладки заголовка.
	OnTitleTabClosed func(idx int, header string)
	// OnTitleTabNew вызывается по клику на «+» в полосе вкладок.
	// nil — кнопка «+» не показывается.
	OnTitleTabNew func()

	// titleTabs — состояние режима вкладок (nil — режим выключен).
	// См. window_tabs.go: EnableTitleTabs / AddTitleTab.
	titleTabs *titleTabsState

	// ── Перетаскивание контуром (OutlineDrag) ───────────────────────────────

	// OutlineDrag — таскать за заголовок КОНТУР окна, а не само окно; окно
	// переезжает один раз, при отпускании кнопки. Так работает режим
	// «не показывать содержимое окна при перетаскивании» (Windows) и
	// PERF_DISABLE_FULLWINDOWDRAG у RDP-клиентов: по сети уходит контур в
	// пару сотен байт вместо кадров движущегося окна.
	//
	// Работает и для окон на канвасе (окно двигает себя само), и в нативном
	// режиме (OnDragMove вызывается один раз с итоговым смещением); в
	// нативном режиме контур рисуется внутри окна и обрезается его краями.
	OutlineDrag bool

	// OutlineDragStyle — из чего состоит контур. По умолчанию
	// OutlineDragBorder: только рамка, несколько тонких полос за кадр.
	// Заливка (OutlineDragFilled) перекрашивает КАЖДЫЙ пиксель под окном —
	// на удалённом рабочем столе это дороже, чем просто двигать окно, ради
	// чего режим и заводился. Включайте её только для локального экрана.
	OutlineDragStyle OutlineDragStyle

	// OutlineDragFill — цвет заливки при OutlineDragFilled (A=0 → из темы,
	// а если и там не задан — тон рамки окна). Alpha-premultiplied.
	OutlineDragFill color.RGBA

	// outlineRect — текущее положение контура; непустой только пока идёт
	// перетаскивание контуром. outlineDX/DY — суммарное смещение мыши.
	outlineRect image.Rectangle
	outlineDX   int
	outlineDY   int

	// ── Drag окна (для borderless-режима) ───────────────────────────────────

	// OnDragMove вызывается при перетаскивании за заголовок.
	// dx, dy — смещение мыши с предыдущего кадра.
	// Используется window.Window для SetWindowPosition.
	//
	// Сам по себе хук НЕ отключает ресайз за края: приложение может забрать
	// перемещение и сохранить изменение размера. Ресайз краями выключает
	// SetNativeHosted — там размером ведает ОС.
	OnDragMove func(dx, dy int)

	// nativeHosted — окно живёт в нативном окне ОС (window.Window): размер и
	// положение меняет система, виджетный ресайз за края конфликтовал бы с ней.
	nativeHosted bool

	dragging   bool
	dragStartX int
	dragStartY int
	dragWinX   int // позиция окна при начале drag (для headless self-drag)
	dragWinY   int

	// ── Resize перетаскиванием краёв (для «виртуальных» окон на канвасе) ─────
	resizing     bool            // идёт ли изменение размера за край
	resizeDir    winEdge         // направление(я) активного ресайза (битовая маска)
	resizeStart  image.Rectangle // bounds окна на момент начала ресайза
	resizeStartX int             // координаты мыши на момент начала ресайза
	resizeStartY int

	// ── Взведённая кнопка заголовка (release-семантика) ─────────────────────
	// armedBtn — какая кнопка управления «взведена» нажатием; колбэк вызывается
	// на отпускании, только если курсор всё ещё над ней.
	armedBtn titleButton

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
		MainWindow:          true,
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
// Если TitleStyle == WindowTitleAuto, определяет по текущей ОС, но классика
// Win2000 — исконно виндовый хром: traffic lights в ней не существуют, поэтому
// Auto в классике всегда Windows-стиль независимо от ОС (иначе на darwin
// hit-тест и отрисовка кнопок уходили в мак-ветку внутри 3D-рамки).
func (w *Window) resolvedTitleStyle() WindowTitleStyle {
	if w.TitleStyle == WindowTitleAuto {
		if w.style().Classic3D {
			return WindowTitleWin
		}
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
		// PopupMenu, прикреплённое к корню (например, трей-меню — оно живёт
		// в дереве, чтобы его оверлей собирал popup-хост), позиционируется
		// собственным Show() — растягивать его нельзя: закрытое меню размером
		// с клиентскую область становилось верхним в Z невидимым поглотителем
		// ВСЕГО ввода после любой перекладки (смена темы, ресайз окна).
		if _, ok := child.(*PopupMenu); ok {
			continue
		}
		// Начинка полосы заголовка живёт в полосе, а не в клиентской области:
		// растянуть её на ContentBounds значило бы положить поиск поверх всего
		// содержимого окна.
		if w.isTitleBarWidget(child) {
			continue
		}
		child.SetBounds(cb)
	}
	w.layoutNavPanel()
	w.layoutTitleBar()
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
	// Режим вкладок в заголовке: полоса выше (как в Windows Terminal, ~40
	// логических px), чтобы карточки вкладок дышали. Классика Win2000 всё
	// равно ограничена effTitleH=24 — там компактные bevel-ярлыки.
	if w.titleTabsActive() && !w.style().Classic3D {
		return 40
	}
	return 32
}

// effTitleH возвращает ЭФФЕКТИВНУЮ высоту заголовка. В классике Win2000
// заголовок ниже, чем в современных темах (реальный Win2000 ≈18px), поэтому
// ограничиваем его 24px — так кнопки и глифы вписываются в пропорции референса.
// В остальных темах эффективная высота совпадает с titleH().
func (w *Window) effTitleH() int {
	th := w.titleH()
	if th == 0 {
		return 0
	}
	if w.style().Classic3D && th > 24 {
		return 24
	}
	return th
}

// style возвращает стиль темы этого окна: свой, если тема окну назначалась,
// иначе общий. В отличие от currentStyle() отвечает одинаково и во время
// отрисовки, и при обработке ввода.
func (w *Window) style() ThemeStyle {
	if w.ownStyle != nil {
		return *w.ownStyle
	}
	return currentStyle()
}

// OpaqueRegion реализует OpaqueRegioner: что окно закрывает непрозрачно.
//
// Окно заливает свои границы целиком (Draw начинается именно с этого),
// поэтому закрывает оно всё, кроме скруглённых углов. Полупрозрачный фон не
// закрывает ничего: под ним видно то, что лежит ниже, и рисовать это надо.
func (w *Window) OpaqueRegion() []image.Rectangle {
	if w.Background.A < 255 {
		return nil
	}
	return oneRegion(&w.opaqueBuf, opaqueRect(w.Bounds(), w.CornerRadius))
}

// borderW возвращает ширину рамки (0 для borderless).
func (w *Window) borderW() int {
	if w.Style == WindowStyleNone {
		return 0
	}
	return 1
}

// classicFrameW — толщина объёмной 3D-рамки окна в классике Win2000 (px):
// 1px внешний raised-контур + 3px полоса «лица» (resize-frame) + 1px
// внутренняя утопленная кромка. Заголовок и клиентская область вписаны
// внутрь этой рамки.
const classicFrameW = 5

// frameW возвращает толщину рамки окна с учётом стиля темы: в классике
// Win2000 — толстая 3D-рамка (classicFrameW), в остальных темах — 1px
// (0 для borderless). В отличие от borderW отражает реальный визуальный
// отступ, на который смещаются заголовок и контент.
func (w *Window) frameW() int {
	if w.Style == WindowStyleNone {
		return 0
	}
	if w.style().Classic3D {
		return classicFrameW
	}
	return 1
}

// ContentBounds возвращает клиентскую область — прямоугольник для дочерних виджетов.
// Расположена под заголовком, внутри рамки. В классике Win2000 заголовок сам
// вписан внутрь толстой 3D-рамки, поэтому верхний отступ = рамка + заголовок.
func (w *Window) ContentBounds() image.Rectangle {
	b := w.Bounds()
	th := w.titleH()
	fw := w.frameW()
	top := b.Min.Y + th
	if w.style().Classic3D {
		top = b.Min.Y + fw + w.effTitleH()
	}
	left := b.Min.X + fw
	// Боковая панель занимает левую колонку целиком — клиентская область
	// начинается за ней.
	if nb := w.NavPanelBounds(); !nb.Empty() {
		left = nb.Max.X
	}
	return image.Rect(
		left,
		top,
		b.Max.X-fw,
		b.Max.Y-fw,
	)
}

// ─── Кнопки заголовка: геометрия и hit-test ─────────────────────────────────

const (
	winBtnW    = 46 // ширина кнопки управления (Windows-стиль)
	toolBtnW   = 32 // ширина кнопки для ToolWindow
	macCircleR = 6  // радиус traffic light (macOS)
	macStartX  = 18 // отступ первого кружка от левого края
	macSpacing = 22 // расстояние между центрами кружков
	macHitSlop = 10 // допуск клика по кружку
)

// ─── Resize перетаскиванием краёв ───────────────────────────────────────────

// winEdge — битовая маска задействованных краёв при ресайзе (8 направлений).
type winEdge int

const (
	edgeNone winEdge = 0
	edgeN    winEdge = 1 << iota // верхний край
	edgeS                        // нижний край
	edgeW                        // левый край
	edgeE                        // правый край
)

const (
	winResizeBorder = 6   // ширина полосы-захвата вдоль каждого края, px
	winMinW         = 120 // минимальная ширина окна при ресайзе, px
	winMinH         = 80  // минимальная высота окна при ресайзе, px
)

// titleButton — «взведённая» кнопка заголовка (release-семантика).
type titleButton int

const (
	titleBtnNone titleButton = iota
	titleBtnClose
	titleBtnMin
	titleBtnMax
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

// classicBtnSide возвращает сторону квадратной кнопки заголовка в классике
// Win2000 — чуть меньше эффективной высоты заголовка (в оригинале ≈16×14 при
// заголовке 18; здесь ≈18px при effTitleH=24).
func (w *Window) classicBtnSide() int {
	s := w.effTitleH() - 6
	if s < 10 {
		s = 10
	}
	return s
}

// classicTitleBtnRects возвращает прямоугольники кнопок ×, □, ─ для
// классического стиля Win2000 (общая геометрия для отрисовки и hit-test).
// Кнопки прижаты к правому краю заголовка (внутри рамки) с отступом 2px справа
// и 3px сверху; крест отделён от пары ─ □ зазором 2px. Отсутствующие кнопки —
// пустой Rectangle.
func (w *Window) classicTitleBtnRects() (closeR, maxR, minR image.Rectangle) {
	tb := w.titleBarRect()
	if tb.Empty() {
		return
	}
	const edgePad, topPad, gap = 2, 3, 2
	side := w.classicBtnSide()
	nc := w.btnCount()
	by := tb.Min.Y + topPad
	closeX := tb.Max.X - edgePad - side
	closeR = image.Rect(closeX, by, closeX+side, by+side)
	if nc >= 3 {
		mxX := closeX - gap - side
		maxR = image.Rect(mxX, by, mxX+side, by+side)
		minR = image.Rect(mxX-side, by, mxX, by+side)
	} else if nc == 2 {
		mnX := closeX - gap - side
		minR = image.Rect(mnX, by, mnX+side, by+side)
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
	if w.style().Classic3D {
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
	if w.style().Classic3D {
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
	if w.style().Classic3D {
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

	// ── Дочерние виджеты ────────────────────────────────────────────────────
	w.drawChildren(ctx)

	// Боковая панель поднята под верхний край и закрыла подпись — возвращаем
	// её поверх (см. navhost.go).
	if w.navPanel != nil && th > 0 {
		w.drawTitleCaptionOverNav(ctx)
	}

	// ── Рамка (хром) — ПОВЕРХ детей ─────────────────────────────────────────
	// Рамку рисуем после drawChildren: XAML-контент с абсолютными координатами
	// может доходить до самого края клиентской области и в прежнем порядке
	// «замазывал» полосу рамки (в классике — толстую 3D-рамку, в современных
	// темах — 1px XOR-рамку главного окна по периметру). Overlay/popup-меню
	// детей движок рисует отдельным проходом (DrawOverlay) — они не страдают.
	// Для borderless нативного окна рисуем только боковые линии и низ:
	// верхняя линия не нужна — заголовок уже заполняет верхний край.
	if w.Style != WindowStyleNone {
		if st := w.style(); st.Classic3D {
			// Классика Win2000: толстая объёмная 3D-рамка по всему периметру
			// (заголовок и контент уже вписаны внутрь неё).
			w.drawClassicFrame(ctx, st)
		} else {
			bc := w.resolveColor(w.BorderColor, win10.Border)
			if w.inactive {
				// Рамка неактивного окна светлее (как в Win11).
				bc = mixRGBA(bc, w.Background, 0.5)
			}
			// Главное окно в не-классических темах: контрастная XOR-рамка по всему
			// периметру вместо обычной (на светлом фоне канваса светлая рамка окна
			// сливалась бы).
			if w.MainWindow {
				xc := xorBorderColor(w.xorBorderBase())
				if cr > 0 {
					ctx.DrawRoundBorder(x, y, bw, bh, cr, xc)
				} else {
					ctx.DrawBorder(x, y, bw, bh, xc)
				}
			} else if cr > 0 {
				ctx.DrawRoundBorder(x, y, bw, bh, cr, bc)
			} else {
				// Левая, правая и нижняя линии (без верхней — там заголовок)
				ctx.DrawVLine(x, y+th, bh-th, bc)      // левая (от низа заголовка)
				ctx.DrawVLine(x+bw-1, y+th, bh-th, bc) // правая
				ctx.DrawHLine(x, y+bh-1, bw, bc)       // нижняя
			}
			// Разделитель под заголовком. В режиме вкладок прерывается под
			// корешком активной вкладки — он сливается с клиентской областью.
			if th > 0 {
				if ar := w.titleTabsActiveRect(); !ar.Empty() && ar.Min.X > x && ar.Max.X < x+bw {
					ctx.DrawHLine(x, y+th-1, ar.Min.X+1-x, bc)
					ctx.DrawHLine(ar.Max.X-1, y+th-1, x+bw-ar.Max.X+1, bc)
				} else {
					ctx.DrawHLine(x, y+th-1, bw, bc)
				}
			}
		}
	}
}

// ─── Windows-стиль заголовка ────────────────────────────────────────────────

func (w *Window) drawWinTitleBar(ctx DrawContext) {
	b := w.Bounds()
	// Полоса заголовка: в классике смещена внутрь толстой 3D-рамки, в прочих
	// темах совпадает с верхом окна (геометрия идентична прежней).
	tb := w.titleBarRect()
	x, y, bw := tb.Min.X, tb.Min.Y, tb.Dx()
	// Эффективная высота: в классике заголовок ниже (effTitleH), в остальных
	// темах совпадает с titleH() — геометрия текста/фона/бейджа считается от неё.
	th := w.effTitleH()
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

	// Индикатор локали — рисуем ДО заголовка, чтобы обрезать заголовок
	// эллипсисом по левому краю бейджа (иначе длинный Title в узком окне
	// наезжает на плашку «EN»/«RU»).
	badgeLeft, haveBadge := 0, false
	if w.ShowLocaleIndicator {
		nc0 := w.btnCount()
		rightX := b.Max.X - 8
		if w.style().Classic3D {
			if _, _, minR := w.classicTitleBtnRects(); !minR.Empty() {
				rightX = minR.Min.X - 8
			} else if closeR, _, _ := w.classicTitleBtnRects(); !closeR.Empty() {
				rightX = closeR.Min.X - 8
			}
		} else if nc0 > 0 {
			rightX = b.Max.X - w.btnWidth()*nc0 - 8
		}
		badgeRect := drawLocaleBadge(ctx, rightX, y, th, tc)
		w.setLocaleBadgeRect(badgeRect)
		if !badgeRect.Empty() {
			badgeLeft, haveBadge = badgeRect.Min.X, true
		}
	}

	// Правая граница текста заголовка: левый край бейджа (если есть) либо
	// левый край блока кнопок управления.
	titleRight := b.Max.X
	if haveBadge {
		titleRight = badgeLeft
	} else if nc0 := w.btnCount(); nc0 > 0 {
		if w.style().Classic3D {
			if _, _, minR := w.classicTitleBtnRects(); !minR.Empty() {
				titleRight = minR.Min.X
			} else if closeR, _, _ := w.classicTitleBtnRects(); !closeR.Empty() {
				titleRight = closeR.Min.X
			}
		} else {
			titleRight = b.Max.X - w.btnWidth()*nc0
		}
	}

	// Текст заголовка: вертикально по центру, отступ 12px слева
	// (в классике — жирный, как в Win2000). Обрезаем эллипсисом, только если
	// заголовок реально доходит до titleRight (левый край бейджа/кнопок).
	// Без искусственного зазора: заголовок, который вписывался раньше (до
	// появления бейджа), должен рендериться так же — иначе короткие названия
	// теряли последнюю букву под эллипсис при наличии плашки локали.
	// Начинка приложения стоит правее подписи — подпись обрезаем по её левому
	// краю: иначе длинный заголовок наехал бы на чужой виджет.
	if w.titleBarContent != nil {
		if cl := w.titleContentLeft() - titleBarGap; cl < titleRight {
			titleRight = cl
		}
	}

	textX := x + 12
	if w.navBtn != nil && !w.navBtn.bounds.Empty() {
		textX = w.navBtn.bounds.Max.X + titleBarGap
	}
	textY := y + (th-13)/2
	if w.navBtn != nil {
		w.navBtn.fg = tc
	}
	if w.titleTabsActive() {
		// Режим вкладок: полосу заголовка занимают вкладки, текст Title
		// не рисуется. В классике старт после отступа иконки, потолок —
		// эффективная высота заголовка.
		w.drawTitleTabs(ctx, x+8, titleRight-4, y, th)
	} else {
		title := w.Title
		if titleMaxW := titleRight - textX; titleMaxW <= 0 {
			title = ""
		} else {
			title = ellipsizeText(ctx, title, titleMaxW, DefaultFontSizePt)
		}
		drawTitleText(ctx, title, textX, textY, tc)
	}

	nc := w.btnCount()
	if nc == 0 {
		return
	}

	// Классика Win2000: кнопки управления — выпуклые bevel-кнопки на «лице»
	// с чёрными глифами (иначе светло-серые глифы сливаются с navy-заголовком).
	if st := w.style(); st.Classic3D {
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

// drawClassicFrame рисует толстую объёмную 3D-рамку окна Win2000. Устройство
// изнутри наружу совпадает с настоящей Windows 2000:
//   - внешний 1px raised-контур: BevelLight сверху/слева, BevelDark снизу/справа;
//   - 3px полоса «лица» (BtnBG) — визуальный resize-frame (уже залита фоном окна);
//   - внутренняя 1px утопленная кромка: BevelShadow сверху/слева, BevelLight
//     снизу/справа — лёгкое углубление к клиентской области.
func (w *Window) drawClassicFrame(ctx DrawContext, st ThemeStyle) {
	b := w.Bounds()
	x, y, ww, hh := b.Min.X, b.Min.Y, b.Dx(), b.Dy()

	// 1) Внешний raised-контур (самый край окна).
	ctx.DrawHLine(x, y, ww, st.BevelLight)
	ctx.DrawVLine(x, y, hh, st.BevelLight)
	ctx.DrawHLine(x, y+hh-1, ww, st.BevelDark)
	ctx.DrawVLine(x+ww-1, y, hh, st.BevelDark)

	// 2) Полоса «лица» толщиной 3px уже присутствует (фон окна = BtnBG).

	// 3) Внутренняя утопленная кромка на глубине frameW-1.
	in := classicFrameW - 1
	ctx.DrawHLine(x+in, y+in, ww-2*in, st.BevelShadow)
	ctx.DrawVLine(x+in, y+in, hh-2*in, st.BevelShadow)
	ctx.DrawHLine(x+in, y+hh-1-in, ww-2*in, st.BevelLight)
	ctx.DrawVLine(x+ww-1-in, y+in, hh-2*in, st.BevelLight)
}

// drawClassicTitleButtons рисует кнопки ─ □ × в классическом стиле Win2000:
// выпуклые bevel-кнопки цвета «лица» с чёрными пиксельными глифами. Взведённая
// кнопка (armedBtn при зажатой ЛКМ) рисуется ВДАВЛЕННОЙ (bevel sunken + глиф
// смещён на 1px вправо-вниз).
func (w *Window) drawClassicTitleButtons(ctx DrawContext, st ThemeStyle) {
	face := w.resolveColor(win10.BtnBG, win10.PanelBG) // «лицо» Win2000 (#D4D0C8)
	glyph := win10.BtnText                             // чёрный глиф
	closeR, maxR, minR := w.classicTitleBtnRects()

	// draw рисует одну bevel-кнопку и её глиф. kind задаёт тип глифа;
	// вдавленность определяется тем, взведена ли эта кнопка (armedBtn).
	draw := func(r image.Rectangle, kind titleButton) {
		if r.Empty() {
			return
		}
		ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), face)
		pressed := w.armedBtn == kind
		if pressed {
			drawBevelSunken(ctx, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), st)
		} else {
			drawBevelRaised(ctx, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), st)
		}
		cx := r.Min.X + r.Dx()/2
		cy := r.Min.Y + r.Dy()/2
		if pressed { // вдавленная кнопка: глиф смещён вправо-вниз
			cx++
			cy++
		}
		switch kind {
		case titleBtnClose:
			drawGlyphClose(ctx, cx, cy, r.Dx(), glyph)
		case titleBtnMax:
			drawGlyphMax(ctx, cx, cy, r.Dx(), glyph)
		case titleBtnMin:
			drawGlyphMin(ctx, cx, cy, r.Dx(), glyph)
		}
	}

	draw(minR, titleBtnMin)
	draw(maxR, titleBtnMax)
	draw(closeR, titleBtnClose)
}

// drawGlyphClose рисует крест ✕ толщиной 2px, вписанный в кнопку стороной side,
// с центром (cx, cy). Глиф компактный (~8×8 при кнопке 18px, ≈45% стороны) —
// по пропорциям референсной кнопки Win2000, где крест НЕ занимает всю кнопку.
func drawGlyphClose(ctx DrawContext, cx, cy, side int, col color.RGBA) {
	h := side * 7 / 32 // половина диагонали ≈3px при side=18 → крест ~8×8
	if h < 3 {
		h = 3
	}
	for i := -h; i <= h; i++ {
		ctx.SetPixel(cx+i, cy+i, col)
		ctx.SetPixel(cx+i+1, cy+i, col) // 2-й пиксель по горизонтали → толщина 2px
		ctx.SetPixel(cx+i, cy-i, col)
		ctx.SetPixel(cx+i+1, cy-i, col)
	}
}

// drawGlyphMax рисует значок развёртывания □ — квадрат-рамку 1px (~9×9 при
// кнопке 18px) с утолщённой (2px) верхней гранью, как строка заголовка окна.
func drawGlyphMax(ctx DrawContext, cx, cy, side int, col color.RGBA) {
	gw := side / 2 // ≈9px при side=18
	if gw < 8 {
		gw = 8
	}
	gh := gw
	gx := cx - gw/2
	gy := cy - gh/2
	ctx.DrawBorder(gx, gy, gw, gh, col)
	ctx.DrawHLine(gx, gy+1, gw, col) // утолщённая верхняя грань (2px)
}

// drawGlyphMin рисует значок сворачивания ─ — короткую горизонтальную полоску
// 2px толщиной (~8px ширины при кнопке 18px) у нижней трети кнопки.
func drawGlyphMin(ctx DrawContext, cx, cy, side int, col color.RGBA) {
	bw := side * 8 / 18 // ≈8px при side=18
	if bw < 6 {
		bw = 6
	}
	bx := cx - bw/2
	by := cy + side/6 // ближе к низу кнопки
	ctx.DrawHLine(bx, by, bw, col)
	ctx.DrawHLine(bx, by+1, bw, col)
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

	// Индикатор локали — рисуем ДО заголовка (traffic lights слева в Mac-стиле,
	// бейдж — в правом верхнем углу): нужен левый край бейджа для обрезки.
	badgeLeft, haveBadge := 0, false
	if w.ShowLocaleIndicator {
		badgeRect := drawLocaleBadge(ctx, b.Max.X-8, y, th, tc)
		w.setLocaleBadgeRect(badgeRect)
		if !badgeRect.Empty() {
			badgeLeft, haveBadge = badgeRect.Min.X, true
		}
	}

	// Текст заголовка: по центру, но не залезая на «светофоры» слева и бейдж
	// справа. Обрезаем эллипсисом до доступной ширины и клампим позицию.
	leftLimit := x
	if nc >= 1 {
		leftLimit = x + macStartX + (nc-1)*macSpacing + macCircleR + 8
	}
	rightLimit := b.Max.X - 8
	if haveBadge {
		rightLimit = badgeLeft - 8
	}
	if w.navBtn != nil {
		w.navBtn.fg = tc
	}
	if w.titleTabsActive() {
		// Режим вкладок: после traffic lights — полоса вкладок.
		w.drawTitleTabs(ctx, leftLimit+8, rightLimit, y, th)
		return
	}
	if w.titleBarContent != nil {
		// Начинка приложения занимает полосу после «светофора». Подпись в
		// mac-стиле центрирована и легла бы поверх неё — не рисуем, как и в
		// режиме вкладок.
		return
	}
	textY := y + (th-13)/2
	title := w.Title
	if maxW := rightLimit - leftLimit; maxW <= 0 {
		title = ""
	} else {
		title = ellipsizeText(ctx, title, maxW, 10)
		textW := ctx.MeasureText(title, 10)
		textX := x + (bw-textW)/2
		if textX < leftLimit {
			textX = leftLimit
		}
		if textX+textW > rightLimit {
			textX = rightLimit - textW
		}
		ctx.DrawText(title, textX, textY, tc)
	}
}

// ─── Drag / Capture ─────────────────────────────────────────────────────────

// WantsCapture возвращает true при клике в зоны, требующие захвата мыши:
// кнопки управления (release-семантика — нужен release даже если курсор
// ушёл с кнопки), полосы ресайза по краям и drag-зона заголовка.
// Захват позволяет движку доставлять move/release только этому Window.
func (w *Window) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	pt := image.Pt(e.X, e.Y)
	// Область перетаскивания, объявленная приложением (dialog_chrome.go).
	// Проверяется ДО отказа borderless-окну: у окна без полосы заголовка это
	// единственный способ сообщить движку, где у него шапка.
	if w.dragAreaHit(pt) {
		return true
	}
	if w.Style == WindowStyleNone {
		return false
	}
	// Кнопки управления — захватываем ради release (arm → fire/cancel).
	if pt.In(w.CloseBtnRect()) {
		return true
	}
	if r := w.MinBtnRect(); !r.Empty() && pt.In(r) {
		return true
	}
	if r := w.MaxBtnRect(); !r.Empty() && pt.In(r) {
		return true
	}
	// Полоса ресайза по краю окна (приоритетнее drag заголовка в верхней зоне).
	if w.resizeEdgeAt(e.X, e.Y) != edgeNone {
		return true
	}
	// Клик по вкладке/«×»/«+» в полосе заголовка обрабатывается на нажатии
	// и захвата не требует (иначе капча зависла бы без release-ветки).
	if w.titleTabHitZone(pt) {
		return false
	}
	// Начинка приложения в полосе (поиск, кнопка сворачивания) забирает клик
	// себе: захват окном увёл бы события у неё, и поле в заголовке нельзя было
	// бы ни выделить, ни прокрутить.
	if w.titleBarChildHit(pt) {
		return false
	}
	// Drag за заголовок.
	tb := w.titleBarRect()
	if tb.Empty() || !pt.In(tb) {
		return false
	}
	return true
}

// ─── Resize перетаскиванием краёв ───────────────────────────────────────────

// edgeResizeEnabled сообщает, разрешён ли ресайз перетаскиванием краёв.
// Только для «виртуальных» окон на канвасе (headless): в нативном режиме
// размер меняет ОС, и виджетный ресайз краёв конфликтовал бы с ней. Запрещён
// для NoResize/CanMinimize и borderless-окон (WindowStyleNone).
//
// Признак нативного режима — SetNativeHosted, а не наличие OnDragMove:
// приложение вправе забрать себе перемещение окна (например, чтобы рисовать
// контур или тащить окно по своей сцене), не теряя изменение размера.
func (w *Window) edgeResizeEnabled() bool {
	return w.Resize == ResizeModeCanResize &&
		w.Style != WindowStyleNone &&
		!w.nativeHosted
}

// SetNativeHosted помечает окно как живущее в нативном окне ОС: размер и
// положение ведёт система, поэтому виджетный ресайз за края выключается.
// Вызывается window.Window при подключении окна; приложениям на канвасе
// вызывать не нужно.
func (w *Window) SetNativeHosted(v bool) { w.nativeHosted = v }

// IsNativeHosted сообщает, помечено ли окно как нативно размещённое.
func (w *Window) IsNativeHosted() bool { return w.nativeHosted }

// resizeEdgeAt возвращает край(а) окна под точкой (x, y) для ресайза —
// полоса шириной winResizeBorder вдоль каждой границы bounds, включая углы
// (комбинация двух краёв). edgeNone — точка вне зоны ресайза или ресайз
// запрещён.
func (w *Window) resizeEdgeAt(x, y int) winEdge {
	if !w.edgeResizeEnabled() {
		return edgeNone
	}
	b := w.Bounds()
	if !image.Pt(x, y).In(b) {
		return edgeNone
	}
	// Ширина полосы-захвата: в классике совпадает с толщиной 3D-рамки
	// (frameW), в прочих темах — winResizeBorder.
	m := winResizeBorder
	if w.style().Classic3D {
		m = w.frameW()
	}
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

// Cursor реализует CursorProvider: над полосой ресайза возвращает
// соответствующий resize-курсор, иначе обычную стрелку.
func (w *Window) Cursor(x, y int) Cursor {
	switch w.resizeEdgeAt(x, y) {
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

// applyResize пересчитывает bounds окна по стартовому прямоугольнику и текущей
// дельте мыши, уважая минимальный размер (winMinW×winMinH). Опорный прямоуголь-
// ник фиксирован (resizeStart) — накопления ошибки нет.
func (w *Window) applyResize(x, y int) {
	b := w.resizeStart
	nx0, ny0, nx1, ny1 := b.Min.X, b.Min.Y, b.Max.X, b.Max.Y
	dx := x - w.resizeStartX
	dy := y - w.resizeStartY
	if w.resizeDir&edgeW != 0 {
		nx0 = b.Min.X + dx
	}
	if w.resizeDir&edgeE != 0 {
		nx1 = b.Max.X + dx
	}
	if w.resizeDir&edgeN != 0 {
		ny0 = b.Min.Y + dy
	}
	if w.resizeDir&edgeS != 0 {
		ny1 = b.Max.Y + dy
	}
	// Минимальный размер: публичные MinWidth/MinHeight (0 → дефолт winMinW×winMinH).
	minW := w.MinWidth
	if minW <= 0 {
		minW = winMinW
	}
	minH := w.MinHeight
	if minH <= 0 {
		minH = winMinH
	}
	// Не даём схлопнуть окно, тянем «неподвижный» край.
	if nx1-nx0 < minW {
		if w.resizeDir&edgeW != 0 {
			nx0 = nx1 - minW
		} else {
			nx1 = nx0 + minW
		}
	}
	if ny1-ny0 < minH {
		if w.resizeDir&edgeN != 0 {
			ny0 = ny1 - minH
		} else {
			ny1 = ny0 + minH
		}
	}
	nb := image.Rect(nx0, ny0, nx1, ny1)
	if nb != w.Bounds() {
		// Window.SetBounds сам пересчитает ContentBounds → дочерние виджеты.
		w.SetBounds(nb)
	}
}

// SetCaptureManager сохраняет CaptureManager для освобождения захвата при отпускании.
func (w *Window) SetCaptureManager(cm CaptureManager) {
	w.capMgr = cm
}

// ─── Mouse events ───────────────────────────────────────────────────────────

// OnMouseMove обновляет hover-состояние кнопок заголовка и обрабатывает
// drag/resize.
func (w *Window) OnMouseMove(x, y int) {
	// Изменение размера за край окна
	if w.resizing {
		w.applyResize(x, y)
		return
	}

	// Drag за заголовок
	if w.dragging {
		dx := x - w.dragStartX
		dy := y - w.dragStartY
		if dx == 0 && dy == 0 {
			return
		}
		// Перетаскивание контуром: само окно стоит на месте, двигается только
		// контур-overlay. Итоговое смещение применяется при отпускании.
		if !w.outlineRect.Empty() {
			b := w.Bounds()
			old := w.outlineRect
			w.outlineDX, w.outlineDY = dx, dy
			w.outlineRect = image.Rect(
				w.dragWinX+dx, w.dragWinY+dy,
				w.dragWinX+dx+b.Dx(), w.dragWinY+dy+b.Dy(),
			)
			// Изменились только полосы контура: там, где он был, и там, где
			// он стал. Именно полосы и заявляются — объединение двух рамок,
			// разошедшихся на пять точек, накрывает всё окно целиком, и
			// потребитель перерисовывает эту площадь на каждый шаг мыши.
			w.damageOutline(old)
			w.damageOutline(w.outlineRect)
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
				moved := image.Rect(newX, newY, newX+b.Dx(), newY+b.Dy())
				w.SetBounds(moved)
				// Картинка окна та же, просто в другом месте. Объявляем
				// перенос: потребитель скопирует её у себя вместо того,
				// чтобы принимать заново на каждый шаг мыши.
				NotifyWidgetMove(w, b, moved)
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
	c4 := w.titleTabsMouseMove(x, y)
	if c1 || c2 || c3 || c4 {
		w.Invalidate()
	}
}

// titleBarRect возвращает прямоугольник заголовка (без кнопок управления).
// В классике Win2000 заголовок вписан внутрь толстой 3D-рамки — прямоугольник
// смещён внутрь на её толщину (frameW).
func (w *Window) titleBarRect() image.Rectangle {
	b := w.Bounds()
	th := w.titleH()
	if th == 0 {
		return image.Rectangle{}
	}
	if w.style().Classic3D {
		fw := w.frameW()
		eth := w.effTitleH()
		return image.Rect(b.Min.X+fw, b.Min.Y+fw, b.Max.X-fw, b.Min.Y+fw+eth)
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

	// Отпускание кнопки — завершаем resize / drag / arm.
	if !e.Pressed {
		if w.resizing {
			w.resizing = false
			w.resizeDir = edgeNone
			if w.capMgr != nil {
				w.capMgr.ReleaseCapture()
			}
			return true
		}
		if w.dragging {
			w.dragging = false
			// Контурное перетаскивание: окно переезжает один раз, здесь.
			if !w.outlineRect.Empty() {
				dest := w.outlineRect
				dx, dy := w.outlineDX, w.outlineDY
				w.outlineRect = image.Rectangle{}
				w.outlineDX, w.outlineDY = 0, 0
				from := w.Bounds()
				if w.OnDragMove != nil {
					w.OnDragMove(dx, dy) // нативное окно переносит ОС
				} else if dx != 0 || dy != 0 {
					w.SetBounds(dest) // сам сообщит об объединении старой и новой области
					// Окно не изменилось — оно переехало. Объявляем перенос:
					// потребитель скопирует картинку у себя вместо того,
					// чтобы принимать её заново.
					NotifyWidgetMove(w, from, dest)
				}
				// Гасим контур ровно там, где он был; полная инвалидация
				// скрыла бы от хоста, что окно просто переехало, — а по этому
				// он решает, можно ли обойтись командой копирования.
				notifyRectChanged(dest.Union(from))
			}
			if w.capMgr != nil {
				w.capMgr.ReleaseCapture()
			}
			return true
		}
		// Взведённая кнопка заголовка: колбэк только если курсор всё ещё над
		// той же кнопкой (release в стороне — отмена без вызова).
		if w.armedBtn != titleBtnNone {
			armed := w.armedBtn
			w.armedBtn = titleBtnNone
			w.Invalidate() // кнопка «отпущена» — перерисовать в выпуклом виде
			over := false
			switch armed {
			case titleBtnClose:
				over = pt.In(w.CloseBtnRect())
			case titleBtnMin:
				if r := w.MinBtnRect(); !r.Empty() {
					over = pt.In(r)
				}
			case titleBtnMax:
				if r := w.MaxBtnRect(); !r.Empty() {
					over = pt.In(r)
				}
			}
			if w.capMgr != nil {
				w.capMgr.ReleaseCapture()
			}
			if over {
				switch armed {
				case titleBtnClose:
					if w.OnClose != nil {
						w.OnClose()
					}
				case titleBtnMin:
					if w.OnMinimize != nil {
						w.OnMinimize()
					}
				case titleBtnMax:
					if w.OnMaximize != nil {
						w.OnMaximize()
					}
				}
			}
			return true
		}
		return false
	}

	// Нажатие — «взводим» кнопки управления (колбэк на release). В классике
	// взведённая кнопка рисуется вдавленной — инвалидируем окно для перерисовки.
	if pt.In(w.CloseBtnRect()) {
		w.armedBtn = titleBtnClose
		w.Invalidate()
		return true
	}
	if r := w.MinBtnRect(); !r.Empty() && pt.In(r) {
		w.armedBtn = titleBtnMin
		w.Invalidate()
		return true
	}
	if r := w.MaxBtnRect(); !r.Empty() && pt.In(r) {
		w.armedBtn = titleBtnMax
		w.Invalidate()
		return true
	}

	// Вкладки заголовка: клик по вкладке/«×»/«+» (до начала drag).
	if pt.In(w.titleBarRect()) && w.titleTabsMouseDown(pt) {
		return true
	}

	// Нажатие в полосу ресайза по краю — начинаем изменение размера
	// (проверяется до drag заголовка: верхняя 6px-зона — resize, ниже — drag).
	if dir := w.resizeEdgeAt(e.X, e.Y); dir != edgeNone {
		DismissAll(w) // закрываем dropdown/popup перед resize
		w.resizing = true
		w.resizeDir = dir
		w.resizeStart = w.Bounds()
		w.resizeStartX = e.X
		w.resizeStartY = e.Y
		return true
	}

	// Нажатие на заголовок или на объявленную область — начинаем drag
	if (pt.In(w.titleBarRect()) && !w.titleBarChildHit(pt)) || w.dragAreaHit(pt) {
		DismissAll(w) // закрываем dropdown/popup перед drag
		w.dragging = true
		w.dragStartX = e.X
		w.dragStartY = e.Y
		b := w.Bounds()
		w.dragWinX = b.Min.X
		w.dragWinY = b.Min.Y
		if w.OutlineDrag {
			w.outlineRect = b // непустой прямоугольник = идёт drag контуром
			w.outlineDX, w.outlineDY = 0, 0
		}
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
	if !w.outlineRect.Empty() {
		return true // контур перетаскивания рисуется поверх всего
	}
	w.localeMu.Lock()
	defer w.localeMu.Unlock()
	return w.localeMenuOpen && w.ShowLocaleIndicator
}

// DrawOverlay рисует контур перетаскивания и выпадающий список выбора
// локали поверх всего окна.
func (w *Window) DrawOverlay(ctx DrawContext) {
	w.drawDragOutline(ctx)

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

// damageOutline заявляет области, которые занимает контур.
//
// Пустой контур — четыре тонкие полосы по краям: рамка в две линии плюс
// запас на скругление. Залитый — весь прямоугольник: под заливкой меняется
// каждый пиксель, и дробить её на полосы нечестно.
func (w *Window) damageOutline(r image.Rectangle) {
	if r.Empty() {
		return
	}
	if w.OutlineDragStyle == OutlineDragFilled {
		notifyRectChanged(r)
		return
	}

	// Толщина: две линии рамки плюс скругление, если оно есть — дуга уходит
	// внутрь от края ровно на радиус.
	t := outlineStripThickness
	if cr := w.CornerRadius; cr > t {
		t = cr
	}
	if t*2 >= r.Dy() || t*2 >= r.Dx() {
		// Контур тоньше собственных полос — дробить нечего.
		notifyRectChanged(r)
		return
	}

	notifyRectChanged(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+t))     // верх
	notifyRectChanged(image.Rect(r.Min.X, r.Max.Y-t, r.Max.X, r.Max.Y))     // низ
	notifyRectChanged(image.Rect(r.Min.X, r.Min.Y+t, r.Min.X+t, r.Max.Y-t)) // лево
	notifyRectChanged(image.Rect(r.Max.X-t, r.Min.Y+t, r.Max.X, r.Max.Y-t)) // право
}

// outlineStripThickness — толщина полосы контура: две линии рамки и точка
// запаса на сглаживание.
const outlineStripThickness = 3

// drawDragOutline рисует контур окна при OutlineDrag: полупрозрачная заливка
// в тон хрома плюс сплошная рамка. Пока контур виден, само окно стоит на
// месте — оно переедет при отпускании кнопки.
func (w *Window) drawDragOutline(ctx DrawContext) {
	r := w.outlineRect
	if r.Empty() {
		return
	}
	fill, border := w.outlineColors()
	// Заливка — только по явному запросу: она перекрашивает каждый пиксель
	// под контуром, и по сети это дороже, чем двигать само окно.
	if w.OutlineDragStyle == OutlineDragFilled && fill.A > 0 {
		if da, ok := ctx.(DrawContextAlpha); ok {
			da.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), fill)
		}
	}
	if cr := w.CornerRadius; cr > 0 {
		ctx.DrawRoundBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), cr, border)
	} else {
		ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), border)
	}
	// Вторая линия внутрь — контур виден и на фоне того же тона.
	ctx.DrawBorder(r.Min.X+1, r.Min.Y+1, r.Dx()-2, r.Dy()-2, border)
}

// outlineColors возвращает заливку (alpha-premultiplied) и цвет рамки контура.
// Заливка берётся из поля окна, иначе из темы, иначе считается от цвета рамки;
// рисуется она только при OutlineDragFilled (см. drawDragOutline).
func (w *Window) outlineColors() (fill, border color.RGBA) {
	border = w.resolveColor(w.BorderColor, win10.Border)
	switch {
	case w.OutlineDragFill.A > 0:
		fill = w.OutlineDragFill
	case win10.OutlineDragFill.A > 0:
		fill = win10.OutlineDragFill
	default:
		// Полупрозрачный тон рамки: straight-цвет × alpha (FillRectAlpha
		// ждёт alpha-premultiplied — см. Canvas.FillRectAlpha).
		const a = 56
		fill = color.RGBA{
			R: uint8(int(border.R) * a / 255),
			G: uint8(int(border.G) * a / 255),
			B: uint8(int(border.B) * a / 255),
			A: a,
		}
	}
	return fill, border
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
	// Запоминаем стиль: от него зависит геометрия, а её спрашивают и вне
	// отрисовки — см. Window.ownStyle.
	st := t.Style
	w.ownStyle = &st

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

	// Клиентская область зависит от стиля темы: классика Win2000 — толстая
	// 3D-рамка (5px) и низкий титлбар (24), модерн — 1px и 32. Без перекладки
	// после переключения темы контент до первого ресайза налезал на рамку и
	// титлбар (или отставал от них).
	if b := w.Bounds(); !b.Empty() {
		w.SetBounds(b)
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

// xorBorderColor возвращает XOR-инверсию цвета фона (побитовое НЕ каждого
// канала, alpha = 255). Чистая функция — гарантирует контраст рамки с фоном.
func xorBorderColor(bg color.RGBA) color.RGBA {
	return color.RGBA{R: ^bg.R, G: ^bg.G, B: ^bg.B, A: 255}
}

// xorBorderBase возвращает фактический фон окна для расчёта XOR-рамки.
// Если фон окна полупрозрачный/нулевой (A == 0) — фолбэк на WindowBG темы.
func (w *Window) xorBorderBase() color.RGBA {
	if w.Background.A == 0 {
		return win10.WindowBG
	}
	return w.Background
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
