// Package window предоставляет нативное OS-окно для GUI-движка headless-gui.
//
// Использует собственные нативные бэкенды:
//   - Windows: Win32 API (user32/gdi32), чистый Go без CGO
//   - Linux:   X11 протокол напрямую через Unix socket, без CGO
//   - macOS:   Cocoa через purego (Objective-C runtime), без CGO
//
// Использование:
//
//	eng := engine.New(1920, 1080, 30)
//	eng.SetRoot(buildUI())
//	eng.Start()
//
//	win := window.New(eng, "My App")
//	win.Run() // блокирует до закрытия окна
//
//	eng.Stop()
package window

import (
	"image"
	stdraw "image/draw"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// localeProvider — опциональная возможность нативного окна сообщать и
// переключать раскладку клавиатуры ОС. Реализуется бэкендами Windows/Linux;
// на платформах без поддержки локаль управляется только из приложения.
type localeProvider interface {
	// CurrentLocaleCode возвращает код активной раскладки ("EN","RU",…) или "".
	CurrentLocaleCode() string
	// AvailableLocaleCodes возвращает установленные в ОС раскладки.
	AvailableLocaleCodes() []string
	// ActivateLocaleCode переключает раскладку ОС; true при успехе.
	ActivateLocaleCode(code string) bool
}

// EngineAPI — интерфейс движка, необходимый для оконного рендеринга.
// Реализуется *engine.Engine.
type EngineAPI interface {
	Frames() <-chan output.Frame
	CanvasSize() (w, h int)
	Root() widget.Widget
	SendMouseMove(x, y int)
	SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
	SendKeyEvent(e widget.KeyEvent)
	CursorAt(x, y int) widget.Cursor
}

// engineScaler — опциональная поддержка HiDPI движком (реализует *engine.Engine).
// CanvasSize при этом логический, кадры и события — физические.
type engineScaler interface {
	SetScale(k float64)
	Scale() float64
	PhysicalSize() (w, h int)
}

// scaleDetector — опциональная возможность бэкенда сообщить системный
// масштаб монитора (Win32: DPI после включения per-monitor awareness).
type scaleDetector interface {
	DetectScale() float64
}

// dpiChangeNotifier — опциональное уведомление о смене DPI монитора
// (перетаскивание окна между мониторами; Win32 WM_DPICHANGED).
type dpiChangeNotifier interface {
	SetOnDpiChanged(fn func(scale float64))
}

// activationNotifier — опциональное уведомление о смене активности окна
// (Win32 WM_ACTIVATE, X11 FocusIn/FocusOut). widget.Window рисует
// неактивный заголовок приглушённым (Win2000 — серый градиент).
type activationNotifier interface {
	SetOnActivate(fn func(active bool))
}

// setupActivation подключает активность нативного окна к widget.Window.
func (win *Window) setupActivation() {
	an, ok := win.native.(activationNotifier)
	if !ok {
		return
	}
	ww, ok := win.eng.Root().(*widget.Window)
	if !ok {
		return
	}
	an.SetOnActivate(func(active bool) {
		ww.SetActive(active)
		// При деактивации носителя (клик в другое приложение) закрываем
		// вынесенные popup-оверлеи — как системные меню. Только в hosted-режиме.
		if !active && win.popupHost != nil {
			if c, ok := win.eng.(interface{ CloseAllOverlays() }); ok {
				c.CloseAllOverlays()
			}
		}
	})
}

// installPopupHost регистрирует хост popup-оверлеев, если бэкенд умеет окна-
// попапы и маршалинг на UI-поток, а движок принимает PopupSink. На бэкендах без
// поддержки (Wayland/macOS) и в headless — no-op: оверлеи рисуются в холст.
func (win *Window) installPopupHost() {
	if _, ok := win.native.(popupWindow); !ok {
		return
	}
	inv, ok := win.native.(uiThreadInvoker)
	if !ok {
		return
	}
	peng, ok := win.eng.(popupEngine)
	if !ok {
		return
	}
	setter, ok := win.eng.(interface {
		SetPopupSink(sink func(frames []engine.PopupFrame))
	})
	if !ok {
		return
	}
	win.popupHost = newPopupHost(win.native, inv, peng, win.scale)
	setter.SetPopupSink(win.popupHost.apply)
}

// detectScale возвращает HiDPI-масштаб: env HEADLESS_GUI_SCALE имеет
// приоритет (ручное управление на X11/macOS), иначе — от бэкенда.
func detectScale(native NativeWindow) float64 {
	if v := os.Getenv("HEADLESS_GUI_SCALE"); v != "" {
		if k, err := strconv.ParseFloat(v, 64); err == nil && k >= 0.5 && k <= 4 {
			return k
		}
	}
	if sd, ok := native.(scaleDetector); ok {
		if k := sd.DetectScale(); k > 0 {
			return k
		}
	}
	return 1
}

// physicalSize возвращает физический размер кадра движка
// (или логический, если движок не поддерживает HiDPI).
func (win *Window) physicalSize() (int, int) {
	if sa, ok := win.eng.(engineScaler); ok {
		return sa.PhysicalSize()
	}
	return win.eng.CanvasSize()
}

// surface — общий слой рендер-насоса и ввода: связывает движок (EngineAPI),
// нативное окно и HiDPI-масштаб. Его переиспользуют главное окно (Window его
// встраивает) и вторичное окно модального диалога (dialogHost) — чтобы не
// копировать насос кадров и проброс ввода. Оконно-специфичная логика
// (resize/close/minimize/maximize, активность, локаль, смена DPI) живёт в
// Window; surface её не касается.
type surface struct {
	eng    EngineAPI
	native NativeWindow

	// HiDPI: масштаб монитора (1.0 без HiDPI). Логический размер живёт у
	// владельца; нативное окно и буфер кадра — физические (логические × scale).
	scale float64

	// Текущий полный кадр (накапливаем dirty-тайлы).
	mu      sync.Mutex
	current *image.RGBA
	// pendingDirty — объединение областей тайлов, применённых с последнего
	// блита; сбрасывается после отправки в нативное окно.
	pendingDirty image.Rectangle

	// Состояние модификаторов (обновляется в onKeyDown/onKeyUp).
	modShift atomic.Bool
	modCtrl  atomic.Bool
	modAlt   atomic.Bool

	// Предыдущие координаты мыши (для drag).
	lastMX, lastMY int
}

// Window — нативное окно ОС для GUI-движка.
//
// Жизненный цикл:
//
//	win := window.New(eng, "Заголовок")
//	win.SetMaxFPS(60)      // опционально
//	win.SetResizable(true) // опционально
//	win.Run()              // блокирует до закрытия окна
type Window struct {
	surface

	title string
	w, h  int

	// Флаг: запрошено закрытие окна (кнопка ×).
	closeRequested atomic.Bool

	// Настройки окна.
	maxFPS       int
	resizable    bool
	cornerRadius int // скругление углов окна (0 = прямые); применяется после Create

	// popupHost — хост popup-оверлеев (dropdown/меню в собственных окнах ОС).
	// nil, если бэкенд не поддерживает окна-попапы (Wayland/macOS → in-canvas).
	popupHost *popupHost

	// ── Трей и уведомления (Windows) ─────────────────────────────────────────
	// Сеттеры трея вызываются до Run() (native ещё nil), поэтому желаемое
	// состояние буферизуется и применяется в Run() после создания окна
	// (applyPendingTray). На платформах без поддержки (native не trayHost) —
	// тихий no-op. См. tray.go / tray_windows.go.
	trayIcon          image.Image
	trayTooltip       string
	trayIconWant      bool
	trayMenu          *widget.PopupMenu
	onTrayClick       func(button widget.MouseButton, doubleClick bool)
	onBalloonClick    func()
	trayDispatcherSet bool
}

// New создаёт окно для заданного движка с указанным заголовком.
// Размер окна берётся из CanvasSize() движка.
func New(eng EngineAPI, title string) *Window {
	w, h := eng.CanvasSize()
	win := &Window{
		title:  title,
		w:      w,
		h:      h,
		maxFPS: 60,
	}
	win.eng = eng
	win.current = image.NewRGBA(image.Rect(0, 0, w, h))
	return win
}

// SetMaxFPS задаёт максимальный FPS отрисовки окна (по умолчанию 60).
func (win *Window) SetMaxFPS(fps int) *Window {
	win.maxFPS = fps
	return win
}

// SetResizable разрешает/запрещает изменение размера окна пользователем.
func (win *Window) SetResizable(v bool) *Window {
	win.resizable = v
	if win.native != nil {
		win.native.SetResizable(v)
	}
	return win
}

// Close программно закрывает окно: Run() вернётся, приложение завершится
// штатно (аналог нажатия ×). Безопасно вызывать из обработчиков UI;
// до Run() — no-op.
func (win *Window) Close() {
	win.closeRequested.Store(true)
	if win.native != nil {
		win.native.Close()
	}
}

// SetCornerRadius задаёт скругление углов нативного окна (0 = прямые).
// Удобно вызывать при смене темы: win.SetCornerRadius(theme.Style.WindowCorner).
// До Run() запоминается и применяется после создания окна.
func (win *Window) SetCornerRadius(r int) {
	win.cornerRadius = r
	if win.native != nil {
		win.native.SetCornerRadius(r)
	}
}

// Run открывает нативное окно и запускает цикл событий.
// Блокирует вызывающую горутину до закрытия окна.
// ВАЖНО: вызывать из главной горутины (main).
func (win *Window) Run() error {
	win.native = NewNativeWindow()

	// HiDPI: определяем масштаб монитора (env HEADLESS_GUI_SCALE или
	// бэкенд) и сообщаем движку ДО расчёта размеров окна.
	win.scale = 1
	if sa, ok := win.eng.(engineScaler); ok {
		if k := detectScale(win.native); k != 1 {
			sa.SetScale(k)
		}
		win.scale = sa.Scale()
	}

	// Если корень — widget.Window, синхронизируем параметры:
	// нативное окно получает размер, заголовок и resizable из XAML,
	// а widget.Window получает bounds = (0,0)-(w,h) нативного окна.
	win.syncFromWidgetWindow()

	// Создаём окно ФИЗИЧЕСКОГО размера (логический × scale).
	pw, ph := win.physicalSize()
	win.mu.Lock()
	win.current = image.NewRGBA(image.Rect(0, 0, pw, ph))
	win.mu.Unlock()
	if err := win.native.Create(win.title, pw, ph); err != nil {
		return err
	}
	// Разрешаем ресайз за края (borderless: зоны рамки отдаёт бэкенд).
	win.native.SetResizable(win.resizable)

	// Минимальный размер окна из widget.Window (MinWidth/MinHeight): логические
	// значения × HiDPI-scale → физические пиксели для ОС (Win32 WM_GETMINMAXINFO).
	if ww, ok := win.eng.Root().(*widget.Window); ok && (ww.MinWidth > 0 || ww.MinHeight > 0) {
		pmw := int(float64(ww.MinWidth)*win.scale + 0.5)
		pmh := int(float64(ww.MinHeight)*win.scale + 0.5)
		win.native.SetMinSize(pmw, pmh)
	}

	// Смена DPI монитора (перенос окна) — перестраиваем масштаб и буферы.
	if dn, ok := win.native.(dpiChangeNotifier); ok {
		dn.SetOnDpiChanged(func(k float64) {
			sa, ok := win.eng.(engineScaler)
			if !ok || k <= 0 || k == win.scale {
				return
			}
			sa.SetScale(k)
			win.scale = k
			npw, nph := win.physicalSize()
			win.mu.Lock()
			win.current = image.NewRGBA(image.Rect(0, 0, npw, nph))
			win.mu.Unlock()
			win.native.SetSize(npw, nph)
		})
	}

	// Применяем скругление углов, если задано до Create.
	if win.cornerRadius > 0 {
		win.native.SetCornerRadius(win.cornerRadius)
	}

	// Подключаем виджет-окно (drag, close, minimize, maximize)
	win.setupWidgetWindow()

	// Оконно-специфичные callbacks (resize, close).
	win.setupResizeClose()

	// Общий проброс ввода (мышь/клавиатура) — surface.
	win.setupInput()

	// Перерисовка по WM_PAINT/Expose из кэша последнего кадра.
	win.setupExposeRedraw()

	// Активность окна (фокус ОС) → приглушённый заголовок widget.Window.
	win.setupActivation()

	// Синхронизация локали с раскладкой клавиатуры ОС (Windows/Linux).
	win.setupLocaleSync()

	// Хост нативных модалок: если бэкенд поддерживает owner-окна (Win32) и
	// движок принимает хост — модалки будут открываться в собственных окнах.
	win.installModalHost()

	// Хост popup-оверлеев: если бэкенд умеет окна-попапы (Win32/X11) — dropdown'ы
	// и меню будут открываться в собственных окнах ОС и выходить за границы окна.
	win.installPopupHost()

	// Применяем отложенное состояние трея/уведомлений (иконка, меню, колбэки),
	// заданное до Run(). На платформах без поддержки — no-op.
	win.applyPendingTray()

	// Запускаем горутину чтения кадров из движка
	go win.framePump()

	// Блокирующий цикл событий (возврат = окно закрыто)
	return win.native.RunEventLoop()
}

// syncFromWidgetWindow считывает параметры из widget.Window (XAML <Window>)
// и синхронизирует их с нативным окном.
// Вызывается до Create() — чтобы нативное окно создалось с правильными размерами.
func (win *Window) syncFromWidgetWindow() {
	root := win.eng.Root()
	if root == nil {
		return
	}
	ww, ok := root.(*widget.Window)
	if !ok {
		return
	}

	// ── Заголовок из XAML (если не задан вручную через New) ──────────────
	if ww.Title != "" && ww.Title != "Caption" {
		win.title = ww.Title
	}

	// ── Размер из XAML <Window Width="..." Height="..."> ────────────────
	b := ww.Bounds()
	if b.Dx() > 0 && b.Dy() > 0 {
		win.w = b.Dx()
		win.h = b.Dy()
	}

	// ── Скругление окна из widget.Window.CornerRadius (задаётся темой) ───
	if ww.CornerRadius > 0 && win.cornerRadius == 0 {
		win.cornerRadius = ww.CornerRadius
	}

	// ── Обновляем canvas движка под размер widget.Window ─────────────────
	if rs, ok := win.eng.(interface{ SetResolution(w, h int) }); ok {
		rs.SetResolution(win.w, win.h)
	}

	// ── Пересоздаём буфер ───────────────────────────────────────────────
	win.mu.Lock()
	win.current = image.NewRGBA(image.Rect(0, 0, win.w, win.h))
	win.mu.Unlock()

	// ── widget.Window bounds = полная область нативного окна (0,0)-(w,h)
	// Это ключевой момент: виджет должен занимать всё окно.
	ww.SetBounds(image.Rect(0, 0, win.w, win.h))

	// ── ResizeMode → resizable ──────────────────────────────────────────
	switch ww.Resize {
	case widget.ResizeModeCanResize:
		win.resizable = true
	case widget.ResizeModeNoResize:
		win.resizable = false
	case widget.ResizeModeCanMinimize:
		win.resizable = false
	}
}

// setupWidgetWindow подключает drag/close/minimize/maximize
// если корневой виджет — widget.Window.
func (win *Window) setupWidgetWindow() {
	root := win.eng.Root()
	if root == nil {
		return
	}
	ww, ok := root.(*widget.Window)
	if !ok {
		return
	}

	// Drag за заголовок → перемещение нативного окна.
	// dx/dy приходят в логических пикселях (виджет), позиция окна — физическая.
	ww.OnDragMove = func(dx, dy int) {
		if win.scale != 1 {
			dx = int(float64(dx)*win.scale + 0.5)
			dy = int(float64(dy)*win.scale + 0.5)
		}
		x, y := win.native.GetPosition()
		win.native.SetPosition(x+dx, y+dy)
	}

	// Кнопка × → закрытие.
	if ww.OnClose == nil {
		ww.OnClose = func() {
			win.closeRequested.Store(true)
			win.native.Close()
		}
	}

	// Кнопка ─ → свернуть.
	if ww.OnMinimize == nil {
		ww.OnMinimize = func() {
			win.native.Minimize()
		}
	}

	// Кнопка □ → развернуть / восстановить.
	if ww.OnMaximize == nil {
		ww.OnMaximize = func() {
			if win.native.IsMaximized() {
				win.native.Restore()
			} else {
				win.native.Maximize()
			}
		}
	}
}

// setupResizeClose подключает оконно-специфичные callback'и: изменение
// размера окна (пересоздание канваса/буфера) и запрос закрытия (кнопка ×).
// Проброс мыши/клавиатуры — в surface.setupInput.
func (win *Window) setupResizeClose() {
	// ── Resize ───────────────────────────────────────────────────────────────
	// newW/newH — ФИЗИЧЕСКИЕ пиксели от ОС; движок и виджеты живут
	// в логических (округление по масштабу).
	win.native.SetOnResize(func(newW, newH int) {
		if newW <= 0 || newH <= 0 {
			return
		}
		lw, lh := newW, newH
		if win.scale != 1 && win.scale > 0 {
			lw = int(float64(newW)/win.scale + 0.5)
			lh = int(float64(newH)/win.scale + 0.5)
		}
		win.w = lw
		win.h = lh

		// Обновляем размер canvas движка (логический).
		if rs, ok := win.eng.(interface{ SetResolution(w, h int) }); ok {
			rs.SetResolution(lw, lh)
		}

		// Пересоздаём буфер под физический размер кадра.
		pw, ph := win.physicalSize()
		win.mu.Lock()
		win.current = image.NewRGBA(image.Rect(0, 0, pw, ph))
		win.mu.Unlock()

		// Обновляем bounds корневого виджета (widget.Window заполняет всё окно)
		if root := win.eng.Root(); root != nil {
			root.SetBounds(image.Rect(0, 0, lw, lh))
		}
	})

	// ── Close ────────────────────────────────────────────────────────────────
	win.native.SetOnClose(func() bool {
		win.closeRequested.Store(true)
		return true // разрешаем закрытие
	})
}

// setupInput подключает проброс ввода (мышь/клавиатура/символы) от нативного
// окна к движку. Общий для главного окна и вторичных окон модалок.
func (s *surface) setupInput() {
	// ── Mouse move ───────────────────────────────────────────────────────────
	s.native.SetOnMouseMove(func(x, y int) {
		s.lastMX = x
		s.lastMY = y
		s.eng.SendMouseMove(x, y)
		// Обновляем форму курсора под указателем (если бэкенд поддерживает).
		if sc, ok := s.native.(interface{ SetCursor(c int) }); ok {
			sc.SetCursor(int(s.eng.CursorAt(x, y)))
		}
	})

	// ── Mouse buttons ────────────────────────────────────────────────────────
	s.native.SetOnMouseButton(func(x, y, button int, pressed bool) {
		s.lastMX = x
		s.lastMY = y

		var btn widget.MouseButton
		switch button {
		case 0:
			btn = widget.MouseLeft
		case 1:
			btn = widget.MouseRight
		case 2:
			btn = widget.MouseMiddle
		case 3:
			btn = widget.MouseWheelUp
		case 4:
			btn = widget.MouseWheelDown
		default:
			return
		}
		s.eng.SendMouseButton(x, y, btn, pressed)
	})

	// ── Precise wheel (пиксельная дельта колеса/тачпада) ─────────────────────
	// Опционально: бэкенды с высокоточным колесом (Win32 WM_MOUSEWHEEL,
	// Wayland wl_pointer.axis) вызывают этот колбэк вместо тиковых кнопок 3/4.
	// Бэкенды без поддержки (X11) продолжают слать тики через SetOnMouseButton.
	if pw, ok := s.native.(interface {
		SetOnMouseWheelPixels(fn func(x, y int, dx, dy float64))
	}); ok {
		pw.SetOnMouseWheelPixels(func(x, y int, dx, dy float64) {
			s.lastMX = x
			s.lastMY = y
			if we, ok := s.eng.(interface {
				SendMouseWheelPixels(x, y int, dx, dy float64)
			}); ok {
				we.SendMouseWheelPixels(x, y, dx, dy)
			}
		})
	}

	// ── Key down ─────────────────────────────────────────────────────────────
	s.native.SetOnKeyDown(func(vk int) {
		// Обновляем модификаторы
		switch vk {
		case VK_SHIFT:
			s.modShift.Store(true)
		case VK_CONTROL:
			s.modCtrl.Store(true)
		case VK_ALT:
			s.modAlt.Store(true)
		}

		code := vkToKeyCode(vk)
		if code != widget.KeyUnknown {
			s.eng.SendKeyEvent(widget.KeyEvent{
				Code:    code,
				Rune:    0,
				Mod:     s.currentMod(),
				Pressed: true,
			})
		}
	})

	// ── Key up ───────────────────────────────────────────────────────────────
	s.native.SetOnKeyUp(func(vk int) {
		// Обновляем модификаторы
		switch vk {
		case VK_SHIFT:
			s.modShift.Store(false)
		case VK_CONTROL:
			s.modCtrl.Store(false)
		case VK_ALT:
			s.modAlt.Store(false)
		}

		code := vkToKeyCode(vk)
		if code != widget.KeyUnknown {
			s.eng.SendKeyEvent(widget.KeyEvent{
				Code:    code,
				Rune:    0,
				Mod:     s.currentMod(),
				Pressed: false,
			})
		}
	})

	// ── Char (Unicode символ) ────────────────────────────────────────────────
	s.native.SetOnChar(func(r rune) {
		if r >= 32 {
			s.eng.SendKeyEvent(widget.KeyEvent{
				Code:    widget.KeyUnknown,
				Rune:    r,
				Mod:     s.currentMod(),
				Pressed: true,
			})
		}
	})
}

// setupLocaleSync подключает раскладку клавиатуры ОС к индикатору локали.
//
// Если нативное окно поддерживает localeProvider:
//   - заполняет список доступных локалей (для контекстного меню);
//   - выставляет начальную локаль из активной раскладки ОС;
//   - регистрирует applier — выбор в меню переключает раскладку ОС;
//   - запускает поллер: переключение системной комбинацией клавиш
//     отражается на индикаторе автоматически.
//
// В headless-режиме (без нативного окна) этот код не вызывается — там
// источником истины остаётся widget.SetLocale из приложения.
func (win *Window) setupLocaleSync() {
	lp, ok := win.native.(localeProvider)
	if !ok {
		return
	}
	codes := lp.AvailableLocaleCodes()
	if len(codes) > 0 {
		widget.SetAvailableLocales(codes)
	}
	if cur := lp.CurrentLocaleCode(); cur != "" {
		widget.SetLocale(cur)
	} else if len(codes) > 0 {
		// ОС не сообщает активную раскладку (напр. Linux без XKB-привязок) —
		// берём первую доступную как начальную.
		widget.SetLocale(codes[0])
	}
	widget.SetLocaleApplier(func(code string) bool {
		return lp.ActivateLocaleCode(code)
	})
	go win.localePoll(lp)
}

// localePoll периодически опрашивает раскладку ОС и отражает её на индикаторе.
// Останавливается при закрытии окна.
func (win *Window) localePoll(lp localeProvider) {
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		if win.closeRequested.Load() {
			return
		}
		if cur := lp.CurrentLocaleCode(); cur != "" {
			widget.SetLocale(cur) // no-op, если не изменилась
		}
	}
}

// dirtyRectBlitter — опциональная возможность нативного бэкенда выводить
// только изменившуюся область кадра (вместо полного BlitRGBA).
// Контракт: первый вызов после изменения размера буфера всегда получает
// полный прямоугольник (framePump это гарантирует).
type dirtyRectBlitter interface {
	BlitRGBADirty(img *image.RGBA, dirty image.Rectangle)
}

// exposeNotifier — опциональная возможность бэкенда сообщать, что ОС просит
// перерисовать область окна (WM_PAINT / X11 Expose: окно было перекрыто,
// свёрнуто и т.п.). Окно отвечает блитом из кэша последнего кадра — не
// дожидаясь нового кадра от движка (при статичном UI его может не быть).
type exposeNotifier interface {
	SetOnExpose(fn func(r image.Rectangle))
}

// setupExposeRedraw регистрирует перерисовку по запросу ОС из s.current.
// macOS не требует этого: содержимое CALayer ретейнится композитором.
func (s *surface) setupExposeRedraw() {
	en, ok := s.native.(exposeNotifier)
	if !ok {
		return
	}
	en.SetOnExpose(func(r image.Rectangle) {
		s.mu.Lock()
		cur := s.current
		s.mu.Unlock()

		r = r.Intersect(cur.Bounds())
		if r.Empty() {
			return
		}
		if db, ok := s.native.(dirtyRectBlitter); ok && r != cur.Bounds() {
			db.BlitRGBADirty(cur, r)
		} else {
			s.native.BlitRGBA(cur)
		}
	})
}

// framePump читает кадры из движка и отправляет на отрисовку.
// Запускается в отдельной горутине.
//
// Полное копирование кадра перед блитом не делается: applyFrame и блит
// выполняются последовательно в этой же горутине, а при resize старый буфер
// остаётся валидным для чтения (заменяется целиком, не мутируется).
func (s *surface) framePump() {
	frames := s.eng.Frames()
	db, hasDirtyBlit := s.native.(dirtyRectBlitter)
	var lastBounds image.Rectangle // границы буфера на момент прошлого блита

	for frame := range frames {
		s.applyFrame(frame)

		s.mu.Lock()
		cur := s.current
		dirty := s.pendingDirty
		s.pendingDirty = image.Rectangle{}
		s.mu.Unlock()

		full := cur.Bounds()
		if full != lastBounds {
			// Размер сменился (resize) — нативный буфер должен быть
			// заполнен целиком, частичный блит недопустим.
			dirty = full
			lastBounds = full
		}
		dirty = dirty.Intersect(full)
		if dirty.Empty() {
			continue
		}
		if hasDirtyBlit && dirty != full {
			db.BlitRGBADirty(cur, dirty)
		} else {
			s.native.BlitRGBA(cur)
		}
	}
}

// currentMod возвращает текущие модификаторы клавиатуры.
func (s *surface) currentMod() widget.KeyMod {
	var mod widget.KeyMod
	if s.modCtrl.Load() {
		mod |= widget.ModCtrl
	}
	if s.modShift.Load() {
		mod |= widget.ModShift
	}
	if s.modAlt.Load() {
		mod |= widget.ModAlt
	}
	return mod
}

// ─── Внутренние методы ───────────────────────────────────────────────────────

// applyFrame накладывает dirty-тайлы кадра на текущий буфер и копит
// объединение их областей в pendingDirty (для частичного блита).
func (s *surface) applyFrame(frame output.Frame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tile := range frame.Tiles {
		s.pendingDirty = s.pendingDirty.Union(
			image.Rect(tile.X, tile.Y, tile.X+tile.W, tile.Y+tile.H))
		rowBytes := tile.W * 4
		for row := 0; row < tile.H; row++ {
			srcOff := row * rowBytes
			dstY := tile.Y + row
			if dstY >= s.current.Bounds().Dy() {
				break
			}
			dstOff := s.current.PixOffset(tile.X, dstY)
			dstEnd := dstOff + rowBytes
			if dstEnd > len(s.current.Pix) {
				break
			}
			copy(s.current.Pix[dstOff:dstEnd], tile.Data[srcOff:srcOff+rowBytes])
		}
	}
}

// ─── Маппинг VK → widget.KeyCode ────────────────────────────────────────────

// vkToKeyCode переводит VK_* код в widget.KeyCode.
// VK_* константы специально совпадают с widget.KeyCode, поэтому маппинг прямой.
func vkToKeyCode(vk int) widget.KeyCode {
	switch vk {
	case VK_BACKSPACE:
		return widget.KeyBackspace
	case VK_TAB:
		return widget.KeyTab
	case VK_ENTER:
		return widget.KeyEnter
	case VK_ESCAPE:
		return widget.KeyEscape
	case VK_SPACE:
		return widget.KeySpace
	case VK_LEFT:
		return widget.KeyLeft
	case VK_UP:
		return widget.KeyUp
	case VK_RIGHT:
		return widget.KeyRight
	case VK_DOWN:
		return widget.KeyDown
	case VK_INSERT:
		return widget.KeyInsert
	case VK_DELETE:
		return widget.KeyDelete
	case VK_HOME:
		return widget.KeyHome
	case VK_END:
		return widget.KeyEnd
	case VK_PRIOR:
		return widget.KeyPageUp
	case VK_NEXT:
		return widget.KeyPageDown
	case VK_A:
		return widget.KeyA
	case VK_C:
		return widget.KeyC
	case VK_V:
		return widget.KeyV
	case VK_X:
		return widget.KeyX
	case VK_Y:
		return widget.KeyY
	case VK_Z:
		return widget.KeyZ
	}
	return widget.KeyUnknown
}

// ─── Утилиты ─────────────────────────────────────────────────────────────────

// FullFrameSnapshot возвращает копию текущего полного кадра (для скриншота и т.п.).
func (win *Window) FullFrameSnapshot() *image.RGBA {
	win.mu.Lock()
	defer win.mu.Unlock()
	snap := image.NewRGBA(win.current.Bounds())
	stdraw.Draw(snap, snap.Bounds(), win.current, image.Point{}, stdraw.Src)
	return snap
}

// Native возвращает нативное окно для прямого доступа (опционально).
func (win *Window) Native() NativeWindow {
	return win.native
}
