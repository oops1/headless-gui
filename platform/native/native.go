// native.go — контракт нативного окна NativeWindow и платформенно-независимые
// коды клавиш VK_*.
//
// Бэкенды могут дополнительно реализовать необязательные интерфейсы, которые
// окно (window/core) проверяет утверждением типа: dirtyRectBlitter — тогда
// окно передаёт им только изменившуюся область кадра, filesDropTarget — приём
// файлов, перетащенных из ОС (Win32 WM_DROPFILES, X11 XDND, Wayland
// wl_data_device), и другие. Пока бэкенды живут в window/core, эти интерфейсы
// объявлены там; сюда они переезжают вместе с бэкендами (ADR-0013).

package native

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

	// SetResizable разрешает пользователю менять размер окна за края.
	// Окна создаются borderless, поэтому зоны захвата у краёв реализует
	// бэкенд (Win32 — WM_NCHITTEST). Бэкенды без поддержки — no-op.
	SetResizable(v bool)

	// SetMinSize задаёт минимальный размер окна В ФИЗИЧЕСКИХ пикселях
	// (вызывающая сторона умножает логический минимум на HiDPI-scale).
	// Win32 отдаёт его ОС в WM_GETMINMAXINFO; прочие бэкенды — no-op.
	// 0 → дефолт движка (Win32: 320×240).
	SetMinSize(width, height int)

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

	// SetCornerRadius задаёт радиус скругления углов окна (0 = прямые).
	// На платформах без поддержки — no-op.
	SetCornerRadius(r int)

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
	// button: 0=left, 1=right, 2=middle, 3/4=тик колеса вверх/вниз, 5=X1
	// («назад»), 6=X2 («вперёд»). pressed: true=нажата.
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
	VK_INSERT    = 0x2D
	VK_DELETE    = 0x2E
	VK_HOME      = 0x24
	VK_END       = 0x23
	VK_PRIOR     = 0x21 // Page Up
	VK_NEXT      = 0x22 // Page Down

	// Буквенные клавиши (для Ctrl+C и т.д.)
	VK_A = 0x41
	VK_C = 0x43
	VK_V = 0x56
	VK_X = 0x58
	VK_Y = 0x59
	VK_Z = 0x5A

	// Функциональные клавиши F1–F12
	VK_F1  = 0x70
	VK_F2  = 0x71
	VK_F3  = 0x72
	VK_F4  = 0x73
	VK_F5  = 0x74
	VK_F6  = 0x75
	VK_F7  = 0x76
	VK_F8  = 0x77
	VK_F9  = 0x78
	VK_F10 = 0x79
	VK_F11 = 0x7A
	VK_F12 = 0x7B

	// Модификаторы
	VK_SHIFT   = 0x10
	VK_CONTROL = 0x11
	VK_ALT     = 0x12

	// Буквы и цифры целиком: отображение клавиш идёт диапазонами, и
	// перечислять их по одной незачем — нужны только границы.
	VK_0 = 0x30
	VK_9 = 0x39

	// Буквы целиком: таблицы раскладок перечисляют клавиши поимённо, и
	// писать там 0x42 вместо VK_B значит заставлять читателя считать в уме.
	VK_B = 0x42
	VK_D = 0x44
	VK_E = 0x45
	VK_F = 0x46
	VK_G = 0x47
	VK_H = 0x48
	VK_I = 0x49
	VK_J = 0x4A
	VK_K = 0x4B
	VK_L = 0x4C
	VK_M = 0x4D
	VK_N = 0x4E
	VK_O = 0x4F
	VK_P = 0x50
	VK_Q = 0x51
	VK_R = 0x52
	VK_S = 0x53
	VK_T = 0x54
	VK_U = 0x55
	VK_W = 0x57

	// Клавиша контекстного меню и переключатели.
	VK_APPS     = 0x5D
	VK_LWIN     = 0x5B // левая клавиша Windows (Super_L)
	VK_RWIN     = 0x5C // правая клавиша Windows (Super_R)
	VK_PAUSE    = 0x13
	VK_CAPITAL  = 0x14 // Caps Lock
	VK_SNAPSHOT = 0x2C // Print Screen
	VK_NUMLOCK  = 0x90
	VK_SCROLL   = 0x91

	// Цифровая клавиатура.
	VK_NUMPAD0   = 0x60
	VK_NUMPAD1   = 0x61
	VK_NUMPAD2   = 0x62
	VK_NUMPAD3   = 0x63
	VK_NUMPAD4   = 0x64
	VK_NUMPAD5   = 0x65
	VK_NUMPAD6   = 0x66
	VK_NUMPAD7   = 0x67
	VK_NUMPAD8   = 0x68
	VK_NUMPAD9   = 0x69
	VK_MULTIPLY  = 0x6A
	VK_ADD       = 0x6B
	VK_SEPARATOR = 0x6C
	VK_SUBTRACT  = 0x6D
	VK_DECIMAL   = 0x6E
	VK_DIVIDE    = 0x6F

	// Остальные функциональные клавиши ряда (F13–F24).
	VK_F13 = 0x7C
	VK_F24 = 0x87

	// Клавиши, дошедшие до приложения только в v3.39 (раньше их не было в
	// таблицах бэкендов, и событие терялось).
	VK_CLEAR      = 0x0C
	VK_KANA       = 0x15 // Kana / Hangul
	VK_JUNJA      = 0x17
	VK_FINAL      = 0x18
	VK_KANJI      = 0x19 // Kanji / Hanja
	VK_CONVERT    = 0x1C
	VK_NONCONVERT = 0x1D
	VK_ACCEPT     = 0x1E
	VK_MODECHANGE = 0x1F
	VK_SLEEP      = 0x5F
	VK_LMENU      = 0xA4
	VK_PROCESSKEY = 0xE5 // клавиша ушла редактору ввода (IME)

	// Браузерные и мультимедийные клавиши, подряд (0xA6..0xB7).
	VK_BROWSER_BACK      = 0xA6
	VK_BROWSER_FORWARD   = 0xA7
	VK_BROWSER_REFRESH   = 0xA8
	VK_BROWSER_STOP      = 0xA9
	VK_BROWSER_SEARCH    = 0xAA
	VK_BROWSER_FAVORITES = 0xAB
	VK_BROWSER_HOME      = 0xAC
	VK_VOLUME_MUTE       = 0xAD
	VK_VOLUME_DOWN       = 0xAE
	VK_VOLUME_UP         = 0xAF
	VK_MEDIA_NEXT_TRACK  = 0xB0
	VK_MEDIA_PREV_TRACK  = 0xB1
	VK_MEDIA_STOP        = 0xB2
	VK_MEDIA_PLAY_PAUSE  = 0xB3
	VK_LAUNCH_MAIL       = 0xB4
	VK_LAUNCH_MEDIA      = 0xB5
	VK_LAUNCH_APP1       = 0xB6
	VK_LAUNCH_APP2       = 0xB7

	// OEM-клавиши: символ зависит от раскладки, код — нет.
	VK_OEM_1      = 0xBA // ;:
	VK_OEM_PLUS   = 0xBB // =+
	VK_OEM_COMMA  = 0xBC // ,<
	VK_OEM_MINUS  = 0xBD // -_
	VK_OEM_PERIOD = 0xBE // .>
	VK_OEM_2      = 0xBF // /?
	VK_OEM_3      = 0xC0 // `~
	VK_OEM_4      = 0xDB // [{
	VK_OEM_5      = 0xDC // \|
	VK_OEM_6      = 0xDD // ]}
	VK_OEM_7      = 0xDE // '"
)
