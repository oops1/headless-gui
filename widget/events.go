// Package widget — типы событий ввода и интерфейсы обработчиков.
//
// Определения хранятся в пакете widget (а не engine), чтобы избежать
// циклических импортов: engine → widget, widget ⊄ engine.
package widget

// ─── Mouse ───────────────────────────────────────────────────────────────────

// MouseButton идентифицирует кнопку мыши.
type MouseButton int

const (
	MouseLeft   MouseButton = 0
	MouseRight  MouseButton = 1
	MouseMiddle MouseButton = 2
)

// MouseEvent содержит данные события мыши.
type MouseEvent struct {
	X, Y    int
	Button  MouseButton
	Pressed bool // true = нажата, false = отпущена
}

// MouseMoveHandler реализуется виджетами, реагирующими на перемещение курсора.
type MouseMoveHandler interface {
	OnMouseMove(x, y int)
}

// MouseClickHandler реализуется виджетами, реагирующими на кнопки мыши.
// Возвращает true, если событие поглощено.
type MouseClickHandler interface {
	OnMouseButton(e MouseEvent) bool
}

// ─── Keyboard ────────────────────────────────────────────────────────────────

// KeyCode — аппаратно-независимый код клавиши.
type KeyCode int

const (
	KeyUnknown   KeyCode = 0
	KeyBackspace KeyCode = 8
	KeyTab       KeyCode = 9
	KeyEnter     KeyCode = 13
	KeyEscape    KeyCode = 27
	KeySpace     KeyCode = 32
	KeyHome      KeyCode = 36
	KeyLeft      KeyCode = 37
	KeyUp        KeyCode = 38
	KeyRight     KeyCode = 39
	KeyDown      KeyCode = 40
	KeyDelete    KeyCode = 46
	KeyEnd       KeyCode = 35
	KeyA         KeyCode = 65
	KeyC         KeyCode = 67
	KeyV         KeyCode = 86
	KeyX         KeyCode = 88
	KeyZ         KeyCode = 90
)

// KeyMod — битовая маска нажатых модификаторов.
type KeyMod int

const (
	ModNone  KeyMod = 0
	ModShift KeyMod = 1 << 0
	ModCtrl  KeyMod = 1 << 1
	ModAlt   KeyMod = 1 << 2
	ModMeta  KeyMod = 1 << 3
)

// KeyEvent содержит данные клавиатурного события.
type KeyEvent struct {
	Code    KeyCode
	Rune    rune   // Unicode-символ для печатаемых клавиш (0 для служебных)
	Mod     KeyMod
	Pressed bool // true = нажата, false = отпущена
}

// KeyHandler реализуется виджетами, принимающими ввод с клавиатуры.
type KeyHandler interface {
	OnKeyEvent(e KeyEvent)
}

// ─── Mouse Capture ──────────────────────────────────────────────────────────

// CaptureRequester реализуется виджетами, которые могут захватить мышь (drag).
// При захвате все события мыши (кнопки + перемещение) идут виджету-захватчику,
// пока он не вызовет ReleaseCapture через CaptureManager.
type CaptureRequester interface {
	// WantsCapture возвращает true, если виджет хочет захватить мышь
	// для данного события нажатия. Проверяется при каждом mousedown
	// у всех предков hit-виджета (от ближайшего к корню).
	WantsCapture(e MouseEvent) bool
}

// CaptureManager — интерфейс для управления захватом мыши.
// Реализуется движком; виджет получает его через SetCaptureManager.
type CaptureManager interface {
	SetCapture(w Widget)
	ReleaseCapture()
}

// CaptureAware реализуется виджетами, которым нужен доступ к CaptureManager.
// Движок вызывает SetCaptureManager при SetRoot.
type CaptureAware interface {
	SetCaptureManager(cm CaptureManager)
}

// ─── Focus ───────────────────────────────────────────────────────────────────

// Focusable реализуется виджетами, способными получать фокус ввода.
type Focusable interface {
	SetFocused(focused bool)
	IsFocused() bool
}

// TabIndexProvider — опциональный интерфейс для виджетов с явным порядком Tab-навигации.
// Виджеты без этого интерфейса получают TabIndex = 0 (обход по порядку DFS в дереве).
// Отрицательный TabIndex исключает виджет из Tab-навигации.
type TabIndexProvider interface {
	TabIndex() int
}

// CollectFocusables собирает все Focusable-виджеты из поддерева w
// в порядке DFS (depth-first, порядок отрисовки).
// Виджеты с TabIndex < 0 исключаются.
func CollectFocusables(w Widget) []Widget {
	var result []Widget
	collectFocusablesDFS(w, &result)
	return result
}

func collectFocusablesDFS(w Widget, out *[]Widget) {
	if _, ok := w.(Focusable); ok {
		// Проверяем TabIndex: отрицательный — пропускаем
		if tip, ok := w.(TabIndexProvider); ok && tip.TabIndex() < 0 {
			// Исключён из Tab-навигации
		} else {
			*out = append(*out, w)
		}
	}
	for _, child := range w.Children() {
		collectFocusablesDFS(child, out)
	}
}
