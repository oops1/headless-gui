// Package window — платформенная абстракция нативного окна.
//
// NativeWindow определяет минимальный набор операций, необходимый GUI-движку:
// создание borderless-окна, обработка ввода, вывод RGBA-буфера на экран.
//
// Реализации:
//
//	native_windows.go  — Win32 API (user32/gdi32), чистый Go без CGO
//	native_linux.go    — X11 через github.com/jezek/xgb, чистый Go без CGO
//	native_darwin.go   — Cocoa через github.com/ebitengine/purego, без CGO
package window

import "image"

// NativeWindow — минимальный интерфейс нативного окна ОС.
//
// Все реализации гарантируют:
//   - Borderless-окно (без нативного chrome)
//   - Обработку мыши и клавиатуры через callback'и
//   - Отрисовку RGBA-буфера в клиентскую область окна
type NativeWindow interface {
	// Create создаёт окно с заданным заголовком и размером.
	// Окно создаётся без нативной рамки (borderless).
	Create(title string, width, height int) error

	// RunEventLoop запускает цикл обработки событий.
	// Блокирует до закрытия окна. ДОЛЖЕН вызываться из главной горутины.
	RunEventLoop() error

	// Close закрывает окно и освобождает ресурсы.
	Close()

	// ── Управление окном ─────────────────────────────────────────────────────

	// SetTitle меняет заголовок окна.
	SetTitle(title string)

	// SetSize меняет размер окна (в logical pixels).
	SetSize(width, height int)

	// GetSize возвращает текущий размер окна (в logical pixels).
	GetSize() (width, height int)

	// SetPosition устанавливает позицию окна на экране.
	SetPosition(x, y int)

	// GetPosition возвращает позицию окна на экране.
	GetPosition() (x, y int)

	// Minimize сворачивает окно в панель задач / dock.
	Minimize()

	// Maximize развёртывает окно на весь экран.
	Maximize()

	// Restore восстанавливает окно после maximize/minimize.
	Restore()

	// IsMaximized возвращает true, если окно развёрнуто.
	IsMaximized() bool

	// ── Рендеринг ────────────────────────────────────────────────────────────

	// BlitRGBA выводит RGBA-буфер в клиентскую область окна.
	// Вызывается из рендер-потока при готовности нового кадра.
	BlitRGBA(img *image.RGBA)

	// ── Callbacks (устанавливаются до RunEventLoop) ──────────────────────────

	// SetOnResize — callback при изменении размера окна (новые width, height).
	SetOnResize(fn func(w, h int))

	// SetOnClose — callback при попытке закрыть окно (Alt+F4, taskbar и т.д.).
	// Если возвращает true — окно закрывается; false — остаётся.
	SetOnClose(fn func() bool)

	// SetOnMouseMove — callback при движении мыши (x, y в клиентских координатах).
	SetOnMouseMove(fn func(x, y int))

	// SetOnMouseButton — callback при нажатии/отпускании кнопки мыши.
	// button: 0=left, 1=right, 2=middle. pressed: true=нажата.
	SetOnMouseButton(fn func(x, y, button int, pressed bool))

	// SetOnKeyDown — callback при нажатии клавиши.
	// vk — виртуальный код клавиши (платформенно-независимый).
	SetOnKeyDown(fn func(vk int))

	// SetOnKeyUp — callback при отпускании клавиши.
	SetOnKeyUp(fn func(vk int))

	// SetOnChar — callback при вводе символа (Unicode rune).
	// Учитывает раскладку, Shift, CapsLock, IME.
	SetOnChar(fn func(r rune))
}

// ─── Виртуальные коды клавиш (платформенно-независимые) ─────────────────────

// VK_* — платформенно-независимые коды клавиш.
// Каждая реализация (Win32/X11/Cocoa) маппит свои коды в эти.
const (
	VK_BACKSPACE = 0x08
	VK_TAB       = 0x09
	VK_ENTER     = 0x0D
	VK_ESCAPE    = 0x1B
	VK_SPACE     = 0x20
	VK_LEFT      = 0x25
	VK_UP        = 0x26
	VK_RIGHT     = 0x27
	VK_DOWN      = 0x28
	VK_DELETE    = 0x2E
	VK_HOME      = 0x24
	VK_END       = 0x23

	// Буквенные клавиши (для Ctrl+C и т.д.)
	VK_A = 0x41
	VK_C = 0x43
	VK_V = 0x56
	VK_X = 0x58
	VK_Z = 0x5A

	// Модификаторы
	VK_SHIFT   = 0x10
	VK_CONTROL = 0x11
	VK_ALT     = 0x12
)
