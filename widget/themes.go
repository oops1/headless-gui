// Package widget — определения тем и переключение тем.
package widget

import "image/color"

// Theme содержит полную цветовую палитру виджетов.
// Каждое поле подписано: какой виджет / элемент интерфейса его использует.
type Theme struct {
	// ═══════════════════════════════════════════════════════════════════════
	// Окно (Window) и панели (Panel, DockPanel, StackPanel)
	// ═══════════════════════════════════════════════════════════════════════

	WindowBG    color.RGBA // Window.Background — основной фон окна
	PanelBG     color.RGBA // Panel.Background, MenuBar.Background — фон панелей и менюбара
	TitleBG     color.RGBA // Window title bar — фон заголовка окна
	TitleText   color.RGBA // Window title bar, MenuBar.TextColor, Dialog.TitleColor — текст заголовка
	Border      color.RGBA // Window, Panel, ListView, Dialog, ScrollView, MenuBar — рамки
	ShadowColor color.RGBA // PopupMenu, Dialog — тень под overlay-элементами

	// ═══════════════════════════════════════════════════════════════════════
	// Кнопка (Button)
	// ═══════════════════════════════════════════════════════════════════════

	BtnBG        color.RGBA // Button — фон кнопки в нормальном состоянии
	BtnBorder    color.RGBA // Button — рамка кнопки
	BtnHoverBG   color.RGBA // Button — фон кнопки при наведении мыши
	BtnPressedBG color.RGBA // Button — фон кнопки при нажатии
	BtnText      color.RGBA // Button — цвет текста кнопки

	// ═══════════════════════════════════════════════════════════════════════
	// Текстовое поле (TextInput)
	// ═══════════════════════════════════════════════════════════════════════

	InputBG          color.RGBA // TextInput — фон поля ввода
	InputBorder      color.RGBA // TextInput — рамка поля
	InputFocus       color.RGBA // TextInput — рамка при фокусе
	InputText        color.RGBA // TextInput — цвет вводимого текста
	InputCaret       color.RGBA // TextInput — цвет каретки (курсора)
	InputPlaceholder color.RGBA // TextInput — цвет текста-подсказки (placeholder)

	// ═══════════════════════════════════════════════════════════════════════
	// Метка (Label)
	// ═══════════════════════════════════════════════════════════════════════

	LabelText color.RGBA // Label, ListView.TextColor — цвет текста метки
	LabelBG   color.RGBA // Label — фон метки (обычно прозрачный)

	// ═══════════════════════════════════════════════════════════════════════
	// Прогресс-бар (ProgressBar)
	// ═══════════════════════════════════════════════════════════════════════

	ProgressBG   color.RGBA // ProgressBar — фон (дорожка)
	ProgressFill color.RGBA // ProgressBar — заполненная часть

	// ═══════════════════════════════════════════════════════════════════════
	// Выпадающий список (Dropdown) и PopupMenu
	// ═══════════════════════════════════════════════════════════════════════

	DropBG     color.RGBA // Dropdown, PopupMenu.Background, MenuBar.ActiveBG — фон выпадающего списка/попапа
	DropBorder color.RGBA // Dropdown, PopupMenu.BorderColor — рамка списка/попапа
	DropText   color.RGBA // Dropdown, PopupMenu.TextColor — текст в списке/попапе
	DropArrow  color.RGBA // Dropdown — стрелка раскрытия ▼
	DropItemBG color.RGBA // Dropdown — фон выделенного пункта в списке

	// ═══════════════════════════════════════════════════════════════════════
	// Чекбокс (CheckBox) и Радиокнопка (RadioButton)
	// ═══════════════════════════════════════════════════════════════════════

	CheckBG      color.RGBA // CheckBox, RadioButton — фон квадратика/кружка
	CheckBorder  color.RGBA // CheckBox, RadioButton — рамка
	CheckMark    color.RGBA // CheckBox, RadioButton — галочка / точка
	CheckHoverBG color.RGBA // CheckBox, RadioButton — фон при наведении
	CheckText    color.RGBA // CheckBox, RadioButton — текст метки рядом

	// ═══════════════════════════════════════════════════════════════════════
	// Вкладки (TabControl)
	// ═══════════════════════════════════════════════════════════════════════

	TabBG         color.RGBA // TabControl — фон неактивной вкладки
	TabActiveBG   color.RGBA // TabControl — фон активной вкладки
	TabBorder     color.RGBA // TabControl — рамка вкладок
	TabText       color.RGBA // TabControl — текст неактивной вкладки
	TabActiveText color.RGBA // TabControl — текст активной вкладки
	TabContentBG  color.RGBA // TabControl — фон области содержимого вкладки

	// ═══════════════════════════════════════════════════════════════════════
	// Ползунок (Slider)
	// ═══════════════════════════════════════════════════════════════════════

	SliderTrackBG color.RGBA // Slider — фон дорожки
	SliderFill    color.RGBA // Slider — заполненная часть (от 0 до value)
	SliderThumb   color.RGBA // Slider — ползунок (ручка)
	SliderBorder  color.RGBA // Slider — рамка дорожки

	// ═══════════════════════════════════════════════════════════════════════
	// Переключатель (ToggleSwitch)
	// ═══════════════════════════════════════════════════════════════════════

	ToggleBG     color.RGBA // ToggleSwitch — фон выключенного переключателя
	ToggleOnBG   color.RGBA // ToggleSwitch — фон включённого переключателя
	ToggleThumb  color.RGBA // ToggleSwitch — кружок-ручка
	ToggleBorder color.RGBA // ToggleSwitch — рамка

	// ═══════════════════════════════════════════════════════════════════════
	// Скроллбар (ScrollView) и списки (ListView, TreeView)
	// ═══════════════════════════════════════════════════════════════════════

	ScrollTrackBG  color.RGBA // ScrollView, ListView — фон трека скроллбара
	ScrollThumbBG  color.RGBA // ScrollView, ListView — ползунок скроллбара
	ListItemHover  color.RGBA // ListView, TreeView, MenuBar.HoverBG, PopupMenu.HoverBG — hover по элементу
	ListItemSelect color.RGBA // ListView, TreeView — фон выделенного элемента

	// ═══════════════════════════════════════════════════════════════════════
	// Дерево (TreeView)
	// ═══════════════════════════════════════════════════════════════════════

	TreeText  color.RGBA // TreeView — цвет текста узлов
	TreeArrow color.RGBA // TreeView — цвет стрелки expand/collapse (▶/▼)

	// ═══════════════════════════════════════════════════════════════════════
	// Диалог (Dialog)
	// ═══════════════════════════════════════════════════════════════════════

	DialogBG      color.RGBA // Dialog — фон окна диалога
	DialogTitleBG color.RGBA // Dialog — фон заголовка диалога
	DialogDim     color.RGBA // Dialog — затемнение фона за диалогом (overlay)

	// ═══════════════════════════════════════════════════════════════════════
	// Разделитель (GridSplitter)
	// ═══════════════════════════════════════════════════════════════════════

	SplitterBG      color.RGBA // GridSplitter — фон разделителя
	SplitterHoverBG color.RGBA // GridSplitter — фон при наведении

	// ═══════════════════════════════════════════════════════════════════════
	// Строка состояния (StatusBar / нижняя панель)
	// ═══════════════════════════════════════════════════════════════════════

	StatusBarBG   color.RGBA // StatusBar — фон строки состояния
	StatusBarText color.RGBA // StatusBar — текст строки состояния

	// ═══════════════════════════════════════════════════════════════════════
	// Заголовок колонок (DataGrid / ListView header)
	// ═══════════════════════════════════════════════════════════════════════

	HeaderBG   color.RGBA // DataGrid/ListView — фон заголовков колонок
	HeaderText color.RGBA // DataGrid/ListView — текст заголовков колонок

	// ═══════════════════════════════════════════════════════════════════════
	// Системные / общие
	// ═══════════════════════════════════════════════════════════════════════

	Accent    color.RGBA // ScrollView hover, Slider, ProgressBar, Focus border — акцентный цвет (Win10 blue)
	Scrollbar color.RGBA // глобальный цвет скроллбара (fallback)
	Disabled  color.RGBA // все виджеты — цвет текста/элементов в отключённом состоянии
}

// ─── Dark Theme (Windows 10 Dark Mode) ──────────────────────────────────────

// DarkTheme возвращает тему Windows 10 Dark Mode.
func DarkTheme() *Theme {
	return &Theme{
		// Окно и панели
		WindowBG:    color.RGBA{R: 30, G: 30, B: 30, A: 255},    // #1E1E1E — тёмный фон окна
		PanelBG:     color.RGBA{R: 45, G: 45, B: 48, A: 255},    // #2D2D30 — фон панелей/менюбара
		TitleBG:     color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7 — синий заголовок
		TitleText:   color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый текст заголовка
		Border:      color.RGBA{R: 63, G: 63, B: 70, A: 255},    // #3F3F46 — серая рамка
		ShadowColor: color.RGBA{R: 0, G: 0, B: 0, A: 80},        // тень

		// Кнопки
		BtnBG:        color.RGBA{R: 51, G: 51, B: 55, A: 255},    // #333337
		BtnBorder:    color.RGBA{R: 63, G: 63, B: 70, A: 255},    // #3F3F46
		BtnHoverBG:   color.RGBA{R: 62, G: 62, B: 66, A: 255},    // #3E3E42
		BtnPressedBG: color.RGBA{R: 0, G: 122, B: 204, A: 255},   // #007ACC — акцент при нажатии
		BtnText:      color.RGBA{R: 241, G: 241, B: 241, A: 255}, // #F1F1F1

		// Текстовое поле
		InputBG:          color.RGBA{R: 37, G: 37, B: 38, A: 255},    // #252526
		InputBorder:      color.RGBA{R: 63, G: 63, B: 70, A: 255},    // #3F3F46
		InputFocus:       color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7
		InputText:        color.RGBA{R: 212, G: 212, B: 212, A: 255}, // #D4D4D4
		InputCaret:       color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7
		InputPlaceholder: color.RGBA{R: 110, G: 110, B: 110, A: 255}, // #6E6E6E

		// Метки
		LabelText: color.RGBA{R: 212, G: 212, B: 212, A: 255}, // #D4D4D4
		LabelBG:   color.RGBA{R: 0, G: 0, B: 0, A: 0},         // прозрачный

		// Прогресс-бар
		ProgressBG:   color.RGBA{R: 37, G: 37, B: 38, A: 255},  // #252526
		ProgressFill: color.RGBA{R: 0, G: 120, B: 215, A: 255}, // #0078D7

		// Выпадающий список / PopupMenu
		DropBG:     color.RGBA{R: 37, G: 37, B: 38, A: 250},    // #252526 — фон попапа
		DropBorder: color.RGBA{R: 63, G: 63, B: 70, A: 255},    // #3F3F46
		DropText:   color.RGBA{R: 212, G: 212, B: 212, A: 255}, // #D4D4D4
		DropArrow:  color.RGBA{R: 180, G: 180, B: 180, A: 255}, // стрелка
		DropItemBG: color.RGBA{R: 0, G: 120, B: 215, A: 200},   // #0078D7 — выделенный пункт

		// CheckBox / RadioButton
		CheckBG:      color.RGBA{R: 37, G: 37, B: 38, A: 255},    // #252526
		CheckBorder:  color.RGBA{R: 136, G: 136, B: 136, A: 255}, // #888888
		CheckMark:    color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белая галочка
		CheckHoverBG: color.RGBA{R: 51, G: 51, B: 55, A: 255},    // #333337
		CheckText:    color.RGBA{R: 212, G: 212, B: 212, A: 255}, // #D4D4D4

		// TabControl
		TabBG:         color.RGBA{R: 45, G: 45, B: 48, A: 255},    // #2D2D30 — неактивная вкладка
		TabActiveBG:   color.RGBA{R: 30, G: 30, B: 30, A: 255},    // #1E1E1E — активная = фон окна
		TabBorder:     color.RGBA{R: 63, G: 63, B: 70, A: 255},    // #3F3F46
		TabText:       color.RGBA{R: 153, G: 153, B: 153, A: 255}, // #999999
		TabActiveText: color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый
		TabContentBG:  color.RGBA{R: 30, G: 30, B: 30, A: 255},    // #1E1E1E

		// Slider
		SliderTrackBG: color.RGBA{R: 51, G: 51, B: 55, A: 255},    // #333337
		SliderFill:    color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7
		SliderThumb:   color.RGBA{R: 204, G: 204, B: 204, A: 255}, // #CCCCCC
		SliderBorder:  color.RGBA{R: 63, G: 63, B: 70, A: 255},    // #3F3F46

		// ToggleSwitch
		ToggleBG:     color.RGBA{R: 51, G: 51, B: 55, A: 255},    // #333337 — OFF
		ToggleOnBG:   color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7 — ON
		ToggleThumb:  color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый кружок
		ToggleBorder: color.RGBA{R: 136, G: 136, B: 136, A: 255}, // #888888

		// ScrollView / ListView / TreeView
		ScrollTrackBG:  color.RGBA{R: 37, G: 37, B: 38, A: 255},  // #252526
		ScrollThumbBG:  color.RGBA{R: 78, G: 78, B: 78, A: 255},  // #4E4E4E
		ListItemHover:  color.RGBA{R: 45, G: 45, B: 48, A: 255},  // #2D2D30
		ListItemSelect: color.RGBA{R: 0, G: 120, B: 215, A: 100}, // #0078D7 полупрозрачный

		// TreeView
		TreeText:  color.RGBA{R: 212, G: 212, B: 212, A: 255}, // #D4D4D4
		TreeArrow: color.RGBA{R: 160, G: 160, B: 160, A: 255}, // #A0A0A0

		// Dialog
		DialogBG:      color.RGBA{R: 45, G: 45, B: 48, A: 255},  // #2D2D30
		DialogTitleBG: color.RGBA{R: 0, G: 120, B: 215, A: 255}, // #0078D7
		DialogDim:     color.RGBA{R: 0, G: 0, B: 0, A: 128},     // полупрозрачное затемнение

		// GridSplitter
		SplitterBG:      color.RGBA{R: 63, G: 63, B: 70, A: 255},  // #3F3F46
		SplitterHoverBG: color.RGBA{R: 0, G: 120, B: 215, A: 255}, // #0078D7 при hover

		// StatusBar
		StatusBarBG:   color.RGBA{R: 0, G: 122, B: 204, A: 255},   // #007ACC — синий (как VS Code)
		StatusBarText: color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый

		// DataGrid / ListView header
		HeaderBG:   color.RGBA{R: 45, G: 45, B: 48, A: 255},    // #2D2D30
		HeaderText: color.RGBA{R: 212, G: 212, B: 212, A: 255}, // #D4D4D4

		// Системные
		Accent:    color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7 — Windows 10 blue
		Scrollbar: color.RGBA{R: 78, G: 78, B: 78, A: 255},    // #4E4E4E
		Disabled:  color.RGBA{R: 109, G: 109, B: 109, A: 255}, // #6D6D6D
	}
}

// ─── Light Theme (Windows 10 Light Mode) ────────────────────────────────────

// LightTheme возвращает тему Windows 10 Light Mode.
func LightTheme() *Theme {
	return &Theme{
		// Окно и панели
		WindowBG:    color.RGBA{R: 243, G: 243, B: 243, A: 255}, // #F3F3F3 — светлый фон окна
		PanelBG:     color.RGBA{R: 248, G: 248, B: 248, A: 255}, // #F8F8F8 — фон панелей/менюбара
		TitleBG:     color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый заголовок (как в Windows)
		TitleText:   color.RGBA{R: 0, G: 0, B: 0, A: 255},       // черный текст заголовка
		Border:      color.RGBA{R: 216, G: 216, B: 216, A: 255}, // #D8D8D8 — светлая рамка
		ShadowColor: color.RGBA{R: 0, G: 0, B: 0, A: 20},        // слабая тень

		// Кнопки
		BtnBG:        color.RGBA{R: 251, G: 251, B: 251, A: 255}, // #FBFBFB
		BtnBorder:    color.RGBA{R: 200, G: 200, B: 200, A: 255}, // #C8C8C8
		BtnHoverBG:   color.RGBA{R: 243, G: 243, B: 243, A: 255}, // #F3F3F3
		BtnPressedBG: color.RGBA{R: 229, G: 229, B: 229, A: 255}, // #E5E5E5
		BtnText:      color.RGBA{R: 32, G: 32, B: 32, A: 255},    // #202020

		// Текстовое поле
		InputBG:          color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый
		InputBorder:      color.RGBA{R: 200, G: 200, B: 200, A: 255}, // #C8C8C8
		InputFocus:       color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7
		InputText:        color.RGBA{R: 32, G: 32, B: 32, A: 255},    // #202020
		InputCaret:       color.RGBA{R: 0, G: 0, B: 0, A: 255},       // черный
		InputPlaceholder: color.RGBA{R: 140, G: 140, B: 140, A: 255}, // #8C8C8C

		// Метки
		LabelText: color.RGBA{R: 32, G: 32, B: 32, A: 255}, // #202020
		LabelBG:   color.RGBA{R: 0, G: 0, B: 0, A: 0},      // прозрачный

		// Прогресс-бар
		ProgressBG:   color.RGBA{R: 230, G: 230, B: 230, A: 255}, // #E6E6E6
		ProgressFill: color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7

		// Выпадающий список / PopupMenu
		DropBG:     color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый
		DropBorder: color.RGBA{R: 200, G: 200, B: 200, A: 255}, // #C8C8C8
		DropText:   color.RGBA{R: 32, G: 32, B: 32, A: 255},    // #202020
		DropArrow:  color.RGBA{R: 100, G: 100, B: 100, A: 255}, // #646464
		DropItemBG: color.RGBA{R: 229, G: 243, B: 255, A: 255}, // #E5F3FF

		// CheckBox / RadioButton
		CheckBG:      color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый
		CheckBorder:  color.RGBA{R: 120, G: 120, B: 120, A: 255}, // #787878
		CheckMark:    color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белая галочка на синем фоне
		CheckHoverBG: color.RGBA{R: 243, G: 243, B: 243, A: 255}, // #F3F3F3
		CheckText:    color.RGBA{R: 32, G: 32, B: 32, A: 255},    // #202020

		// TabControl
		TabBG:         color.RGBA{R: 248, G: 248, B: 248, A: 255}, // #F8F8F8
		TabActiveBG:   color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый — активная
		TabBorder:     color.RGBA{R: 216, G: 216, B: 216, A: 255}, // #D8D8D8
		TabText:       color.RGBA{R: 100, G: 100, B: 100, A: 255}, // #646464
		TabActiveText: color.RGBA{R: 0, G: 0, B: 0, A: 255},       // черный
		TabContentBG:  color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый

		// Slider
		SliderTrackBG: color.RGBA{R: 230, G: 230, B: 230, A: 255}, // #E6E6E6
		SliderFill:    color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7
		SliderThumb:   color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый
		SliderBorder:  color.RGBA{R: 180, G: 180, B: 180, A: 255}, // #B4B4B4

		// ToggleSwitch
		ToggleBG:     color.RGBA{R: 200, G: 200, B: 200, A: 255}, // #C8C8C8 — OFF
		ToggleOnBG:   color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7 — ON
		ToggleThumb:  color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый кружок
		ToggleBorder: color.RGBA{R: 160, G: 160, B: 160, A: 255}, // #A0A0A0

		// ScrollView / ListView / TreeView
		ScrollTrackBG:  color.RGBA{R: 245, G: 245, B: 245, A: 255}, // #F5F5F5
		ScrollThumbBG:  color.RGBA{R: 200, G: 200, B: 200, A: 255}, // #C8C8C8
		ListItemHover:  color.RGBA{R: 242, G: 242, B: 242, A: 255}, // #F2F2F2
		ListItemSelect: color.RGBA{R: 0, G: 120, B: 215, A: 60},    // #0078D7 полупрозрачный

		// TreeView
		TreeText:  color.RGBA{R: 32, G: 32, B: 32, A: 255},    // #202020
		TreeArrow: color.RGBA{R: 100, G: 100, B: 100, A: 255}, // #646464

		// Dialog
		DialogBG:      color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый
		DialogTitleBG: color.RGBA{R: 255, G: 255, B: 255, A: 255}, // белый
		DialogDim:     color.RGBA{R: 0, G: 0, B: 0, A: 60},        // слабое затемнение

		// GridSplitter
		SplitterBG:      color.RGBA{R: 216, G: 216, B: 216, A: 255}, // #D8D8D8
		SplitterHoverBG: color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7

		// StatusBar
		StatusBarBG:   color.RGBA{R: 240, G: 240, B: 240, A: 255}, // #F0F0F0
		StatusBarText: color.RGBA{R: 32, G: 32, B: 32, A: 255},    // #202020

		// DataGrid / ListView header
		HeaderBG:   color.RGBA{R: 245, G: 245, B: 245, A: 255}, // #F5F5F5
		HeaderText: color.RGBA{R: 32, G: 32, B: 32, A: 255},    // #202020

		// Системные
		Accent:    color.RGBA{R: 0, G: 120, B: 215, A: 255},   // #0078D7
		Scrollbar: color.RGBA{R: 200, G: 200, B: 200, A: 255}, // #C8C8C8
		Disabled:  color.RGBA{R: 170, G: 170, B: 170, A: 255}, // #AAAAAA
	}
}

// Themeable — виджет, поддерживающий применение темы.
type Themeable interface {
	ApplyTheme(t *Theme)
}

// ApplyGlobalTheme обновляет глобальные цвета по умолчанию (используются в New*-конструкторах).
// Вызывается engine.SetTheme перед рекурсивным обходом дерева виджетов.
func ApplyGlobalTheme(t *Theme) {
	// Окно / панели
	win10.WindowBG = t.WindowBG
	win10.PanelBG = t.PanelBG
	win10.TitleBG = t.TitleBG
	win10.TitleText = t.TitleText
	win10.Border = t.Border
	win10.ShadowColor = t.ShadowColor

	// Кнопки
	win10.BtnBG = t.BtnBG
	win10.BtnBorder = t.BtnBorder
	win10.BtnHoverBG = t.BtnHoverBG
	win10.BtnPressedBG = t.BtnPressedBG
	win10.BtnText = t.BtnText

	// Текстовое поле
	win10.InputBG = t.InputBG
	win10.InputBorder = t.InputBorder
	win10.InputFocus = t.InputFocus
	win10.InputText = t.InputText
	win10.InputCaret = t.InputCaret
	win10.InputPlaceholder = t.InputPlaceholder

	// Метки
	win10.LabelText = t.LabelText
	win10.LabelBG = t.LabelBG

	// Прогресс-бар
	win10.ProgressBG = t.ProgressBG
	win10.ProgressFill = t.ProgressFill

	// Dropdown / PopupMenu
	win10.DropBG = t.DropBG
	win10.DropBorder = t.DropBorder
	win10.DropText = t.DropText
	win10.DropArrow = t.DropArrow
	win10.DropItemBG = t.DropItemBG

	// CheckBox / RadioButton
	win10.CheckBG = t.CheckBG
	win10.CheckBorder = t.CheckBorder
	win10.CheckMark = t.CheckMark
	win10.CheckHoverBG = t.CheckHoverBG
	win10.CheckText = t.CheckText

	// TabControl
	win10.TabBG = t.TabBG
	win10.TabActiveBG = t.TabActiveBG
	win10.TabBorder = t.TabBorder
	win10.TabText = t.TabText
	win10.TabActiveText = t.TabActiveText
	win10.TabContentBG = t.TabContentBG

	// Slider
	win10.SliderTrackBG = t.SliderTrackBG
	win10.SliderFill = t.SliderFill
	win10.SliderThumb = t.SliderThumb
	win10.SliderBorder = t.SliderBorder

	// ToggleSwitch
	win10.ToggleBG = t.ToggleBG
	win10.ToggleOnBG = t.ToggleOnBG
	win10.ToggleThumb = t.ToggleThumb
	win10.ToggleBorder = t.ToggleBorder

	// ScrollView / ListView / TreeView
	win10.ScrollTrackBG = t.ScrollTrackBG
	win10.ScrollThumbBG = t.ScrollThumbBG
	win10.ListItemHover = t.ListItemHover
	win10.ListItemSelect = t.ListItemSelect

	// TreeView
	win10.TreeText = t.TreeText
	win10.TreeArrow = t.TreeArrow

	// Dialog
	win10.DialogBG = t.DialogBG
	win10.DialogTitleBG = t.DialogTitleBG
	win10.DialogDim = t.DialogDim

	// GridSplitter
	win10.SplitterBG = t.SplitterBG
	win10.SplitterHoverBG = t.SplitterHoverBG

	// StatusBar
	win10.StatusBarBG = t.StatusBarBG
	win10.StatusBarText = t.StatusBarText

	// DataGrid / ListView header
	win10.HeaderBG = t.HeaderBG
	win10.HeaderText = t.HeaderText

	// Системные
	win10.Accent = t.Accent
	win10.Scrollbar = t.Scrollbar
	win10.Disabled = t.Disabled
}