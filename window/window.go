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
	"sync"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// EngineAPI — интерфейс движка, необходимый для оконного рендеринга.
// Реализуется *engine.Engine.
type EngineAPI interface {
	Frames() <-chan output.Frame
	CanvasSize() (w, h int)
	Root() widget.Widget
	SendMouseMove(x, y int)
	SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
	SendKeyEvent(e widget.KeyEvent)
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
	eng   EngineAPI
	title string
	w, h  int

	native NativeWindow

	// Текущий полный кадр (накапливаем dirty-тайлы).
	mu      sync.Mutex
	current *image.RGBA

	// Флаг: запрошено закрытие окна (кнопка ×).
	closeRequested atomic.Bool

	// Настройки окна.
	maxFPS    int
	resizable bool

	// Состояние модификаторов (обновляется в onKeyDown/onKeyUp).
	modShift   atomic.Bool
	modCtrl    atomic.Bool
	modAlt     atomic.Bool

	// Предыдущие координаты мыши (для drag).
	lastMX, lastMY int
}

// New создаёт окно для заданного движка с указанным заголовком.
// Размер окна берётся из CanvasSize() движка.
func New(eng EngineAPI, title string) *Window {
	w, h := eng.CanvasSize()
	return &Window{
		eng:     eng,
		title:   title,
		w:       w,
		h:       h,
		current: image.NewRGBA(image.Rect(0, 0, w, h)),
		maxFPS:  60,
	}
}

// SetMaxFPS задаёт максимальный FPS отрисовки окна (по умолчанию 60).
func (win *Window) SetMaxFPS(fps int) *Window {
	win.maxFPS = fps
	return win
}

// SetResizable разрешает/запрещает изменение размера окна пользователем.
func (win *Window) SetResizable(v bool) *Window {
	win.resizable = v
	return win
}

// Run открывает нативное окно и запускает цикл событий.
// Блокирует вызывающую горутину до закрытия окна.
// ВАЖНО: вызывать из главной горутины (main).
func (win *Window) Run() error {
	win.native = NewNativeWindow()

	// Если корень — widget.Window, синхронизируем параметры:
	// нативное окно получает размер, заголовок и resizable из XAML,
	// а widget.Window получает bounds = (0,0)-(w,h) нативного окна.
	win.syncFromWidgetWindow()

	// Создаём окно с актуальными размерами
	if err := win.native.Create(win.title, win.w, win.h); err != nil {
		return err
	}

	// Подключаем виджет-окно (drag, close, minimize, maximize)
	win.setupWidgetWindow()

	// Подключаем callbacks ввода
	win.setupInputCallbacks()

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
	ww.OnDragMove = func(dx, dy int) {
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

// setupInputCallbacks подключает callback'и ввода от нативного окна к движку.
func (win *Window) setupInputCallbacks() {
	// ── Resize ───────────────────────────────────────────────────────────────
	win.native.SetOnResize(func(newW, newH int) {
		if newW <= 0 || newH <= 0 {
			return
		}
		win.w = newW
		win.h = newH

		// Пересоздаём буфер
		win.mu.Lock()
		win.current = image.NewRGBA(image.Rect(0, 0, newW, newH))
		win.mu.Unlock()

		// Обновляем размер canvas движка
		if rs, ok := win.eng.(interface{ SetResolution(w, h int) }); ok {
			rs.SetResolution(newW, newH)
		}

		// Обновляем bounds корневого виджета (widget.Window заполняет всё окно)
		if root := win.eng.Root(); root != nil {
			root.SetBounds(image.Rect(0, 0, newW, newH))
		}
	})

	// ── Close ────────────────────────────────────────────────────────────────
	win.native.SetOnClose(func() bool {
		win.closeRequested.Store(true)
		return true // разрешаем закрытие
	})

	// ── Mouse move ───────────────────────────────────────────────────────────
	win.native.SetOnMouseMove(func(x, y int) {
		win.lastMX = x
		win.lastMY = y
		win.eng.SendMouseMove(x, y)
	})

	// ── Mouse buttons ────────────────────────────────────────────────────────
	win.native.SetOnMouseButton(func(x, y, button int, pressed bool) {
		win.lastMX = x
		win.lastMY = y

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
		win.eng.SendMouseButton(x, y, btn, pressed)
	})

	// ── Key down ─────────────────────────────────────────────────────────────
	win.native.SetOnKeyDown(func(vk int) {
		// Обновляем модификаторы
		switch vk {
		case VK_SHIFT:
			win.modShift.Store(true)
		case VK_CONTROL:
			win.modCtrl.Store(true)
		case VK_ALT:
			win.modAlt.Store(true)
		}

		code := vkToKeyCode(vk)
		if code != widget.KeyUnknown {
			win.eng.SendKeyEvent(widget.KeyEvent{
				Code:    code,
				Rune:    0,
				Mod:     win.currentMod(),
				Pressed: true,
			})
		}
	})

	// ── Key up ───────────────────────────────────────────────────────────────
	win.native.SetOnKeyUp(func(vk int) {
		// Обновляем модификаторы
		switch vk {
		case VK_SHIFT:
			win.modShift.Store(false)
		case VK_CONTROL:
			win.modCtrl.Store(false)
		case VK_ALT:
			win.modAlt.Store(false)
		}

		code := vkToKeyCode(vk)
		if code != widget.KeyUnknown {
			win.eng.SendKeyEvent(widget.KeyEvent{
				Code:    code,
				Rune:    0,
				Mod:     win.currentMod(),
				Pressed: false,
			})
		}
	})

	// ── Char (Unicode символ) ────────────────────────────────────────────────
	win.native.SetOnChar(func(r rune) {
		if r >= 32 {
			win.eng.SendKeyEvent(widget.KeyEvent{
				Code:    widget.KeyUnknown,
				Rune:    r,
				Mod:     win.currentMod(),
				Pressed: true,
			})
		}
	})
}

// framePump читает кадры из движка и отправляет на отрисовку.
// Запускается в отдельной горутине.
func (win *Window) framePump() {
	frames := win.eng.Frames()
	for frame := range frames {
		win.applyFrame(frame)

		// Отправляем текущий буфер на отрисовку
		win.mu.Lock()
		snap := image.NewRGBA(win.current.Bounds())
		copy(snap.Pix, win.current.Pix)
		win.mu.Unlock()

		win.native.BlitRGBA(snap)
	}
}

// currentMod возвращает текущие модификаторы клавиатуры.
func (win *Window) currentMod() widget.KeyMod {
	var mod widget.KeyMod
	if win.modCtrl.Load() {
		mod |= widget.ModCtrl
	}
	if win.modShift.Load() {
		mod |= widget.ModShift
	}
	if win.modAlt.Load() {
		mod |= widget.ModAlt
	}
	return mod
}

// ─── Внутренние методы ───────────────────────────────────────────────────────

// applyFrame накладывает dirty-тайлы кадра на текущий буфер.
func (win *Window) applyFrame(frame output.Frame) {
	win.mu.Lock()
	defer win.mu.Unlock()
	for _, tile := range frame.Tiles {
		rowBytes := tile.W * 4
		for row := 0; row < tile.H; row++ {
			srcOff := row * rowBytes
			dstY := tile.Y + row
			if dstY >= win.current.Bounds().Dy() {
				break
			}
			dstOff := win.current.PixOffset(tile.X, dstY)
			dstEnd := dstOff + rowBytes
			if dstEnd > len(win.current.Pix) {
				break
			}
			copy(win.current.Pix[dstOff:dstEnd], tile.Data[srcOff:srcOff+rowBytes])
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
	case VK_DELETE:
		return widget.KeyDelete
	case VK_HOME:
		return widget.KeyHome
	case VK_END:
		return widget.KeyEnd
	case VK_A:
		return widget.KeyA
	case VK_C:
		return widget.KeyC
	case VK_V:
		return widget.KeyV
	case VK_X:
		return widget.KeyX
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
