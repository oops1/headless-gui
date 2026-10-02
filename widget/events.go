// Package widget — типы событий ввода и интерфейсы обработчиков.
//
// Определения хранятся в пакете widget (а не engine), чтобы избежать
// циклических импортов: engine → widget, widget ⊄ engine.
package widget

import "sort"

// ─── Mouse ───────────────────────────────────────────────────────────────────

// MouseButton идентифицирует кнопку мыши.
type MouseButton int

const (
	MouseLeft      MouseButton = 0
	MouseRight     MouseButton = 1
	MouseMiddle    MouseButton = 2
	MouseWheelUp   MouseButton = 3
	MouseWheelDown MouseButton = 4
)

// MouseEvent содержит данные события мыши.
type MouseEvent struct {
	X, Y    int
	Button  MouseButton
	Pressed bool // true = нажата, false = отпущена
	// Mod — модификаторы, зажатые в момент события (Ctrl/Shift/Alt).
	//
	// Без них множественный выбор недостижим: DataGrid умеет и toggle по
	// Ctrl, и диапазон по Shift, но узнать о них было неоткуда — событие
	// несло только координаты и кнопку. Заполняет движок, забирая состояние
	// у бэкенда окна (Engine.SetModifiers).
	Mod KeyMod
}

// MouseMoveHandler реализуется виджетами, реагирующими на перемещение курсора.
type MouseMoveHandler interface {
	OnMouseMove(x, y int)
}

// CursorNowhere — координата, которой движок сообщает виджету «курсора над
// тобой больше нет».
//
// Обычный уход курсора виджет замечает сам: OnMouseMove приходит с точкой вне
// его границ. Но есть случай, когда точка В границах, а курсора над виджетом
// всё равно нет — сверху лежит оверлей: открытое меню, календарь, выпадающий
// список. Раньше кнопка под таким оверлеем исправно подсвечивалась, и сквозь
// стеклянную панель Windows 11 подсветка была видна.
//
// Отдельного «курсор ушёл» в протоколе нет намеренно: любой виджет, считающий
// наведение по своим границам, разберётся с этой точкой сам — она не лежит ни
// в чьих границах. Виджету, которому координата нужна для чего-то кроме
// наведения (позиционирование подсказки), даны CursorIsNowhere и возможность
// такое движение пропустить.
const CursorNowhere = -1 << 30

// CursorIsNowhere сообщает, что движение означает уход курсора, а не позицию.
func CursorIsNowhere(x, y int) bool { return x == CursorNowhere && y == CursorNowhere }

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
	KeyPageUp    KeyCode = 33
	KeyPageDown  KeyCode = 34
	KeyHome      KeyCode = 36
	KeyLeft      KeyCode = 37
	KeyUp        KeyCode = 38
	KeyRight     KeyCode = 39
	KeyDown      KeyCode = 40
	KeyInsert    KeyCode = 45
	KeyDelete    KeyCode = 46
	KeyEnd       KeyCode = 35
	KeyF1        KeyCode = 112
	KeyF2        KeyCode = 113
	KeyF3        KeyCode = 114
	KeyF4        KeyCode = 115
	KeyF5        KeyCode = 116
	KeyF6        KeyCode = 117
	KeyF7        KeyCode = 118
	KeyF8        KeyCode = 119
	KeyF9        KeyCode = 120
	KeyF10       KeyCode = 121
	KeyF11       KeyCode = 122
	KeyF12       KeyCode = 123
	// F13..F24 — продолжение того же ряда виртуальных кодов (0x7C..0x87).
	// Клавиатуры с ними редки, но XAML вправе их назвать, и молчаливое
	// «клавиша неизвестна» здесь хуже неиспользуемой константы.
	KeyF13 KeyCode = 124
	KeyF14 KeyCode = 125
	KeyF15 KeyCode = 126
	KeyF16 KeyCode = 127
	KeyF17 KeyCode = 128
	KeyF18 KeyCode = 129
	KeyF19 KeyCode = 130
	KeyF20 KeyCode = 131
	KeyF21 KeyCode = 132
	KeyF22 KeyCode = 133
	KeyF23 KeyCode = 134
	KeyF24 KeyCode = 135

	// Цифровой ряд: виртуальные коды здесь совпадают с кодом самого символа
	// (0x30..0x39), как их и разбирает разметка (parseKeyName).
	Key0 KeyCode = 48
	Key1 KeyCode = 49
	Key2 KeyCode = 50
	Key3 KeyCode = 51
	Key4 KeyCode = 52
	Key5 KeyCode = 53
	Key6 KeyCode = 54
	Key7 KeyCode = 55
	Key8 KeyCode = 56
	Key9 KeyCode = 57

	// Буквы: код совпадает с заглавной латинской буквой (0x41..0x5A).
	//
	// Важно: это код ФИЗИЧЕСКОЙ клавиши латинской раскладки, а не введённый
	// символ. При русской раскладке клавиша «ы» по-прежнему KeyS, иначе
	// Ctrl+S перестал бы работать, стоило переключить язык.
	KeyA KeyCode = 65
	KeyB KeyCode = 66
	KeyC KeyCode = 67
	KeyD KeyCode = 68
	KeyE KeyCode = 69
	KeyF KeyCode = 70
	KeyG KeyCode = 71
	KeyH KeyCode = 72
	KeyI KeyCode = 73
	KeyJ KeyCode = 74
	KeyK KeyCode = 75
	KeyL KeyCode = 76
	KeyM KeyCode = 77
	KeyN KeyCode = 78
	KeyO KeyCode = 79
	KeyP KeyCode = 80
	KeyQ KeyCode = 81
	KeyR KeyCode = 82
	KeyS KeyCode = 83
	KeyT KeyCode = 84
	KeyU KeyCode = 85
	KeyV KeyCode = 86
	KeyW KeyCode = 87
	KeyX KeyCode = 88
	KeyY KeyCode = 89
	KeyZ KeyCode = 90

	// Клавиша контекстного меню (между правыми Alt и Ctrl).
	KeyMenu KeyCode = 93

	// KeyAlt приходит, когда Alt нажали и отпустили, ничего между ними не
	// нажав, — этим в Windows активируют строку меню.
	//
	// Как модификатор Alt по-прежнему ходит в KeyEvent.Mod и своего события
	// не имеет: сочетание Alt+F — это KeyF с ModAlt, а не две клавиши
	// подряд. Отдельный код нужен ровно для одного жеста, у которого нет
	// второй клавиши.
	KeyAlt KeyCode = 18

	// Цифровая клавиатура (0x60..0x6F). Отдельно от цифрового ряда:
	// калькулятору важно, с какой стороны нажали, а Num Lock делает их
	// то цифрами, то навигацией.
	KeyNumpad0         KeyCode = 96
	KeyNumpad1         KeyCode = 97
	KeyNumpad2         KeyCode = 98
	KeyNumpad3         KeyCode = 99
	KeyNumpad4         KeyCode = 100
	KeyNumpad5         KeyCode = 101
	KeyNumpad6         KeyCode = 102
	KeyNumpad7         KeyCode = 103
	KeyNumpad8         KeyCode = 104
	KeyNumpad9         KeyCode = 105
	KeyMultiply        KeyCode = 106
	KeyAdd             KeyCode = 107
	KeyNumpadSeparator KeyCode = 108
	KeySubtract        KeyCode = 109
	KeyDecimal         KeyCode = 110
	KeyDivide          KeyCode = 111

	// Переключатели и системные клавиши.
	KeyPause       KeyCode = 19
	KeyCapsLock    KeyCode = 20
	KeyPrintScreen KeyCode = 44
	KeyNumLock     KeyCode = 144
	KeyScrollLock  KeyCode = 145

	// OEM-клавиши: те, чей символ зависит от раскладки. Названы по тому,
	// что на них написано в раскладке США, — так же, как у Windows, чьи
	// значения здесь и взяты.
	KeyOemSemicolon  KeyCode = 186 // ;: — VK_OEM_1
	KeyOemPlus       KeyCode = 187 // =+ — одинаков во всех раскладках
	KeyOemComma      KeyCode = 188 // ,<
	KeyOemMinus      KeyCode = 189 // -_
	KeyOemPeriod     KeyCode = 190 // .>
	KeyOemSlash      KeyCode = 191 // /? — VK_OEM_2
	KeyOemTilde      KeyCode = 192 // `~ — VK_OEM_3
	KeyOemOpenBrace  KeyCode = 219 // [{ — VK_OEM_4
	KeyOemBackslash  KeyCode = 220 // \| — VK_OEM_5
	KeyOemCloseBrace KeyCode = 221 // ]} — VK_OEM_6
	KeyOemQuote      KeyCode = 222 // '" — VK_OEM_7
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
	Rune    rune // Unicode-символ для печатаемых клавиш (0 для служебных)
	Mod     KeyMod
	Pressed bool // true = нажата, false = отпущена

	// Repeat — событие от автоповтора удерживаемой клавиши, а не от нового
	// нажатия.
	//
	// Различать их нужно там, где нажатие что-то переключает: удержание
	// Caps Lock не должно гасить и зажигать его десять раз в секунду, а
	// повтор Enter — отправлять форму снова и снова. Для набора текста и
	// движения каретки разницы нет: там повтор и есть то, чего ждут.
	Repeat bool
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

// CollectFocusables собирает все Focusable-виджеты из поддерева w.
// Порядок: по возрастанию TabIndex (WPF TabIndex), при равенстве — порядок DFS
// (стабильная сортировка). Виджеты с TabIndex < 0 исключаются.
func CollectFocusables(w Widget) []Widget {
	var result []Widget
	collectFocusablesDFS(w, &result)
	sort.SliceStable(result, func(i, j int) bool {
		return tabIndexOf(result[i]) < tabIndexOf(result[j])
	})
	return result
}

// tabIndexOf возвращает TabIndex виджета (0, если не задан).
func tabIndexOf(w Widget) int {
	if tip, ok := w.(TabIndexProvider); ok {
		return tip.TabIndex()
	}
	return 0
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
