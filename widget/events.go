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

	// Боковые кнопки «назад/вперёд» (v3.34). X1 — «назад» (Win32 XBUTTON1,
	// X11 кнопка 8, Linux BTN_SIDE), X2 — «вперёд» (XBUTTON2, кнопка 9,
	// BTN_EXTRA). Виджеты, которым они не нужны, их игнорируют: все
	// стандартные виджеты реагируют только на MouseLeft; до корня дерева
	// событие всплывает как любое непоглощённое.
	MouseButtonX1 MouseButton = 5
	MouseButtonX2 MouseButton = 6
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

	// Clicks — номер нажатия в серии: 1 — одиночное, 2 — двойное, 3 — тройное
	// и так далее. Отпускание кнопки несёт номер своего нажатия.
	//
	// Считает движок, один раз на всё дерево: до этого двойной щелчок
	// высматривал каждый виджет сам — своим полем «время прошлого клика»,
	// своим порогом и своей меркой «мышь не сдвинулась», — и тройного
	// щелчка не умел никто. Интервал движок берёт системный, то есть тот,
	// который человек выставил в параметрах мыши.
	//
	// 0 — события пришли не от движка (прямой вызов OnMouseButton в тестах
	// или в приложении со своей доставкой): виджету тогда считать самому.
	Clicks int
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

// PointerLeaveHandler реализуется виджетами, которым нужно знать, что указатель
// покинул ОКНО целиком (v3.39).
//
// Отличие от OnMouseMove: обычный уход за границу виджета и уход с окна —
// одно и то же движение в точку вне виджета, и по одной точке их не различить
// (мышь, захваченная при перетаскивании, тоже бывает снаружи). Здесь событие
// одно и только про окно: Win32 WM_MOUSELEAVE, X11 LeaveNotify, Wayland
// wl_pointer.leave. Прежний сигнал (движение в точку вне окна) приходит как и
// раньше, этот — дополнительный.
//
// Зовётся на горутине движка, всем видимым виджетам дерева, которые его
// реализуют, — не только тому, над которым был указатель. Нужен, например,
// клиенту удалённого рабочего стола: отпустить захват и не считать курсор
// «внутри» сессии.
type PointerLeaveHandler interface {
	OnPointerLeave()
}

// ─── Wheel ───────────────────────────────────────────────────────────────────

// WheelNotch — сколько единиц WheelEvent.DeltaX/DeltaY приходится на один
// щелчок колеса мыши (WHEEL_DELTA в Windows, MOUSE_WHEEL в RDP).
const WheelNotch = 120.0

// WheelEvent — сырое событие колеса или тачпада (v3.39).
//
// Существует рядом с прежним путём (OnMouseWheelPixels / тики кнопок), а не
// вместо него: тот отдаёт ПИКСЕЛИ под шаг прокрутки движка, а удалённому
// рабочему столу нужна дельта оборудования как есть. Виджет, реализующий
// WheelEventHandler и принявший событие, останавливает всплытие — тогда
// пиксельный путь для этого события не запускается вовсе.
type WheelEvent struct {
	// X, Y — точка под указателем в логических координатах кадра виджета.
	X, Y int

	// DeltaX, DeltaY — прокрутка в единицах WheelNotch (120 на щелчок).
	// Знаки те же, что у пиксельного пути движка: DeltaY > 0 — вниз (к
	// пользователю), DeltaX > 0 — вправо. (Windows и RDP считают вертикаль
	// наоборот: «вверх» там положительно, то есть wheel = −DeltaY.)
	//
	// Обычная мышь даёт кратные 120 значения; тачпад и колесо с плавной
	// прокруткой — дробные, и малые.
	DeltaX, DeltaY float64

	// Precise — дельта «высокоточная» (тачпад, свободно вращающееся колесо):
	// произвольная, не обязательно кратна WheelNotch, и приходит часто.
	// false — щелчки колеса: целое число по 120.
	Precise bool

	// Mod — модификаторы, зажатые в момент прокрутки.
	Mod KeyMod
}

// WheelEventHandler реализуется виджетами, которым нужна сырая дельта колеса.
// Возвращает true, если событие поглощено (всплытие и пиксельный путь
// останавливаются).
type WheelEventHandler interface {
	OnWheelEvent(e WheelEvent) bool
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

	// KeyWin — клавиша Windows (Super, Meta, «Пуск»). Код — VK_LWIN (0x5B);
	// правую клавишу (VK_RWIN, 0x5C) бэкенды сводят к тому же коду: для
	// приложения важно, что нажали «Win», а не с какой стороны.
	//
	// Как модификатор клавиша по-прежнему ходит в KeyEvent.Mod (ModMeta);
	// отдельное событие нужно оболочке, которой одиночное нажатие Win
	// открывает «Пуск».
	KeyWin KeyCode = 91

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

	// ── Добавлено в v3.39 (значения прежних констант не менялись) ───────────
	//
	// Коды продолжают тот же ряд виртуальных кодов Windows. Они нужны, чтобы
	// клавиша, которой раньше не было в перечислении, дошла до приложения
	// именованным событием, а не пропала: удалённому рабочему столу важна
	// каждая нажатая клавиша. Событие с кодом KeyUnknown и нулевой руной —
	// клавиша без имени; её опознаёт KeyEvent.ScanCode.

	// Модификаторы как клавиши. Раньше Shift/Ctrl/Alt жили только в
	// KeyEvent.Mod, а сами нажатия до виджетов не доходили; клиенту удалённого
	// рабочего стола нужны именно они — со стороной (левый/правый) в ScanCode
	// и Extended. KeyAlt остаётся жестом «Alt нажали и отпустили»; удержание
	// самой клавиши Alt приходит как KeyAltKey.
	KeyShift   KeyCode = 16  // Shift (любой из двух)
	KeyControl KeyCode = 17  // Ctrl (любой из двух)
	KeyAltKey  KeyCode = 164 // физическая клавиша Alt: нажатие и отпускание (VK_LMENU)

	KeyClear KeyCode = 12 // Clear (цифровая клавиатура без Num Lock на Windows)

	// Восточноазиатские клавиши переключения ввода.
	KeyKana       KeyCode = 21 // Kana / Hangul (VK_KANA = VK_HANGUL)
	KeyJunja      KeyCode = 23
	KeyFinal      KeyCode = 24
	KeyKanji      KeyCode = 25 // Kanji / Hanja (VK_KANJI = VK_HANJA)
	KeyConvert    KeyCode = 28 // Henkan
	KeyNonConvert KeyCode = 29 // Muhenkan
	KeyAccept     KeyCode = 30
	KeyModeChange KeyCode = 31
	KeySleep      KeyCode = 95

	// Браузерные и мультимедийные клавиши (VK_BROWSER_* .. VK_LAUNCH_APP2).
	KeyBrowserBack      KeyCode = 166
	KeyBrowserForward   KeyCode = 167
	KeyBrowserRefresh   KeyCode = 168
	KeyBrowserStop      KeyCode = 169
	KeyBrowserSearch    KeyCode = 170
	KeyBrowserFavorites KeyCode = 171
	KeyBrowserHome      KeyCode = 172
	KeyVolumeMute       KeyCode = 173
	KeyVolumeDown       KeyCode = 174
	KeyVolumeUp         KeyCode = 175
	KeyMediaNext        KeyCode = 176
	KeyMediaPrev        KeyCode = 177
	KeyMediaStop        KeyCode = 178
	KeyMediaPlayPause   KeyCode = 179
	KeyLaunchMail       KeyCode = 180
	KeyLaunchMedia      KeyCode = 181
	KeyLaunchApp1       KeyCode = 182
	KeyLaunchApp2       KeyCode = 183

	// KeyIMEProcess — клавиша ушла редактору ввода (VK_PROCESSKEY): символ,
	// который из неё получится, придёт отдельным текстовым событием.
	KeyIMEProcess KeyCode = 229
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

	// ScanCode — сырой код физической клавиши от операционной системы (v3.39).
	// 0 — события пришли не от окна ОС (синтетические, тесты, символы набора).
	//
	// Значение НЕ приведено к единой таблице, оно зависит от платформы:
	//   - Windows: биты 16–23 lParam (аппаратный скан-код набора 1; признак
	//     «расширенной» клавиши — в Extended);
	//   - X11: keycode − 8, то есть код evdev (Linux input-event-codes.h);
	//   - Wayland: код из wl_keyboard.key, тоже evdev;
	//   - macOS: keyCode события NSEvent (kVK_*).
	// Клиенту, которому нужен скан-код набора 1 (RDP, VNC-подобные), таблицу
	// «evdev/kVK → набор 1» держать у себя: она зависит от протокола, а не
	// от окна. Приходит и у клавиш с именованным Code (скан-код — то, что
	// физически нажато), и у клавиш без имени (Code == KeyUnknown, Rune == 0).
	ScanCode uint16

	// Extended — клавиша «расширенная» (префикс E0 набора 1): правые Ctrl/Alt,
	// стрелки и блок навигации, Win, Apps, Num Enter, Num /. Заполняется на
	// Windows (бит 24 lParam); на остальных платформах всегда false — там
	// сторона и блок клавиши уже различимы по самому ScanCode.
	Extended bool
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
