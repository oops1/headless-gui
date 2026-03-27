// Package window предоставляет нативное OS-окно для GUI-движка headless-gui.
//
// Использует Ebiten v2 — на Windows рендеринг через DirectX 11 (без CGO),
// на Linux/macOS — через OpenGL.
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

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"headless-gui/output"
	"headless-gui/widget"
)

// EngineAPI — интерфейс движка, необходимый для оконного рендеринга.
// Реализуется *engine.Engine.
type EngineAPI interface {
	Frames() <-chan output.Frame
	CanvasSize() (w, h int)
	SendMouseMove(x, y int)
	SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
	SendKeyEvent(e widget.KeyEvent)
}

// Window — нативное окно на базе Ebiten v2.
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

	// Текущий полный кадр (накапливаем dirty-тайлы).
	mu      sync.Mutex
	current *image.RGBA

	// Ebiten-текстура для отрисовки.
	ebitenImg *ebiten.Image

	// Флаг: есть ли обновлённые данные для Draw.
	hasUpdate atomic.Bool

	// Настройки окна.
	maxFPS    int
	resizable bool

	// Предыдущее состояние ввода (для edge-detection).
	prevMX, prevMY int
	prevLMB        bool
	prevRMB        bool
	prevMMB        bool
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

// Run открывает окно и запускает цикл событий Ebiten.
// Блокирует вызывающую горутину до закрытия окна.
// ВАЖНО: вызывать из главной горутины (main).
func (win *Window) Run() error {
	ebiten.SetWindowSize(win.w, win.h)
	ebiten.SetWindowTitle(win.title)
	ebiten.SetTPS(win.maxFPS)

	if win.resizable {
		ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	} else {
		ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	}

	return ebiten.RunGame(win)
}

// ─── ebiten.Game ─────────────────────────────────────────────────────────────

// Update вызывается Ebiten каждый тик (обработка ввода + получение кадров).
func (win *Window) Update() error {
	// ── Читаем все готовые кадры из движка (non-blocking) ──────────────────
	frames := win.eng.Frames()
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				// Канал закрыт — движок остановлен.
				return ebiten.Termination
			}
			win.applyFrame(frame)
			win.hasUpdate.Store(true)
		default:
			goto doneFrames
		}
	}
doneFrames:

	// ── Мышь: движение ─────────────────────────────────────────────────────
	mx, my := ebiten.CursorPosition()
	if mx != win.prevMX || my != win.prevMY {
		win.eng.SendMouseMove(mx, my)
		win.prevMX, win.prevMY = mx, my
	}

	// ── Мышь: кнопки ───────────────────────────────────────────────────────
	lmb := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if lmb != win.prevLMB {
		win.eng.SendMouseButton(mx, my, widget.MouseLeft, lmb)
		win.prevLMB = lmb
	}
	rmb := ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight)
	if rmb != win.prevRMB {
		win.eng.SendMouseButton(mx, my, widget.MouseRight, rmb)
		win.prevRMB = rmb
	}
	mmb := ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle)
	if mmb != win.prevMMB {
		win.eng.SendMouseButton(mx, my, widget.MouseMiddle, mmb)
		win.prevMMB = mmb
	}

	// ── Клавиатура: навигационные клавиши (just-pressed) ───────────────────
	mod := currentModifiers()
	for _, key := range inpututil.AppendJustPressedKeys(nil) {
		code := ebitenKeyCode(key)
		if code != widget.KeyUnknown {
			win.eng.SendKeyEvent(widget.KeyEvent{
				Code:    code,
				Rune:    0,
				Mod:     mod,
				Pressed: true,
			})
		}
	}
	for _, key := range inpututil.AppendJustReleasedKeys(nil) {
		code := ebitenKeyCode(key)
		if code != widget.KeyUnknown {
			win.eng.SendKeyEvent(widget.KeyEvent{
				Code:    code,
				Rune:    0,
				Mod:     mod,
				Pressed: false,
			})
		}
	}

	// ── Клавиатура: ввод символов (Unicode, учитывает раскладку) ──────────
	// AppendInputChars даёт реальные символы с учётом Shift, CapsLock, IME.
	for _, r := range ebiten.AppendInputChars(nil) {
		if r >= 32 {
			win.eng.SendKeyEvent(widget.KeyEvent{
				Code:    widget.KeyUnknown,
				Rune:    r,
				Mod:     mod,
				Pressed: true,
			})
		}
	}

	return nil
}

// Draw вызывается Ebiten для отрисовки кадра на экране.
func (win *Window) Draw(screen *ebiten.Image) {
	if !win.hasUpdate.Swap(false) && win.ebitenImg != nil {
		// Нет новых данных — рисуем предыдущий кадр.
		screen.DrawImage(win.ebitenImg, nil)
		return
	}

	win.mu.Lock()
	pix := make([]byte, len(win.current.Pix))
	copy(pix, win.current.Pix)
	win.mu.Unlock()

	if win.ebitenImg == nil {
		win.ebitenImg = ebiten.NewImage(win.w, win.h)
	}
	win.ebitenImg.WritePixels(pix)
	screen.DrawImage(win.ebitenImg, nil)
}

// Layout возвращает логический размер холста.
func (win *Window) Layout(outsideW, outsideH int) (int, int) {
	return win.w, win.h
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
			dstOff := win.current.PixOffset(tile.X, tile.Y+row)
			copy(win.current.Pix[dstOff:dstOff+rowBytes], tile.Data[srcOff:srcOff+rowBytes])
		}
	}
}

// ─── Маппинг ввода ───────────────────────────────────────────────────────────

// ebitenKeyCode переводит ebiten.Key в widget.KeyCode.
// Возвращает KeyUnknown для печатаемых символов — они обрабатываются через AppendInputChars.
func ebitenKeyCode(k ebiten.Key) widget.KeyCode {
	switch k {
	case ebiten.KeyBackspace:
		return widget.KeyBackspace
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		return widget.KeyEnter
	case ebiten.KeyEscape:
		return widget.KeyEscape
	case ebiten.KeyTab:
		return widget.KeyTab
	case ebiten.KeySpace:
		return widget.KeySpace
	case ebiten.KeyArrowLeft:
		return widget.KeyLeft
	case ebiten.KeyArrowRight:
		return widget.KeyRight
	case ebiten.KeyArrowUp:
		return widget.KeyUp
	case ebiten.KeyArrowDown:
		return widget.KeyDown
	case ebiten.KeyHome:
		return widget.KeyHome
	case ebiten.KeyEnd:
		return widget.KeyEnd
	case ebiten.KeyDelete:
		return widget.KeyDelete

	// Ctrl-комбинации (A, C, V, X, Z) — отправляем как KeyCode.
	// Движок проверяет Mod&ModCtrl чтобы не путать с обычным вводом.
	case ebiten.KeyA:
		return widget.KeyA
	case ebiten.KeyC:
		return widget.KeyC
	case ebiten.KeyV:
		return widget.KeyV
	case ebiten.KeyX:
		return widget.KeyX
	case ebiten.KeyZ:
		return widget.KeyZ
	}
	return widget.KeyUnknown
}

// currentModifiers возвращает текущие модификаторы клавиатуры.
func currentModifiers() widget.KeyMod {
	var mod widget.KeyMod
	if ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight) {
		mod |= widget.ModCtrl
	}
	if ebiten.IsKeyPressed(ebiten.KeyShift) || ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) {
		mod |= widget.ModShift
	}
	if ebiten.IsKeyPressed(ebiten.KeyAlt) || ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight) {
		mod |= widget.ModAlt
	}
	return mod
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
